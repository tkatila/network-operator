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
	"net/netip"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var networkconfigurationlog = logf.Log.WithName("networkconfiguration-resource")

const (
	gaudiScaleOut = "gaudi-so"
)

type ipRangeError struct{}

func (i ipRangeError) Error() string {
	return "Invalid IP Range provided"
}

type emptyNodeSelectorError struct{}

func (e emptyNodeSelectorError) Error() string {
	return "empty node-selector"
}

type unknownConfigurationError struct{}

func (e unknownConfigurationError) Error() string {
	return "unknown error"
}

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *NetworkConfiguration) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-network-intel-com-v1alpha1-networkconfiguration,mutating=true,failurePolicy=fail,sideEffects=None,groups=network.intel.com,resources=networkconfigurations,verbs=create;update,versions=v1alpha1,name=mnetworkconfiguration.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &NetworkConfiguration{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *NetworkConfiguration) Default() {
	networkconfigurationlog.Info("default", "name", r.Name)

	switch r.Spec.ConfigurationType {
	case gaudiScaleOut:
		if len(r.Spec.GaudiScaleOut.Image) == 0 {
			r.Spec.GaudiScaleOut.Image = "intel/intel-network-linkdiscovery:latest"
		}
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
//+kubebuilder:webhook:path=/validate-network-intel-com-v1alpha1-networkconfiguration,mutating=false,failurePolicy=fail,sideEffects=None,groups=network.intel.com,resources=networkconfigurations,verbs=create;update,versions=v1alpha1,name=vnetworkconfiguration.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &NetworkConfiguration{}

func validateGaudiSoSpec(s GaudiScaleOutSpec) error {
	if len(s.L3IpRange) > 0 {
		if !strings.Contains(s.L3IpRange, "/") {
			return ipRangeError{}
		}

		parts := strings.Split(s.L3IpRange, "/")

		_, err := netip.ParseAddr(parts[0])
		if err != nil {
			return err
		}

		mask, err := strconv.ParseUint(parts[1], 10, 8)
		if err != nil {
			return err
		}
		if mask > 32 {
			return ipRangeError{}
		}
	}

	return nil
}

func validateSpec(s NetworkConfigurationSpec) (admission.Warnings, error) {
	if len(s.NodeSelector) == 0 {
		return nil, emptyNodeSelectorError{}
	}

	switch s.ConfigurationType {
	case gaudiScaleOut:
		return nil, validateGaudiSoSpec(s.GaudiScaleOut)
	default:
		return nil, unknownConfigurationError{}
	}
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *NetworkConfiguration) ValidateCreate() (admission.Warnings, error) {
	networkconfigurationlog.Info("validate create", "name", r.Name)

	return validateSpec(r.Spec)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *NetworkConfiguration) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	networkconfigurationlog.Info("validate update", "name", r.Name)

	return validateSpec(r.Spec)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *NetworkConfiguration) ValidateDelete() (admission.Warnings, error) {
	networkconfigurationlog.Info("validate delete", "name", r.Name)

	return nil, nil
}
