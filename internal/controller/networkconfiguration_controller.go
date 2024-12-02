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

// TODO:
// * Move gaudi scale-out specific code under a "gaudi controller". In preparation for host-nic scale-out scenarios.
// * Gather possible warnings/errors from Pods into CR's errors

package controller

import (
	"context"
	"fmt"
	"os"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	networkv1alpha1 "github.com/intel/intel-network-operator-for-kubernetes/api/v1alpha1"
	daemonsets "github.com/intel/intel-network-operator-for-kubernetes/config/daemonsets"
)

//+kubebuilder:rbac:groups=network.intel.com,resources=networkconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=network.intel.com,resources=networkconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=network.intel.com,resources=networkconfigurations/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch

// NetworkConfigurationReconciler reconciles a NetworkConfiguration object
type NetworkConfigurationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	ownerKey = ".metadata.controller"

	gaudiScaleOutSelection = "gaudi-so"

	layerSelectionL2    = "L2"
	layerSelectionL3    = "L3"
	layerSelectionL3BGP = "L3BGP"
)

// fetchObjects returns the required objects for Reconcile.
func (r *NetworkConfigurationReconciler) fetchExistingDaemonSet(ctx context.Context, req ctrl.Request, log logr.Logger) (*apps.DaemonSetList, error) {
	var childDaemonSets apps.DaemonSetList
	if err := r.List(ctx, &childDaemonSets, client.InNamespace(req.Namespace), client.MatchingFields{ownerKey: req.Name}); err != nil {
		log.Error(err, "unable to list child DaemonSets")

		return nil, err
	}

	return &childDaemonSets, nil
}

func updateGaudiScaleOutDaemonSet(ds *apps.DaemonSet, netconf *networkv1alpha1.NetworkConfiguration) {
	ds.Name = netconf.Name
	ds.ObjectMeta.Namespace = netconf.Namespace
	ds.ObjectMeta.Name = netconf.Name

	if len(netconf.Spec.NodeSelector) > 0 {
		ds.Spec.Template.Spec.NodeSelector = netconf.Spec.NodeSelector
	}

	if len(netconf.Spec.GaudiScaleOut.Image) > 0 {
		ds.Spec.Template.Spec.Containers[0].Image = netconf.Spec.GaudiScaleOut.Image
	}

	args := []string{}

	switch netconf.Spec.GaudiScaleOut.Layer {
	case layerSelectionL2:
		fallthrough
	case layerSelectionL3:
		fallthrough
	case layerSelectionL3BGP:
		toAdd := fmt.Sprintf("--layer=%s", netconf.Spec.GaudiScaleOut.Layer)

		args = append(args, toAdd)
	}

	ds.Spec.Template.Spec.Containers[0].Args = args
}

func (r *NetworkConfigurationReconciler) createGaudiScaleOutDaemonset(netconf client.Object, ctx context.Context, log logr.Logger) (ctrl.Result, error) {
	ds := daemonsets.GaudiL3BGPDaemonSet()

	cr := netconf.(*networkv1alpha1.NetworkConfiguration)

	log.Info("Creating Gaudi Scale-Out DaemonSet", "name", cr.Name)

	updateGaudiScaleOutDaemonSet(ds, cr)

	if err := ctrl.SetControllerReference(netconf.(metav1.Object), ds, r.Scheme); err != nil {
		log.Error(err, "unable to set controller reference")

		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, ds); err != nil {
		log.Error(err, "unable to create DaemonSet")

		return ctrl.Result{}, err
	}

	log.Info("Gaudi scale-out daemonset created")

	return ctrl.Result{}, nil
}

func (r *NetworkConfigurationReconciler) createDaemonSet(ctx context.Context, netconf client.Object, log logr.Logger) (ctrl.Result, error) {
	cr := netconf.(*networkv1alpha1.NetworkConfiguration)

	switch cr.Spec.ConfigurationType {
	case gaudiScaleOutSelection:
		return r.createGaudiScaleOutDaemonset(netconf, ctx, log)
	default:
		log.Info("Unknown configuration type, this shouldn't happen!", "type", cr.Spec.ConfigurationType)

		return ctrl.Result{}, os.ErrInvalid
	}
}

func (r *NetworkConfigurationReconciler) updateDaemonSet(ds *apps.DaemonSet, netconf client.Object) {
	cr := netconf.(*networkv1alpha1.NetworkConfiguration)

	switch cr.Spec.ConfigurationType {
	case gaudiScaleOutSelection:
		updateGaudiScaleOutDaemonSet(ds, cr)
	default:
		panic("Unknown configuration type, this shouldn't happen!")
	}
}

func (r *NetworkConfigurationReconciler) updateStatus(rawObj client.Object, ds *apps.DaemonSet, ctx context.Context, log logr.Logger) (ctrl.Result, error) {
	nc := rawObj.(*networkv1alpha1.NetworkConfiguration)

	updated := false

	if nc.Status.Targets != ds.Status.DesiredNumberScheduled {
		nc.Status.Targets = ds.Status.DesiredNumberScheduled
		updated = true
	}

	if nc.Status.ReadyNodes != ds.Status.NumberReady {
		nc.Status.ReadyNodes = ds.Status.NumberReady
		updated = true
	}

	nc.Status.Errors = []string{}

	if nc.Status.ReadyNodes < nc.Status.Targets {
		nc.Status.State = "Working on it.."
	} else {
		nc.Status.State = "All good"
	}

	if updated {
		if err := r.Status().Update(ctx, nc); apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			log.Error(err, "unable to update network conf status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func createEmptyObject() client.Object {
	return &networkv1alpha1.NetworkConfiguration{}
}

func (r *NetworkConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	log := log.FromContext(ctx)

	log.Info("Reconcile now.")

	netConfObj := createEmptyObject()

	if err := r.Get(ctx, req.NamespacedName, netConfObj); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("nothing to do")

			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	// fetch possible existing daemonset

	olderDs, err := r.fetchExistingDaemonSet(ctx, req, log)
	if err != nil {
		log.Error(err, "failed to fetch previous daemonsets")

		return ctrl.Result{}, err
	}

	if len(olderDs.Items) == 0 {
		return r.createDaemonSet(ctx, netConfObj, log)
	}

	// Update DaemonSet

	ds := &olderDs.Items[0]
	originalDs := ds.DeepCopy()

	r.updateDaemonSet(ds, netConfObj)

	dsDiff := cmp.Diff(originalDs.Spec.Template.Spec, ds.Spec.Template.Spec, diff.IgnoreUnset())
	if len(dsDiff) > 0 {
		log.Info("DS difference", "diff", dsDiff)

		if err := r.Update(ctx, ds); err != nil {
			log.Error(err, "unable to update daemonset", "DaemonSet", ds)

			return ctrl.Result{}, err
		}
	}

	// Update Pods Statuses

	return r.updateStatus(netConfObj, ds, ctx, log)
}

func indexDaemonSets(ctx context.Context, mgr ctrl.Manager, apiGVString, pluginKind string) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &apps.DaemonSet{}, ownerKey,
		func(rawObj client.Object) []string {
			// grab the DaemonSet object, extract the owner...
			ds := rawObj.(*apps.DaemonSet)
			owner := metav1.GetControllerOf(ds)

			if owner == nil {
				return nil
			}

			// make sure it's a network configuration
			if owner.APIVersion != apiGVString || owner.Kind != pluginKind {
				return nil
			}

			// and if so, return it.
			return []string{owner.Name}
		})
}

func indexPods(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &v1.Pod{}, ownerKey,
		func(rawObj client.Object) []string {
			// grab the Pod object, extract the owner...
			pod := rawObj.(*v1.Pod)
			owner := metav1.GetControllerOf(pod)

			if owner == nil {
				return nil
			}

			// make sure it's a DaemonSet
			if owner.APIVersion != apps.SchemeGroupVersion.String() || owner.Kind != "DaemonSet" {
				return nil
			}

			// and if so, return it.
			return []string{owner.Name}
		})
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Scheme = mgr.GetScheme()

	ctx := context.Background()
	apiGVString := networkv1alpha1.GroupVersion.String()
	kind := "NetworkConfiguration"

	// Index DaemonSets (CR).
	if err := indexDaemonSets(ctx, mgr, apiGVString, kind); err != nil {
		return err
	}

	// Index Pods with their owner (DaemonSet).
	if err := indexPods(ctx, mgr); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkv1alpha1.NetworkConfiguration{}).
		Owns(&apps.DaemonSet{}).
		Complete(r)
}
