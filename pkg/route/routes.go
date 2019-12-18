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
package route

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"k8s.io/klog"
	schedulerapi "k8s.io/kubernetes/pkg/scheduler/api"

	"tkestack.io/gpu-admission/pkg/predicate"
	"tkestack.io/gpu-admission/pkg/prioritize"
	"tkestack.io/gpu-admission/pkg/version"
)

const (
	// version router path
	versionPath = "/version"
	apiPrefix   = "/scheduler"
	// predication router path
	predicatesPrefix = apiPrefix + "/predicates"
	// prioritize router path
	prioritiesPrefix = apiPrefix + "/priorities"
)

func checkBody(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		http.Error(w, "Please send a request body", 400)
		return
	}
}

// PredicateRoute sets router table for predication
func PredicateRoute(predicate predicate.Predicate) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		checkBody(w, r)

		var buf bytes.Buffer
		body := io.TeeReader(r.Body, &buf)

		var extenderArgs schedulerapi.ExtenderArgs
		var extenderFilterResult *schedulerapi.ExtenderFilterResult

		if err := json.NewDecoder(body).Decode(&extenderArgs); err != nil {
			extenderFilterResult = &schedulerapi.ExtenderFilterResult{
				Nodes:       nil,
				FailedNodes: nil,
				Error:       err.Error(),
			}
		} else {
			extenderFilterResult = predicate.Filter(extenderArgs)
			klog.V(4).Infof("%s: ExtenderArgs = %+v", predicate.Name(), extenderArgs)
		}

		if resultBody, err := json.Marshal(extenderFilterResult); err != nil {
			klog.Errorf("Failed to marshal extenderFilterResult: %+v, %+v",
				err, extenderFilterResult)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		} else {
			klog.V(4).Infof("%s: extenderFilterResult = %s",
				predicate.Name(), string(resultBody))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(resultBody)
		}
	}
}

// PrioritizeRoute sets the router table for prioritization
func PrioritizeRoute(prioritize prioritize.Prioritize) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		checkBody(w, r)

		var buf bytes.Buffer
		body := io.TeeReader(r.Body, &buf)

		var extenderArgs schedulerapi.ExtenderArgs
		var hostPriorityList *schedulerapi.HostPriorityList

		if err := json.NewDecoder(body).Decode(&extenderArgs); err != nil {
			klog.Errorf("Bad request for prioritize, body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		hostPriorityList, err := prioritize.Handler(extenderArgs)
		if err != nil {
			klog.Errorf("Failed to prioritize %+v: %v", extenderArgs, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		klog.V(4).Infof("%s: ExtenderArgs = %+v, priorize result: %+v",
			prioritize.Name(), extenderArgs, hostPriorityList)

		if resultBody, err := json.Marshal(hostPriorityList); err != nil {
			klog.Errorf("Failed to marshal hostPriorityList: %+v, %+v", err, hostPriorityList)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		} else {
			klog.V(4).Infof("%s: hostPriorityList = %s", prioritize.Name(), string(resultBody))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(resultBody)
		}
	}
}

// VersionRoute returns the version of router in response
func VersionRoute(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	fmt.Fprint(w, fmt.Sprint(version.Get()))
}

func AddVersion(router *httprouter.Router) {
	router.GET(versionPath, DebugLogging(VersionRoute, versionPath))
}

// DebugLogging wraps handler for debugging purposes
func DebugLogging(h httprouter.Handle, path string) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		klog.V(10).Infof("%s request body = %s", path, r.Body)
		h(w, r, p)
		klog.V(10).Infof("%s response = %s", path, w)
	}
}

func AddPredicate(router *httprouter.Router, predicate predicate.Predicate) {
	path := predicatesPrefix
	router.POST(path, DebugLogging(PredicateRoute(predicate), path))
}

func AddPrioritize(router *httprouter.Router, prioritize prioritize.Prioritize) {
	path := prioritiesPrefix
	router.POST(path, DebugLogging(PrioritizeRoute(prioritize), path))
}
