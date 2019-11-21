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
package prioritize

import (
	"k8s.io/kubernetes/pkg/scheduler/api"

	"tkestack.io/gpu-admission/pkg/util"
)

// We prioritize cpu node first than GPU node.
type CPUNodeFirst struct {
}

// Name implements Prioritize interface
func (cpu *CPUNodeFirst) Name() string {
	return "CPUNodeFirst"
}

// Handler implements Prioritize interface
func (cpu *CPUNodeFirst) Handler(args api.ExtenderArgs) (*api.HostPriorityList, error) {
	var priorityList api.HostPriorityList

	for _, node := range args.Nodes.Items {
		priority := api.HostPriority{Host: node.Name}
		if util.IsGPUEnabledNode(&node) {
			priority.Score = 0
		} else {
			priority.Score = 1
		}
		priorityList = append(priorityList, priority)
	}

	return &priorityList, nil
}
