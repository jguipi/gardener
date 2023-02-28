// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package networkpolicy

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/controllerutils/mapper"
)

// ControllerName is the name of the controller.
const ControllerName = "networkpolicy"

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager, targetCluster cluster.Cluster) error {
	if r.TargetClient == nil {
		r.TargetClient = targetCluster.GetClient()
	}

	for _, n := range r.Config.NamespaceSelectors {
		namespaceSelector := n

		selector, err := metav1.LabelSelectorAsSelector(&namespaceSelector)
		if err != nil {
			return fmt.Errorf("failed parsing namespace selector %s to labels.Selector: %w", namespaceSelector, err)
		}
		r.selectors = append(r.selectors, selector)
	}

	c, err := builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: pointer.IntDeref(r.Config.ConcurrentSyncs, 0),
		}).
		Watches(
			source.NewKindWithCache(&corev1.Service{}, targetCluster.GetCache()),
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(r.ServicePredicate()),
		).
		Build(r)
	if err != nil {
		return err
	}

	namespace := &metav1.PartialObjectMetadata{}
	namespace.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Namespace"))

	return c.Watch(
		source.NewKindWithCache(namespace, targetCluster.GetCache()),
		mapper.EnqueueRequestsFrom(mapper.MapFunc(r.MapToAllServices), mapper.UpdateWithNew, c.GetLogger()),
	)
}

// ServicePredicate returns a predicate which filters UPDATE events on services such that only updates to the deletion
// timestamp, the port list, the pod label selector, or well-known annotations are relevant.
func (r *Reconciler) ServicePredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			service, ok := e.ObjectNew.(*corev1.Service)
			if !ok {
				return false
			}

			oldService, ok := e.ObjectOld.(*corev1.Service)
			if !ok {
				return false
			}

			return (oldService.DeletionTimestamp == nil && service.DeletionTimestamp != nil) ||
				!apiequality.Semantic.DeepEqual(service.Spec.Selector, oldService.Spec.Selector) ||
				!apiequality.Semantic.DeepEqual(service.Spec.Ports, oldService.Spec.Ports) ||
				oldService.Annotations[resourcesv1alpha1.NetworkingPodLabelSelectorNamespaceAlias] != service.Annotations[resourcesv1alpha1.NetworkingPodLabelSelectorNamespaceAlias] ||
				oldService.Annotations[resourcesv1alpha1.NetworkingNamespaceSelectors] != service.Annotations[resourcesv1alpha1.NetworkingNamespaceSelectors] ||
				oldService.Annotations[resourcesv1alpha1.NetworkingFromWorldToPorts] != service.Annotations[resourcesv1alpha1.NetworkingFromWorldToPorts] ||
				oldService.Annotations[resourcesv1alpha1.NetworkingFromPolicyPodLabelSelector] != service.Annotations[resourcesv1alpha1.NetworkingFromPolicyPodLabelSelector] ||
				oldService.Annotations[resourcesv1alpha1.NetworkingFromPolicyAllowedPorts] != service.Annotations[resourcesv1alpha1.NetworkingFromPolicyAllowedPorts]
		},
	}
}

// MapToAllServices is a mapper.MapFunc for mapping a Namespace to all Services.
func (r *Reconciler) MapToAllServices(ctx context.Context, log logr.Logger, _ client.Reader, _ client.Object) []reconcile.Request {
	serviceList := &metav1.PartialObjectMetadataList{}
	serviceList.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ServiceList"))
	if err := r.TargetClient.List(ctx, serviceList); err != nil {
		log.Error(err, "Failed to list services")
		return nil
	}

	var requests []reconcile.Request

	for _, service := range serviceList.Items {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: service.Name, Namespace: service.Namespace}})
	}

	return requests
}
