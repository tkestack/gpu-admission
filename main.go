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
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/component-base/logs"
	"k8s.io/klog"

	"tkestack.io/gpu-admission/pkg/predicate"
	"tkestack.io/gpu-admission/pkg/route"
	"tkestack.io/gpu-admission/pkg/version/verflag"
)

var (
	kubeconfig     string
	masterURL      string
	listenAddress  string
	profileAddress string
)

func main() {
	addFlags(pflag.CommandLine)

	logs.InitLogs()
	defer logs.FlushLogs()
	initFlags()
	flag.CommandLine.Parse([]string{})
	verflag.PrintAndExitIfRequested()

	router := httprouter.New()
	route.AddVersion(router)

	var (
		clientCfg *rest.Config
		err       error
	)

	clientCfg, err = clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(clientCfg)
	if err != nil {
		klog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	gpuFilter, err := predicate.NewGPUFilter(kubeClient)
	if err != nil {
		klog.Fatalf("Failed to new gpu quota filter: %s", err.Error())
	}
	route.AddPredicate(router, gpuFilter)

	go func() {
		log.Println(http.ListenAndServe(profileAddress, nil))
	}()

	klog.Infof("Server starting on %s", listenAddress)
	if err := http.ListenAndServe(listenAddress, router); err != nil {
		log.Fatal(err)
	}
}

func addFlags(fs *pflag.FlagSet) {
	fs.StringVar(&kubeconfig, "kubeconfig", "",
		"Path to a kubeconfig. Only required if out-of-cluster.")
	fs.StringVar(&masterURL, "master", "",
		"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	fs.StringVar(&listenAddress, "address", "127.0.0.1:3456", "The address it will listen")
	fs.StringVar(&profileAddress, "pprofAddress", "127.0.0.1:3457", "The address for debug")
}

func wordSepNormalizeFunc(f *pflag.FlagSet, name string) pflag.NormalizedName {
	if strings.Contains(name, "_") {
		return pflag.NormalizedName(strings.Replace(name, "_", "-", -1))
	}
	return pflag.NormalizedName(name)
}

// InitFlags normalizes and parses the command line flags
func initFlags() {
	pflag.CommandLine.SetNormalizeFunc(wordSepNormalizeFunc)
	// Only glog flags will be added
	flag.CommandLine.VisitAll(func(goflag *flag.Flag) {
		switch goflag.Name {
		case "logtostderr", "alsologtostderr",
			"v", "stderrthreshold", "vmodule", "log_backtrace_at", "log_dir":
			pflag.CommandLine.AddGoFlag(goflag)
		}
	})

	pflag.Parse()
}
