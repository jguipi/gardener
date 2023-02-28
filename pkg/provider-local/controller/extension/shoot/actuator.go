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

package shoot

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener/extensions/pkg/controller/extension"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	kubernetesclient "github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/utils/managedresources"
)

const (
	// ApplicationName is the name of the application.
	ApplicationName string = "local-ext-shoot"
	// ManagedResourceNamesShoot is the name used to describe the managed shoot resources.
	ManagedResourceNamesShoot string = ApplicationName
)

type actuator struct {
	client client.Client
}

// NewActuator returns an actuator responsible for Extension resources.
func NewActuator() extension.Actuator {
	return &actuator{}
}

// InjectClient injects the controller runtime client into the reconciler.
func (a *actuator) InjectClient(client client.Client) error {
	a.client = client
	return nil
}

// Reconcile the extension resource.
func (a *actuator) Reconcile(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	namespace := ex.Namespace

	shootResources, err := getShootResources()
	if err != nil {
		return err
	}

	var (
		injectedLabels       = map[string]string{v1beta1constants.ShootNoCleanup: "true"}
		secretNameWithPrefix = true
		keepObjects          = false
	)

	return managedresources.Create(
		ctx,
		a.client,
		namespace,
		ManagedResourceNamesShoot,
		map[string]string{},
		secretNameWithPrefix,
		"",
		shootResources,
		&keepObjects,
		injectedLabels,
		nil,
	)
}

// Delete the extension resource.
func (a *actuator) Delete(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	namespace := ex.GetNamespace()
	twoMinutes := 2 * time.Minute

	timeoutShootCtx, cancelShootCtx := context.WithTimeout(ctx, twoMinutes)
	defer cancelShootCtx()

	if err := managedresources.DeleteForShoot(ctx, a.client, namespace, ManagedResourceNamesShoot); err != nil {
		return err
	}

	return managedresources.WaitUntilDeleted(timeoutShootCtx, a.client, namespace, ManagedResourceNamesShoot)
}

// Migrate the extension resource.
func (a *actuator) Migrate(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	// Keep objects for shoot managed resources so that they are not deleted from the shoot during the migration
	if err := managedresources.SetKeepObjects(ctx, a.client, ex.GetNamespace(), ManagedResourceNamesShoot, true); err != nil {
		return err
	}

	return a.Delete(ctx, log, ex)
}

// Restore the extension resource.
func (a *actuator) Restore(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	return a.Reconcile(ctx, log, ex)
}

func getLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name": ApplicationName,
	}
}

func getShootResources() (map[string][]byte, error) {
	shootRegistry := managedresources.NewRegistry(kubernetesclient.ShootScheme, kubernetesclient.ShootCodec, kubernetesclient.ShootSerializer)
	return shootRegistry.AddAllAndSerialize(
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ApplicationName,
				Namespace: metav1.NamespaceSystem,
				Labels:    getLabels(),
			},
			AutomountServiceAccountToken: pointer.Bool(false),
		},
	)
}
