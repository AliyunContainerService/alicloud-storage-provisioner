/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/denverdino/aliyungo/common"
	"github.com/kubernetes-incubator/external-storage/lib/controller"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	DEFAULT_REGION               = common.Hangzhou
	KUBERNETES_ALICLOUD_IDENTITY = fmt.Sprintf("Kubernetes.Alicloud/Provision.Disk-%s", ProvisionVersion())

	provisioner = flag.String("provisioner", "alicloud/disk", "Name of the provisioner. The provisioner will only provision volumes for claims that request a StorageClass with a provisioner field set equal to this name.")
	master      = flag.String("master", "", "Master URL to build a client config from. Either this or kubeconfig needs to be set if the provisioner is being run out of cluster.")
	kubeconfig  = flag.String("kubeconfig", "", "Absolute path to the kubeconfig file. Either this or master needs to be set if the provisioner is being run out of cluster.")
)

func main() {
	flag.Parse()
	flag.Set("logtostderr", "true")
	//defer glog.Flush()

	// set log file
	setLogAttribute()

	// Create the client according to whether we are running in or out-of-cluster
	var config *rest.Config
	var err error
	if *master != "" || *kubeconfig != "" {
		log.Infof("Either master or kubeconfig specified. building kube config from that..")
		config, err = clientcmd.BuildConfigFromFlags(*master, *kubeconfig)
	} else {
		log.Infof("Building kube configs for running in cluster...")
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		log.Fatalf("Failed to create config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// The controller needs to know what the server version is because out-of-tree
	// provisioners aren't officially supported until 1.5
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		log.Fatalf("Error getting server version: %v", err)
	}

	// Create the provisioner: it implements the Provisioner interface expected by
	// the controller
	diskProvisioner := NewDiskProvisioner(clientset)

	if diskProvisioner == nil {
		panic("error init alicloud disk controller.")
	}
	// Start the provision controller which will dynamically provision disk NFS
	// PVs
	pc := controller.NewProvisionController(
		clientset,
		*provisioner,
		diskProvisioner,
		serverVersion.GitVersion,
	)

	pc.Run(wait.NeverStop)
}
