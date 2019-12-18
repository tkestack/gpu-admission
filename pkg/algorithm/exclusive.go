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
	"tkestack.io/gpu-admission/pkg/util"
)

type exclusiveMode struct {
	node *device.NodeInfo
}

//NewExclusiveMode returns a new exclusiveMode struct.
//
//Evaluate() of exclusiveMode returns one or more empty devices
//which fullfil the request.
//
//Exclusive mode means GPU devices are not sharing, only one
//application can use them.
func NewExclusiveMode(n *device.NodeInfo) *exclusiveMode {
	return &exclusiveMode{n}
}

func (al *exclusiveMode) Evaluate(cores uint, _ uint) []*device.DeviceInfo {
	var (
		devs        []*device.DeviceInfo
		deviceCount = al.node.GetDeviceCount()
		tmpStore    = make([]*device.DeviceInfo, deviceCount)
		sorter      = exclusiveModeSort(
			device.ByAllocatableCores,
			device.ByAllocatableMemory,
			device.ByID)
		num = int(cores / util.HundredCore)
	)

	for i := 0; i < deviceCount; i++ {
		tmpStore[i] = al.node.GetDeviceMap()[i]
	}

	sorter.Sort(tmpStore)

	for _, dev := range tmpStore {
		if num == 0 {
			break
		}
		if dev.AllocatableCores() == util.HundredCore {
			devs = append(devs, dev)
			num -= 1
			continue
		}
	}

	if num > 0 {
		return nil
	}

	if klog.V(2) {
		for _, dev := range devs {
			klog.V(4).Infof("Pick up %d , cores: %d, memory: %d",
				dev.GetID(), dev.AllocatableCores(), dev.AllocatableMemory())
		}
	}

	return devs
}

type exclusiveModePriority struct {
	data []*device.DeviceInfo
	less []device.LessFunc
}

func exclusiveModeSort(less ...device.LessFunc) *exclusiveModePriority {
	return &exclusiveModePriority{
		less: less,
	}
}

func (emp *exclusiveModePriority) Sort(data []*device.DeviceInfo) {
	emp.data = data
	sort.Sort(emp)
}

func (emp *exclusiveModePriority) Len() int {
	return len(emp.data)
}

func (emp *exclusiveModePriority) Swap(i, j int) {
	emp.data[i], emp.data[j] = emp.data[j], emp.data[i]
}

func (emp *exclusiveModePriority) Less(i, j int) bool {
	var k int

	for k = 0; k < len(emp.less)-1; k++ {
		less := emp.less[k]
		switch {
		case less(emp.data[i], emp.data[j]):
			return true
		case less(emp.data[j], emp.data[i]):
			return false
		}
	}

	return emp.less[k](emp.data[i], emp.data[j])
}
