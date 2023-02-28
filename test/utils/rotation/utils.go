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

package rotation

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
)

// SecretConfigNamesToSecrets is a map for secret config names to a list of corev1.Secret objects.
type SecretConfigNamesToSecrets map[string][]corev1.Secret

// GroupByName groups all secrets by name.
func GroupByName(allSecrets []corev1.Secret) SecretConfigNamesToSecrets {
	grouped := make(SecretConfigNamesToSecrets)
	for _, secret := range allSecrets {
		grouped[secret.Labels["name"]] = append(grouped[secret.Labels["name"]], secret)
	}

	for _, secrets := range grouped {
		sort.Sort(AgeSorter(secrets))
	}
	return grouped
}

// AgeSorter implements sort.Interface for a slice of secrets for sorting by age.
type AgeSorter []corev1.Secret

func (x AgeSorter) Len() int           { return len(x) }
func (x AgeSorter) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }
func (x AgeSorter) Less(i, j int) bool { return x[i].CreationTimestamp.Before(&x[j].CreationTimestamp) }
