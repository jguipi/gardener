// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package extensionscheck

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/gardener/pkg/apis/core"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/apis/core/v1beta1/helper"
	"github.com/gardener/gardener/pkg/controllermanager/apis/config"
	"github.com/gardener/gardener/pkg/controllermanager/controller/seed/utils"
)

var conditionsToCheck = []gardencorev1beta1.ConditionType{
	gardencorev1beta1.ControllerInstallationValid,
	gardencorev1beta1.ControllerInstallationInstalled,
	gardencorev1beta1.ControllerInstallationHealthy,
	gardencorev1beta1.ControllerInstallationProgressing,
}

// Reconciler reconciles Seeds and maintains the ExtensionsReady condition according to the observed status of the
// referencing ControllerInstallations.
type Reconciler struct {
	Client client.Client
	Config config.SeedExtensionsCheckControllerConfiguration
	Clock  clock.Clock
}

// Reconcile reconciles Seeds and maintains the ExtensionsReady condition according to the observed status of the
// referencing ControllerInstallations.
func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	seed := &gardencorev1beta1.Seed{}
	if err := r.Client.Get(ctx, request.NamespacedName, seed); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	controllerInstallationList := &gardencorev1beta1.ControllerInstallationList{}
	if err := r.Client.List(ctx, controllerInstallationList, client.MatchingFields{core.SeedRefName: seed.Name}); err != nil {
		return reconcile.Result{}, err
	}

	var (
		notValid     = make(map[string]string)
		notInstalled = make(map[string]string)
		notHealthy   = make(map[string]string)
		progressing  = make(map[string]string)
	)

	for _, controllerInstallation := range controllerInstallationList.Items {
		// not needed for real client, but fake client doesn't support field selector
		// see https://github.com/kubernetes-sigs/controller-runtime/issues/1376
		// could be solved by switching from fake client to real client against envtest
		if controllerInstallation.Spec.SeedRef.Name != seed.Name {
			continue
		}

		if len(controllerInstallation.Status.Conditions) == 0 {
			notInstalled[controllerInstallation.Name] = "extension was not yet installed"
			continue
		}

		var (
			conditionsReady    = 0
			requiredConditions = map[gardencorev1beta1.ConditionType]struct{}{}
		)

		for _, condition := range conditionsToCheck {
			requiredConditions[condition] = struct{}{}
		}

		for _, condition := range controllerInstallation.Status.Conditions {
			if _, ok := requiredConditions[condition.Type]; !ok {
				continue
			}

			switch {
			case condition.Type == gardencorev1beta1.ControllerInstallationValid && condition.Status != gardencorev1beta1.ConditionTrue:
				notValid[controllerInstallation.Name] = condition.Message
			case condition.Type == gardencorev1beta1.ControllerInstallationInstalled && condition.Status != gardencorev1beta1.ConditionTrue:
				notInstalled[controllerInstallation.Name] = condition.Message
			case condition.Type == gardencorev1beta1.ControllerInstallationHealthy && condition.Status != gardencorev1beta1.ConditionTrue:
				notHealthy[controllerInstallation.Name] = condition.Message
			case condition.Type == gardencorev1beta1.ControllerInstallationProgressing && condition.Status != gardencorev1beta1.ConditionFalse:
				progressing[controllerInstallation.Name] = condition.Message
			}

			conditionsReady++
		}

		if _, found := notHealthy[controllerInstallation.Name]; !found && conditionsReady != len(requiredConditions) {
			notHealthy[controllerInstallation.Name] = "not all required conditions found in ControllerInstallation"
		}
	}

	condition := helper.GetOrInitConditionWithClock(r.Clock, seed.Status.Conditions, gardencorev1beta1.SeedExtensionsReady)
	extensionsReadyThreshold := utils.GetThresholdForCondition(r.Config.ConditionThresholds, gardencorev1beta1.SeedExtensionsReady)

	switch {
	case len(notValid) != 0:
		condition = utils.SetToProgressingOrFalse(r.Clock, extensionsReadyThreshold, condition, "NotAllExtensionsValid", fmt.Sprintf("Some extensions are not valid: %+v", notValid))
	case len(notInstalled) != 0:
		condition = utils.SetToProgressingOrFalse(r.Clock, extensionsReadyThreshold, condition, "NotAllExtensionsInstalled", fmt.Sprintf("Some extensions are not installed: %+v", notInstalled))
	case len(notHealthy) != 0:
		condition = utils.SetToProgressingOrFalse(r.Clock, extensionsReadyThreshold, condition, "NotAllExtensionsHealthy", fmt.Sprintf("Some extensions are not healthy: %+v", notHealthy))
	case len(progressing) != 0:
		condition = utils.SetToProgressingOrFalse(r.Clock, extensionsReadyThreshold, condition, "SomeExtensionsProgressing", fmt.Sprintf("Some extensions are progressing: %+v", progressing))
	default:
		condition = helper.UpdatedConditionWithClock(r.Clock, condition, gardencorev1beta1.ConditionTrue, "AllExtensionsReady", "All extensions installed into the seed cluster are ready and healthy.")
	}

	if err := utils.PatchSeedCondition(ctx, log, r.Client.Status(), seed, condition); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{RequeueAfter: r.Config.SyncPeriod.Duration}, nil
}
