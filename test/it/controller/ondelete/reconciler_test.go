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
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
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

// TestOnDeleteReconciler runs all integration scenarios against a shared
// envtest environment; each scenario runs in its own namespace so state is
// isolated. Test names describe the behaviour being verified.
func TestOnDeleteReconciler(t *testing.T) {
	env := initializeOnDeleteReconcilerTestEnv(t, sharedITTestEnv)

	tests := []struct {
		name string
		fn   func(t *testing.T, ns string, env reconcilerTestEnv)
	}{
		{"controller-boots-without-error-when-no-statefulset-exists", testControllerBootsWithoutError},
		{"startup-synthetic-create-event-picks-up-existing-rollout", testStartupPicksUpMidRollout},
		{"happy-path-rollout-deletes-outdated-pods-one-at-a-time", testHappyPathRollout},
		{"step-3-holds-when-a-current-revision-pod-is-not-ready", testStep3HoldsUnreadyUpdated},
		{"scale-up-holds-top-gate-until-pod-count-matches-desired", testScaleUpHoldsTopGate},
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

// testControllerBootsWithoutError is a smoke test: the manager and reconciler
// were already started by initializeOnDeleteReconcilerTestEnv; this just
// asserts the environment is healthy by round-tripping a resource.
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
	setStsUpdateRevision(ctx, t, cl, sts, "rev-new")
	// All 3 pods on rev-old but Ready — a mid-rollout state discovered on
	// startup.
	createStsPods(ctx, t, cl, sts, 3, "rev-old", true)
	createMemberLeases(ctx, t, cl, ns, sts, followerFor)

	// Any one of the three pods must be deleted (Step 4 picks a follower — all
	// are followers here). Bounded wait: single reconcile plus a 10s
	// RequeueAfter ceiling gives plenty of margin.
	g := NewWithT(t)
	g.Eventually(func() int {
		var deleted int
		for i := 0; i < 3; i++ {
			key := types.NamespacedName{Name: fmt.Sprintf("%s-%d", sts.Name, i), Namespace: ns}
			pod := &corev1.Pod{}
			err := cl.Get(ctx, key, pod)
			if apierrorsNotFound(err) || (err == nil && pod.DeletionTimestamp != nil) {
				deleted++
			}
		}
		return deleted
	}, 30*time.Second, 500*time.Millisecond).Should(Equal(1))
}

// testHappyPathRollout verifies that a rollout that begins after the
// controller has started (StatefulSet Update event with a new updateRevision)
// deletes exactly one outdated pod per reconciliation cycle.
func testHappyPathRollout(t *testing.T, ns string, env reconcilerTestEnv) {
	ctx := env.itTestEnv.GetContext()
	cl := env.itTestEnv.GetClient()

	_, sts := createEtcdAndStatefulSet(ctx, t, cl, ns, 3)
	setStsUpdateRevision(ctx, t, cl, sts, "rev-a")
	// All pods start at rev-a and are Ready.
	pods := createStsPods(ctx, t, cl, sts, 3, "rev-a", true)
	createMemberLeases(ctx, t, cl, ns, sts, followerFor)

	// Give the controller time to observe the initial state (nothing to do).
	g := NewWithT(t)
	g.Consistently(func() int {
		var deleted int
		for _, p := range pods {
			pod := &corev1.Pod{}
			err := cl.Get(ctx, client.ObjectKeyFromObject(p), pod)
			if apierrorsNotFound(err) || (err == nil && pod.DeletionTimestamp != nil) {
				deleted++
			}
		}
		return deleted
	}, 3*time.Second, 500*time.Millisecond).Should(Equal(0))

	// Trigger a rollout by bumping updateRevision. No pods change revision —
	// they become outdated relative to the new target.
	setStsUpdateRevision(ctx, t, cl, sts, "rev-b")

	// Exactly one pod should be selected and deleted (Step 4).
	g.Eventually(func() int {
		var deleted int
		for _, p := range pods {
			pod := &corev1.Pod{}
			err := cl.Get(ctx, client.ObjectKeyFromObject(p), pod)
			if apierrorsNotFound(err) || (err == nil && pod.DeletionTimestamp != nil) {
				deleted++
			}
		}
		return deleted
	}, 30*time.Second, 500*time.Millisecond).Should(Equal(1))

	// And no more, because envtest doesn't recreate pods so the top gate holds
	// on "pod count below desired" forever after the first delete.
	g.Consistently(func() int {
		var deleted int
		for _, p := range pods {
			pod := &corev1.Pod{}
			err := cl.Get(ctx, client.ObjectKeyFromObject(p), pod)
			if apierrorsNotFound(err) || (err == nil && pod.DeletionTimestamp != nil) {
				deleted++
			}
		}
		return deleted
	}, 5*time.Second, 500*time.Millisecond).Should(Equal(1))
}

// testStep3HoldsUnreadyUpdated verifies that when a pod is at the target
// revision but not Ready (updated pod has not rejoined quorum), Step 3 holds
// and no participating outdated pod is deleted.
func testStep3HoldsUnreadyUpdated(t *testing.T, ns string, env reconcilerTestEnv) {
	ctx := env.itTestEnv.GetContext()
	cl := env.itTestEnv.GetClient()

	_, sts := createEtcdAndStatefulSet(ctx, t, cl, ns, 3)
	setStsUpdateRevision(ctx, t, cl, sts, "rev-new")

	// p0 is already at rev-new but NOT ready (simulates a pod just recreated
	// from a prior delete, still coming up). p1 and p2 are at rev-old and
	// participating. Step 3 must hold — deleting p1 or p2 would drop quorum.
	labels := sts.Spec.Selector.MatchLabels
	_ = createStsPodsWithRevisionAndReady(ctx, t, cl, sts, labels, map[int]podSpec{
		0: {revision: "rev-new", ready: false},
		1: {revision: "rev-old", ready: true},
		2: {revision: "rev-old", ready: true},
	})
	createMemberLeases(ctx, t, cl, ns, sts, followerFor)

	// Neither p1 nor p2 should be deleted while p0 is not Ready.
	consistentlyPodNotDeleted(ctx, t, cl, types.NamespacedName{Name: sts.Name + "-1", Namespace: ns}, 5*time.Second, 500*time.Millisecond)
	consistentlyPodNotDeleted(ctx, t, cl, types.NamespacedName{Name: sts.Name + "-2", Namespace: ns}, 5*time.Second, 500*time.Millisecond)
}

// testScaleUpHoldsTopGate covers the "1 -> 3 replicas + template change"
// scenario: only p0 exists on rev-old, sts.Spec.Replicas=3, updateRevision=rev-new.
// The top gate must hold on "pod count below desired" and p0 must NOT be
// deleted.
func testScaleUpHoldsTopGate(t *testing.T, ns string, env reconcilerTestEnv) {
	ctx := env.itTestEnv.GetContext()
	cl := env.itTestEnv.GetClient()

	_, sts := createEtcdAndStatefulSet(ctx, t, cl, ns, 3)
	setStsUpdateRevision(ctx, t, cl, sts, "rev-new")
	// Only 1 of 3 pods exists — as if the STS controller has just been asked
	// to scale up but has not yet created p1 and p2. p0 is on rev-old and
	// participating.
	createStsPods(ctx, t, cl, sts, 1, "rev-old", true)
	createMemberLeases(ctx, t, cl, ns, sts, followerFor)

	// p0 must NOT be deleted; the top gate holds.
	consistentlyPodNotDeleted(ctx, t, cl, types.NamespacedName{Name: sts.Name + "-0", Namespace: ns}, 5*time.Second, 500*time.Millisecond)
}
