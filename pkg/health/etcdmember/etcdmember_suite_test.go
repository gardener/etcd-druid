// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdmember_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEtcdMember(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Etcd Member Suite")
}
