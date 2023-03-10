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

package shoot_test

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardencoreversionedclientset "github.com/gardener/gardener/pkg/client/core/clientset/versioned"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	gardenerenvtest "github.com/gardener/gardener/pkg/envtest"
	"github.com/gardener/gardener/pkg/logger"
	schedulerfeatures "github.com/gardener/gardener/pkg/scheduler/features"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
)

func TestShoot(t *testing.T) {
	schedulerfeatures.RegisterFeatureGates()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Test Integration Scheduler Shoot Suite")
}

const (
	testID       = "scheduler-test"
	providerType = "provider-type"
)

var (
	ctx = context.Background()
	log logr.Logger

	restConfig          *rest.Config
	testEnv             *gardenerenvtest.GardenerTestEnvironment
	testClient          client.Client
	versionedTestClient *gardencoreversionedclientset.Clientset

	testNamespace     *corev1.Namespace
	testSecretBinding *gardencorev1beta1.SecretBinding
)

var _ = BeforeSuite(func() {
	logf.SetLogger(logger.MustNewZapLogger(logger.DebugLevel, logger.FormatJSON, zap.WriteTo(GinkgoWriter)))
	log = logf.Log.WithName(testID)

	By("Start test environment")
	testEnv = &gardenerenvtest.GardenerTestEnvironment{
		GardenerAPIServer: &gardenerenvtest.GardenerAPIServer{
			Args: []string{
				"--disable-admission-plugins=DeletionConfirmation,ResourceReferenceManager,ExtensionValidator,ShootQuotaValidator,SeedValidator",
				"--feature-gates=HAControlPlanes=true"},
		},
	}

	var err error
	restConfig, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(restConfig).NotTo(BeNil())

	DeferCleanup(func() {
		By("Stop test environment")
		Expect(testEnv.Stop()).To(Succeed())
	})

	By("Create test clients")
	testClient, err = client.New(restConfig, client.Options{Scheme: kubernetes.GardenScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(testClient).NotTo(BeNil())

	versionedTestClient, err = gardencoreversionedclientset.NewForConfig(restConfig)
	Expect(err).NotTo(HaveOccurred())
	Expect(versionedTestClient).NotTo(BeNil())

	By("Create test Namespace")
	testNamespace = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			// create dedicated namespace for each test run, so that we can run multiple tests concurrently for stress tests
			GenerateName: "garden-",
		},
	}
	Expect(testClient.Create(ctx, testNamespace)).To(Succeed())
	log.Info("Created Namespace for test", "namespaceName", testNamespace.Name)

	DeferCleanup(func() {
		By("Delete test Namespace")
		Expect(testClient.Delete(ctx, testNamespace)).To(Or(Succeed(), BeNotFoundError()))
	})

	project := &gardencorev1beta1.Project{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",
		},
		Spec: gardencorev1beta1.ProjectSpec{
			Namespace: &testNamespace.Name,
		},
	}

	By("Create Project")
	Expect(testClient.Create(ctx, project)).To(Succeed())
	log.Info("Created Project for test", "project", client.ObjectKeyFromObject(project))

	DeferCleanup(func() {
		By("Delete Project")
		Expect(client.IgnoreNotFound(testClient.Delete(ctx, project))).To(Succeed())
	})

	By("Create SecretBinding")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",
			Namespace:    testNamespace.Name,
		},
	}
	Expect(testClient.Create(ctx, secret)).To(Succeed())
	log.Info("Created Secret for test", "secret", client.ObjectKeyFromObject(secret))

	DeferCleanup(func() {
		By("Delete Secret")
		Expect(client.IgnoreNotFound(testClient.Delete(ctx, secret))).To(Succeed())
	})

	testSecretBinding = &gardencorev1beta1.SecretBinding{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",
			Namespace:    testNamespace.Name,
		},
		Provider: &gardencorev1beta1.SecretBindingProvider{
			Type: providerType,
		},
		SecretRef: corev1.SecretReference{
			Name:      secret.Name,
			Namespace: secret.Namespace,
		},
	}
	Expect(testClient.Create(ctx, testSecretBinding)).To(Succeed())
	log.Info("Created SecretBinding for test", "secretBinding", client.ObjectKeyFromObject(testSecretBinding))

	DeferCleanup(func() {
		By("Delete SecretBinding")
		Expect(client.IgnoreNotFound(testClient.Delete(ctx, testSecretBinding))).To(Succeed())
	})
})
