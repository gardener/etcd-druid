// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"testing"
	"time"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"

	"github.com/go-logr/logr/testr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// newTestReconciler creates a new Reconciler instance for testing.
func newTestReconciler(t *testing.T, cl client.Client) *Reconciler {
	return &Reconciler{
		logger: testr.New(t),
		client: cl,
		config: &druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration{
			RequeueInterval: &metav1.Duration{Duration: 60 * time.Second},
		},
	}
}
