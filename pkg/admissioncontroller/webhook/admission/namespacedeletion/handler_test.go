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

package namespacedeletion_test

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	. "github.com/gardener/gardener/pkg/admissioncontroller/webhook/admission/namespacedeletion"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/logger"
	mockcache "github.com/gardener/gardener/pkg/mock/controller-runtime/cache"
	mockclient "github.com/gardener/gardener/pkg/mock/controller-runtime/client"
	kubernetesutils "github.com/gardener/gardener/pkg/utils/kubernetes"
)

var _ = Describe("handler", func() {
	var (
		ctx = context.TODO()
		log logr.Logger

		handler *Handler

		ctrl       *gomock.Controller
		mockCache  *mockcache.MockCache
		mockReader *mockclient.MockReader

		namespaceName     = "foo"
		projectName       = "bar"
		namespace         *corev1.Namespace
		shootMetadataList *metav1.PartialObjectMetadataList
	)

	BeforeEach(func() {
		log = logger.MustNewZapLogger(logger.DebugLevel, logger.FormatJSON, logzap.WriteTo(GinkgoWriter))

		ctrl = gomock.NewController(GinkgoT())
		mockCache = mockcache.NewMockCache(ctrl)
		mockReader = mockclient.NewMockReader(ctrl)

		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   namespaceName,
				Labels: map[string]string{"project.gardener.cloud/name": projectName},
			},
		}

		shootMetadataList = &metav1.PartialObjectMetadataList{}
		shootMetadataList.SetGroupVersionKind(gardencorev1beta1.SchemeGroupVersion.WithKind("ShootList"))

		handler = &Handler{Logger: log, APIReader: mockReader, Client: mockCache}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	test := func(op admissionv1.Operation, matcher gomegatypes.GomegaMatcher) {
		ctx = admission.NewContextWithRequest(ctx, admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{Name: namespaceName}})
		Expect(handler.ValidateDelete(ctx, nil)).To(matcher)
	}

	It("should pass because no projects available", func() {
		mockCache.EXPECT().Get(gomock.Any(), kubernetesutils.Key(namespaceName), gomock.AssignableToTypeOf(&corev1.Namespace{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj *corev1.Namespace, _ ...client.GetOption) error {
			namespace.DeepCopyInto(obj)
			return nil
		})
		mockCache.EXPECT().Get(gomock.Any(), kubernetesutils.Key(projectName), gomock.AssignableToTypeOf(&gardencorev1beta1.Project{})).Return(apierrors.NewNotFound(schema.GroupResource{}, ""))

		test(admissionv1.Delete, Succeed())
	})

	It("should pass because namespace is not project related", func() {
		mockCache.EXPECT().Get(gomock.Any(), kubernetesutils.Key(namespaceName), gomock.AssignableToTypeOf(&corev1.Namespace{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj *corev1.Namespace, _ ...client.GetOption) error {
			(&corev1.Namespace{}).DeepCopyInto(obj)
			return nil
		})

		test(admissionv1.Delete, Succeed())
	})

	It("should fail because get namespace fails", func() {
		mockCache.EXPECT().Get(gomock.Any(), kubernetesutils.Key(namespaceName), gomock.AssignableToTypeOf(&corev1.Namespace{})).Return(fmt.Errorf("fake"))

		test(admissionv1.Delete, MatchError(ContainSubstring("fake")))
	})

	It("should fail because getting the projects fails", func() {
		mockCache.EXPECT().Get(gomock.Any(), kubernetesutils.Key(namespaceName), gomock.AssignableToTypeOf(&corev1.Namespace{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj *corev1.Namespace, _ ...client.GetOption) error {
			namespace.DeepCopyInto(obj)
			return nil
		})
		mockCache.EXPECT().Get(gomock.Any(), kubernetesutils.Key(projectName), gomock.AssignableToTypeOf(&gardencorev1beta1.Project{})).Return(fmt.Errorf("fake"))

		test(admissionv1.Delete, MatchError(ContainSubstring("fake")))
	})

	It("should pass because namespace is already gone", func() {
		mockCache.EXPECT().Get(gomock.Any(), kubernetesutils.Key(namespaceName), gomock.AssignableToTypeOf(&corev1.Namespace{})).Return(apierrors.NewNotFound(schema.GroupResource{}, ""))

		test(admissionv1.Delete, Succeed())
	})

	Context("related project available", func() {
		var relatedProject gardencorev1beta1.Project

		It("should pass because namespace is already marked for deletion", func() {
			mockCache.EXPECT().Get(gomock.Any(), kubernetesutils.Key(namespaceName), gomock.AssignableToTypeOf(&corev1.Namespace{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj *corev1.Namespace, _ ...client.GetOption) error {
				now := metav1.Now()
				namespace.SetDeletionTimestamp(&now)
				namespace.DeepCopyInto(obj)
				return nil
			})
			mockCache.EXPECT().Get(gomock.Any(), kubernetesutils.Key(projectName), gomock.AssignableToTypeOf(&gardencorev1beta1.Project{}))

			test(admissionv1.Delete, Succeed())
		})

		It("should forbid namespace deletion because project is not marked for deletion", func() {
			mockCache.EXPECT().Get(gomock.Any(), kubernetesutils.Key(namespaceName), gomock.AssignableToTypeOf(&corev1.Namespace{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj *corev1.Namespace, _ ...client.GetOption) error {
				namespace.DeepCopyInto(obj)
				return nil
			})
			mockCache.EXPECT().Get(gomock.Any(), kubernetesutils.Key(projectName), gomock.AssignableToTypeOf(&gardencorev1beta1.Project{}))

			test(admissionv1.Delete, MatchError(ContainSubstring("direct deletion of namespace")))
		})

		Context("related project marked for deletion ", func() {
			BeforeEach(func() {
				now := metav1.Now()
				relatedProject.SetDeletionTimestamp(&now)

				mockCache.EXPECT().Get(gomock.Any(), kubernetesutils.Key(namespaceName), gomock.AssignableToTypeOf(&corev1.Namespace{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj *corev1.Namespace, _ ...client.GetOption) error {
					namespace.DeepCopyInto(obj)
					return nil
				})
				mockCache.EXPECT().Get(gomock.Any(), kubernetesutils.Key(projectName), gomock.AssignableToTypeOf(&gardencorev1beta1.Project{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj *gardencorev1beta1.Project, _ ...client.GetOption) error {
					relatedProject.DeepCopyInto(obj)
					return nil
				})
			})

			It("should fail because listing shoots fails", func() {
				mockReader.EXPECT().List(gomock.Any(), shootMetadataList, client.InNamespace(namespaceName), client.Limit(1)).DoAndReturn(func(_ context.Context, list *metav1.PartialObjectMetadataList, _ ...client.ListOption) error {
					return fmt.Errorf("fake")
				})

				test(admissionv1.Delete, MatchError(ContainSubstring("fake")))
			})

			It("should pass because namespace is does not contain any shoots", func() {
				mockReader.EXPECT().List(gomock.Any(), shootMetadataList, client.InNamespace(namespaceName), client.Limit(1)).DoAndReturn(func(_ context.Context, list *metav1.PartialObjectMetadataList, _ ...client.ListOption) error {
					return nil
				})

				test(admissionv1.Delete, Succeed())
			})

			It("should forbid namespace deletion because it still contain shoots", func() {
				mockReader.EXPECT().List(gomock.Any(), shootMetadataList, client.InNamespace(namespaceName), client.Limit(1)).DoAndReturn(func(_ context.Context, list *metav1.PartialObjectMetadataList, _ ...client.ListOption) error {
					list.Items = []metav1.PartialObjectMetadata{{ObjectMeta: metav1.ObjectMeta{Name: "shoot1", Namespace: namespaceName}}}
					return nil
				})

				test(admissionv1.Delete, MatchError(ContainSubstring("still contains Shoots")))
			})
		})
	})
})
