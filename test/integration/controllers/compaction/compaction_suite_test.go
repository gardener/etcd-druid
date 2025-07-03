// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package compaction

import (
	"context"
	"fmt"
	"testing"
	"time"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/controller/compaction"
	"github.com/gardener/etcd-druid/test/integration/controllers/assets"
	"github.com/gardener/etcd-druid/test/integration/setup"

	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	intTestEnv *setup.IntegrationTestEnv
	k8sClient  client.Client
	namespace  string
)

const (
	testNamespacePrefix = "compaction-"
)

func TestCompactionController(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(
		t,
		"Compaction Controller Suite",
	)
}

var _ = BeforeSuite(func() {
	crdPaths := []string{assets.GetEtcdCrdPath()}
	imageVector := assets.CreateImageVector()

	intTestEnv = setup.NewIntegrationTestEnv(testNamespacePrefix, "compaction-int-tests", crdPaths)
	intTestEnv.RegisterReconcilers(func(mgr manager.Manager) {
		reconciler := compaction.NewReconcilerWithImageVector(mgr, druidconfigv1alpha1.CompactionControllerConfiguration{
			Enabled:                      true,
			ConcurrentSyncs:              ptr.To(5),
			EventsThreshold:              100,
			TriggerFullSnapshotThreshold: 300,
			ActiveDeadlineDuration:       metav1.Duration{Duration: 2 * time.Minute},
			MetricsScrapeWaitDuration:    metav1.Duration{Duration: 60 * time.Second},
		}, imageVector)

		// mock fullSnapshotTrigger for integration tests
		reconciler.FullSnapshotTrigger = func(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) error {
			fullLease := &coordinationv1.Lease{}
			fullLeaseName := druidv1alpha1.GetFullSnapshotLeaseName(etcd.ObjectMeta)
			if err := reconciler.Client.Get(ctx, client.ObjectKey{Namespace: etcd.Namespace, Name: fullLeaseName}, fullLease); err != nil {
				return err
			}
			deltaLease := &coordinationv1.Lease{}
			deltaLeaseName := druidv1alpha1.GetDeltaSnapshotLeaseName(etcd.ObjectMeta)
			if err := reconciler.Client.Get(ctx, client.ObjectKey{Namespace: etcd.Namespace, Name: deltaLeaseName}, deltaLease); err != nil {
				return err
			}
			fullLease.Spec.HolderIdentity = deltaLease.Spec.HolderIdentity
			if err := reconciler.Client.Update(ctx, fullLease); err != nil {
				return err
			}
			return nil
		}
		Expect(reconciler.RegisterWithManager(mgr)).To(Succeed())
	}).StartManager()
	k8sClient = intTestEnv.K8sClient
	namespace = intTestEnv.TestNs.Name
})
