---
title: StatefulSet Quorum-Aware Updates with OnDelete Strategy
dep-number: 07
creation-date: 2024-09-20
status: implemented
authors:
- "@anveshreddy18"
reviewers:
- "etcd-druid-maintainers"
---

# DEP-07: Druid Controlled Quorum-Aware Pod Updates via StatefulSet OnDelete Strategy

## Summary

Today, etcd-druid provisions etcd clusters as StatefulSets configured with the RollingUpdate update strategy. On a pod-template change, the StatefulSet controller rolls pods in a fixed highest-to-lowest ordinal order, with no awareness of etcd member health or role. When an existing member is already unhealthy, this ordering can cause an avoidable transient quorum loss during routine spec updates.

This proposal introduces configuring the StatefulSet with the `OnDelete` update strategy provided by Kubernetes. Under OnDelete, the StatefulSet controller does not roll pods automatically on pod-template changes; instead, a new dedicated controller in etcd-druid deletes and recreates pods in an order that accounts for member health and cluster role. Voluntary updates - image bumps, configuration changes, resource changes - are then propagated under druid's control, with significantly reduced risk of unintended quorum loss in multi-node clusters.

## Terminology

- **OnDelete**: A StatefulSet update strategy where pods are only updated when they are explicitly deleted. The StatefulSet controller does not automatically rollout the pods on template changes.
- **RollingUpdate**: A StatefulSet update strategy where the StatefulSet controller automatically updates pods one by one, from the highest ordinal to the lowest.
- **Quorum**: The minimum number of etcd members that must agree on a value for the cluster to make progress. For a 3-member cluster, quorum is 2.
- **Leader**: The etcd member responsible for handling client write requests and coordinating replication.
- **Follower**: An etcd member that replicates data from the leader and can serve linearizable reads.
- **Participating pod**: A pod whose etcd container is currently serving as a leader or follower in the cluster's quorum (i.e., it passes its readiness probe).
- **Non-participating pod**: A pod whose etcd member is not currently contributing to quorum, regardless of cause (process down, member not yet joined, network-partitioned etc.).

## Motivation

Currently etcd-druid deploys etcd clusters as StatefulSets with `RollingUpdate` strategy. The StatefulSet controller rolls pods from the highest ordinal to the lowest, without considering the health or role of individual etcd members. This creates a risk of unintended quorum loss.

Consider a 3-member cluster with pods P-0, P-1, and P-2. Suppose P-0 is currently unhealthy (it could be partitioned from the network, on a failing node, or hitting an internal etcd error) while P-1 and P-2 are healthy and form a quorum. An operator now triggers a routine spec update, e.g. an image bump. The StatefulSet controller starts at the highest ordinal, deletes P-2, and waits for it to come back ready before proceeding. During that window only P-1 is participating; with 1 of 3 members up, the cluster has lost quorum and stalls writes until P-2 returns. The same outcome repeats when P-1 is rolled next. The unhealthy P-0 (the pod whose deletion would have been safe, since it wasn't contributing anyway) is rolled last, by which time the avoidable downtime has already been incurred.

The following diagram illustrates how the `RollingUpdate` strategy can lead to quorum loss in this scenario:

<div align="center">
<img src="assets/07-rolling-update-state-diagram.png" alt="RollingUpdate state diagram showing quorum loss when an unhealthy pod exists" width="500">
</div>

The StatefulSet controller starts from Pod N (the highest ordinal), terminates it, and waits for the new pod to become ready. If the terminated pod is not the originally unhealthy one, cluster goes into a transient quorum loss with 2 members down.

Moving the pod update process under druid's control, via the `OnDelete` strategy, addresses three concerns:

1. **Reduce transient quorum loss.** Select outdated pods for deletion based on member health, so it doesn't prematurely remove a healthy member while an unhealthy one waits its turn in the sequence.
2. **Reduce unnecessary leader elections.** Update followers before the leader, so the leader-election does not happen as frequently during a rollout.
3. **Enable safer in-place volume changes.** A safer PVC resize procedure benefits from controlled replacement of pods, which is what `OnDelete` provides. The resize flow itself is out of scope for this DEP and is covered separately (see [Interaction with PVC Resizing](#interaction-with-pvc-resizing)).

For a single-node etcd cluster, concerns 1 and 2 do not apply: there is no quorum to preserve and no leader-election to avoid.

### Goals

- Prevent unintended quorum loss during voluntary pod updates by replacing the default StatefulSet rolling order with a druid-controlled, health-aware order.

### Non-Goals

- This proposal does not claim to prevent all forms of quorum loss. It reduces the likelihood of quorum loss that arises from applying the StatefulSet's default, health-agnostic rolling order to a quorum-sensitive workload.
- This proposal does not cover the PVC resizing or volume replacement flow itself. `OnDelete` is recommended for a safe resize procedure, but the resize flow is covered in a separate proposal (see [Interaction with PVC Resizing](#interaction-with-pvc-resizing)).

## Proposal

### Feature Gate

Quorum-aware pod updates with the `OnDelete` strategy are opt-in via a new feature gate, `QuorumAwareUpdates`, declared alongside the existing gates in `api/config/v1alpha1/features.go`. Operators enable it by setting it in the `featureGates` map of the operator configuration:

```yaml
# OperatorConfiguration
featureGates:
  QuorumAwareUpdates: true
```

- **Maturity:** Alpha
- **Default:** `false`
- **Scope:** Operator-wide. The gate applies to every etcd cluster managed by that etcd-druid instance; there is no per-cluster override.
- **Effect when enabled:** The Etcd reconciler sets `spec.updateStrategy.type = OnDelete` on every managed StatefulSet, and the OnDelete controller (described below) orchestrates pod updates.
- **Effect when disabled:** The Etcd reconciler sets `spec.updateStrategy.type = RollingUpdate` on the StatefulSet, preserving the current behaviour. The OnDelete controller's predicate does not match, so it stays inert.

**Why a feature gate.**

The choice of StatefulSet update strategy is an internal implementation detail of how etcd-druid realises an etcd cluster on top of Kubernetes primitives. The user's contract is the `Etcd` resource; they should not need to know that a StatefulSet is involved at all, let alone which of its update strategies is in effect. Exposing the choice as a spec field on `Etcd` would leak that implementation detail into a stable API surface and foreclose the option to evolve away from StatefulSets later without an awkward API deprecation.

A feature gate is the right mechanism for an operator-controlled toggle that is purely operational: it gives the operator a single switch to enable the new behaviour at a time of their choosing, and, critically, a safe rollback path if a regression is observed in production. Disabling the gate (or downgrading to a version that pre-dates it) restores the pre-OnDelete behaviour without any per-cluster cleanup. Once the feature is exercised long enough to be considered stable, the gate will be graduated through beta to GA and eventually locked to `true`, at which point no per-operator knob remains to maintain.

### The OnDelete Controller

A new controller, separate from the existing Etcd reconciler, is responsible for managing pod updates when the OnDelete strategy is active. This controller watches StatefulSet resources and reconciles whenever the StatefulSet's `.status.updateRevision` changes, indicating that a pod template update needs to be propagated to pods.

**Why a separate controller instead of extending the StatefulSet component:**

The OnDelete pod update process could be folded into the existing Etcd reconciliation loop by extending the StatefulSet component. We chose a separate controller for two reasons:

- **Preserves the Etcd reconciler's current role.** The reconciler today writes the desired state of the cluster's Kubernetes resources (StatefulSet, ConfigMap, Services, Leases, etc.) and relies on each resource's controller to act on it. It does not act on the individual pods. Adding the OnDelete update logic to the reconciler would extend its responsibilities into pod-lifecycle management for the first time; a separate controller keeps that boundary intact and lets the OnDelete logic evolve independently of the rest of the reconciliation pipeline.
- **Mirrors how `RollingUpdate` is managed today.** Under `RollingUpdate`, the Etcd reconciler writes the StatefulSet spec and the Kubernetes StatefulSet controller takes that spec and updates pods accordingly. Under `OnDelete`, the StatefulSet controller steps back from pod updates and a dedicated controller in etcd-druid takes on that role instead. The split (one component computes the desired StatefulSet spec, another manages the pods that realise it) is the same pattern; only the second half moves into etcd-druid.

**Coordination with the Etcd reconciler.**

The Etcd reconciler writes the StatefulSet spec; the OnDelete controller only deletes pods. They don't contend on the same field, so there is no update conflict on the StatefulSet object itself.

The OnDelete controller is stateless: every reconciliation re-reads the current StatefulSet's `.status.updateRevision` and the current pod set. If the Etcd reconciler pushes a new pod template while OnDelete is mid-rollout of a prior revision, the next reconciliation observes the new `updateRevision`, treats every pod whose `controller-revision-hash` no longer matches as outdated (including ones already updated against the prior revision), and continues the procedure from there. No explicit handover between controllers is needed.

When the feature gate's effective state changes, either at runtime (operator config update + restart) or as a result of a druid version upgrade/downgrade, the Etcd reconciler updates the StatefulSet's `spec.updateStrategy.type` first; the OnDelete controller's predicate (`spec.updateStrategy.type == OnDelete`) then engages or disengages accordingly. The Kubernetes StatefulSet controller and the OnDelete controller therefore never act on pod updates concurrently. See [Transitioning Between Strategies](#transitioning-between-strategies) for the full handover behaviour.

**Controller predicate:** The controller only reconciles StatefulSets whose `spec.updateStrategy.type` is set to `OnDelete`. This means the controller is always registered in the controller manager but has zero overhead for clusters using `RollingUpdate`.

### Determining Whether a Pod Needs Updating

A pod is considered outdated if its `controller-revision-hash` label does not match the StatefulSet's `.status.updateRevision`. The `controller-revision-hash` is a Kubernetes-managed label on each pod that reflects the revision of the pod template it was created from.

> **Note:** The OnDelete controller relies on the `controller-revision-hash` label being present on pods and on the StatefulSet's `.status.updateRevision` to signal the target revision. While `appsv1.StatefulSetRevisionLabel` is a public constant and the string comparison logic is safe regardless of how the hash is computed, the contract of how and whether the k8s populates this label on pods is not formally guaranteed. If Kubernetes changes how the StatefulSet controller determines whether pods are up-to-date, the OnDelete controller's outdated-pod detection may need to be revisited.
>
> The `controller-revision-hash` is computed from the pod template spec only. Changes to `volumeClaimTemplates` do not affect this hash. See [Interaction with PVC Resizing](#interaction-with-pvc-resizing).

### Health Assessment

The OnDelete controller needs two kinds of signal: per-pod signals that help it *select* which pod to update next, and a cluster-level signal that tells it whether a deletion is *safe*. The two per-pod signals, derived from Kubernetes state, are:

1. **Is the pod participating in the etcd quorum?** This is determined by the etcd container's readiness probe, which the kubelet evaluates against the pod's IP directly (it does not go through any Service). The probe hits etcd-wrapper's `/readyz` endpoint, which issues a linearizable `Get` against the local etcd process. Because a linearizable read can only be served with quorum:
   - A **passing** probe means the member is up and in contact with a healthy quorum: it is participating.
   - A **failing** probe means either this member is itself down or partitioned, or the cluster as a whole has lost quorum. The probe alone cannot distinguish these two cases: when quorum is lost, every pod's probe fails, including pods whose local etcd process is perfectly healthy.

2. **What is the etcd container's readiness state?** The controller classifies each pod into one of three readiness states derived from `pod.status.containerStatuses[]` for the etcd container:

   - **Not ready**: the etcd container status is published and its readiness probe is failing. The pod is not participating in quorum.
   - **Readiness unknown**: no status has been published yet.
   - **Ready**: the etcd container's readiness probe is succeeding; the pod is participating in quorum.

   This classification is used to *order* deletions (not-ready pods are updated first).

Both per-pod signals can be *lagging*: the readiness probe only flips a member to not-ready after its `failureThreshold` consecutive failures, so a member that has silently gone bad can still report ready for several probe periods. That lag is acceptable for *selecting* a pod to delete, but not for *deciding whether a deletion is safe*. For the safety decision the controller consults etcd directly, giving the cluster-level signal:

**Does the live cluster still hold quorum, and would removing this member break it?** The controller issues a single `MemberList` call to the cluster and reads, for each member, its live role and whether it is healthy right now. This live view, and not the lagging readiness probe, is what gates every deletion in the [Pod Update Procedure](#pod-update-procedure). Health is fail-closed: if a member cannot be reached, or the call fails outright, it is treated as unhealthy and the deletion is held rather than risked.


### Pod Update Procedure

The controller deletes at most one pod per reconciliation cycle. The invariant that protects quorum is this: it never deletes a pod whose removal would drop the live cluster below quorum. Before every deletion the controller checks the live member list (see [Health Assessment](#health-assessment)) and confirms that the healthy voting members surviving the removal still form a quorum; if they would not, it holds and requeues instead of deleting. Participating pods are therefore updated strictly one at a time, each only after the previously-updated pod has rejoined quorum.

Non-participating pods do not count toward quorum, so they carry no such constraint: the controller recreates them in successive cycles without waiting for each one to come back.

There is one deliberate exception to the one-at-a-time cadence. If the live check shows the cluster has *already* lost quorum, there is no longer a quorum to protect, and waiting one pod at a time only prolongs the outage. In that case the controller deletes all outdated pods together, so the StatefulSet recreates them at the new revision and might resolve the previous issues the cluster was facing.

This cadence applies to clusters of all sizes. Clusters with more than three replicas have the quorum headroom to update multiple *participating* pods concurrently, but that optimization is intentionally deferred: parallel updates would require handling partial-batch outcomes (one pod of a batch rejoining while another stalls), which significantly complicates the controller's decisions. The first iteration keeps the conservative one-at-a-time cadence for participating pods at every cluster size (see [Future Scope](#future-scope)).

The procedure in each reconciliation cycle:

**Step 1: Check if all pods are up to date.** Compare each pod's `controller-revision-hash` with the StatefulSet's `.status.updateRevision`. If all match, the update is complete.

**Intermediate Step** Once at least one pod is outdated and the pod set is settled (no deletion in flight and the StatefulSet has created all desired pods), the controller fetches the live member list once for the remainder of the cycle. Single-node clusters are the exception: they have no quorum to protect, so the controller skips the live fetch and quorum checks entirely and proceeds straight to deleting the sole outdated pod (see [Single-Node Clusters](#single-node-clusters)). If the live view shows quorum is already lost, the controller takes the recovery path described above - deleting all outdated pods together - and skips the per-pod steps below. Otherwise it proceeds, applying the quorum check to each candidate before deleting it.

**Step 2: Check for non-participating pods that need updating.** If any outdated pod is non-participating (etcd container not-ready), select one for deletion. Selection across all outdated pods uses the following order comparison table:

| Priority | Readiness | Role | Rationale |
|----------|-----------|------|-----------|
| First (delete first) | **Not ready** | any | Not participating in quorum, so a deletion cannot break it. Within this group, pick higher ordinal first for determinism. |
| Second | Ready or unknown | Learner | Non-voting, so a deletion costs nothing to quorum. |
| Third | Ready or unknown | Follower | Voting but not the leader, so deleting avoids a leader election. |
| Fourth | Ready or unknown | Unknown role | Role could not be determined, so treated conservatively like a follower. |
| Last (delete last) | Ready or unknown | **Leader** | Deleting the leader forces a leader election, so it is updated last. |

Within the not-ready group role is irrelevant (a non-participating member triggers no leader election). When there's a tie at any level, higher ordinal is preferred.

Delete the selected pod (subject to the quorum check below) and requeue. Deleting any of these pods causes the StatefulSet controller to recreate it at the new revision, so it leaves the outdated set and is never re-selected in this step. If a recreated pod cannot come back, the rollout holds at Step 3 (which waits for all updated pods to participate) rather than being retried here.

**Step 3: Ensure all updated pods are participating.** Before touching any participating pod, verify that every pod already carrying the latest revision is in a participating state. If any updated pod is still non-participating (for example, a pod deleted in a previous cycle has not yet rejoined the quorum), requeue and wait. This prevents the controller from removing a second participating member while a previously-updated one is still recovering.

**Step 4: Update one participating pod.** At this point, all outdated pods are participating. Select one for deletion using the same total ordering as Step 2 comparator within the ready/unknown group, learner → follower → unknown-role → leader.

> **Note:** The controller determines member roles from the live `MemberList` response (the same call used for the quorum check). The leader is identified from `StatusResponse.Leader` reported by any healthy member, not from a lease. Although the `MemberList` call is linearizable, the per-member `Status` probes that contain role and health data are made individually to each member endpoint after the list succeeds. A probe timeout or transient dial failure leaves that member's role as `Unknown`, which the comparator then treats as a follower. At worst this may cause the controller to delete what it believes is a follower when the member is in fact the leader, triggering one extra leader election.

> **Note:** When the leader pod is deleted with a normal grace period, the etcd server attempts a graceful [leadership transfer](https://github.com/etcd-io/etcd/blob/326d5a2e7765d1d918865d2c3897f0a27320db80/server/etcdserver/server.go#L1319-L1331) to a follower before shutting down. This transfer is **best-effort**: etcd initiates it during shutdown but does not wait for it to succeed, so a leader election can still occur if the handover does not complete in time. In the common case the handover succeeds and the cluster avoids a full leader-election downtime, which is why an explicit move-leader call from the controller before updating a leader is not needed.

Delete the selected pod (subject to the quorum check below) and requeue.

**Quorum check before every deletion.** Steps 2 and 4 select *which* pod to delete from Kubernetes-level signals, but the deletion itself is gated on the live cluster view. Immediately before deleting, the controller maps the selected pod to its etcd member and confirms, from the live member list, that the healthy voting members surviving the removal still form a quorum. If removal would be unsafe — or if the pod cannot be mapped to a live member, or the member list could not be fetched — the controller holds and requeues instead of deleting. This is what makes a stale readiness observation or an imprecise role determination tolerable: they can only misguide the *selection*, but never cause a deletion that breaches quorum, because the live check has the final say. Single-node clusters are exempt from this, as covered above.

**Step 5: Repeat** until all pods are up to date.

The following diagram summarizes the OnDelete update procedure:

<div align="center">
<img src="assets/07-OnDelete-StateDiagram.png" alt="OnDelete state diagram showing the simplified pod update procedure" width="700">
</div>

### Pod Deletion Method

The controller deletes pods by calling the pod `DELETE` endpoint (`client.Delete`), not the `pods/eviction` subresource. These are two distinct API operations: a pod `DELETE` does not consult the PodDisruptionBudget, whereas an eviction does. The OnDelete controller relies on its own health-aware logic to decide when a deletion is safe, so it does not need the PDB's budget check on top.

**Why Delete instead of Evict:**

Several approaches were evaluated:

1. **Eviction without `unhealthyPodEvictionPolicy`**: The PDB blocks all evictions when any pod is unready (since `minAvailable` drops to the threshold). This deadlocks the controller when a pod is already unhealthy, which is precisely the scenario OnDelete is designed to handle.

2. **Eviction with `unhealthyPodEvictionPolicy: IfHealthyBudget`**: Allows eviction of unhealthy pods when enough healthy pods exist. Handles the common case of one unhealthy pod, but still deadlocks when two or more pods are unhealthy.

3. **Eviction with `unhealthyPodEvictionPolicy: AlwaysAllow`**: Allows eviction of all unhealthy pods regardless of budget. Solves the deadlock but weakens PDB protection against external actors (VPA, node drain, cluster autoscaler) that could concurrently evict unhealthy pods.

4. **Direct Delete**: Simple and does not interact with the PDB at all.

The recommended approach is direct `Delete` for the following reasons:

- The current `RollingUpdate` strategy already deletes pods via the pod `DELETE` endpoint, not the `pods/eviction` subresource ([Kubernetes StatefulSet controller source](https://github.com/kubernetes/kubernetes/blob/865560c3d264865c2cf7e86863feb220e7848bba/pkg/controller/statefulset/stateful_pod_control.go#L118-L120)). Switching to `OnDelete` does not change the deletion mechanism, only the ordering.
- Using eviction would require managing PDB policy settings and handling blocked evictions with fallback logic, adding complexity without proportional safety benefit given that the controller is already health-aware.

**Limitation of the Delete approach:**

Because direct `Delete` bypasses the PDB, there is a theoretical race condition: an external actor (VPA, node drain) could disrupt a pod between the OnDelete controller's health check and its delete call, resulting in two pods being unavailable simultaneously. In practice this is rare because the window is very short, and the same race exists with the current `RollingUpdate` strategy which also uses direct deletions. If the race does occur, the controller's next reconciliation will observe the degraded state and pause further updates until the cluster recovers.

The PDB remains configured as today (`minAvailable = replicas/2 + 1` for multi-node clusters) and continues to protect against evictions from external actors like VPA, node drain, and cluster autoscaler. The OnDelete controller's direct deletes bypass the PDB by design, because the controller has domain knowledge that the PDB lacks (etcd member health, role, quorum status).

### Safeguarding Etcd Pods from Voluntary Disruptions

Two mechanisms protect etcd pods from being disrupted by external actors:

1. **PodDisruptionBudget (PDB)**: For a 3-member etcd cluster, `minAvailable` is set to 2, allowing at most 1 pod to be evicted at a time. For single-node clusters, `minAvailable` is 0 (PDB provides no protection). The PDB protects against evictions from VPA, node drain, and similar actors. The OnDelete controller bypasses the PDB via direct delete calls because it makes its own health-aware decisions.

2. **Cluster autoscaler `safe-to-evict` annotation**: The `cluster-autoscaler.kubernetes.io/safe-to-evict: "false"` annotation prevents the cluster autoscaler from evicting etcd pods during node scale-down.

### Single-Node Clusters

For single-node etcd clusters, the OnDelete controller follows the same procedure but with a simplification: there is only one pod, so there is no ordering decision to make. A single-node cluster also has no quorum to protect — deleting its sole pod is the only way to roll the update forward — so the controller skips the live member fetch and the quorum check entirely, deletes the single outdated pod, and waits for it to come back.

### StatefulSet Status Fields

The Kubernetes StatefulSet controller historically did not promote `status.currentRevision` to match `status.updateRevision` when the `OnDelete` strategy was used ([kubernetes/kubernetes#73492](https://github.com/kubernetes/kubernetes/issues/73492), [kubernetes/kubernetes#106055](https://github.com/kubernetes/kubernetes/issues/106055)). This has now been fixed upstream by [kubernetes/kubernetes#136833](https://github.com/kubernetes/kubernetes/pull/136833) and ships in Kubernetes `v1.37`. For clusters on Kubernetes versions below `v1.37`, however, the etcd-druid code that compares `currentRevision` with `updateRevision` (`IsStatefulSetReady` and the `AllMembersUpdated` condition) would incorrectly report the cluster as not ready under the `OnDelete` strategy.

To support both pre-fix and post-fix Kubernetes versions, the OnDelete controller computes the equivalent information directly from the pods: it compares each pod's `controller-revision-hash` with the StatefulSet's `updateRevision`. The existing readiness and condition checks will be updated to use this pod-level comparison when the `OnDelete` strategy is active. This pod-level approach is robust across Kubernetes versions, so it remains valid after etcd-druid raises its minimum supported Kubernetes version to one that includes the upstream fix.

### Transitioning Between Strategies

The Etcd reconciler is the single owner of `StatefulSet.spec.updateStrategy.type`. On every reconciliation it sets this field based on the current state of the `QuorumAwareUpdates` feature gate: enabled → `OnDelete`, disabled → `RollingUpdate`. This invariant means that whenever the gate's effective state changes, the StatefulSet's strategy follows on the next reconciliation, and no manual intervention is required to keep the two in sync.

The gate's effective state can change in two ways:

1. **Feature gate toggled at runtime.** The operator updates the gate value in `OperatorConfiguration.FeatureGates` and restarts the etcd-druid pod so the new value is loaded.
2. **Druid version upgrade or downgrade.** Upgrading to a version that introduces the gate (with the gate enabled) flips the effective state to `OnDelete`. Downgrading to a version that pre-dates the gate is equivalent to disabling it, because the older Etcd reconciler has no awareness of the gate and will unconditionally set `RollingUpdate`.

**Switch from `RollingUpdate` to `OnDelete`.**

1. On the next Etcd reconciliation, the StatefulSet component sets `spec.updateStrategy.type` to `OnDelete`. The Kubernetes StatefulSet controller stops auto-rolling pods.
2. The OnDelete controller's predicate now matches the StatefulSet and engages. If a `RollingUpdate` was in flight (some pods at the new revision, some not), the OnDelete controller observes the same `controller-revision-hash` labels and continues from where the previous controller left off, in health-aware order. Pods already updated by the StatefulSet controller are not re-deleted.
3. If no pods are outdated, the OnDelete controller simply watches for future template changes.
4. If the previous `RollingUpdate` had stalled waiting for some pod to come back, the OnDelete controller also waits for those; the wait behaviour is unchanged, only the selection order for subsequent pods differs.

**Switch from `OnDelete` to `RollingUpdate`.**

1. On the next Etcd reconciliation, the StatefulSet component sets `spec.updateStrategy.type` to `RollingUpdate`. The OnDelete controller's predicate no longer matches; it disengages.
2. The Kubernetes StatefulSet controller resumes pod updates in ordinal order. Because it also consults `controller-revision-hash` and `.status.updateRevision`, pods already updated by the OnDelete controller are not re-rolled: only the remaining outdated ones are processed.

Both switches are inherently safe because they touch only the StatefulSet's `spec.updateStrategy.type` field; no pod is deleted as part of the switch itself. A pod whose deletion is in flight when the switch happens completes its lifecycle normally and is replaced; the new strategy applies to subsequent pods.

**Note on version downgrade.** Downgrading to a version of etcd-druid that pre-dates this DEP is functionally identical to disabling the gate (the second switch above), since the older reconciler will set `RollingUpdate` on its next reconciliation. Any OnDelete rollout in progress at the time of the downgrade continues under Kubernetes StatefulSet control afterwards, with no loss of progress.

### VPA Interaction

The Vertical Pod Autoscaler (VPA) in `Recreate` or `InPlaceOrRecreate` mode does not modify the StatefulSet spec directly. VPA operates through two independent mechanisms:

1. **VPA Updater**: Evicts pods that don't match the recommended resource values. This eviction respects the PDB.
2. **VPA Admission Controller**: When the StatefulSet controller recreates a pod, the admission webhook injects VPA-recommended resource values into the new pod's spec at creation time.

Since VPA does not modify the StatefulSet pod template, it does not trigger a new `updateRevision` and the OnDelete controller is not involved in VPA-driven resource updates. VPA continues to operate independently regardless of the update strategy.

### Interaction with PVC Resizing

The PVC resizing story ([etcd-druid#481](https://github.com/gardener/etcd-druid/issues/481)) benefits from the `OnDelete` strategy: replacing pods one-by-one in a controlled, health-aware order is a safer way to carry out a per-pod resize. `OnDelete` is recommended for that flow rather than strictly required. The resize flow itself is out of scope for this DEP and will be covered in a separate proposal.

The OnDelete controller's own detection of outdated pods is based on the `controller-revision-hash` label, which is computed from the pod template spec only and does not include `volumeClaimTemplates`. A change to `storageCapacity` or `storageClass` alone will therefore not trigger the OnDelete controller; the PVC resize flow will handle pod replacement through its own mechanism.

### Future Scope

- **Backup-restore container health in update ordering**: The current design does not consider the health of the backup-restore sidecar container when deciding which pod to update next. The rationale is that backup-restore health does not affect quorum, and prioritizing it could lead to unnecessary leader elections (for example, if a pod with an unhealthy backup-restore happens to be the leader). If future operational experience shows value in considering backup-restore health as a secondary sorting criterion, the priority order can be extended.
- **Concurrent pod updates for larger clusters**: Clusters with more than three replicas have quorum headroom to update multiple participating pods at once. For example, a 5-member cluster can lose 2 and keep quorum, a 7-member cluster can lose 3, and so on. A future iteration can introduce a quorum-aware concurrency limit (a `maxUnavailable`-like bound reinterpreted for quorum-based workloads) so that the controller updates as many participating pods in parallel as the cluster's headroom allows, while never breaching quorum.

## Alternatives

### RollingUpdate with `maxUnavailable`

Kubernetes supports a `maxUnavailable` field on the StatefulSet's `RollingUpdate` strategy ([StatefulSet documentation](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#maximum-unavailable-pods)). Setting `maxUnavailable: 1` ensures that only one pod is unavailable during the update process, which provides the same quorum safety guarantee as OnDelete for the common case.

However, `maxUnavailable` has two limitations that OnDelete addresses:

1. **Update progress when a pod is already unhealthy.** The StatefulSet controller waits for any unavailable pod to recover before proceeding. If a pod is already unhealthy and the update itself might fix it (for example, a configuration change or image bump), the controller is stuck waiting indefinitely. The OnDelete controller does not have this limitation because it can proactively delete the unhealthy pod first, allowing the update to make progress.

2. **No control over update order.** The StatefulSet controller still follows the ordinal order (highest to lowest) regardless of cluster health state. The OnDelete controller can prioritize non-participating pods and update followers before the leader, minimizing unnecessary leader elections during the rollout.
