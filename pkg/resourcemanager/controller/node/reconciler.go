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

package node

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/gardener/pkg/api/indexer"
	"github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/resourcemanager/apis/config"
	"github.com/gardener/gardener/pkg/resourcemanager/controller/node/helper"
	"github.com/gardener/gardener/pkg/utils/kubernetes/health"
)

// Reconciler manages taints on new Node objects to block scheduling of user workload pods until all node critical
// components are ready.
type Reconciler struct {
	TargetClient client.Client
	Config       config.NodeControllerConfig
	Recorder     record.EventRecorder
}

// Reconcile checks if the critical components not ready taint can be removed from the Node object.
func (r *Reconciler) Reconcile(reconcileCtx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(reconcileCtx)

	ctx, cancel := context.WithTimeout(reconcileCtx, time.Minute)
	defer cancel()

	node := &corev1.Node{}
	if err := r.TargetClient.Get(ctx, req.NamespacedName, node); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	// Predicates only filter watch events but don't filter when an object (or rather a reconcile.Request) is already in
	// the queue. Though, some other party might remove the taint while the controller is in backoff.
	// Hence, we should always check whether there is work left to do in the controller in addition to predicates.
	if !NodeHasCriticalComponentsNotReadyTaint(node) {
		return reconcile.Result{}, nil
	}

	// prep for checks: list all DaemonSets and all node-critical pods on the given node
	daemonSetList := &appsv1.DaemonSetList{}
	if err := r.TargetClient.List(ctx, daemonSetList, client.MatchingLabels{v1beta1constants.LabelNodeCriticalComponent: "true"}); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed listing node-critical DaemonSets on node: %w", err)
	}

	podList := &corev1.PodList{}
	if err := r.TargetClient.List(ctx, podList, client.MatchingFields{indexer.PodNodeName: node.Name}, client.MatchingLabels{v1beta1constants.LabelNodeCriticalComponent: "true"}); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed listing node-critical Pods on node: %w", err)
	}

	var (
		requiredDrivers = GetRequiredDrivers(podList.Items)
		existingDrivers sets.Set[string]
	)

	// getting the CSINode object and checking for existing drivers is only
	// necessary if at least one driver is required by the pods.
	if len(requiredDrivers) >= 1 {
		var err error
		existingDrivers, err = GetExistingDriversFromCSINode(ctx, r.TargetClient, client.ObjectKeyFromObject(node))
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed getting existing drivers from CSINode object for node: %w", err)
		}
	}

	// - for all node-critical DaemonSets: check whether a daemon pod has already been scheduled to the node
	// - for all scheduled node-critical Pods on the node: check their readiness
	// - for all drivers required by csi-driver-node pods: check if they exist
	if !(AllNodeCriticalDaemonPodsAreScheduled(log, r.Recorder, node, daemonSetList.Items, podList.Items) &&
		AllNodeCriticalPodsAreReady(log, r.Recorder, node, podList.Items) &&
		AllCSINodeDriversAreReady(log, r.Recorder, node, requiredDrivers, existingDrivers)) {
		backoff := r.Config.Backoff.Duration
		log.V(1).Info("Checking node again after backoff", "backoff", backoff)
		return reconcile.Result{RequeueAfter: backoff}, nil
	}

	log.Info("All node-critical components got ready, removing taint")
	r.Recorder.Event(node, corev1.EventTypeNormal, "NodeCriticalComponentsReady", "All node-critical components got ready, removing taint")
	return reconcile.Result{}, RemoveTaint(ctx, r.TargetClient, node)
}

var daemonSetGVK = appsv1.SchemeGroupVersion.WithKind("DaemonSet")

// AllNodeCriticalDaemonPodsAreScheduled returns true if all node-critical DaemonSets that should be scheduled to the
// given node have been scheduled. It uses ownerReferences of the given node-critical pods on the node for this check.
func AllNodeCriticalDaemonPodsAreScheduled(log logr.Logger, recorder record.EventRecorder, node *corev1.Node, daemonSets []appsv1.DaemonSet, nodeCriticalPods []corev1.Pod) bool {
	// collect a set of all scheduled DaemonSets on the node
	scheduledDaemonSets := sets.New[types.UID]()
	for _, pod := range nodeCriticalPods {
		controllerRef := metav1.GetControllerOf(&pod)
		if controllerRef == nil || schema.FromAPIVersionAndKind(controllerRef.APIVersion, controllerRef.Kind) != daemonSetGVK {
			continue
		}

		scheduledDaemonSets.Insert(controllerRef.UID)
	}

	// filter for DaemonSets that were not scheduled to the node yet
	var unscheduledDaemonSets []client.ObjectKey
	for _, daemonSet := range daemonSets {
		if daemonSet.Spec.Template.ObjectMeta.Labels[v1beta1constants.LabelNodeCriticalComponent] != "true" {
			continue
		}

		// determine whether DaemonSet needs to be scheduled to the node at all
		if shouldRun, _ := helper.NodeShouldRunDaemonPod(node, &daemonSet); !shouldRun {
			continue
		}

		// check whether DaemonSet has corresponding daemon pod on the node
		key := client.ObjectKeyFromObject(&daemonSet)
		if !scheduledDaemonSets.Has(daemonSet.UID) {
			unscheduledDaemonSets = append(unscheduledDaemonSets, key)
		}
	}

	if len(unscheduledDaemonSets) > 0 {
		log.Info("Node-critical DaemonSets found that were not scheduled to Node yet", "daemonSets", unscheduledDaemonSets)
		recorder.Eventf(node, corev1.EventTypeWarning, "UnscheduledNodeCriticalDaemonSets", "Node-critical DaemonSets found that were not scheduled to Node yet: %s", objectKeysToString(unscheduledDaemonSets))
		return false
	}

	return true
}

// AllNodeCriticalPodsAreReady returns true if all the given pods are ready by checking their Ready conditions.
func AllNodeCriticalPodsAreReady(log logr.Logger, recorder record.EventRecorder, node *corev1.Node, nodeCriticalPods []corev1.Pod) bool {
	var unreadyPods []client.ObjectKey
	for _, pod := range nodeCriticalPods {
		if !health.IsPodReady(&pod) {
			unreadyPods = append(unreadyPods, client.ObjectKeyFromObject(&pod))
		}
	}

	if len(unreadyPods) > 0 {
		log.Info("Unready node-critical Pods found on Node", "pods", unreadyPods)
		recorder.Eventf(node, corev1.EventTypeWarning, "UnreadyNodeCriticalPods", "Unready node-critical Pods found on Node: %s", objectKeysToString(unreadyPods))
		return false
	}

	return true
}

// GetRequiredDrivers searches through the pods annotations, and returns a set
// of driver names if it finds annotations with the wait-for-csi-node prefix;
// otherwise it returns an empty set.
func GetRequiredDrivers(pods []corev1.Pod) sets.Set[string] {
	requiredDrivers := sets.Set[string]{}
	for _, pod := range pods {
		for key, value := range pod.Annotations {
			if strings.HasPrefix(key, constants.AnnotationPrefixWaitForCSINode) {
				requiredDrivers.Insert(value)
			}
		}
	}
	return requiredDrivers
}

// GetExistingDriversFromCSINode returns a set of all driver names that are
// present in the CSINode object. A non-existent CSINode object is not
// considered an error, an empty set of existing drivers is returned instead.
func GetExistingDriversFromCSINode(ctx context.Context, client client.Client, csiNodeName types.NamespacedName) (sets.Set[string], error) {
	existingDrivers := sets.Set[string]{}

	// per specification, Node and CSINode have the same name
	csiNode := &storagev1.CSINode{}
	if err := client.Get(ctx, csiNodeName, csiNode); err != nil {
		if apierrors.IsNotFound(err) {
			return existingDrivers, nil
		}
		return nil, err
	}

	for _, driver := range csiNode.Spec.Drivers {
		existingDrivers.Insert(driver.Name)
	}

	return existingDrivers, nil
}

// AllCSINodeDriversAreReady compares a set of required drivers (i.e. drivers
// that are specified by csi-driver-node pods) with a set of existing drivers
// (i.e. drivers for which the CSINode object had information stored in spec).
// Either set could be empty.
func AllCSINodeDriversAreReady(log logr.Logger, recorder record.EventRecorder, node *corev1.Node, requiredDrivers, existingDrivers sets.Set[string]) bool {
	unreadyDrivers := requiredDrivers.Difference(existingDrivers)
	if unreadyDrivers.Len() >= 1 {
		log.Info("Unready required CSI drivers for Node", "drivers", unreadyDrivers.UnsortedList())
		recorder.Eventf(node, corev1.EventTypeWarning, "UnreadyRequiredCSIDrivers", "Unready required CSI drivers for Node: %s", unreadyDrivers.UnsortedList())
	}
	return unreadyDrivers.Len() == 0
}

// RemoveTaint removes the taint managed by this controller from the given node object
func RemoveTaint(ctx context.Context, w client.Writer, node *corev1.Node) error {
	patch := client.MergeFromWithOptions(node.DeepCopy(), client.MergeFromWithOptimisticLock{})
	var newTaints []corev1.Taint
	for _, taint := range node.Spec.Taints {
		if taint.Key != v1beta1constants.TaintNodeCriticalComponentsNotReady {
			newTaints = append(newTaints, taint)
		}
	}
	node.Spec.Taints = newTaints

	// Always try to patch the node object even if we did not modify it.
	// Optimistic locking will cause the patch to fail if we operate on an old version of the object.
	return w.Patch(ctx, node, patch)
}

func objectKeysToString(objKeys []client.ObjectKey) string {
	var keys []string
	for _, objKey := range objKeys {
		keys = append(keys, objKey.String())
	}

	return strings.Join(keys, ", ")
}
