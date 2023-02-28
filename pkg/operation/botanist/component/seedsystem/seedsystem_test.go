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

package seedsystem_test

import (
	"context"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/operation/botanist/component"
	. "github.com/gardener/gardener/pkg/operation/botanist/component/seedsystem"
	"github.com/gardener/gardener/pkg/utils/retry"
	retryfake "github.com/gardener/gardener/pkg/utils/retry/fake"
	"github.com/gardener/gardener/pkg/utils/test"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
)

var _ = Describe("SeedSystem", func() {
	var (
		ctx = context.TODO()

		managedResourceName        = "system"
		namespace                  = "some-namespace"
		reserveExcessCapacityImage = "some-image:some-tag"

		c         client.Client
		values    Values
		component component.DeployWaiter

		managedResource       *resourcesv1alpha1.ManagedResource
		managedResourceSecret *corev1.Secret

		deploymentYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    resources.gardener.cloud/skip-health-check: "true"
  creationTimestamp: null
  labels:
    app: kubernetes
    role: reserve-excess-capacity
  name: reserve-excess-capacity
  namespace: ` + namespace + `
spec:
  replicas: 2
  revisionHistoryLimit: 2
  selector:
    matchLabels:
      app: kubernetes
      role: reserve-excess-capacity
  strategy: {}
  template:
    metadata:
      creationTimestamp: null
      labels:
        app: kubernetes
        role: reserve-excess-capacity
    spec:
      containers:
      - image: ` + reserveExcessCapacityImage + `
        imagePullPolicy: IfNotPresent
        name: pause-container
        resources:
          limits:
            cpu: "2"
            memory: 6Gi
          requests:
            cpu: "2"
            memory: 6Gi
      priorityClassName: gardener-reserve-excess-capacity
      terminationGracePeriodSeconds: 5
status: {}
`
	)

	BeforeEach(func() {
		c = fakeclient.NewClientBuilder().WithScheme(kubernetes.SeedScheme).Build()
		values = Values{
			ReserveExcessCapacity: ReserveExcessCapacityValues{
				Enabled:  true,
				Image:    reserveExcessCapacityImage,
				Replicas: 2,
			},
		}
		component = New(c, namespace, values)

		managedResource = &resourcesv1alpha1.ManagedResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      managedResourceName,
				Namespace: namespace,
			},
		}
		managedResourceSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "managedresource-" + managedResource.Name,
				Namespace: namespace,
			},
		}
	})

	Describe("#Deploy", func() {
		JustBeforeEach(func() {
			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResource), managedResource)).To(BeNotFoundError())
			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResourceSecret), managedResourceSecret)).To(BeNotFoundError())

			Expect(component.Deploy(ctx)).To(Succeed())

			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResource), managedResource)).To(Succeed())
			Expect(managedResource).To(DeepEqual(&resourcesv1alpha1.ManagedResource{
				TypeMeta: metav1.TypeMeta{
					APIVersion: resourcesv1alpha1.SchemeGroupVersion.String(),
					Kind:       "ManagedResource",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            managedResource.Name,
					Namespace:       managedResource.Namespace,
					Labels:          map[string]string{"gardener.cloud/role": "seed-system-component"},
					ResourceVersion: "1",
				},
				Spec: resourcesv1alpha1.ManagedResourceSpec{
					Class: pointer.String("seed"),
					SecretRefs: []corev1.LocalObjectReference{{
						Name: managedResourceSecret.Name,
					}},
					KeepObjects: pointer.Bool(false),
				},
			}))

			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResourceSecret), managedResourceSecret)).To(Succeed())
			Expect(managedResourceSecret.Type).To(Equal(corev1.SecretTypeOpaque))
		})

		It("should successfully deploy the resources", func() {
			Expect(managedResourceSecret.Data).To(HaveLen(11))
			Expect(string(managedResourceSecret.Data["deployment__"+namespace+"__reserve-excess-capacity.yaml"])).To(Equal(deploymentYAML))
			expectPriorityClasses(managedResourceSecret.Data)
		})
	})

	Describe("#Destroy", func() {
		It("should successfully destroy all resources", func() {
			Expect(c.Create(ctx, managedResource)).To(Succeed())
			Expect(c.Create(ctx, managedResourceSecret)).To(Succeed())

			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResource), managedResource)).To(Succeed())
			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResourceSecret), managedResourceSecret)).To(Succeed())

			Expect(component.Destroy(ctx)).To(Succeed())

			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResource), managedResource)).To(BeNotFoundError())
			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResourceSecret), managedResourceSecret)).To(BeNotFoundError())
		})
	})

	Context("waiting functions", func() {
		var fakeOps *retryfake.Ops

		BeforeEach(func() {
			fakeOps = &retryfake.Ops{MaxAttempts: 1}
			DeferCleanup(test.WithVars(
				&retry.Until, fakeOps.Until,
				&retry.UntilTimeout, fakeOps.UntilTimeout,
			))
		})

		Describe("#Wait", func() {
			It("should fail because reading the ManagedResource fails", func() {
				Expect(component.Wait(ctx)).To(MatchError(ContainSubstring("not found")))
			})

			It("should fail because the ManagedResource doesn't become healthy", func() {
				fakeOps.MaxAttempts = 2

				Expect(c.Create(ctx, &resourcesv1alpha1.ManagedResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:       managedResourceName,
						Namespace:  namespace,
						Generation: 1,
					},
					Status: resourcesv1alpha1.ManagedResourceStatus{
						ObservedGeneration: 1,
						Conditions: []gardencorev1beta1.Condition{
							{
								Type:   resourcesv1alpha1.ResourcesApplied,
								Status: gardencorev1beta1.ConditionFalse,
							},
							{
								Type:   resourcesv1alpha1.ResourcesHealthy,
								Status: gardencorev1beta1.ConditionFalse,
							},
						},
					},
				})).To(Succeed())

				Expect(component.Wait(ctx)).To(MatchError(ContainSubstring("is not healthy")))
			})

			It("should successfully wait for the managed resource to become healthy", func() {
				fakeOps.MaxAttempts = 2

				Expect(c.Create(ctx, &resourcesv1alpha1.ManagedResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:       managedResourceName,
						Namespace:  namespace,
						Generation: 1,
					},
					Status: resourcesv1alpha1.ManagedResourceStatus{
						ObservedGeneration: 1,
						Conditions: []gardencorev1beta1.Condition{
							{
								Type:   resourcesv1alpha1.ResourcesApplied,
								Status: gardencorev1beta1.ConditionTrue,
							},
							{
								Type:   resourcesv1alpha1.ResourcesHealthy,
								Status: gardencorev1beta1.ConditionTrue,
							},
						},
					},
				})).To(Succeed())

				Expect(component.Wait(ctx)).To(Succeed())
			})
		})

		Describe("#WaitCleanup", func() {
			It("should fail when the wait for the managed resource deletion times out", func() {
				fakeOps.MaxAttempts = 2

				Expect(c.Create(ctx, managedResource)).To(Succeed())

				Expect(component.WaitCleanup(ctx)).To(MatchError(ContainSubstring("still exists")))
			})

			It("should not return an error when it's already removed", func() {
				Expect(component.WaitCleanup(ctx)).To(Succeed())
			})
		})
	})
})

func expectPriorityClasses(data map[string][]byte) {
	expected := []struct {
		name        string
		value       int32
		description string
	}{
		{"gardener-system-900", 999998900, "PriorityClass for Seed system components"},
		{"gardener-system-800", 999998800, "PriorityClass for Seed system components"},
		{"gardener-system-700", 999998700, "PriorityClass for Seed system components"},
		{"gardener-system-600", 999998600, "PriorityClass for Seed system components"},
		{"gardener-reserve-excess-capacity", -5, "PriorityClass for reserving excess capacity on a Seed cluster"},
		{"gardener-system-500", 999998500, "PriorityClass for Shoot control plane components"},
		{"gardener-system-400", 999998400, "PriorityClass for Shoot control plane components"},
		{"gardener-system-300", 999998300, "PriorityClass for Shoot control plane components"},
		{"gardener-system-200", 999998200, "PriorityClass for Shoot control plane components"},
		{"gardener-system-100", 999998100, "PriorityClass for Shoot control plane components"},
	}

	for _, pc := range expected {
		ExpectWithOffset(1, data).To(HaveKeyWithValue("priorityclass____"+pc.name+".yaml", []byte(`apiVersion: scheduling.k8s.io/v1
description: `+pc.description+`
kind: PriorityClass
metadata:
  creationTimestamp: null
  name: `+pc.name+`
value: `+strconv.FormatInt(int64(pc.value), 10)+`
`),
		))
	}
}
