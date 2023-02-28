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

package exposureclass

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
)

var _ = Describe("Controller", func() {
	const finalizerName = "gardener"

	var (
		ctx        = context.TODO()
		fakeClient client.Client
		reconciler reconcile.Reconciler

		exposureClassName string
		exposureClass     *gardencorev1beta1.ExposureClass
		shoot             *gardencorev1beta1.Shoot
	)

	BeforeEach(func() {
		fakeClient = fakeclient.NewClientBuilder().WithScheme(kubernetes.GardenScheme).Build()

		exposureClassName = "test-exposureclass"
		reconciler = &Reconciler{Client: fakeClient, Recorder: &record.FakeRecorder{}}
		exposureClass = &gardencorev1beta1.ExposureClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: exposureClassName,
			},
		}

		shoot = &gardencorev1beta1.Shoot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-shoot",
				Namespace: "test-namespace",
			},
			Spec: gardencorev1beta1.ShootSpec{
				ExposureClassName: &exposureClassName,
			},
		}
	})

	It("should return nil because object is not found", func() {
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(exposureClass), &gardencorev1beta1.ExposureClass{})).To(BeNotFoundError())

		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: exposureClassName}})
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(err).NotTo(HaveOccurred())
	})

	Context("when deletion timestamp is not set", func() {
		BeforeEach(func() {
			Expect(fakeClient.Create(ctx, exposureClass)).To(Succeed())
		})

		It("should ensure the finalizer", func() {
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: exposureClassName}})
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(exposureClass), exposureClass)).To(Succeed())
			Expect(result).To(Equal(reconcile.Result{}))
			Expect(err).NotTo(HaveOccurred())
			Expect(exposureClass.GetFinalizers()).Should(ConsistOf(finalizerName))
		})
	})

	Context("when deletion timestamp is set", func() {
		BeforeEach(func() {
			now := metav1.Now()
			exposureClass.DeletionTimestamp = &now
			exposureClass.Finalizers = []string{finalizerName}

			Expect(fakeClient.Create(ctx, exposureClass)).To(Succeed())
		})

		It("should do nothing because finalizer is not present", func() {
			Expect(fakeClient.Create(ctx, shoot)).To(Succeed())
			patch := client.MergeFrom(exposureClass.DeepCopy())
			exposureClass.Finalizers = []string{"test-finalizer"}
			Expect(fakeClient.Patch(ctx, exposureClass, patch)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: exposureClassName}})
			Expect(result).To(Equal(reconcile.Result{}))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error because Shoot referencing ExposureClass exists", func() {
			Expect(fakeClient.Create(ctx, shoot)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: exposureClassName}})
			Expect(result).To(Equal(reconcile.Result{}))
			Expect(err).To(MatchError(ContainSubstring("Cannot delete ExposureClasss")))
		})

		It("should remove the finalizer because no Shoot is referencing the ExposureClass", func() {
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: exposureClassName}})
			Expect(result).To(Equal(reconcile.Result{}))
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(exposureClass), exposureClass)).To(BeNotFoundError())
		})
	})
})
