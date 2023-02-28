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

package app

import (
	"context"
	"fmt"
	"os"
	goruntime "runtime"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/component-base/version"
	"k8s.io/component-base/version/verflag"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerconfigv1alpha1 "sigs.k8s.io/controller-runtime/pkg/config/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	"github.com/gardener/gardener/extensions/pkg/webhook/certificates"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/controllerutils/routes"
	gardenerhealthz "github.com/gardener/gardener/pkg/healthz"
	"github.com/gardener/gardener/pkg/logger"
	"github.com/gardener/gardener/pkg/operator/apis/config"
	operatorclient "github.com/gardener/gardener/pkg/operator/client"
	"github.com/gardener/gardener/pkg/operator/controller"
	operatorfeatures "github.com/gardener/gardener/pkg/operator/features"
	"github.com/gardener/gardener/pkg/operator/webhook"
)

// Name is a const for the name of this component.
const Name = "gardener-operator"

// NewCommand creates a new cobra.Command for running gardener-operator.
func NewCommand() *cobra.Command {
	opts := &options{}

	cmd := &cobra.Command{
		Use:   Name,
		Short: "Launch the " + Name,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			verflag.PrintAndExitIfRequested()

			if err := opts.complete(); err != nil {
				return err
			}
			if err := opts.validate(); err != nil {
				return err
			}

			log, err := logger.NewZapLogger(opts.config.LogLevel, opts.config.LogFormat)
			if err != nil {
				return fmt.Errorf("error instantiating zap logger: %w", err)
			}

			logf.SetLogger(log)
			klog.SetLogger(log)

			log.Info("Starting "+Name, "version", version.Get())
			cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				log.Info(fmt.Sprintf("FLAG: --%s=%s", flag.Name, flag.Value)) //nolint:logcheck
			})

			// don't output usage on further errors raised during execution
			cmd.SilenceUsage = true
			// further errors will be logged properly, don't duplicate
			cmd.SilenceErrors = true

			return run(cmd.Context(), log, opts.config)
		},
	}

	flags := cmd.Flags()
	verflag.AddFlags(flags)
	opts.addFlags(flags)

	return cmd
}

func run(ctx context.Context, log logr.Logger, cfg *config.OperatorConfiguration) error {
	// Add feature flags
	if err := operatorfeatures.FeatureGate.SetFromMap(cfg.FeatureGates); err != nil {
		return err
	}
	log.Info("Feature Gates", "featureGates", operatorfeatures.FeatureGate.String())

	log.Info("Getting rest config")
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		cfg.RuntimeClientConnection.Kubeconfig = kubeconfig
	}

	restConfig, err := kubernetes.RESTConfigFromClientConnectionConfiguration(&cfg.RuntimeClientConnection, nil, kubernetes.AuthTokenFile)
	if err != nil {
		return err
	}

	log.Info("Setting up manager")
	mgr, err := manager.New(restConfig, manager.Options{
		Logger:                  log,
		Scheme:                  operatorclient.RuntimeScheme,
		Namespace:               v1beta1constants.GardenNamespace,
		GracefulShutdownTimeout: pointer.Duration(5 * time.Second),

		Host:                   cfg.Server.Webhooks.BindAddress,
		Port:                   cfg.Server.Webhooks.Port,
		CertDir:                "/tmp/gardener-operator-cert",
		HealthProbeBindAddress: fmt.Sprintf("%s:%d", cfg.Server.HealthProbes.BindAddress, cfg.Server.HealthProbes.Port),
		MetricsBindAddress:     fmt.Sprintf("%s:%d", cfg.Server.Metrics.BindAddress, cfg.Server.Metrics.Port),

		LeaderElection:                cfg.LeaderElection.LeaderElect,
		LeaderElectionResourceLock:    cfg.LeaderElection.ResourceLock,
		LeaderElectionID:              cfg.LeaderElection.ResourceName,
		LeaderElectionNamespace:       cfg.LeaderElection.ResourceNamespace,
		LeaderElectionReleaseOnCancel: true,
		LeaseDuration:                 &cfg.LeaderElection.LeaseDuration.Duration,
		RenewDeadline:                 &cfg.LeaderElection.RenewDeadline.Duration,
		RetryPeriod:                   &cfg.LeaderElection.RetryPeriod.Duration,
		Controller: controllerconfigv1alpha1.ControllerConfigurationSpec{
			RecoverPanic: pointer.Bool(true),
		},
	})
	if err != nil {
		return err
	}

	if cfg.Debugging != nil && cfg.Debugging.EnableProfiling {
		if err := (routes.Profiling{}).AddToManager(mgr); err != nil {
			return fmt.Errorf("failed adding profiling handlers to manager: %w", err)
		}
		if cfg.Debugging.EnableContentionProfiling {
			goruntime.SetBlockProfileRate(1)
		}
	}

	log.Info("Setting up health check endpoints")
	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("informer-sync", gardenerhealthz.NewCacheSyncHealthz(mgr.GetCache())); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("webhook-server", mgr.GetWebhookServer().StartedChecker()); err != nil {
		return err
	}

	log.Info("Adding certificate management to manager")
	mode, url := extensionswebhook.ModeService, os.Getenv("WEBHOOK_URL")
	if v := os.Getenv("WEBHOOK_MODE"); v != "" {
		mode = v
	}
	validatingWebhookConfiguration := webhook.GetValidatingWebhookConfiguration(mode, url)

	if err := certificates.AddCertificateManagementToManager(
		ctx,
		mgr,
		clock.RealClock{},
		validatingWebhookConfiguration,
		nil,
		nil,
		nil,
		"",
		Name,
		v1beta1constants.GardenNamespace,
		mode,
		url,
	); err != nil {
		return fmt.Errorf("failed adding webhook certificate management to manager: %w", err)
	}

	log.Info("Adding runnables to manager")
	if err := mgr.Add(&controllerutils.ControlledRunner{
		Manager: mgr,
		BootstrapRunnables: []manager.Runnable{
			reconcileValidatingWebhookConfiguration(ctx, mgr, validatingWebhookConfiguration),
		},
		ActualRunnables: []manager.Runnable{
			manager.RunnableFunc(func(context.Context) error { return webhook.AddToManager(mgr) }),
			manager.RunnableFunc(func(context.Context) error { return controller.AddToManager(mgr, cfg) }),
		},
	}); err != nil {
		return err
	}

	log.Info("Starting manager")
	return mgr.Start(ctx)
}

func reconcileValidatingWebhookConfiguration(ctx context.Context, mgr manager.Manager, validatingWebhookConfiguration *admissionregistrationv1.ValidatingWebhookConfiguration) manager.RunnableFunc {
	return func(context.Context) error {
		mgr.GetLogger().Info("Reconciling webhook configuration", "validatingWebhookConfiguration", client.ObjectKeyFromObject(validatingWebhookConfiguration))

		obj := &admissionregistrationv1.ValidatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: validatingWebhookConfiguration.Name}}
		_, err := controllerutils.CreateOrGetAndStrategicMergePatch(ctx, mgr.GetClient(), obj, func() error {
			obj.Webhooks = validatingWebhookConfiguration.Webhooks
			return nil
		})
		validatingWebhookConfiguration = obj
		return err
	}
}
