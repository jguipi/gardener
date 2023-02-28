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

package internaldomainsecret_test

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	. "github.com/gardener/gardener/pkg/admissioncontroller/webhook/admission/internaldomainsecret"
	gardencore "github.com/gardener/gardener/pkg/apis/core"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/logger"
	mockclient "github.com/gardener/gardener/pkg/mock/controller-runtime/client"
	gardenerutils "github.com/gardener/gardener/pkg/utils/gardener"
)

var _ = Describe("handler", func() {
	var (
		ctrl       *gomock.Controller
		mockReader *mockclient.MockReader

		ctx     = context.TODO()
		fakeErr = fmt.Errorf("fake err")
		log     logr.Logger
		handler *Handler

		secret            *corev1.Secret
		shootMetadataList *metav1.PartialObjectMetadataList

		resourceName         = "foo"
		regularNamespaceName = "regular-namespace"
		gardenNamespaceName  = v1beta1constants.GardenNamespace
		seedName             string
		seedNamespace        string
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockReader = mockclient.NewMockReader(ctrl)

		ctx = admission.NewContextWithRequest(ctx, admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{Name: resourceName}})
		log = logger.MustNewZapLogger(logger.DebugLevel, logger.FormatJSON, logzap.WriteTo(GinkgoWriter))
		handler = &Handler{Logger: log, APIReader: mockReader}

		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: gardenNamespaceName,
				Annotations: map[string]string{
					"dns.gardener.cloud/provider": "foo",
					"dns.gardener.cloud/domain":   "bar",
				},
				Labels: map[string]string{
					"gardener.cloud/role": "internal-domain",
				},
			},
		}
		shootMetadataList = &metav1.PartialObjectMetadataList{}
		shootMetadataList.SetGroupVersionKind(gardencorev1beta1.SchemeGroupVersion.WithKind("ShootList"))

		seedName = "aws"
		seedNamespace = gardenerutils.ComputeGardenNamespace(seedName)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Context("ignored requests", func() {
		It("should only handle garden and seed namespaces", func() {
			secret.Namespace = regularNamespaceName
			Expect(handler.ValidateCreate(ctx, secret)).To(Succeed())
			Expect(handler.ValidateUpdate(ctx, secret, secret)).To(Succeed())
			Expect(handler.ValidateDelete(ctx, secret)).To(Succeed())
		})
	})

	Context("create", func() {
		It("should fail because the check for other internal domain secrets failed", func() {
			mockReader.EXPECT().List(
				gomock.Any(),
				gomock.AssignableToTypeOf(&metav1.PartialObjectMetadataList{}),
				client.InNamespace(gardenNamespaceName),
				client.MatchingLabels{v1beta1constants.GardenRole: v1beta1constants.GardenRoleInternalDomain},
				client.Limit(1),
			).Return(fakeErr)

			err := handler.ValidateCreate(ctx, secret)
			statusError, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue())
			Expect(statusError.Status().Code).To(Equal(int32(http.StatusInternalServerError)))
			Expect(statusError.Status().Message).To(ContainSubstring(fakeErr.Error()))
		})

		It("should fail because another internal domain secret exists in the garden namespace", func() {
			mockReader.EXPECT().List(
				gomock.Any(),
				gomock.AssignableToTypeOf(&metav1.PartialObjectMetadataList{}),
				client.InNamespace(gardenNamespaceName),
				client.MatchingLabels{v1beta1constants.GardenRole: v1beta1constants.GardenRoleInternalDomain},
				client.Limit(1),
			).DoAndReturn(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
				(&metav1.PartialObjectMetadataList{Items: []metav1.PartialObjectMetadata{{}}}).DeepCopyInto(list.(*metav1.PartialObjectMetadataList))
				return nil
			})

			Expect(handler.ValidateCreate(ctx, secret)).To(MatchError(ContainSubstring("there can be only one secret with the 'internal-domain' secret role")))
		})

		It("should fail because another internal domain secret exists in the same seed namespace", func() {
			mockReader.EXPECT().List(
				gomock.Any(),
				gomock.AssignableToTypeOf(&metav1.PartialObjectMetadataList{}),
				client.InNamespace(seedNamespace),
				client.MatchingLabels{v1beta1constants.GardenRole: v1beta1constants.GardenRoleInternalDomain},
				client.Limit(1),
			).DoAndReturn(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
				(&metav1.PartialObjectMetadataList{Items: []metav1.PartialObjectMetadata{{}}}).DeepCopyInto(list.(*metav1.PartialObjectMetadataList))
				return nil
			})

			secret.Namespace = seedNamespace
			Expect(handler.ValidateCreate(ctx, secret)).To(MatchError(ContainSubstring("there can be only one secret with the 'internal-domain' secret role")))
		})

		It("should fail because the secret misses domain info", func() {
			mockReader.EXPECT().List(
				gomock.Any(),
				gomock.AssignableToTypeOf(&metav1.PartialObjectMetadataList{}),
				client.InNamespace(gardenNamespaceName),
				client.MatchingLabels{v1beta1constants.GardenRole: v1beta1constants.GardenRoleInternalDomain},
				client.Limit(1),
			)

			secret.Annotations = nil
			Expect(handler.ValidateCreate(ctx, secret)).To(MatchError(ContainSubstring("domain secret has no annotations")))
		})

		It("should pass because no other internal domain secret exists", func() {
			mockReader.EXPECT().List(
				gomock.Any(),
				gomock.AssignableToTypeOf(&metav1.PartialObjectMetadataList{}),
				client.InNamespace(gardenNamespaceName),
				client.MatchingLabels{v1beta1constants.GardenRole: v1beta1constants.GardenRoleInternalDomain},
				client.Limit(1),
			)

			Expect(handler.ValidateCreate(ctx, secret)).To(Succeed())
		})
	})

	Context("update", func() {
		Context("when secret is newly labeled with gardener.cloud/role=internal-domain", func() {
			var oldSecret *corev1.Secret

			BeforeEach(func() {
				oldSecret = secret.DeepCopy()
				oldSecret.Labels = nil
			})

			It("should fail because the check for other internal domain secrets failed", func() {
				mockReader.EXPECT().List(
					gomock.Any(),
					gomock.AssignableToTypeOf(&metav1.PartialObjectMetadataList{}),
					client.InNamespace(gardenNamespaceName),
					client.MatchingLabels{v1beta1constants.GardenRole: v1beta1constants.GardenRoleInternalDomain},
					client.Limit(1),
				).Return(fakeErr)

				err := handler.ValidateUpdate(ctx, oldSecret, secret)
				statusError, ok := err.(*apierrors.StatusError)
				Expect(ok).To(BeTrue())
				Expect(statusError.Status().Code).To(Equal(int32(http.StatusInternalServerError)))
				Expect(statusError.Status().Message).To(ContainSubstring(fakeErr.Error()))
			})

			It("should fail because another internal domain secret exists in the garden namespace", func() {
				mockReader.EXPECT().List(
					gomock.Any(),
					gomock.AssignableToTypeOf(&metav1.PartialObjectMetadataList{}),
					client.InNamespace(gardenNamespaceName),
					client.MatchingLabels{v1beta1constants.GardenRole: v1beta1constants.GardenRoleInternalDomain},
					client.Limit(1),
				).DoAndReturn(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
					(&metav1.PartialObjectMetadataList{Items: []metav1.PartialObjectMetadata{{}}}).DeepCopyInto(list.(*metav1.PartialObjectMetadataList))
					return nil
				})

				Expect(handler.ValidateUpdate(ctx, oldSecret, secret)).To(MatchError(ContainSubstring("there can be only one secret with the 'internal-domain' secret role")))
			})
		})

		It("should fail because the old secret misses domain info", func() {
			oldSecret := secret.DeepCopy()
			oldSecret.Annotations = nil
			Expect(handler.ValidateUpdate(ctx, oldSecret, secret)).To(MatchError(ContainSubstring("domain secret has no annotations")))
		})

		It("should fail because the secret misses domain info", func() {
			oldSecret := secret.DeepCopy()
			secret.Annotations = nil
			Expect(handler.ValidateUpdate(ctx, oldSecret, secret)).To(MatchError(ContainSubstring("domain secret has no annotations")))
		})

		It("should forbid because the domain is changed but shoot listing failed", func() {
			mockReader.EXPECT().List(
				gomock.Any(),
				gomock.AssignableToTypeOf(&metav1.PartialObjectMetadataList{}),
				client.Limit(1),
			).Return(fakeErr)

			oldSecret := secret.DeepCopy()
			secret.Annotations["dns.gardener.cloud/domain"] = "foobar"

			err := handler.ValidateUpdate(ctx, oldSecret, secret)
			statusError, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue())
			Expect(statusError.Status().Code).To(Equal(int32(http.StatusInternalServerError)))
			Expect(statusError.Status().Message).To(ContainSubstring(fakeErr.Error()))
		})

		It("should forbid because the global domain is changed but shoots exist", func() {
			mockReader.EXPECT().List(
				gomock.Any(),
				gomock.AssignableToTypeOf(&metav1.PartialObjectMetadataList{}),
				client.Limit(1),
			).DoAndReturn(func(_ context.Context, list client.ObjectList, limitOne client.ListOption) error {
				(&metav1.PartialObjectMetadataList{Items: []metav1.PartialObjectMetadata{{}}}).DeepCopyInto(list.(*metav1.PartialObjectMetadataList))
				return nil
			})

			oldSecret := secret.DeepCopy()
			secret.Annotations["dns.gardener.cloud/domain"] = "foobar"
			Expect(handler.ValidateUpdate(ctx, oldSecret, secret)).To(MatchError(ContainSubstring("cannot change domain because there are still shoots left in the system")))
		})

		It("should forbid because the domain in seed namespace is changed but shoots using the seed exist", func() {
			mockReader.EXPECT().List(
				gomock.Any(),
				gomock.AssignableToTypeOf(&metav1.PartialObjectMetadataList{}),
				client.MatchingFields{gardencore.ShootSeedName: seedName},
				client.Limit(1),
			).DoAndReturn(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
				(&metav1.PartialObjectMetadataList{Items: []metav1.PartialObjectMetadata{{}}}).DeepCopyInto(list.(*metav1.PartialObjectMetadataList))
				return nil
			})

			secret.Namespace = seedNamespace
			oldSecret := secret.DeepCopy()
			secret.Annotations["dns.gardener.cloud/domain"] = "foobar"
			Expect(handler.ValidateUpdate(ctx, oldSecret, secret)).To(MatchError(ContainSubstring("cannot change domain because there are still shoots left in the system")))
		})

		It("should allow because the domain is changed but no shoots exist", func() {
			mockReader.EXPECT().List(
				gomock.Any(),
				gomock.AssignableToTypeOf(&metav1.PartialObjectMetadataList{}),
				client.Limit(1),
			)

			oldSecret := secret.DeepCopy()
			secret.Annotations["dns.gardener.cloud/domain"] = "foobar"
			Expect(handler.ValidateUpdate(ctx, oldSecret, secret)).To(Succeed())
		})

		It("should allow because the domain is not changed", func() {
			oldSecret := secret.DeepCopy()
			secret.Annotations["dns.gardener.cloud/provider"] = "foobar"
			Expect(handler.ValidateUpdate(ctx, oldSecret, secret)).To(Succeed())
		})
	})

	Context("delete", func() {
		It("should fail because the shoot listing fails", func() {
			mockReader.EXPECT().List(
				gomock.Any(),
				gomock.AssignableToTypeOf(&metav1.PartialObjectMetadataList{}),
				client.Limit(1),
			).Return(fakeErr)

			err := handler.ValidateDelete(ctx, secret)
			statusError, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue())
			Expect(statusError.Status().Code).To(Equal(int32(http.StatusInternalServerError)))
			Expect(statusError.Status().Message).To(ContainSubstring(fakeErr.Error()))
		})

		It("should fail because at least one shoot exists", func() {
			mockReader.EXPECT().List(
				gomock.Any(),
				gomock.AssignableToTypeOf(&metav1.PartialObjectMetadataList{}),
				client.Limit(1),
			).DoAndReturn(func(_ context.Context, list client.ObjectList, limitOne client.ListOption) error {
				(&metav1.PartialObjectMetadataList{Items: []metav1.PartialObjectMetadata{{}}}).DeepCopyInto(list.(*metav1.PartialObjectMetadataList))
				return nil
			})

			Expect(handler.ValidateDelete(ctx, secret)).To(MatchError(ContainSubstring("cannot delete internal domain secret because there are still shoots left in the system")))
		})

		It("should fail because at least one shoot on the seed exists", func() {
			mockReader.EXPECT().List(
				gomock.Any(),
				gomock.AssignableToTypeOf(&metav1.PartialObjectMetadataList{}),
				client.MatchingFields{gardencore.ShootSeedName: seedName},
				client.Limit(1),
			).DoAndReturn(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
				(&metav1.PartialObjectMetadataList{Items: []metav1.PartialObjectMetadata{{}}}).DeepCopyInto(list.(*metav1.PartialObjectMetadataList))
				return nil
			})

			secret.Namespace = seedNamespace
			Expect(handler.ValidateDelete(ctx, secret)).To(MatchError(ContainSubstring("cannot delete internal domain secret because there are still shoots left in the system")))
		})

		It("should pass because no shoots exist", func() {
			mockReader.EXPECT().List(
				gomock.Any(),
				gomock.AssignableToTypeOf(&metav1.PartialObjectMetadataList{}),
				client.Limit(1),
			)

			Expect(handler.ValidateDelete(ctx, secret)).To(Succeed())
		})
	})
})
