// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestServiceAccountMatchesUsername(t *testing.T) {
	testCases := []struct {
		testName  string
		name      string
		namespace string
		username  string
		expect    bool
	}{
		{
			testName:  "username does not have desired serviceaccount prefix",
			name:      "foo",
			namespace: "bar",
			username:  "foo",
			expect:    false,
		},
		{
			testName:  "username does not have namespace as the middle part",
			name:      "foo",
			namespace: "bar",
			username:  "system:serviceaccount:foo",
			expect:    false,
		},
		{
			testName:  "username does not have the name suffix",
			name:      "foo",
			namespace: "bar",
			username:  "system:serviceaccount:foo:",
			expect:    false,
		},
		{
			testName:  "username has empty namespace",
			name:      "foo",
			namespace: "bar",
			username:  "system:serviceaccount::foo",
			expect:    false,
		},
		{
			testName:  "username has empty name",
			name:      "foo",
			namespace: "bar",
			username:  "",
			expect:    false,
		},
		{
			testName:  "username only has the serviceaccount prefix",
			name:      "foo",
			namespace: "bar",
			username:  "system:serviceaccount:",
			expect:    false,
		},
		{
			testName:  "username has serviceaccount prefix followed by namespace, followed by name",
			name:      "foo",
			namespace: "bar",
			username:  "system:serviceaccount:bar:foo",
			expect:    true,
		},
		{
			testName:  "username has mismatched namespace",
			name:      "foo",
			namespace: "bar",
			username:  "system:serviceaccount:baz:foo",
			expect:    false,
		},
		{
			testName:  "username has mismatched name",
			name:      "foo",
			namespace: "bar",
			username:  "system:serviceaccount:bar:baz",
			expect:    false,
		},
	}

	g := NewWithT(t)
	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			t.Parallel()
			g.Expect(ServiceAccountMatchesUsername(tc.namespace, tc.name, tc.username)).To(Equal(tc.expect))
		})
	}
}
