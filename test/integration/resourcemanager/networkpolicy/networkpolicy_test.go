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
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/gardener/gardener/pkg/utils/test/matchers"
)

var _ = Describe("NetworkPolicy Controller tests", func() {
	var (
		namespace      *corev1.Namespace
		otherNamespace *corev1.Namespace
		service        *corev1.Service

		serviceSelector        = map[string]string{"foo": "bar"}
		customPodLabelSelector = "custom-selector"

		port1Protocol   = corev1.ProtocolTCP
		port1TargetPort = intstr.FromInt(5678)
		port1Suffix     = fmt.Sprintf("-%s-%s", strings.ToLower(string(port1Protocol)), port1TargetPort.String())

		port2Protocol   = corev1.ProtocolUDP
		port2TargetPort = intstr.FromString("testport")
		port2Suffix     = fmt.Sprintf("-%s-%s", strings.ToLower(string(port2Protocol)), port2TargetPort.String())

		port3Protocol   = corev1.ProtocolUDP
		port3TargetPort = intstr.FromString("testport2")
		port3Suffix     = fmt.Sprintf("-%s-%s", strings.ToLower(string(port3Protocol)), port3TargetPort.String())

		port4Protocol   = corev1.ProtocolTCP
		port4TargetPort = intstr.FromInt(9012)
		port4Suffix     = fmt.Sprintf("-%s-%s", strings.ToLower(string(port4Protocol)), port4TargetPort.String())

		ensureNetworkPolicies = func(asyncAssertion func(int, interface{}, ...interface{}) AsyncAssertion, should bool) func() {
			return func() {
				assertedFunc := func(g Gomega) []networkingv1.NetworkPolicy {
					networkPolicyList := &networkingv1.NetworkPolicyList{}
					g.Expect(testClient.List(ctx, networkPolicyList, client.InNamespace(service.Namespace))).To(Succeed())
					return networkPolicyList.Items
				}
				expectation := ContainElements(
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + port1Suffix)})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("egress-to-" + service.Name + port1Suffix)})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + port2Suffix)})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("egress-to-" + service.Name + port2Suffix)})}),
				)

				if should {
					asyncAssertion(1, assertedFunc).Should(expectation)
				} else {
					asyncAssertion(1, assertedFunc).ShouldNot(expectation)
				}
			}
		}
		ensureNetworkPoliciesGetCreated      = ensureNetworkPolicies(EventuallyWithOffset, true)
		ensureNetworkPoliciesGetDeleted      = ensureNetworkPolicies(EventuallyWithOffset, false)
		ensureNetworkPoliciesDoNotGetCreated = ensureNetworkPolicies(ConsistentlyWithOffset, false)
		ensureNetworkPoliciesDoNotGetDeleted = ensureNetworkPolicies(ConsistentlyWithOffset, true)

		ensureCrossNamespaceNetworkPolicies = func(asyncAssertion func(int, interface{}, ...interface{}) AsyncAssertion, should bool) func() {
			return func() {
				// ingress rules
				assertedFunc := func(g Gomega) []networkingv1.NetworkPolicy {
					networkPolicyList := &networkingv1.NetworkPolicyList{}
					g.Expect(testClient.List(ctx, networkPolicyList, client.InNamespace(service.Namespace))).To(Succeed())
					return networkPolicyList.Items
				}
				expectation := ContainElements(
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + port1Suffix + "-from-" + otherNamespace.Name)})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + port2Suffix + "-from-" + otherNamespace.Name)})}),
				)

				if should {
					asyncAssertion(1, assertedFunc).Should(expectation)
				} else {
					asyncAssertion(1, assertedFunc).ShouldNot(expectation)
				}

				// egress rules
				assertedFunc = func(g Gomega) []networkingv1.NetworkPolicy {
					networkPolicyList := &networkingv1.NetworkPolicyList{}
					g.Expect(testClient.List(ctx, networkPolicyList, client.InNamespace(otherNamespace.Name))).To(Succeed())
					return networkPolicyList.Items
				}
				expectation = ContainElements(
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("egress-to-" + service.Namespace + "-" + service.Name + port1Suffix)})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("egress-to-" + service.Namespace + "-" + service.Name + port2Suffix)})}),
				)

				if should {
					asyncAssertion(1, assertedFunc).Should(expectation)
				} else {
					asyncAssertion(1, assertedFunc).ShouldNot(expectation)
				}
			}
		}
		ensureCrossNamespaceNetworkPoliciesGetCreated      = ensureCrossNamespaceNetworkPolicies(EventuallyWithOffset, true)
		ensureCrossNamespaceNetworkPoliciesGetDeleted      = ensureCrossNamespaceNetworkPolicies(EventuallyWithOffset, false)
		ensureCrossNamespaceNetworkPoliciesDoNotGetCreated = ensureCrossNamespaceNetworkPolicies(ConsistentlyWithOffset, false)

		ensureNetworkPoliciesWithCustomPodLabelSelector = func(asyncAssertion func(int, interface{}, ...interface{}) AsyncAssertion, should bool) func() {
			return func() {
				assertedFunc := func(g Gomega) []networkingv1.NetworkPolicy {
					networkPolicyList := &networkingv1.NetworkPolicyList{}
					g.Expect(testClient.List(ctx, networkPolicyList, client.InNamespace(service.Namespace))).To(Succeed())
					return networkPolicyList.Items
				}
				expectation := ContainElements(
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + port3Suffix + "-via-" + customPodLabelSelector)})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("egress-to-" + service.Name + port3Suffix + "-via-" + customPodLabelSelector)})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + port4Suffix + "-via-" + customPodLabelSelector)})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("egress-to-" + service.Name + port4Suffix + "-via-" + customPodLabelSelector)})}),
				)

				if should {
					asyncAssertion(1, assertedFunc).Should(expectation)
				} else {
					asyncAssertion(1, assertedFunc).ShouldNot(expectation)
				}
			}
		}
		ensureNetworkPoliciesWithCustomPodLabelSelectorGetCreated = ensureNetworkPoliciesWithCustomPodLabelSelector(EventuallyWithOffset, true)
		ensureNetworkPoliciesWithCustomPodLabelSelectorGetDeleted = ensureNetworkPoliciesWithCustomPodLabelSelector(EventuallyWithOffset, false)

		ensureIngressFromWorldNetworkPolicy = func(asyncAssertion func(int, interface{}, ...interface{}) AsyncAssertion, should bool) func() {
			return func() {
				assertedFunc := func(g Gomega) []networkingv1.NetworkPolicy {
					networkPolicyList := &networkingv1.NetworkPolicyList{}
					g.Expect(testClient.List(ctx, networkPolicyList, client.InNamespace(service.Namespace))).To(Succeed())
					return networkPolicyList.Items
				}
				expectation := ContainElement(
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + "-from-world")})}),
				)

				if should {
					asyncAssertion(1, assertedFunc).Should(expectation)
				} else {
					asyncAssertion(1, assertedFunc).ShouldNot(expectation)
				}
			}
		}
		ensureIngressFromWorldNetworkPolicyGetsCreated       = ensureIngressFromWorldNetworkPolicy(EventuallyWithOffset, true)
		ensureIngressFromWorldNetworkPolicyGetsDeleted       = ensureIngressFromWorldNetworkPolicy(EventuallyWithOffset, false)
		ensureIngressFromWorldNetworkPolicyDoesNotGetCreated = ensureIngressFromWorldNetworkPolicy(ConsistentlyWithOffset, false)
	)

	BeforeEach(func() {
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-ns-" + testRunID,
				Labels: map[string]string{testID: testRunID},
			},
		}
		otherNamespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "other-ns-" + testRunID,
				Labels: map[string]string{"other": "namespace"},
			},
		}
		service = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "service-",
				Namespace:    namespace.Name,
			},
			Spec: corev1.ServiceSpec{
				Selector: serviceSelector,
				Ports: []corev1.ServicePort{
					{Name: "port1", Port: 1234, Protocol: port1Protocol, TargetPort: port1TargetPort},
					{Name: "port2", Port: 9012, Protocol: port2Protocol, TargetPort: port2TargetPort},
				},
			},
		}
	})

	JustBeforeEach(func() {
		By("Create test Namespace")
		Expect(testClient.Create(ctx, namespace)).To(Succeed())
		log.Info("Created test Namespace", "namespace", client.ObjectKeyFromObject(namespace))

		DeferCleanup(func() {
			By("Delete test Namespace")
			Expect(testClient.Delete(ctx, namespace)).To(Or(Succeed(), BeNotFoundError()))
			log.Info("Deleted test Namespace", "namespace", client.ObjectKeyFromObject(namespace))

			By("Wait until manager has observed test Namespace deletion")
			Eventually(func() error {
				return mgrClient.Get(ctx, client.ObjectKeyFromObject(namespace), namespace)
			}).Should(BeNotFoundError())
		})

		By("Create other Namespace")
		Expect(testClient.Create(ctx, otherNamespace)).To(Succeed())
		log.Info("Created other Namespace", "namespace", client.ObjectKeyFromObject(otherNamespace))

		DeferCleanup(func() {
			By("Delete other Namespace")
			Expect(testClient.Delete(ctx, otherNamespace)).To(Or(Succeed(), BeNotFoundError()))
			log.Info("Deleted other Namespace", "namespace", client.ObjectKeyFromObject(otherNamespace))

			By("Wait until manager has observed other Namespace deletion")
			Eventually(func() error {
				return mgrClient.Get(ctx, client.ObjectKeyFromObject(otherNamespace), otherNamespace)
			}).Should(BeNotFoundError())
		})

		By("Create Service")
		Expect(testClient.Create(ctx, service)).To(Succeed())
		log.Info("Created Service", "service", client.ObjectKeyFromObject(service))

		DeferCleanup(func() {
			By("Delete Service")
			Expect(testClient.Delete(ctx, service)).To(Or(Succeed(), BeNotFoundError()))
			log.Info("Deleted Service", "service", client.ObjectKeyFromObject(service))

			By("Wait until manager has observed Service deletion")
			Eventually(func() error {
				return mgrClient.Get(ctx, client.ObjectKeyFromObject(service), service)
			}).Should(BeNotFoundError())
		})
	})

	Context("without pod selector in service", func() {
		BeforeEach(func() {
			service.Spec.Selector = nil
		})

		It("should not create any network policies", func() {
			By("Ensure no policies are created")
			ensureNetworkPoliciesDoNotGetCreated()
		})
	})

	Context("service in handled namespace", func() {
		JustBeforeEach(func() {
			By("Wait until finalizer was set on service")
			Eventually(func(g Gomega) []string {
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(service), service)).To(Succeed())
				return service.Finalizers
			}).Should(ConsistOf("resources.gardener.cloud/networkpolicy-controller"))
		})

		It("should create the expected network policies", func() {
			By("Wait until ingress policy was created for first port")
			Eventually(func(g Gomega) networkingv1.NetworkPolicySpec {
				networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "ingress-to-" + service.Name + port1Suffix, Namespace: service.Namespace}}
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
				return networkPolicy.Spec
			}).Should(Equal(networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{MatchLabels: serviceSelector},
				Ingress: []networkingv1.NetworkPolicyIngressRule{{
					From:  []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"networking.resources.gardener.cloud/to-" + service.Name + port1Suffix: "allowed"}}}},
					Ports: []networkingv1.NetworkPolicyPort{{Protocol: &port1Protocol, Port: &port1TargetPort}},
				}},
			}))

			By("Wait until egress policy was created for first port")
			Eventually(func(g Gomega) networkingv1.NetworkPolicySpec {
				networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "egress-to-" + service.Name + port1Suffix, Namespace: service.Namespace}}
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
				return networkPolicy.Spec
			}).Should(Equal(networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
				PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"networking.resources.gardener.cloud/to-" + service.Name + port1Suffix: "allowed"}},
				Egress: []networkingv1.NetworkPolicyEgressRule{{
					To:    []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: serviceSelector}}},
					Ports: []networkingv1.NetworkPolicyPort{{Protocol: &port1Protocol, Port: &port1TargetPort}},
				}},
			}))

			By("Wait until ingress policy was created for second port")
			Eventually(func(g Gomega) networkingv1.NetworkPolicySpec {
				networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "ingress-to-" + service.Name + port2Suffix, Namespace: service.Namespace}}
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
				return networkPolicy.Spec
			}).Should(Equal(networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{MatchLabels: serviceSelector},
				Ingress: []networkingv1.NetworkPolicyIngressRule{{
					From:  []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"networking.resources.gardener.cloud/to-" + service.Name + port2Suffix: "allowed"}}}},
					Ports: []networkingv1.NetworkPolicyPort{{Protocol: &port2Protocol, Port: &port2TargetPort}},
				}},
			}))

			By("Wait until egress policy was created for second port")
			Eventually(func(g Gomega) networkingv1.NetworkPolicySpec {
				networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "egress-to-" + service.Name + port2Suffix, Namespace: service.Namespace}}
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
				return networkPolicy.Spec
			}).Should(Equal(networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
				PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"networking.resources.gardener.cloud/to-" + service.Name + port2Suffix: "allowed"}},
				Egress: []networkingv1.NetworkPolicyEgressRule{{
					To:    []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: serviceSelector}}},
					Ports: []networkingv1.NetworkPolicyPort{{Protocol: &port2Protocol, Port: &port2TargetPort}},
				}},
			}))
		})

		It("should not create any cross-namespace policies or ingress-from-world policy", func() {
			ensureCrossNamespaceNetworkPoliciesDoNotGetCreated()
			ensureIngressFromWorldNetworkPolicyDoesNotGetCreated()
		})

		It("should reconcile the policies when the ports in service are changed", func() {
			By("Wait until all policies are created")
			ensureNetworkPoliciesGetCreated()

			By("Patch Service")
			patch := client.MergeFrom(service.DeepCopy())
			service.Spec.Ports = []corev1.ServicePort{service.Spec.Ports[1]}
			service.Spec.Ports = append(service.Spec.Ports, corev1.ServicePort{Name: "newport", Port: 1357, Protocol: corev1.ProtocolUDP, TargetPort: intstr.FromInt(2468)})
			Expect(testClient.Patch(ctx, service, patch)).To(Succeed())

			By("Wait until all policies were reconciled")
			Eventually(func(g Gomega) []networkingv1.NetworkPolicy {
				networkPolicyList := &networkingv1.NetworkPolicyList{}
				g.Expect(testClient.List(ctx, networkPolicyList, client.InNamespace(service.Namespace))).To(Succeed())
				return networkPolicyList.Items
			}).Should(And(
				Not(ContainElements(
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + port1Suffix)})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("egress-to-" + service.Name + port1Suffix)})}),
				)),
				ContainElements(
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + port2Suffix)})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("egress-to-" + service.Name + port2Suffix)})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + "-udp-2468")})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("egress-to-" + service.Name + "-udp-2468")})}),
				),
			))
		})

		It("should delete the policies when the pod selector in service is removed", func() {
			By("Wait until all policies are created")
			ensureNetworkPoliciesGetCreated()

			By("Patch Service")
			patch := client.MergeFrom(service.DeepCopy())
			service.Spec.Selector = nil
			Expect(testClient.Patch(ctx, service, patch)).To(Succeed())

			By("Wait until all policies are deleted")
			ensureNetworkPoliciesGetDeleted()
		})

		It("should delete the policies when the service gets deleted", func() {
			By("Wait until all policies are created")
			ensureNetworkPoliciesGetCreated()

			By("Delete Service")
			Expect(testClient.Delete(ctx, service)).To(Succeed())

			By("Wait until all policies are deleted")
			ensureNetworkPoliciesGetDeleted()
		})

		It("should delete the policies when the namespace is no longer handled", func() {
			By("Wait until all policies are created")
			ensureNetworkPoliciesGetCreated()

			By("Patch Namespace and remove label")
			patch := client.MergeFrom(namespace.DeepCopy())
			namespace.Labels[testID] = "foo"
			Expect(testClient.Patch(ctx, namespace, patch)).To(Succeed())

			By("Wait until all policies are deleted")
			ensureNetworkPoliciesGetDeleted()
		})
	})

	Context("service in non-handled namespace", func() {
		BeforeEach(func() {
			service.Namespace = otherNamespace.Name
		})

		It("should not create any network policies", func() {
			By("Ensure no policies are created")
			ensureNetworkPoliciesDoNotGetCreated()
		})

		It("should create network policies as soon as the namespace is handled", func() {
			By("Patch Namespace")
			patch := client.MergeFrom(otherNamespace.DeepCopy())
			metav1.SetMetaDataLabel(&otherNamespace.ObjectMeta, testID, testRunID)
			Expect(testClient.Patch(ctx, otherNamespace, patch)).To(Succeed())

			By("Ensure no policies are created")
			ensureNetworkPoliciesGetCreated()
		})
	})

	Context("service with namespace selector", func() {
		BeforeEach(func() {
			metav1.SetMetaDataAnnotation(&service.ObjectMeta, "networking.resources.gardener.cloud/namespace-selectors", `[{"matchLabels":{"other":"namespace"}}]`)
		})

		It("should create the expected cross-namespace network policies", func() {
			ensureNetworkPoliciesGetCreated()

			By("Wait until ingress from other-namespace policy was created for first port")
			Eventually(func(g Gomega) networkingv1.NetworkPolicySpec {
				networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "ingress-to-" + service.Name + port1Suffix + "-from-" + otherNamespace.Name, Namespace: service.Namespace}}
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
				return networkPolicy.Spec
			}).Should(Equal(networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{MatchLabels: serviceSelector},
				Ingress: []networkingv1.NetworkPolicyIngressRule{{
					From: []networkingv1.NetworkPolicyPeer{{
						PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{"networking.resources.gardener.cloud/to-" + service.Namespace + "-" + service.Name + port1Suffix: "allowed"}},
						NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": otherNamespace.Name}},
					}},
					Ports: []networkingv1.NetworkPolicyPort{{Protocol: &port1Protocol, Port: &port1TargetPort}},
				}},
			}))

			By("Wait until egress from other-namespace policy was created for first port")
			Eventually(func(g Gomega) networkingv1.NetworkPolicySpec {
				networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "egress-to-" + service.Namespace + "-" + service.Name + port1Suffix, Namespace: otherNamespace.Name}}
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
				return networkPolicy.Spec
			}).Should(Equal(networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
				PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"networking.resources.gardener.cloud/to-" + service.Namespace + "-" + service.Name + port1Suffix: "allowed"}},
				Egress: []networkingv1.NetworkPolicyEgressRule{{
					To: []networkingv1.NetworkPolicyPeer{{
						PodSelector:       &metav1.LabelSelector{MatchLabels: serviceSelector},
						NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": service.Namespace}},
					}},
					Ports: []networkingv1.NetworkPolicyPort{{Protocol: &port1Protocol, Port: &port1TargetPort}},
				}},
			}))

			By("Wait until ingress from other-namespace policy was created for second port")
			Eventually(func(g Gomega) networkingv1.NetworkPolicySpec {
				networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "ingress-to-" + service.Name + port2Suffix + "-from-" + otherNamespace.Name, Namespace: service.Namespace}}
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
				return networkPolicy.Spec
			}).Should(Equal(networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{MatchLabels: serviceSelector},
				Ingress: []networkingv1.NetworkPolicyIngressRule{{
					From: []networkingv1.NetworkPolicyPeer{{
						PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{"networking.resources.gardener.cloud/to-" + service.Namespace + "-" + service.Name + port2Suffix: "allowed"}},
						NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": otherNamespace.Name}},
					}},
					Ports: []networkingv1.NetworkPolicyPort{{Protocol: &port2Protocol, Port: &port2TargetPort}},
				}},
			}))

			By("Wait until egress from other-namespace policy was created for second port")
			Eventually(func(g Gomega) networkingv1.NetworkPolicySpec {
				networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "egress-to-" + service.Namespace + "-" + service.Name + port2Suffix, Namespace: otherNamespace.Name}}
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
				return networkPolicy.Spec
			}).Should(Equal(networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
				PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"networking.resources.gardener.cloud/to-" + service.Namespace + "-" + service.Name + port2Suffix: "allowed"}},
				Egress: []networkingv1.NetworkPolicyEgressRule{{
					To: []networkingv1.NetworkPolicyPeer{{
						PodSelector:       &metav1.LabelSelector{MatchLabels: serviceSelector},
						NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": service.Namespace}},
					}},
					Ports: []networkingv1.NetworkPolicyPort{{Protocol: &port2Protocol, Port: &port2TargetPort}},
				}},
			}))
		})

		It("should reconcile the policies when the ports in service are changed", func() {
			By("Wait until all policies are created")
			ensureNetworkPoliciesGetCreated()
			ensureCrossNamespaceNetworkPoliciesGetCreated()

			By("Patch Service")
			patch := client.MergeFrom(service.DeepCopy())
			service.Spec.Ports = []corev1.ServicePort{service.Spec.Ports[1]}
			service.Spec.Ports = append(service.Spec.Ports, corev1.ServicePort{Name: "newport", Port: 1357, Protocol: corev1.ProtocolUDP, TargetPort: intstr.FromInt(2468)})
			Expect(testClient.Patch(ctx, service, patch)).To(Succeed())

			By("Wait until cross-namespace policies were reconciled")
			Eventually(func(g Gomega) []networkingv1.NetworkPolicy {
				networkPolicyList := &networkingv1.NetworkPolicyList{}
				g.Expect(testClient.List(ctx, networkPolicyList, client.InNamespace(service.Namespace))).To(Succeed())
				return networkPolicyList.Items
			}).Should(And(
				Not(ContainElements(
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + port1Suffix + "-from-" + otherNamespace.Name)})}),
				)),
				ContainElements(
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + port2Suffix + "-from-" + otherNamespace.Name)})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + "-udp-2468-from-" + otherNamespace.Name)})}),
				),
			))

			Eventually(func(g Gomega) []networkingv1.NetworkPolicy {
				networkPolicyList := &networkingv1.NetworkPolicyList{}
				g.Expect(testClient.List(ctx, networkPolicyList, client.InNamespace(otherNamespace.Name))).To(Succeed())
				return networkPolicyList.Items
			}).Should(And(
				Not(ContainElements(
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("egress-to-" + service.Namespace + "-" + service.Name + port1Suffix)})}),
				)),
				ContainElements(
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("egress-to-" + service.Namespace + "-" + service.Name + port2Suffix)})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("egress-to-" + service.Namespace + "-" + service.Name + "-udp-2468")})}),
				),
			))
		})

		It("should delete the policies when the pod selector in service is removed", func() {
			By("Wait until all policies are created")
			ensureNetworkPoliciesGetCreated()
			ensureCrossNamespaceNetworkPoliciesGetCreated()

			By("Patch Service")
			patch := client.MergeFrom(service.DeepCopy())
			service.Spec.Selector = nil
			Expect(testClient.Patch(ctx, service, patch)).To(Succeed())

			By("Wait until all policies are deleted")
			ensureNetworkPoliciesGetDeleted()
			ensureCrossNamespaceNetworkPoliciesGetDeleted()
		})

		It("should delete the policies when the service gets deleted", func() {
			By("Wait until all policies are created")
			ensureNetworkPoliciesGetCreated()
			ensureCrossNamespaceNetworkPoliciesGetCreated()

			By("Delete Service")
			Expect(testClient.Delete(ctx, service)).To(Succeed())

			By("Wait until all policies are deleted")
			ensureNetworkPoliciesGetDeleted()
			ensureCrossNamespaceNetworkPoliciesGetDeleted()
		})

		It("should delete the policies when the namespace is no longer handled", func() {
			By("Wait until all policies are created")
			ensureNetworkPoliciesGetCreated()
			ensureCrossNamespaceNetworkPoliciesGetCreated()

			By("Patch Namespace and remove label")
			patch := client.MergeFrom(otherNamespace.DeepCopy())
			otherNamespace.Labels["other"] = "namespace2"
			Expect(testClient.Patch(ctx, otherNamespace, patch)).To(Succeed())

			By("Wait until all policies are deleted")
			ensureNetworkPoliciesDoNotGetDeleted()
			ensureCrossNamespaceNetworkPoliciesGetDeleted()
		})

		It("should create the expected cross-namespace policies as soon as a new namespace appears", func() {
			newNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				GenerateName: "new-ns-",
				Labels:       map[string]string{"other": "namespace"},
			}}

			By("Create new Namespace")
			Expect(testClient.Create(ctx, newNamespace)).To(Succeed())
			log.Info("Created new Namespace", "namespace", client.ObjectKeyFromObject(newNamespace))

			DeferCleanup(func() {
				By("Delete new Namespace")
				Expect(testClient.Delete(ctx, newNamespace)).To(Or(Succeed(), BeNotFoundError()))
				log.Info("Deleted new Namespace", "namespace", client.ObjectKeyFromObject(newNamespace))

				By("Wait until manager has observed new Namespace deletion")
				Eventually(func() error {
					return mgrClient.Get(ctx, client.ObjectKeyFromObject(newNamespace), newNamespace)
				}).Should(BeNotFoundError())
			})

			By("Wait until all ingress policies are created")
			Eventually(func(g Gomega) []networkingv1.NetworkPolicy {
				networkPolicyList := &networkingv1.NetworkPolicyList{}
				g.Expect(testClient.List(ctx, networkPolicyList, client.InNamespace(service.Namespace))).To(Succeed())
				return networkPolicyList.Items
			}).Should(ContainElements(
				MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + port1Suffix + "-from-" + otherNamespace.Name)})}),
				MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + port2Suffix + "-from-" + otherNamespace.Name)})}),
			))

			By("Wait until all egress policies are created")
			Eventually(func(g Gomega) []networkingv1.NetworkPolicy {
				networkPolicyList := &networkingv1.NetworkPolicyList{}
				g.Expect(testClient.List(ctx, networkPolicyList, client.InNamespace(newNamespace.Name))).To(Succeed())
				return networkPolicyList.Items
			}).Should(ContainElements(
				MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("egress-to-" + service.Namespace + "-" + service.Name + port1Suffix)})}),
				MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("egress-to-" + service.Namespace + "-" + service.Name + port2Suffix)})}),
			))
		})

		Context("with pod label selector namespace alias", func() {
			alias := "alias"

			BeforeEach(func() {
				metav1.SetMetaDataAnnotation(&service.ObjectMeta, "networking.resources.gardener.cloud/pod-label-selector-namespace-alias", alias)
			})

			It("should create the expected cross-namespace network policies", func() {
				ensureNetworkPoliciesGetCreated()

				By("Wait until ingress from other-namespace policy was created for first port")
				Eventually(func(g Gomega) *metav1.LabelSelector {
					networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "ingress-to-" + service.Name + port1Suffix + "-from-" + otherNamespace.Name, Namespace: service.Namespace}}
					g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
					return networkPolicy.Spec.Ingress[0].From[0].PodSelector
				}).Should(Equal(&metav1.LabelSelector{MatchLabels: map[string]string{"networking.resources.gardener.cloud/to-" + alias + "-" + service.Name + port1Suffix: "allowed"}}))

				By("Wait until egress from other-namespace policy was created for first port")
				Eventually(func(g Gomega) metav1.LabelSelector {
					networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "egress-to-" + service.Namespace + "-" + service.Name + port1Suffix, Namespace: otherNamespace.Name}}
					g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
					return networkPolicy.Spec.PodSelector
				}).Should(Equal(metav1.LabelSelector{MatchLabels: map[string]string{"networking.resources.gardener.cloud/to-" + alias + "-" + service.Name + port1Suffix: "allowed"}}))

				By("Wait until ingress from other-namespace policy was created for second port")
				Eventually(func(g Gomega) *metav1.LabelSelector {
					networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "ingress-to-" + service.Name + port2Suffix + "-from-" + otherNamespace.Name, Namespace: service.Namespace}}
					g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
					return networkPolicy.Spec.Ingress[0].From[0].PodSelector
				}).Should(Equal(&metav1.LabelSelector{MatchLabels: map[string]string{"networking.resources.gardener.cloud/to-" + alias + "-" + service.Name + port2Suffix: "allowed"}}))

				By("Wait until egress from other-namespace policy was created for second port")
				Eventually(func(g Gomega) metav1.LabelSelector {
					networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "egress-to-" + service.Namespace + "-" + service.Name + port2Suffix, Namespace: otherNamespace.Name}}
					g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
					return networkPolicy.Spec.PodSelector
				}).Should(Equal(metav1.LabelSelector{MatchLabels: map[string]string{"networking.resources.gardener.cloud/to-" + alias + "-" + service.Name + port2Suffix: "allowed"}}))
			})
		})
	})

	Context("service with custom pod label selector", func() {
		BeforeEach(func() {
			metav1.SetMetaDataAnnotation(&service.ObjectMeta, "networking.resources.gardener.cloud/from-policy-pod-label-selector", customPodLabelSelector)
			metav1.SetMetaDataAnnotation(&service.ObjectMeta, "networking.resources.gardener.cloud/from-policy-allowed-ports", `[{"protocol":"`+string(port3Protocol)+`","port":"`+port3TargetPort.String()+`"},{"protocol":"`+string(port4Protocol)+`","port":`+port4TargetPort.String()+`}]`)
		})

		It("should create the expected network policies", func() {
			By("Wait until ingress policy was created for first port")
			Eventually(func(g Gomega) networkingv1.NetworkPolicySpec {
				networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "ingress-to-" + service.Name + port3Suffix + "-via-" + customPodLabelSelector, Namespace: service.Namespace}}
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
				return networkPolicy.Spec
			}).Should(Equal(networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{MatchLabels: serviceSelector},
				Ingress: []networkingv1.NetworkPolicyIngressRule{{
					From:  []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"networking.resources.gardener.cloud/to-" + customPodLabelSelector: "allowed"}}}},
					Ports: []networkingv1.NetworkPolicyPort{{Protocol: &port3Protocol, Port: &port3TargetPort}},
				}},
			}))

			By("Wait until egress policy was created for first port")
			Eventually(func(g Gomega) networkingv1.NetworkPolicySpec {
				networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "egress-to-" + service.Name + port3Suffix + "-via-" + customPodLabelSelector, Namespace: service.Namespace}}
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
				return networkPolicy.Spec
			}).Should(Equal(networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
				PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"networking.resources.gardener.cloud/to-" + customPodLabelSelector: "allowed"}},
				Egress: []networkingv1.NetworkPolicyEgressRule{{
					To:    []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: serviceSelector}}},
					Ports: []networkingv1.NetworkPolicyPort{{Protocol: &port3Protocol, Port: &port3TargetPort}},
				}},
			}))

			By("Wait until ingress policy was created for second port")
			Eventually(func(g Gomega) networkingv1.NetworkPolicySpec {
				networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "ingress-to-" + service.Name + port4Suffix + "-via-" + customPodLabelSelector, Namespace: service.Namespace}}
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
				return networkPolicy.Spec
			}).Should(Equal(networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{MatchLabels: serviceSelector},
				Ingress: []networkingv1.NetworkPolicyIngressRule{{
					From:  []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"networking.resources.gardener.cloud/to-" + customPodLabelSelector: "allowed"}}}},
					Ports: []networkingv1.NetworkPolicyPort{{Protocol: &port4Protocol, Port: &port4TargetPort}},
				}},
			}))

			By("Wait until egress policy was created for second port")
			Eventually(func(g Gomega) networkingv1.NetworkPolicySpec {
				networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "egress-to-" + service.Name + port4Suffix + "-via-" + customPodLabelSelector, Namespace: service.Namespace}}
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
				return networkPolicy.Spec
			}).Should(Equal(networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
				PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"networking.resources.gardener.cloud/to-" + customPodLabelSelector: "allowed"}},
				Egress: []networkingv1.NetworkPolicyEgressRule{{
					To:    []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: serviceSelector}}},
					Ports: []networkingv1.NetworkPolicyPort{{Protocol: &port4Protocol, Port: &port4TargetPort}},
				}},
			}))
		})

		It("should reconcile the policies when the allowed ports are changed", func() {
			By("Wait until all policies are created")
			ensureNetworkPoliciesWithCustomPodLabelSelectorGetCreated()

			By("Patch Service")
			patch := client.MergeFrom(service.DeepCopy())
			metav1.SetMetaDataAnnotation(&service.ObjectMeta, "networking.resources.gardener.cloud/from-policy-allowed-ports", `[{"protocol":"`+string(port4Protocol)+`","port":`+port4TargetPort.String()+`},{"protocol":"`+string(corev1.ProtocolUDP)+`","port":2468}]`)
			Expect(testClient.Patch(ctx, service, patch)).To(Succeed())

			By("Wait until all policies were reconciled")
			Eventually(func(g Gomega) []networkingv1.NetworkPolicy {
				networkPolicyList := &networkingv1.NetworkPolicyList{}
				g.Expect(testClient.List(ctx, networkPolicyList, client.InNamespace(service.Namespace))).To(Succeed())
				return networkPolicyList.Items
			}).Should(And(
				Not(ContainElements(
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + port3Suffix + "-via-" + customPodLabelSelector)})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("egress-to-" + service.Name + port3Suffix + "-via-" + customPodLabelSelector)})}),
				)),
				ContainElements(
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + port4Suffix + "-via-" + customPodLabelSelector)})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("egress-to-" + service.Name + port4Suffix + "-via-" + customPodLabelSelector)})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("ingress-to-" + service.Name + "-udp-2468-via-" + customPodLabelSelector)})}),
					MatchFields(IgnoreExtras, Fields{"ObjectMeta": MatchFields(IgnoreExtras, Fields{"Name": Equal("egress-to-" + service.Name + "-udp-2468-via-" + customPodLabelSelector)})}),
				),
			))
		})

		It("should not create any cross-namespace policies or ingress-from-world policy", func() {
			ensureCrossNamespaceNetworkPoliciesDoNotGetCreated()
			ensureIngressFromWorldNetworkPolicyDoesNotGetCreated()
		})

		It("should delete the policies when the custom pod label selector in service annotations is removed", func() {
			By("Wait until all policies are created")
			ensureNetworkPoliciesWithCustomPodLabelSelectorGetCreated()

			By("Patch Service")
			patch := client.MergeFrom(service.DeepCopy())
			delete(service.Annotations, "networking.resources.gardener.cloud/from-policy-pod-label-selector")
			Expect(testClient.Patch(ctx, service, patch)).To(Succeed())

			By("Wait until all policies are deleted")
			ensureNetworkPoliciesWithCustomPodLabelSelectorGetDeleted()
		})

		It("should delete the policies when the allowed ports in service annotations are removed", func() {
			By("Wait until all policies are created")
			ensureNetworkPoliciesWithCustomPodLabelSelectorGetCreated()

			By("Patch Service")
			patch := client.MergeFrom(service.DeepCopy())
			delete(service.Annotations, "networking.resources.gardener.cloud/from-policy-allowed-ports")
			Expect(testClient.Patch(ctx, service, patch)).To(Succeed())

			By("Wait until all policies are deleted")
			ensureNetworkPoliciesWithCustomPodLabelSelectorGetDeleted()
		})

		It("should delete the policies when the service gets deleted", func() {
			By("Wait until all policies are created")
			ensureNetworkPoliciesWithCustomPodLabelSelectorGetCreated()

			By("Delete Service")
			Expect(testClient.Delete(ctx, service)).To(Succeed())

			By("Wait until all policies are deleted")
			ensureNetworkPoliciesWithCustomPodLabelSelectorGetDeleted()
		})

		It("should delete the policies when the namespace is no longer handled", func() {
			By("Wait until all policies are created")
			ensureNetworkPoliciesWithCustomPodLabelSelectorGetCreated()

			By("Patch Namespace and remove label")
			patch := client.MergeFrom(namespace.DeepCopy())
			namespace.Labels[testID] = "foo"
			Expect(testClient.Patch(ctx, namespace, patch)).To(Succeed())

			By("Wait until all policies are deleted")
			ensureNetworkPoliciesWithCustomPodLabelSelectorGetDeleted()
		})
	})

	Context("service with ingress from world", func() {
		BeforeEach(func() {
			metav1.SetMetaDataAnnotation(&service.ObjectMeta, "networking.resources.gardener.cloud/from-world-to-ports", `[{"port":`+port1TargetPort.String()+`,"protocol":"`+string(port1Protocol)+`"},{"port":"`+port2TargetPort.String()+`","protocol":"`+string(port2Protocol)+`"}]`)
		})

		It("should create the expected ingress-from-world network policy", func() {
			ensureNetworkPoliciesGetCreated()

			By("Wait until ingress from world policy was created")
			Eventually(func(g Gomega) networkingv1.NetworkPolicySpec {
				networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "ingress-to-" + service.Name + "-from-world", Namespace: service.Namespace}}
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
				return networkPolicy.Spec
			}).Should(Equal(networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{MatchLabels: serviceSelector},
				Ingress: []networkingv1.NetworkPolicyIngressRule{{
					From: []networkingv1.NetworkPolicyPeer{
						{PodSelector: &metav1.LabelSelector{}, NamespaceSelector: &metav1.LabelSelector{}},
						{IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0"}},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &port1Protocol, Port: &port1TargetPort},
						{Protocol: &port2Protocol, Port: &port2TargetPort},
					},
				}},
			}))
		})

		It("should reconcile the policies when the ports in service are changed", func() {
			By("Wait until all policies are created")
			ensureIngressFromWorldNetworkPolicyGetsCreated()

			By("Patch Service")
			patch := client.MergeFrom(service.DeepCopy())
			metav1.SetMetaDataAnnotation(&service.ObjectMeta, "networking.resources.gardener.cloud/from-world-to-ports", "[]")
			Expect(testClient.Patch(ctx, service, patch)).To(Succeed())

			By("Wait until ingress from world policy was updated")
			Eventually(func(g Gomega) networkingv1.NetworkPolicySpec {
				networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "ingress-to-" + service.Name + "-from-world", Namespace: service.Namespace}}
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(networkPolicy), networkPolicy)).To(Succeed())
				return networkPolicy.Spec
			}).Should(Equal(networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{MatchLabels: serviceSelector},
				Ingress: []networkingv1.NetworkPolicyIngressRule{{
					From: []networkingv1.NetworkPolicyPeer{
						{PodSelector: &metav1.LabelSelector{}, NamespaceSelector: &metav1.LabelSelector{}},
						{IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0"}},
					},
				}},
			}))
		})

		It("should delete the policies when the pod selector in service is removed", func() {
			By("Wait until all policies are created")
			ensureNetworkPoliciesGetCreated()
			ensureIngressFromWorldNetworkPolicyGetsCreated()

			By("Patch Service")
			patch := client.MergeFrom(service.DeepCopy())
			service.Spec.Selector = nil
			Expect(testClient.Patch(ctx, service, patch)).To(Succeed())

			By("Wait until all policies are deleted")
			ensureNetworkPoliciesGetDeleted()
			ensureIngressFromWorldNetworkPolicyGetsDeleted()
		})

		It("should delete the policies when the service gets deleted", func() {
			By("Wait until all policies are created")
			ensureNetworkPoliciesGetCreated()
			ensureIngressFromWorldNetworkPolicyGetsCreated()

			By("Delete Service")
			Expect(testClient.Delete(ctx, service)).To(Succeed())

			By("Wait until all policies are deleted")
			ensureNetworkPoliciesGetDeleted()
			ensureIngressFromWorldNetworkPolicyGetsDeleted()
		})
	})
})
