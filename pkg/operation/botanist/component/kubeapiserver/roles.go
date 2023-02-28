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

package kubeapiserver

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/gardener/gardener/pkg/controllerutils"
)

const (
	serviceAccountName = "kube-apiserver"
	roleNameHAVPN      = "kube-apiserver-vpn-client-init"
)

func (k *kubeAPIServer) emptyServiceAccount() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: k.namespace,
		},
	}
}

func (k *kubeAPIServer) emptyRoleHAVPN() *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleNameHAVPN,
			Namespace: k.namespace,
		},
	}
}

func (k *kubeAPIServer) emptyRoleBindingHAVPN() *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleNameHAVPN,
			Namespace: k.namespace,
		},
	}
}

func (k *kubeAPIServer) reconcileServiceAccount(ctx context.Context) error {
	sa := k.emptyServiceAccount()
	_, err := controllerutils.GetAndCreateOrMergePatch(ctx, k.client.Client(), sa, func() error {
		sa.AutomountServiceAccountToken = pointer.Bool(false)
		return nil
	})
	return err
}

func (k *kubeAPIServer) reconcileRoleHAVPN(ctx context.Context) error {
	role := k.emptyRoleHAVPN()
	_, err := controllerutils.GetAndCreateOrMergePatch(ctx, k.client.Client(), role, func() error {
		role.Rules = []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch", "patch", "update"},
			},
		}
		return nil
	})
	return err
}

func (k *kubeAPIServer) reconcileRoleBindingHAVPN(ctx context.Context) error {
	roleBinding := k.emptyRoleBindingHAVPN()
	_, err := controllerutils.GetAndCreateOrMergePatch(ctx, k.client.Client(), roleBinding, func() error {
		roleBinding.RoleRef = rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     roleNameHAVPN,
		}
		roleBinding.Subjects = []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccountName,
				Namespace: k.namespace,
			},
		}
		return nil
	})
	return err
}
