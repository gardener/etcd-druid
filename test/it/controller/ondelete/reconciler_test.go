// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gardener/etcd-druid/api/core/v1alpha1/crds"
	"github.com/gardener/etcd-druid/test/it/assets"
	"github.com/gardener/etcd-druid/test/it/setup"
	testutils "github.com/gardener/etcd-druid/test/utils"

	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

const testNamespacePrefix = "ondelete-reconciler-test"

var sharedITTestEnv setup.DruidTestEnvironment

func TestMain(m *testing.M) {
	k8sVersion, err := assets.GetK8sVersionFromEnv()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to get the kubernetes version: %v\n", err)
		os.Exit(1)
	}
	if _, err := crds.IsK8sVersionEqualToOrAbove129(k8sVersion); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to compare k8s version: %v\n", err)
		os.Exit(1)
	}
	etcdCrd, err := assets.GetEtcdCrd(k8sVersion)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to get Etcd CRD: %v\n", err)
		os.Exit(1)
	}
	var closer setup.DruidTestEnvCloser
	sharedITTestEnv, closer, err = setup.NewDruidTestEnvironment("ondelete-reconciler", []*apiextensionsv1.CustomResourceDefinition{etcdCrd})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to create integration test environment: %v\n", err)
		os.Exit(1)
	}
	exitCode := m.Run()
	closer()
	os.Exit(exitCode)
}

func TestOnDeleteReconciler(t *testing.T) {
	env := initializeOnDeleteReconcilerTestEnv(t, sharedITTestEnv)

	tests := []struct {
		name string
		fn   func(t *testing.T, ns string, env reconcilerTestEnv)
	}{
		{"controller-boots-without-error-when-no-statefulset-exists", testControllerBootsWithoutError},
		{"rollingupdate-statefulset-is-ignored-by-the-controller", testRollingUpdateStsIgnored},
		{"startup-synthetic-create-event-picks-up-existing-rollout", testStartupPicksUpMidRollout},
		{"happy-path-rollout-deletes-outdated-pods-one-at-a-time", testHappyPathRollout},
		{"status-only-statefulset-update-does-not-cause-a-delete", testStatusOnlyUpdateDoesNotTrigger},
		{"holds-participating-outdated-pods-while-an-updated-pod-is-not-ready", testHoldsWhileUpdatedPodNotReady},
		{"deletes-ready-outdated-pod-only-after-updated-pods-are-ready", testDeletesOutdatedPodAfterUpdatedPodsReady},
		{"scale-up-holds-top-gate-until-pod-count-matches-desired", testScaleUpHoldsTopGate},
		{"quorum-veto-holds-when-a-delete-would-break-quorum", testQuorumVetoHolds},
		{"selects-a-follower-for-deletion-before-observed-leader", testSelectsFollowerBeforeObservedLeader},
		{"scale-up-with-template-change-does-not-delete-existing-pod", testScaleUpWithTemplateChangeHoldsExisting},
	}

	g := NewWithT(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ns := testutils.GenerateTestNamespaceName(t, testNamespacePrefix, 8)
			g.Expect(env.itTestEnv.CreateTestNamespace(ns)).To(Succeed())
			t.Logf("running test %q in namespace %q", t.Name(), ns)
			tc.fn(t, ns, env)
		})
	}
}

// testControllerBootsWithoutError is a basic sanity check after the controller is registered & started.
func testControllerBootsWithoutError(t *testing.T, ns string, env reconcilerTestEnv) {
	g := NewWithT(t)
	cl := env.itTestEnv.GetClient()
	sts := &appsv1.StatefulSet{}
	err := cl.Get(env.itTestEnv.GetContext(), client.ObjectKey{Name: "nonexistent", Namespace: ns}, sts)
	g.Expect(client.IgnoreNotFound(err)).To(Succeed())
}

// testStartupPicksUpMidRollout verifies that when the controller starts (or
// this reconciler observes a StatefulSet for the first time), it processes
// existing OnDelete StatefulSets whose pods are on an old revision. This is
// the guarantee that an operator restart during a rollout continues correctly.
func testStartupPicksUpMidRollout(t *testing.T, ns string, env reconcilerTestEnv) {
	ctx := env.itTestEnv.GetContext()
	cl := env.itTestEnv.GetClient()

	_, sts := createEtcdAndStatefulSet(ctx, t, cl, ns, 3)
	// All 3 pods on rev-old but Ready: a mid-rollout state discovered on startup.
	createStsPods(ctx, t, cl, sts, 3, "rev-old", true)
	setStsUpdateRevision(ctx, t, cl, sts, "rev-new")

	g := NewWithT(t)
	g.Eventually(func() []int { return deletedPodOrdinals(ctx, cl, sts) }, 30*time.Second, 500*time.Millisecond).Should(HaveLen(1))
}

// testHappyPathRollout simulates a full rollout. Envtest runs no StatefulSet
// controller, so the test recreates each deleted pod at the new revision and
// decides when it becomes Ready.
func testHappyPathRollout(t *testing.T, ns string, env reconcilerTestEnv) {
	ctx := env.itTestEnv.GetContext()
	cl := env.itTestEnv.GetClient()
	g := NewWithT(t)

	_, sts := createEtcdAndStatefulSet(ctx, t, cl, ns, 3)
	unavailablePods := func() []string { return unavailablePodNames(ctx, cl, sts) }
	createStsPods(ctx, t, cl, sts, 3, "rev-a", true)
	setStsUpdateRevision(ctx, t, cl, sts, "rev-a")
	g.Eventually(unavailablePods, 10*time.Second, 200*time.Millisecond).Should(BeEmpty())
	g.Consistently(unavailablePods, 3*time.Second, 500*time.Millisecond).Should(BeEmpty())

	setStsUpdateRevision(ctx, t, cl, sts, "rev-b")
	var replacedOrdinals []int
	for range 3 {
		var deleted []int
		g.Eventually(func() []int {
			deleted = deletedPodOrdinals(ctx, cl, sts)
			return deleted
		}, 30*time.Second, 200*time.Millisecond).Should(HaveLen(1))
		replacedOrdinals = append(replacedOrdinals, deleted[0])

		recreated := createStsPod(ctx, t, cl, sts, deleted[0], "rev-b", false)
		// While the recreated pod is not Ready, no other pod may be deleted.
		g.Consistently(unavailablePods, 3*time.Second, 200*time.Millisecond).Should(ConsistOf(recreated.Name))
		setPodReady(ctx, t, cl, recreated, true)
	}

	g.Expect(replacedOrdinals).To(ConsistOf(0, 1, 2))
	g.Eventually(unavailablePods, 10*time.Second, 200*time.Millisecond).Should(BeEmpty())
	g.Consistently(unavailablePods, 3*time.Second, 500*time.Millisecond).Should(BeEmpty())
}

// testHoldsWhileUpdatedPodNotReady verifies that while a pod at the target
// revision is not Ready (it has not rejoined quorum), no participating
// outdated pod is deleted.
func testHoldsWhileUpdatedPodNotReady(t *testing.T, ns string, env reconcilerTestEnv) {
	ctx := env.itTestEnv.GetContext()
	cl := env.itTestEnv.GetClient()

	_, sts := createEtcdAndStatefulSet(ctx, t, cl, ns, 3)
	// p0 was recreated by a prior delete and is still coming up; deleting p1 or p2 now would drop quorum.
	createStsPodsWithRevisionAndReady(ctx, t, cl, sts, []podSpec{
		{revision: "rev-new", ready: false},
		{revision: "rev-old", ready: true},
		{revision: "rev-old", ready: true},
	})
	setStsUpdateRevision(ctx, t, cl, sts, "rev-new")

	consistentlyPodNotDeleted(ctx, t, cl, stsPodKey(sts, 1), 5*time.Second, 500*time.Millisecond)
	consistentlyPodNotDeleted(ctx, t, cl, stsPodKey(sts, 2), 5*time.Second, 500*time.Millisecond)
}

// testDeletesOutdatedPodAfterUpdatedPodsReady verifies that a Ready outdated pod
// is retained while the updated pods are not Ready, and is deleted once they are.
func testDeletesOutdatedPodAfterUpdatedPodsReady(t *testing.T, ns string, env reconcilerTestEnv) {
	ctx := env.itTestEnv.GetContext()
	cl := env.itTestEnv.GetClient()

	_, sts := createEtcdAndStatefulSet(ctx, t, cl, ns, 3)
	pods := createStsPodsWithRevisionAndReady(ctx, t, cl, sts, []podSpec{
		{revision: "rev-old", ready: true},
		{revision: "rev-new", ready: false},
		{revision: "rev-new", ready: false},
	})
	setStsUpdateRevision(ctx, t, cl, sts, "rev-new")

	consistentlyPodNotDeleted(ctx, t, cl, stsPodKey(sts, 0), 5*time.Second, 500*time.Millisecond)

	setPodReady(ctx, t, cl, pods[1], true)
	setPodReady(ctx, t, cl, pods[2], true)
	g := NewWithT(t)
	g.Eventually(func() []int { return deletedPodOrdinals(ctx, cl, sts) }, 30*time.Second, 500*time.Millisecond).Should(Equal([]int{0}))
}

// testScaleUpHoldsTopGate covers the "1 -> 3 replicas + template change"
// scenario: only p0 exists on rev-old, sts.Spec.Replicas=3, updateRevision=rev-new.
// The top gate must hold on "pod count below desired" and p0 must NOT be
// deleted.
func testScaleUpHoldsTopGate(t *testing.T, ns string, env reconcilerTestEnv) {
	ctx := env.itTestEnv.GetContext()
	cl := env.itTestEnv.GetClient()

	_, sts := createEtcdAndStatefulSet(ctx, t, cl, ns, 3)
	// Only 1 of 3 pods exists, as if the STS controller has just been asked
	// to scale up but has not yet created p1 and p2.
	createStsPods(ctx, t, cl, sts, 1, "rev-old", true)
	setStsUpdateRevision(ctx, t, cl, sts, "rev-new")

	consistentlyPodNotDeleted(ctx, t, cl, stsPodKey(sts, 0), 5*time.Second, 500*time.Millisecond)
}

// testQuorumVetoHolds verifies that when the live cluster is one failure away
// from losing quorum, the controller refuses to delete an outdated pod whose
// restart would breach quorum.
func testQuorumVetoHolds(t *testing.T, ns string, env reconcilerTestEnv) {
	ctx := env.itTestEnv.GetContext()
	cl := env.itTestEnv.GetClient()

	// member-0 is Unhealthy: restarting member-1 or member-2 would leave 1 of 3
	// voters healthy, below the quorum of 2, whichever one is selected.
	env.etcdClient.SetMembers(liveMembers(3, healthyExcept(0)))
	t.Cleanup(func() { env.etcdClient.SetMembers(liveMembers(3, healthyExcept())) })

	_, sts := createEtcdAndStatefulSet(ctx, t, cl, ns, 3)
	// All pods look participating to Kubernetes; only the live etcd view shows
	// member-0 unhealthy, so the quorum check is what must stop the delete.
	createStsPods(ctx, t, cl, sts, 3, "rev-old", true)
	setStsUpdateRevision(ctx, t, cl, sts, "rev-new")

	for ordinal := range 3 {
		consistentlyPodNotDeleted(ctx, t, cl, stsPodKey(sts, ordinal), 5*time.Second, 500*time.Millisecond)
	}
}

// testRollingUpdateStsIgnored verifies the onDeleteStrategy predicate: a
// StatefulSet on the RollingUpdate strategy must never see a Reconcile from
// this controller, even when its pods are on an outdated revision.
func testRollingUpdateStsIgnored(t *testing.T, ns string, env reconcilerTestEnv) {
	ctx := env.itTestEnv.GetContext()
	cl := env.itTestEnv.GetClient()

	_, sts := createEtcdAndStatefulSet(ctx, t, cl, ns, 3)
	// Flip the strategy to RollingUpdate after creation so the ondelete
	// controller's predicate rejects every event on this STS.
	patchStsUpdateStrategy(ctx, t, cl, sts, appsv1.RollingUpdateStatefulSetStrategyType)
	createStsPods(ctx, t, cl, sts, 3, "rev-old", true)
	setStsUpdateRevision(ctx, t, cl, sts, "rev-new")

	for ordinal := range 3 {
		consistentlyPodNotDeleted(ctx, t, cl, stsPodKey(sts, ordinal), 3*time.Second, 500*time.Millisecond)
	}
}

// testStatusOnlyUpdateDoesNotTrigger verifies the updateRevisionChanged
// predicate: a status-only StatefulSet update (no change to updateRevision
// or strategy type) must not enqueue a Reconcile from the primary watch.
// The controller could still be re-entered via the Pod-owned watch, so we
// assert only that no delete occurs.
func testStatusOnlyUpdateDoesNotTrigger(t *testing.T, ns string, env reconcilerTestEnv) {
	ctx := env.itTestEnv.GetContext()
	cl := env.itTestEnv.GetClient()

	_, sts := createEtcdAndStatefulSet(ctx, t, cl, ns, 3)
	createStsPods(ctx, t, cl, sts, 3, "rev-a", true)
	setStsUpdateRevision(ctx, t, cl, sts, "rev-a")

	// All pods at target revision, nothing outdated. Bump only a non-relevant
	// status field (readyReplicas). If the predicate lets this through the
	// reconciler would still find no outdated pods and no-op, so absence of
	// deletion is the correct signal here.
	patchStsReadyReplicas(ctx, t, cl, sts, 3)

	g := NewWithT(t)
	g.Consistently(func() []int { return deletedPodOrdinals(ctx, cl, sts) }, 3*time.Second, 500*time.Millisecond).Should(BeEmpty())
}

// testSelectsFollowerBeforeObservedLeader verifies that when all outdated pods
// are participating, the controller deletes a follower before the leader
// reported by the live member list.
func testSelectsFollowerBeforeObservedLeader(t *testing.T, ns string, env reconcilerTestEnv) {
	ctx := env.itTestEnv.GetContext()
	cl := env.itTestEnv.GetClient()

	// The leader is at the highest ordinal, which an ordinal-only pick would choose first.
	const leaderOrdinal = 2
	env.etcdClient.SetMembers(liveMembersWithLeader(3, leaderOrdinal, healthyExcept()))
	t.Cleanup(func() { env.etcdClient.SetMembers(liveMembers(3, healthyExcept())) })

	_, sts := createEtcdAndStatefulSet(ctx, t, cl, ns, 3)
	createStsPods(ctx, t, cl, sts, 3, "rev-old", true)
	setStsUpdateRevision(ctx, t, cl, sts, "rev-new")

	g := NewWithT(t)
	g.Eventually(func() []int { return deletedPodOrdinals(ctx, cl, sts) }, 30*time.Second, 500*time.Millisecond).Should(HaveLen(1))
	consistentlyPodNotDeleted(ctx, t, cl, stsPodKey(sts, leaderOrdinal), 5*time.Second, 500*time.Millisecond)
}

// testScaleUpWithTemplateChangeHoldsExisting covers scaling from 1 -> 3 replicas with a template change in the same update.
// The existing participating member (p0, on the old revision) must NOT be deleted until the scale-up pods (p1, p2) exist and are ready.
// This tests guards against the controller prematurely deleting the only live member and causing avoidable downtime.
// The top gate holds on "pod count below desired" while p1/p2 are missing.
func testScaleUpWithTemplateChangeHoldsExisting(t *testing.T, ns string, env reconcilerTestEnv) {
	ctx := env.itTestEnv.GetContext()
	cl := env.itTestEnv.GetClient()

	// Start as a healthy single-member cluster that is being scaled to 3.
	env.etcdClient.SetMembers(liveMembers(1, healthyExcept()))
	t.Cleanup(func() { env.etcdClient.SetMembers(liveMembers(3, healthyExcept())) })

	_, sts := createEtcdAndStatefulSet(ctx, t, cl, ns, 3)
	// Only p0 exists (old revision, participating); the StatefulSet controller has not created p1/p2 yet.
	createStsPods(ctx, t, cl, sts, 1, "rev-old", true)
	setStsUpdateRevision(ctx, t, cl, sts, "rev-new")

	consistentlyPodNotDeleted(ctx, t, cl, stsPodKey(sts, 0), 5*time.Second, 500*time.Millisecond)
}
