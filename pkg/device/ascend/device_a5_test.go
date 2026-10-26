/*
Copyright 2026 The HAMi Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ascend

import (
	"fmt"
	"testing"

	"gopkg.in/yaml.v2"
	"gotest.tools/v3/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Project-HAMi/HAMi/pkg/device"
)

// ascend950DTConfig 是 device-config 里 A5 Atlas 850E 的条目原文：整卡芯片，
// 不配 templates / aiCPU / resourceCoreName / vcannRT / superPod。
const ascend950DTConfig = `
chipName: Ascend950DT
commonWord: Ascend950DT
resourceName: huawei.com/Ascend950DT
resourceMemoryName: huawei.com/Ascend950DT-memory
memoryAllocatable: 98304
memoryCapacity: 98304
aiCore: 32
`

func newAscend950DT(t *testing.T) *Devices {
	t.Helper()
	var cfg VNPUConfig
	assert.NilError(t, yaml.Unmarshal([]byte(ascend950DTConfig), &cfg))
	return &Devices{config: cfg}
}

// a5NodeDevices 按 ascend-device-plugin 对 A5 的注册结果构造 8 张空闲卡：
// 每卡 Count=1（整卡）、98304 MB、32 AI Core，无 NetworkID。
func a5NodeDevices() []*device.DeviceUsage {
	devs := make([]*device.DeviceUsage, 8)
	for i := range devs {
		devs[i] = &device.DeviceUsage{
			ID:        fmt.Sprintf("a5-die-%d", i),
			Index:     uint(i),
			Type:      "Ascend950DT",
			Count:     1,
			Totalmem:  98304,
			Totalcore: 32,
			Health:    true,
		}
	}
	return devs
}

// S2：Ascend950DT 只能整卡用——整卡请求准入通过、部分显存请求在准入期被拒；
// 多卡请求走通用路径（不进 910C 成对逻辑），偶数、奇数张都能调度。
func TestAscend950DT_WholeCardOnly(t *testing.T) {
	tests := []struct {
		name     string
		limits   corev1.ResourceList
		wantErr  bool
		wantNums int // >0 时继续走 GenerateResourceRequests + Fit，期望拿到这么多张卡
	}{
		{
			name:     "1 whole card, no memory resource",
			limits:   corev1.ResourceList{"huawei.com/Ascend950DT": resource.MustParse("1")},
			wantNums: 1,
		},
		{
			name:     "1 whole card, explicit full memory",
			limits:   corev1.ResourceList{"huawei.com/Ascend950DT": resource.MustParse("1"), "huawei.com/Ascend950DT-memory": resource.MustParse("98304")},
			wantNums: 1,
		},
		{
			name:    "half card memory rejected (no split templates)",
			limits:  corev1.ResourceList{"huawei.com/Ascend950DT": resource.MustParse("1"), "huawei.com/Ascend950DT-memory": resource.MustParse("49152")},
			wantErr: true,
		},
		{
			name:     "8 cards fit the whole node",
			limits:   corev1.ResourceList{"huawei.com/Ascend950DT": resource.MustParse("8")},
			wantNums: 8,
		},
		{
			// 910C 成对逻辑下奇数张会被准入拒掉或 Fit 凑不齐；A5 单芯一卡，3 张必须原样可调度。
			name:     "odd count (3) schedulable",
			limits:   corev1.ResourceList{"huawei.com/Ascend950DT": resource.MustParse("3")},
			wantNums: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dev := newAscend950DT(t)
			ctr := corev1.Container{Resources: corev1.ResourceRequirements{
				Limits:   tc.limits.DeepCopy(),
				Requests: tc.limits.DeepCopy(),
			}}
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "a5-pod", Annotations: map[string]string{}}}

			found, err := dev.MutateAdmission(&ctr, pod)
			assert.Assert(t, found)
			if tc.wantErr {
				assert.Assert(t, err != nil, "expected sub-card request to be rejected at admission")
				return
			}
			assert.NilError(t, err)
			// 不得像 910C 那样把请求张数改写成偶数。
			gotCount := ctr.Resources.Limits["huawei.com/Ascend950DT"]
			assert.Equal(t, gotCount.Value(), int64(tc.wantNums))

			req, err := dev.GenerateResourceRequests(&ctr)
			assert.NilError(t, err)
			assert.Equal(t, req.Nums, int64(tc.wantNums))
			assert.Equal(t, req.Type, "Ascend950DT")

			fit, result, reason := dev.Fit(a5NodeDevices(), req, pod, &device.NodeInfo{ID: "a5-node"}, &device.PodDevices{})
			assert.Assert(t, fit, "Fit rejected: %s", reason)
			got := result["Ascend950DT"]
			assert.Equal(t, len(got), tc.wantNums)
			for _, d := range got {
				assert.Equal(t, d.Usedmem, int64(98304), "device %s should be taken as a whole card", d.UUID)
			}
		})
	}
}

func TestAscend950DT_RejectSlicing(t *testing.T) {
	for _, mode := range []string{"", VNPUModeTemplate, VNPUModeHamiCore, VNPUModeENPU, "vcann-rt"} {
		for _, memory := range []string{"0", "49152", "98303", "98305"} {
			t.Run(mode+"/"+memory, func(t *testing.T) {
				dev := newAscend950DT(t)
				pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{VNPUModeAnnotation: mode}}}
				ctr := &corev1.Container{Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
					"huawei.com/Ascend950DT":        resource.MustParse("1"),
					"huawei.com/Ascend950DT-memory": resource.MustParse(memory),
				}}}
				_, err := dev.MutateAdmission(ctr, pod)
				assert.Assert(t, err != nil, "partial or unsupported mode request must be rejected")
			})
		}
		if mode != "" && mode != VNPUModeTemplate {
			t.Run(mode+"/whole-card", func(t *testing.T) {
				dev := newAscend950DT(t)
				pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{VNPUModeAnnotation: mode}}}
				ctr := &corev1.Container{Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
					"huawei.com/Ascend950DT": resource.MustParse("1"),
				}}}
				_, err := dev.MutateAdmission(ctr, pod)
				assert.Assert(t, err != nil, "soft slicing is not supported even with full memory")
			})
		}
	}
}

func TestAscend950DT_WholeCardWithGlobalSoftSliceMode(t *testing.T) {
	for _, mode := range []string{VNPUModeTemplate, VNPUModeHamiCore, VNPUModeENPU} {
		t.Run(mode, func(t *testing.T) {
			dev := newAscend950DT(t)
			dev.vnpuMode = mode
			ctr := &corev1.Container{Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
				"huawei.com/Ascend950DT": resource.MustParse("3"),
			}}}
			req, err := dev.GenerateResourceRequests(ctr)
			assert.NilError(t, err)
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "a5-whole"}}
			fit, result, reason := dev.Fit(a5NodeDevices(), req, pod, nil, nil)
			assert.Assert(t, fit, "whole card fit with global mode %s: %s", mode, reason)
			assert.Equal(t, len(result[Ascend950DTType]), 3)
			for _, d := range result[Ascend950DTType] {
				assert.Equal(t, d.Usedmem, int64(98304))
			}
			req.Memreq = 49152
			fit, _, _ = dev.Fit(a5NodeDevices(), req, pod, nil, nil)
			assert.Assert(t, !fit, "Fit must also reject partial memory when admission was bypassed")
			req.Memreq = 98304
			pod.Annotations = map[string]string{VNPUModeAnnotation: VNPUModeHamiCore}
			fit, _, _ = dev.Fit(a5NodeDevices(), req, pod, nil, nil)
			assert.Assert(t, !fit, "Fit must reject explicit soft slicing")
		})
	}
}
