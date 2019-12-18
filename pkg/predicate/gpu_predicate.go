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
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog"
	schedulerapi "k8s.io/kubernetes/pkg/scheduler/api"

	"tkestack.io/gpu-admission/pkg/algorithm"
	"tkestack.io/gpu-admission/pkg/device"
	"tkestack.io/gpu-admission/pkg/util"
)

type GPUFilter struct {
	conf            *GPUFilterConfig
	kubeClient      kubernetes.Interface
	configMapLister listerv1.ConfigMapLister
	nodeLister      listerv1.NodeLister
	podLister       listerv1.PodLister

	// Quota records each namespace's quota.
	quota                   map[string]NamespaceQuota
	quotaLastSyncedRevision string
	sync.Mutex
}

type GPUFilterConfig struct {
	// Quota config is stored in configMap. Its key is specified by QuotaConfigKey,
	// the value is a map which maps namespace to a map which indicates how many gpu model
	// the corresponding namespace could use.
	// Example of config'value :
	// {
	//   "A": {
	//     "pool": [ "public" ], // Pods in namespace 'A' could use pool 'public'
	//     "quota": {
	//       "M40": 2,
	//       "P100": 3
	//     }
	//   },
	//   "B": {
	//     "pool": [ "wx" ], // Pods in namespace 'B' could use pool 'wx'
	//     "quota": {
	//       "M40": 8,
	//       "P100": 2
	//     }
	//   }
	// }
	QuotaConfigMapName      string `json:"QuotaConfigMapName"`
	QuotaConfigMapNamespace string `json:"QuotaConfigMapNamespace"`
	// Each GPU nodes are labelled with 'GPUModelLabel' and 'GPUPoolLabel'.
	GPUModelLabel string `json:"GPUModelLabel"`
	GPUPoolLabel  string `json:"GPUPoolLabel"`
	// Suppose the following case: pod0 requests 1 P40, pod1 requests 1 P40, and the quota is 1 P40.
	// If all goes right, pod1 could not be scheduled. However if things happen as this way: schedule
	// pod0 -> schedule pod1 -> bind pod0 -> bind pod1, we could not find 1 P40 has been used by pod0
	// when scheduling pod1 because it has not been bound to node and gpu-admission could not
	// find it! In order to make it simpler and right, we will sleep for some time, then the things
	// will always happen as this way: schedule pod0 -> bind pod0 -> schedule pod1 -> bind pod1.
	SkipBindTime time.Duration `json:"SkipBindTime"`
}

type NamespaceQuota struct {
	// Quota is a map, whose key is GPUModule and value is limit
	Quota map[string]int `json:"quota"`
	// Pools that could be used
	Pool []string `json:"pool"`
}

const (
	DefaultQuotaSyncInterval     = 5 * time.Second
	DefaultGPUConfigMapName      = "gpuquota"
	DefaultGPUConfigMapNamespace = "kube-system"
	QuotaConfigKey               = "gpu_quota"

	NAME = "GPUQuotaPredicate"

	NamespaceField = "metadata.namespace"
	NameFiled      = "metadata.name"
	PodPhaseFiled  = "status.phase"

	DefaultSkipBindTime = 300 * time.Microsecond
	waitTimeout         = 10 * time.Second
)

func NewGPUFilter(configFile string, client kubernetes.Interface) (*GPUFilter, error) {
	var gpuFilterConfig *GPUFilterConfig
	if err := util.ParseConifg(configFile, &gpuFilterConfig); err != nil {
		return nil, fmt.Errorf("invalid GPUFilter config in file %s", configFile)
	}
	if len(gpuFilterConfig.QuotaConfigMapNamespace) == 0 {
		gpuFilterConfig.QuotaConfigMapNamespace = DefaultGPUConfigMapNamespace
	}
	if len(gpuFilterConfig.QuotaConfigMapName) == 0 {
		gpuFilterConfig.QuotaConfigMapName = DefaultGPUConfigMapName
	}
	if len(gpuFilterConfig.GPUModelLabel) == 0 || len(gpuFilterConfig.GPUPoolLabel) == 0 {
		return nil, fmt.Errorf("GPUModelLabel or GPUPoolLabel config not found")
	}
	if gpuFilterConfig.SkipBindTime == 0 {
		gpuFilterConfig.SkipBindTime = DefaultSkipBindTime
	}
	klog.Infof("SkipBindTime is %v", gpuFilterConfig.SkipBindTime)
	return newGPUFilter(gpuFilterConfig, client)
}

func newGPUFilter(
	gpuFilterConfig *GPUFilterConfig,
	client kubernetes.Interface) (*GPUFilter, error) {
	configMapListOptions := func(options *metav1.ListOptions) {
		options.FieldSelector = fields.SelectorFromSet(map[string]string{
			NamespaceField: gpuFilterConfig.QuotaConfigMapNamespace,
			NameFiled:      gpuFilterConfig.QuotaConfigMapName}).String()
	}
	configMapInformerFactory := kubeinformers.
		NewSharedInformerFactoryWithOptions(client, time.Second*30,
			kubeinformers.WithNamespace(gpuFilterConfig.QuotaConfigMapNamespace),
			kubeinformers.WithTweakListOptions(configMapListOptions))

	nodeInformerFactory := kubeinformers.NewSharedInformerFactory(client, time.Second*30)

	podListOptions := func(options *metav1.ListOptions) {
		options.FieldSelector = fmt.Sprintf("%s!=%s", PodPhaseFiled, corev1.PodSucceeded)
	}
	podInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(client,
		time.Second*30, kubeinformers.WithNamespace(metav1.NamespaceAll),
		kubeinformers.WithTweakListOptions(podListOptions))

	gpuQuotaFilter := &GPUFilter{
		conf:            gpuFilterConfig,
		kubeClient:      client,
		configMapLister: configMapInformerFactory.Core().V1().ConfigMaps().Lister(),
		nodeLister:      nodeInformerFactory.Core().V1().Nodes().Lister(),
		podLister:       podInformerFactory.Core().V1().Pods().Lister(),
	}

	go wait.Forever(gpuQuotaFilter.syncQuota, DefaultQuotaSyncInterval)
	go configMapInformerFactory.Start(nil)
	go nodeInformerFactory.Start(nil)
	go podInformerFactory.Start(nil)

	return gpuQuotaFilter, nil
}

func (gpuFilter *GPUFilter) syncQuota() {
	configData, err := gpuFilter.getConfigData()
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			klog.V(4).Infof("GPU quota is not set")
			gpuFilter.setQuota(nil, "none")
		} else {
			klog.Errorf("Failed to read config map(%s/%s) fro GPUFilter: %s",
				gpuFilter.conf.QuotaConfigMapNamespace, gpuFilter.conf.QuotaConfigMapName, err)
		}
		return
	}
	if configData.ResourceVersion == gpuFilter.quotaLastSyncedRevision {
		klog.V(4).Infof("Same resource version, no need to sync GPU quota")
		return
	}
	if quota, err := parseQuota(configData.Data); err != nil {
		klog.Errorf("Failed to parse quota: %s", err)
	} else {
		gpuFilter.setQuota(quota, configData.ResourceVersion)
	}
}

func (gpuFilter *GPUFilter) getConfigData() (*corev1.ConfigMap, error) {
	name := gpuFilter.conf.QuotaConfigMapName
	namespace := gpuFilter.conf.QuotaConfigMapNamespace
	return gpuFilter.configMapLister.ConfigMaps(namespace).Get(name)
}

func parseQuota(configData map[string]string) (quota map[string]NamespaceQuota, err error) {
	quotaData, ok := configData[QuotaConfigKey]
	if !ok {
		return nil, fmt.Errorf("key %s not found", QuotaConfigKey)
	}
	quota = make(map[string]NamespaceQuota)
	if err := json.Unmarshal([]byte(quotaData), &quota); err != nil {
		klog.Errorf("Failed to parse GPU quota config : %s", err)
		return nil, err
	}
	return quota, nil
}

func (gpuFilter *GPUFilter) Name() string {
	return NAME
}

type filterFunc func(*corev1.Pod, []corev1.Node) ([]corev1.Node, schedulerapi.FailedNodesMap,
	error)

func (gpuFilter *GPUFilter) Filter(
	args schedulerapi.ExtenderArgs,
) *schedulerapi.ExtenderFilterResult {
	if !util.IsGPURequiredPod(args.Pod) {
		return &schedulerapi.ExtenderFilterResult{
			Nodes:       args.Nodes,
			FailedNodes: nil,
			Error:       "",
		}
	}
	// Quota has not been synced.
	if len(gpuFilter.quotaLastSyncedRevision) == 0 {
		klog.V(1).Info("GPU quota has not been synced, please retry later")
		return &schedulerapi.ExtenderFilterResult{
			Error: "GPU quota has not been synced, please retry later",
		}
	}

	time.Sleep(gpuFilter.conf.SkipBindTime)

	filters := []filterFunc{
		gpuFilter.quotaFilter,
		//deviceFilter should always be the last filter
		gpuFilter.deviceFilter,
	}
	filteredNodes := args.Nodes.Items
	failedNodesMap := make(schedulerapi.FailedNodesMap)
	for _, filter := range filters {
		passedNodes, failedNodes, err := filter(args.Pod, filteredNodes)
		if err != nil {
			return &schedulerapi.ExtenderFilterResult{
				Error: err.Error(),
			}
		}
		filteredNodes = passedNodes
		for name, reason := range failedNodes {
			failedNodesMap[name] = reason
		}
	}

	return &schedulerapi.ExtenderFilterResult{
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
	pod *corev1.Pod, nodes []corev1.Node) ([]corev1.Node, schedulerapi.FailedNodesMap, error) {
	// #lizard forgives
	var (
		filteredNodes  = make([]corev1.Node, 0)
		failedNodesMap = make(schedulerapi.FailedNodesMap)
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
			return filteredNodes, failedNodesMap, nil
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

func (gpuFilter *GPUFilter) quotaFilter(
	pod *corev1.Pod,
	nodes []corev1.Node,
) (filteredNodes []corev1.Node, failedNodesMap schedulerapi.FailedNodesMap, err error) {
	quota, exists := gpuFilter.getQuotaForNamespace(pod.Namespace)
	if !exists {
		klog.V(4).Infof("No GPU quota limit for %s", pod.Namespace)
		return nodes, nil, nil
	}
	if gpuModels, err := gpuFilter.filterGPUModel(pod, quota); err != nil {
		klog.Errorf("Failed to filer GPU models for pod %s: %s", pod.Name, err)
		return nil, nil, err
	} else {
		return gpuFilter.filterNodes(nodes, gpuModels, quota.Pool)
	}
}

func (gpuFilter *GPUFilter) setQuota(quota map[string]NamespaceQuota, resourceVersion string) {
	gpuFilter.Lock()
	defer gpuFilter.Unlock()
	gpuFilter.quota = quota
	gpuFilter.quotaLastSyncedRevision = resourceVersion
	klog.V(4).Infof("Update quota %+v with resource version %s successfully",
		quota, resourceVersion)
}

func (gpuFilter *GPUFilter) getQuotaForNamespace(
	namespace string,
) (quota NamespaceQuota, exists bool) {
	gpuFilter.Lock()
	defer gpuFilter.Unlock()
	quota, exists = gpuFilter.quota[namespace]
	klog.V(4).Infof("Quota for namespace %s is %+v", namespace, quota)
	return
}

// Find GPU models whose's usage by pods (that are in same namespace with 'pod') are under limit.
func (gpuFilter *GPUFilter) filterGPUModel(
	pod *corev1.Pod, namespaceQuota NamespaceQuota) ([]string, error) {
	var filteredGPUModels []string
	for gpuModel, limit := range namespaceQuota.Quota {
		limit = limit * util.HundredCore
		nodeSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
			MatchLabels: map[string]string{gpuFilter.conf.GPUModelLabel: gpuModel}})
		if err != nil {
			return nil, err
		}
		pods, err := gpuFilter.listPodsOnNodes(nodeSelector, pod.Namespace)
		if err != nil {
			return nil, err
		}
		gpuUsed := calculateGPUUsage(append(pods, pod))
		if gpuUsed <= limit {
			filteredGPUModels = append(filteredGPUModels, gpuModel)
		}
		klog.V(4).Infof(
			"Pods in namespace %s will use %d %s GPU cards after adding this pod, quota is %d",
			pod.Namespace, gpuUsed, gpuModel, limit)
	}
	klog.V(4).Infof("These GPU models could be used by pod %s: %+v", pod.Name, filteredGPUModels)
	return filteredGPUModels, nil
}

func (gpuFilter *GPUFilter) listPodsOnNodes(
	nodeSelector labels.Selector, podNameSpace string) ([]*corev1.Pod, error) {
	nodes, err := gpuFilter.nodeLister.List(nodeSelector)
	if err != nil {
		return nil, err
	}
	var records = make(map[string]bool)
	for _, node := range nodes {
		records[node.Name] = true
	}

	pods, err := gpuFilter.podLister.Pods(podNameSpace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var ret []*corev1.Pod
	for _, pod := range pods {
		klog.V(9).Infof("List pod %s/%s in namespace %s", pod.Namespace, pod.Name, podNameSpace)
		if _, exist := records[pod.Spec.NodeName]; exist {
			ret = append(ret, pod)
		}
	}
	return ret, nil
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
			Patch(pod.Name, k8stypes.StrategicMergePatchType, payloadBytes)
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

func calculateGPUUsage(pods []*corev1.Pod) int {
	var totalGPURequests int
	for _, pod := range pods {
		if quotaPod(pod) {
			totalGPURequests += int(util.GetGPUResourceOfPod(pod, util.VCoreAnnotation))
		}
	}
	return totalGPURequests
}

// Copied from k8s.io/kubernetes/pkg/quota/evaluator/core/pods.go
// QuotaPod returns true if the pod is eligible to track against a quota
// if it's not in a terminal state according to its phase.
func quotaPod(pod *corev1.Pod) bool {
	// see GetPhase in kubelet.go for details on how it covers all restart policy conditions
	// https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/kubelet.go#L3001
	return !(corev1.PodFailed == pod.Status.Phase || corev1.PodSucceeded == pod.Status.Phase)
}

// Filter nodes those labels satisfy 'gpuModels' and 'pool'
func (gpuFilter *GPUFilter) filterNodes(
	nodes []corev1.Node,
	gpuModels,
	pools []string,
) (filteredNodes []corev1.Node, failedNodesMap schedulerapi.FailedNodesMap,
	err error) {
	var gpuModelSelector, poolSelector labels.Selector

	klog.V(4).Infof("Filter nodes with gpuModels(%+v) and pools(%+v)", gpuModels, pools)

	if len(gpuModels) != 0 {
		gpuModelSelector, err = metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      gpuFilter.conf.GPUModelLabel,
				Operator: metav1.LabelSelectorOpIn,
				Values:   gpuModels,
			}}})
		if err != nil {
			return nil, nil, err
		}
	} else {
		gpuModelSelector = labels.Nothing()
	}

	// If pool is empty, it means that pod could use every pool,
	// it is OK to leave it as a empty selector.
	if len(pools) != 0 {
		poolSelector, err = metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      gpuFilter.conf.GPUPoolLabel,
				Operator: metav1.LabelSelectorOpIn,
				Values:   pools,
			}}})
		if err != nil {
			return nil, nil, err
		}
	} else {
		poolSelector = labels.Everything()
	}

	failedNodesMap = schedulerapi.FailedNodesMap{}
	for _, node := range nodes {
		if gpuModelSelector.Matches(labels.Set(node.Labels)) &&
			poolSelector.Matches(labels.Set(node.Labels)) {
			filteredNodes = append(filteredNodes, node)
			klog.V(5).Infof("Add %s to filteredNodes", node.Name)
		} else {
			failedNodesMap[node.Name] = "ExceedsGPUQuota"
			klog.V(5).Infof("Add %s to failedNodesMap", node.Name)
		}
	}
	return filteredNodes, failedNodesMap, nil
}
