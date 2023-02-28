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

package validation

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	operatorv1alpha1 "github.com/gardener/gardener/pkg/apis/operator/v1alpha1"
)

const (
	// HandlerName is the name of this admission webhook handler.
	HandlerName = "validator"
	// WebhookPath is the HTTP handler path for this admission webhook handler.
	WebhookPath = "/webhooks/validate-operator-gardener-cloud-v1alpha1-garden"
)

// AddToManager adds Handler to the given manager.
func (h *Handler) AddToManager(mgr manager.Manager) error {
	webhook := admission.
		WithCustomValidator(&operatorv1alpha1.Garden{}, h).
		WithRecoverPanic(true)

	mgr.GetWebhookServer().Register(WebhookPath, webhook)
	return nil
}
