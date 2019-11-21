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
package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"

	"k8s.io/client-go/rest"

	"tkestack.io/gpu-admission/pkg/predicate"
	"tkestack.io/gpu-admission/pkg/prioritize"
	"tkestack.io/gpu-admission/pkg/route"
	"tkestack.io/gpu-admission/pkg/version/verflag"

	"github.com/golang/glog"
	"github.com/julienschmidt/httprouter"
	"github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/component-base/logs"
	"k8s.io/klog"
)

var (
	kubeconfig             string
	masterURL              string
	configFile             string
	listenAddress          string
	profileAddress         string
	inClusterMode          bool
)

func main() {
	flag.CommandLine.Parse([]string{})

	router := httprouter.New()
	route.AddVersion(router)

	var (
		clientCfg *rest.Config
		err       error
	)

	if !inClusterMode {
		clientCfg, err = clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	} else {
		clientCfg, err = rest.InClusterConfig()
	}

	if err != nil {
		glog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(clientCfg)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	gpuFilter, err := predicate.NewGPUFilter(configFile, kubeClient)
	if err != nil {
		glog.Fatalf("Failed to new gpu quota filter: %s", err.Error())
	}
	route.AddPredicate(router, gpuFilter)

	route.AddPrioritize(router, &prioritize.CPUNodeFirst{})

	go func() {
		log.Println(http.ListenAndServe(profileAddress, nil))
	}()

	glog.Infof("Server starting on %s", listenAddress)
	if err := http.ListenAndServe(listenAddress, router); err != nil {
		log.Fatal(err)
	}
}

func init() {
	addFlags(pflag.CommandLine)

	klog.InitFlags(nil)
	logs.InitLogs()
	defer logs.FlushLogs()

	verflag.PrintAndExitIfRequested()
}

func addFlags(fs *pflag.FlagSet) {
	fs.StringVar(&kubeconfig, "kubeconfig", "",
		"Path to a kubeconfig. Only required if out-of-cluster.")
	fs.StringVar(&masterURL, "master", "",
		"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	fs.StringVar(&configFile, "config", "", "The config path for gpu-admission.")
	fs.StringVar(&listenAddress, "address", "127.0.0.1:3456", "The address it will listen")
	fs.StringVar(&profileAddress, "pprofAddress", "127.0.0.1:3457", "The address for debug")
	fs.BoolVar(&inClusterMode, "incluster-mode", false,
		"Tell controller kubeconfig is built from in cluster")
}
