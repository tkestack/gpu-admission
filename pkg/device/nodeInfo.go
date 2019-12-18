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
package device

import (
	"sort"

	"k8s.io/api/core/v1"
	"k8s.io/klog"

	"tkestack.io/gpu-admission/pkg/util"
)

type NodeInfo struct {
	name        string
	node        *v1.Node
	devs        map[int]*DeviceInfo
	deviceCount int
	totalMemory uint
	usedCore    uint
	usedMemory  uint
}

func NewNodeInfo(node *v1.Node, pods []*v1.Pod) *NodeInfo {
	klog.V(4).Infof("debug: NewNodeInfo() creates nodeInfo for %s", node.Name)

	devMap := map[int]*DeviceInfo{}
	nodeTotalMemory := uint(util.GetCapacityOfNode(node, util.VMemoryAnnotation))
	deviceCount := util.GetGPUDeviceCountOfNode(node)
	deviceTotalMemory := nodeTotalMemory / uint(deviceCount)
	for i := 0; i < deviceCount; i++ {
		devMap[i] = newDeviceInfo(i, deviceTotalMemory)
	}

	ret := &NodeInfo{
		name:        node.Name,
		node:        node,
		devs:        devMap,
		deviceCount: deviceCount,
		totalMemory: nodeTotalMemory,
	}

	// According to the pods' annotations, construct the node allocation
	// state
	for _, pod := range pods {
		for i, c := range pod.Spec.Containers {
			predicateIndexes, err := util.GetPredicateIdxOfContainer(pod, i)
			if err != nil {
				continue
			}
			for _, index := range predicateIndexes {
				var vcore, vmemory uint
				if index >= deviceCount {
					klog.Infof("invalid predicateIndex %d larger than device count", index)
					continue
				}
				vcore = util.GetGPUResourceOfContainer(&c, util.VCoreAnnotation)
				if vcore < util.HundredCore {
					vmemory = util.GetGPUResourceOfContainer(&c, util.VMemoryAnnotation)
				} else {
					vcore = util.HundredCore
					vmemory = deviceTotalMemory
				}
				err = ret.AddUsedResources(index, vcore, vmemory)
				if err != nil {
					klog.Infof("failed to update used resource for node %s dev %d due to %v",
						node.Name, index, err)
				}
			}

		}
	}

	return ret
}

// AddUsedResources records the used GPU core and memory
func (n *NodeInfo) AddUsedResources(devID int, vcore uint, vmemory uint) error {
	err := n.devs[devID].AddUsedResources(vcore, vmemory)
	if err != nil {
		klog.Infof("failed to update used resource for node %s dev %d due to %v", n.name, devID, err)
		return err
	}
	n.usedCore += vcore
	n.usedMemory += vmemory
	return nil
}

// GetDeviceCount returns the number of GPU devices
func (n *NodeInfo) GetDeviceCount() int {
	return n.deviceCount
}

// GetDeviceMap returns each GPU device information structure
func (n *NodeInfo) GetDeviceMap() map[int]*DeviceInfo {
	return n.devs
}

// GetNode returns the original node structure of kubernetes
func (n *NodeInfo) GetNode() *v1.Node {
	return n.node
}

// GetName returns node name
func (n *NodeInfo) GetName() string {
	return n.name
}

// GetAvailableCore returns the remaining cores of this node
func (n *NodeInfo) GetAvailableCore() int {
	return n.deviceCount*util.HundredCore - int(n.usedCore)
}

// GetAvailableMemory returns the remaining memory of this node
func (n *NodeInfo) GetAvailableMemory() int {
	return int(n.totalMemory - n.usedMemory)
}

type nodeInfoPriority struct {
	data []*NodeInfo
	less []LessFunc
}

func NodeInfoSort(less ...LessFunc) *nodeInfoPriority {
	return &nodeInfoPriority{
		less: less,
	}
}

func (nip *nodeInfoPriority) Sort(data []*NodeInfo) {
	nip.data = data
	sort.Sort(nip)
}

func (nip *nodeInfoPriority) Len() int {
	return len(nip.data)
}

func (nip *nodeInfoPriority) Swap(i, j int) {
	nip.data[i], nip.data[j] = nip.data[j], nip.data[i]
}

func (nip *nodeInfoPriority) Less(i, j int) bool {
	var k int

	for k = 0; k < len(nip.less)-1; k++ {
		less := nip.less[k]
		switch {
		case less(nip.data[i], nip.data[j]):
			return true
		case less(nip.data[j], nip.data[i]):
			return false
		}
	}

	return nip.less[k](nip.data[i], nip.data[j])
}
