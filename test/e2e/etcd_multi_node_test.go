// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	druidstore "github.com/gardener/etcd-druid/internal/store"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Etcd", func() {
	var (
		storageContainer string
		provider         TestProvider
	)

	BeforeEach(func() {
		// take first provider
		provider = providers[0]
		storageContainer = getEnvAndExpectNoError(envStorageContainer)
	})

	Context("when multi-node is configured", func() {
		It("should perform etcd operations excluding zero downtime maintenance", func() {
			etcdName := "multi-node-etcd"
			storePrefix := etcdName
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			purgeSnapstoreIfNeeded(ctx, cl, provider, storageContainer, etcdName)

			etcd := getDefaultMultiNodeEtcd(etcdName, namespace, storageContainer, storePrefix, provider)
			objLogger := logger.WithValues("etcd", client.ObjectKeyFromObject(etcd))

			By("Creating etcd")
			createAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

			By("Hibernating etcd (Scaling down from 3 replicas to 0)")
			hibernateAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

			By("Waking up etcd (Scaling up from 0 to 3 replicas)")
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
			etcd.Spec.Replicas = multiNodeEtcdReplicas
			updateAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

			By("Member restart with data-dir/pvc intact")
			objLogger.Info("Deleting one member pod")
			deletePod(ctx, cl, objLogger, etcd, fmt.Sprintf("%s-2", etcdName))
			checkForUnreadyEtcdMembers(ctx, cl, objLogger, etcd, 30*time.Second)
			checkEtcdReady(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

			By("Single member restoration")
			objLogger.Info("Creating debug pod")
			debugPod := createDebugPod(ctx, etcd)
			objLogger.Info("Deleting member directory of one member pod")
			deleteMemberDir(ctx, cl, objLogger, etcd, debugPod.Name, debugPod.Spec.Containers[0].Name)
			checkForUnreadyEtcdMembers(ctx, cl, objLogger, etcd, 30*time.Second)
			checkEtcdReady(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

			By("Deleting debug pod")
			Expect(client.IgnoreNotFound(cl.Delete(ctx, debugPod))).ToNot(HaveOccurred())

			By("Deleting etcd")
			deleteAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)
		})

		It("should perform zero downtime maintenance operation: defragmentation", func() {
			etcdName := "defrag-zero-downtime-etcd"
			storePrefix := etcdName
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			purgeSnapstoreIfNeeded(ctx, cl, provider, storageContainer, etcdName)

			etcd := getDefaultMultiNodeEtcd(etcdName, namespace, storageContainer, storePrefix, provider)
			objLogger := logger.WithValues("etcd", client.ObjectKeyFromObject(etcd))

			By("Creating etcd")
			createAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

			By("Deploying etcd zero downtime validator job")
			job := startEtcdZeroDownTimeValidatorJob(ctx, cl, etcd, "rolling-update")
			defer cleanUpTestHelperJob(ctx, cl, job.Name)

			By("Conducting zero downtime rolling updates")
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
			etcd.Spec.Etcd.Quota.Add(*resource.NewMilliQuantity(int64(10), resource.DecimalSI))
			updateAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)
			checkEtcdZeroDownTimeValidatorJob(ctx, cl, client.ObjectKeyFromObject(job), objLogger)

			By("Performing zero downtime maintenance operation: defragmentation")
			objLogger.Info("Configuring defragmentation schedule for every 1 minute")
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
			*etcd.Spec.Etcd.DefragmentationSchedule = "*/1 * * * *"
			updateAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)
			checkEtcdZeroDownTimeValidatorJob(ctx, cl, client.ObjectKeyFromObject(job), objLogger)
			checkDefragmentationFinished(ctx, cl, etcd, objLogger)

			objLogger.Info("Checking for any Etcd downtime")
			checkEtcdZeroDownTimeValidatorJob(ctx, cl, client.ObjectKeyFromObject(job), objLogger)

			By("Deleting etcd")
			deleteAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)
		})
	})

	Describe("Single-node etcd configuration", func() {

		Context("Scaling up from single-node to multi-node without TLS", func() {

			It("should scale a single-node etcd (TLS not enabled for peerUrl) to a multi-node etcd cluster (TLS not enabled for peerUrl)", func() {
				ctx, cancelFunc := context.WithTimeout(context.Background(), 10*time.Minute)
				defer cancelFunc()
				etcdName := "scale-up-non-tls"
				storePrefix = etcdName

				purgeSnapstoreIfNeeded(ctx, cl, provider, storageContainer, etcdName)
				etcd := getDefaultEtcd(etcdName, namespace, storageContainer, storePrefix, provider)
				objLogger := logger.WithValues("etcd-multi-node", client.ObjectKeyFromObject(etcd))

				By("Creating a single-node etcd")
				createAndCheckEtcd(ctx, cl, objLogger, etcd, singleNodeEtcdTimeout)

				By("Scaling up a healthy cluster (from 1 to 3 replicas)")
				Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
				etcd.Spec.Replicas = 3
				etcd.Spec.Labels["multi-node"] = "true"
				updateAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

				By("Deleting etcd")
				deleteAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)
			})
		})

		Context("Scaling up from single-node to multi-node with TLS", func() {

			It("executes scaling up with TLS", func() {
				ctx, cancelFunc := context.WithTimeout(context.Background(), 10*time.Minute)
				defer cancelFunc()
				etcdName := "scale-up-tls"
				storePrefix = etcdName

				purgeSnapstoreIfNeeded(ctx, cl, provider, storageContainer, etcdName)
				etcd := getDefaultEtcd(etcdName, namespace, storageContainer, storePrefix, provider)
				objLogger := logger.WithValues("etcd-multi-node", client.ObjectKeyFromObject(etcd))

				By("Creating a single-node etcd")
				createAndCheckEtcd(ctx, cl, objLogger, etcd, singleNodeEtcdTimeout)

				By("Scaling up a healthy cluster (from 1 to 3 replicas) with TLS enabled for peerUrl")
				Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
				etcd.Spec.Replicas = 3
				etcd.Spec.Etcd.PeerUrlTLS = getPeerTls(provider.Suffix)
				updateAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)
				By("Deleting etcd")
				deleteAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)
			})
		})

		Context("Scaling down from single-node to zero and back up without TLS", func() {

			It("executes scaling down to 0 and back up to 3 replicas without TLS", func() {
				ctx, cancelFunc := context.WithTimeout(context.Background(), 10*time.Minute)
				defer cancelFunc()
				etcdName := "scale-down-and-up-non-tls"
				storePrefix = etcdName

				purgeSnapstoreIfNeeded(ctx, cl, provider, storageContainer, etcdName)
				etcd := getDefaultEtcd(etcdName, namespace, storageContainer, storePrefix, provider)
				objLogger := logger.WithValues("etcd-multi-node", client.ObjectKeyFromObject(etcd))

				By("Creating a single-node etcd")
				createAndCheckEtcd(ctx, cl, objLogger, etcd, singleNodeEtcdTimeout)

				By("Scaling down a healthy cluster (from 1 to 0 replica)")
				Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
				etcd.Spec.Replicas = 0
				updateAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

				By("Scaling up cluster (from 0 to 1 replica)")
				Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
				etcd.Spec.Replicas = 1
				updateAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

				By("Scaling up a healthy cluster (from 1 to 3 replica)")
				Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
				etcd.Spec.Replicas = 3
				updateAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

				By("Deleting etcd")
				deleteAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)
			})
		})

		Context("Scaling down from single-node to zero and back up with TLS", func() {

			It("should scale down a single-node etcd to 0 replica, then scale up from 0->1 replica and then from 1->3 replicas with TLS enabled for cluster peerUrl", func() {
				ctx, cancelFunc := context.WithTimeout(context.Background(), 10*time.Minute)
				defer cancelFunc()
				etcdName := "scale-down-and-up-tls"
				storePrefix = etcdName

				purgeSnapstoreIfNeeded(ctx, cl, provider, storageContainer, etcdName)
				etcd := getDefaultEtcd(etcdName, namespace, storageContainer, storePrefix, provider)
				objLogger := logger.WithValues("etcd-multi-node", client.ObjectKeyFromObject(etcd))

				By("Creating a single-node etcd")
				createAndCheckEtcd(ctx, cl, objLogger, etcd, singleNodeEtcdTimeout)

				By("Scaling down a healthy cluster (from 1 to 0 replica)")
				Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
				etcd.Spec.Replicas = 0
				updateAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

				By("Scaling up cluster (from 0 to 1 replica)")
				Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
				etcd.Spec.Replicas = 1
				updateAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

				By("Scaling up a healthy cluster (from 1 to 3 replica) with TLS enabled for peerUrl")
				Expect(cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
				etcd.Spec.Replicas = 3
				etcd.Spec.Etcd.PeerUrlTLS = getPeerTls(provider.Suffix)
				updateAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)

				By("Deleting etcd")
				deleteAndCheckEtcd(ctx, cl, objLogger, etcd, multiNodeEtcdTimeout)
			})
		})
	})
})

// purgeSnapstoreIfNeeded purges the snapstore based on the provider settings.
func purgeSnapstoreIfNeeded(ctx context.Context, cl client.Client, provider TestProvider, storageContainer, storePrefix string) {
	By("Purge snapstore")
	snapstoreProvider := provider.Storage.Provider
	if snapstoreProvider == druidstore.Local {
		purgeLocalSnapstoreJob := purgeLocalSnapstore(ctx, cl, storageContainer, storePrefix)
		defer cleanUpTestHelperJob(ctx, cl, purgeLocalSnapstoreJob.Name)
	} else {
		store, err := getSnapstore(string(snapstoreProvider), storageContainer, storePrefix, isEmulatorEnabled(provider))
		ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
		ExpectWithOffset(1, purgeSnapstore(store)).To(Succeed())
	}
}

func deleteMemberDir(ctx context.Context, cl client.Client, logger logr.Logger, etcd *druidv1alpha1.Etcd, podName, containerName string) {
	ExpectWithOffset(1, deleteDir(ctx, kubeconfigPath, namespace, podName, containerName, "/var/etcd/data/new.etcd/member")).To(Succeed())
	checkUnreadySts(ctx, cl, logger, etcd)
}

func deletePod(ctx context.Context, cl client.Client, logger logr.Logger, etcd *druidv1alpha1.Etcd, podName string) {
	pod := &corev1.Pod{}
	ExpectWithOffset(1, cl.Get(ctx, types.NamespacedName{Name: podName, Namespace: namespace}, pod)).To(Succeed())
	ExpectWithOffset(1, cl.Delete(ctx, pod, client.PropagationPolicy(metav1.DeletePropagationForeground))).To(Succeed())
	checkUnreadySts(ctx, cl, logger, etcd)
}

func checkUnreadySts(ctx context.Context, cl client.Client, logger logr.Logger, etcd *druidv1alpha1.Etcd) {
	logger.Info("waiting for sts to become unready")
	EventuallyWithOffset(2, func() error {
		sts := &appsv1.StatefulSet{}
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcd), sts); err != nil {
			return err
		}
		if sts.Status.ReadyReplicas == *sts.Spec.Replicas {
			return fmt.Errorf("sts %s is still in ready state", sts.Name)
		}
		return nil
	}, multiNodeEtcdTimeout, pollingInterval).Should(BeNil())
	logger.Info("sts is unready")
}

func checkForUnreadyEtcdMembers(ctx context.Context, cl client.Client, logger logr.Logger, etcd *druidv1alpha1.Etcd, timeout time.Duration) {
	logger.Info("Waiting for at least one etcd member to become unready")
	EventuallyWithOffset(1, func() error {
		ctx, cancelFunc := context.WithTimeout(ctx, timeout)
		defer cancelFunc()

		if err := cl.Get(ctx, types.NamespacedName{Name: etcd.Name, Namespace: namespace}, etcd); err != nil {
			return err
		}

		if etcd.Status.ReadyReplicas == etcd.Spec.Replicas {
			return fmt.Errorf("etcd ready replicas is still same as spec replicas")
		}

		return nil
	}, timeout, pollingInterval).Should(BeNil())
	logger.Info("at least one etcd member is unready")
}

// checkEventuallyEtcdRollingUpdateDone is a helper function, uses Gomega Eventually.
// Returns the function until etcd rolling updates is done for given timeout and polling interval or
// it raises assertion error.
//
// checkEventuallyEtcdRollingUpdateDone ensures rolling updates of etcd resources(etcd/sts) is done.
func checkEventuallyEtcdRollingUpdateDone(ctx context.Context, cl client.Client, etcd *druidv1alpha1.Etcd,
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
func hibernateAndCheckEtcd(ctx context.Context, cl client.Client, logger logr.Logger, etcd *druidv1alpha1.Etcd, timeout time.Duration) {
	ExpectWithOffset(1, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		ExpectWithOffset(2, cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)).To(Succeed())
		etcd.SetAnnotations(
			map[string]string{
				druidv1alpha1.DruidOperationAnnotation: druidv1alpha1.DruidOperationReconcile,
			})
		etcd.Spec.Replicas = 0
		return cl.Update(ctx, etcd)
	})).ToNot(HaveOccurred())

	logger.Info("Waiting for statefulset spec to reflect change in replicas to 0")
	EventuallyWithOffset(1, func() error {
		sts := &appsv1.StatefulSet{}
		ExpectWithOffset(2, cl.Get(ctx, client.ObjectKeyFromObject(etcd), sts)).To(Succeed())
		if sts.Spec.Replicas == nil {
			return fmt.Errorf("etcd %q replicas is empty", etcd.Name)
		}
		if *sts.Spec.Replicas != 0 {
			return fmt.Errorf("etcd %q replicas is %d, but expected to be 0", etcd.Name, *sts.Spec.Replicas)
		}
		return nil
	}, timeout, pollingInterval).Should(BeNil())

	logger.Info("Checking etcd")
	EventuallyWithOffset(1, func() error {
		etcd := getEmptyEtcd(etcd.Name, namespace)
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd); err != nil {
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
			if c.Status != druidv1alpha1.ConditionTrue {
				return fmt.Errorf("etcd %s status condition is %q, but expected to be %s ",
					etcd.Name, c.Status, druidv1alpha1.ConditionTrue)
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
func updateAndCheckEtcd(ctx context.Context, cl client.Client, logger logr.Logger, etcd *druidv1alpha1.Etcd, timeout time.Duration) {
	sts := &appsv1.StatefulSet{}
	ExpectWithOffset(1, cl.Get(ctx, client.ObjectKeyFromObject(etcd), sts)).To(Succeed())
	Expect(etcd).ToNot(BeNil())
	Expect(etcd.Status.ObservedGeneration).ToNot(BeNil())
	oldStsObservedGeneration, oldEtcdObservedGeneration := sts.Status.ObservedGeneration, *etcd.Status.ObservedGeneration

	// update reconcile annotation, druid to reconcile and update the changes.
	ExpectWithOffset(1, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		etcdObj := &druidv1alpha1.Etcd{}
		ExpectWithOffset(1, cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcdObj)).To(Succeed())
		etcdObj.SetAnnotations(
			map[string]string{
				druidv1alpha1.DruidOperationAnnotation: druidv1alpha1.DruidOperationReconcile,
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
	r, _ := k8slabels.NewRequirement("job-name", selection.Equals, []string{jobName})
	opts := &client.ListOptions{
		LabelSelector: k8slabels.NewSelector().Add(*r),
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
// Etcd cluster zero downtime by continuously checking etcd cluster health.
// This job fails once health check fails and associated pod results in error status.
func startEtcdZeroDownTimeValidatorJob(ctx context.Context, cl client.Client, etcd *druidv1alpha1.Etcd, testName string) *batchv1.Job {
	job := etcdZeroDownTimeValidatorJob(etcd.Name+"-client", testName, etcd.Spec.Etcd.ClientUrlTLS)

	logger.Info("Creating job to ensure etcd zero downtime", "job", job.Name)
	ExpectWithOffset(1, cl.Create(ctx, job)).ShouldNot(HaveOccurred())

	// Wait until zeroDownTimeValidator job is up and running.
	checkJobReady(ctx, cl, job.Name)
	logger.Info("Job is ready", "job", job.Name)
	return job
}

// getEtcdLeaderPodName returns the leader pod name by using lease
func getEtcdLeaderPodName(ctx context.Context, cl client.Client, etcd *druidv1alpha1.Etcd) (*types.NamespacedName, error) {
	leaseList := &v1.LeaseList{}
	r1, err := k8slabels.NewRequirement(druidv1alpha1.LabelPartOfKey, selection.Equals, []string{etcd.Name})
	ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
	r2, err := k8slabels.NewRequirement(druidv1alpha1.LabelComponentKey, selection.Equals, []string{common.ComponentNameMemberLease})
	ExpectWithOffset(1, err).ShouldNot(HaveOccurred())

	opts := &client.ListOptions{
		Namespace:     etcd.Namespace,
		LabelSelector: k8slabels.NewSelector().Add(*r1, *r2),
	}
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
	if _, err = io.Copy(buf, podLogs); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// checkDefragmentationFinished checks defragmentation is finished or not for given etcd.
func checkDefragmentationFinished(ctx context.Context, cl client.Client, etcd *druidv1alpha1.Etcd, logger logr.Logger) {
	// Wait until etcd cluster defragmentation is finish.
	logger.Info("Waiting for defragmentation to finish")
	EventuallyWithOffset(1, func() error {
		leaderPodKey, err := getEtcdLeaderPodName(ctx, cl, etcd)
		if err != nil {
			return err
		}
		// Get etcd leader pod logs to ensure etcd cluster defragmentation is finished or not.
		logs, err := getPodLogs(ctx, leaderPodKey, &corev1.PodLogOptions{
			Container:    "backup-restore",
			SinceSeconds: ptr.To[int64](60),
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

// checkJobSucceeded checks for the k8s job to succeed.
func checkJobSucceeded(ctx context.Context, cl client.Client, jobName string) {
	EventuallyWithOffset(1, func() error {
		job := &batchv1.Job{}
		if err := cl.Get(ctx, types.NamespacedName{Namespace: namespace, Name: jobName}, job); err != nil {
			return err
		}
		if job.Status.Succeeded == 0 {
			return fmt.Errorf("waiting for job %v to succeed", job.Name)
		}
		return nil
	}, time.Minute, pollingInterval).Should(BeNil())
}

// purgeLocalSnapstore deploys a job to purge the contents of the given Local provider snapstore
func purgeLocalSnapstore(ctx context.Context, cl client.Client, storeContainer, storePrefix string) *batchv1.Job {
	job := getPurgeLocalSnapstoreJob(storeContainer, storePrefix)

	logger.Info("Creating job to purge local snapstore", "job", job.Name)
	ExpectWithOffset(1, cl.Create(ctx, job)).ShouldNot(HaveOccurred())

	// Wait until purgeLocalSnapstore job has succeeded.
	checkJobSucceeded(ctx, cl, job.Name)
	logger.Info("Job has succeeded", "job", job.Name)
	return job
}
