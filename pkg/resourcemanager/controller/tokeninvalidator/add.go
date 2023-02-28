// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package tokeninvalidator

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
const ControllerName = "token-invalidator"

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager, targetCluster cluster.Cluster) error {
	if r.TargetReader == nil {
		r.TargetReader = targetCluster.GetAPIReader()
	}
	if r.TargetClient == nil {
		r.TargetClient = targetCluster.GetClient()
	}

	secret := &metav1.PartialObjectMetadata{}
	secret.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))

	c, err := builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			RateLimiter:             r.RateLimiter,
		}).
		Watches(
			source.NewKindWithCache(secret, targetCluster.GetCache()),
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(r.SecretPredicate()),
		).
		Build(r)
	if err != nil {
		return err
	}

	return c.Watch(
		source.NewKindWithCache(&corev1.ServiceAccount{}, targetCluster.GetCache()),
		mapper.EnqueueRequestsFrom(mapper.MapFunc(r.MapServiceAccountToSecrets), mapper.UpdateWithOldAndNew, c.GetLogger()),
		r.ServiceAccountPredicate(),
	)
}

// SecretPredicate returns the predicate for secrets.
func (r *Reconciler) SecretPredicate() predicate.Predicate {
	isRelevantSecret := func(obj client.Object) bool {
		return obj.GetAnnotations()[corev1.ServiceAccountNameKey] != ""
	}

	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return isRelevantSecret(e.Object) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return isRelevantSecret(e.ObjectNew) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return false },
		GenericFunc: func(e event.GenericEvent) bool { return false },
	}
}

// ServiceAccountPredicate returns the predicate for service accounts.
func (r *Reconciler) ServiceAccountPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldSA, ok := e.ObjectOld.(*corev1.ServiceAccount)
			if !ok {
				return false
			}

			newSA, ok := e.ObjectNew.(*corev1.ServiceAccount)
			if !ok {
				return false
			}

			return !apiequality.Semantic.DeepEqual(oldSA.AutomountServiceAccountToken, newSA.AutomountServiceAccountToken) ||
				oldSA.Labels[resourcesv1alpha1.StaticTokenSkip] != newSA.Labels[resourcesv1alpha1.StaticTokenSkip]
		},
		DeleteFunc:  func(e event.DeleteEvent) bool { return false },
		GenericFunc: func(e event.GenericEvent) bool { return false },
	}
}

// MapServiceAccountToSecrets maps the ServiceAccount to all referenced secrets.
func (r *Reconciler) MapServiceAccountToSecrets(_ context.Context, _ logr.Logger, _ client.Reader, obj client.Object) []reconcile.Request {
	sa, ok := obj.(*corev1.ServiceAccount)
	if !ok {
		return nil
	}

	out := make([]reconcile.Request, 0, len(sa.Secrets))

	for _, secretRef := range sa.Secrets {
		out = append(out, reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      secretRef.Name,
			Namespace: sa.Namespace,
		}})
	}

	return out
}
