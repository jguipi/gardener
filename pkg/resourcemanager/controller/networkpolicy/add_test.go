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

package networkpolicy_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	resourcemanagerclient "github.com/gardener/gardener/pkg/resourcemanager/client"
	. "github.com/gardener/gardener/pkg/resourcemanager/controller/networkpolicy"
)

var _ = Describe("Add", func() {
	var (
		ctx        = context.TODO()
		log        logr.Logger
		fakeClient client.Client
		reconciler *Reconciler
	)

	BeforeEach(func() {
		fakeClient = fakeclient.NewClientBuilder().WithScheme(resourcemanagerclient.TargetScheme).Build()
		reconciler = &Reconciler{
			TargetClient: fakeClient,
		}
	})

	Describe("#ServicePredicate", func() {
		var (
			p       predicate.Predicate
			service *corev1.Service
		)

		BeforeEach(func() {
			p = reconciler.ServicePredicate()
			service = &corev1.Service{}
		})

		Describe("#Create", func() {
			It("should return true", func() {
				Expect(p.Create(event.CreateEvent{})).To(BeTrue())
			})
		})

		Describe("#Update", func() {
			It("should return false because new object is no service", func() {
				Expect(p.Update(event.UpdateEvent{})).To(BeFalse())
			})

			It("should return false because old object is no service", func() {
				Expect(p.Update(event.UpdateEvent{ObjectNew: service})).To(BeFalse())
			})

			It("should return false because nothing changed", func() {
				Expect(p.Update(event.UpdateEvent{ObjectOld: service, ObjectNew: service})).To(BeFalse())
			})

			It("should return true because the deletion timestamp was set", func() {
				oldService := service.DeepCopy()
				service.DeletionTimestamp = &metav1.Time{}

				Expect(p.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: service})).To(BeTrue())
			})

			It("should return true because the selector was changed", func() {
				oldService := service.DeepCopy()
				service.Spec.Selector = map[string]string{"foo": "bar"}

				Expect(p.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: service})).To(BeTrue())
			})

			It("should return true because the ports were changed", func() {
				oldService := service.DeepCopy()
				service.Spec.Ports = []corev1.ServicePort{{}}

				Expect(p.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: service})).To(BeTrue())
			})

			It("should return true because the namespace-selectors annotation was changed", func() {
				oldService := service.DeepCopy()
				service.Annotations = map[string]string{"networking.resources.gardener.cloud/namespace-selectors": "foo"}

				Expect(p.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: service})).To(BeTrue())
			})

			It("should return true because the pod-label-selector-namespace-alias annotation was changed", func() {
				oldService := service.DeepCopy()
				service.Annotations = map[string]string{"networking.resources.gardener.cloud/pod-label-selector-namespace-alias": "foo"}

				Expect(p.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: service})).To(BeTrue())
			})

			It("should return true because the from-world-to-ports annotation was changed", func() {
				oldService := service.DeepCopy()
				service.Annotations = map[string]string{"networking.resources.gardener.cloud/from-world-to-ports": "foo"}

				Expect(p.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: service})).To(BeTrue())
			})

			It("should return true because the from-policy-pod-label-selector annotation was changed", func() {
				oldService := service.DeepCopy()
				service.Annotations = map[string]string{"networking.resources.gardener.cloud/from-policy-pod-label-selector": "foo"}

				Expect(p.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: service})).To(BeTrue())
			})

			It("should return true because the from-policy-allowed-ports annotation was changed", func() {
				oldService := service.DeepCopy()
				service.Annotations = map[string]string{"networking.resources.gardener.cloud/from-policy-allowed-ports": "foo"}

				Expect(p.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: service})).To(BeTrue())
			})
		})

		Describe("#Delete", func() {
			It("should return true", func() {
				Expect(p.Delete(event.DeleteEvent{})).To(BeTrue())
			})
		})

		Describe("#Generic", func() {
			It("should return true", func() {
				Expect(p.Generic(event.GenericEvent{})).To(BeTrue())
			})
		})
	})

	Describe("#MapToAllServices", func() {
		var (
			service1 *corev1.Service
			service2 *corev1.Service
		)

		BeforeEach(func() {
			log = logr.Discard()

			service1 = &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "service1", Namespace: "namespace1"}}
			service2 = &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "service2", Namespace: "namespace2"}}
		})

		It("should map to all services", func() {
			Expect(fakeClient.Create(ctx, service1)).To(Succeed())
			Expect(fakeClient.Create(ctx, service2)).To(Succeed())

			Expect(reconciler.MapToAllServices(ctx, log, nil, nil)).To(ConsistOf(
				reconcile.Request{NamespacedName: types.NamespacedName{Namespace: service1.Namespace, Name: service1.Name}},
				reconcile.Request{NamespacedName: types.NamespacedName{Namespace: service2.Namespace, Name: service2.Name}},
			))
		})

		It("should return nil if there are no services", func() {
			Expect(reconciler.MapToAllServices(ctx, log, nil, nil)).To(BeNil())
		})
	})
})
