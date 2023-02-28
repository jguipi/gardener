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

package predicate_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	. "github.com/gardener/gardener/pkg/controllerutils/predicate"
	"github.com/gardener/gardener/pkg/utils/test"
)

var _ = Describe("Predicate", func() {
	Describe("#IsDeleting", func() {
		var (
			shoot        *gardencorev1beta1.Shoot
			predicate    predicate.Predicate
			createEvent  event.CreateEvent
			updateEvent  event.UpdateEvent
			deleteEvent  event.DeleteEvent
			genericEvent event.GenericEvent
		)

		BeforeEach(func() {
			shoot = &gardencorev1beta1.Shoot{
				ObjectMeta: metav1.ObjectMeta{},
			}

			predicate = IsDeleting()

			createEvent = event.CreateEvent{
				Object: shoot,
			}
			updateEvent = event.UpdateEvent{
				ObjectOld: shoot,
				ObjectNew: shoot,
			}
			deleteEvent = event.DeleteEvent{
				Object: shoot,
			}
			genericEvent = event.GenericEvent{
				Object: shoot,
			}
		})

		Context("shoot doesn't have a deletion timestamp", func() {
			It("should be false", func() {
				gomega.Expect(predicate.Create(createEvent)).To(gomega.BeFalse())
				gomega.Expect(predicate.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(predicate.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(predicate.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})

		Context("shoot has a deletion timestamp", func() {
			time := metav1.Now()

			BeforeEach(func() {
				shoot.ObjectMeta.DeletionTimestamp = &time
			})

			It("should be true", func() {
				gomega.Expect(predicate.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(predicate.Update(updateEvent)).To(gomega.BeTrue())
				gomega.Expect(predicate.Delete(deleteEvent)).To(gomega.BeTrue())
				gomega.Expect(predicate.Generic(genericEvent)).To(gomega.BeTrue())
			})
		})
	})

	Describe("#HasName", func() {
		var (
			shoot        *gardencorev1beta1.Shoot
			predicate    predicate.Predicate
			createEvent  event.CreateEvent
			updateEvent  event.UpdateEvent
			deleteEvent  event.DeleteEvent
			genericEvent event.GenericEvent
		)

		BeforeEach(func() {
			shoot = &gardencorev1beta1.Shoot{
				ObjectMeta: metav1.ObjectMeta{Name: "foobar"},
			}

			predicate = HasName(shoot.Name)

			createEvent = event.CreateEvent{
				Object: shoot,
			}
			updateEvent = event.UpdateEvent{
				ObjectOld: shoot,
				ObjectNew: shoot,
			}
			deleteEvent = event.DeleteEvent{
				Object: shoot,
			}
			genericEvent = event.GenericEvent{
				Object: shoot,
			}
		})

		Context("shoot has the requested name", func() {
			It("should be true", func() {
				gomega.Expect(predicate.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(predicate.Update(updateEvent)).To(gomega.BeTrue())
				gomega.Expect(predicate.Delete(deleteEvent)).To(gomega.BeTrue())
				gomega.Expect(predicate.Generic(genericEvent)).To(gomega.BeTrue())
			})
		})

		Context("shoot does not have the requested name", func() {
			BeforeEach(func() {
				shoot.Name = "something-else"
			})

			It("should be false", func() {
				gomega.Expect(predicate.Create(createEvent)).To(gomega.BeFalse())
				gomega.Expect(predicate.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(predicate.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(predicate.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})
	})

	DescribeTable("#ForEventTypes",
		func(events []EventType, createMatcher, updateMatcher, deleteMatcher, genericMatcher gomegatypes.GomegaMatcher) {
			p := ForEventTypes(events...)

			gomega.Expect(p.Create(event.CreateEvent{})).To(createMatcher)
			gomega.Expect(p.Update(event.UpdateEvent{})).To(updateMatcher)
			gomega.Expect(p.Delete(event.DeleteEvent{})).To(deleteMatcher)
			gomega.Expect(p.Generic(event.GenericEvent{})).To(genericMatcher)
		},

		Entry("none", nil, gomega.BeFalse(), gomega.BeFalse(), gomega.BeFalse(), gomega.BeFalse()),
		Entry("create", []EventType{Create}, gomega.BeTrue(), gomega.BeFalse(), gomega.BeFalse(), gomega.BeFalse()),
		Entry("update", []EventType{Update}, gomega.BeFalse(), gomega.BeTrue(), gomega.BeFalse(), gomega.BeFalse()),
		Entry("delete", []EventType{Delete}, gomega.BeFalse(), gomega.BeFalse(), gomega.BeTrue(), gomega.BeFalse()),
		Entry("generic", []EventType{Generic}, gomega.BeFalse(), gomega.BeFalse(), gomega.BeFalse(), gomega.BeTrue()),
		Entry("create, update", []EventType{Create, Update}, gomega.BeTrue(), gomega.BeTrue(), gomega.BeFalse(), gomega.BeFalse()),
		Entry("create, delete", []EventType{Create, Delete}, gomega.BeTrue(), gomega.BeFalse(), gomega.BeTrue(), gomega.BeFalse()),
		Entry("create, generic", []EventType{Create, Generic}, gomega.BeTrue(), gomega.BeFalse(), gomega.BeFalse(), gomega.BeTrue()),
		Entry("update, delete", []EventType{Update, Delete}, gomega.BeFalse(), gomega.BeTrue(), gomega.BeTrue(), gomega.BeFalse()),
		Entry("update, generic", []EventType{Update, Generic}, gomega.BeFalse(), gomega.BeTrue(), gomega.BeFalse(), gomega.BeTrue()),
		Entry("delete, generic", []EventType{Delete, Generic}, gomega.BeFalse(), gomega.BeFalse(), gomega.BeTrue(), gomega.BeTrue()),
		Entry("create, update, delete", []EventType{Create, Update, Delete}, gomega.BeTrue(), gomega.BeTrue(), gomega.BeTrue(), gomega.BeFalse()),
		Entry("create, update, generic", []EventType{Create, Update, Generic}, gomega.BeTrue(), gomega.BeTrue(), gomega.BeFalse(), gomega.BeTrue()),
		Entry("create, delete, generic", []EventType{Create, Delete, Generic}, gomega.BeTrue(), gomega.BeFalse(), gomega.BeTrue(), gomega.BeTrue()),
		Entry("update, delete, generic", []EventType{Update, Delete, Generic}, gomega.BeFalse(), gomega.BeTrue(), gomega.BeTrue(), gomega.BeTrue()),
		Entry("create, update, delete, generic", []EventType{Create, Update, Delete, Generic}, gomega.BeTrue(), gomega.BeTrue(), gomega.BeTrue(), gomega.BeTrue()),
	)

	Describe("#RelevantConditionsChanged", func() {
		var (
			p                 predicate.Predicate
			shoot             *gardencorev1beta1.Shoot
			conditionsToCheck = []gardencorev1beta1.ConditionType{"Foo", "Bar"}
		)

		BeforeEach(func() {
			shoot = &gardencorev1beta1.Shoot{}
			p = RelevantConditionsChanged(
				func(obj client.Object) []gardencorev1beta1.Condition {
					return obj.(*gardencorev1beta1.Shoot).Status.Conditions
				},
				conditionsToCheck...,
			)
		})

		Describe("#Create", func() {
			It("should return true", func() {
				gomega.Expect(p.Create(event.CreateEvent{})).To(gomega.BeTrue())
			})
		})

		Describe("#Update", func() {
			It("should return false because there is no relevant change", func() {
				gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: shoot, ObjectOld: shoot})).To(gomega.BeFalse())
			})

			tests := func(conditionType gardencorev1beta1.ConditionType) {
				It("should return true because condition was added", func() {
					oldShoot := shoot.DeepCopy()
					shoot.Status.Conditions = []gardencorev1beta1.Condition{{Type: conditionType}}
					gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: shoot, ObjectOld: oldShoot})).To(gomega.BeTrue())
				})

				It("should return true because condition was removed", func() {
					shoot.Status.Conditions = []gardencorev1beta1.Condition{{Type: conditionType}}
					oldShoot := shoot.DeepCopy()
					shoot.Status.Conditions = nil
					gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: shoot, ObjectOld: oldShoot})).To(gomega.BeTrue())
				})

				It("should return true because condition status was changed", func() {
					shoot.Status.Conditions = []gardencorev1beta1.Condition{{Type: conditionType}}
					oldShoot := shoot.DeepCopy()
					shoot.Status.Conditions[0].Status = gardencorev1beta1.ConditionTrue
					gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: shoot, ObjectOld: oldShoot})).To(gomega.BeTrue())
				})

				It("should return true because condition reason was changed", func() {
					shoot.Status.Conditions = []gardencorev1beta1.Condition{{Type: conditionType}}
					oldShoot := shoot.DeepCopy()
					shoot.Status.Conditions[0].Reason = "reason"
					gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: shoot, ObjectOld: oldShoot})).To(gomega.BeTrue())
				})

				It("should return true because condition message was changed", func() {
					shoot.Status.Conditions = []gardencorev1beta1.Condition{{Type: conditionType}}
					oldShoot := shoot.DeepCopy()
					shoot.Status.Conditions[0].Message = "message"
					gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: shoot, ObjectOld: oldShoot})).To(gomega.BeTrue())
				})
			}

			Context("first condition", func() {
				tests(conditionsToCheck[0])
			})

			Context("second condition", func() {
				tests(conditionsToCheck[1])
			})
		})

		Describe("#Delete", func() {
			It("should return true", func() {
				gomega.Expect(p.Delete(event.DeleteEvent{})).To(gomega.BeTrue())
			})
		})

		Describe("#Generic", func() {
			It("should return true", func() {
				gomega.Expect(p.Generic(event.GenericEvent{})).To(gomega.BeTrue())
			})
		})
	})

	Describe("#ManagedResourceConditionsChanged", func() {
		var (
			p               predicate.Predicate
			managedResource *resourcesv1alpha1.ManagedResource
		)

		BeforeEach(func() {
			managedResource = &resourcesv1alpha1.ManagedResource{}
			p = ManagedResourceConditionsChanged()
		})

		Describe("#Create", func() {
			It("should return true", func() {
				gomega.Expect(p.Create(event.CreateEvent{})).To(gomega.BeTrue())
			})
		})

		Describe("#Update", func() {
			It("should return false because there is no relevant change", func() {
				gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: managedResource, ObjectOld: managedResource})).To(gomega.BeFalse())
			})

			tests := func(conditionType gardencorev1beta1.ConditionType) {
				It("should return true because condition was added", func() {
					oldShoot := managedResource.DeepCopy()
					managedResource.Status.Conditions = []gardencorev1beta1.Condition{{Type: conditionType}}
					gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: managedResource, ObjectOld: oldShoot})).To(gomega.BeTrue())
				})

				It("should return true because condition was removed", func() {
					managedResource.Status.Conditions = []gardencorev1beta1.Condition{{Type: conditionType}}
					oldShoot := managedResource.DeepCopy()
					managedResource.Status.Conditions = nil
					gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: managedResource, ObjectOld: oldShoot})).To(gomega.BeTrue())
				})

				It("should return true because condition status was changed", func() {
					managedResource.Status.Conditions = []gardencorev1beta1.Condition{{Type: conditionType}}
					oldShoot := managedResource.DeepCopy()
					managedResource.Status.Conditions[0].Status = gardencorev1beta1.ConditionTrue
					gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: managedResource, ObjectOld: oldShoot})).To(gomega.BeTrue())
				})

				It("should return true because condition reason was changed", func() {
					managedResource.Status.Conditions = []gardencorev1beta1.Condition{{Type: conditionType}}
					oldShoot := managedResource.DeepCopy()
					managedResource.Status.Conditions[0].Reason = "reason"
					gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: managedResource, ObjectOld: oldShoot})).To(gomega.BeTrue())
				})

				It("should return true because condition message was changed", func() {
					managedResource.Status.Conditions = []gardencorev1beta1.Condition{{Type: conditionType}}
					oldShoot := managedResource.DeepCopy()
					managedResource.Status.Conditions[0].Message = "message"
					gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: managedResource, ObjectOld: oldShoot})).To(gomega.BeTrue())
				})
			}

			Context("ResourcesApplied condition condition", func() {
				tests(resourcesv1alpha1.ResourcesApplied)
			})

			Context("ResourcesHealthy condition condition", func() {
				tests(resourcesv1alpha1.ResourcesHealthy)
			})

			Context("ResourcesProgressing condition condition", func() {
				tests(resourcesv1alpha1.ResourcesProgressing)
			})
		})

		Describe("#Delete", func() {
			It("should return true", func() {
				gomega.Expect(p.Delete(event.DeleteEvent{})).To(gomega.BeTrue())
			})
		})

		Describe("#Generic", func() {
			It("should return true", func() {
				gomega.Expect(p.Generic(event.GenericEvent{})).To(gomega.BeTrue())
			})
		})
	})

	Describe("#ExtensionStatusChanged", func() {
		var (
			p                     predicate.Predicate
			extensionBackupBucket *extensionsv1alpha1.BackupBucket
			bucketName            = "bucket"
			extensionBackupEntry  *extensionsv1alpha1.BackupEntry
			entryName             = "entry"
		)

		BeforeEach(func() {
			extensionBackupBucket = &extensionsv1alpha1.BackupBucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: bucketName,
				},
			}
			extensionBackupEntry = &extensionsv1alpha1.BackupEntry{
				ObjectMeta: metav1.ObjectMeta{
					Name: entryName,
				},
			}
			p = ExtensionStatusChanged()
		})

		It("should return false for all events because the extension backupbucket has operation annotation reconcile", func() {
			metav1.SetMetaDataAnnotation(&extensionBackupBucket.ObjectMeta, "gardener.cloud/operation", "reconcile")

			gomega.Expect(p.Create(event.CreateEvent{Object: extensionBackupBucket})).To(gomega.BeFalse())
			gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: extensionBackupBucket})).To(gomega.BeFalse())
			gomega.Expect(p.Delete(event.DeleteEvent{Object: extensionBackupBucket})).To(gomega.BeFalse())
			gomega.Expect(p.Generic(event.GenericEvent{Object: extensionBackupBucket})).To(gomega.BeFalse())
		})

		It("should not return false for create events just because the extension backupEntry has operation annotation restore or migrate", func() {
			extensionBackupEntry.Status.LastOperation = &gardencorev1beta1.LastOperation{State: gardencorev1beta1.LastOperationStateFailed}
			metav1.SetMetaDataAnnotation(&extensionBackupEntry.ObjectMeta, "gardener.cloud/operation", "migrate")

			gomega.Expect(p.Create(event.CreateEvent{Object: extensionBackupEntry})).To(gomega.BeTrue())

			metav1.SetMetaDataAnnotation(&extensionBackupEntry.ObjectMeta, "gardener.cloud/operation", "restore")

			gomega.Expect(p.Create(event.CreateEvent{Object: extensionBackupEntry})).To(gomega.BeTrue())
		})

		It("should return false for update because the extension backupEntry has operation annotation restore but the old backupEntry doesn't have it", func() {
			oldExtensionBackupEntry := extensionBackupEntry.DeepCopy()
			metav1.SetMetaDataAnnotation(&extensionBackupEntry.ObjectMeta, "gardener.cloud/operation", "restore")

			gomega.Expect(p.Update(event.UpdateEvent{ObjectOld: oldExtensionBackupEntry, ObjectNew: extensionBackupEntry})).To(gomega.BeFalse())
		})

		It("should not return false for update because of the operation annotation restore or migrate when the old backupEntry also have it", func() {
			metav1.SetMetaDataAnnotation(&extensionBackupEntry.ObjectMeta, "gardener.cloud/operation", "restore")
			oldExtensionBackupEntry := extensionBackupEntry.DeepCopy()
			extensionBackupEntry.Status.LastOperation = &gardencorev1beta1.LastOperation{State: gardencorev1beta1.LastOperationStateSucceeded}

			gomega.Expect(p.Update(event.UpdateEvent{ObjectOld: oldExtensionBackupEntry, ObjectNew: extensionBackupEntry})).To(gomega.BeTrue())

			metav1.SetMetaDataAnnotation(&extensionBackupEntry.ObjectMeta, "gardener.cloud/operation", "migrate")
			metav1.SetMetaDataAnnotation(&oldExtensionBackupEntry.ObjectMeta, "gardener.cloud/operation", "migrate")

			gomega.Expect(p.Update(event.UpdateEvent{ObjectOld: oldExtensionBackupEntry, ObjectNew: extensionBackupEntry})).To(gomega.BeTrue())
		})

		It("should return false for create and update because the extension backupbucket status has no lastOperation present", func() {
			extensionBackupBucket.Status.LastOperation = nil
			newExtensionBackupBucket := extensionBackupBucket.DeepCopy()

			gomega.Expect(p.Create(event.CreateEvent{Object: extensionBackupBucket})).To(gomega.BeFalse())
			gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: newExtensionBackupBucket, ObjectOld: extensionBackupBucket})).To(gomega.BeFalse())
			gomega.Expect(p.Delete(event.DeleteEvent{Object: extensionBackupBucket})).To(gomega.BeFalse())
			gomega.Expect(p.Generic(event.GenericEvent{Object: extensionBackupBucket})).To(gomega.BeFalse())
		})

		It("should return true for create events because the extension backupbucket status lastOperation state is Failed", func() {
			extensionBackupBucket.Status.LastOperation = &gardencorev1beta1.LastOperation{State: gardencorev1beta1.LastOperationStateFailed}

			gomega.Expect(p.Create(event.CreateEvent{Object: extensionBackupBucket})).To(gomega.BeTrue())
		})

		It("should return false for create events because the extension backupbucket status lastOperation state is not Failed", func() {
			extensionBackupBucket.Status.LastOperation = &gardencorev1beta1.LastOperation{State: gardencorev1beta1.LastOperationStateSucceeded}

			gomega.Expect(p.Create(event.CreateEvent{Object: extensionBackupBucket})).To(gomega.BeFalse())
		})

		It("should return true for  update events because the extension backupbucket status lastOperation state is Succeeded or Error or Failed and the old state is Processing", func() {
			extensionBackupBucket.Status.LastOperation = &gardencorev1beta1.LastOperation{State: gardencorev1beta1.LastOperationStateProcessing}
			newExtensionBackupBucket := extensionBackupBucket.DeepCopy()

			newExtensionBackupBucket.Status.LastOperation = &gardencorev1beta1.LastOperation{State: gardencorev1beta1.LastOperationStateSucceeded}
			gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: newExtensionBackupBucket, ObjectOld: extensionBackupBucket})).To(gomega.BeTrue())

			newExtensionBackupBucket.Status.LastOperation = &gardencorev1beta1.LastOperation{State: gardencorev1beta1.LastOperationStateError}
			gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: newExtensionBackupBucket, ObjectOld: extensionBackupBucket})).To(gomega.BeTrue())

			newExtensionBackupBucket.Status.LastOperation = &gardencorev1beta1.LastOperation{State: gardencorev1beta1.LastOperationStateFailed}
			gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: newExtensionBackupBucket, ObjectOld: extensionBackupBucket})).To(gomega.BeTrue())
		})

		It("should return true for update events because the extension backupbucket status lastOperation state is Succeeded or Error or Failed and the old state is nil", func() {
			extensionBackupBucket.Status.LastOperation = nil
			newExtensionBackupBucket := extensionBackupBucket.DeepCopy()

			newExtensionBackupBucket.Status.LastOperation = &gardencorev1beta1.LastOperation{State: gardencorev1beta1.LastOperationStateSucceeded}
			gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: newExtensionBackupBucket, ObjectOld: extensionBackupBucket})).To(gomega.BeTrue())

			newExtensionBackupBucket.Status.LastOperation = &gardencorev1beta1.LastOperation{State: gardencorev1beta1.LastOperationStateError}
			gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: newExtensionBackupBucket, ObjectOld: extensionBackupBucket})).To(gomega.BeTrue())

			newExtensionBackupBucket.Status.LastOperation = &gardencorev1beta1.LastOperation{State: gardencorev1beta1.LastOperationStateFailed}
			gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: newExtensionBackupBucket, ObjectOld: extensionBackupBucket})).To(gomega.BeTrue())
		})

		It("should return false for update events because the extension backupbucket status lastOperation has changed from Succeeded or Error to Processing", func() {
			extensionBackupBucket.Status.LastOperation = &gardencorev1beta1.LastOperation{State: gardencorev1beta1.LastOperationStateSucceeded}
			newExtensionBackupBucket := extensionBackupBucket.DeepCopy()
			newExtensionBackupBucket.Status.LastOperation = &gardencorev1beta1.LastOperation{State: gardencorev1beta1.LastOperationStateProcessing}

			gomega.Expect(p.Create(event.CreateEvent{Object: newExtensionBackupBucket})).To(gomega.BeFalse())
			gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: newExtensionBackupBucket, ObjectOld: extensionBackupBucket})).To(gomega.BeFalse())
			gomega.Expect(p.Delete(event.DeleteEvent{Object: newExtensionBackupBucket})).To(gomega.BeFalse())
			gomega.Expect(p.Generic(event.GenericEvent{Object: newExtensionBackupBucket})).To(gomega.BeFalse())
		})

		It("should return false for update events because the extension backupbucket status lastOperation is Succeeded but it's same as old Object", func() {
			extensionBackupBucket.Status.LastOperation = &gardencorev1beta1.LastOperation{State: gardencorev1beta1.LastOperationStateSucceeded}
			newExtensionBackupBucket := extensionBackupBucket.DeepCopy()

			gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: newExtensionBackupBucket, ObjectOld: extensionBackupBucket})).To(gomega.BeFalse())
		})
	})

	Describe("#IsBeingMigratedPredicate", func() {
		var (
			ctx        = context.TODO()
			fakeClient client.Client
			p          predicate.Predicate

			obj      *gardencorev1beta1.BackupEntry
			seedName = "seed"
		)

		BeforeEach(func() {
			fakeClient = fakeclient.NewClientBuilder().WithScheme(kubernetes.GardenScheme).Build()

			p = IsBeingMigratedPredicate(fakeClient, seedName, nil)
			gomega.Expect(inject.StopChannelInto(ctx.Done(), p)).To(gomega.BeTrue())

			DeferCleanup(test.WithVar(&IsObjectBeingMigrated, func(_ context.Context, _ client.Reader, obj client.Object, _ string, _ func(client.Object) (*string, *string)) bool {
				return false
			}))

			obj = &gardencorev1beta1.BackupEntry{}
		})

		It("should call the IsObjectBeingMigrated helper functions", func() {
			gomega.Expect(p.Create(event.CreateEvent{Object: obj})).To(gomega.BeFalse())
			gomega.Expect(p.Update(event.UpdateEvent{ObjectNew: obj})).To(gomega.BeFalse())
			gomega.Expect(p.Delete(event.DeleteEvent{Object: obj})).To(gomega.BeFalse())
			gomega.Expect(p.Generic(event.GenericEvent{Object: obj})).To(gomega.BeFalse())
		})
	})

	Describe("#SeedNamePredicate", func() {
		var (
			p        predicate.Predicate
			seedName = "seed"
		)

		DescribeTable("filter by seedName",
			func(specSeedName, statusSeedName *string, match gomegatypes.GomegaMatcher) {
				p = SeedNamePredicate(seedName, func(client.Object) (*string, *string) {
					return specSeedName, statusSeedName
				})

				gomega.Expect(p.Create(event.CreateEvent{})).To(match)
				gomega.Expect(p.Update(event.UpdateEvent{})).To(match)
				gomega.Expect(p.Delete(event.DeleteEvent{})).To(match)
				gomega.Expect(p.Generic(event.GenericEvent{})).To(match)
			},

			Entry("spec.seedName and status.seedName are nil", nil, nil, gomega.BeFalse()),
			Entry("spec.seedName does not match and status.seedName is nil", pointer.String("otherSeed"), nil, gomega.BeFalse()),
			Entry("spec.seedName and status.seedName do not match", pointer.String("otherSeed"), pointer.String("otherSeed"), gomega.BeFalse()),
			Entry("spec.seedName is nil but status.seedName matches", nil, pointer.String("seed"), gomega.BeFalse()),
			Entry("spec.seedName matches and status.seedName is nil", pointer.String("seed"), nil, gomega.BeTrue()),
			Entry("spec.seedName does not match but status.seedName matches", pointer.String("otherSeed"), pointer.String("seed"), gomega.BeTrue()),
		)
	})
})
