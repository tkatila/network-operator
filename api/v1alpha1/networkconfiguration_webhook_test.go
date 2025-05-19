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

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("NicClusterPolicy Webhook", func() {

	Context("When creating NetworkConfiguration under Defaulting Webhook", func() {
		It("Should fill in the default value if layer 3 is selected with Gaudi", func() {
			nc := NetworkConfiguration{}

			nc.Spec.ConfigurationType = gaudiScaleOut
			nc.Spec.GaudiScaleOut.Layer = "L2"

			nc.Default()

			Expect(nc.Spec.GaudiScaleOut.Image).To(BeEquivalentTo("intel/intel-network-linkdiscovery:latest"))
		})
	})

	Context("When creating NetworkConfiguration under Validating Webhook", func() {
		It("Should deny if there's no nodeSelector", func() {
			nc := NetworkConfiguration{}

			nc.Spec.ConfigurationType = gaudiScaleOut

			Expect(nc.ValidateCreate()).Error().NotTo(BeNil())
		})

		It("Should deny if the configuration type is invalid InputVal", func() {
			nc := NetworkConfiguration{}
			nc.Spec.NodeSelector = map[string]string{
				"foo": "bar",
			}

			nc.Spec.ConfigurationType = "foo bar"

			Expect(nc.ValidateCreate()).Error().To(BeEquivalentTo(unknownConfigurationError{}))
		})

		It("Should accept good nodeSelectors", func() {
			nc := NetworkConfiguration{
				Spec: NetworkConfigurationSpec{
					ConfigurationType: gaudiScaleOut,
					GaudiScaleOut: GaudiScaleOutSpec{
						Layer: "L3BGP",
					},
					NodeSelector: map[string]string{},
				},
			}

			goodValues := []map[string]string{
				{"intel.feature.node.kubernetes.io/gaudi-ready": "true"},
				{"gpu.intel.com": "xpu"},
			}

			for _, v := range goodValues {
				nc.Spec.NodeSelector = v

				Expect(nc.ValidateCreate()).Error().To(BeNil(), "selector: %+v", v)
			}
		})

		It("Should prevent bad nodeSelectors InputVal", func() {
			nc := NetworkConfiguration{
				Spec: NetworkConfigurationSpec{
					ConfigurationType: gaudiScaleOut,
					GaudiScaleOut: GaudiScaleOutSpec{
						Layer: "L3",
					},
					NodeSelector: map[string]string{
						"foobar.com?foo": "bar",
					},
				},
			}

			badValues := []map[string]string{
				{"__.com/foo": "bar"},
				{"foo.com_": "bar"},
				{"foo.com": "_bar"},
				{"foo.com": "???foo"},
				{"foo.com": "foo_"},
				{"foo.com": "0123456789012345678901234567890123456789012345678901234567890123"},
				{"foo.com/bar/plaaplaa_": "ok"},
				{"foo.com_/bar": "ok"},
			}

			for _, v := range badValues {
				nc.Spec.NodeSelector = v

				Expect(nc.ValidateCreate()).Error().To(Not(BeNil()), "selector: %+v", v)
			}
		})

		It("Should accept update with good values and fail with bad ones InputVal", func() {
			nc := NetworkConfiguration{
				ObjectMeta: v1.ObjectMeta{
					Name: "test",
				},
				Spec: NetworkConfigurationSpec{
					ConfigurationType: gaudiScaleOut,
					GaudiScaleOut: GaudiScaleOutSpec{
						Layer: "L3",
					},
					NodeSelector: map[string]string{
						"foo": "bar",
					},
				},
			}
			nc2 := nc.DeepCopy()

			Expect(nc2.ValidateUpdate(&nc)).Error().To(BeNil())

			nc2.Spec.NodeSelector = map[string]string{
				"foobar.com?foo": "bar", // bad
			}

			Expect(nc2.ValidateUpdate(&nc)).Error().NotTo(BeNil())
		})

		It("Should always accept delete", func() {
			nc := NetworkConfiguration{
				Spec: NetworkConfigurationSpec{
					ConfigurationType: gaudiScaleOut,
					GaudiScaleOut: GaudiScaleOutSpec{
						Layer: "L3BGP",
					},
					NodeSelector: map[string]string{
						"foo": "bar",
					},
				},
			}

			Expect(nc.ValidateDelete()).Error().To(BeNil())
		})
	})
})
