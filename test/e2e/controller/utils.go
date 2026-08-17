// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/e2e/testenv"
	e2eutils "github.com/gardener/etcd-druid/test/e2e/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	. "github.com/onsi/gomega"
)

const (
	testNamespacePrefix = "etcd-e2e"

	// test parameters
	timeoutTest                = 1 * time.Hour
	timeoutEtcdCreation        = 5 * time.Minute
	timeoutEtcdDeletion        = 2 * time.Minute
	timeoutEtcdHibernation     = 2 * time.Minute
	timeoutEtcdUnhibernation   = 5 * time.Minute
	timeoutEtcdUpdation        = 10 * time.Minute
	timeoutEtcdDisruptionStart = 60 * time.Second
	timeoutEtcdRecovery        = 5 * time.Minute
	timeoutDeployJob           = 2 * time.Minute
	timeoutReconciliation      = 3 * time.Minute
	timeoutMemberLeases        = 5 * time.Minute
	timeoutExtMembersReady     = 10 * time.Minute

	pollingInterval = 2 * time.Second

	// gracePeriodAfterProcessKill is longer than the etcd container's readiness probe
	// initialDelaySeconds (15s), ensuring the probe must pass in the restarted container
	// before waitForStaticPodReady is called.
	gracePeriodAfterProcessKill = 30 * time.Second

	endpointsDirName  = "endpoints"
	endpointsFileName = "endpoints"
)

var (
	testEnv             *testenv.TestEnvironment
	retainTestArtifacts e2eutils.RetainTestArtifactsMode
	providers           = []druidv1alpha1.StorageProvider{"none"}
)

// getSecret retrieves a secret by name from the specified namespace.
func getSecret(testEnv *testenv.TestEnvironment, namespace, secretName string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := testEnv.Client().Get(testEnv.Context(), types.NamespacedName{Namespace: namespace, Name: secretName}, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s in namespace %s: %w", secretName, namespace, err)
	}
	return secret, nil
}

// checkSecretFinalizer checks if the specified secret has or does not have the etcd finalizer based on expectFinalizer.
func checkSecretFinalizer(testEnv *testenv.TestEnvironment, namespace, secretName string, expectFinalizer bool) error {
	secret, err := getSecret(testEnv, namespace, secretName)
	if err != nil {
		return err
	}

	if expectFinalizer == controllerutil.ContainsFinalizer(secret, druidapicommon.EtcdFinalizerName) {
		return nil
	}
	return fmt.Errorf("expected finalizer %v on secret %s in namespace %s, but was not satisfied", druidapicommon.EtcdFinalizerName, secretName, namespace)
}

// updateEtcdTLSAndLabels updates the TLS configurations and labels of the given Etcd resource.
func updateEtcdTLSAndLabels(etcd *druidv1alpha1.Etcd, clientTLSEnabled, peerTLSEnabled, backupRestoreTLSEnabled bool, additionalLabels map[string]string) {
	etcd.Spec.Etcd.ClientUrlTLS = nil
	if clientTLSEnabled {
		etcd.Spec.Etcd.ClientUrlTLS = testutils.GetClientTLSConfig()
	}

	etcd.Spec.Etcd.PeerUrlTLS = nil
	if peerTLSEnabled {
		etcd.Spec.Etcd.PeerUrlTLS = testutils.GetPeerTLSConfig()
	}

	etcd.Spec.Backup.TLS = nil
	if backupRestoreTLSEnabled {
		etcd.Spec.Backup.TLS = testutils.GetBackupRestoreTLSConfig()
	}

	etcd.Spec.Labels = testutils.MergeMaps(etcd.Spec.Labels, additionalLabels)
}

// --- Externally managed members helpers ---

// portSet holds the port numbers for an etcd cluster, allowing parallel tests
// on the same hostNetwork worker nodes without port conflicts.
type portSet struct {
	ClientPort  int32
	PeerPort    int32
	BackupPort  int32
	WrapperPort int32
}

// allocatePorts returns a unique portSet for a given test case name.
// Ports are deterministically derived from the name so that re-runs are reproducible.
func allocatePorts(testCaseName string) portSet {
	h := fnv.New32a()
	_, _ = h.Write([]byte(testCaseName))
	base := int32(10000 + (h.Sum32()%4000)*10) // #nosec G115 -- result is bounded to [10000, 49990], fits int32
	return portSet{
		ClientPort:  base,
		PeerPort:    base + 1,
		BackupPort:  base + 2,
		WrapperPort: base + 3,
	}
}

// workerName returns the KIND node name for the given ordinal.
// KIND naming: ordinal 0 → {cluster}-worker, ordinal N → {cluster}-worker{N+1}
func workerName(ordinal int) string {
	clusterName := os.Getenv(e2eutils.EnvKindClusterName)
	if clusterName == "" {
		clusterName = e2eutils.DefaultKindClusterName
	}
	if ordinal == 0 {
		return clusterName + "-worker"
	}
	return fmt.Sprintf("%s-worker%d", clusterName, ordinal+1)
}

// workerBaseDir returns the base directory on the worker node for the given namespace and etcd name.
func workerBaseDir(namespace, etcdName string) string {
	return fmt.Sprintf("/var/lib/%s/%s", namespace, etcdName)
}

// manifestName returns a unique static pod manifest filename for the given namespace and etcd name.
func manifestName(namespace, etcdName string) string {
	return fmt.Sprintf("%s-%s", namespace, etcdName)
}

// setupWorker prepares a KIND worker node by creating required directories and returns its IP.
func setupWorker(g *WithT, ctx context.Context, cl client.Client, etcdName, namespace string, ordinal int) string {
	worker := workerName(ordinal)

	node := &corev1.Node{}
	g.Expect(cl.Get(ctx, types.NamespacedName{Name: worker}, node)).To(Succeed())

	var workerIP string
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			workerIP = addr.Address
			break
		}
	}
	g.Expect(workerIP).ToNot(BeEmpty(), "could not determine InternalIP for worker %s", worker)

	base := workerBaseDir(namespace, etcdName)
	script := fmt.Sprintf(`set -e
mkdir -p %[1]s/etcd-config-file %[1]s/data %[1]s/serviceaccount \
  %[1]s/etcd-ca %[1]s/etcd-server-tls %[1]s/etcd-client-tls \
  %[1]s/etcd-peer-ca %[1]s/etcd-peer-server-tls \
  %[1]s/backup-restore-ca %[1]s/backup-restore-server-tls %[1]s/backup-restore-client-tls \
  %[1]s/endpoints`, base)
	g.Expect(dockerExec(worker, script)).To(Succeed())
	chownNonroot(g, worker, base)

	return workerIP
}

// cleanupWorkers removes static pod manifests and data directories from all workers.
func cleanupWorkers(etcdName, namespace string, numWorkers int) {
	manifest := manifestName(namespace, etcdName)
	base := workerBaseDir(namespace, etcdName)
	for i := range numWorkers {
		worker := workerName(i)
		_ = dockerExec(worker, fmt.Sprintf("rm -rf /etc/kubernetes/manifests/%s.yaml %s", manifest, base))
	}
}

// prepareServiceAccount creates a SA token secret, waits for the token controller
// to populate it, and writes the token and CA cert to local files.
func prepareServiceAccount(g *WithT, ctx context.Context, cl client.Client, etcdName, namespace string) (tokenFile, caCertFile string) {
	secretName := etcdName + "-sa-token"

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Annotations: map[string]string{
				"kubernetes.io/service-account.name": etcdName,
			},
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}
	g.Expect(cl.Create(ctx, secret)).To(Succeed())

	var token, caCert []byte
	g.Eventually(func() error {
		if err := cl.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
			return err
		}
		token = secret.Data["token"]
		caCert = secret.Data["ca.crt"]
		if len(token) == 0 || len(caCert) == 0 {
			return fmt.Errorf("token or ca.crt not yet populated")
		}
		return nil
	}, 60*time.Second, 2*time.Second).Should(Succeed())

	saDir := filepath.Join(e2eutils.ExtMembersResourcesDir, namespace, "serviceaccount")
	g.Expect(os.MkdirAll(saDir, 0755)).To(Succeed()) // #nosec G301 -- test directory

	tokenFile = filepath.Join(saDir, "token")
	caCertFile = filepath.Join(saDir, "ca.crt")
	g.Expect(os.WriteFile(tokenFile, token, 0600)).To(Succeed())   // #nosec G306 -- test file
	g.Expect(os.WriteFile(caCertFile, caCert, 0600)).To(Succeed()) // #nosec G306 -- test file

	return tokenFile, caCertFile
}

// writeConfigToWorker reads the ConfigMap for the etcd and writes all config
// files to the worker node in a single docker exec call.
func writeConfigToWorker(g *WithT, ctx context.Context, cl client.Client, etcdName, namespace string, ordinal int) {
	worker := workerName(ordinal)
	base := workerBaseDir(namespace, etcdName)

	cm := &corev1.ConfigMap{}
	g.Expect(cl.Get(ctx, types.NamespacedName{Name: etcdName + "-config", Namespace: namespace}, cm)).To(Succeed())

	script := "set -e\n"
	for key, data := range cm.Data {
		script += fmt.Sprintf("cat > '%s/etcd-config-file/%s' << 'CONFIG_EOF'\n%s\nCONFIG_EOF\n", base, key, data)
	}
	g.Expect(dockerExec(worker, script)).To(Succeed())
}

// deployStaticPod deploys an etcd static pod to a worker node.
func deployStaticPod(g *WithT, ctx context.Context, cl client.Client, etcdName, namespace string, ordinal int, saTokenFile, caCertFile string, priorMemberIPs []string) {
	worker := workerName(ordinal)
	base := workerBaseDir(namespace, etcdName)

	writeConfigToWorker(g, ctx, cl, etcdName, namespace, ordinal)

	saDir := base + "/serviceaccount"
	g.Expect(dockerCp(saTokenFile, worker, saDir+"/token")).To(Succeed())
	g.Expect(dockerCp(caCertFile, worker, saDir+"/ca.crt")).To(Succeed())
	chownNonroot(g, worker, saDir)

	writeEndpointsFileToWorker(g, namespace, etcdName, ordinal, priorMemberIPs)

	podYAML, err := translateStatefulSetToPod(ctx, cl, etcdName, namespace)
	g.Expect(err).ToNot(HaveOccurred())

	writeManifest(g, worker, namespace, etcdName, podYAML)
}

// updateWorkerConfig updates config files on a worker without restarting the pod.
func updateWorkerConfig(g *WithT, ctx context.Context, cl client.Client, etcdName, namespace string, ordinal int) {
	writeConfigToWorker(g, ctx, cl, etcdName, namespace, ordinal)
}

// writeEndpointsFileToWorker writes the given IPs (one per line) to the endpoints file on the
// specified worker node. An empty slice produces an empty file, which etcd-backup-restore
// treats as the bootstrap state for the first member (no prior peers).
func writeEndpointsFileToWorker(g *WithT, namespace, etcdName string, ordinal int, ips []string) {
	worker := workerName(ordinal)
	base := workerBaseDir(namespace, etcdName)

	content := strings.Join(ips, "\n")
	if len(ips) > 0 {
		content += "\n"
	}

	localDir := filepath.Join(e2eutils.ExtMembersResourcesDir, namespace, endpointsDirName)
	g.Expect(os.MkdirAll(localDir, 0755)).To(Succeed()) // #nosec G301 -- local directory creation for test purposes.
	localFile := filepath.Join(localDir, fmt.Sprintf("%s-%d", endpointsFileName, ordinal))
	g.Expect(os.WriteFile(localFile, []byte(content), 0600)).To(Succeed()) // #nosec G306 -- test file

	remotePath := fmt.Sprintf("%s/%s/%s", base, endpointsDirName, endpointsFileName)
	g.Expect(dockerCp(localFile, worker, remotePath)).To(Succeed())
	chownNonroot(g, worker, fmt.Sprintf("%s/%s", base, endpointsDirName))
}

// translateStatefulSetToPod fetches the StatefulSet created by druid and converts
// its pod template into a static Pod manifest suitable for KIND worker nodes.
func translateStatefulSetToPod(ctx context.Context, cl client.Client, etcdName, namespace string) ([]byte, error) {
	sts := &appsv1.StatefulSet{}
	if err := cl.Get(ctx, types.NamespacedName{Name: etcdName, Namespace: namespace}, sts); err != nil {
		return nil, fmt.Errorf("failed to get StatefulSet %s/%s: %w", namespace, etcdName, err)
	}

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcdName,
			Namespace: namespace,
			Labels:    sts.Spec.Template.Labels,
		},
		Spec: *sts.Spec.Template.Spec.DeepCopy(),
	}

	pod.Spec.HostNetwork = true
	pod.Spec.EnableServiceLinks = ptr.To(false)
	pod.Spec.ServiceAccountName = ""
	pod.Spec.DeprecatedServiceAccount = ""
	pod.Spec.AutomountServiceAccountToken = ptr.To(false)

	base := workerBaseDir(namespace, etcdName)

	dirType := corev1.HostPathDirectoryOrCreate
	var volumes []corev1.Volume
	for _, v := range pod.Spec.Volumes {
		if v.ConfigMap != nil || v.Name == etcdName {
			continue
		}
		if v.Secret != nil {
			volumes = append(volumes, corev1.Volume{
				Name: v.Name,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{Path: fmt.Sprintf("%s/%s", base, v.Name), Type: &dirType},
				},
			})
			continue
		}
		volumes = append(volumes, v)
	}
	volumes = append(volumes,
		corev1.Volume{
			Name: "etcd-config-file",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{Path: base + "/etcd-config-file", Type: &dirType},
			},
		},
		corev1.Volume{
			Name: etcdName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{Path: base + "/data", Type: &dirType},
			},
		},
		corev1.Volume{
			Name: "serviceaccount",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{Path: base + "/serviceaccount", Type: &dirType},
			},
		},
	)
	pod.Spec.Volumes = volumes

	if len(pod.Spec.Containers) > 1 {
		pod.Spec.Containers[1].VolumeMounts = append(pod.Spec.Containers[1].VolumeMounts, corev1.VolumeMount{
			Name:      "serviceaccount",
			MountPath: "/var/run/secrets/kubernetes.io/serviceaccount",
			ReadOnly:  true,
		})
	}

	return yaml.Marshal(pod)
}

// writeManifest writes a static pod YAML to /etc/kubernetes/manifests/ on the worker.
func writeManifest(g *WithT, worker, namespace, etcdName string, podYAML []byte) {
	manifest := manifestName(namespace, etcdName)
	localPath := filepath.Join(e2eutils.ExtMembersResourcesDir, namespace, manifest+".yaml")
	g.Expect(os.WriteFile(localPath, podYAML, 0600)).To(Succeed()) // #nosec G306 -- test file
	g.Expect(dockerCp(localPath, worker, fmt.Sprintf("/etc/kubernetes/manifests/%s.yaml", manifest))).To(Succeed())
}

// readEndpointsFileFromWorker reads the endpoints file from a worker node and returns the IPs.
func readEndpointsFileFromWorker(namespace, etcdName string, ordinal int) ([]string, error) {
	worker := workerName(ordinal)
	base := workerBaseDir(namespace, etcdName)
	remotePath := fmt.Sprintf("%s/%s/%s", base, endpointsDirName, endpointsFileName)

	cmd := exec.Command("docker", "exec", worker, "cat", remotePath) // #nosec G204 -- e2e test utility
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read endpoints file on worker %s: %w", worker, err)
	}

	var ips []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			ips = append(ips, line)
		}
	}
	return ips, nil
}

// waitForEndpointsOnAllWorkers polls the endpoints file on each worker until all workers
// have exactly the expected IPs (in any order), or the timeout is reached.
func waitForEndpointsOnAllWorkers(g *WithT, namespace, etcdName string, numWorkers int, expectedIPs []string) {
	g.Eventually(func() error {
		for i := range numWorkers {
			ips, err := readEndpointsFileFromWorker(namespace, etcdName, i)
			if err != nil {
				return err
			}
			if len(ips) != len(expectedIPs) {
				return fmt.Errorf("worker %d: got %d IPs (%v), want %d (%v)", i, len(ips), ips, len(expectedIPs), expectedIPs)
			}
			ipSet := make(map[string]struct{}, len(ips))
			for _, ip := range ips {
				ipSet[ip] = struct{}{}
			}
			for _, expected := range expectedIPs {
				if _, ok := ipSet[expected]; !ok {
					return fmt.Errorf("worker %d: missing IP %s in endpoints file (got %v)", i, expected, ips)
				}
			}
		}
		return nil
	}, timeoutExtMembersReady, pollingInterval).Should(Succeed())
}

// removeStaticPodManifest removes a static pod manifest from a worker node,
// which causes kubelet to stop the pod shortly after.
func removeStaticPodManifest(g *WithT, namespace, etcdName string, ordinal int) {
	worker := workerName(ordinal)
	manifest := manifestName(namespace, etcdName)
	g.Expect(dockerExec(worker, fmt.Sprintf("rm -f /etc/kubernetes/manifests/%s.yaml", manifest))).To(Succeed())
}

// waitForStaticPodStopped polls until the mirror pod for the static pod on the given
// worker is gone from the API server. Kubelet creates a mirror pod named
// <etcdName>-<nodeName> in the same namespace; its absence confirms the pod has stopped,
// making it safe to wipe the data directory.
func waitForStaticPodStopped(g *WithT, ctx context.Context, cl client.Client, namespace, etcdName string, ordinal int) {
	mirrorPodName := fmt.Sprintf("%s-%s", etcdName, workerName(ordinal))
	g.Eventually(func() error {
		pod := &corev1.Pod{}
		err := cl.Get(ctx, types.NamespacedName{Name: mirrorPodName, Namespace: namespace}, pod)
		if err == nil {
			return fmt.Errorf("mirror pod %s/%s still exists", namespace, mirrorPodName)
		}
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("unexpected error checking mirror pod: %w", err)
		}
		return nil
	}, 60*time.Second, 2*time.Second).Should(Succeed(), "mirror pod should be gone before corrupting data")
}

// waitForStaticPodReady polls until the mirror pod for the static pod on the given worker
// is Running with all containers ready. Call this after redeploying a static pod to confirm
// the member is actually up before checking higher-level conditions like the Etcd ready status,
// which may be stale from a previous reconciliation.
func waitForStaticPodReady(g *WithT, ctx context.Context, cl client.Client, namespace, etcdName string, ordinal int) {
	mirrorPodName := fmt.Sprintf("%s-%s", etcdName, workerName(ordinal))
	g.Eventually(func() error {
		pod := &corev1.Pod{}
		if err := cl.Get(ctx, types.NamespacedName{Name: mirrorPodName, Namespace: namespace}, pod); err != nil {
			return fmt.Errorf("mirror pod %s/%s not found: %w", namespace, mirrorPodName, err)
		}
		if pod.Status.Phase != corev1.PodRunning {
			return fmt.Errorf("mirror pod %s/%s phase is %s, want Running", namespace, mirrorPodName, pod.Status.Phase)
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready {
				return fmt.Errorf("container %s in mirror pod %s/%s is not ready", cs.Name, namespace, mirrorPodName)
			}
		}
		return nil
	}, timeoutExtMembersReady, pollingInterval).Should(Succeed(), "mirror pod should be ready before checking Etcd status")
}

// killEtcdWrapperProcess sends SIGKILL to the etcd-wrapper process on the given worker node.
// Kubelet will restart the container automatically after the process exits.
func killEtcdWrapperProcess(g *WithT, ordinal int) {
	g.Expect(dockerExec(workerName(ordinal), "pkill -9 etcd-wrapper")).To(Succeed())
}

// corruptMemberDataDir wipes the etcd data directory on a worker node to simulate data corruption.
// The static pod must be stopped before calling this to avoid the running etcd process racing
// with the directory removal.
func corruptMemberDataDir(g *WithT, namespace, etcdName string, ordinal int) {
	worker := workerName(ordinal)
	base := workerBaseDir(namespace, etcdName)
	g.Expect(dockerExec(worker, fmt.Sprintf("rm -rf %s/data && mkdir -p %s/data", base, base))).To(Succeed())
	chownNonroot(g, worker, fmt.Sprintf("%s/data", base))
}

// chownNonroot sets ownership of a directory on a worker node to UID/GID 65532,
// the nonroot user that etcd and backup-restore containers run as.
func chownNonroot(g *WithT, worker, path string) {
	g.Expect(dockerExec(worker, fmt.Sprintf("chown -R 65532:65532 %s", path))).To(Succeed())
}

// dockerExec runs a bash script on a KIND worker node via docker exec.
func dockerExec(worker, script string) error {
	cmd := exec.Command("docker", "exec", worker, "bash", "-c", script) // #nosec G204 -- e2e test utility
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// dockerCp copies a local file to a path inside a KIND worker node.
func dockerCp(localPath, worker, remotePath string) error {
	cmd := exec.Command("docker", "cp", localPath, fmt.Sprintf("%s:%s", worker, remotePath)) // #nosec G204 -- e2e test utility
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// copyTLSToWorker copies TLS cert files from local directories to the worker node
// and fixes ownership so the non-root etcd/backup-restore processes can read them.
func copyTLSToWorker(g *WithT, etcdName, namespace string, ordinal int, etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir string) {
	worker := workerName(ordinal)
	base := workerBaseDir(namespace, etcdName)

	type copyEntry struct {
		localDir  string
		remoteDir string
		files     [][2]string // [localName, remoteName]
	}
	entries := []copyEntry{
		{etcdCertsDir, base + "/etcd-ca", [][2]string{{"ca.crt", "ca.crt"}, {"ca.key", "ca.key"}}},
		{etcdCertsDir, base + "/etcd-server-tls", [][2]string{{"server.crt", "tls.crt"}, {"server.key", "tls.key"}}},
		{etcdCertsDir, base + "/etcd-client-tls", [][2]string{{"client.crt", "tls.crt"}, {"client.key", "tls.key"}}},
		{etcdPeerCertsDir, base + "/etcd-peer-ca", [][2]string{{"ca.crt", "ca.crt"}, {"ca.key", "ca.key"}}},
		{etcdPeerCertsDir, base + "/etcd-peer-server-tls", [][2]string{{"server.crt", "tls.crt"}, {"server.key", "tls.key"}}},
		{etcdbrCertsDir, base + "/backup-restore-ca", [][2]string{{"ca.crt", "ca.crt"}, {"ca.key", "ca.key"}}},
		{etcdbrCertsDir, base + "/backup-restore-server-tls", [][2]string{{"server.crt", "tls.crt"}, {"server.key", "tls.key"}}},
		{etcdbrCertsDir, base + "/backup-restore-client-tls", [][2]string{{"client.crt", "tls.crt"}, {"client.key", "tls.key"}}},
	}

	for _, e := range entries {
		for _, f := range e.files {
			g.Expect(dockerCp(filepath.Join(e.localDir, f[0]), worker, e.remoteDir+"/"+f[1])).To(Succeed())
		}
	}

	chownNonroot(g, worker, base)
}
