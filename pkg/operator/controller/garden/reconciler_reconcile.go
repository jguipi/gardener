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

package garden

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	operatorv1alpha1 "github.com/gardener/gardener/pkg/apis/operator/v1alpha1"
	"github.com/gardener/gardener/pkg/apis/operator/v1alpha1/helper"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/controllerutils"
	gardenletconfig "github.com/gardener/gardener/pkg/gardenlet/apis/config"
	"github.com/gardener/gardener/pkg/operation/botanist/component"
	"github.com/gardener/gardener/pkg/operation/botanist/component/etcd"
	"github.com/gardener/gardener/pkg/operation/botanist/component/hvpa"
	"github.com/gardener/gardener/pkg/operation/botanist/component/vpa"
	"github.com/gardener/gardener/pkg/utils/flow"
	secretsmanager "github.com/gardener/gardener/pkg/utils/secrets/manager"
	"github.com/gardener/gardener/pkg/utils/timewindow"
)

func (r *Reconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	garden *operatorv1alpha1.Garden,
	secretsManager secretsmanager.Interface,
) (
	reconcile.Result,
	error,
) {
	applier := kubernetes.NewApplier(r.RuntimeClient, r.RuntimeClient.RESTMapper())

	if !controllerutil.ContainsFinalizer(garden, finalizerName) {
		log.Info("Adding finalizer")
		if err := controllerutils.AddFinalizers(ctx, r.RuntimeClient, garden, finalizerName); err != nil {
			return reconcile.Result{}, err
		}
	}

	// VPA is a prerequisite. If it's enabled then we deploy the CRD (and later also the related components) as part of
	// the flow. However, when it's disabled then we check whether it is indeed available (and fail, otherwise).
	if !vpaEnabled(garden.Spec.RuntimeCluster.Settings) {
		if _, err := r.RuntimeClient.RESTMapper().RESTMapping(schema.GroupKind{Group: "autoscaling.k8s.io", Kind: "VerticalPodAutoscaler"}); err != nil {
			return reconcile.Result{}, fmt.Errorf("VPA is required for runtime cluster but CRD is not installed: %s", err)
		}
	}

	// create + label namespace
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: r.GardenNamespace}}
	log.Info("Labeling and annotating namespace", "namespaceName", namespace.Name)
	if _, err := controllerutils.CreateOrGetAndMergePatch(ctx, r.RuntimeClient, namespace, func() error {
		metav1.SetMetaDataLabel(&namespace.ObjectMeta, resourcesv1alpha1.HighAvailabilityConfigConsider, "true")
		metav1.SetMetaDataAnnotation(&namespace.ObjectMeta, resourcesv1alpha1.HighAvailabilityConfigZones, strings.Join(garden.Spec.RuntimeCluster.Provider.Zones, ","))
		return nil
	}); err != nil {
		return reconcile.Result{}, err
	}

	log.Info("Generating CA certificates for runtime and virtual clusters")
	for _, config := range caCertConfigurations() {
		if _, err := secretsManager.Generate(ctx, config, caCertGenerateOptionsFor(config.GetName(), helper.GetCARotationPhase(garden.Status.Credentials))...); err != nil {
			return reconcile.Result{}, err
		}
	}

	log.Info("Instantiating component deployers")
	// garden system components
	vpaCRD := vpa.NewCRD(applier, nil)
	hvpaCRD := hvpa.NewCRD(applier)
	if !hvpaEnabled() {
		hvpaCRD = component.OpDestroy(hvpaCRD)
	}
	gardenerResourceManager, err := r.newGardenerResourceManager(garden, secretsManager)
	if err != nil {
		return reconcile.Result{}, err
	}
	system := r.newSystem()
	verticalPodAutoscaler, err := r.newVerticalPodAutoscaler(garden, secretsManager)
	if err != nil {
		return reconcile.Result{}, err
	}
	hvpaController, err := r.newHVPA()
	if err != nil {
		return reconcile.Result{}, err
	}
	etcdDruid, err := r.newEtcdDruid()
	if err != nil {
		return reconcile.Result{}, err
	}

	// virtual garden control plane components
	etcdMain, err := r.newEtcd(log, garden, secretsManager, v1beta1constants.ETCDRoleMain, etcd.ClassImportant)
	if err != nil {
		return reconcile.Result{}, err
	}
	etcdEvents, err := r.newEtcd(log, garden, secretsManager, v1beta1constants.ETCDRoleEvents, etcd.ClassNormal)
	if err != nil {
		return reconcile.Result{}, err
	}
	kubeAPIServerService := r.newKubeAPIServerService(log, garden)

	var (
		g            = flow.NewGraph("Garden reconciliation")
		deployVPACRD = g.Add(flow.Task{
			Name: "Deploying custom resource definition for VPA",
			Fn:   flow.TaskFn(vpaCRD.Deploy).DoIf(vpaEnabled(garden.Spec.RuntimeCluster.Settings)),
		})
		reconcileHVPACRD = g.Add(flow.Task{
			Name: "Reconciling custom resource definition for HVPA",
			Fn:   hvpaCRD.Deploy,
		})
		deployGardenerResourceManager = g.Add(flow.Task{
			Name:         "Deploying and waiting for gardener-resource-manager to be healthy",
			Fn:           component.OpWait(gardenerResourceManager).Deploy,
			Dependencies: flow.NewTaskIDs(deployVPACRD, reconcileHVPACRD),
		})
		deploySystemResources = g.Add(flow.Task{
			Name:         "Deploying system resources",
			Fn:           system.Deploy,
			Dependencies: flow.NewTaskIDs(deployGardenerResourceManager),
		})
		deployVPA = g.Add(flow.Task{
			Name:         "Deploying Kubernetes vertical pod autoscaler",
			Fn:           verticalPodAutoscaler.Deploy,
			Dependencies: flow.NewTaskIDs(deployGardenerResourceManager),
		})
		deployHVPA = g.Add(flow.Task{
			Name:         "Deploying HVPA controller",
			Fn:           hvpaController.Deploy,
			Dependencies: flow.NewTaskIDs(deployGardenerResourceManager),
		})
		deployEtcdDruid = g.Add(flow.Task{
			Name:         "Deploying ETCD Druid",
			Fn:           etcdDruid.Deploy,
			Dependencies: flow.NewTaskIDs(deployGardenerResourceManager),
		})
		syncPointSystemComponents = flow.NewTaskIDs(
			deploySystemResources,
			deployVPA,
			deployHVPA,
			deployEtcdDruid,
		)

		deployEtcds = g.Add(flow.Task{
			Name:         "Deploying main and events ETCDs of virtual garden",
			Fn:           r.deployEtcdsFunc(garden, etcdMain, etcdEvents),
			Dependencies: flow.NewTaskIDs(syncPointSystemComponents),
		})
		_ = g.Add(flow.Task{
			Name:         "Waiting until main and event ETCDs report readiness",
			Fn:           flow.Parallel(etcdMain.Wait, etcdEvents.Wait),
			Dependencies: flow.NewTaskIDs(deployEtcds),
		})
		_ = g.Add(flow.Task{
			Name:         "Deploying and waiting for kube-apiserver service in the runtime cluster",
			Fn:           component.OpWait(kubeAPIServerService).Deploy,
			Dependencies: flow.NewTaskIDs(syncPointSystemComponents),
		})
	)

	if err := g.Compile().Run(ctx, flow.Opts{Log: log}); err != nil {
		return reconcile.Result{}, flow.Errors(err)
	}

	return reconcile.Result{}, secretsManager.Cleanup(ctx)
}

func (r *Reconciler) deployEtcdsFunc(garden *operatorv1alpha1.Garden, etcdMain, etcdEvents etcd.Interface) func(context.Context) error {
	return func(ctx context.Context) error {
		if etcdConfig := garden.Spec.VirtualCluster.ETCD; etcdConfig != nil && etcdConfig.Main != nil && etcdConfig.Main.Backup != nil {
			snapshotSchedule, err := timewindow.DetermineSchedule(
				"%d %d * * *",
				garden.Spec.VirtualCluster.Maintenance.TimeWindow.Begin,
				garden.Spec.VirtualCluster.Maintenance.TimeWindow.End,
				garden.UID,
				garden.CreationTimestamp,
				timewindow.RandomizeWithinFirstHourOfTimeWindow,
			)
			if err != nil {
				return err
			}

			var backupLeaderElection *gardenletconfig.ETCDBackupLeaderElection
			if r.Config.Controllers.Garden.ETCDConfig != nil {
				backupLeaderElection = r.Config.Controllers.Garden.ETCDConfig.BackupLeaderElection
			}

			container, prefix := etcdConfig.Main.Backup.BucketName, "virtual-garden-etcd-main"
			if idx := strings.Index(etcdConfig.Main.Backup.BucketName, "/"); idx != -1 {
				container = etcdConfig.Main.Backup.BucketName[:idx]
				prefix = fmt.Sprintf("%s/%s", strings.TrimSuffix(etcdConfig.Main.Backup.BucketName[idx+1:], "/"), prefix)
			}

			etcdMain.SetBackupConfig(&etcd.BackupConfig{
				Provider:             etcdConfig.Main.Backup.Provider,
				SecretRefName:        etcdConfig.Main.Backup.SecretRef.Name,
				Container:            container,
				Prefix:               prefix,
				FullSnapshotSchedule: snapshotSchedule,
				LeaderElection:       backupLeaderElection,
			})
		}

		// Roll out the new peer CA first so that every member in the cluster trusts the old and the new CA.
		// This is required because peer certificates which are used for client and server authentication at the same time,
		// are re-created with the new CA in the `Deploy` step.
		if helper.GetCARotationPhase(garden.Status.Credentials) == gardencorev1beta1.RotationPreparing {
			if err := flow.Sequential(
				flow.Parallel(etcdMain.RolloutPeerCA, etcdEvents.RolloutPeerCA),
				flow.Parallel(etcdMain.Wait, etcdEvents.Wait),
			)(ctx); err != nil {
				return err
			}
		}

		// Deploy NetworkPolicy allowing ETCD to talk to the runtime cluster's API server.
		// TODO(rfranzke): Remove this in the future when the network policy deployment has been refactored.
		networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "etcd-to-world", Namespace: r.GardenNamespace}}
		if _, err := controllerutils.GetAndCreateOrMergePatch(ctx, r.RuntimeClient, networkPolicy, func() error {
			networkPolicy.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{
				To: []networkingv1.NetworkPolicyPeer{{
					IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0"},
				}},
			}}
			networkPolicy.Spec.PodSelector = metav1.LabelSelector{MatchLabels: etcd.GetLabels()}
			networkPolicy.Spec.PolicyTypes = []networkingv1.PolicyType{networkingv1.PolicyTypeEgress}
			return nil
		}); err != nil {
			return err
		}

		return flow.Parallel(etcdMain.Deploy, etcdEvents.Deploy)(ctx)
	}
}
