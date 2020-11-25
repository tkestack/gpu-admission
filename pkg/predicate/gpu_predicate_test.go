/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */
package predicate

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"tkestack.io/gpu-admission/pkg/util"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

type podRawInfo struct {
	Name       string
	UID        string
	Containers []containerRawInfo
}

type containerRawInfo struct {
	Name   string
	Cores  int
	Memory int
}

const (
	deviceCount = 2
	totalMemory = 8
	namespace   = "test-ns"
)

func TestDeviceFilter(t *testing.T) {
	k8sClient := fake.NewSimpleClientset()
	gpuFilter, err := NewGPUFilter(k8sClient)
	if err != nil {
		t.Fatalf("failed to create new gpuFilter due to %v", err)
	}

	nodeList := []corev1.Node{}
	for i := 0; i < 3; i++ {
		n := corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "testnode" + strconv.Itoa(i),
			},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					util.VCoreAnnotation:   resource.MustParse(fmt.Sprintf("%d", deviceCount*util.HundredCore)),
					util.VMemoryAnnotation: resource.MustParse(fmt.Sprintf("%d", totalMemory)),
				},
			},
		}
		nodeList = append(nodeList, n)
	}
	testCases := []podRawInfo{
		{
			Name: "pod-0",
			UID:  "uid-0",
			Containers: []containerRawInfo{
				{
					Name:   "container-0",
					Cores:  10,
					Memory: 1,
				},
				{
					Name:   "container-1",
					Cores:  10,
					Memory: 1,
				},
			},
		},
		{
			Name: "pod-1",
			UID:  "uid-1",
			Containers: []containerRawInfo{
				{
					Name:   "container-0",
					Cores:  100,
					Memory: 3,
				},
				{
					Name:   "container-1",
					Cores:  80,
					Memory: 3,
				},
			},
		},
		{
			Name: "pod-2",
			UID:  "uid-2",
			Containers: []containerRawInfo{
				{
					Name:   "container-0",
					Cores:  200,
					Memory: 10,
				},
				{
					Name: "container-without-gpu",
				},
			},
		},
		{
			Name: "pod-3",
			UID:  "uid-3",
			Containers: []containerRawInfo{
				{
					Name:   "container-1",
					Cores:  10,
					Memory: 2,
				},
				{
					Name: "container-without-gpu",
				},
			},
		},
	}

	testResults := []struct {
		nodeName string
	}{
		{
			nodeName: "testnode0",
		},
		{
			nodeName: "testnode1",
		},
		{
			nodeName: "testnode2",
		},
		{
			nodeName: "testnode0",
		},
	}

	for i, cs := range testCases {
		containers := []corev1.Container{}
		for _, c := range cs.Containers {
			container := corev1.Container{
				Name: c.Name,
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						util.VCoreAnnotation:   resource.MustParse(fmt.Sprintf("%d", c.Cores)),
						util.VMemoryAnnotation: resource.MustParse(fmt.Sprintf("%d", c.Memory)),
					},
				},
			}
			containers = append(containers, container)
		}
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        cs.Name,
				UID:         k8stypes.UID(cs.UID),
				Annotations: make(map[string]string),
			},
			Spec: corev1.PodSpec{
				Containers: containers,
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
			},
		}
		pod, _ = k8sClient.CoreV1().Pods(namespace).Create(context.Background(), pod, metav1.CreateOptions{})

		// wait for podLister to sync
		time.Sleep(time.Second * 2)

		nodes, failedNodes, err := gpuFilter.deviceFilter(pod, nodeList)
		if err != nil {
			t.Fatalf("deviceFilter return err: %v", err)
		}
		if len(nodes) != 1 {
			t.Fatalf("deviceFilter should return exact one node: %v, failedNodes: %v", nodes, failedNodes)
		}

		if nodes[0].Name != testResults[i].nodeName {
			t.Fatalf("choose the wrong node: %s, expect: %s", nodes[0].Name, testResults[i].nodeName)
		}
		// wait for podLister to sync
		time.Sleep(time.Second * 2)

		// get the latest pod and bind it to the node
		pod, _ = k8sClient.CoreV1().Pods(namespace).Get(context.Background(), pod.Name, metav1.GetOptions{})
		pod.Spec.NodeName = nodes[0].Name
		pod.Status.Phase = corev1.PodRunning
		pod, _ = k8sClient.CoreV1().Pods("test-ns").Update(context.Background(), pod, metav1.UpdateOptions{})
	}

}
