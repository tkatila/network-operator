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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NetworkConfigurationSpec defines the desired state of NetworkConfiguration
type NetworkConfigurationSpec struct {
	// Configuration type that the operator will configure to the nodes. Possible options: gaudi-so.
	// TODO: plausible other options: host-nic
	// +kubebuilder:validation:Enum=gaudi-so
	ConfigurationType string `json:"configurationType"`

	// Select which nodes the operator should target. Align with labels created by NFD.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Gaudi Scale-Out specific settings. Only valid when configuration type is 'gaudi-so'
	GaudiScaleOut GaudiScaleOutSpec `json:"gaudiScaleOut,omitempty"`

	// Host-NIC Scale-Out specific settings. Only valid when configuration type is 'host-nic'
	HostNicScaleOut HostNicScaleOutSpec `json:"hostnicScaleOut,omitempty"`

	// LogLevel sets the operator's log level.
	// +kubebuilder:validation:Minimum=0
	LogLevel int `json:"logLevel,omitempty"`
}

// NetworkConfigurationSpec defines the desired state of NetworkConfiguration
type GaudiScaleOutSpec struct {
	// Layer where the configuration should occur. Possible options: L2 and L3.
	// +kubebuilder:validation:Enum=L2;L3
	Layer string `json:"layer,omitempty"`

	// Container image to handle interface configurations on the worker nodes.
	Image string `json:"image,omitempty"`

	// Normal image pull policy used in the resulting daemonset.
	// +kubebuilder:validation:Enum=Never;Always;IfNotPresent
	PullPolicy string `json:"pullPolicy,omitempty"`

	// IP range to be distributed for the scale-out interfaces over all nodes. Used with L3 layer selection.
	// Should be an IPv4 subnet string. e.g. 192.168.100.0/24
	// TODO: move to an external CRD and refer here?
	L3IpRange string `json:"l3IpRange,omitempty"`
}

// Alternative to Gaudi ScaleOut spec
// NOTE: Highly subject to change
type HostNicScaleOutSpec struct {
	// IP range to be distributed for the scale-out interfaces over all nodes.
	IPRange string `json:"ipRange,omitempty"`

	// Vendor for the scale-out NIC(s).
	// +kubebuilder:validation:Enum=melanox
	Vendor string `json:"vendor,omitempty"`
}

// NetworkConfigurationStatus defines the observed state of NetworkConfiguration
type NetworkConfigurationStatus struct {
	Targets    int32    `json:"targets"`
	ReadyNodes int32    `json:"ready"`
	State      string   `json:"state"`
	Errors     []string `json:"errors"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:path=networkconfigurations,scope=Cluster
//+kubebuilder:subresource:status

// NetworkConfiguration is the Schema for the networkconfigurations API
type NetworkConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkConfigurationSpec   `json:"spec,omitempty"`
	Status NetworkConfigurationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NetworkConfigurationList contains a list of NetworkConfiguration
type NetworkConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkConfiguration{}, &NetworkConfigurationList{})
}
