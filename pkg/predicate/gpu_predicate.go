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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog"
	extenderv1 "k8s.io/kube-scheduler/extender/v1"

	"tkestack.io/gpu-admission/pkg/algorithm"
	"tkestack.io/gpu-admission/pkg/device"
	"tkestack.io/gpu-admission/pkg/util"
)

type GPUFilter struct {
	kubeClient kubernetes.Interface
	nodeLister listerv1.NodeLister
	podLister  listerv1.PodLister
}

const (
	NAME          = "GPUPredicate"
	PodPhaseField = "status.phase"
	waitTimeout   = 10 * time.Second
)

func NewGPUFilter(client kubernetes.Interface) (*GPUFilter, error) {
	nodeInformerFactory := kubeinformers.NewSharedInformerFactory(client, time.Second*30)

	podListOptions := func(options *metav1.ListOptions) {
		options.FieldSelector = fmt.Sprintf("%s!=%s", PodPhaseField, corev1.PodSucceeded)
	}
	podInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(client,
		time.Second*30, kubeinformers.WithNamespace(metav1.NamespaceAll),
		kubeinformers.WithTweakListOptions(podListOptions))

	gpuFilter := &GPUFilter{
		kubeClient: client,
		nodeLister: nodeInformerFactory.Core().V1().Nodes().Lister(),
		podLister:  podInformerFactory.Core().V1().Pods().Lister(),
	}

	go nodeInformerFactory.Start(nil)
	go podInformerFactory.Start(nil)

	return gpuFilter, nil
}

func (gpuFilter *GPUFilter) Name() string {
	return NAME
}

type filterFunc func(*corev1.Pod, []corev1.Node) ([]corev1.Node, extenderv1.FailedNodesMap,
	error)

func (gpuFilter *GPUFilter) Filter(
	args extenderv1.ExtenderArgs,
) *extenderv1.ExtenderFilterResult {
	if !util.IsGPURequiredPod(args.Pod) {
		return &extenderv1.ExtenderFilterResult{
			Nodes:       args.Nodes,
			FailedNodes: nil,
			Error:       "",
		}
	}

	filters := []filterFunc{
		gpuFilter.deviceFilter,
	}
	filteredNodes := args.Nodes.Items
	failedNodesMap := make(extenderv1.FailedNodesMap)
	for _, filter := range filters {
		passedNodes, failedNodes, err := filter(args.Pod, filteredNodes)
		if err != nil {
			return &extenderv1.ExtenderFilterResult{
				Error: err.Error(),
			}
		}
		filteredNodes = passedNodes
		for name, reason := range failedNodes {
			failedNodesMap[name] = reason
		}
	}

	return &extenderv1.ExtenderFilterResult{
		Nodes: &corev1.NodeList{
			Items: filteredNodes,
		},
		FailedNodes: failedNodesMap,
		Error:       "",
	}
}

//deviceFilter will choose one and only one node fullfil the request,
//so it should always be the last filter of gpuFilter
func (gpuFilter *GPUFilter) deviceFilter(
	pod *corev1.Pod, nodes []corev1.Node) ([]corev1.Node, extenderv1.FailedNodesMap, error) {
	// #lizard forgives
	var (
		filteredNodes  = make([]corev1.Node, 0)
		failedNodesMap = make(extenderv1.FailedNodesMap)
		nodeInfoList   []*device.NodeInfo
		success        bool
		sorter         = device.NodeInfoSort(
			device.ByAllocatableCores,
			device.ByAllocatableMemory,
			device.ByID)
	)
	for k := range pod.Annotations {
		if strings.Contains(k, util.GPUAssigned) ||
			strings.Contains(k, util.PredicateTimeAnnotation) ||
			strings.Contains(k, util.PredicateGPUIndexPrefix) {
			return filteredNodes, failedNodesMap, fmt.Errorf("pod %s had been predicated!", pod.Name)
		}
	}

	for i := range nodes {
		node := &nodes[i]
		if !util.IsGPUEnabledNode(node) {
			failedNodesMap[node.Name] = "no GPU device"
			continue
		}
		pods, err := gpuFilter.ListPodsOnNode(node)
		if err != nil {
			failedNodesMap[node.Name] = "failed to get pods on node"
			continue
		}
		nodeInfo := device.NewNodeInfo(node, pods)
		nodeInfoList = append(nodeInfoList, nodeInfo)
	}
	sorter.Sort(nodeInfoList)

	for _, nodeInfo := range nodeInfoList {
		node := nodeInfo.GetNode()
		if success {
			failedNodesMap[node.Name] = fmt.Sprintf(
				"pod %s has already been matched to another node", pod.UID)
			continue
		}

		alloc := algorithm.NewAllocator(nodeInfo)
		newPod, err := alloc.Allocate(pod)
		if err != nil {
			failedNodesMap[node.Name] = fmt.Sprintf(
				"pod %s does not match with this node", pod.UID)
			continue
		} else {
			annotationMap := make(map[string]string)
			for k, v := range newPod.Annotations {
				if strings.Contains(k, util.GPUAssigned) ||
					strings.Contains(k, util.PredicateTimeAnnotation) ||
					strings.Contains(k, util.PredicateGPUIndexPrefix) ||
					strings.Contains(k, util.PredicateNode) {
					annotationMap[k] = v
				}
			}
			err := gpuFilter.patchPodWithAnnotations(newPod, annotationMap)
			if err != nil {
				failedNodesMap[node.Name] = "update pod annotation failed"
				continue
			}
			filteredNodes = append(filteredNodes, *node)
			success = true
		}
	}

	return filteredNodes, failedNodesMap, nil
}

func (gpuFilter *GPUFilter) ListPodsOnNode(node *corev1.Node) ([]*corev1.Pod, error) {
	// #lizard forgives
	pods, err := gpuFilter.podLister.Pods(corev1.NamespaceAll).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var ret []*corev1.Pod
	for _, pod := range pods {
		klog.V(9).Infof("List pod %s", pod.Name)
		var predicateNode string
		if pod.Spec.NodeName == "" && pod.Annotations != nil {
			if v, ok := pod.Annotations[util.PredicateNode]; ok {
				predicateNode = v
			}
		}
		if (pod.Spec.NodeName == node.Name || predicateNode == node.Name) &&
			pod.Status.Phase != corev1.PodSucceeded &&
			pod.Status.Phase != corev1.PodFailed {
			ret = append(ret, pod)
			klog.V(9).Infof("get pod %s on node %s", pod.UID, node.Name)
		}
	}
	return ret, nil
}

func (gpuFilter *GPUFilter) patchPodWithAnnotations(
	pod *corev1.Pod, annotationMap map[string]string) error {
	// update annotations by patching to the pod
	type patchMetadata struct {
		Annotations map[string]string `json:"annotations"`
	}
	type patchPod struct {
		Metadata patchMetadata `json:"metadata"`
	}
	payload := patchPod{
		Metadata: patchMetadata{
			Annotations: annotationMap,
		},
	}

	payloadBytes, _ := json.Marshal(payload)
	err := wait.PollImmediate(time.Second, waitTimeout, func() (bool, error) {
		_, err := gpuFilter.kubeClient.CoreV1().Pods(pod.Namespace).
			Patch(context.Background(), pod.Name, k8stypes.StrategicMergePatchType, payloadBytes, metav1.PatchOptions{})
		if err == nil {
			return true, nil
		}
		if util.ShouldRetry(err) {
			return false, nil
		}

		return false, err
	})
	if err != nil {
		msg := fmt.Sprintf("failed to add annotation %v to pod %s due to %s",
			annotationMap, pod.UID, err.Error())
		klog.Infof(msg)
		return fmt.Errorf(msg)
	}
	return nil
}
