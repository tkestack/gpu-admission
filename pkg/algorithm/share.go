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
package algorithm

import (
	"sort"

	"k8s.io/klog"

	"tkestack.io/gpu-admission/pkg/device"
)

type shareMode struct {
	node *device.NodeInfo
}

//NewShareMode returns a new shareMode struct.
//
//Evaluate() of shareMode returns one device with minimum available cores
//which fullfil the request.
//
//Share mode means multiple application may share one GPU device which uses
//GPU more efficiently.
func NewShareMode(n *device.NodeInfo) *shareMode {
	return &shareMode{n}
}

func (al *shareMode) Evaluate(cores uint, memory uint) []*device.DeviceInfo {
	var (
		devs        []*device.DeviceInfo
		deviceCount = al.node.GetDeviceCount()
		tmpStore    = make([]*device.DeviceInfo, deviceCount)
		sorter      = shareModeSort(device.ByAllocatableCores, device.ByAllocatableMemory, device.ByID)
	)

	for i := 0; i < deviceCount; i++ {
		tmpStore[i] = al.node.GetDeviceMap()[i]
	}

	sorter.Sort(tmpStore)

	for _, dev := range tmpStore {
		if dev.AllocatableCores() >= cores && dev.AllocatableMemory() >= memory {
			klog.V(4).Infof("Pick up %d , cores: %d, memory: %d",
				dev.GetID(), dev.AllocatableCores(), dev.AllocatableMemory())
			devs = append(devs, dev)
			break
		}
	}

	return devs
}

type shareModePriority struct {
	data []*device.DeviceInfo
	less []device.LessFunc
}

func shareModeSort(less ...device.LessFunc) *shareModePriority {
	return &shareModePriority{
		less: less,
	}
}

func (smp *shareModePriority) Sort(data []*device.DeviceInfo) {
	smp.data = data
	sort.Sort(smp)
}

func (smp *shareModePriority) Len() int {
	return len(smp.data)
}

func (smp *shareModePriority) Swap(i, j int) {
	smp.data[i], smp.data[j] = smp.data[j], smp.data[i]
}

func (smp *shareModePriority) Less(i, j int) bool {
	var k int

	for k = 0; k < len(smp.less)-1; k++ {
		less := smp.less[k]
		switch {
		case less(smp.data[i], smp.data[j]):
			return true
		case less(smp.data[j], smp.data[i]):
			return false
		}
	}

	return smp.less[k](smp.data[i], smp.data[j])
}
