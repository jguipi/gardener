// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package workloadidentity

import "time"

// Functions exported for testing.

var (
	GetKeyID       = getKeyID
	GetSigner      = getSigner
	GetRSASigner   = getRSASigner
	GetECDSASigner = getECDSASigner
)

func SetNow(n func() time.Time) {
	now = n
}

func Now() func() time.Time {
	return now
}
