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

package predicate_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	. "github.com/gardener/gardener/extensions/pkg/predicate"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
)

var _ = Describe("Default", func() {
	Describe("DefaultControllerPredicates", func() {
		var (
			pred      predicate.Predicate
			obj       *extensionsv1alpha1.Infrastructure
			namespace = "shoot--foo--bar"
			now       = time.Now()
		)

		BeforeEach(func() {
			obj = &extensionsv1alpha1.Infrastructure{ObjectMeta: metav1.ObjectMeta{Namespace: namespace}}
		})

		When("operation annotation is not ignored", func() {
			BeforeEach(func() {
				pred = DefaultControllerPredicates(false)[0]
			})

			Describe("#Create", func() {
				It("should return false when object is nil", func() {
					Expect(pred.Create(event.CreateEvent{})).To(BeFalse())
				})

				DescribeTable("return true when operation annotation is set",
					func(operation string) {
						obj.SetAnnotations(map[string]string{"gardener.cloud/operation": operation})
						Expect(pred.Create(event.CreateEvent{Object: obj})).To(BeTrue())
					},

					Entry("reconcile", "reconcile"),
					Entry("migrate", "migrate"),
					Entry("restore", "restore"),
				)

				It("should return true when the deletion timestamp is set", func() {
					obj.SetDeletionTimestamp(&metav1.Time{})
					Expect(pred.Create(event.CreateEvent{Object: obj})).To(BeTrue())
				})

				DescribeTable("return true when last operation has not succeeded",
					func(state gardencorev1beta1.LastOperationState) {
						obj.Status.LastOperation = &gardencorev1beta1.LastOperation{State: state}
						Expect(pred.Create(event.CreateEvent{Object: obj})).To(BeTrue())
					},

					Entry("processing", gardencorev1beta1.LastOperationStateProcessing),
					Entry("error", gardencorev1beta1.LastOperationStateError),
					Entry("failed", gardencorev1beta1.LastOperationStateFailed),
					Entry("pending", gardencorev1beta1.LastOperationStatePending),
					Entry("aborted", gardencorev1beta1.LastOperationStateAborted),
				)

				It("should return true when the last operation is not yet set", func() {
					Expect(pred.Create(event.CreateEvent{Object: obj})).To(BeTrue())
				})

				It("should return false when operation annotation is not present, object has no deletion timestamp and last operation has not succeeded", func() {
					obj.Status.LastOperation = &gardencorev1beta1.LastOperation{State: gardencorev1beta1.LastOperationStateSucceeded}
					Expect(pred.Create(event.CreateEvent{Object: obj})).To(BeFalse())
				})
			})

			Describe("#Update", func() {
				DescribeTable("return true when operation annotation is set",
					func(operation string) {
						obj.SetAnnotations(map[string]string{"gardener.cloud/operation": operation})
						Expect(pred.Update(event.UpdateEvent{ObjectNew: obj})).To(BeTrue())
					},

					Entry("reconcile", "reconcile"),
					Entry("migrate", "migrate"),
					Entry("restore", "restore"),
				)

				It("should return true when the deletion timestamp is set and the status is equal", func() {
					obj.SetDeletionTimestamp(&metav1.Time{})
					oldObj := obj.DeepCopy()
					Expect(pred.Update(event.UpdateEvent{ObjectNew: obj, ObjectOld: oldObj})).To(BeTrue())
				})

				It("should return false when the deletion timestamp is set and the status changed", func() {
					obj.SetDeletionTimestamp(&metav1.Time{})
					oldObj := obj.DeepCopy()
					obj.Status.ObservedGeneration = 3
					Expect(pred.Update(event.UpdateEvent{ObjectNew: obj, ObjectOld: oldObj})).To(BeFalse())
				})

				It("should return true when last operation has not succeeded and the timestamp changed", func() {
					obj.Status.LastOperation = &gardencorev1beta1.LastOperation{State: gardencorev1beta1.LastOperationStateError}
					oldObj := obj.DeepCopy()
					obj.SetAnnotations(map[string]string{"gardener.cloud/timestamp": now.UTC().Add(time.Second).String()})
					Expect(pred.Update(event.UpdateEvent{ObjectNew: obj, ObjectOld: oldObj})).To(BeTrue())
				})

				It("should return false when last operation has succeeded and the timestamp changed", func() {
					obj.Status.LastOperation = &gardencorev1beta1.LastOperation{State: gardencorev1beta1.LastOperationStateSucceeded}
					oldObj := obj.DeepCopy()
					obj.SetAnnotations(map[string]string{"gardener.cloud/timestamp": now.UTC().Add(time.Second).String()})
					Expect(pred.Update(event.UpdateEvent{ObjectNew: obj, ObjectOld: oldObj})).To(BeFalse())
				})
			})

			Describe("#Delete", func() {
				It("should return false", func() {
					Expect(pred.Delete(event.DeleteEvent{})).To(BeFalse())
				})
			})

			Describe("#Generic", func() {
				It("should return false", func() {
					Expect(pred.Generic(event.GenericEvent{})).To(BeFalse())
				})
			})
		})
	})
})
