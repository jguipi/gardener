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

package cloudprovider_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	"github.com/gardener/gardener/extensions/pkg/webhook/cloudprovider"
	extensionscmdwebhook "github.com/gardener/gardener/extensions/pkg/webhook/cmd"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/logger"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	testcloudprovider "github.com/gardener/gardener/test/integration/extensions/webhook/cloudprovider"
)

func TestCloudProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Test Integration Extensions Webhook CloudProvider Suite")
}

const (
	testID       = "extensions-webhook-cloudprovider-test"
	providerName = "test-provider"
)

var (
	ctx = context.TODO()
	log logr.Logger

	restConfig *rest.Config
	testEnv    *envtest.Environment
	testClient client.Client

	cluster       *extensionsv1alpha1.Cluster
	testNamespace *corev1.Namespace
)

var _ = BeforeSuite(func() {
	logf.SetLogger(logger.MustNewZapLogger(logger.DebugLevel, logger.FormatJSON, zap.WriteTo(GinkgoWriter)))
	log = logf.Log.WithName(testID)

	By("Start test environment")
	testEnv = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "..", "..", "..", "example", "seed-crds", "10-crd-extensions.gardener.cloud_clusters.yaml"),
			},
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	restConfig, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(restConfig).NotTo(BeNil())

	DeferCleanup(func() {
		By("Stop test environment")
		Expect(testEnv.Stop()).To(Succeed())
	})

	By("Create test client")
	testClient, err = client.New(restConfig, client.Options{Scheme: kubernetes.SeedScheme})
	Expect(err).NotTo(HaveOccurred())

	By("Create test Namespace")
	testNamespace = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			// create dedicated namespace for each test run, so that we can run multiple tests concurrently for stress tests
			GenerateName: testID + "-",
			Labels: map[string]string{
				v1beta1constants.LabelShootProvider: providerName,
			},
		},
	}
	Expect(testClient.Create(ctx, testNamespace)).To(Succeed())
	log.Info("Created Namespace for test", "namespaceName", testNamespace.Name)

	DeferCleanup(func() {
		By("Delete test Namespace")
		Expect(testClient.Delete(ctx, testNamespace)).To(Or(Succeed(), BeNotFoundError()))
	})

	By("Setup manager")
	mgr, err := manager.New(restConfig, manager.Options{
		Port:               testEnv.WebhookInstallOptions.LocalServingPort,
		Host:               testEnv.WebhookInstallOptions.LocalServingHost,
		CertDir:            testEnv.WebhookInstallOptions.LocalServingCertDir,
		Scheme:             kubernetes.SeedScheme,
		MetricsBindAddress: "0",
		Namespace:          testNamespace.Name,
	})
	Expect(err).NotTo(HaveOccurred())

	By("Register webhook")
	Expect(addTestWebhookToManager(mgr)).To(Succeed())

	By("Start manager")
	mgrContext, mgrCancel := context.WithCancel(ctx)

	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(mgrContext)).To(Succeed())
	}()

	DeferCleanup(func() {
		By("Stop manager")
		mgrCancel()
	})

	By("Create Cluster")
	cluster = &extensionsv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace.Name,
		},
		Spec: extensionsv1alpha1.ClusterSpec{
			CloudProfile: runtime.RawExtension{Raw: []byte("{}")},
			Seed:         runtime.RawExtension{Raw: []byte("{}")},
			Shoot:        runtime.RawExtension{Raw: []byte("{}")},
		},
	}

	Expect(testClient.Create(ctx, cluster)).To(Succeed())
	log.Info("Created Cluster for test", "cluster", client.ObjectKeyFromObject(cluster))

	DeferCleanup(func() {
		By("Delete Cluster")
		Expect(client.IgnoreNotFound(testClient.Delete(ctx, cluster))).To(Succeed())
	})
})

func addTestWebhookToManager(mgr manager.Manager) error {
	switchOptions := extensionscmdwebhook.NewSwitchOptions(
		extensionscmdwebhook.Switch("cloudprovider", func(mgr manager.Manager) (*extensionswebhook.Webhook, error) {
			return cloudprovider.New(mgr, cloudprovider.Args{
				Provider:             providerName,
				Mutator:              cloudprovider.NewMutator(log, testcloudprovider.NewEnsurer(log)),
				EnableObjectSelector: true,
			})
		}),
	)

	addToManagerOptions := extensionscmdwebhook.NewAddToManagerOptions(providerName, "", nil, &extensionscmdwebhook.ServerOptions{
		Mode: extensionswebhook.ModeURL,
		URL:  fmt.Sprintf("%s:%d", testEnv.WebhookInstallOptions.LocalServingHost, testEnv.WebhookInstallOptions.LocalServingPort),
	}, switchOptions)

	if err := addToManagerOptions.Complete(); err != nil {
		return err
	}

	_, err := addToManagerOptions.Completed().AddToManager(ctx, mgr)
	return err
}
