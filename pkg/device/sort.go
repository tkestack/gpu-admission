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

//LessFunc represents funcion to compare two DeviceInfo or NodeInfo
type LessFunc func(p1, p2 interface{}) bool

var (
	// ByAllocatableCores compares two device or node by allocatable cores
	ByAllocatableCores = func(p1, p2 interface{}) bool {
		var result bool
		switch p1.(type) {
		case *DeviceInfo:
			d1 := p1.(*DeviceInfo)
			d2 := p2.(*DeviceInfo)
			result = d1.AllocatableCores() < d2.AllocatableCores()
		case *NodeInfo:
			n1 := p1.(*NodeInfo)
			n2 := p2.(*NodeInfo)
			result = n1.GetAvailableCore() < n2.GetAvailableCore()
		}
		return result
	}

	// ByAllocatableMemory compares two device or node by allocatable memory
	ByAllocatableMemory = func(p1, p2 interface{}) bool {
		var result bool
		switch p1.(type) {
		case *DeviceInfo:
			d1 := p1.(*DeviceInfo)
			d2 := p2.(*DeviceInfo)
			result = d1.AllocatableMemory() < d2.AllocatableMemory()
		case *NodeInfo:
			n1 := p1.(*NodeInfo)
			n2 := p2.(*NodeInfo)
			result = n1.GetAvailableMemory() < n2.GetAvailableMemory()
		}
		return result
	}

	ByID = func(p1, p2 interface{}) bool {
		var result bool
		switch p1.(type) {
		case *DeviceInfo:
			d1 := p1.(*DeviceInfo)
			d2 := p2.(*DeviceInfo)
			result = d1.GetID() < d2.GetID()
		case *NodeInfo:
			n1 := p1.(*NodeInfo)
			n2 := p2.(*NodeInfo)
			result = n1.GetName() < n2.GetName()
		}
		return result
	}
)
