// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path"
	"strings"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	druidstore "github.com/gardener/etcd-druid/internal/store"

	"github.com/gardener/etcd-backup-restore/pkg/snapstore"
	brtypes "github.com/gardener/etcd-backup-restore/pkg/types"
	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	envProviders = "INFRA_PROVIDERS"

	envS3AccessKeyID     = "AWS_ACCESS_KEY_ID"
	envS3SecretAccessKey = "AWS_SECRET_ACCESS_KEY" // #nosec G101 -- refers to the env var, and is not the credential itself.
	envS3Region          = "AWS_REGION"

	envABSStorageAccount = "STORAGE_ACCOUNT"
	envABSStorageKey     = "STORAGE_KEY"

	envGCSServiceAccount = "GCP_SERVICEACCOUNT_JSON_PATH"

	etcdBackupSecretPrefix = "etcd-backup"
	etcdPrefix             = "etcd"

	etcdCommandMaxRetries = 3

	debugPodName          = "etcd-debug"
	debugPodContainerName = "etcd-debug"
)

// Storage contains information about the storage provider.
type Storage struct {
	Provider   druidv1alpha1.StorageProvider
	SecretData map[string][]byte
}

// TestProvider contains test related information.
type TestProvider struct {
	Name    string
	Suffix  string
	Storage *Storage
}

var (
	// TLS cert-pairs have been pre-built with DNS entries
	// for etcds residing in `shoot` namespace.
	// Changing namespace requires rebuilding these certs
	// Refer https://github.com/gardener/etcd-backup-restore/blob/master/doc/usage/generating_ssl_certificates.md
	// to build new certs for the tests
	namespace = "shoot"

	roleLabelKey          = "role"
	defaultRoleLabelValue = "main"

	labels = map[string]string{
		"app":        "etcd-statefulset",
		roleLabelKey: defaultRoleLabelValue,
	}

	stsLabels = map[string]string{
		"app":                              "etcd-statefulset",
		roleLabelKey:                       defaultRoleLabelValue,
		"networking.gardener.cloud/to-dns": "allowed",
		"networking.gardener.cloud/to-private-networks": "allowed",
		"networking.gardener.cloud/to-public-networks":  "allowed",
	}
	stsAnnotations = map[string]string{}

	etcdMetrics                 = "basic"
	etcdDefragmentationSchedule = "0 */24 * * *"
	etcdQuota                   = resource.MustParse("2Gi")
	etcdResources               = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			"cpu":    resource.MustParse("100m"),
			"memory": resource.MustParse("256Mi"),
		},
	}
	etcdClientPort    = int32(2379)
	etcdServerPort    = int32(2380)
	etcdSnapshotCount = int64(75000)

	backupPort                 = int32(8080)
	backupFullSnapshotSchedule = "0 */1 * * *"
	backupResources            = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			"cpu":    resource.MustParse("100m"),
			"memory": resource.MustParse("256Mi"),
		},
	}
	backupGarbageCollectionPolicy  = druidv1alpha1.GarbageCollectionPolicy(druidv1alpha1.GarbageCollectionPolicyExponential)
	backupGarbageCollectionPeriod  = metav1.Duration{Duration: 5 * time.Minute}
	backupDeltaSnapshotPeriod      = metav1.Duration{Duration: 1 * time.Second}
	backupDeltaSnapshotMemoryLimit = resource.MustParse("100Mi")
	gzipCompression                = druidv1alpha1.GzipCompression
	backupCompression              = druidv1alpha1.CompressionSpec{
		Enabled: ptr.To(true),
		Policy:  &gzipCompression,
	}
	defaultBackupStore = druidv1alpha1.StoreSpec{
		SecretRef: &corev1.SecretReference{
			Name:      etcdBackupSecretPrefix,
			Namespace: namespace,
		},
	}

	autoCompactionMode      = druidv1alpha1.Periodic
	autoCompactionRetention = "2m"
	sharedConfig            = druidv1alpha1.SharedConfig{
		AutoCompactionMode:      &autoCompactionMode,
		AutoCompactionRetention: &autoCompactionRetention,
	}

	storageCapacity = resource.MustParse("10Gi")
)

const (
	replicas              int32 = 1
	multiNodeEtcdReplicas int32 = 3
)

func getEmptyEtcd(name, namespace string) *druidv1alpha1.Etcd {
	return &druidv1alpha1.Etcd{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
}

func getDefaultEtcd(name, namespace, container, prefix string, provider TestProvider) *druidv1alpha1.Etcd {
	etcd := getEmptyEtcd(name, namespace)

	etcd.Spec.Annotations = stsAnnotations

	labelsCopy := make(map[string]string)
	maps.Copy(labelsCopy, labels)
	labelsCopy[roleLabelKey] = provider.Suffix
	etcd.Labels = labelsCopy

	stsLabelsCopy := make(map[string]string)
	maps.Copy(stsLabelsCopy, stsLabels)
	stsLabelsCopy[roleLabelKey] = provider.Suffix
	etcd.Spec.Labels = stsLabelsCopy

	etcdTLS := defaultTls(provider.Suffix)

	etcd.Spec.Etcd = druidv1alpha1.EtcdConfig{
		Metrics:                 (*druidv1alpha1.MetricsLevel)(&etcdMetrics),
		DefragmentationSchedule: &etcdDefragmentationSchedule,
		Quota:                   &etcdQuota,
		Resources:               &etcdResources,
		ClientPort:              &etcdClientPort,
		ServerPort:              &etcdServerPort,
		SnapshotCount:           &etcdSnapshotCount,
		ClientUrlTLS:            &etcdTLS,
	}

	etcd.Spec.Common = sharedConfig
	etcd.Spec.Replicas = int32(replicas)
	etcd.Spec.StorageCapacity = &storageCapacity

	if provider.Storage != nil {
		backupStore := defaultBackupStore.DeepCopy()
		backupStore.Container = &container
		backupStore.Provider = &provider.Storage.Provider
		backupStore.Prefix = prefix
		backupStore.SecretRef = &corev1.SecretReference{
			Name:      fmt.Sprintf("%s-%s", etcdBackupSecretPrefix, provider.Suffix),
			Namespace: namespace,
		}

		backupTLS := defaultTls(provider.Suffix)

		etcd.Spec.Backup = druidv1alpha1.BackupSpec{
			Port:                     &backupPort,
			TLS:                      &backupTLS,
			FullSnapshotSchedule:     &backupFullSnapshotSchedule,
			Resources:                &backupResources,
			GarbageCollectionPolicy:  &backupGarbageCollectionPolicy,
			GarbageCollectionPeriod:  &backupGarbageCollectionPeriod,
			DeltaSnapshotPeriod:      &backupDeltaSnapshotPeriod,
			DeltaSnapshotMemoryLimit: &backupDeltaSnapshotMemoryLimit,
			SnapshotCompression:      &backupCompression,
			Store:                    backupStore,
		}
	}

	return etcd
}

func getDefaultMultiNodeEtcd(name, namespace, container, prefix string, provider TestProvider) *druidv1alpha1.Etcd {
	etcd := getDefaultEtcd(name, namespace, container, prefix, provider)
	etcd.Spec.Replicas = multiNodeEtcdReplicas
	etcd.Spec.Etcd.PeerUrlTLS = getPeerTls(provider.Suffix)
	return etcd
}

func defaultTls(provider string) druidv1alpha1.TLSConfig {
	return druidv1alpha1.TLSConfig{
		ServerTLSSecretRef: corev1.SecretReference{
			Name:      fmt.Sprintf("%s-%s", "etcd-server-cert", provider),
			Namespace: namespace,
		},
		ClientTLSSecretRef: corev1.SecretReference{
			Name:      fmt.Sprintf("%s-%s", "etcd-client-tls", provider),
			Namespace: namespace,
		},
		TLSCASecretRef: druidv1alpha1.SecretReference{
			SecretReference: corev1.SecretReference{
				Name:      fmt.Sprintf("%s-%s", "ca-etcd", provider),
				Namespace: namespace,
			},
		},
	}
}

func getPeerTls(provider string) *druidv1alpha1.TLSConfig {
	return &druidv1alpha1.TLSConfig{
		ServerTLSSecretRef: corev1.SecretReference{
			Name:      fmt.Sprintf("%s-%s", "etcd-server-cert", provider),
			Namespace: namespace,
		},
		TLSCASecretRef: druidv1alpha1.SecretReference{
			SecretReference: corev1.SecretReference{
				Name:      fmt.Sprintf("%s-%s", "ca-etcd", provider),
				Namespace: namespace,
			},
		},
	}
}

// EndpointStatus stores result from output of etcdctl endpoint status command
type EndpointStatus []struct {
	Endpoint string `json:"Endpoint"`
	Status   struct {
		Header struct {
			ClusterID int64 `json:"cluster_id"`
			MemberID  int64 `json:"member_id"`
			Revision  int64 `json:"revision"`
			RaftTerm  int64 `json:"raft_term"`
		} `json:"header"`
		Version   string `json:"version"`
		DbSize    int64  `json:"dbSize"`
		Leader    int64  `json:"leader"`
		RaftIndex int64  `json:"raftIndex"`
		RaftTerm  int64  `json:"raftTerm"`
	} `json:"Status"`
}

// SnapListResult stores the snaplist and any associated error
type SnapListResult struct {
	Snapshots brtypes.SnapList `json:"snapshots"`
	Error     error            `json:"error"`
}

// LatestSnapshots stores the result from output of /snapshot/latest http call
type LatestSnapshots struct {
	FullSnapshot   *brtypes.Snapshot   `json:"fullSnapshot"`
	DeltaSnapshots []*brtypes.Snapshot `json:"deltaSnapshots"`
}

func getEnvOrError(key string) (string, error) {
	if value, ok := os.LookupEnv(key); ok {
		return value, nil
	}

	return "", fmt.Errorf("environment variable not found: %s", key)
}

func getEnvOrFallback(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	return fallback
}

const (
	providerAWS   = "aws"
	providerAzure = "azure"
	providerGCP   = "gcp"
	providerLocal = "local"
	providerNone  = "none"
)

func getProviders() ([]TestProvider, error) {
	providerNames := strings.Split(getEnvOrFallback(envProviders, ""), ",")

	var providers []TestProvider

	for _, p := range providerNames {
		var provider TestProvider
		switch p {
		case providerAWS:
			s3AccessKeyID := getEnvOrFallback(envS3AccessKeyID, "")
			s3SecretAccessKey := getEnvOrFallback(envS3SecretAccessKey, "")
			s3Region := getEnvOrFallback(envS3Region, "")
			if s3AccessKeyID != "" && s3SecretAccessKey != "" && s3Region != "" {
				provider = TestProvider{
					Name:   "aws",
					Suffix: "aws",
					Storage: &Storage{
						Provider: druidstore.S3,
						SecretData: map[string][]byte{
							"accessKeyID":     []byte(s3AccessKeyID),
							"secretAccessKey": []byte(s3SecretAccessKey),
							"region":          []byte(s3Region),
						},
					},
				}
				localStackHost := getEnvOrFallback("LOCALSTACK_HOST", "")
				if localStackHost != "" {
					provider.Storage.SecretData["endpoint"] = []byte("http://" + localStackHost)
					provider.Storage.SecretData["s3ForcePathStyle"] = []byte("true")
				}
			}
		case providerAzure:
			absStorageAccount := getEnvOrFallback(envABSStorageAccount, "")
			absStorageKey := getEnvOrFallback(envABSStorageKey, "")
			if absStorageAccount != "" && absStorageKey != "" {
				provider = TestProvider{
					Name:   "az",
					Suffix: "az",
					Storage: &Storage{
						Provider: druidstore.ABS,
						SecretData: map[string][]byte{
							"storageAccount": []byte(absStorageAccount),
							"storageKey":     []byte(absStorageKey),
						},
					},
				}
				azuriteDomain := getEnvOrFallback("AZURITE_DOMAIN", "")
				if azuriteDomain != "" {
					provider.Storage.SecretData["emulatorEnabled"] = []byte("true")
					provider.Storage.SecretData["domain"] = []byte(azuriteDomain)
				}
			}
		case providerGCP:
			gcsServiceAccountPath := getEnvOrFallback(envGCSServiceAccount, "")
			if gcsServiceAccountPath != "" {
				gcsSA, err := os.ReadFile(gcsServiceAccountPath) // #nosec G304 -- test files.
				if err != nil {
					return nil, err
				}

				provider = TestProvider{
					Name:   "gcp",
					Suffix: "gcp",
					Storage: &Storage{
						Provider: druidstore.GCS,
						SecretData: map[string][]byte{
							"serviceaccount.json": gcsSA,
						},
					},
				}
				fakegcsHost := getEnvOrFallback("FAKEGCS_HOST", "")
				if fakegcsHost != "" {
					provider.Storage.SecretData["storageAPIEndpoint"] = []byte("http://" + fakegcsHost + "/storage/v1/")
					provider.Storage.SecretData["emulatorEnabled"] = []byte("true")
				}
			}
		case providerLocal:
			provider = TestProvider{
				Name:   "local",
				Suffix: "local",
				Storage: &Storage{
					Provider:   druidstore.Local,
					SecretData: map[string][]byte{},
				},
			}
		}
		providers = append(providers, provider)
	}

	return providers, nil
}

func isEmulatorEnabled(provider TestProvider) bool {
	switch provider.Name {
	case "aws":
		return provider.Storage.SecretData["endpoint"] != nil
	case "az", "gcp":
		if val, ok := provider.Storage.SecretData["emulatorEnabled"]; ok {
			return string(val) == "true"
		}
		return false
	case "local":
		return false
	}
	return false
}

func getKubeconfig(kubeconfigPath string) (*rest.Config, error) {
	return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
}

func getKubernetesTypedClient(kubeconfigPath string) (*kubernetes.Clientset, error) {
	config, err := getKubeconfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

func getKubernetesClient(kubeconfigPath string) (client.Client, error) {
	config, err := getKubeconfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	return client.New(config, client.Options{})
}

func deploySecret(ctx context.Context, cl client.Client, logger logr.Logger, name, namespace string, labels map[string]string, secretType corev1.SecretType, secretData map[string][]byte) error {
	secret := corev1.Secret{}
	secret.Name = name
	secret.Namespace = namespace
	secret.Labels = labels
	secret.Type = secretType
	secret.Data = secretData

	logger.Info("creating secret", "secret", client.ObjectKeyFromObject(&secret))
	return cl.Create(ctx, &secret)
}

func buildAndDeployTLSSecrets(ctx context.Context, cl client.Client, logger logr.Logger, namespace, certsPath string, providers []TestProvider) error {
	var (
		caCertFile          = "ca.crt"
		caKeyFile           = "ca.key"
		caSecretName        = "ca-etcd"
		tlsServerCertFile   = "server.crt"
		tlsServerKeyFile    = "server.key"
		tlsServerSecretName = "etcd-server-cert" // #nosec G101 -- refers to the name of the secret, and is not the credential itself.
		tlsClientCertFile   = "client.crt"
		tlsClientKeyFile    = "client.key"
		tlsClientSecretName = "etcd-client-tls" // #nosec G101 -- refers to the name of the secret, and is not the credential itself.
		secretData          map[string][]byte
	)

	for _, provider := range providers {
		caCert, err := os.ReadFile(path.Join(certsPath, caCertFile)) // #nosec G304 -- test files.
		if err != nil {
			return err
		}
		caKey, err := os.ReadFile(path.Join(certsPath, caKeyFile)) // #nosec G304 -- test files.
		if err != nil {
			return err
		}
		secretData = map[string][]byte{
			"ca.crt": caCert,
			"ca.key": caKey,
		}
		secretName := fmt.Sprintf("%s-%s", caSecretName, provider.Suffix)
		if err := deploySecret(ctx, cl, logger, secretName, namespace, labels, corev1.SecretTypeOpaque, secretData); err != nil {
			return err
		}

		tlsServerCert, err := os.ReadFile(path.Join(certsPath, tlsServerCertFile)) // #nosec G304 -- test files.
		if err != nil {
			return err
		}
		tlsServerKey, err := os.ReadFile(path.Join(certsPath, tlsServerKeyFile)) // #nosec G304 -- test files.
		if err != nil {
			return err
		}
		secretData = map[string][]byte{
			"ca.crt":  caCert,
			"tls.crt": tlsServerCert,
			"tls.key": tlsServerKey,
		}
		secretName = fmt.Sprintf("%s-%s", tlsServerSecretName, provider.Suffix)
		if err := deploySecret(ctx, cl, logger, secretName, namespace, labels, corev1.SecretTypeTLS, secretData); err != nil {
			return err
		}

		tlsClientCert, err := os.ReadFile(path.Join(certsPath, tlsClientCertFile)) // #nosec G304 -- test files.
		if err != nil {
			return err
		}
		tlsClientKey, err := os.ReadFile(path.Join(certsPath, tlsClientKeyFile)) // #nosec G304 -- test files.
		if err != nil {
			return err
		}
		secretData = map[string][]byte{
			"ca.crt":  caCert,
			"tls.crt": tlsClientCert,
			"tls.key": tlsClientKey,
		}
		secretName = fmt.Sprintf("%s-%s", tlsClientSecretName, provider.Suffix)
		if err := deploySecret(ctx, cl, logger, secretName, namespace, labels, corev1.SecretTypeTLS, secretData); err != nil {
			return err
		}
	}

	return nil
}

func deployBackupSecret(ctx context.Context, cl client.Client, logger logr.Logger, provider TestProvider, namespace, storageContainer string) error {

	if provider.Storage == nil || provider.Storage.SecretData == nil {
		return nil
	}
	secretData := provider.Storage.SecretData
	providerSuffix := provider.Suffix
	secretName := fmt.Sprintf("%s-%s", etcdBackupSecretPrefix, providerSuffix)

	etcdBackupSecret := corev1.Secret{}
	etcdBackupSecret.Name = secretName
	etcdBackupSecret.Namespace = namespace
	etcdBackupSecret.Labels = labels
	etcdBackupSecret.Type = corev1.SecretTypeOpaque
	etcdBackupSecret.Data = secretData
	etcdBackupSecret.Data["bucketName"] = []byte(storageContainer)

	logger.Info("creating secret", "secret", client.ObjectKeyFromObject(&etcdBackupSecret))

	return cl.Create(ctx, &etcdBackupSecret)
}

// getRemoteCommandExecutor builds and returns a remote command Executor from the given command on the specified container
func getRemoteCommandExecutor(kubeconfigPath, namespace, podName, containerName, command string) (remotecommand.Executor, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	coreClient := clientset.CoreV1()

	req := coreClient.RESTClient().
		Post().
		Namespace(namespace).
		Resource("pods").
		Name(podName).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command: []string{
				"/bin/sh",
				"-c",
				command,
			},
			Stdin:  false,
			Stdout: true,
			Stderr: true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return nil, err
	}

	return exec, nil
}

// executeRemoteCommand executes a remote shell command on the given pod and container
// and returns the stdout and stderr logs
func executeRemoteCommand(ctx context.Context, kubeconfigPath, namespace, podName, containerName, command string) (string, string, error) {
	exec, err := getRemoteCommandExecutor(kubeconfigPath, namespace, podName, containerName, command)
	if err != nil {
		return "", "", err
	}

	buf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	if err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: buf,
		Stderr: errBuf,
	}); err != nil {
		return "", "", err
	}

	return strings.TrimSpace(buf.String()), strings.TrimSpace(errBuf.String()), nil
}

func getSnapstore(storageProvider, storageContainer, storePrefix string, isEmulatorEnabled bool) (brtypes.SnapStore, error) {
	snapstoreConfig := &brtypes.SnapstoreConfig{
		Provider:          storageProvider,
		Container:         storageContainer,
		Prefix:            path.Join(storePrefix, "v2"),
		IsEmulatorEnabled: isEmulatorEnabled,
	}
	store, err := snapstore.GetSnapstore(snapstoreConfig)
	if err != nil {
		return nil, err
	}

	return store, nil
}

func purgeSnapstore(store brtypes.SnapStore) error {
	// List all snapshots since the snapstore is being purged
	snapList, err := store.List(true)
	if err != nil {
		return err
	}

	for _, snap := range snapList {
		if err = store.Delete(*snap); err != nil {
			return err
		}
	}

	return nil
}

func getPurgeLocalSnapstoreJob(storeContainer, storePrefix string) *batchv1.Job {
	directory := corev1.HostPathDirectory

	return newTestHelperJob(
		"purge-local-snapstore",
		&corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "host-dir",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/etc",
							Type: &directory,
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:    "infra",
					Image:   "ubuntu:24.04",
					Command: []string{"/bin/bash"},
					Args: []string{"-c",
						fmt.Sprintf("rm -rf /host-dir-etc/gardener/local-backupbuckets/%s/%s/*", storeContainer, storePrefix),
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "host-dir",
							MountPath: "/host-dir-etc",
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	)
}

func populateEtcd(ctx context.Context, logger logr.Logger, kubeconfigPath, namespace, etcdName, podName, containerName, keyPrefix, valuePrefix string, startKeyNo, endKeyNo int, _ time.Duration) error {
	var (
		cmd     string
		stdout  string
		stderr  string
		retries = 0
		err     error
	)
	install := "echo '" +
		"ETCD_VERSION=${ETCD_VERSION:-v3.5.6};" +
		"curl -L https://github.com/coreos/etcd/releases/download/$ETCD_VERSION/etcd-$ETCD_VERSION-linux-amd64.tar.gz -o etcd-$ETCD_VERSION-linux-amd64.tar.gz;" +
		"tar xzvf etcd-$ETCD_VERSION-linux-amd64.tar.gz; rm etcd-$ETCD_VERSION-linux-amd64.tar.gz;" +
		"cd etcd-$ETCD_VERSION-linux-amd64; cp etcd /usr/local/bin/; cp etcdctl /usr/local/bin/;" +
		"rm -rf etcd-$ETCD_VERSION-linux-amd64;'" +
		" > test.sh && sh test.sh"

	stdout, stderr, err = executeRemoteCommand(ctx, kubeconfigPath, namespace, podName, containerName, install)
	if err != nil {
		logger.Error(err, fmt.Sprintf("Failed to inatall etcdctl. err: %s, stderr: %s, stdout:%s", err, stderr, stdout))
	}

	for i := startKeyNo; i <= endKeyNo; {
		cmd = fmt.Sprintf("ETCDCTL_API=3 etcdctl --endpoints=https://%s-client.shoot.svc:%d --cacert /var/etcd/ssl/ca/ca.crt --cert=/var/etcd/ssl/client/tls.crt --key=/var/etcd/ssl/client/tls.key put %s-%d %s-%d", etcdName, etcdClientPort, keyPrefix, i, valuePrefix, i)
		stdout, stderr, err = executeRemoteCommand(ctx, kubeconfigPath, namespace, podName, containerName, cmd)
		if err != nil || stderr != "" || stdout != "OK" {
			logger.Error(err, fmt.Sprintf("failed to put (%s-%d, %s-%d): stdout: %s; stderr: %s. Retrying", keyPrefix, i, valuePrefix, i, stdout, stderr))
			retries++
			if retries >= etcdCommandMaxRetries {
				return fmt.Errorf("failed to put (%s-%d, %s-%d): stdout: %s; stderr: %s; err: %v", keyPrefix, i, valuePrefix, i, stdout, stderr, err)
			}
			continue
		}
		logger.Info("put key-value successful", "key", fmt.Sprintf("%s-%d", keyPrefix, i), "value", fmt.Sprintf("%s-%d", valuePrefix, i))
		retries = 0
		i++
	}

	return nil
}

func getEtcdKey(ctx context.Context, kubeconfigPath, namespace, etcdName, podName, containerName, keyPrefix string, suffix int) (string, string, error) {
	var (
		cmd    string
		stdout string
		stderr string
		err    error
	)

	cmd = fmt.Sprintf("ETCDCTL_API=3 etcdctl --endpoints=https://%s-client.shoot.svc:%d --cacert /var/etcd/ssl/ca/ca.crt --cert=/var/etcd/ssl/client/tls.crt --key=/var/etcd/ssl/client/tls.key get %s-%d", etcdName, etcdClientPort, keyPrefix, suffix)
	stdout, stderr, err = executeRemoteCommand(ctx, kubeconfigPath, namespace, podName, containerName, cmd)
	if err != nil || stderr != "" {
		return "", "", fmt.Errorf("failed to get %s-%d: stdout: %s; stderr: %s; err: %v", keyPrefix, suffix, stdout, stderr, err)
	}
	splits := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(splits) != 2 {
		return "", "", fmt.Errorf("error splitting stdout into 2 parts: stdout: %s", stdout)
	}

	return strings.TrimSpace(splits[0]), strings.TrimSpace(splits[1]), nil
}

func getEtcdKeys(ctx context.Context, logger logr.Logger, kubeconfigPath, namespace, etcdName, podName, containerName, keyPrefix string, start, end int) (map[string]string, error) {
	var (
		key         string
		val         string
		retries     = 0
		keyValueMap = make(map[string]string)
		err         error
	)
	for i := start; i <= end; {
		key, val, err = getEtcdKey(ctx, kubeconfigPath, namespace, etcdName, podName, containerName, keyPrefix, i)
		if err != nil {
			logger.Info("failed to get key. Retrying...", "key", fmt.Sprintf("%s-%d", keyPrefix, i))
			retries++
			if retries >= etcdCommandMaxRetries {
				return nil, fmt.Errorf("failed to get key %s-%d", keyPrefix, i)
			}
			continue
		}
		retries = 0
		logger.Info("fetched key-value pair from etcd", "key", key, "value", val)
		keyValueMap[key] = val
		i++
	}

	return keyValueMap, nil
}

func triggerOnDemandSnapshot(ctx context.Context, kubeconfigPath, namespace, etcdName, podName, containerName string, port int, snapshotKind string) (*brtypes.Snapshot, error) {
	var (
		snapshot *brtypes.Snapshot
		snapKind string
	)

	switch snapshotKind {
	case brtypes.SnapshotKindFull:
		snapKind = "full"
	case brtypes.SnapshotKindDelta:
		snapKind = "delta"
	default:
		return nil, fmt.Errorf("invalid snapshotKind %s", snapshotKind)
	}
	cmd := fmt.Sprintf("curl https://%s-client.shoot.svc:%d/snapshot/%s -k -s", etcdName, port, snapKind)
	stdout, stderr, err := executeRemoteCommand(ctx, kubeconfigPath, namespace, podName, containerName, cmd)
	if err != nil || stdout == "" {
		return nil, fmt.Errorf("failed to trigger on-demand %s snapshot for %s: stdout: %s; stderr: %s; err: %v", snapKind, podName, stdout, stderr, err)
	}
	if err = json.Unmarshal([]byte(stdout), &snapshot); err != nil {
		return nil, err
	}

	return snapshot, nil
}

func getLatestSnapshots(ctx context.Context, kubeconfigPath, namespace, etcdName, podName, containerName string, port int) (*LatestSnapshots, error) {
	var latestSnapshots *LatestSnapshots
	cmd := fmt.Sprintf("curl https://%s-client.shoot.svc:%d/snapshot/latest -k -s", etcdName, port)
	stdout, stderr, err := executeRemoteCommand(ctx, kubeconfigPath, namespace, podName, containerName, cmd)
	if err != nil || stdout == "" {
		return nil, fmt.Errorf("failed to fetch latest snapshots taken for %s: stdout: %s; stderr: %s; err: %v", podName, stdout, stderr, err)
	}
	if err = json.Unmarshal([]byte(stdout), &latestSnapshots); err != nil {
		return nil, err
	}

	latestSnapshots.FullSnapshot.CreatedOn = latestSnapshots.FullSnapshot.CreatedOn.Truncate(time.Second)
	for _, snap := range latestSnapshots.DeltaSnapshots {
		snap.CreatedOn = snap.CreatedOn.Truncate(time.Second)
	}

	return latestSnapshots, nil
}

func deleteDir(ctx context.Context, kubeconfigPath, namespace, podName, containerName string, dirPath string) error {
	cmd := fmt.Sprintf("rm -rf %s", dirPath)
	stdout, stderr, err := executeRemoteCommand(ctx, kubeconfigPath, namespace, podName, containerName, cmd)
	if err != nil || stdout != "" {
		return fmt.Errorf("failed to delete directory %s for %s: stdout: %s; stderr: %s; err: %v", dirPath, podName, stdout, stderr, err)
	}
	return nil
}

func getEnvAndExpectNoError(key string) string {
	val, err := getEnvOrError(key)
	utilruntime.Must(err)
	return val
}

// newTestHelperJob returns the K8s Job for given commands to be executed inside k8s cluster.
// This test helper job can be used to validate test cases from inside the k8s cluster by executing the bash scripts.
func newTestHelperJob(jobName string, podSpec *corev1.PodSpec) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: *podSpec,
			},
			BackoffLimit: ptr.To[int32](0),
		},
	}
}

// etcdZeroDownTimeValidatorJob creates a Kubernetes job that validates the zero downtime of an
// Etcd cluster by continuously checking the cluster's health. The job fails if a health check fails,
// resulting in the associated pod entering an error state.
func etcdZeroDownTimeValidatorJob(etcdSvc, testName string, tls *druidv1alpha1.TLSConfig) *batchv1.Job {
	return newTestHelperJob(
		fmt.Sprintf("etcd-zero-down-time-validator-%s", testName),
		&corev1.PodSpec{
			Volumes: []corev1.Volume{
				createTLSVolume("client-url-ca-etcd", tls.TLSCASecretRef.Name),
				createTLSVolume("client-url-etcd-server-tls", tls.ServerTLSSecretRef.Name),
				createTLSVolume("client-url-etcd-client-tls", tls.ClientTLSSecretRef.Name),
			},
			Containers: []corev1.Container{
				{
					Name:    fmt.Sprintf("etcd-zero-down-time-validator-%s", testName),
					Image:   "alpine/curl",
					Command: []string{"/bin/sh", "-c"},
					Args:    []string{generateHealthCheckScript(etcdSvc)},
					VolumeMounts: []corev1.VolumeMount{
						{MountPath: "/var/etcd/ssl/ca", Name: "client-url-ca-etcd"},
						{MountPath: "/var/etcd/ssl/server", Name: "client-url-etcd-server-tls"},
						{MountPath: "/var/etcd/ssl/client", Name: "client-url-etcd-client-tls", ReadOnly: true},
					},
					ReadinessProbe: createProbe("/tmp/healthy"),
					LivenessProbe:  createProbe("/tmp/healthy"),
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	)
}

// createTLSVolume simplifies creation of TLS volumes for the job.
func createTLSVolume(name, secretName string) corev1.Volume {
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

// generateHealthCheckScript generates the shell script used to check Etcd health.
func generateHealthCheckScript(etcdSvc string) string {
	return fmt.Sprintf(`failed=0; threshold=2;
    while true; do
        if ! curl --cacert /var/etcd/ssl/ca/ca.crt --cert /var/etcd/ssl/client/tls.crt --key /var/etcd/ssl/client/tls.key https://%s:2379/health -s -f -o /dev/null; then
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
    done`, etcdSvc)
}

// createProbe creates a probe with specified file path for readiness and liveness checks.
func createProbe(filePath string) *corev1.Probe {
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

func getDebugPod(etcd *druidv1alpha1.Etcd) *corev1.Pod {
	volumeName := etcd.Name
	if etcd.Spec.VolumeClaimTemplate != nil && len(*etcd.Spec.VolumeClaimTemplate) != 0 {
		volumeName = *etcd.Spec.VolumeClaimTemplate
	}

	pvcName := volumeName + "-" + etcd.Name + "-0"

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      debugPodName + "-" + etcd.Name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  debugPodContainerName,
					Image: "nginx",
					VolumeMounts: []corev1.VolumeMount{
						{
							MountPath: "/var/etcd/ssl/ca",
							Name:      "client-url-ca-etcd",
						},
						{
							MountPath: "/var/etcd/ssl/server",
							Name:      "client-url-etcd-server-tls",
						},
						{
							MountPath: "/var/etcd/ssl/client",
							Name:      "client-url-etcd-client-tls",
							ReadOnly:  true,
						},
						{
							MountPath: "/var/etcd/data",
							Name:      volumeName,
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "client-url-ca-etcd",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.Name,
							DefaultMode: ptr.To(common.ModeOwnerReadWriteGroupRead),
						},
					},
				},
				{
					Name: "client-url-etcd-server-tls",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  etcd.Spec.Etcd.ClientUrlTLS.ServerTLSSecretRef.Name,
							DefaultMode: ptr.To(common.ModeOwnerReadWriteGroupRead),
						},
					},
				},
				{
					Name: "client-url-etcd-client-tls",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  etcd.Spec.Etcd.ClientUrlTLS.ClientTLSSecretRef.Name,
							DefaultMode: ptr.To(common.ModeOwnerReadWriteGroupRead),
						},
					},
				},
				{
					Name: volumeName,
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
}
