// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"hash/fnv"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/store"
	"github.com/gardener/etcd-druid/test/e2e/testenv"
	e2etestutils "github.com/gardener/etcd-druid/test/e2e/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	. "github.com/onsi/gomega"
)

const (
	// environment variables
	envKubeconfigPath      = "KUBECONFIG"
	envRetainTestArtifacts = "RETAIN_TEST_ARTIFACTS"
	envBackupProviders     = "PROVIDERS"

	// test parameters
	timeoutTest                = 1 * time.Hour
	timeoutEtcdCreation        = 5 * time.Minute
	timeoutEtcdDeletion        = 2 * time.Minute
	timeoutEtcdHibernation     = 2 * time.Minute
	timeoutEtcdUnhibernation   = 5 * time.Minute
	timeoutEtcdUpdation        = 10 * time.Minute
	timeoutEtcdDisruptionStart = 30 * time.Second
	timeoutEtcdRecovery        = 5 * time.Minute
	timeoutDeployJob           = 2 * time.Minute
	timeoutReconciliation      = 3 * time.Minute
	timeoutMemberLeases        = 5 * time.Minute
	timeoutExtMembersReady     = 10 * time.Minute

	pollingInterval = 2 * time.Second

	testNamespacePrefix = "etcd-e2e"
	defaultEtcdName     = "test"
)

var (
	testEnv             *testenv.TestEnvironment
	retainTestArtifacts retainTestArtifactsMode
	providers           = []druidv1alpha1.StorageProvider{"none"}
)

type retainTestArtifactsMode string

const (
	// retainTestArtifactsAll indicates that all test artifacts should be retained.
	retainTestArtifactsAll retainTestArtifactsMode = "all"
	// retainTestArtifactsFailed indicates that only artifacts from failed tests should be retained.
	retainTestArtifactsFailed retainTestArtifactsMode = "failed"
	// retainTestArtifactsNone indicates that no test artifacts should be retained.
	retainTestArtifactsNone retainTestArtifactsMode = "none"
)

const (
	pkiResourcesDir              = "pki-resources"
	extMembersResourcesDir       = "ext-members-resources"
	defaultBackupStoreSecretName = "etcd-backup"
	kindClusterNameEnv           = "KIND_CLUSTER_NAME"
	defaultKINDClusterName       = "etcd-druid-e2e"
)

// getProviderSuffix returns the storage provider suffix for the given storage provider,
// to be used for generating the namespace for a test case.
func getProviderSuffix(provider druidv1alpha1.StorageProvider) string {
	if provider == "none" {
		return "none"
	}
	p, err := store.StorageProviderFromInfraProvider(ptr.To(provider))
	if err != nil {
		return ""
	}
	return strings.ToLower(p)
}

// initializeTestCase sets up the test environment by creating a namespace, generating PKI resources,
// and creating the necessary TLS secrets and backup secret.
func initializeTestCase(g *WithT, testEnv *testenv.TestEnvironment, logger logr.Logger, testNamespace, etcdName string, provider druidv1alpha1.StorageProvider) {
	createNamespace(g, testEnv, logger, testNamespace)
	etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir := generatePKIResourcesToDefaultDirectory(g, logger, testNamespace, etcdName)
	createTLSSecrets(g, testEnv, logger, testNamespace, etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir)
	createBackupSecret(g, testEnv, logger, testNamespace, provider)
}

// initializeExternalMembersTestCase sets up the test environment for externally managed members tests.
// No PKI or backup secret needed for non-TLS tests.
func initializeExternalMembersTestCase(g *WithT, testEnv *testenv.TestEnvironment, logger logr.Logger, testNamespace string) {
	createNamespace(g, testEnv, logger, testNamespace)
}

// initializeExternalMembersTestCaseWithTLS sets up the test environment for externally managed
// members tests with TLS enabled. It generates PKI resources with IP SANs for the given member
// IPs and creates the corresponding TLS secrets.
func initializeExternalMembersTestCaseWithTLS(g *WithT, testEnv *testenv.TestEnvironment, logger logr.Logger, testNamespace, etcdName string, memberIPs []net.IP) (etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir string) {
	createNamespace(g, testEnv, logger, testNamespace)
	etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir = generatePKIResourcesWithIPSANs(g, logger, testNamespace, etcdName, memberIPs)
	createTLSSecrets(g, testEnv, logger, testNamespace, etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir)
	return etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir
}

// createNamespace creates a new namespace for testing.
func createNamespace(g *WithT, testEnv *testenv.TestEnvironment, logger logr.Logger, testNamespace string) {
	logger.Info("creating test namespace")
	g.Expect(testEnv.CreateTestNamespace(testNamespace)).To(Succeed())
	logger.Info("successfully created test namespace")
}

// generatePKIResourcesToDefaultDirectory generates PKI resources for etcd and returns the directories containing the generated certificates.
func generatePKIResourcesToDefaultDirectory(g *WithT, logger logr.Logger, testNamespace, etcdName string) (string, string, string) {
	logger.Info("generating PKI resources")
	certDir := fmt.Sprintf("%s/%s", pkiResourcesDir, testNamespace)
	// certs for etcd server-client communication
	etcdCertsDir := fmt.Sprintf("%s/etcd", certDir)
	g.Expect(os.MkdirAll(etcdCertsDir, 0755)).To(Succeed()) // #nosec: G301 -- local directory creation for test purposes.
	g.Expect(testutils.GeneratePKIResourcesToDirectory(logger, etcdCertsDir, etcdName, testNamespace)).To(Succeed())
	// certs for etcd peer communication
	etcdPeerCertsDir := fmt.Sprintf("%s/etcd-peer", certDir)
	g.Expect(os.MkdirAll(etcdPeerCertsDir, 0755)).To(Succeed()) // #nosec: G301 -- local directory creation for test purposes.
	g.Expect(testutils.GeneratePKIResourcesToDirectory(logger, etcdPeerCertsDir, etcdName, testNamespace)).To(Succeed())
	// certs for etcd-backup-restore TLS
	etcdbrCertsDir := fmt.Sprintf("%s/etcd-backup-restore", certDir)
	g.Expect(os.MkdirAll(etcdbrCertsDir, 0755)).To(Succeed()) // #nosec: G301 -- local directory creation for test purposes.
	g.Expect(testutils.GeneratePKIResourcesToDirectory(logger, etcdbrCertsDir, etcdName, testNamespace)).To(Succeed())
	logger.Info("successfully generated PKI resources")
	return etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir
}

// generatePKIResourcesWithIPSANs generates PKI resources with IP SANs for externally managed member certs.
func generatePKIResourcesWithIPSANs(g *WithT, logger logr.Logger, testNamespace, etcdName string, memberIPs []net.IP) (string, string, string) {
	logger.Info("generating PKI resources with IP SANs", "memberIPs", memberIPs)
	certDir := fmt.Sprintf("%s/%s", extMembersResourcesDir, testNamespace)

	etcdCertsDir := fmt.Sprintf("%s/etcd", certDir)
	g.Expect(os.MkdirAll(etcdCertsDir, 0755)).To(Succeed()) // #nosec: G301 -- local directory creation for test purposes.
	g.Expect(testutils.GeneratePKIResourcesWithIPSANs(logger, etcdCertsDir, etcdName, testNamespace, memberIPs)).To(Succeed())

	etcdPeerCertsDir := fmt.Sprintf("%s/etcd-peer", certDir)
	g.Expect(os.MkdirAll(etcdPeerCertsDir, 0755)).To(Succeed()) // #nosec: G301 -- local directory creation for test purposes.
	g.Expect(testutils.GeneratePKIResourcesWithIPSANs(logger, etcdPeerCertsDir, etcdName, testNamespace, memberIPs)).To(Succeed())

	etcdbrCertsDir := fmt.Sprintf("%s/etcd-backup-restore", certDir)
	g.Expect(os.MkdirAll(etcdbrCertsDir, 0755)).To(Succeed()) // #nosec: G301 -- local directory creation for test purposes.
	g.Expect(testutils.GeneratePKIResourcesWithIPSANs(logger, etcdbrCertsDir, etcdName, testNamespace, memberIPs)).To(Succeed())

	logger.Info("successfully generated PKI resources with IP SANs")
	return etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir
}

// createTLSSecrets creates the necessary TLS secrets in the specified namespace using the provided certificate directories.
func createTLSSecrets(g *WithT, testEnv *testenv.TestEnvironment, logger logr.Logger, testNamespace string, etcdCertsDir string, etcdPeerCertsDir string, etcdbrCertsDir string) {
	logger.Info("creating TLS secrets")
	// TLS secrets for etcd server-client communication
	g.Expect(e2etestutils.CreateCASecret(testEnv.Context(), testEnv.Client(), testutils.ClientTLSCASecretName, testNamespace, etcdCertsDir)).To(Succeed())
	g.Expect(e2etestutils.CreateServerTLSSecret(testEnv.Context(), testEnv.Client(), testutils.ClientTLSServerCertSecretName, testNamespace, etcdCertsDir)).To(Succeed())
	g.Expect(e2etestutils.CreateClientTLSSecret(testEnv.Context(), testEnv.Client(), testutils.ClientTLSClientCertSecretName, testNamespace, etcdCertsDir)).To(Succeed())
	// TLS secrets for etcd peer communication
	g.Expect(e2etestutils.CreateCASecret(testEnv.Context(), testEnv.Client(), testutils.PeerTLSCASecretName, testNamespace, etcdPeerCertsDir)).To(Succeed())
	g.Expect(e2etestutils.CreateServerTLSSecret(testEnv.Context(), testEnv.Client(), testutils.PeerTLSServerCertSecretName, testNamespace, etcdPeerCertsDir)).To(Succeed())
	// TLS secrets for etcd-backup-restore TLS
	g.Expect(e2etestutils.CreateCASecret(testEnv.Context(), testEnv.Client(), testutils.BackupRestoreTLSCASecretName, testNamespace, etcdbrCertsDir)).To(Succeed())
	g.Expect(e2etestutils.CreateServerTLSSecret(testEnv.Context(), testEnv.Client(), testutils.BackupRestoreTLSServerCertSecretName, testNamespace, etcdbrCertsDir)).To(Succeed())
	g.Expect(e2etestutils.CreateClientTLSSecret(testEnv.Context(), testEnv.Client(), testutils.BackupRestoreTLSClientCertSecretName, testNamespace, etcdbrCertsDir)).To(Succeed())
	logger.Info("successfully created TLS secrets")
}

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

// createBackupSecret creates a backup secret in the specified namespace.
func createBackupSecret(g *WithT, testEnv *testenv.TestEnvironment, logger logr.Logger, namespace string, provider druidv1alpha1.StorageProvider) {
	logger.Info("creating backup secret")
	g.Expect(testutils.CreateBackupSecret(testEnv.Context(), testEnv.Client(), defaultBackupStoreSecretName, namespace, provider)).To(Succeed())
	logger.Info("successfully created backup secret")
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

// cleanupTestArtifacts deletes the test namespace if shouldCleanup is true.
func cleanupTestArtifacts(retainTestArtifacts retainTestArtifactsMode, testSucceeded bool, testEnv *testenv.TestEnvironment, logger logr.Logger, g *WithT, ns string) {
	if !shouldCleanup(retainTestArtifacts, testSucceeded) {
		logger.Info("retaining test artifacts as per configuration")
		return
	}

	logger.Info(fmt.Sprintf("deleting namespace %s", ns))
	g.Expect(testEnv.DeleteTestNamespace(ns)).To(Succeed())
	logger.Info("successfully deleted namespace")
}

// shouldCleanupWorkers returns true if worker node artifacts should be cleaned up.
func shouldCleanupWorkers(retainTestArtifacts retainTestArtifactsMode, testSucceeded bool) bool {
	return shouldCleanup(retainTestArtifacts, testSucceeded)
}

// shouldCleanup returns true if test artifacts should be cleaned up based on the retain mode and test result.
func shouldCleanup(retainTestArtifacts retainTestArtifactsMode, testSucceeded bool) bool {
	switch retainTestArtifacts {
	case retainTestArtifactsAll:
		return false
	case retainTestArtifactsFailed:
		return testSucceeded
	default:
		return true
	}
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
	clusterName := os.Getenv(kindClusterNameEnv)
	if clusterName == "" {
		clusterName = defaultKINDClusterName
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
  %[1]s/backup-restore-ca %[1]s/backup-restore-server-tls %[1]s/backup-restore-client-tls`, base)
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

	saDir := filepath.Join(extMembersResourcesDir, namespace, "serviceaccount")
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
func deployStaticPod(g *WithT, ctx context.Context, cl client.Client, etcdName, namespace string, ordinal int, saTokenFile, caCertFile string) {
	worker := workerName(ordinal)
	base := workerBaseDir(namespace, etcdName)

	writeConfigToWorker(g, ctx, cl, etcdName, namespace, ordinal)

	saDir := base + "/serviceaccount"
	g.Expect(dockerCp(saTokenFile, worker, saDir+"/token")).To(Succeed())
	g.Expect(dockerCp(caCertFile, worker, saDir+"/ca.crt")).To(Succeed())
	chownNonroot(g, worker, saDir)

	podYAML, err := translateStatefulSetToPod(ctx, cl, etcdName, namespace)
	g.Expect(err).ToNot(HaveOccurred())

	writeManifest(g, worker, namespace, etcdName, podYAML)
}

// updateWorkerConfig updates config files on a worker without restarting the pod.
func updateWorkerConfig(g *WithT, ctx context.Context, cl client.Client, etcdName, namespace string, ordinal int) {
	writeConfigToWorker(g, ctx, cl, etcdName, namespace, ordinal)
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
	pod.Spec.AutomountServiceAccountToken = nil
	pod.Spec.Tolerations = []corev1.Toleration{
		{Key: "node.gardener.cloud/etcd-machine", Value: "true", Effect: corev1.TaintEffectNoSchedule},
		{Effect: corev1.TaintEffectNoExecute, Operator: corev1.TolerationOpExists},
	}

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
	localPath := filepath.Join(extMembersResourcesDir, namespace, manifest+".yaml")
	g.Expect(os.WriteFile(localPath, podYAML, 0600)).To(Succeed()) // #nosec G306 -- test file
	g.Expect(dockerCp(localPath, worker, fmt.Sprintf("/etc/kubernetes/manifests/%s.yaml", manifest))).To(Succeed())
}

// waitForReconciliation waits until the Etcd's observed generation matches its generation.
func waitForReconciliation(g *WithT, etcd *druidv1alpha1.Etcd) {
	g.Eventually(func() error {
		if err := testEnv.Client().Get(testEnv.Context(), client.ObjectKeyFromObject(etcd), etcd); err != nil {
			return err
		}
		if etcd.Status.ObservedGeneration == nil {
			return fmt.Errorf("observed generation is nil")
		}
		if *etcd.Status.ObservedGeneration != etcd.Generation {
			return fmt.Errorf("observed generation %d != generation %d", *etcd.Status.ObservedGeneration, etcd.Generation)
		}
		return nil
	}, timeoutReconciliation, pollingInterval).Should(Succeed())
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
