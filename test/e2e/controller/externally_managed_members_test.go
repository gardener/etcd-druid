// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"
	"net"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr/testr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

func TestExternallyManagedMembersScaleOut(t *testing.T) {
	t.Parallel()
	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})

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
			g := NewWithT(t)
			var testSucceeded bool

			ctx := testEnv.Context()
			cl := testEnv.Client()
			testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
			logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
			defer func() {
				cleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
			}()

			// Set up 3 worker nodes and record their IPs
			workerIPs := make([]string, 3)
			for i := range 3 {
				logger.Info("setting up worker node", "ordinal", i)
				workerIPs[i] = setupWorker(g, ctx, cl, defaultEtcdName, testNamespace, i)
				logger.Info("worker node ready", "ordinal", i, "ip", workerIPs[i])
			}
			defer func() {
				if shouldCleanupWorkers(retainTestArtifacts, testSucceeded) {
					logger.Info("cleaning up workers")
					cleanupWorkers(defaultEtcdName, testNamespace, 3)
				} else {
					logger.Info("retaining worker artifacts")
				}
			}()

			var etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir string
			if tc.tlsEnabled {
				memberIPs := make([]net.IP, len(workerIPs))
				for i, ip := range workerIPs {
					memberIPs[i] = net.ParseIP(ip)
					g.Expect(memberIPs[i]).ToNot(BeNil(), "failed to parse worker IP %s", ip)
				}
				logger.Info("initializing TLS PKI resources")
				etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir = initializeExternalMembersTestCaseWithTLS(g, testEnv, logger, testNamespace, defaultEtcdName, memberIPs)
			} else {
				initializeExternalMembersTestCase(g, testEnv, logger, testNamespace)
			}

			// --- Scale step 1: replicas=1, addresses=[ip0] ---
			ports := allocatePorts(tcName)
			logger.Info("step 1: creating Etcd CR with 1 replica", "ports", ports)
			etcdBuilder := testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, testNamespace).
				WithReplicas(1).
				WithEtcdClientPort(ptr.To(ports.ClientPort)).
				WithEtcdServerPort(ptr.To(ports.PeerPort)).
				WithBackupPort(ptr.To(ports.BackupPort)).
				WithEtcdWrapperPort(ptr.To(ports.WrapperPort)).
				WithBackupRestoreContainerImage("europe-docker.pkg.dev/gardener-project/snapshots/gardener/etcdbrctl:v0.43.0-dev").
				WithExternallyManagedMembers(workerIPs[:1])
			if tc.tlsEnabled {
				etcdBuilder = etcdBuilder.WithClientTLS().WithPeerTLS().WithBackupRestoreTLS()
			}
			etcd := etcdBuilder.Build()

			etcd.SetAnnotations(map[string]string{druidv1alpha1.DruidOperationAnnotation: druidv1alpha1.DruidOperationReconcile})
			g.Expect(cl.Create(ctx, etcd)).To(Succeed())

			logger.Info("waiting for druid reconciliation")
			waitForReconciliation(g, etcd)

			logger.Info("preparing service account token")
			saTokenFile, caCertFile := prepareServiceAccount(g, ctx, cl, defaultEtcdName, testNamespace)

			// Deploy static pod on worker-0
			logger.Info("deploying static pod on worker-0")
			if tc.tlsEnabled {
				copyTLSToWorker(g, defaultEtcdName, testNamespace, 0, etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir)
			}
			deployStaticPod(g, ctx, cl, defaultEtcdName, testNamespace, 0, saTokenFile, caCertFile)

			logger.Info("waiting for 1 member to be ready")
			testEnv.CheckEtcdReady(g, etcd, timeoutExtMembersReady)
			logger.Info("step 1: 1 member ready")

			testEnv.VerifyMemberLeases(g, etcd, workerIPs[:1], timeoutMemberLeases)
			testEnv.VerifyStatefulSetZeroReplicas(g, etcd)
			testEnv.VerifyNoServicesOrPDB(g, etcd)

			// --- Scale step 2: replicas=2, addresses=[ip0, ip1] ---
			logger.Info("step 2: scaling to 2 replicas")
			g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
			etcd.Spec.Replicas = 2
			etcd.Spec.ExternallyManagedMemberAddresses = workerIPs[:2]
			etcd.SetAnnotations(map[string]string{druidv1alpha1.DruidOperationAnnotation: druidv1alpha1.DruidOperationReconcile})
			g.Expect(cl.Update(ctx, etcd)).To(Succeed())

			logger.Info("waiting for druid reconciliation after scale to 2")
			waitForReconciliation(g, etcd)

			logger.Info("updating config on worker-0")
			updateWorkerConfig(g, ctx, cl, defaultEtcdName, testNamespace, 0)

			// Deploy static pod on worker-1
			logger.Info("deploying static pod on worker-1")
			if tc.tlsEnabled {
				copyTLSToWorker(g, defaultEtcdName, testNamespace, 1, etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir)
			}
			deployStaticPod(g, ctx, cl, defaultEtcdName, testNamespace, 1, saTokenFile, caCertFile)

			logger.Info("waiting for 2 members to be ready")
			testEnv.CheckEtcdReady(g, etcd, timeoutExtMembersReady)
			logger.Info("step 2: 2 members ready")

			testEnv.VerifyMemberLeases(g, etcd, workerIPs[:2], timeoutMemberLeases)
			testEnv.VerifyStatefulSetZeroReplicas(g, etcd)

			// --- Scale step 3: replicas=3, addresses=[ip0, ip1, ip2] ---
			logger.Info("step 3: scaling to 3 replicas")
			g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
			etcd.Spec.Replicas = 3
			etcd.Spec.ExternallyManagedMemberAddresses = workerIPs[:3]
			etcd.SetAnnotations(map[string]string{druidv1alpha1.DruidOperationAnnotation: druidv1alpha1.DruidOperationReconcile})
			g.Expect(cl.Update(ctx, etcd)).To(Succeed())

			logger.Info("waiting for druid reconciliation after scale to 3")
			waitForReconciliation(g, etcd)

			logger.Info("updating config on worker-0 and worker-1")
			updateWorkerConfig(g, ctx, cl, defaultEtcdName, testNamespace, 0)
			updateWorkerConfig(g, ctx, cl, defaultEtcdName, testNamespace, 1)

			// Deploy static pod on worker-2
			logger.Info("deploying static pod on worker-2")
			if tc.tlsEnabled {
				copyTLSToWorker(g, defaultEtcdName, testNamespace, 2, etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir)
			}
			deployStaticPod(g, ctx, cl, defaultEtcdName, testNamespace, 2, saTokenFile, caCertFile)

			logger.Info("waiting for 3 members to be ready")
			testEnv.CheckEtcdReady(g, etcd, timeoutExtMembersReady)
			logger.Info("step 3: all 3 members ready")

			testEnv.VerifyMemberLeases(g, etcd, workerIPs[:3], timeoutMemberLeases)
			testEnv.VerifyStatefulSetZeroReplicas(g, etcd)
			testEnv.VerifyNoServicesOrPDB(g, etcd)

			// Verify final Etcd status
			g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
			g.Expect(etcd.Status.Ready).ToNot(BeNil())
			g.Expect(*etcd.Status.Ready).To(BeTrue())

			logger.Info("test passed", "purpose", tc.purpose)
			testSucceeded = true
		})
	}
}
