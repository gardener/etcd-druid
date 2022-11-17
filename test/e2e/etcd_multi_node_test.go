package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gardener/etcd-druid/api/v1alpha1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s_labels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Etcd", func() {
	var (
		etcdName         string
		storageContainer string
		parentCtx        context.Context
		provider         TestProvider
	)

	BeforeEach(func() {
		parentCtx = context.Background()
		// take first provider
		provider = providers[0]

		etcdName = fmt.Sprintf("etcd-%s", provider.Name)

		storageContainer = getEnvAndExpectNoError(envStorageContainer)

		snapstoreProvider := provider.Storage.Provider
		store, err := getSnapstore(string(snapstoreProvider), storageContainer, storePrefix)
		Expect(err).ShouldNot(HaveOccurred())

		// purge any existing backups in bucket
		Expect(purgeSnapstore(store)).To(Succeed())
		Expect(deployBackupSecret(parentCtx, cl, logger, provider, etcdNamespace, storageContainer))

	})

	AfterEach(func() {
		// remove etcd objects if any old etcd objects exists.
		purgeEtcd(parentCtx, cl, providers)
	})

	Context("when multi-node is configured", func() {
		It("should perform etcd operations", func() {
			ctx, cancelFunc := context.WithTimeout(parentCtx, 15*time.Minute)
			defer cancelFunc()

			etcd := getDefaultMultiNodeEtcd(etcdName, namespace, storageContainer, storePrefix, provider)
			objLogger := logger.WithValues("etcd-multi-node", client.ObjectKeyFromObject(etcd))

			By("Create etcd")
			createAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

			By("Hibernate etcd (Scale down from 3->0)")
			hibernateAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

			By("Wakeup etcd (Scale up from 0->3)")
			// scale up etcd replicas to 3 and ensures etcd cluster with 3 replicas is ready.
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
			etcd.Spec.Replicas = multiNodeEtcdReplicas
			updateAndCheckEtcd(ctx, cl, logger, etcd, multiNodeEtcdTimeout)

			By("Zero downtime rolling updates")
			job := startEtcdZeroDownTimeValidatorJob(ctx, cl, etcd, "rolling-update")
			// this defer ensures to remove the job.
			defer cleanUpTestHelperJob(ctx, cl, job.Name)

			Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
			// trigger rolling update by updating etcd quota
			etcd.Spec.Etcd.Quota.Add(*resource.NewMilliQuantity(int64(10), resource.DecimalSI))
			updateAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)
			checkEtcdZeroDownTimeValidatorJob(ctx, cl, client.ObjectKeyFromObject(job), objLogger)

			By("Zero downtime maintenance operation: defragmentation")
			objLogger.Info("Configure defragmentation schedule for every 1 minute")
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
			*etcd.Spec.Etcd.DefragmentationSchedule = "*/1 * * * *"
			updateAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)
			// check any downtime due to rolling update.
			checkEtcdZeroDownTimeValidatorJob(ctx, cl, client.ObjectKeyFromObject(job), objLogger)
			checkDefragmentationFinished(ctx, cl, etcd, objLogger)

			objLogger.Info("Checking any Etcd downtime")
			// Checking Etcd cluster is healthy and there is no downtime while defragmentation.
			// K8s job zeroDownTimeValidator will fail, if there is any downtime in Etcd cluster health.
			checkEtcdZeroDownTimeValidatorJob(ctx, cl, client.ObjectKeyFromObject(job), objLogger)

			By("Member restart with data-dir/pvc intact")
			objLogger.Info("Delete one member pod")
			deletePod(ctx, cl, objLogger, etcd, "etcd-aws-2")
			checkEtcdReady(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

			By("Single member restoration")
			objLogger.Info("Delete member dir of one member pod")
			deleteMemberDir(ctx, cl, objLogger, etcd, "etcd-aws-2")
			checkEtcdReady(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

			objLogger.Info("Corrupt DB file of one member of cluster")
			corruptMemberDBFile(ctx, cl, objLogger, etcd, "etcd-aws-2")
			checkEtcdReady(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

			By("Delete etcd")
			deleteAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)
		})
	})

	Context("when single-node is configured", func() {
		It("should scale single-node etcd to multi-node etcd cluster", func() {
			ctx, cancelFunc := context.WithTimeout(parentCtx, 10*time.Minute)
			defer cancelFunc()

			etcd := getDefaultEtcd(etcdName, namespace, storageContainer, storePrefix, provider)
			objLogger := logger.WithValues("etcd-multi-node", client.ObjectKeyFromObject(etcd))

			By("Create single-node etcd")
			createAndCheckEtcd(ctx, cl, objLogger, etcd, singleNodeEtcdTimeout)

			By("Scale up of a healthy cluster (from 1->3)")
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
			etcd.Spec.Replicas = multiNodeEtcdReplicas
			updateAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

			// TODO: Uncomment me once scale down replicas from 3 to 1 is supported.
			// By("Scale down of a healthy cluster (from 3 to 1)")
			// Expect( cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
			// etcd.Spec.Replicas = replicas
			// updateAndCheckEtcd(ctx, cl, objLogger, etcd)

			By("Delete single-node etcd")
			deleteAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)
		})
	})
})

func deleteMemberDir(ctx context.Context, cl client.Client, logger logr.Logger, etcd *v1alpha1.Etcd, podName string) {
	ExpectWithOffset(1, deleteDir(kubeconfigPath, namespace, podName, "backup-restore", "/var/etcd/data/new.etcd/member")).To(Succeed())
	checkUnreadySts(ctx, cl, logger, etcd)
}

func deletePod(ctx context.Context, cl client.Client, logger logr.Logger, etcd *v1alpha1.Etcd, podName string) {
	pod := &corev1.Pod{}
	ExpectWithOffset(1, cl.Get(ctx, types.NamespacedName{Name: podName, Namespace: namespace}, pod)).To(Succeed())
	ExpectWithOffset(1, cl.Delete(ctx, pod, client.PropagationPolicy(metav1.DeletePropagationForeground))).To(Succeed())
	checkUnreadySts(ctx, cl, logger, etcd)
}

func corruptMemberDBFile(ctx context.Context, cl client.Client, logger logr.Logger, etcd *v1alpha1.Etcd, podName string) {
	ExpectWithOffset(1, corruptDBFile(kubeconfigPath, namespace, podName, "backup-restore", "/var/etcd/data/new.etcd/member/snap/db")).To(Succeed())
	checkUnreadySts(ctx, cl, logger, etcd)
}

func checkUnreadySts(ctx context.Context, cl client.Client, logger logr.Logger, etcd *v1alpha1.Etcd) {
	logger.Info("waiting for sts to become unready")
	EventuallyWithOffset(2, func() error {
		sts := &appsv1.StatefulSet{}
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcd), sts); err != nil {
			return err
		}
		if sts.Status.ReadyReplicas == *sts.Spec.Replicas {
			return fmt.Errorf("sts %s is still in ready state", "etcd-aws")
		}
		return nil
	}, multiNodeEtcdTimeout, pollingInterval).Should(BeNil())
	logger.Info("sts is unready")
}

func checkReadySts(ctx context.Context, cl client.Client, logger logr.Logger, etcd *v1alpha1.Etcd) {
	logger.Info("waiting for sts to become ready again", "statefulSetName", "etcd-aws")
	EventuallyWithOffset(2, func() error {
		sts := &appsv1.StatefulSet{}
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcd), sts); err != nil {
			return err
		}
		if sts.Status.ReadyReplicas != *sts.Spec.Replicas {
			return fmt.Errorf("sts %s unready", "etcd-aws")
		}
		return nil
	}, multiNodeEtcdTimeout, pollingInterval).Should(BeNil())
	logger.Info("sts is ready", "statefulSetName", "etcd-aws")
}

// checkEventuallyEtcdRollingUpdateDone is a helper function, uses Gomega Eventually.
// Returns the function until etcd rolling updates is done for given timeout and polling interval or
// it raises assertion error.
//
// checkEventuallyEtcdRollingUpdateDone ensures rolling updates of etcd resources(etcd/sts) is done.
func checkEventuallyEtcdRollingUpdateDone(ctx context.Context, cl client.Client, etcd *v1alpha1.Etcd,
	oldStsObservedGeneration, oldEtcdObservedGeneration int64, timeout time.Duration) {
	EventuallyWithOffset(1, func() error {
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd); err != nil {
			return fmt.Errorf("error occurred while getting etcd object: %v ", err)
		}

		if *etcd.Status.ObservedGeneration <= oldEtcdObservedGeneration {
			return fmt.Errorf("waiting for etcd %q rolling update to complete", etcd.Name)
		}

		sts := &appsv1.StatefulSet{}
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcd), sts); err != nil {
			return fmt.Errorf("error occurred while getting sts object: %v ", err)
		}

		if sts.Generation <= oldStsObservedGeneration || sts.Generation != sts.Status.ObservedGeneration {
			return fmt.Errorf("waiting for statefulset rolling update to complete %d pods at revision %s",
				sts.Status.UpdatedReplicas, sts.Status.UpdateRevision)
		}

		if sts.Status.UpdatedReplicas != *sts.Spec.Replicas {
			return fmt.Errorf("waiting for statefulset rolling update to complete, UpdatedReplicas is %d, but expected to be %d",
				sts.Status.UpdatedReplicas, *sts.Spec.Replicas)
		}

		return nil
	}, timeout, pollingInterval).Should(BeNil())
}

// hibernateAndCheckEtcd scales down etcd replicas to zero and ensures
func hibernateAndCheckEtcd(ctx context.Context, cl client.Client, logger logr.Logger, etcd *v1alpha1.Etcd, timeout time.Duration) {
	ExpectWithOffset(1, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		ExpectWithOffset(2, cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
		etcd.SetAnnotations(
			map[string]string{
				v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile,
			})
		etcd.Spec.Replicas = 0
		return cl.Update(ctx, etcd)
	})).ToNot(HaveOccurred())
	logger.Info("Waiting to hibernate")

	logger.Info("Checking etcd")
	EventuallyWithOffset(1, func() error {
		etcd := getEmptyEtcd(etcd.Name, namespace)
		err := cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)
		if err != nil {
			return err
		}

		if etcd.Status.Ready != nil && *etcd.Status.Ready != true {
			return fmt.Errorf("etcd %s is not ready", etcd.Name)
		}

		// TODO: uncomment me once scale down is supported,
		// currently ClusterSize is not updated while scaling down.
		//
		// if etcd.Status.ClusterSize == nil {
		// 	return fmt.Errorf("etcd %s cluster size is empty", etcd.Name)
		// }
		//
		// if *etcd.Status.ClusterSize != 0 {
		// 	return fmt.Errorf("etcd %q cluster size is %d, but expected to be 0",
		// 		etcdName, *etcd.Status.ClusterSize)
		// }

		if etcd.Status.ReadyReplicas != 0 {
			return fmt.Errorf("etcd readyReplicas is %d, but expected to be 0", etcd.Status.ReadyReplicas)
		}

		if len(etcd.Status.Conditions) == 0 {
			return fmt.Errorf("etcd %s status conditions is empty", etcd.Name)
		}

		for _, c := range etcd.Status.Conditions {
			if c.Status != v1alpha1.ConditionTrue {
				return fmt.Errorf("etcd %s status condition is %q, but expected to be %s ",
					etcd.Name, c.Status, v1alpha1.ConditionTrue)
			}
		}

		return nil
	}, timeout, pollingInterval).Should(BeNil())

	logger.Info("Checking statefulset")
	sts := &appsv1.StatefulSet{}
	ExpectWithOffset(1, cl.Get(ctx, client.ObjectKeyFromObject(etcd), sts)).To(Succeed())
	ExpectWithOffset(1, sts.Spec.Replicas).ShouldNot(BeNil())
	ExpectWithOffset(1, *sts.Spec.Replicas).To(BeNumerically("==", 0))
	ExpectWithOffset(1, sts.Status.ReadyReplicas).To(BeNumerically("==", 0))
	logger.Info("etcd is hibernated")
}

// updateAndCheckEtcd updates the given etcd obj in the Kubernetes cluster.
func updateAndCheckEtcd(ctx context.Context, cl client.Client, logger logr.Logger, etcd *v1alpha1.Etcd, timeout time.Duration) {
	sts := &appsv1.StatefulSet{}
	ExpectWithOffset(1, cl.Get(ctx, client.ObjectKeyFromObject(etcd), sts)).To(Succeed())
	oldStsObservedGeneration, oldEtcdObservedGeneration := sts.Status.ObservedGeneration, *etcd.Status.ObservedGeneration

	// update reconcile annotation, druid to reconcile and update the changes.
	ExpectWithOffset(1, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		etcdObj := &v1alpha1.Etcd{}
		ExpectWithOffset(1, cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcdObj)).To(Succeed())
		etcdObj.SetAnnotations(
			map[string]string{
				v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile,
			})
		etcdObj.Spec = etcd.Spec
		return cl.Update(ctx, etcdObj)
	})).ToNot(HaveOccurred())

	// Ensuring update is successful by verifying
	// ObservedGeneration ID of sts, etcd before and after update done.
	checkEventuallyEtcdRollingUpdateDone(ctx, cl, etcd,
		oldStsObservedGeneration, oldEtcdObservedGeneration, timeout)
	checkEtcdReady(ctx, cl, logger, etcd, timeout)
}

// cleanUpTestHelperJob ensures to remove the given job in the kubernetes cluster if job exists.
func cleanUpTestHelperJob(ctx context.Context, cl client.Client, jobName string) {
	ExpectWithOffset(1,
		client.IgnoreNotFound(
			cl.Delete(ctx,
				&batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      jobName,
						Namespace: namespace,
					},
				},
				client.PropagationPolicy(metav1.DeletePropagationForeground),
			),
		),
	).To(Succeed())
}

// checkJobReady checks k8s job associated pod is ready, up and running.
// checkJobReady is a helper function, uses Gomega Eventually.
func checkJobReady(ctx context.Context, cl client.Client, jobName string) {
	r, _ := k8s_labels.NewRequirement("job-name", selection.Equals, []string{jobName})
	opts := &client.ListOptions{
		LabelSelector: k8s_labels.NewSelector().Add(*r),
		Namespace:     namespace,
	}

	EventuallyWithOffset(1, func() error {
		podList := &corev1.PodList{}
		if err := cl.List(ctx, podList, opts); err != nil {
			return fmt.Errorf("error occurred while getting pod object: %v ", err)
		}

		if len(podList.Items) == 0 {
			return fmt.Errorf("job %s associated pod is not scheduled", jobName)
		}

		for _, c := range podList.Items[0].Status.Conditions {
			if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
				return nil
			}

		}
		return fmt.Errorf("waiting for pod %v to be ready", podList.Items[0].Name)
	}, time.Minute, pollingInterval).Should(BeNil())
}

// etcdZeroDownTimeValidatorJob returns k8s job which ensures
// Etcd cluster zero down time by continuously checking etcd cluster health.
// This job fails once health check fails and associated pod results in error status.
func startEtcdZeroDownTimeValidatorJob(ctx context.Context, cl client.Client,
	etcd *v1alpha1.Etcd, testName string) *batchv1.Job {
	job := etcdZeroDownTimeValidatorJob(etcd.Name+"-client", testName, etcd.Spec.Etcd.ClientUrlTLS)

	logger.Info(fmt.Sprintf("Creating job %s to ensure etcd zero downtime", job.Name))
	ExpectWithOffset(1, cl.Create(ctx, job)).ShouldNot(HaveOccurred())

	// Wait until zeroDownTimeValidator job is up and running.
	checkJobReady(ctx, cl, job.Name)
	logger.Info(fmt.Sprintf("Job %s is ready", job.Name))
	return job
}

// getEtcdLeaderPodName returns the leader pod name by using lease
func getEtcdLeaderPodName(ctx context.Context, cl client.Client, namespace string) (*types.NamespacedName, error) {
	leaseList := &v1.LeaseList{}
	opts := &client.ListOptions{Namespace: namespace}
	ExpectWithOffset(1, cl.List(ctx, leaseList, opts)).ShouldNot(HaveOccurred())

	for _, lease := range leaseList.Items {
		if lease.Spec.HolderIdentity == nil {
			return nil, fmt.Errorf("error occurred while finding leader, etcd lease %q spec.holderIdentity is nil", lease.Name)
		}
		if strings.HasSuffix(*lease.Spec.HolderIdentity, ":Leader") {
			return &types.NamespacedName{Namespace: namespace, Name: lease.Name}, nil
		}
	}
	return nil, fmt.Errorf("leader doesn't exist for namespace %q", namespace)
}

// getPodLogs returns logs for the given pod
func getPodLogs(ctx context.Context, PodKey *types.NamespacedName, opts *corev1.PodLogOptions) (string, error) {
	typedClient, err := getKubernetesTypedClient(kubeconfigPath)
	if err != nil {
		return "", err
	}

	req := typedClient.CoreV1().Pods(namespace).GetLogs(PodKey.Name, opts)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// checkDefragmentationFinished checks defragmentation is finished or not for given etcd.
func checkDefragmentationFinished(ctx context.Context, cl client.Client, etcd *v1alpha1.Etcd, logger logr.Logger) {
	// Wait until etcd cluster defragmentation is finish.
	logger.Info("Waiting for defragmentation to finish")
	EventuallyWithOffset(1, func() error {
		leaderPodKey, err := getEtcdLeaderPodName(ctx, cl, namespace)
		if err != nil {
			return err
		}
		// Get etcd leader pod logs to ensure etcd cluster defragmentation is finished or not.
		logs, err := getPodLogs(ctx, leaderPodKey, &corev1.PodLogOptions{
			Container:    "backup-restore",
			SinceSeconds: pointer.Int64(60),
		})

		if err != nil {
			return fmt.Errorf("error occurred while getting [%s] pod logs, error: %v ", leaderPodKey, err)
		}

		for i := 0; i < int(multiNodeEtcdReplicas); i++ {
			if !strings.Contains(logs,
				fmt.Sprintf("Finished defragmenting etcd member[https://%s-%d.%s-peer.%s.svc:2379]",
					etcd.Name, i, etcd.Name, namespace)) {
				return fmt.Errorf("etcd %q defragmentation is not finished for member %q-%d", etcd.Name, etcd.Name, i)
			}
		}

		logger.Info("Defragmentation is finished")
		return nil
	}, time.Minute*6, pollingInterval).Should(BeNil())

}

// checkEtcdZeroDownTimeValidatorJob ensures etcd cluster health downtime,
// there is no downtime if given job is active.
func checkEtcdZeroDownTimeValidatorJob(ctx context.Context, cl client.Client, jobName types.NamespacedName,
	logger logr.Logger) {

	job := &batchv1.Job{}
	ExpectWithOffset(1, cl.Get(ctx, jobName, job)).To(Succeed())
	ExpectWithOffset(1, job.Status.Failed).Should(BeZero())
	logger.Info("Etcd Cluster is healthy and there is no downtime")
}

func purgeEtcd(ctx context.Context, cl client.Client, providers []TestProvider) {
	for _, p := range providers {
		e := getEmptyEtcd(fmt.Sprintf("etcd-%s", p.Name), namespace)
		if err := cl.Get(ctx, client.ObjectKeyFromObject(e), e); err == nil {
			ExpectWithOffset(1, kutil.DeleteObject(ctx, cl, e)).To(Succeed())
			EventuallyWithOffset(1, func() error {
				ctx, cancelFunc := context.WithTimeout(ctx, time.Minute)
				defer cancelFunc()
				return cl.Get(ctx, client.ObjectKeyFromObject(e), e)
			}, time.Minute, pollingInterval).Should(matchers.BeNotFoundError())
			purgeEtcdPVCs(ctx, cl, e.Name)
		}
	}
}
