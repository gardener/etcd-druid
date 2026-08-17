// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	e2eutils "github.com/gardener/etcd-druid/test/e2e/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

// TestExternallyManagedMembersScaleOut tests a 1->2->3 scale-out with externally managed members.
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
			f := createFixture(t, tcName, tc.tlsEnabled)
			bringupExternalMembersCluster(f)

			testEnv.VerifyStatefulSetZeroReplicas(f.g, f.etcd)
			testEnv.VerifyNoServicesOrPDB(f.g, f.etcd)

			f.logger.Info("test passed", "purpose", tc.purpose)
			*f.testSucceeded = true
		})
	}
}

// TestExternallyManagedMembersScaleIn tests a 3->2->1 scale-in with externally managed members.
func TestExternallyManagedMembersScaleIn(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		purpose    string
		tlsEnabled bool
	}{
		{
			name:    "no-tls",
			purpose: "test sequential 3->2->1 scale-in with externally managed members",
		},
		{
			name:       "tls",
			purpose:    "test sequential 3->2->1 scale-in with TLS and externally managed members",
			tlsEnabled: true,
		},
	}

	for _, tc := range testCases {
		tcName := fmt.Sprintf("ext-members-scalein-%s", tc.name)
		t.Run(tcName, func(t *testing.T) {
			t.Parallel()
			f := createFixture(t, tcName, tc.tlsEnabled)
			bringupExternalMembersCluster(f)

			resizeExternalMembersCluster(f, 2)
			resizeExternalMembersCluster(f, 1)

			f.logger.Info("test passed", "purpose", tc.purpose)
			*f.testSucceeded = true
		})
	}
}

// TestExternallyManagedMembersDataCorruption wipes a member's data directory and expects it
// to be removed and re-join as a new learner automatically.
func TestExternallyManagedMembersDataCorruption(t *testing.T) {
	t.Parallel()

	f := createFixture(t, "ext-members-data-corruption", false)
	bringupExternalMembersCluster(f)

	const corruptedOrdinal = 0
	f.logger.Info("removing static pod manifest to stop member", "ordinal", corruptedOrdinal)
	removeStaticPodManifest(f.g, f.testNamespace, e2eutils.DefaultEtcdName, corruptedOrdinal)
	waitForStaticPodStopped(f.g, f.ctx, f.cl, f.testNamespace, e2eutils.DefaultEtcdName, corruptedOrdinal)

	f.logger.Info("wiping data directory on worker", "ordinal", corruptedOrdinal)
	corruptMemberDataDir(f.g, f.testNamespace, e2eutils.DefaultEtcdName, corruptedOrdinal)

	f.logger.Info("redeploying static pod on worker", "ordinal", corruptedOrdinal)
	deployStaticPod(f.g, f.ctx, f.cl, e2eutils.DefaultEtcdName, f.testNamespace, corruptedOrdinal, f.kubeconfigFile, f.workerIPs[:2])

	// The Etcd ready condition may be stale; confirm the pod itself is back first.
	f.logger.Info("waiting for redeployed pod to be ready", "ordinal", corruptedOrdinal)
	waitForStaticPodReady(f.g, f.ctx, f.cl, f.testNamespace, e2eutils.DefaultEtcdName, corruptedOrdinal)

	f.logger.Info("waiting for cluster to recover after data corruption")
	testEnv.CheckEtcdReady(f.g, f.etcd, timeoutExtMembersReady)
	f.logger.Info("cluster recovered")

	f.logger.Info("test passed: cluster recovered from data corruption on one externally managed member")
	*f.testSucceeded = true
}

// TestExternallyManagedMembersDataValidation kills etcd-wrapper abruptly and verifies the
// member passes full data validation at startup and rejoins the cluster.
func TestExternallyManagedMembersDataValidation(t *testing.T) {
	t.Parallel()

	f := createFixture(t, "ext-members-data-validation", false)
	bringupExternalMembersCluster(f)

	const killedOrdinal = 0
	f.logger.Info("killing etcd-wrapper process on worker", "ordinal", killedOrdinal)
	killEtcdWrapperProcess(f.g, f.testNamespace, killedOrdinal)

	f.logger.Info("waiting for grace period after process kill", "duration", gracePeriodAfterProcessKill)
	time.Sleep(gracePeriodAfterProcessKill)

	f.logger.Info("waiting for pod to be ready after process kill", "ordinal", killedOrdinal)
	waitForStaticPodReady(f.g, f.ctx, f.cl, f.testNamespace, e2eutils.DefaultEtcdName, killedOrdinal)

	f.logger.Info("waiting for Etcd to report ready after recovery")
	testEnv.CheckEtcdReady(f.g, f.etcd, timeoutExtMembersReady)

	f.logger.Info("test passed: member passed full data validation at startup after abrupt kill")
	*f.testSucceeded = true
}

// extMembersTestFixture holds the shared state for externally managed member tests.
type extMembersTestFixture struct {
	g                *WithT
	ctx              context.Context
	cl               client.Client
	logger           logr.Logger
	testNamespace    string
	workerIPs        []string
	etcd             *druidv1alpha1.Etcd
	testSucceeded    *bool
	certs            *tlsCertDirs
	kubeconfigFile   string
	deployedReplicas int
}

// tlsCertDirs holds local directory paths for TLS certificates used by externally managed member tests.
type tlsCertDirs struct {
	etcd, etcdPeer, etcdbr string
}

// createFixture sets up the test namespace, logger, workers, cleanup handlers, and TLS PKI if
// requested. Callers must set *f.testSucceeded = true to suppress artifact cleanup on success.
func createFixture(t *testing.T, tcName string, tlsEnabled bool) *extMembersTestFixture {
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
		return f
	}

	memberIPs := make([]net.IP, len(f.workerIPs))
	for i, ip := range f.workerIPs {
		memberIPs[i] = net.ParseIP(ip)
		f.g.Expect(memberIPs[i]).ToNot(BeNil(), "failed to parse worker IP %s", ip)
	}
	f.logger.Info("initializing TLS PKI resources")
	etcdDir, etcdPeerDir, etcdbrDir := e2eutils.InitializeExternalMembersTestCaseWithTLS(f.g, testEnv, f.logger, f.testNamespace, e2eutils.DefaultEtcdName, memberIPs)
	f.certs = &tlsCertDirs{etcd: etcdDir, etcdPeer: etcdPeerDir, etcdbr: etcdbrDir}
	return f
}

// bringupExternalMembersCluster creates the Etcd CR and bootstraps a 1->2->3 member cluster.
func bringupExternalMembersCluster(f *extMembersTestFixture) {
	ports := allocatePorts(f.testNamespace)
	f.logger.Info("creating Etcd CR with 1 replica", "ports", ports)
	etcdBuilder := testutils.EtcdBuilderWithoutDefaults(e2eutils.DefaultEtcdName, f.testNamespace).
		WithReplicas(1).
		WithEtcdClientPort(ptr.To(ports.ClientPort)).
		WithEtcdServerPort(ptr.To(ports.PeerPort)).
		WithBackupPort(ptr.To(ports.BackupPort)).
		WithEtcdWrapperPort(ptr.To(ports.WrapperPort)).
		WithExternallyManagedMembers(f.workerIPs[:1]).
		WithDynamicEndpoints(dynamicEndpointsSpec(f.testNamespace, e2eutils.DefaultEtcdName)).
		WithVolumes([]corev1.Volume{{
			Name: kubeconfigDirName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: workerBaseDir(f.testNamespace, e2eutils.DefaultEtcdName) + "/" + kubeconfigDirName,
					Type: ptr.To(corev1.HostPathDirectoryOrCreate),
				},
			},
		}}).
		WithBackupVolumeMounts([]corev1.VolumeMount{{
			Name:      kubeconfigDirName,
			MountPath: "/etc/kubeconfig",
			ReadOnly:  true,
		}}).
		WithBackupEnv([]corev1.EnvVar{{
			Name:  "KUBECONFIG",
			Value: "/etc/kubeconfig/" + kubeconfigFileName,
		}}).
		WithRunAsRoot(ptr.To(true))
	if f.certs != nil {
		etcdBuilder = etcdBuilder.WithClientTLS().WithPeerTLS().WithBackupRestoreTLS()
	}
	etcd := etcdBuilder.Build()
	testEnv.CreateEtcd(f.g, etcd)
	testEnv.WaitForReconciliation(f.g, etcd, timeoutReconciliation)
	f.etcd = etcd

	f.kubeconfigFile = fetchAdminKubeconfig(f.g, f.testNamespace)

	resizeExternalMembersCluster(f, 1)
	resizeExternalMembersCluster(f, 2)
	resizeExternalMembersCluster(f, 3)
}

// resizeExternalMembersCluster resizes the cluster in either direction and waits for full
// convergence; the test stops pods of dropped members and deploys pods for new members.
func resizeExternalMembersCluster(f *extMembersTestFixture, replicas int) {
	etcdName := e2eutils.DefaultEtcdName

	resizeEtcdCR(f, replicas)

	// Perform scale-in if required
	for ordinal := f.deployedReplicas - 1; ordinal >= replicas; ordinal-- {
		f.logger.Info("waiting for dropped member to be removed from the etcd cluster", "expectedMembers", ordinal)
		testEnv.CheckEtcdMemberCount(f.g, f.etcd, ordinal, timeoutExtMembersReady)
		f.logger.Info("stopping static pod of removed member", "ordinal", ordinal)
		removeStaticPodManifest(f.g, f.testNamespace, etcdName, ordinal)
		waitForStaticPodStopped(f.g, f.ctx, f.cl, f.testNamespace, etcdName, ordinal)
	}

	// perform scale-out if required
	for ordinal := f.deployedReplicas; ordinal < replicas; ordinal++ {
		if f.certs != nil {
			copyTLSToWorker(f.g, etcdName, f.testNamespace, ordinal, f.certs.etcd, f.certs.etcdPeer, f.certs.etcdbr)
		}
		deployStaticPod(f.g, f.ctx, f.cl, etcdName, f.testNamespace, ordinal, f.kubeconfigFile, f.workerIPs[:ordinal])
	}

	f.deployedReplicas = replicas

	f.logger.Info("waiting for members to converge", "replicas", replicas)
	testEnv.CheckEtcdMemberCount(f.g, f.etcd, replicas, timeoutExtMembersReady)
	testEnv.VerifyMemberLeases(f.g, f.etcd, f.workerIPs[:replicas], timeoutMemberLeases)
	waitForEndpointsOnAllWorkers(f.g, f.testNamespace, etcdName, replicas, f.workerIPs[:replicas])
	testEnv.CheckEtcdReady(f.g, f.etcd, timeoutExtMembersReady)
}

// resizeEtcdCR sets the Etcd CR's replica count and member addresses and waits for the reconcile.
func resizeEtcdCR(f *extMembersTestFixture, replicas int) {
	f.logger.Info("resizing Etcd CR", "replicas", replicas, "memberAddresses", f.workerIPs[:replicas])
	testEnv.UpdateEtcd(f.g, f.etcd, func(etcd *druidv1alpha1.Etcd) {
		etcd.Spec.Replicas = int32(replicas) // #nosec G115 -- replicas is a small positive test constant, fits int32
		etcd.Spec.ExternallyManagedMemberAddresses = f.workerIPs[:replicas]
	})
	testEnv.WaitForReconciliation(f.g, f.etcd, timeoutReconciliation)
}
