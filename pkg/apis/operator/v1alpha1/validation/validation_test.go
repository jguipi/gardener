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

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	operatorv1alpha1 "github.com/gardener/gardener/pkg/apis/operator/v1alpha1"
	. "github.com/gardener/gardener/pkg/apis/operator/v1alpha1/validation"
)

var _ = Describe("Validation Tests", func() {
	Describe("#ValidateGarden", func() {
		var garden *operatorv1alpha1.Garden

		BeforeEach(func() {
			garden = &operatorv1alpha1.Garden{
				ObjectMeta: metav1.ObjectMeta{
					Name: "garden",
				},
			}
		})

		Context("operation validation", func() {
			It("should do nothing if the operation annotation is not set", func() {
				Expect(ValidateGarden(garden)).To(BeEmpty())
			})

			DescribeTable("should not allow starting/completing rotation when garden has deletion timestamp",
				func(operation string) {
					garden.DeletionTimestamp = &metav1.Time{}
					metav1.SetMetaDataAnnotation(&garden.ObjectMeta, "gardener.cloud/operation", operation)

					Expect(ValidateGarden(garden)).To(ContainElement(PointTo(MatchFields(IgnoreExtras, Fields{
						"Type":  Equal(field.ErrorTypeForbidden),
						"Field": Equal("metadata.annotations[gardener.cloud/operation]"),
					}))))
				},

				Entry("start all rotations", "rotate-credentials-start"),
				Entry("complete all rotations", "rotate-credentials-complete"),
				Entry("start CA rotation", "rotate-ca-start"),
				Entry("complete CA rotation", "rotate-ca-complete"),
			)

			DescribeTable("starting rotation of all credentials",
				func(allowed bool, status operatorv1alpha1.GardenStatus) {
					metav1.SetMetaDataAnnotation(&garden.ObjectMeta, "gardener.cloud/operation", "rotate-credentials-start")
					garden.Status = status

					matcher := BeEmpty()
					if !allowed {
						matcher = ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
							"Type":  Equal(field.ErrorTypeForbidden),
							"Field": Equal("metadata.annotations[gardener.cloud/operation]"),
						})))
					}

					Expect(ValidateGarden(garden)).To(matcher)
				},

				Entry("ca rotation phase is preparing", false, operatorv1alpha1.GardenStatus{
					Credentials: &operatorv1alpha1.Credentials{
						Rotation: &operatorv1alpha1.CredentialsRotation{
							CertificateAuthorities: &gardencorev1beta1.CARotation{
								Phase: gardencorev1beta1.RotationPreparing,
							},
						},
					},
				}),
				Entry("ca rotation phase is prepared", false, operatorv1alpha1.GardenStatus{
					Credentials: &operatorv1alpha1.Credentials{
						Rotation: &operatorv1alpha1.CredentialsRotation{
							CertificateAuthorities: &gardencorev1beta1.CARotation{
								Phase: gardencorev1beta1.RotationPrepared,
							},
						},
					},
				}),
				Entry("ca rotation phase is completing", false, operatorv1alpha1.GardenStatus{
					Credentials: &operatorv1alpha1.Credentials{
						Rotation: &operatorv1alpha1.CredentialsRotation{
							CertificateAuthorities: &gardencorev1beta1.CARotation{
								Phase: gardencorev1beta1.RotationCompleting,
							},
						},
					},
				}),
				Entry("ca rotation phase is completed", true, operatorv1alpha1.GardenStatus{
					Credentials: &operatorv1alpha1.Credentials{
						Rotation: &operatorv1alpha1.CredentialsRotation{
							CertificateAuthorities: &gardencorev1beta1.CARotation{
								Phase: gardencorev1beta1.RotationCompleted,
							},
						},
					},
				}),
			)

			DescribeTable("completing rotation of all credentials",
				func(allowed bool, status operatorv1alpha1.GardenStatus) {
					metav1.SetMetaDataAnnotation(&garden.ObjectMeta, "gardener.cloud/operation", "rotate-credentials-complete")
					garden.Status = status

					matcher := BeEmpty()
					if !allowed {
						matcher = ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
							"Type":  Equal(field.ErrorTypeForbidden),
							"Field": Equal("metadata.annotations[gardener.cloud/operation]"),
						})))
					}

					Expect(ValidateGarden(garden)).To(matcher)
				},

				Entry("ca rotation phase is preparing", false, operatorv1alpha1.GardenStatus{
					Credentials: &operatorv1alpha1.Credentials{
						Rotation: &operatorv1alpha1.CredentialsRotation{
							CertificateAuthorities: &gardencorev1beta1.CARotation{
								Phase: gardencorev1beta1.RotationPreparing,
							},
						},
					},
				}),
				Entry("all rotation phases are prepared", true, operatorv1alpha1.GardenStatus{
					Credentials: &operatorv1alpha1.Credentials{
						Rotation: &operatorv1alpha1.CredentialsRotation{
							CertificateAuthorities: &gardencorev1beta1.CARotation{
								Phase: gardencorev1beta1.RotationPrepared,
							},
						},
					},
				}),
				Entry("ca rotation phase is completing", false, operatorv1alpha1.GardenStatus{
					Credentials: &operatorv1alpha1.Credentials{
						Rotation: &operatorv1alpha1.CredentialsRotation{
							CertificateAuthorities: &gardencorev1beta1.CARotation{
								Phase: gardencorev1beta1.RotationCompleting,
							},
						},
					},
				}),
				Entry("ca rotation phase is completed", false, operatorv1alpha1.GardenStatus{
					Credentials: &operatorv1alpha1.Credentials{
						Rotation: &operatorv1alpha1.CredentialsRotation{
							CertificateAuthorities: &gardencorev1beta1.CARotation{
								Phase: gardencorev1beta1.RotationCompleted,
							},
						},
					},
				}),
			)

			DescribeTable("starting CA rotation",
				func(allowed bool, status operatorv1alpha1.GardenStatus) {
					metav1.SetMetaDataAnnotation(&garden.ObjectMeta, "gardener.cloud/operation", "rotate-ca-start")
					garden.Status = status

					matcher := BeEmpty()
					if !allowed {
						matcher = ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
							"Type":  Equal(field.ErrorTypeForbidden),
							"Field": Equal("metadata.annotations[gardener.cloud/operation]"),
						})))
					}

					Expect(ValidateGarden(garden)).To(matcher)
				},

				Entry("ca rotation phase is preparing", false, operatorv1alpha1.GardenStatus{
					Credentials: &operatorv1alpha1.Credentials{
						Rotation: &operatorv1alpha1.CredentialsRotation{
							CertificateAuthorities: &gardencorev1beta1.CARotation{
								Phase: gardencorev1beta1.RotationPreparing,
							},
						},
					},
				}),
				Entry("ca rotation phase is prepared", false, operatorv1alpha1.GardenStatus{
					Credentials: &operatorv1alpha1.Credentials{
						Rotation: &operatorv1alpha1.CredentialsRotation{
							CertificateAuthorities: &gardencorev1beta1.CARotation{
								Phase: gardencorev1beta1.RotationPrepared,
							},
						},
					},
				}),
				Entry("ca rotation phase is completing", false, operatorv1alpha1.GardenStatus{
					Credentials: &operatorv1alpha1.Credentials{
						Rotation: &operatorv1alpha1.CredentialsRotation{
							CertificateAuthorities: &gardencorev1beta1.CARotation{
								Phase: gardencorev1beta1.RotationCompleting,
							},
						},
					},
				}),
				Entry("ca rotation phase is completed", true, operatorv1alpha1.GardenStatus{
					Credentials: &operatorv1alpha1.Credentials{
						Rotation: &operatorv1alpha1.CredentialsRotation{
							CertificateAuthorities: &gardencorev1beta1.CARotation{
								Phase: gardencorev1beta1.RotationCompleted,
							},
						},
					},
				}),
			)

			DescribeTable("completing CA rotation",
				func(allowed bool, status operatorv1alpha1.GardenStatus) {
					metav1.SetMetaDataAnnotation(&garden.ObjectMeta, "gardener.cloud/operation", "rotate-ca-complete")
					garden.Status = status

					matcher := BeEmpty()
					if !allowed {
						matcher = ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
							"Type":  Equal(field.ErrorTypeForbidden),
							"Field": Equal("metadata.annotations[gardener.cloud/operation]"),
						})))
					}

					Expect(ValidateGarden(garden)).To(matcher)
				},

				Entry("ca rotation phase is preparing", false, operatorv1alpha1.GardenStatus{
					Credentials: &operatorv1alpha1.Credentials{
						Rotation: &operatorv1alpha1.CredentialsRotation{
							CertificateAuthorities: &gardencorev1beta1.CARotation{
								Phase: gardencorev1beta1.RotationPreparing,
							},
						},
					},
				}),
				Entry("ca rotation phase is prepared", true, operatorv1alpha1.GardenStatus{
					Credentials: &operatorv1alpha1.Credentials{
						Rotation: &operatorv1alpha1.CredentialsRotation{
							CertificateAuthorities: &gardencorev1beta1.CARotation{
								Phase: gardencorev1beta1.RotationPrepared,
							},
						},
					},
				}),
				Entry("ca rotation phase is completing", false, operatorv1alpha1.GardenStatus{
					Credentials: &operatorv1alpha1.Credentials{
						Rotation: &operatorv1alpha1.CredentialsRotation{
							CertificateAuthorities: &gardencorev1beta1.CARotation{
								Phase: gardencorev1beta1.RotationCompleting,
							},
						},
					},
				}),
				Entry("ca rotation phase is completed", false, operatorv1alpha1.GardenStatus{
					Credentials: &operatorv1alpha1.Credentials{
						Rotation: &operatorv1alpha1.CredentialsRotation{
							CertificateAuthorities: &gardencorev1beta1.CARotation{
								Phase: gardencorev1beta1.RotationCompleted,
							},
						},
					},
				}),
			)
		})
	})

	Describe("#ValidateGarden", func() {
		var oldGarden, newGarden *operatorv1alpha1.Garden

		BeforeEach(func() {
			oldGarden = &operatorv1alpha1.Garden{
				ObjectMeta: metav1.ObjectMeta{
					Name: "garden",
				},
			}
			newGarden = oldGarden.DeepCopy()
		})

		Context("high availability setting", func() {
			It("should not be possible to remove the high availability setting once set", func() {
				oldGarden.Spec.VirtualCluster.ControlPlane = &operatorv1alpha1.ControlPlane{HighAvailability: &operatorv1alpha1.HighAvailability{}}

				Expect(ValidateGardenUpdate(oldGarden, newGarden)).To(ContainElement(PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeInvalid),
					"Field": Equal("spec.virtualCluster.controlPlane.highAvailability"),
				}))))
			})
		})
	})
})
