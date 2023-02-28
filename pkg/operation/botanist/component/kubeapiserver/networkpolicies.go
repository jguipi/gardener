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

package kubeapiserver

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/controllerutils"
	kubeapiserverconstants "github.com/gardener/gardener/pkg/operation/botanist/component/kubeapiserver/constants"
)

const (
	networkPolicyNameAllowFromShootAPIServer = "allow-from-shoot-apiserver"
	networkPolicyNameAllowToShootAPIServer   = "allow-to-shoot-apiserver"
	networkPolicyNameAllowKubeAPIServer      = "allow-" + v1beta1constants.DeploymentNameKubeAPIServer
)

func (k *kubeAPIServer) emptyNetworkPolicy(name string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: k.namespace}}
}

func (k *kubeAPIServer) reconcileNetworkPolicyAllowFromShootAPIServer(ctx context.Context, networkPolicy *networkingv1.NetworkPolicy) error {
	_, err := controllerutils.GetAndCreateOrMergePatch(ctx, k.client.Client(), networkPolicy, func() error {
		networkPolicy.Annotations = map[string]string{
			v1beta1constants.GardenerDescription: fmt.Sprintf("DEPRECATED: Do not use this policy anymore - label "+
				"kube-apiserver pods with `networking.resources.gardener.cloud/to-<service-name>-tcp-<container-port>=allowed` "+
				"instead. Allows Egress from Shoot's Kubernetes API Server to talk to pods labeled with '%s=%s'.",
				v1beta1constants.LabelNetworkPolicyFromShootAPIServer, v1beta1constants.LabelNetworkPolicyAllowed),
		}
		networkPolicy.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					v1beta1constants.LabelNetworkPolicyFromShootAPIServer: v1beta1constants.LabelNetworkPolicyAllowed,
				},
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: GetLabels(),
					},
				}},
			}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		}
		return nil
	})
	return err
}

func (k *kubeAPIServer) reconcileNetworkPolicyAllowToShootAPIServer(ctx context.Context, networkPolicy *networkingv1.NetworkPolicy) error {
	var (
		protocol = corev1.ProtocolTCP
		port     = intstr.FromInt(kubeapiserverconstants.Port)
	)

	_, err := controllerutils.GetAndCreateOrMergePatch(ctx, k.client.Client(), networkPolicy, func() error {
		networkPolicy.Annotations = map[string]string{
			v1beta1constants.GardenerDescription: fmt.Sprintf("DEPRECATED: Do not use this policy anymore - label "+
				"your pods with `networking.resources.gardener.cloud/to-kube-apiserver-tcp-443=allowed` instead. Allows "+
				"Egress from pods labeled with '%s=%s' to talk to Shoot's Kubernetes API Server.",
				v1beta1constants.LabelNetworkPolicyToShootAPIServer, v1beta1constants.LabelNetworkPolicyAllowed),
		}
		networkPolicy.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					v1beta1constants.LabelNetworkPolicyToShootAPIServer: v1beta1constants.LabelNetworkPolicyAllowed,
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{{
				To: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: GetLabels(),
					},
				}},
				Ports: []networkingv1.NetworkPolicyPort{{
					Protocol: &protocol,
					Port:     &port,
				}},
			}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		}
		return nil
	})
	return err
}
