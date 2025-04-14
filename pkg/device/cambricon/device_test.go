/*
Copyright 2024 The HAMi Authors.

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

package cambricon

import (
	"context"
	"flag"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/Project-HAMi/HAMi/pkg/util"
	"github.com/Project-HAMi/HAMi/pkg/util/client"
)

func Test_GetNodeDevices(t *testing.T) {
	config := CambriconConfig{
		ResourceMemoryName: "cambricon.com/mlu.smlu.vmemory",
		ResourceCoreName:   "cambricon.com/mlu.smlu.vcore",
	}
	InitMLUDevice(config)
	tests := []struct {
		name string
		args corev1.Node
		want []*util.DeviceInfo
	}{
		{
			name: "test with vaild configuration",
			args: corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Status: corev1.NodeStatus{
					Capacity: corev1.ResourceList{
						"cambricon.com/mlu.smlu.vcore":   *resource.NewQuantity(1, resource.DecimalSI),
						"cambricon.com/mlu.smlu.vmemory": *resource.NewQuantity(1, resource.DecimalSI),
					},
				},
			},
			want: []*util.DeviceInfo{
				{
					Index:   0,
					ID:      "test-cambricon-mlu-0",
					Count:   int32(100),
					Devmem:  int32(25600),
					Devcore: int32(100),
					Type:    CambriconMLUDevice,
					Numa:    0,
					Health:  true,
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dev := CambriconDevices{}
			result, err := dev.GetNodeDevices(test.args)
			if err != nil {
				assert.NoError(t, err)
			}
			for k, v := range test.want {
				assert.Equal(t, v, result[k])
			}
		})
	}
}

func Test_MutateAdmission(t *testing.T) {
	config := CambriconConfig{
		ResourceMemoryName: "cambricon.com/mlu.smlu.vmemory",
		ResourceCoreName:   "cambricon.com/mlu.smlu.vcore",
		ResourceCountName:  "cambricon.com/mlu",
	}
	InitMLUDevice(config)
	tests := []struct {
		name string
		args struct {
			ctr corev1.Container
			pod corev1.Pod
		}
		want bool
		err  error
	}{
		{
			name: "set to resources limits",
			args: struct {
				ctr corev1.Container
				pod corev1.Pod
			}{
				ctr: corev1.Container{
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							"cambricon.com/mlu": resource.MustParse("1"),
						},
					},
				},
				pod: corev1.Pod{},
			},
			want: true,
			err:  nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dev := CambriconDevices{}
			result, _ := dev.MutateAdmission(&test.args.ctr, &test.args.pod)
			assert.Equal(t, result, test.want)
		})
	}
}

func Test_ParseConfig(t *testing.T) {
	tests := []struct {
		name string
		args flag.FlagSet
	}{
		{
			name: "test",
			args: flag.FlagSet{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ParseConfig(&test.args)
		})
	}
}

func Test_CheckType(t *testing.T) {
	dev := CambriconDevices{}
	tests := []struct {
		name string
		args struct {
			annos map[string]string
			d     util.DeviceUsage
			n     util.ContainerDeviceRequest
		}
		want1 bool
		want2 bool
	}{
		{
			name: "the same type",
			args: struct {
				annos map[string]string
				d     util.DeviceUsage
				n     util.ContainerDeviceRequest
			}{
				annos: map[string]string{},
				d:     util.DeviceUsage{},
				n: util.ContainerDeviceRequest{
					Type: dev.CommonWord(),
				},
			},
			want1: true,
			want2: true,
		},
		{
			name: "the different type",
			args: struct {
				annos map[string]string
				d     util.DeviceUsage
				n     util.ContainerDeviceRequest
			}{
				annos: map[string]string{},
				d:     util.DeviceUsage{},
				n: util.ContainerDeviceRequest{
					Type: "TEST",
				},
			},
			want1: false,
			want2: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result1, result2, _ := dev.CheckType(test.args.annos, test.args.d, test.args.n)
			assert.Equal(t, result1, test.want1)
			assert.Equal(t, result2, test.want2)
		})
	}
}

func Test_CheckUUID(t *testing.T) {
	tests := []struct {
		name string
		args struct {
			annos map[string]string
			d     util.DeviceUsage
		}
		want bool
	}{
		{
			name: "don't set UserUUID,NoUserUUID and annotation",
			args: struct {
				annos map[string]string
				d     util.DeviceUsage
			}{annos: map[string]string{},
				d: util.DeviceUsage{},
			},
			want: true,
		},
		{
			name: "set UserUUID and annotation, don't set NoUserUUID",
			args: struct {
				annos map[string]string
				d     util.DeviceUsage
			}{
				annos: map[string]string{
					"cambricon.com/use-gpuuuid": "test123,111",
				},
				d: util.DeviceUsage{
					ID: "test123",
				},
			},
			want: true,
		},
		{
			name: "don't set UserUUID, set NoUserUUID and annotation",
			args: struct {
				annos map[string]string
				d     util.DeviceUsage
			}{
				annos: map[string]string{
					"cambricon.com/nouse-gpuuuid": "test123,111",
				},
				d: util.DeviceUsage{
					ID: "test123",
				},
			},
			want: false,
		},
		{
			name: "set UserUUID, don't set NoUserUUID,annotation and device not match",
			args: struct {
				annos map[string]string
				d     util.DeviceUsage
			}{
				annos: map[string]string{
					"cambricon.com/nouse-gpuuuid": "test123,111",
				},
				d: util.DeviceUsage{
					ID: "test456",
				},
			},
			want: true,
		},
		{
			name: "don't set UserUUID,set NoUserUUID,annotation and device not match",
			args: struct {
				annos map[string]string
				d     util.DeviceUsage
			}{
				annos: map[string]string{
					"cambricon.com/use-gpuuuid": "test123,111",
				},
				d: util.DeviceUsage{
					ID: "test456",
				},
			},
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dev := CambriconDevices{}
			result := dev.CheckUUID(test.args.annos, test.args.d)
			assert.Equal(t, result, test.want)
		})
	}
}

func Test_GenerateResourceRequests(t *testing.T) {
	tests := []struct {
		name string
		args corev1.Container
		want util.ContainerDeviceRequest
	}{
		{
			name: "don't set to limits and request",
			args: corev1.Container{
				Resources: corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{},
					Requests: corev1.ResourceList{},
				},
			},
			want: util.ContainerDeviceRequest{
				Nums: 0,
			},
		},
		{
			name: "resourcecoresname,resourcecountname and resourcememoryname set to limits and request",
			args: corev1.Container{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cambricon.com/mlu":              resource.MustParse("1"),
						"cambricon.com/mlu.smlu.vmemory": resource.MustParse("1000"),
						"cambricon.com/mlu.smlu.vcore":   resource.MustParse("2"),
					},
					Requests: corev1.ResourceList{
						"cambricon.com/mlu":              resource.MustParse("1"),
						"cambricon.com/mlu.smlu.vmemory": resource.MustParse("1000"),
						"cambricon.com/mlu.smlu.vcore":   resource.MustParse("2"),
					},
				},
			},
			want: util.ContainerDeviceRequest{
				Nums:             int32(1),
				Type:             CambriconMLUDevice,
				Memreq:           int32(256000),
				MemPercentagereq: int32(0),
				Coresreq:         int32(2),
			},
		},
		{
			name: "resourcememoryname don't set to limits and requests",
			args: corev1.Container{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cambricon.com/mlu":              resource.MustParse("1"),
						"cambricon.com/mlu.smlu.vmemory": resource.MustParse("1000"),
					},
					Requests: corev1.ResourceList{
						"cambricon.com/mlu":              resource.MustParse("1"),
						"cambricon.com/mlu.smlu.vmemory": resource.MustParse("1000"),
					},
				},
			},
			want: util.ContainerDeviceRequest{
				Nums:             int32(1),
				Type:             CambriconMLUDevice,
				Memreq:           int32(256000),
				MemPercentagereq: int32(0),
				Coresreq:         int32(100),
			},
		},
		{
			name: "resourcecoresname don't set to limits and requests",
			args: corev1.Container{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cambricon.com/mlu":            resource.MustParse("1"),
						"cambricon.com/mlu.smlu.vcore": resource.MustParse("2"),
					},
					Requests: corev1.ResourceList{
						"cambricon.com/mlu":            resource.MustParse("1"),
						"cambricon.com/mlu.smlu.vcore": resource.MustParse("2"),
					},
				},
			},
			want: util.ContainerDeviceRequest{
				Nums:             int32(1),
				Type:             CambriconMLUDevice,
				Memreq:           int32(0),
				MemPercentagereq: int32(100),
				Coresreq:         int32(2),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := CambriconConfig{
				ResourceMemoryName: "cambricon.com/mlu.smlu.vmemory",
				ResourceCoreName:   "cambricon.com/mlu.smlu.vcore",
				ResourceCountName:  "cambricon.com/mlu",
			}
			InitMLUDevice(config)
			dev := CambriconDevices{}
			result := dev.GenerateResourceRequests(&test.args)
			assert.Equal(t, test.want, result)
		})
	}
}

func Test_PatchAnnotations(t *testing.T) {
	tests := []struct {
		name string
		args struct {
			annoinput map[string]string
			pd        util.PodDevices
		}
		want map[string]string
	}{
		{
			name: "exist device",
			args: struct {
				annoinput map[string]string
				pd        util.PodDevices
			}{
				annoinput: map[string]string{},
				pd: util.PodDevices{
					CambriconMLUDevice: util.PodSingleDevice{
						[]util.ContainerDevice{
							{
								Idx:       0,
								UUID:      "device-0",
								Type:      "MLU",
								Usedcores: 1,
								Usedmem:   256000,
							},
						},
					},
				},
			},
			want: map[string]string{
				"CAMBRICON_DSMLU_ASSIGHED":                  "false",
				"CAMBRICON_DSMLU_PROFILE":                   "0_1_1000",
				"hami.io/cambricon-mlu-devices-to-allocate": "device-0,MLU,256000,1:;",
				"hami.io/cambricon-mlu-devices-allocated":   "device-0,MLU,256000,1:;",
			},
		},
		{
			name: "no device",
			args: struct {
				annoinput map[string]string
				pd        util.PodDevices
			}{
				annoinput: map[string]string{},
				pd:        util.PodDevices{},
			},
			want: map[string]string{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dev := CambriconDevices{}
			result := dev.PatchAnnotations(&test.args.annoinput, test.args.pd)
			assert.Equal(t, len(test.want), len(result), "Expected length of result to match want")
			for k, v := range test.want {
				assert.Equal(t, v, result[k], "pod add annotation key [%s], values is [%s]", k, result[k])
			}
		})
	}
}

func Test_setNodeLock(t *testing.T) {
	client.InitGlobalClient(client.WithBurst(10), client.WithQPS(5.0))
	tests := []struct {
		name      string
		node      corev1.Node
		expectErr bool
		expectMsg string
	}{
		{
			name: "node is locked",
			node: corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-01",
					Annotations: map[string]string{
						"cambricon.com/dsmlu.lock": "test123",
					},
				},
			},
			expectErr: true,
			expectMsg: "node node-01 is locked",
		},
		{
			name: "set node lock",
			node: corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "node-02",
					Annotations: map[string]string{},
				},
			},
			expectErr: false,
		},
	}

	client.KubeClient = fake.NewSimpleClientset()
	k8sClient := client.GetClient()
	if k8sClient != nil {

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				ctx := context.Background()

				defer func() {
					if tt.node.Name != "" {
						// Delete the node to clean up
						err := k8sClient.CoreV1().Nodes().Delete(ctx, tt.node.Name, metav1.DeleteOptions{})
						if err != nil {
							t.Errorf("failed to delete node %s: %v", tt.node.Name, err)
						}
					}
				}()

				_, err := k8sClient.CoreV1().Nodes().Create(ctx, &tt.node, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("failed to create node %s: %v", tt.node.Name, err)
				}

				dev := CambriconDevices{}
				err = dev.setNodeLock(&tt.node)

				if tt.expectErr {
					if err == nil {
						t.Errorf("expected error but got none")
					} else if !strings.Contains(err.Error(), tt.expectMsg) {
						t.Errorf("expected error to contain '%s' but got '%s'", tt.expectMsg, err.Error())
					}
				} else {
					if err != nil {
						t.Errorf("did not expect error but got %v", err)
					}
				}
			})
		}
	}
}

// Setup function to initialize resources for each test case.
func setupTest(t *testing.T) (*corev1.Node, *corev1.Pod, func(), *fake.Clientset) {
	ctx := context.Background()

	clientset := fake.NewSimpleClientset()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceName(MLUResourceCount):  resource.MustParse("2"),
				corev1.ResourceName(MLUResourceMemory): resource.MustParse("4096"),
				corev1.ResourceName(MLUResourceCores):  resource.MustParse("200"),
			},
		},
	}
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "test-container",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceName(MLUResourceCount):  resource.MustParse("1"),
						corev1.ResourceName(MLUResourceMemory): resource.MustParse("2048"),
						corev1.ResourceName(MLUResourceCores):  resource.MustParse("100"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceName(MLUResourceCount):  resource.MustParse("1"),
						corev1.ResourceName(MLUResourceMemory): resource.MustParse("2048"),
						corev1.ResourceName(MLUResourceCores):  resource.MustParse("100"),
					},
				},
			}},
		},
	}

	config := CambriconConfig{
		ResourceCountName:  MLUResourceCount,
		ResourceMemoryName: MLUResourceMemory,
		ResourceCoreName:   MLUResourceCores,
	}
	InitMLUDevice(config)

	_, err := clientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	return node, pod, func() {
		clientset.CoreV1().Nodes().Delete(ctx, node.Name, metav1.DeleteOptions{})
	}, clientset
}

func Test_LockNode(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		wantErr     bool
	}{
		{
			name:        "node is not locked",
			annotations: map[string]string{},
			wantErr:     false,
		},
		{
			name: "node is already locked within 2 minutes",
			annotations: map[string]string{
				DsmluLockTime: time.Now().Add(-time.Minute).Format(time.RFC3339),
			},
			wantErr: true,
		},
		{
			name: "lock time expired (more than 2 minutes)",
			annotations: map[string]string{
				DsmluLockTime: time.Now().Add(-time.Hour).Format(time.RFC3339),
			},
			wantErr: false,
		},
		{
			name: "invalid lock time format",
			annotations: map[string]string{
				DsmluLockTime: "invalid-format",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, pod, teardown, clientset := setupTest(t)
			client.KubeClient = clientset
			defer teardown()

			// Set up the node with the specified annotations.
			node.Annotations = tt.annotations

			dev := InitMLUDevice(CambriconConfig{
				ResourceCountName:  MLUResourceCount,
				ResourceMemoryName: MLUResourceMemory,
				ResourceCoreName:   MLUResourceCores,
			})

			err := dev.LockNode(node, pod)
			if (err != nil) != tt.wantErr {
				t.Errorf("LockNode() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Optionally check if the node was correctly patched with the lock annotation.
			if !tt.wantErr {
				fetchedNode, _ := clientset.CoreV1().Nodes().Get(context.TODO(), node.Name, metav1.GetOptions{})
				if _, ok := fetchedNode.Annotations[DsmluLockTime]; !ok && !tt.wantErr {
					t.Error("Expected node to be locked but it wasn't")
				}
			}
		})
	}
}

func Test_ReleaseNodeLock(t *testing.T) {
	tests := []struct {
		name string
		args struct {
			node corev1.Node
			pod  corev1.Pod
		}
		err error
	}{
		{
			name: "no annation",
			args: struct {
				node corev1.Node
				pod  corev1.Pod
			}{
				node: corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-01",
					},
				},
				pod: corev1.Pod{},
			},
			err: nil,
		},
		{
			name: "annation no lock value",
			args: struct {
				node corev1.Node
				pod  corev1.Pod
			}{
				node: corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-02",
						Annotations: map[string]string{
							"test": "test123",
						},
					},
				},
				pod: corev1.Pod{},
			},
			err: nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dev := CambriconDevices{}
			result := dev.ReleaseNodeLock(&test.args.node, &test.args.pod)
			assert.Equal(t, test.err, result)
		})
	}
}
