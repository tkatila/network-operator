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
)

var _ = Describe("NetworkConfiguration Webhook", func() {

	Context("When creating NetworkConfiguration under Defaulting Webhook", func() {
		It("Should fill in the default value if layer 3 is selected with Gaudi", func() {
			nc := NetworkConfiguration{}

			nc.Spec.ConfigurationType = "gaudi-so"
			nc.Spec.GaudiScaleOut.Layer = "L3"

			nc.Default()

			Expect(nc.Spec.GaudiScaleOut.Image).To(BeEquivalentTo("intel/intel-gaudi-scaleout-conf:0.0.1"))
			Expect(nc.Spec.GaudiScaleOut.L3IpRange).To(BeEquivalentTo("192.168.10.0/24"))
		})
	})

	Context("When creating NetworkConfiguration under Validating Webhook", func() {
		It("Should deny if there's no nodeSelector", func() {
			nc := NetworkConfiguration{}

			nc.Spec.ConfigurationType = "gaudi-so"

			Expect(nc.ValidateCreate()).Error().NotTo(BeNil())
		})

		It("Should deny if there's a bad IP range", func() {
			nc := NetworkConfiguration{
				Spec: NetworkConfigurationSpec{
					ConfigurationType: "gaudi-so",
					GaudiScaleOut: GaudiScaleOutSpec{
						Layer: "L3",
					},
					NodeSelector: map[string]string{
						"foo": "bar",
					},
				},
			}

			nc.Spec.GaudiScaleOut.L3IpRange = "10.0.0/20"

			Expect(nc.ValidateCreate()).Error().NotTo(BeNil())

			nc.Spec.GaudiScaleOut.L3IpRange = "10.10.0.0/44"

			Expect(nc.ValidateCreate()).Error().NotTo(BeNil())
		})

		It("Should accept if there's a good IP range", func() {
			nc := NetworkConfiguration{
				Spec: NetworkConfigurationSpec{
					ConfigurationType: "gaudi-so",
					GaudiScaleOut: GaudiScaleOutSpec{
						Layer:     "L3",
						L3IpRange: "10.20.0.0/20",
					},
					NodeSelector: map[string]string{
						"foo": "bar",
					},
				},
			}

			Expect(nc.ValidateCreate()).Error().To(BeNil())
		})

		It("Should accept update with good values and fail with bad ones", func() {
			nc := NetworkConfiguration{
				Spec: NetworkConfigurationSpec{
					ConfigurationType: "gaudi-so",
					GaudiScaleOut: GaudiScaleOutSpec{
						Layer:     "L3",
						L3IpRange: "10.20.0.0/20",
					},
					NodeSelector: map[string]string{
						"foo": "bar",
					},
				},
			}
			nc2 := nc.DeepCopy()

			Expect(nc2.ValidateUpdate(&nc)).Error().To(BeNil())

			nc2.Spec.GaudiScaleOut.L3IpRange = "10.20.0.0/99" // bad

			Expect(nc2.ValidateUpdate(&nc)).Error().NotTo(BeNil())
		})

		It("Should always accept delete", func() {
			nc := NetworkConfiguration{
				Spec: NetworkConfigurationSpec{
					ConfigurationType: "gaudi-so",
					GaudiScaleOut: GaudiScaleOutSpec{
						Layer:     "L3",
						L3IpRange: "10.20.0.0/20",
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
