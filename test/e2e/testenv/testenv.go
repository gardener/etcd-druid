// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package testenv

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	"github.com/gardener/etcd-druid/internal/component/clientservice"
	"github.com/gardener/etcd-druid/internal/component/configmap"
	"github.com/gardener/etcd-druid/internal/component/memberlease"
	"github.com/gardener/etcd-druid/internal/component/peerservice"
	"github.com/gardener/etcd-druid/internal/component/poddistruptionbudget"
	"github.com/gardener/etcd-druid/internal/component/role"
	"github.com/gardener/etcd-druid/internal/component/rolebinding"
	"github.com/gardener/etcd-druid/internal/component/serviceaccount"
	"github.com/gardener/etcd-druid/internal/component/snapshotlease"
	"github.com/gardener/etcd-druid/internal/component/statefulset"
	"github.com/gardener/etcd-druid/internal/images"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

const (
	defaultPollingInterval       = 2 * time.Second
	jobNameZeroDowntimeValidator = "zero-downtime-validator"
	jobNameEtcdLoader            = "etcd-loader"
	jobNameSnapshotter           = "snapshotter"
)

// TestEnvironment encapsulates the test environment for e2e tests.
type TestEnvironment struct {
	ctx       context.Context
	cancelCtx context.CancelFunc
	cl        client.Client
}

// NewTestEnvironment creates a new TestEnvironment instance.
func NewTestEnvironment(ctx context.Context, cancelCtx context.CancelFunc, cl client.Client) *TestEnvironment {
	return &TestEnvironment{
		ctx:       ctx,
		cancelCtx: cancelCtx,
		cl:        cl,
	}
}

// Context returns the context of the test environment.
func (t *TestEnvironment) Context() context.Context {
	return t.ctx
}

// Client returns the client of the test environment.
func (t *TestEnvironment) Client() client.Client {
	return t.cl
}

// Close cancels the context of the test environment.
func (t *TestEnvironment) Close() {
	t.cancelCtx()
}

// PrepareScheme adds the druidv1alpha1 scheme to the test environment's scheme.
func (t *TestEnvironment) PrepareScheme() error {
	return druidv1alpha1.AddToScheme(scheme.Scheme)
}

// CreateTestNamespace creates a namespace with the given name.
func (t *TestEnvironment) CreateTestNamespace(name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	return t.cl.Create(t.ctx, ns)
}

// DeleteTestNamespace deletes the namespace with the given name.
func (t *TestEnvironment) DeleteTestNamespace(name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	return t.cl.Delete(t.ctx, ns)
}

// DeletePVCs deletes all PVCs in the given namespace.
func (t *TestEnvironment) DeletePVCs(namespace string) error {
	pvcs := &corev1.PersistentVolumeClaimList{}
	if err := t.cl.List(t.ctx, pvcs, client.InNamespace(namespace)); err != nil {
		return err
	}

	for _, pvc := range pvcs.Items {
		if err := t.cl.Delete(t.ctx, &pvc); err != nil {
			return err
		}
	}

	return nil
}

// GetEtcd returns the Etcd object with the given name and namespace.
func (t *TestEnvironment) GetEtcd(name, namespace string) (*druidv1alpha1.Etcd, error) {
	etcd := &druidv1alpha1.Etcd{}
	err := t.cl.Get(t.ctx, types.NamespacedName{Name: name, Namespace: namespace}, etcd)
	if err != nil {
		return nil, err
	}
	return etcd, nil
}

// CreateAndCheckEtcd creates an etcd object and checks if it is ready.
func (t *TestEnvironment) CreateAndCheckEtcd(g *WithT, etcd *druidv1alpha1.Etcd, timeout time.Duration) {
	g.Expect(t.cl.Create(t.ctx, etcd)).To(Succeed())
	t.CheckEtcdReady(g, etcd, timeout)
}

// HibernateAndCheckEtcd hibernates the Etcd object and checks if it is in hibernated state.
func (t *TestEnvironment) HibernateAndCheckEtcd(g *WithT, etcd *druidv1alpha1.Etcd, timeout time.Duration) {
	etcd.Spec.Replicas = 0
	etcd.SetAnnotations(map[string]string{druidv1alpha1.DruidOperationAnnotation: druidv1alpha1.DruidOperationReconcile})
	g.Expect(t.cl.Update(t.ctx, etcd)).To(Succeed())
	t.CheckEtcdReady(g, etcd, timeout)
}

// UnhibernateAndCheckEtcd unhibernates the Etcd object and checks if it is in unhibernated state.
func (t *TestEnvironment) UnhibernateAndCheckEtcd(g *WithT, etcd *druidv1alpha1.Etcd, replicas int32, timeout time.Duration) {
	etcd.Spec.Replicas = replicas
	etcd.SetAnnotations(map[string]string{druidv1alpha1.DruidOperationAnnotation: druidv1alpha1.DruidOperationReconcile})
	g.Expect(t.cl.Update(t.ctx, etcd)).To(Succeed())
	t.CheckEtcdReady(g, etcd, timeout)
}

// UpdateAndCheckEtcd updates the Etcd object and checks if the update took effect.
func (t *TestEnvironment) UpdateAndCheckEtcd(g *WithT, etcd *druidv1alpha1.Etcd, timeout time.Duration) {
	etcd.SetAnnotations(map[string]string{druidv1alpha1.DruidOperationAnnotation: druidv1alpha1.DruidOperationReconcile})
	g.Expect(t.cl.Update(t.ctx, etcd)).To(Succeed())
	t.CheckEtcdReady(g, etcd, timeout)
}

// CheckEtcdReady checks if the Etcd object is ready.
func (t *TestEnvironment) CheckEtcdReady(g *WithT, etcd *druidv1alpha1.Etcd, timeout time.Duration) {
	g.Eventually(func() error {
		ctx, cancelFunc := context.WithTimeout(t.ctx, timeout)
		defer cancelFunc()

		if err := t.cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd); err != nil {
			return err
		}

		// Ensure the etcd cluster's current generation matches the observed generation
		if etcd.Status.ObservedGeneration == nil {
			return fmt.Errorf("etcd %s status observed generation is nil", etcd.Name)
		}
		if *etcd.Status.ObservedGeneration != etcd.Generation {
			return fmt.Errorf("etcd '%s' is not at the expected generation (observed: %d, expected: %d)", etcd.Name, *etcd.Status.ObservedGeneration, etcd.Generation)
		}

		etcdPods, err := t.getEtcdPods(etcd)
		if err != nil {
			return fmt.Errorf("failed to get etcd pods: %w", err)
		}
		if len(etcdPods) != int(etcd.Spec.Replicas) {
			return fmt.Errorf("etcd %s has %d pods, expected %d", etcd.Name, len(etcdPods), etcd.Spec.Replicas)
		}

		// if replicas is 0, the subsequent checks do not apply
		if etcd.Spec.Replicas == 0 {
			return nil
		}

		for _, pod := range etcdPods {
			if pod.Status.Phase != corev1.PodRunning {
				return fmt.Errorf("etcd %s pod %s is not running", etcd.Name, pod.Name)
			}
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if !containerStatus.Ready {
					return fmt.Errorf("etcd %s pod %s container %s is not ready", etcd.Name, pod.Name, containerStatus.Name)
				}
			}
		}

		if etcd.Status.Ready == nil || *etcd.Status.Ready != true {
			return fmt.Errorf("etcd %s is not ready", etcd.Name)
		}

		if len(etcd.Status.Conditions) == 0 {
			return fmt.Errorf("etcd %s status conditions is empty", etcd.Name)
		}

		for _, c := range etcd.Status.Conditions {
			// TODO: re-add this check for when store is configured, once the BackupReady is split into two conditions and becomes deterministic
			// Ref: https://github.com/gardener/etcd-druid/issues/867
			if c.Type == druidv1alpha1.ConditionTypeBackupReady {
				continue
			}
			if c.Type == druidv1alpha1.ConditionTypeClusterIDMismatch {
				if c.Status != druidv1alpha1.ConditionFalse {
					return fmt.Errorf("etcd %q status %q condition %s is not False",
						etcd.Name, c.Type, c.Status)
				}
			} else {
				if c.Status != druidv1alpha1.ConditionTrue {
					return fmt.Errorf("etcd %q status %q condition %s is not True",
						etcd.Name, c.Type, c.Status)
				}
			}
		}
		return nil
	}, timeout, defaultPollingInterval).Should(Succeed())
}

// getOperatorRegistry returns the operator registry used by the Etcd operator.
func (t *TestEnvironment) getOperatorRegistry() (component.Registry, error) {
	imageVector, err := images.CreateImageVector()
	if err != nil {
		return nil, fmt.Errorf("failed to create image vector: %w", err)
	}
	reg := component.NewRegistry()
	reg.Register(component.ServiceAccountKind, serviceaccount.New(t.Client(), true))
	reg.Register(component.RoleKind, role.New(t.Client()))
	reg.Register(component.RoleBindingKind, rolebinding.New(t.Client()))
	reg.Register(component.MemberLeaseKind, memberlease.New(t.Client()))
	reg.Register(component.SnapshotLeaseKind, snapshotlease.New(t.Client()))
	reg.Register(component.PodDisruptionBudgetKind, poddistruptionbudget.New(t.Client()))
	reg.Register(component.ClientServiceKind, clientservice.New(t.Client()))
	reg.Register(component.PeerServiceKind, peerservice.New(t.Client()))
	reg.Register(component.ConfigMapKind, configmap.New(t.Client()))
	reg.Register(component.StatefulSetKind, statefulset.New(t.Client(), imageVector))

	return reg, nil
}

// DeleteAndCheckEtcd deletes the Etcd object and checks if it is fully deleted.
func (t *TestEnvironment) DeleteAndCheckEtcd(g *WithT, logger logr.Logger, etcd *druidv1alpha1.Etcd, timeout time.Duration) {
	operatorRegistry, err := t.getOperatorRegistry()
	g.Expect(err).ToNot(HaveOccurred(), "Failed to get operator registry")
	componentOperators := operatorRegistry.AllOperators()

	g.Expect(t.cl.Delete(t.ctx, etcd, client.PropagationPolicy(metav1.DeletePropagationForeground))).To(Succeed())
	g.Eventually(func() error {
		ctx, cancelFunc := context.WithTimeout(t.ctx, timeout)
		defer cancelFunc()

		if err := t.cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd); !apierrors.IsNotFound(err) {
			return fmt.Errorf("etcd %s is not deleted", etcd.Name)
		}

		opCtx := component.NewOperatorContext(ctx, logr.Discard(), uuid.NewString())
		for kind, operator := range componentOperators {
			existingResourceNames, err := operator.GetExistingResourceNames(opCtx, etcd.ObjectMeta)
			if err != nil {
				return fmt.Errorf("failed to get existing resource names for operator %s: %w", kind, err)
			}
			if len(existingResourceNames) > 0 {
				return fmt.Errorf("etcd %s still has %d resources of kind %s", etcd.Name, len(existingResourceNames), kind)
			}
		}

		return nil
	}, timeout, defaultPollingInterval).Should(Succeed())
}

// VerifyEtcdPodLabels verifies that the Etcd pods have the expected labels.
func (t *TestEnvironment) VerifyEtcdPodLabels(g *WithT, etcd *druidv1alpha1.Etcd, expectedLabels map[string]string) {
	g.Expect(testutils.CheckEtcdPodLabels(t.ctx, t.cl, etcd, expectedLabels)).To(Succeed())
}

// VerifyEtcdMemberPeerTLSEnabled verifies that the Etcd member has the expected peer TLS enablement status.
func (t *TestEnvironment) VerifyEtcdMemberPeerTLSEnabled(g *WithT, etcd *druidv1alpha1.Etcd) {
	g.Expect(testutils.VerifyPeerTLSEnabledOnAllMemberLeases(t.ctx, t.cl, etcd)).To(Succeed())
}

// DisruptEtcd disrupts the etcd object by deleting its pods and/or deleting PVCs to simulate transient failures.
func (t *TestEnvironment) DisruptEtcd(g *WithT, etcd *druidv1alpha1.Etcd, numPodsToDelete, numPVCsToDelete int, timeout time.Duration) {
	if numPodsToDelete <= 0 && numPVCsToDelete <= 0 {
		return
	}

	pods, err := t.getEtcdPods(etcd)
	g.Expect(err).ToNot(HaveOccurred(), "Failed to get etcd pods")
	g.Expect(len(pods)).To(Equal(int(etcd.Spec.Replicas)), "Number of pods does not match the expected replicas")
	g.Expect(len(pods)).To(BeNumerically(">=", numPodsToDelete), "Not enough pods to delete")
	pvcs, err := t.getEtcdPVCs(etcd)
	g.Expect(err).ToNot(HaveOccurred(), "Failed to get etcd PVCs")
	g.Expect(len(pvcs)).To(Equal(int(etcd.Spec.Replicas)), "Number of PVCs does not match the expected replicas")
	g.Expect(len(pvcs)).To(BeNumerically(">=", numPVCsToDelete), "Not enough PVCs to delete")

	var deletedPVCUIDs []string
	for i, pvc := range pvcs[:numPVCsToDelete] {
		g.Expect(t.cl.Delete(t.ctx, &pvc, client.PropagationPolicy(metav1.DeletePropagationBackground))).To(Succeed())
		deletedPVCUIDs = append(deletedPVCUIDs, string(pvcs[i].UID))
	}

	var deletedPodUIDs []string
	for i, pod := range pods[:numPodsToDelete] {
		g.Expect(t.cl.Delete(t.ctx, &pod)).To(Succeed())
		deletedPodUIDs = append(deletedPodUIDs, string(pods[i].UID))
	}

	g.Eventually(func() error {
		pods, err := t.getEtcdPods(etcd)
		g.Expect(err).ToNot(HaveOccurred(), "Failed to get etcd pods")
		for _, pod := range pods {
			if slices.Contains(deletedPodUIDs, string(pod.UID)) {
				return fmt.Errorf("pod %s is not yet deleted", pod.Name)
			}
		}
		pvcs, err := t.getEtcdPVCs(etcd)
		g.Expect(err).ToNot(HaveOccurred(), "Failed to get etcd PVCs")
		for _, pvc := range pvcs {
			if slices.Contains(deletedPodUIDs, string(pvc.UID)) {
				return fmt.Errorf("pvc %s is not yet deleted", pvc.Name)
			}
		}
		return nil
	}, timeout, defaultPollingInterval).Should(Succeed())
}

// getEtcdPods returns the pods of the etcd object.
func (t *TestEnvironment) getEtcdPods(etcd *druidv1alpha1.Etcd) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	if err := t.cl.List(t.ctx, podList, client.InNamespace(etcd.Namespace), client.MatchingLabels(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta))); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	return podList.Items, nil
}

// getEtcdPVCs returns the PVCs associated with the Etcd pods.
func (t *TestEnvironment) getEtcdPVCs(etcd *druidv1alpha1.Etcd) ([]corev1.PersistentVolumeClaim, error) {
	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := t.cl.List(t.ctx, pvcList, client.InNamespace(etcd.Namespace), client.MatchingLabels(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta))); err != nil {
		return nil, fmt.Errorf("failed to list PVCs: %w", err)
	}
	return pvcList.Items, nil
}

// DeployZeroDowntimeValidatorJob deploys the zero downtime validator job.
func (t *TestEnvironment) DeployZeroDowntimeValidatorJob(g *WithT, namespace, etcdClientServiceName string, etcdClientServicePort int32, etcdClientTLS *druidv1alpha1.TLSConfig, timeout time.Duration) {
	zdvJob := getZeroDowntimeValidatorJob(namespace, etcdClientServiceName, etcdClientServicePort, etcdClientTLS)
	g.Expect(t.cl.Create(t.ctx, zdvJob)).To(Succeed())

	// Wait for the job to start
	g.Eventually(func() error {
		job := &batchv1.Job{}
		err := t.cl.Get(t.ctx, types.NamespacedName{Name: zdvJob.Name, Namespace: namespace}, job)
		if err != nil {
			return err
		}

		if job.Status.Ready == nil || *job.Status.Ready == 0 {
			return fmt.Errorf("job %s is not ready", job.Name)
		}
		return nil
	}, timeout, defaultPollingInterval).Should(Succeed())
}

// CheckForDowntime checks the downtime from the zero downtime validator job.
func (t *TestEnvironment) CheckForDowntime(g *WithT, namespace string, downtimeExpected bool) {
	job := &batchv1.Job{}
	g.Expect(t.cl.Get(t.ctx, types.NamespacedName{Name: jobNameZeroDowntimeValidator, Namespace: namespace}, job)).To(Succeed())

	if !downtimeExpected {
		g.Expect(job.Status.Failed).To(BeZero())
	}
}

// getZeroDowntimeValidatorJob returns the job object for zero downtime validation.
func getZeroDowntimeValidatorJob(namespace, etcdClientServiceName string, etcdClientServicePort int32, etcdClientTLS *druidv1alpha1.TLSConfig) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobNameZeroDowntimeValidator,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "validator",
							Image:           "alpine/curl",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/bin/sh", "-c"},
							Args:            []string{getHealthCheckScript(etcdClientServiceName, etcdClientServicePort)},
							VolumeMounts: []corev1.VolumeMount{
								{MountPath: common.VolumeMountPathEtcdCA, Name: testutils.ClientTLSCASecretName},
								{MountPath: common.VolumeMountPathEtcdServerTLS, Name: testutils.ClientTLSServerCertSecretName},
								{MountPath: common.VolumeMountPathEtcdClientTLS, Name: testutils.ClientTLSClientCertSecretName, ReadOnly: true},
							},
							ReadinessProbe: getProbe("/tmp/healthy"),
							LivenessProbe:  getProbe("/tmp/healthy"),
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{
						getTLSVolume(testutils.ClientTLSCASecretName, etcdClientTLS.TLSCASecretRef.Name),
						getTLSVolume(testutils.ClientTLSServerCertSecretName, etcdClientTLS.ServerTLSSecretRef.Name),
						getTLSVolume(testutils.ClientTLSClientCertSecretName, etcdClientTLS.ClientTLSSecretRef.Name),
					},
				},
			},
			BackoffLimit: ptr.To[int32](0),
		},
	}
}

// getHealthCheckScript generates the shell script used to check Etcd health.
func getHealthCheckScript(etcdClientServiceName string, etcdClientServicePort int32) string {
	return fmt.Sprintf(`failed=0; threshold=2;
    while true; do
        if ! curl --cacert %s/ca.crt --cert %s/tls.crt --key %s/tls.key https://%s:%d/health -s -f -o /dev/null; then
            echo "etcd is unhealthy, retrying"
            failed=$((failed + 1))
            if [ "$failed" -ge "$threshold" ]; then
                echo "etcd health check failed too many times"
                rm -f /tmp/healthy
                exit 1
            fi
            sleep 2
        else
            echo "etcd is healthy"
            touch /tmp/healthy
            failed=0
            sleep 2
        fi
    done`, common.VolumeMountPathEtcdCA, common.VolumeMountPathEtcdClientTLS, common.VolumeMountPathEtcdClientTLS, etcdClientServiceName, etcdClientServicePort)
}

// getProbe creates a probe with specified file path for readiness and liveness checks.
func getProbe(filePath string) *corev1.Probe {
	return &corev1.Probe{
		InitialDelaySeconds: 5,
		FailureThreshold:    1,
		PeriodSeconds:       1,
		SuccessThreshold:    1,
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"cat", filePath},
			},
		},
	}
}

// DeployEtcdLoaderJob deploys the etcd loader job which loads keys into etcd.
func (t *TestEnvironment) DeployEtcdLoaderJob(g *WithT, namespace, etcdClientServiceName string, etcdClientServicePort int32, etcdClientTLS *druidv1alpha1.TLSConfig, numKeys int64, timeout time.Duration) {
	etcdLoaderJob, err := getEtcdLoaderJob(g, namespace, etcdClientServiceName, etcdClientServicePort, etcdClientTLS, numKeys)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(t.cl.Create(t.ctx, etcdLoaderJob)).To(Succeed())

	g.Eventually(func() error {
		job := &batchv1.Job{}
		err := t.cl.Get(t.ctx, types.NamespacedName{Name: etcdLoaderJob.Name, Namespace: namespace}, job)
		if err != nil {
			return err
		}
		if job.Status.Active > 0 {
			return fmt.Errorf("job %s is still active", job.Name)
		}
		if job.Status.Failed > 0 {
			return fmt.Errorf("job %s has failed", job.Name)
		}
		if job.Status.Succeeded == 0 {
			return fmt.Errorf("job %s is not yet completed", job.Name)
		}
		return nil
	}, timeout, defaultPollingInterval).Should(Succeed())
}

// getEtcdLoaderJob returns the job object for loading keys into etcd.
func getEtcdLoaderJob(g *WithT, namespace, etcdClientServiceName string, etcdClientServicePort int32, etcdClientTLS *druidv1alpha1.TLSConfig, numKeys int64) (*batchv1.Job, error) {
	if numKeys <= 0 {
		return nil, fmt.Errorf("number of keys must be greater than zero")
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", jobNameEtcdLoader, testutils.GenerateRandomAlphanumericString(g, 4)),
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "loader",
							Image:           "alpine/curl",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/bin/sh", "-c"},
							Args:            []string{getEtcdLoaderScript(numKeys, etcdClientServiceName, etcdClientServicePort)},
							VolumeMounts: []corev1.VolumeMount{
								{MountPath: common.VolumeMountPathEtcdCA, Name: testutils.ClientTLSCASecretName},
								{MountPath: common.VolumeMountPathEtcdClientTLS, Name: testutils.ClientTLSClientCertSecretName},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{
						getTLSVolume(testutils.ClientTLSCASecretName, etcdClientTLS.TLSCASecretRef.Name),
						getTLSVolume(testutils.ClientTLSClientCertSecretName, etcdClientTLS.ClientTLSSecretRef.Name),
					},
				},
			},
			BackoffLimit: ptr.To[int32](0),
		},
	}, nil
}

// getEtcdLoaderScript generates the shell script used by the etcd loader job to populate etcd with keys.
func getEtcdLoaderScript(numKeys int64, etcdClientServiceName string, etcdClientServicePort int32) string {
	return fmt.Sprintf(`
	for i in $(seq 1 %d); do
		key=$(echo -n "key-$i" | base64)
		value=$(echo -n "value-$i" | base64)
		curl -s -X POST \
		-H "Content-Type: application/json" \
		--cacert %s/ca.crt \
		--cert %s/tls.crt \
		--key %s/tls.key \
		-d "{\"key\": \"$key\", \"value\": \"$value\"}" \
		-L https://%s:%d/v3/kv/put;
	done
	`, numKeys, common.VolumeMountPathEtcdCA, common.VolumeMountPathEtcdClientTLS, common.VolumeMountPathEtcdClientTLS, etcdClientServiceName, etcdClientServicePort)
}

// getTLSVolume creates a volume for the TLS secret.
func getTLSVolume(name, secretName string) corev1.Volume {
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  secretName,
				DefaultMode: ptr.To(common.ModeOwnerReadWriteGroupRead),
			},
		},
	}
}

// TakeFullSnapshot takes an on-demand full snapshot of the etcd cluster using etcd-backup-restore server's HTTP API.
func (t *TestEnvironment) TakeFullSnapshot(g *WithT, etcd *druidv1alpha1.Etcd, timeout time.Duration) {
	t.TakeSnapshot(g, etcd, "full", timeout)
}

// TakeDeltaSnapshot takes an on-demand delta snapshot of the etcd cluster using etcd-backup-restore server's HTTP API.
func (t *TestEnvironment) TakeDeltaSnapshot(g *WithT, etcd *druidv1alpha1.Etcd, timeout time.Duration) {
	t.TakeSnapshot(g, etcd, "delta", timeout)
}

// TakeSnapshot takes an on-demand snapshot of the etcd cluster using etcd-backup-restore server's HTTP API.
func (t *TestEnvironment) TakeSnapshot(g *WithT, etcd *druidv1alpha1.Etcd, snapshotType string, timeout time.Duration) {
	job := getSnapshotterJob(g, etcd, snapshotType)
	g.Expect(t.cl.Create(t.ctx, job)).To(Succeed())

	g.Eventually(func() error {
		j := &batchv1.Job{}
		err := t.cl.Get(t.ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, j)
		if err != nil {
			return err
		}
		if j.Status.Active > 0 {
			return fmt.Errorf("job %s is still active", j.Name)
		}
		if j.Status.Failed > 0 {
			return fmt.Errorf("job %s has failed", j.Name)
		}
		if j.Status.Succeeded == 0 {
			return fmt.Errorf("job %s is not yet completed", j.Name)
		}
		return nil
	}, timeout, defaultPollingInterval).Should(Succeed())
}

// getSnapshotterJob returns the job object for taking an on-demand snapshot.
func getSnapshotterJob(g *WithT, etcd *druidv1alpha1.Etcd, snapshotType string) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s", jobNameSnapshotter, snapshotType, testutils.GenerateRandomAlphanumericString(g, 4)),
			Namespace: etcd.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "curl",
							Image:           "alpine/curl",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/bin/sh", "-c"},
							Args: []string{
								fmt.Sprintf(`curl --cacert %s/ca.crt --cert %s/tls.crt --key %s/tls.key https://%s:%d/snapshot/%s`,
									common.VolumeMountPathBackupRestoreCA,
									common.VolumeMountPathBackupRestoreClientTLS,
									common.VolumeMountPathBackupRestoreClientTLS,
									druidv1alpha1.GetClientServiceName(etcd.ObjectMeta),
									*etcd.Spec.Backup.Port,
									snapshotType),
							},
							VolumeMounts: []corev1.VolumeMount{
								{MountPath: common.VolumeMountPathBackupRestoreCA, Name: testutils.BackupRestoreTLSCASecretName},
								{MountPath: common.VolumeMountPathBackupRestoreClientTLS, Name: testutils.BackupRestoreTLSClientCertSecretName},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{
						getTLSVolume(testutils.BackupRestoreTLSCASecretName, etcd.Spec.Backup.TLS.TLSCASecretRef.Name),
						getTLSVolume(testutils.BackupRestoreTLSClientCertSecretName, etcd.Spec.Backup.TLS.ClientTLSSecretRef.Name),
					},
				},
			},
			BackoffLimit: ptr.To[int32](0),
		},
	}
}

// EnsureCompaction checks if compaction has succeeded by verifying the snapshot revisions.
func (t *TestEnvironment) EnsureCompaction(g *WithT, etcdObjectMeta metav1.ObjectMeta, expectedFullSnapshotRevision, expectedDeltaSnapshotRevision int64, timeout time.Duration) {
	g.Eventually(func() error {
		fullSnapshotRevision, deltaSnapshotRevision, err := t.getSnapshotRevisions(etcdObjectMeta)
		if err != nil {
			return err
		}

		if deltaSnapshotRevision != expectedDeltaSnapshotRevision {
			return fmt.Errorf("expected delta snapshot revision to be 0, but got %d", deltaSnapshotRevision)
		}

		if fullSnapshotRevision != expectedFullSnapshotRevision {
			return fmt.Errorf("expected full snapshot revision to be %d, but got %d", expectedFullSnapshotRevision, fullSnapshotRevision)
		}

		return nil
	}, timeout, defaultPollingInterval).Should(Succeed())
}

// EnsureNoCompaction checks if compaction has not been triggered by verifying the snapshot revisions.
func (t *TestEnvironment) EnsureNoCompaction(g *WithT, etcdObjectMeta metav1.ObjectMeta, expectedFullSnapshotRevision, expectedDeltaSnapshotRevision int64, duration time.Duration) {
	t.waitForMinimumDuration(duration)

	fullSnapshotRevision, deltaSnapshotRevision, err := t.getSnapshotRevisions(etcdObjectMeta)
	g.Expect(err).ShouldNot(HaveOccurred())

	g.Expect(fullSnapshotRevision).To(Equal(expectedFullSnapshotRevision))
	g.Expect(deltaSnapshotRevision).To(Equal(expectedDeltaSnapshotRevision))
}

// waitForMinimumDuration waits for the specified minimum duration.
func (t *TestEnvironment) waitForMinimumDuration(minDuration time.Duration) {
	select {
	case <-time.After(minDuration):
		return
	case <-t.ctx.Done():
		return
	}
}

// getSnapshotRevisions fetches the full and delta snapshot revisions from the snapshot leases of the Etcd resource.
func (t *TestEnvironment) getSnapshotRevisions(etcdObjectMeta metav1.ObjectMeta) (int64, int64, error) {
	fullSnapshotLease := &coordinationv1.Lease{}
	deltaSnapshotLease := &coordinationv1.Lease{}

	if err := t.cl.Get(t.ctx, types.NamespacedName{
		Name:      druidv1alpha1.GetFullSnapshotLeaseName(etcdObjectMeta),
		Namespace: etcdObjectMeta.Namespace,
	}, fullSnapshotLease); err != nil {
		return 0, 0, fmt.Errorf("failed to fetch full snapshot lease: %w", err)
	}

	if err := t.cl.Get(t.ctx, types.NamespacedName{
		Name:      druidv1alpha1.GetDeltaSnapshotLeaseName(etcdObjectMeta),
		Namespace: etcdObjectMeta.Namespace,
	}, deltaSnapshotLease); err != nil {
		return 0, 0, fmt.Errorf("failed to fetch delta snapshot lease: %w", err)
	}

	var err error
	fullSnapshotRevision := int64(0)
	if fullSnapshotLease.Spec.HolderIdentity != nil {
		fullSnapshotRevision, err = strconv.ParseInt(*fullSnapshotLease.Spec.HolderIdentity, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to parse full snapshot revision: %w", err)
		}
	}

	deltaSnapshotRevision := int64(0)
	if deltaSnapshotLease.Spec.HolderIdentity != nil {
		deltaSnapshotRevision, err = strconv.ParseInt(*deltaSnapshotLease.Spec.HolderIdentity, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to parse delta snapshot revision: %w", err)
		}
	}

	return fullSnapshotRevision, deltaSnapshotRevision, nil
}
