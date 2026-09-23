// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	e2eutils "github.com/gardener/etcd-druid/test/e2e/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

func TestExternallyManagedMembersScaleOut(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		purpose    string
		tlsEnabled bool
	}{
		{
			name:    "no-tls",
			purpose: "test sequential 1->2->3 scale-out with externally managed members",
		},
		{
			name:       "tls",
			purpose:    "test sequential 1->2->3 scale-out with TLS and externally managed members",
			tlsEnabled: true,
		},
	}

	for _, tc := range testCases {
		tcName := fmt.Sprintf("ext-members-scaleout-%s", tc.name)
		t.Run(tcName, func(t *testing.T) {
			t.Parallel()
			f, certs := createFixture(t, tcName, tc.tlsEnabled)
			bringupExternalMembersCluster(f, certs)

			testEnv.VerifyMemberLeases(f.g, f.etcd, f.workerIPs[:3], timeoutMemberLeases)
			testEnv.VerifyStatefulSetZeroReplicas(f.g, f.etcd)
			testEnv.VerifyNoServicesOrPDB(f.g, f.etcd)

			f.logger.Info("waiting for all workers to have all 3 IPs in endpoints file")
			waitForEndpointsOnAllWorkers(f.g, f.testNamespace, e2eutils.DefaultEtcdName, 3, f.workerIPs[:3])
			f.logger.Info("all workers have correct endpoints file")

			f.g.Expect(f.cl.Get(f.ctx, client.ObjectKeyFromObject(f.etcd), f.etcd)).To(Succeed())
			f.g.Expect(f.etcd.Status.Ready).ToNot(BeNil())
			f.g.Expect(*f.etcd.Status.Ready).To(BeTrue())

			f.logger.Info("test passed", "purpose", tc.purpose)
			*f.testSucceeded = true
		})
	}
}

// TestExternallyManagedMembersDataCorruption tests that a 3-member externally managed
// cluster recovers automatically when one member's data directory is wiped.
// The corrupted member is expected to be removed from the cluster by etcd-backup-restore
// and then re-added as a new learner, rejoining without manual intervention.
func TestExternallyManagedMembersDataCorruption(t *testing.T) {
	t.Parallel()

	f, certs := createFixture(t, "ext-members-data-corruption", false)
	bringupExternalMembersCluster(f, certs)

	// Corrupt worker-2: remove its static pod manifest so kubelet stops the pod, then wipe
	// the data directory to simulate data corruption, then redeploy.
	// etcd-backup-restore on the remaining healthy members should detect the stale peer,
	// remove it from the cluster, and the restarted member should re-join as a new learner.
	const corruptedOrdinal = 2
	f.logger.Info("removing static pod manifest to stop member", "ordinal", corruptedOrdinal)
	removeStaticPodManifest(f.g, f.testNamespace, e2eutils.DefaultEtcdName, corruptedOrdinal)
	waitForStaticPodStopped(f.g, f.ctx, f.cl, f.testNamespace, e2eutils.DefaultEtcdName, corruptedOrdinal)

	f.logger.Info("wiping data directory on worker", "ordinal", corruptedOrdinal)
	corruptMemberDataDir(f.g, f.testNamespace, e2eutils.DefaultEtcdName, corruptedOrdinal)

	f.logger.Info("redeploying static pod on worker", "ordinal", corruptedOrdinal)
	saDir := filepath.Join(e2eutils.ExtMembersResourcesDir, f.testNamespace, "serviceaccount")
	deployStaticPod(f.g, f.ctx, f.cl, e2eutils.DefaultEtcdName, f.testNamespace, corruptedOrdinal, filepath.Join(saDir, "token"), filepath.Join(saDir, "ca.crt"), f.workerIPs[:2])

	// Wait for the redeployed pod to be running before checking the Etcd ready condition,
	// which may be stale from a previous reconciliation cycle.
	f.logger.Info("waiting for redeployed pod to be ready", "ordinal", corruptedOrdinal)
	waitForStaticPodReady(f.g, f.ctx, f.cl, f.testNamespace, e2eutils.DefaultEtcdName, corruptedOrdinal)

	// The cluster must return to a fully ready 3-member state.
	f.logger.Info("waiting for cluster to recover after data corruption")
	testEnv.CheckEtcdReady(f.g, f.etcd, timeoutExtMembersReady)
	f.logger.Info("cluster recovered")

	testEnv.VerifyMemberLeases(f.g, f.etcd, f.workerIPs[:3], timeoutMemberLeases)
	testEnv.VerifyStatefulSetZeroReplicas(f.g, f.etcd)
	testEnv.VerifyNoServicesOrPDB(f.g, f.etcd)

	f.g.Expect(f.cl.Get(f.ctx, client.ObjectKeyFromObject(f.etcd), f.etcd)).To(Succeed())
	f.g.Expect(f.etcd.Status.Ready).ToNot(BeNil())
	f.g.Expect(*f.etcd.Status.Ready).To(BeTrue())

	f.logger.Info("test passed: cluster recovered from data corruption on one externally managed member")
	*f.testSucceeded = true
}

// TestExternallyManagedMembersProcessKill tests that a 3-member externally managed cluster
// recovers correctly after the etcd-wrapper process on one member is killed abruptly.
// Kubelet restarts the container automatically; the test verifies that the pod becomes
// ready again and the Etcd ready condition reflects a healthy cluster.
func TestExternallyManagedMembersProcessKill(t *testing.T) {
	t.Parallel()

	f, certs := createFixture(t, "ext-members-process-kill", false)
	bringupExternalMembersCluster(f, certs)

	// Kill etcd-wrapper on worker-2 abruptly. Kubelet will restart the container.
	const killedOrdinal = 2
	f.logger.Info("killing etcd-wrapper process on worker", "ordinal", killedOrdinal)
	killEtcdWrapperProcess(f.g, killedOrdinal)

	// Wait longer than the readiness probe initialDelaySeconds so that the probe must
	// succeed in the restarted container before we check higher-level conditions.
	f.logger.Info("waiting for grace period after process kill", "duration", gracePeriodAfterProcessKill)
	time.Sleep(gracePeriodAfterProcessKill)

	f.logger.Info("waiting for pod to be ready after process kill", "ordinal", killedOrdinal)
	waitForStaticPodReady(f.g, f.ctx, f.cl, f.testNamespace, e2eutils.DefaultEtcdName, killedOrdinal)

	f.logger.Info("waiting for Etcd to report ready after recovery")
	testEnv.CheckEtcdReady(f.g, f.etcd, timeoutExtMembersReady)

	testEnv.VerifyMemberLeases(f.g, f.etcd, f.workerIPs[:3], timeoutMemberLeases)
	testEnv.VerifyStatefulSetZeroReplicas(f.g, f.etcd)
	testEnv.VerifyNoServicesOrPDB(f.g, f.etcd)

	f.g.Expect(f.cl.Get(f.ctx, client.ObjectKeyFromObject(f.etcd), f.etcd)).To(Succeed())
	f.g.Expect(f.etcd.Status.Ready).ToNot(BeNil())
	f.g.Expect(*f.etcd.Status.Ready).To(BeTrue())

	f.logger.Info("test passed: cluster recovered after etcd-wrapper process kill on one externally managed member")
	*f.testSucceeded = true
}

// extMembersTestFixture holds the shared state for externally managed member tests.
type extMembersTestFixture struct {
	g             *WithT
	ctx           context.Context
	cl            client.Client
	logger        logr.Logger
	testNamespace string
	workerIPs     []string
	etcd          *druidv1alpha1.Etcd
	testSucceeded *bool
}

// tlsCertDirs holds local directory paths for TLS certificates used by externally managed member tests.
type tlsCertDirs struct {
	etcd, etcdPeer, etcdbr string
}

// createFixture sets up the test namespace, logger, worker nodes, and cleanup handlers,
// and initializes TLS PKI resources if tlsEnabled. It returns the fixture and optional
// cert dirs to pass to bringupExternalMembersCluster.
// The caller must set *f.testSucceeded = true before returning to suppress artifact cleanup on success.
func createFixture(t *testing.T, tcName string, tlsEnabled bool) (*extMembersTestFixture, *tlsCertDirs) {
	g := NewWithT(t)
	succeeded := false
	f := &extMembersTestFixture{
		g:             g,
		ctx:           testEnv.Context(),
		cl:            testEnv.Client(),
		testSucceeded: &succeeded,
	}

	f.testNamespace = testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
	f.logger = testr.NewWithOptions(t, testr.Options{LogTimestamp: true}).WithName(tcName).WithValues("etcdName", e2eutils.DefaultEtcdName, "namespace", f.testNamespace)

	t.Cleanup(func() {
		e2eutils.CleanupTestArtifacts(retainTestArtifacts, *f.testSucceeded, testEnv, f.logger, f.g, f.testNamespace)
	})

	f.workerIPs = make([]string, 3)
	for i := range 3 {
		f.logger.Info("setting up worker node", "ordinal", i)
		f.workerIPs[i] = setupWorker(f.g, f.ctx, f.cl, e2eutils.DefaultEtcdName, f.testNamespace, i)
		f.logger.Info("worker node ready", "ordinal", i, "ip", f.workerIPs[i])
	}
	t.Cleanup(func() {
		if e2eutils.ShouldCleanup(retainTestArtifacts, *f.testSucceeded) {
			f.logger.Info("cleaning up workers")
			cleanupWorkers(e2eutils.DefaultEtcdName, f.testNamespace, 3)
		} else {
			f.logger.Info("retaining worker artifacts")
		}
	})

	if !tlsEnabled {
		e2eutils.InitializeExternalMembersTestCase(f.g, testEnv, f.logger, f.testNamespace)
		return f, nil
	}

	memberIPs := make([]net.IP, len(f.workerIPs))
	for i, ip := range f.workerIPs {
		memberIPs[i] = net.ParseIP(ip)
		f.g.Expect(memberIPs[i]).ToNot(BeNil(), "failed to parse worker IP %s", ip)
	}
	f.logger.Info("initializing TLS PKI resources")
	etcdDir, etcdPeerDir, etcdbrDir := e2eutils.InitializeExternalMembersTestCaseWithTLS(f.g, testEnv, f.logger, f.testNamespace, e2eutils.DefaultEtcdName, memberIPs)
	return f, &tlsCertDirs{etcd: etcdDir, etcdPeer: etcdPeerDir, etcdbr: etcdbrDir}
}

// bringupExternalMembersCluster creates an Etcd CR and sequentially bootstraps a 3-member
// externally managed cluster, verifying readiness after each scale step.
// certs is nil for non-TLS clusters.
func bringupExternalMembersCluster(f *extMembersTestFixture, certs *tlsCertDirs) {
	ctx := f.ctx
	cl := f.cl

	ports := allocatePorts(f.testNamespace)
	f.logger.Info("creating Etcd CR with 1 replica", "ports", ports)
	etcdBuilder := testutils.EtcdBuilderWithoutDefaults(e2eutils.DefaultEtcdName, f.testNamespace).
		WithReplicas(1).
		WithEtcdClientPort(ptr.To(ports.ClientPort)).
		WithEtcdServerPort(ptr.To(ports.PeerPort)).
		WithBackupPort(ptr.To(ports.BackupPort)).
		WithEtcdWrapperPort(ptr.To(ports.WrapperPort)).
		WithExternallyManagedMembers(f.workerIPs[:1]).
		WithBackupRestoreContainerImage("europe-docker.pkg.dev/gardener-project/snapshots/gardener/etcdbrctl:v0.45.0-dev").
		WithDynamicEndpoints(dynamicEndpointsSpec(f.testNamespace, e2eutils.DefaultEtcdName)).
		WithRunAsRoot(ptr.To(true))
	if certs != nil {
		etcdBuilder = etcdBuilder.WithClientTLS().WithPeerTLS().WithBackupRestoreTLS()
	}
	etcd := etcdBuilder.Build()
	etcd.SetAnnotations(map[string]string{druidv1alpha1.DruidOperationAnnotation: druidv1alpha1.DruidOperationReconcile})
	f.g.Expect(cl.Create(ctx, etcd)).To(Succeed())
	testEnv.WaitForReconciliation(f.g, etcd, timeoutReconciliation)
	f.etcd = etcd

	saTokenFile, caCertFile := prepareServiceAccount(f.g, ctx, cl, e2eutils.DefaultEtcdName, f.testNamespace)

	// scaleAndDeploy scales the Etcd CR to the given replica count, updates existing worker
	// configs, copies TLS certs if needed, and deploys the static pod on the new member.
	// ordinal = replicas-1; existing peers = workerIPs[:ordinal].
	scaleAndDeploy := func(replicas int32) {
		ordinal := int(replicas) - 1
		if replicas > 1 {
			f.g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
			etcd.Spec.Replicas = replicas
			etcd.Spec.ExternallyManagedMemberAddresses = f.workerIPs[:replicas]
			etcd.SetAnnotations(map[string]string{druidv1alpha1.DruidOperationAnnotation: druidv1alpha1.DruidOperationReconcile})
			f.g.Expect(cl.Update(ctx, etcd)).To(Succeed())
			testEnv.WaitForReconciliation(f.g, etcd, timeoutReconciliation)
			for i := range ordinal {
				updateWorkerConfig(f.g, ctx, cl, e2eutils.DefaultEtcdName, f.testNamespace, i)
			}
		}
		if certs != nil {
			copyTLSToWorker(f.g, e2eutils.DefaultEtcdName, f.testNamespace, ordinal, certs.etcd, certs.etcdPeer, certs.etcdbr)
		}
		deployStaticPod(f.g, ctx, cl, e2eutils.DefaultEtcdName, f.testNamespace, ordinal, saTokenFile, caCertFile, f.workerIPs[:ordinal])
		f.logger.Info("waiting for members to be ready", "count", replicas)
		testEnv.CheckEtcdReady(f.g, etcd, timeoutExtMembersReady)
	}

	scaleAndDeploy(1)
	scaleAndDeploy(2)
	scaleAndDeploy(3)
}

func dynamicEndpointsSpec(namespace, etcdName string) druidv1alpha1.DynamicEndpointsSpec {
	base := workerBaseDir(namespace, etcdName)
	return druidv1alpha1.DynamicEndpointsSpec{
		HostPathDir:       fmt.Sprintf("%s/%s", base, endpointsDirName),
		EndpointsFileName: endpointsFileName,
		RefreshEnabled:    ptr.To(true),
		RefreshInterval:   &metav1.Duration{Duration: 10 * time.Second},
	}
}
