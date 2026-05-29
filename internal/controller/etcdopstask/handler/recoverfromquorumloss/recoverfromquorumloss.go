// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package recoverfromquorumloss implements the handler for the RecoverFromQuorumLoss EtcdOpsTask.
package recoverfromquorumloss

import (
	"context"
	"fmt"
	"net/http"
	"time"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	taskhandler "github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	handlerutils "github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler/utils"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	k8sutils "github.com/gardener/etcd-druid/internal/utils/kubernetes"

	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	// ErrAdmitRecoverFromQuorumLoss is the error code for errors during the admit phase.
	ErrAdmitRecoverFromQuorumLoss druidapicommon.ErrorCode = "ERR_ADMIT_RECOVER_FROM_QUORUM_LOSS"
	// ErrExecuteRecoverFromQuorumLoss is the error code for errors during execution.
	ErrExecuteRecoverFromQuorumLoss druidapicommon.ErrorCode = "ERR_EXECUTE_RECOVER_FROM_QUORUM_LOSS"

	// defaultScaleDownTimeout is the default timeout for waiting for the StatefulSet to scale down to 0.
	defaultScaleDownTimeout = 60 * time.Second
	// defaultPodReadyTimeout is the default timeout for waiting for the single-member etcd pod to become ready.
	defaultPodReadyTimeout = 180 * time.Second
	// recoveryStepAnnotation is written on the EtcdOpsTask CR after each step completes, recording
	// the last completed step so Execute can skip already-done work on requeue.
	recoveryStepAnnotation = "druid.gardener.cloud/quorum-recovery-last-completed-step"
	// recoveryWaitStartAnnotation records the RFC3339 timestamp on the EtcdOpsTask CR when a wait
	// step (WaitScaleDown, WaitPodReady) was first entered, so the timeout can be enforced
	// across requeueues.
	recoveryWaitStartAnnotation = "druid.gardener.cloud/quorum-recovery-wait-start"
)

// recoveryStep identifies an individual step in the quorum-loss recovery sequence.
type recoveryStep string

const (
	stepSuspendReconciliation    recoveryStep = "SuspendReconciliation"
	stepDisableComponentProtect  recoveryStep = "DisableComponentProtection"
	stepScaleDown                recoveryStep = "ScaleDown"
	stepWaitScaleDown            recoveryStep = "WaitScaleDown"
	stepDeletePVCs               recoveryStep = "DeletePVCs"
	stepDeleteMemberLeases       recoveryStep = "DeleteMemberLeases"
	stepPatchConfigMap           recoveryStep = "PatchConfigMap"
	stepScaleUp                  recoveryStep = "ScaleUp"
	stepWaitPodReady             recoveryStep = "WaitPodReady"
	stepReenableReconciliation   recoveryStep = "ReenableReconciliation"
	stepReenableComponentProtect recoveryStep = "ReenableComponentProtection"
)

// stepOrder is the canonical execution sequence. Execute iterates this slice and
// skips any step whose index is <= the index of the last completed step.
var stepOrder = []recoveryStep{
	stepSuspendReconciliation,
	stepDisableComponentProtect,
	stepScaleDown,
	stepWaitScaleDown,
	stepDeletePVCs,
	stepDeleteMemberLeases,
	stepPatchConfigMap,
	stepScaleUp,
	stepWaitPodReady,
	stepReenableReconciliation,
	stepReenableComponentProtect,
}

// lastCompletedStepIndex returns the index of the last completed step recorded on the
// EtcdOpsTask CR, or -1 if no step annotation is present (i.e. execution has not started yet).
func lastCompletedStepIndex(task *druidv1alpha1.EtcdOpsTask) int {
	val, ok := task.Annotations[recoveryStepAnnotation]
	if !ok {
		return -1
	}
	for i, s := range stepOrder {
		if string(s) == val {
			return i
		}
	}
	return -1
}

// handler implements the taskhandler.Handler interface for the RecoverFromQuorumLoss task.
type handler struct {
	k8sClient     client.Client
	etcdReference types.NamespacedName
	taskReference types.NamespacedName
	config        druidv1alpha1.RecoverFromQuorumLossConfig
}

// New creates a new handler for the RecoverFromQuorumLoss task.
func New(k8sClient client.Client, task *druidv1alpha1.EtcdOpsTask, _ *http.Client) (taskhandler.Handler, error) {
	cfg := task.Spec.Config.RecoverFromQuorumLoss
	if cfg == nil {
		cfg = &druidv1alpha1.RecoverFromQuorumLossConfig{}
	}
	return &handler{
		k8sClient:     k8sClient,
		etcdReference: task.GetEtcdReference(),
		taskReference: types.NamespacedName{Name: task.Name, Namespace: task.Namespace},
		config:        *cfg,
	}, nil
}

func (h *handler) scaleDownTimeout() time.Duration {
	if h.config.ScaleDownTimeout != nil {
		return h.config.ScaleDownTimeout.Duration
	}
	return defaultScaleDownTimeout
}

func (h *handler) podReadyTimeout() time.Duration {
	if h.config.PodReadyTimeout != nil {
		return h.config.PodReadyTimeout.Duration
	}
	return defaultPodReadyTimeout
}

// Admit validates the preconditions for the RecoverFromQuorumLoss task.
func (h *handler) Admit(ctx context.Context) taskhandler.Result {
	etcd, errResult := handlerutils.GetEtcd(ctx, h.k8sClient, h.etcdReference, druidv1alpha1.LastOperationTypeAdmit)
	if errResult != nil {
		return *errResult
	}

	if etcd.Spec.Replicas <= 1 {
		return taskhandler.Result{
			Description: "Task is not applicable for a single-member etcd cluster. " +
				"The backup-restore sidecar handles automatic recovery for single-member clusters.",
			Error: druiderr.WrapError(
				fmt.Errorf("etcd %s/%s has only %d replica(s); this task requires a multi-member cluster",
					etcd.Namespace, etcd.Name, etcd.Spec.Replicas),
				ErrAdmitRecoverFromQuorumLoss,
				string(druidv1alpha1.LastOperationTypeAdmit),
				"single-member cluster does not require quorum loss recovery",
			),
			Requeue: false,
		}
	}

	if !etcd.IsBackupStoreEnabled() {
		return taskhandler.Result{
			Description: "Task requires a configured backup store. " +
				"Recovery restores etcd data from the latest snapshot; without a backup store there is nothing to restore from.",
			Error: druiderr.WrapError(
				fmt.Errorf("etcd %s/%s has no backup store configured", etcd.Namespace, etcd.Name),
				ErrAdmitRecoverFromQuorumLoss,
				string(druidv1alpha1.LastOperationTypeAdmit),
				"backup store is required for quorum loss recovery",
			),
			Requeue: false,
		}
	}

	if etcd.IsReady() {
		return taskhandler.Result{
			Description: "The etcd cluster is currently ready; no quorum loss detected. " +
				"task is only applicable when the cluster has lost quorum.",
			Error: druiderr.WrapError(
				fmt.Errorf("etcd %s/%s is in ready state; quorum loss not detected", etcd.Namespace, etcd.Name),
				ErrAdmitRecoverFromQuorumLoss,
				string(druidv1alpha1.LastOperationTypeAdmit),
				"etcd cluster is ready; quorum loss not detected",
			),
			Requeue: false,
		}
	}

	return taskhandler.Result{
		Description: "Admit check passed",
	}
}

// Execute performs the multi-step recovery from quorum loss, requeuing after each completed
// step so that progress is visible in the task's LastOperation status. On requeue, steps that
// have already been recorded in the EtcdOpsTask CR annotation are skipped.
//
//  1. Suspend etcd-druid reconciliation.
//  2. Disable etcd component protection.
//  3. Scale the StatefulSet down to 0.
//  4. Wait for the StatefulSet to reach 0 replicas.
//  5. Delete all PVCs.
//  6. Delete all member leases.
//  7. Patch the ConfigMap to configure a single-member cluster (member-0 only).
//  8. Scale up the StatefulSet to 1.
//  9. Wait for the single-member etcd pod to become ready.
//
// 10. Re-enable etcd-druid reconciliation (triggers scale-out to original replica count).
// 11. Re-enable etcd component protection.
func (h *handler) Execute(ctx context.Context) taskhandler.Result {
	etcd, errResult := handlerutils.GetEtcd(ctx, h.k8sClient, h.etcdReference, druidv1alpha1.LastOperationTypeExecution)
	if errResult != nil {
		return *errResult
	}
	task, errResult := handlerutils.GetEtcdOpsTask(ctx, h.k8sClient, h.taskReference, druidv1alpha1.LastOperationTypeExecution)
	if errResult != nil {
		return *errResult
	}

	resumeFrom := lastCompletedStepIndex(task) + 1

	for i := resumeFrom; i < len(stepOrder); i++ {
		step := stepOrder[i]

		var result *taskhandler.Result
		// Re-fetch Etcd before steps that patch its annotations to avoid stale-object conflicts.
		if step == stepReenableReconciliation || step == stepReenableComponentProtect {
			var r *taskhandler.Result
			etcd, r = handlerutils.GetEtcd(ctx, h.k8sClient, h.etcdReference, druidv1alpha1.LastOperationTypeExecution)
			if r != nil {
				return *r
			}
		}

		switch step {
		case stepSuspendReconciliation:
			result = h.runStep(ctx, task, step, "Suspended etcd-druid reconciliation",
				func() error { return h.suspendReconciliation(ctx, etcd) })

		case stepDisableComponentProtect:
			result = h.runStep(ctx, task, step, "Disabled etcd component protection",
				func() error { return h.disableComponentProtection(ctx, etcd) })

		case stepScaleDown:
			result = h.runStep(ctx, task, step, "Scaled StatefulSet down to 0 replicas",
				func() error { return h.scaleStatefulSetTo(ctx, etcd, 0) })

		case stepWaitScaleDown:
			scaledDown, err := h.isStsScaledDown(ctx, types.NamespacedName{
				Name:      druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta),
				Namespace: etcd.Namespace,
			})
			if err != nil {
				return taskhandler.Result{
					Description: "Failed to check StatefulSet scale-down status",
					Error: druiderr.WrapError(err, ErrExecuteRecoverFromQuorumLoss,
						string(druidv1alpha1.LastOperationTypeExecution),
						"failed to get StatefulSet for scale-down check"),
				}
			}
			if !scaledDown {
				if timedOut, elapsed := h.isWaitTimedOut(task, h.scaleDownTimeout()); timedOut {
					return taskhandler.Result{
						Description: fmt.Sprintf("Timed out waiting for StatefulSet to scale down after %s", elapsed.Round(time.Second)),
						Error: druiderr.WrapError(
							fmt.Errorf("StatefulSet did not scale down within %s", h.scaleDownTimeout()),
							ErrExecuteRecoverFromQuorumLoss,
							string(druidv1alpha1.LastOperationTypeExecution),
							"scale-down timeout exceeded"),
					}
				}
				if err := h.ensureWaitStartAnnotation(ctx, task); err != nil {
					return taskhandler.Result{
						Description: "Failed to record wait-start time for scale-down",
						Error: druiderr.WrapError(err, ErrExecuteRecoverFromQuorumLoss,
							string(druidv1alpha1.LastOperationTypeExecution),
							"failed to annotate EtcdOpsTask CR with wait-start time"),
					}
				}
				return taskhandler.Result{
					Description: "Waiting for StatefulSet to scale down to 0 replicas",
					Requeue:     true,
				}
			}
			result = h.runStep(ctx, task, step, "StatefulSet scaled down to 0 replicas", func() error { return nil })

		case stepDeletePVCs:
			result = h.runStep(ctx, task, step, "Deleted all PVCs",
				func() error { return h.deleteAllPVCs(ctx, etcd) })

		case stepDeleteMemberLeases:
			result = h.runStep(ctx, task, step, "Deleted all member leases",
				func() error { return h.deleteAllMemberLeases(ctx, etcd) })

		case stepPatchConfigMap:
			result = h.runStep(ctx, task, step, "Patched ConfigMap to single-member configuration",
				func() error { return h.updateConfigMapForSingleMember(ctx, etcd) })

		case stepScaleUp:
			result = h.runStep(ctx, task, step, "Scaled StatefulSet up to 1 replica",
				func() error { return h.scaleStatefulSetTo(ctx, etcd, 1) })

		case stepWaitPodReady:
			ready, err := h.isPodReady(ctx, types.NamespacedName{
				Name:      druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, 0),
				Namespace: etcd.Namespace,
			})
			if err != nil {
				return taskhandler.Result{
					Description: "Failed to check single-member etcd pod readiness",
					Error: druiderr.WrapError(err, ErrExecuteRecoverFromQuorumLoss,
						string(druidv1alpha1.LastOperationTypeExecution),
						"failed to get pod for readiness check"),
				}
			}
			if !ready {
				if timedOut, elapsed := h.isWaitTimedOut(task, h.podReadyTimeout()); timedOut {
					return taskhandler.Result{
						Description: fmt.Sprintf("Timed out waiting for single-member etcd pod to become ready after %s", elapsed.Round(time.Second)),
						Error: druiderr.WrapError(
							fmt.Errorf("pod did not become ready within %s", h.podReadyTimeout()),
							ErrExecuteRecoverFromQuorumLoss,
							string(druidv1alpha1.LastOperationTypeExecution),
							"pod-ready timeout exceeded"),
					}
				}
				if err := h.ensureWaitStartAnnotation(ctx, task); err != nil {
					return taskhandler.Result{
						Description: "Failed to record wait-start time for pod readiness",
						Error: druiderr.WrapError(err, ErrExecuteRecoverFromQuorumLoss,
							string(druidv1alpha1.LastOperationTypeExecution),
							"failed to annotate EtcdOpsTask CR with wait-start time"),
					}
				}
				return taskhandler.Result{
					Description: "Waiting for single-member etcd pod to become ready",
					Requeue:     true,
				}
			}
			result = h.runStep(ctx, task, step, "Single-member etcd pod is ready", func() error { return nil })

		case stepReenableReconciliation:
			result = h.runStep(ctx, task, step, "Re-enabled etcd-druid reconciliation",
				func() error { return h.enableReconciliation(ctx, etcd) })

		case stepReenableComponentProtect:
			result = h.runStep(ctx, task, step, "Re-enabled etcd component protection",
				func() error { return h.enableComponentProtection(ctx, etcd) })
		}

		if result != nil {
			return *result
		}
	}

	return taskhandler.Result{
		Description: "Recovery from quorum loss completed successfully. " +
			"etcd-druid will reconcile and bring up the remaining cluster members.",
	}
}

// Cleanup removes the cluster-control annotations from the Etcd resource if they are still
// present. This prevents the operator from being permanently locked out when the task fails
// mid-execution. The per-task recovery annotations (recoveryStepAnnotation,
// recoveryWaitStartAnnotation) live on the EtcdOpsTask itself and are garbage-collected with
// it after ttlSecondsAfterFinished, so no explicit removal is needed here.
func (h *handler) Cleanup(ctx context.Context) taskhandler.Result {
	etcd, errResult := handlerutils.GetEtcd(ctx, h.k8sClient, h.etcdReference, druidv1alpha1.LastOperationTypeCleanup)
	if errResult != nil {
		if errResult.Error != nil {
			return taskhandler.Result{
				Description: "Etcd object not found during cleanup; skipping annotation removal",
			}
		}
		return *errResult
	}
	hasSuspend := metav1.HasAnnotation(etcd.ObjectMeta, druidv1alpha1.SuspendEtcdSpecReconcileAnnotation)
	hasDisableProtection := metav1.HasAnnotation(etcd.ObjectMeta, druidv1alpha1.DisableEtcdComponentProtectionAnnotation)
	if !hasSuspend && !hasDisableProtection {
		return taskhandler.Result{
			Description: "No cleanup required for RecoverFromQuorumLoss task",
		}
	}
	patch := client.MergeFrom(etcd.DeepCopy())
	delete(etcd.Annotations, druidv1alpha1.SuspendEtcdSpecReconcileAnnotation)
	delete(etcd.Annotations, druidv1alpha1.DisableEtcdComponentProtectionAnnotation)
	if err := h.k8sClient.Patch(ctx, etcd, patch); err != nil {
		return taskhandler.Result{
			Description: "Failed to remove recovery annotations during cleanup",
			Error: druiderr.WrapError(err, ErrExecuteRecoverFromQuorumLoss,
				string(druidv1alpha1.LastOperationTypeCleanup),
				"failed to remove recovery annotations from Etcd resource"),
			Requeue: true,
		}
	}
	return taskhandler.Result{
		Description: "Cleanup completed: recovery annotations removed from Etcd resource",
	}
}

// runStep executes fn, and on success records the completed step in the EtcdOpsTask CR annotation
// and returns a Requeue result so the reconciler writes the step description to task status.
// Returns nil when the caller should continue to the next step in the same call (never happens
// here — every successful step triggers a requeue except the last, which is handled in Execute).
func (h *handler) runStep(ctx context.Context, task *druidv1alpha1.EtcdOpsTask, step recoveryStep, description string, fn func() error) *taskhandler.Result {
	if err := fn(); err != nil {
		return &taskhandler.Result{
			Description: fmt.Sprintf("Failed during step %s", step),
			Error: druiderr.WrapError(err, ErrExecuteRecoverFromQuorumLoss,
				string(druidv1alpha1.LastOperationTypeExecution),
				fmt.Sprintf("failed to execute step %s", step)),
		}
	}
	if err := h.recordCompletedStep(ctx, task, step); err != nil {
		return &taskhandler.Result{
			Description: fmt.Sprintf("Failed to record completion of step %s", step),
			Error: druiderr.WrapError(err, ErrExecuteRecoverFromQuorumLoss,
				string(druidv1alpha1.LastOperationTypeExecution),
				fmt.Sprintf("failed to annotate EtcdOpsTask CR after step %s", step)),
		}
	}
	// Return nil only for the last step so Execute can return the final success result.
	if step == stepReenableComponentProtect {
		return nil
	}
	return &taskhandler.Result{
		Description: description,
		Requeue:     true,
	}
}

// recordCompletedStep writes the step name into the EtcdOpsTask CR annotation so it can be
// skipped on the next reconcile invocation. It also clears the wait-start annotation
// when a wait step completes, since the deadline is no longer relevant.
func (h *handler) recordCompletedStep(ctx context.Context, task *druidv1alpha1.EtcdOpsTask, step recoveryStep) error {
	patch := client.MergeFrom(task.DeepCopy())
	if task.Annotations == nil {
		task.Annotations = make(map[string]string)
	}
	task.Annotations[recoveryStepAnnotation] = string(step)
	if step == stepWaitScaleDown || step == stepWaitPodReady {
		delete(task.Annotations, recoveryWaitStartAnnotation)
	}
	return h.k8sClient.Patch(ctx, task, patch)
}

// ensureWaitStartAnnotation records the current time on the EtcdOpsTask CR the first time a wait
// step is entered, so isWaitTimedOut can enforce the timeout across requeueues.
func (h *handler) ensureWaitStartAnnotation(ctx context.Context, task *druidv1alpha1.EtcdOpsTask) error {
	if metav1.HasAnnotation(task.ObjectMeta, recoveryWaitStartAnnotation) {
		return nil
	}
	patch := client.MergeFrom(task.DeepCopy())
	if task.Annotations == nil {
		task.Annotations = make(map[string]string)
	}
	task.Annotations[recoveryWaitStartAnnotation] = time.Now().UTC().Format(time.RFC3339)
	return h.k8sClient.Patch(ctx, task, patch)
}

// isWaitTimedOut returns true if more than timeout has elapsed since the wait-start annotation
// was written, along with the elapsed duration. Returns false if the annotation is absent or
// cannot be parsed (treated as not yet timed out).
func (h *handler) isWaitTimedOut(task *druidv1alpha1.EtcdOpsTask, timeout time.Duration) (bool, time.Duration) {
	val, ok := task.Annotations[recoveryWaitStartAnnotation]
	if !ok {
		return false, 0
	}
	start, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return false, 0
	}
	elapsed := time.Since(start)
	return elapsed > timeout, elapsed
}

// suspendReconciliation adds the suspend-reconciliation annotation if not already present.
func (h *handler) suspendReconciliation(ctx context.Context, etcd *druidv1alpha1.Etcd) error {
	if metav1.HasAnnotation(etcd.ObjectMeta, druidv1alpha1.SuspendEtcdSpecReconcileAnnotation) {
		return nil
	}
	patch := client.MergeFrom(etcd.DeepCopy())
	if etcd.Annotations == nil {
		etcd.Annotations = make(map[string]string)
	}
	etcd.Annotations[druidv1alpha1.SuspendEtcdSpecReconcileAnnotation] = "true"
	return h.k8sClient.Patch(ctx, etcd, patch)
}

// disableComponentProtection adds the disable-component-protection annotation if not already present.
func (h *handler) disableComponentProtection(ctx context.Context, etcd *druidv1alpha1.Etcd) error {
	if metav1.HasAnnotation(etcd.ObjectMeta, druidv1alpha1.DisableEtcdComponentProtectionAnnotation) {
		return nil
	}
	patch := client.MergeFrom(etcd.DeepCopy())
	if etcd.Annotations == nil {
		etcd.Annotations = make(map[string]string)
	}
	etcd.Annotations[druidv1alpha1.DisableEtcdComponentProtectionAnnotation] = "true"
	return h.k8sClient.Patch(ctx, etcd, patch)
}

// scaleStatefulSetTo scales the StatefulSet for the given Etcd to the specified number of replicas.
func (h *handler) scaleStatefulSetTo(ctx context.Context, etcd *druidv1alpha1.Etcd, replicas int32) error {
	stsKey := types.NamespacedName{
		Name:      druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta),
		Namespace: etcd.Namespace,
	}
	sts := &appsv1.StatefulSet{}
	if err := h.k8sClient.Get(ctx, stsKey, sts); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if sts.Spec.Replicas != nil && *sts.Spec.Replicas == replicas {
		return nil
	}
	patch := client.MergeFrom(sts.DeepCopy())
	sts.Spec.Replicas = ptr.To(replicas)
	return h.k8sClient.Patch(ctx, sts, patch)
}

// isStsScaledDown checks whether the StatefulSet has 0 running replicas.
func (h *handler) isStsScaledDown(ctx context.Context, stsKey types.NamespacedName) (bool, error) {
	sts := &appsv1.StatefulSet{}
	if err := h.k8sClient.Get(ctx, stsKey, sts); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	return sts.Status.Replicas == 0 && sts.Status.ReadyReplicas == 0, nil
}

// deleteAllPVCs deletes all PersistentVolumeClaims associated with the etcd cluster.
func (h *handler) deleteAllPVCs(ctx context.Context, etcd *druidv1alpha1.Etcd) error {
	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := h.k8sClient.List(ctx, pvcList,
		client.InNamespace(etcd.Namespace),
		client.MatchingLabels{
			"app.kubernetes.io/managed-by": "etcd-druid",
			"app.kubernetes.io/part-of":    etcd.Name,
		},
	); err != nil {
		return err
	}
	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		if err := h.k8sClient.Delete(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// deleteAllMemberLeases deletes all member leases for the etcd cluster.
func (h *handler) deleteAllMemberLeases(ctx context.Context, etcd *druidv1alpha1.Etcd) error {
	leaseList := &coordinationv1.LeaseList{}
	if err := h.k8sClient.List(ctx, leaseList,
		client.InNamespace(etcd.Namespace),
		client.MatchingLabels{
			druidv1alpha1.LabelComponentKey: common.ComponentNameMemberLease,
			"app.kubernetes.io/part-of":     etcd.Name,
		},
	); err != nil {
		return err
	}
	for i := range leaseList.Items {
		lease := &leaseList.Items[i]
		if err := h.k8sClient.Delete(ctx, lease); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// updateConfigMapForSingleMember updates the etcd ConfigMap to configure a single-member cluster
// using only member-0 (ordinal 0). This is required so that etcd starts in new-cluster mode
// instead of trying to rejoin the lost quorum.
//
// The following fields are updated as per the recovery guide:
//   - initial-cluster: reduced to member-0 only
//   - initial-cluster-state: set to "new"
//   - initial-advertise-peer-urls: reduced to member-0 only
//   - advertise-client-urls: reduced to member-0 only
func (h *handler) updateConfigMapForSingleMember(ctx context.Context, etcd *druidv1alpha1.Etcd) error {
	cmKey := types.NamespacedName{
		Name:      druidv1alpha1.GetConfigMapName(etcd.ObjectMeta),
		Namespace: etcd.Namespace,
	}
	cm := &corev1.ConfigMap{}
	if err := h.k8sClient.Get(ctx, cmKey, cm); err != nil {
		return err
	}

	configData, ok := cm.Data[common.EtcdConfigFileName]
	if !ok {
		return fmt.Errorf("key %q not found in ConfigMap %s/%s", common.EtcdConfigFileName, cm.Namespace, cm.Name)
	}

	var etcdConfig map[string]interface{}
	if err := yaml.Unmarshal([]byte(configData), &etcdConfig); err != nil {
		return fmt.Errorf("failed to unmarshal etcd config from ConfigMap %s/%s: %w", cm.Namespace, cm.Name, err)
	}

	member0Name := druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, 0)
	peerSvcName := druidv1alpha1.GetPeerServiceName(etcd.ObjectMeta)

	peerScheme := "http"
	if etcd.Spec.Etcd.PeerUrlTLS != nil {
		peerScheme = "https"
	}
	peerPort := serverPortOrDefault(etcd)

	clientScheme := "http"
	if etcd.Spec.Etcd.ClientUrlTLS != nil {
		clientScheme = "https"
	}
	clientPort := clientPortOrDefault(etcd)

	member0PeerURL := fmt.Sprintf("%s://%s.%s.%s.svc:%d", peerScheme, member0Name, peerSvcName, etcd.Namespace, peerPort)
	member0ClientURL := fmt.Sprintf("%s://%s.%s.%s.svc:%d", clientScheme, member0Name, peerSvcName, etcd.Namespace, clientPort)

	etcdConfig["initial-cluster"] = fmt.Sprintf("%s=%s", member0Name, member0PeerURL)
	etcdConfig["initial-cluster-state"] = "new"
	etcdConfig["initial-advertise-peer-urls"] = map[string][]string{
		member0Name: {member0PeerURL},
	}
	etcdConfig["advertise-client-urls"] = map[string][]string{
		member0Name: {member0ClientURL},
	}

	updatedConfig, err := yaml.Marshal(etcdConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal updated etcd config: %w", err)
	}

	patch := client.MergeFrom(cm.DeepCopy())
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data[common.EtcdConfigFileName] = string(updatedConfig)
	return h.k8sClient.Patch(ctx, cm, patch)
}

// isPodReady checks whether the given pod has its Ready condition set to True.
func (h *handler) isPodReady(ctx context.Context, podKey types.NamespacedName) (bool, error) {
	pod := &corev1.Pod{}
	if err := h.k8sClient.Get(ctx, podKey, pod); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return k8sutils.HasPodReadyConditionTrue(pod), nil
}

// enableReconciliation removes the suspend-reconciliation annotation and adds the
// operation=reconcile annotation to trigger an immediate reconciliation by etcd-druid.
func (h *handler) enableReconciliation(ctx context.Context, etcd *druidv1alpha1.Etcd) error {
	patch := client.MergeFrom(etcd.DeepCopy())
	if etcd.Annotations == nil {
		etcd.Annotations = make(map[string]string)
	}
	delete(etcd.Annotations, druidv1alpha1.SuspendEtcdSpecReconcileAnnotation)
	etcd.Annotations[druidv1alpha1.DruidOperationAnnotation] = druidv1alpha1.DruidOperationReconcile
	return h.k8sClient.Patch(ctx, etcd, patch)
}

// enableComponentProtection removes the disable-component-protection annotation from the Etcd resource.
func (h *handler) enableComponentProtection(ctx context.Context, etcd *druidv1alpha1.Etcd) error {
	if !metav1.HasAnnotation(etcd.ObjectMeta, druidv1alpha1.DisableEtcdComponentProtectionAnnotation) {
		return nil
	}
	patch := client.MergeFrom(etcd.DeepCopy())
	delete(etcd.Annotations, druidv1alpha1.DisableEtcdComponentProtectionAnnotation)
	return h.k8sClient.Patch(ctx, etcd, patch)
}

// serverPortOrDefault returns the etcd server (peer) port, defaulting to 2380 if nil.
func serverPortOrDefault(etcd *druidv1alpha1.Etcd) int32 {
	if etcd.Spec.Etcd.ServerPort != nil {
		return *etcd.Spec.Etcd.ServerPort
	}
	return common.DefaultPortEtcdPeer
}

// clientPortOrDefault returns the etcd client port, defaulting to 2379 if nil.
func clientPortOrDefault(etcd *druidv1alpha1.Etcd) int32 {
	if etcd.Spec.Etcd.ClientPort != nil {
		return *etcd.Spec.Etcd.ClientPort
	}
	return common.DefaultPortEtcdClient
}
