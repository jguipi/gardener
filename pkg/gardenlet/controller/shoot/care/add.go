// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package care

import (
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	predicateutils "github.com/gardener/gardener/pkg/controllerutils/predicate"
	"github.com/gardener/gardener/pkg/utils"
	gardenerutils "github.com/gardener/gardener/pkg/utils/gardener"
)

// ControllerName is the name of this controller.
const ControllerName = "shoot-care"

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager, gardenCluster cluster.Cluster) error {
	if r.GardenClient == nil {
		r.GardenClient = gardenCluster.GetClient()
	}
	if r.Clock == nil {
		r.Clock = clock.RealClock{}
	}

	return builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: pointer.IntDeref(r.Config.Controllers.ShootCare.ConcurrentSyncs, 0),
			// if going into exponential backoff, wait at most the configured sync period
			RateLimiter: workqueue.NewWithMaxWaitRateLimiter(workqueue.DefaultControllerRateLimiter(), r.Config.Controllers.ShootCare.SyncPeriod.Duration),
		}).
		Watches(
			source.NewKindWithCache(&gardencorev1beta1.Shoot{}, gardenCluster.GetCache()),
			r.EventHandler(),
			builder.WithPredicates(
				predicateutils.SeedNamePredicate(r.SeedName, gardenerutils.GetShootSeedNames),
				r.ShootPredicate(),
			),
		).
		Complete(r)
}

// RandomDurationWithMetaDuration is an alias for utils.RandomDurationWithMetaDuration.
var RandomDurationWithMetaDuration = utils.RandomDurationWithMetaDuration

// EventHandler returns a handler for Shoot events.
func (r *Reconciler) EventHandler() handler.EventHandler {
	return &handler.Funcs{
		CreateFunc: func(e event.CreateEvent, q workqueue.RateLimitingInterface) {
			shoot, ok := e.Object.(*gardencorev1beta1.Shoot)
			if !ok {
				return
			}

			req := reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      e.Object.GetName(),
				Namespace: e.Object.GetNamespace(),
			}}

			if shoot.Generation == shoot.Status.ObservedGeneration {
				// spread shoot health checks across sync period to avoid checking on all Shoots roughly at the same
				// time after startup of the gardenlet
				q.AddAfter(req, RandomDurationWithMetaDuration(r.Config.Controllers.ShootCare.SyncPeriod))
				return
			}

			// don't add random duration for enqueueing new Shoots which have never been health checked yet
			q.Add(req)
		},
		UpdateFunc: func(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      e.ObjectNew.GetName(),
				Namespace: e.ObjectNew.GetNamespace(),
			}})
		},
	}
}

// ShootPredicate is a predicate which returns 'true' for create events, and for update events in case the shoot was
// successfully reconciled.
func (r *Reconciler) ShootPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			shoot, ok := e.ObjectNew.(*gardencorev1beta1.Shoot)
			if !ok {
				return false
			}

			oldShoot, ok := e.ObjectOld.(*gardencorev1beta1.Shoot)
			if !ok {
				return false
			}

			// re-evaluate shoot health status right after a reconciliation operation has succeeded
			return shootReconciliationFinishedSuccessful(oldShoot, shoot) || seedGotAssigned(oldShoot, shoot)
		},
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

func shootReconciliationFinishedSuccessful(oldShoot, newShoot *gardencorev1beta1.Shoot) bool {
	return oldShoot.Status.LastOperation != nil &&
		oldShoot.Status.LastOperation.Type != gardencorev1beta1.LastOperationTypeDelete &&
		oldShoot.Status.LastOperation.State == gardencorev1beta1.LastOperationStateProcessing &&
		newShoot.Status.LastOperation != nil &&
		newShoot.Status.LastOperation.Type != gardencorev1beta1.LastOperationTypeDelete &&
		newShoot.Status.LastOperation.State == gardencorev1beta1.LastOperationStateSucceeded
}

func seedGotAssigned(oldShoot, newShoot *gardencorev1beta1.Shoot) bool {
	return oldShoot.Spec.SeedName == nil && newShoot.Spec.SeedName != nil
}
