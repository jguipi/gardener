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

package networkpolicy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/resourcemanager/apis/config"
	"github.com/gardener/gardener/pkg/utils/flow"
	kubernetesutils "github.com/gardener/gardener/pkg/utils/kubernetes"
)

const finalizerName = "resources.gardener.cloud/networkpolicy-controller"

// Reconciler reconciles Service objects and creates NetworkPolicy objects.
type Reconciler struct {
	TargetClient client.Client
	Config       config.NetworkPolicyControllerConfig

	selectors []labels.Selector
}

// Reconcile performs the main reconciliation logic.
func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	service := &corev1.Service{}
	if err := r.TargetClient.Get(ctx, request.NamespacedName, service); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	networkPolicyList := &metav1.PartialObjectMetadataList{}
	networkPolicyList.SetGroupVersionKind(networkingv1.SchemeGroupVersion.WithKind("NetworkPolicyList"))
	if err := r.TargetClient.List(ctx, networkPolicyList, client.MatchingLabels{
		resourcesv1alpha1.NetworkingServiceName:      service.Name,
		resourcesv1alpha1.NetworkingServiceNamespace: service.Namespace,
	}); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed listing network policies for service %s: %w", client.ObjectKeyFromObject(service), err)
	}

	isNamespaceHandled, err := r.namespaceIsHandled(ctx, service.Namespace)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed checking whether namespace %s is handled: %w", service.Namespace, err)
	}

	if service.DeletionTimestamp != nil || service.Spec.Selector == nil || !isNamespaceHandled {
		if !controllerutil.ContainsFinalizer(service, finalizerName) {
			return reconcile.Result{}, nil
		}

		deleteTaskFns := r.deleteStalePolicies(networkPolicyList, nil)
		if err := flow.Parallel(deleteTaskFns...)(ctx); err != nil {
			return reconcile.Result{}, err
		}

		log.Info("Removing finalizer")
		if err := controllerutils.RemoveFinalizers(ctx, r.TargetClient, service, finalizerName); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}

		return reconcile.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(service, finalizerName) {
		log.Info("Adding finalizer")
		if err := controllerutils.AddFinalizers(ctx, r.TargetClient, service, finalizerName); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
	}

	namespaceNames, err := r.fetchRelevantNamespaceNames(ctx, service)
	if err != nil {
		return reconcile.Result{}, err
	}

	reconcileTaskFns, desiredObjectMetaKeys, err := r.reconcileDesiredPolicies(service, namespaceNames)
	if err != nil {
		return reconcile.Result{}, err
	}
	deleteTaskFns := r.deleteStalePolicies(networkPolicyList, desiredObjectMetaKeys)

	return reconcile.Result{}, flow.Parallel(append(reconcileTaskFns, deleteTaskFns...)...)(ctx)
}

func (r *Reconciler) namespaceIsHandled(ctx context.Context, namespaceName string) (bool, error) {
	namespace := &metav1.PartialObjectMetadata{}
	namespace.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Namespace"))
	if err := r.TargetClient.Get(ctx, client.ObjectKey{Name: namespaceName}, namespace); err != nil {
		return false, fmt.Errorf("failed to get namespace %q: %w", namespaceName, err)
	}

	if len(r.selectors) == 0 {
		return true, nil
	}

	for _, selector := range r.selectors {
		if selector.Matches(labels.Set(namespace.GetLabels())) {
			return true, nil
		}
	}

	return false, nil
}

func (r *Reconciler) fetchRelevantNamespaceNames(ctx context.Context, service *corev1.Service) (sets.Set[string], error) {
	var namespaceSelectors []metav1.LabelSelector
	if v, ok := service.Annotations[resourcesv1alpha1.NetworkingNamespaceSelectors]; ok {
		if err := json.Unmarshal([]byte(v), &namespaceSelectors); err != nil {
			return nil, fmt.Errorf("failed unmarshaling %s: %w", v, err)
		}
	}

	namespaceNames := sets.New[string](service.Namespace)

	for _, n := range namespaceSelectors {
		namespaceSelector := n

		selector, err := metav1.LabelSelectorAsSelector(&namespaceSelector)
		if err != nil {
			return nil, fmt.Errorf("failed parsing %s to labels.Selector: %w", namespaceSelector, err)
		}

		namespaceList := &metav1.PartialObjectMetadataList{}
		namespaceList.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("NamespaceList"))
		if err := r.TargetClient.List(ctx, namespaceList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
			return nil, fmt.Errorf("failed listing namespaces with selector %s: %w", selector.String(), err)
		}

		for _, namespace := range namespaceList.Items {
			if namespace.DeletionTimestamp == nil {
				namespaceNames.Insert(namespace.Name)
			}
		}
	}

	return namespaceNames, nil
}

func (r *Reconciler) reconcileDesiredPolicies(service *corev1.Service, namespaceNames sets.Set[string]) ([]flow.TaskFn, []string, error) {
	var (
		taskFns               []flow.TaskFn
		desiredObjectMetaKeys []string

		addTasksForRelevantNamespacesAndPort = func(port networkingv1.NetworkPolicyPort, customPodLabelSelector string) {
			policyID := policyIDFor(service.Name, port)
			podLabelSelector := policyID

			if customPodLabelSelector != "" {
				policyID += "-via-" + customPodLabelSelector
				podLabelSelector = customPodLabelSelector
			}

			for _, n := range namespaceNames.UnsortedList() {
				namespaceName := n
				matchLabels := matchLabelsForServiceAndNamespace(podLabelSelector, service, namespaceName)

				for _, fns := range []struct {
					objectMetaFunc func(string, string, string) metav1.ObjectMeta
					reconcileFunc  func(context.Context, *corev1.Service, networkingv1.NetworkPolicyPort, metav1.ObjectMeta, string, map[string]string) error
				}{
					{objectMetaFunc: ingressPolicyObjectMetaFor, reconcileFunc: r.reconcileIngressPolicy},
					{objectMetaFunc: egressPolicyObjectMetaFor, reconcileFunc: r.reconcileEgressPolicy},
				} {
					reconcileFn := fns.reconcileFunc
					objectMeta := fns.objectMetaFunc(policyID, service.Namespace, namespaceName)
					desiredObjectMetaKeys = append(desiredObjectMetaKeys, key(objectMeta))

					taskFns = append(taskFns, func(ctx context.Context) error {
						return reconcileFn(ctx, service, port, objectMeta, namespaceName, matchLabels)
					})
				}
			}
		}
	)

	for _, p := range service.Spec.Ports {
		port := p
		addTasksForRelevantNamespacesAndPort(networkingv1.NetworkPolicyPort{Protocol: &port.Protocol, Port: &port.TargetPort}, "")
	}

	if customPodLabelSelector, allowedPorts := service.Annotations[resourcesv1alpha1.NetworkingFromPolicyPodLabelSelector], service.Annotations[resourcesv1alpha1.NetworkingFromPolicyAllowedPorts]; customPodLabelSelector != "" && allowedPorts != "" {
		var ports []networkingv1.NetworkPolicyPort
		if err := json.Unmarshal([]byte(allowedPorts), &ports); err != nil {
			return nil, nil, fmt.Errorf("failed unmarshaling %s: %w", allowedPorts, err)
		}

		for _, port := range ports {
			addTasksForRelevantNamespacesAndPort(port, customPodLabelSelector)
		}
	}

	if _, ok := service.Annotations[resourcesv1alpha1.NetworkingFromWorldToPorts]; ok {
		objectMeta := metav1.ObjectMeta{Name: "ingress-to-" + service.Name + "-from-world", Namespace: service.Namespace}
		desiredObjectMetaKeys = append(desiredObjectMetaKeys, key(objectMeta))
		taskFns = append(taskFns, func(ctx context.Context) error {
			return r.reconcileIngressFromWorldPolicy(ctx, service, objectMeta)
		})
	}

	return taskFns, desiredObjectMetaKeys, nil
}

func (r *Reconciler) deleteStalePolicies(networkPolicyList *metav1.PartialObjectMetadataList, desiredObjectMetaKeys []string) []flow.TaskFn {
	objectMetaKeysForDesiredPolicies := make(map[string]struct{})
	for _, objectMetaKey := range desiredObjectMetaKeys {
		objectMetaKeysForDesiredPolicies[objectMetaKey] = struct{}{}
	}

	var taskFns []flow.TaskFn

	for _, n := range networkPolicyList.Items {
		networkPolicy := n

		if _, ok := objectMetaKeysForDesiredPolicies[key(networkPolicy.ObjectMeta)]; !ok {
			taskFns = append(taskFns, func(ctx context.Context) error {
				return kubernetesutils.DeleteObject(ctx, r.TargetClient, &networkPolicy)
			})
		}
	}

	return taskFns
}

func (r *Reconciler) reconcileIngressPolicy(
	ctx context.Context,
	service *corev1.Service,
	port networkingv1.NetworkPolicyPort,
	networkPolicyObjectMeta metav1.ObjectMeta,
	namespaceName string,
	matchLabels map[string]string,
) error {
	networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: networkPolicyObjectMeta}
	_, err := controllerutils.GetAndCreateOrMergePatch(ctx, r.TargetClient, networkPolicy, func() error {
		metav1.SetMetaDataLabel(&networkPolicy.ObjectMeta, resourcesv1alpha1.NetworkingServiceName, service.Name)
		metav1.SetMetaDataLabel(&networkPolicy.ObjectMeta, resourcesv1alpha1.NetworkingServiceNamespace, service.Namespace)

		metav1.SetMetaDataAnnotation(&networkPolicy.ObjectMeta, v1beta1constants.GardenerDescription, fmt.Sprintf("Allows "+
			"ingress %s traffic to port %s for pods selected by the %s service selector from pods running in namespace %s labeled "+
			"with %s.", *port.Protocol, port.Port.String(), client.ObjectKeyFromObject(service), namespaceName, matchLabels))

		networkPolicy.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{
			From: []networkingv1.NetworkPolicyPeer{{
				PodSelector:       &metav1.LabelSelector{MatchLabels: matchLabels},
				NamespaceSelector: ingressNamespaceSelectorFor(service.Namespace, namespaceName),
			}},
			Ports: []networkingv1.NetworkPolicyPort{port},
		}}
		networkPolicy.Spec.Egress = nil
		networkPolicy.Spec.PodSelector = metav1.LabelSelector{MatchLabels: service.Spec.Selector}
		networkPolicy.Spec.PolicyTypes = []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}

		return nil
	})
	return err
}

func (r *Reconciler) reconcileEgressPolicy(
	ctx context.Context,
	service *corev1.Service,
	port networkingv1.NetworkPolicyPort,
	networkPolicyObjectMeta metav1.ObjectMeta,
	namespaceName string,
	matchLabels map[string]string,
) error {
	networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: networkPolicyObjectMeta}

	_, err := controllerutils.GetAndCreateOrMergePatch(ctx, r.TargetClient, networkPolicy, func() error {
		metav1.SetMetaDataLabel(&networkPolicy.ObjectMeta, resourcesv1alpha1.NetworkingServiceName, service.Name)
		metav1.SetMetaDataLabel(&networkPolicy.ObjectMeta, resourcesv1alpha1.NetworkingServiceNamespace, service.Namespace)

		metav1.SetMetaDataAnnotation(&networkPolicy.ObjectMeta, v1beta1constants.GardenerDescription, fmt.Sprintf("Allows "+
			"egress %s traffic to port %s from pods running in namespace %s labeled with %s to pods selected by the %s service "+
			"selector.", *port.Protocol, port.Port.String(), namespaceName, matchLabels, client.ObjectKeyFromObject(service)))

		networkPolicy.Spec.Ingress = nil
		networkPolicy.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{
			To: []networkingv1.NetworkPolicyPeer{{
				PodSelector:       &metav1.LabelSelector{MatchLabels: service.Spec.Selector},
				NamespaceSelector: egressNamespaceSelectorFor(service.Namespace, namespaceName),
			}},
			Ports: []networkingv1.NetworkPolicyPort{port},
		}}
		networkPolicy.Spec.PodSelector = metav1.LabelSelector{MatchLabels: matchLabels}
		networkPolicy.Spec.PolicyTypes = []networkingv1.PolicyType{networkingv1.PolicyTypeEgress}

		return nil
	})

	return err
}

func (r *Reconciler) reconcileIngressFromWorldPolicy(ctx context.Context, service *corev1.Service, networkPolicyObjectMeta metav1.ObjectMeta) error {
	var ports []networkingv1.NetworkPolicyPort
	if err := json.Unmarshal([]byte(service.Annotations[resourcesv1alpha1.NetworkingFromWorldToPorts]), &ports); err != nil {
		return fmt.Errorf("failed unmarshaling %s: %w", service.Annotations[resourcesv1alpha1.NetworkingFromWorldToPorts], err)
	}

	networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: networkPolicyObjectMeta}
	_, err := controllerutils.GetAndCreateOrMergePatch(ctx, r.TargetClient, networkPolicy, func() error {
		metav1.SetMetaDataLabel(&networkPolicy.ObjectMeta, resourcesv1alpha1.NetworkingServiceName, service.Name)
		metav1.SetMetaDataLabel(&networkPolicy.ObjectMeta, resourcesv1alpha1.NetworkingServiceNamespace, service.Namespace)

		metav1.SetMetaDataAnnotation(&networkPolicy.ObjectMeta, v1beta1constants.GardenerDescription, fmt.Sprintf("Allows "+
			"ingress traffic from everywhere to ports %v for pods selected by the %s service selector.", ports,
			client.ObjectKeyFromObject(service)))

		networkPolicy.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{
			From: []networkingv1.NetworkPolicyPeer{
				{PodSelector: &metav1.LabelSelector{}, NamespaceSelector: &metav1.LabelSelector{}},
				{IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0"}},
			},
			Ports: ports,
		}}
		networkPolicy.Spec.Egress = nil
		networkPolicy.Spec.PodSelector = metav1.LabelSelector{MatchLabels: service.Spec.Selector}
		networkPolicy.Spec.PolicyTypes = []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}

		return nil
	})
	return err
}

func policyIDFor(serviceName string, port networkingv1.NetworkPolicyPort) string {
	return fmt.Sprintf("%s-%s-%s", serviceName, strings.ToLower(string(*port.Protocol)), port.Port.String())
}

func matchLabelsForServiceAndNamespace(podLabelSelector string, service *corev1.Service, namespaceName string) map[string]string {
	var infix string

	if service.Namespace != namespaceName {
		infix = service.Namespace

		if namespaceAlias, ok := service.Annotations[resourcesv1alpha1.NetworkingPodLabelSelectorNamespaceAlias]; ok {
			infix = namespaceAlias
		}

		infix += "-"
	}

	return map[string]string{"networking.resources.gardener.cloud/to-" + infix + podLabelSelector: v1beta1constants.LabelNetworkPolicyAllowed}
}

func ingressPolicyObjectMetaFor(policyID, serviceNamespace, namespaceName string) metav1.ObjectMeta {
	name := "ingress-to-" + policyID
	if serviceNamespace != namespaceName {
		name += "-from-" + namespaceName
	}

	return metav1.ObjectMeta{Name: name, Namespace: serviceNamespace}
}

func egressPolicyObjectMetaFor(policyID, serviceNamespace, namespaceName string) metav1.ObjectMeta {
	name := "egress-to-" + policyID
	if serviceNamespace != namespaceName {
		name = "egress-to-" + serviceNamespace + "-" + policyID
	}

	return metav1.ObjectMeta{Name: name, Namespace: namespaceName}
}

func ingressNamespaceSelectorFor(serviceNamespace, namespaceName string) *metav1.LabelSelector {
	if serviceNamespace == namespaceName {
		return nil
	}

	return &metav1.LabelSelector{MatchLabels: map[string]string{corev1.LabelMetadataName: namespaceName}}
}

func egressNamespaceSelectorFor(serviceNamespace, namespaceName string) *metav1.LabelSelector {
	if serviceNamespace == namespaceName {
		return nil
	}

	return &metav1.LabelSelector{MatchLabels: map[string]string{corev1.LabelMetadataName: serviceNamespace}}
}

func key(meta metav1.ObjectMeta) string {
	return meta.Namespace + "/" + meta.Name
}
