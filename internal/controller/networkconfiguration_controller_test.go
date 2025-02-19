// Copyright 2024 Intel Corporation. All Rights Reserved.
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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkv1alpha1 "github.com/intel/intel-network-operator-for-kubernetes/api/v1alpha1"
)

var _ = Describe("NetworkConfiguration Controller", func() {
	const (
		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		networkconfiguration := &networkv1alpha1.NetworkConfiguration{}

		It("should successfully reconcile the resource", func() {
			By("creating the custom resource for the Kind NetworkConfiguration")
			resource := &networkv1alpha1.NetworkConfiguration{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "network.intel.com/v1alpha1",
					Kind:       "NetworkConfiguration",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: networkv1alpha1.NetworkConfigurationSpec{
					ConfigurationType: "gaudi-so",
					GaudiScaleOut: networkv1alpha1.GaudiScaleOutSpec{
						Layer: "L3",
						Image: "intel/my-linkdiscovery:latest",
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, networkconfiguration)).To(Succeed())
				g.Expect(networkconfiguration.Spec.ConfigurationType).To(BeEquivalentTo("gaudi-so"))
				g.Expect(networkconfiguration.Status.Targets).To(BeIdenticalTo(int32(0)))
				g.Expect(networkconfiguration.Status.State).To(BeIdenticalTo("No targets"))
			}, timeout, interval).Should(Succeed())

			var ds apps.DaemonSet

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, &ds)).To(Succeed())
				g.Expect(ds.ObjectMeta.Name).To(BeEquivalentTo(typeNamespacedName.Name))
				g.Expect(ds.Spec.Template.Spec.Containers).To(HaveLen(1))
				g.Expect(ds.Spec.Template.Spec.Containers[0].Image).To(BeEquivalentTo("intel/my-linkdiscovery:latest"))
				g.Expect(ds.Spec.Template.Spec.Containers[0].Args).To(HaveLen(5))
				g.Expect(ds.Spec.Template.Spec.Containers[0].Args[0]).To(BeEquivalentTo("--mode=L3"))
				g.Expect(ds.Spec.Template.Spec.Containers[0].Args[1]).To(BeEquivalentTo("--wait=90"))
				g.Expect(ds.Spec.Template.Spec.Containers[0].Args[2]).To(BeEquivalentTo("--configure=true"))
				g.Expect(ds.Spec.Template.Spec.Containers[0].Args[3]).To(BeEquivalentTo("--gaudinet=/host/etc/habanalabs/gaudinet.json"))
				g.Expect(ds.Spec.Template.Spec.Containers[0].Args[4]).To(BeEquivalentTo("--keep-running"))
				g.Expect(ds.Spec.Template.Spec.Volumes).To(HaveLen(2))
				g.Expect(ds.Spec.Template.Spec.Volumes[0].Name).To(BeEquivalentTo("nfd-features"))
				g.Expect(ds.Spec.Template.Spec.Volumes[1].Name).To(BeEquivalentTo("gaudinetpath"))
				g.Expect(ds.Spec.Template.Spec.Containers[0].VolumeMounts).To(HaveLen(2))
				g.Expect(ds.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(BeEquivalentTo("nfd-features"))
				g.Expect(ds.Spec.Template.Spec.Containers[0].VolumeMounts[1].Name).To(BeEquivalentTo("gaudinetpath"))
			}, timeout, interval).Should(Succeed())

			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())

			resource.Spec.GaudiScaleOut.Layer = "L2"

			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, &ds)).To(Succeed())
				g.Expect(ds.ObjectMeta.Name).To(BeEquivalentTo(typeNamespacedName.Name))
				g.Expect(ds.Spec.Template.Spec.Containers).To(HaveLen(1))
				g.Expect(ds.Spec.Template.Spec.Containers[0].Args).To(HaveLen(3))
				g.Expect(ds.Spec.Template.Spec.Containers[0].Args[0]).To(BeEquivalentTo("--mode=L2"))
				g.Expect(ds.Spec.Template.Spec.Containers[0].Args[1]).To(BeEquivalentTo("--configure=true"))
				g.Expect(ds.Spec.Template.Spec.Containers[0].Args[2]).To(BeEquivalentTo("--keep-running"))
			}, timeout, interval).Should(Succeed())

			Expect(k8sClient.Delete(ctx, networkconfiguration)).To(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, &ds)).To(Not(Succeed()))
			}, timeout, interval).Should(Not(Succeed()))

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, networkconfiguration)).To(Not(Succeed()))
			}, timeout, interval).Should(Succeed())
		})
	})
})
