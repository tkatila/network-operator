// Copyright 2025 Intel Corporation. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fuzz

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	api "github.com/intel/network-operator/api/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func FuzzNetworkConfigurationGaudiSO(f *testing.F) {
	fmt.Println("Fuzzing NetworkConfiguration - GaudiSO")

	scheme := runtime.NewScheme()
	err := api.AddToScheme(scheme)
	if err != nil {
		f.Fatalf("failed to register scheme: %v", err)
	}

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		f.Fatalf("No KUBECONFIG env set, cannot continue")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		f.Fatalf("failed to create config: %v", err)
	}

	kubernetesClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		f.Fatalf("failed to create client: %v", err)
	}

	f.Add("gaudinode", "L2", "tool:latest", 1)

	f.Fuzz(func(t *testing.T, nodeType, layer, image string, logLevel int) {
		spec := &api.NetworkConfiguration{
			TypeMeta: v1.TypeMeta{
				Kind:       "NetworkConfiguration",
				APIVersion: "network.intel.com/v1alpha1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name: "test-network-configuration-gaudi-so",
			},
			Spec: api.NetworkConfigurationSpec{
				ConfigurationType: "gaudi-so",
				NodeSelector: map[string]string{
					"nodetype": nodeType,
				},
				GaudiScaleOut: api.GaudiScaleOutSpec{
					Layer:      layer,
					Image:      image,
					PullPolicy: "Always",
				},
				LogLevel: logLevel,
			},
		}

		// Apply and delete, don't care for the result.
		// Operator should be observed for ERRORs and crashes.
		kubernetesClient.Create(context.Background(), spec)

		time.Sleep(time.Millisecond * 5)

		kubernetesClient.Delete(context.Background(), spec)
	})
}

func FuzzNetworkConfigurationHostNIC(f *testing.F) {
	fmt.Println("Fuzzing NetworkConfiguration - HostNIC")

	scheme := runtime.NewScheme()
	err := api.AddToScheme(scheme)
	if err != nil {
		f.Fatalf("failed to register scheme: %v", err)
	}

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		f.Fatalf("No KUBECONFIG env set, cannot continue")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		f.Fatalf("failed to create config: %v", err)
	}

	kubernetesClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		f.Fatalf("failed to create client: %v", err)
	}

	f.Add("gaudinode", "intel", "192.168.10.0/28", 1)

	f.Fuzz(func(t *testing.T, nodeType, vendor, iprange string, logLevel int) {
		spec := &api.NetworkConfiguration{
			TypeMeta: v1.TypeMeta{
				Kind:       "NetworkConfiguration",
				APIVersion: "network.intel.com/v1alpha1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name: "test-network-configuration-host-nic",
			},
			Spec: api.NetworkConfigurationSpec{
				ConfigurationType: "host-nic",
				NodeSelector: map[string]string{
					"nodetype": nodeType,
				},
				HostNicScaleOut: api.HostNicScaleOutSpec{
					Vendor:  "intel",
					IPRange: iprange,
				},
				LogLevel: logLevel,
			},
		}

		// Apply and delete, don't care for the result.
		// Operator should be observed for ERRORs and crashes.

		kubernetesClient.Create(context.Background(), spec)

		time.Sleep(time.Millisecond * 5)

		kubernetesClient.Delete(context.Background(), spec)
	})
}
