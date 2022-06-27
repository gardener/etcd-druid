// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/gardener/etcd-backup-restore/pkg/snapstore"
	brtypes "github.com/gardener/etcd-backup-restore/pkg/types"
	"github.com/gardener/etcd-druid/api/v1alpha1"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	envProviders = "INFRA_PROVIDERS"

	envS3AccessKeyID     = "AWS_ACCESS_KEY_ID"
	envS3SecretAccessKey = "AWS_SECRET_ACCESS_KEY"
	envS3Region          = "AWS_REGION"

	envABSStorageAccount = "STORAGE_ACCOUNT"
	envABSStorageKey     = "STORAGE_KEY"

	envGCSServiceAccount = "GCP_SERVICEACCOUNT_JSON_PATH"

	etcdBackupSecretPrefix = "etcd-backup"
	etcdPrefix             = "etcd"

	etcdCommandMaxRetries = 3
)

type Provider struct {
	Provider        v1alpha1.StorageProvider
	Suffix          string
	StorageProvider string
	SecretData      map[string][]byte
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
		"app":                     "etcd-statefulset",
		"garden.sapcloud.io/role": "controlplane",
		roleLabelKey:              defaultRoleLabelValue,
	}
	annotations = map[string]string{
		v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile,
	}

	stsLabels = map[string]string{
		"app":                              "etcd-statefulset",
		"garden.sapcloud.io/role":          "controlplane",
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
		Limits: corev1.ResourceList{
			"cpu":    resource.MustParse("1000m"),
			"memory": resource.MustParse("2Gi"),
		},
		Requests: corev1.ResourceList{
			"cpu":    resource.MustParse("500m"),
			"memory": resource.MustParse("1Gi"),
		},
	}
	etcdClientPort = int32(2379)
	etcdServerPort = int32(2380)
	defaultEtcdTLS = v1alpha1.TLSConfig{
		ServerTLSSecretRef: corev1.SecretReference{
			Name:      "etcd-server-cert",
			Namespace: namespace,
		},
		ClientTLSSecretRef: corev1.SecretReference{
			Name:      "etcd-client-tls",
			Namespace: namespace,
		},
		TLSCASecretRef: v1alpha1.SecretReference{
			SecretReference: corev1.SecretReference{
				Name:      "ca-etcd",
				Namespace: namespace,
			},
		},
	}

	backupPort                 = int32(8080)
	backupFullSnapshotSchedule = "0 */1 * * *"
	backupResources            = corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			"cpu":    resource.MustParse("500m"),
			"memory": resource.MustParse("1Gi"),
		},
		Requests: corev1.ResourceList{
			"cpu":    resource.MustParse("100m"),
			"memory": resource.MustParse("256Mi"),
		},
	}
	backupGarbageCollectionPolicy  = v1alpha1.GarbageCollectionPolicy(v1alpha1.GarbageCollectionPolicyExponential)
	backupGarbageCollectionPeriod  = metav1.Duration{Duration: 5 * time.Minute}
	backupDeltaSnapshotPeriod      = metav1.Duration{Duration: 10 * time.Second}
	backupDeltaSnapshotMemoryLimit = resource.MustParse("100Mi")
	gzipCompression                = v1alpha1.GzipCompression
	backupCompression              = v1alpha1.CompressionSpec{
		Enabled: pointer.BoolPtr(true),
		Policy:  &gzipCompression,
	}
	defaultBackupStore = v1alpha1.StoreSpec{
		SecretRef: &corev1.SecretReference{
			Name:      etcdBackupSecretPrefix,
			Namespace: namespace,
		},
	}

	autoCompactionMode      = v1alpha1.Periodic
	autoCompactionRetention = "2m"
	sharedConfig            = v1alpha1.SharedConfig{
		AutoCompactionMode:      &autoCompactionMode,
		AutoCompactionRetention: &autoCompactionRetention,
	}

	replicas        = 1
	storageCapacity = resource.MustParse("10Gi")
)

func getEmptyEtcd(name, namespace string) *v1alpha1.Etcd {
	return &v1alpha1.Etcd{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
}

func getDefaultEtcd(name, namespace, container, prefix string, provider Provider) *v1alpha1.Etcd {
	etcd := getEmptyEtcd(name, namespace)

	etcd.Annotations = annotations
	etcd.Spec.Annotations = stsAnnotations

	labelsCopy := make(map[string]string)
	for k, v := range labels {
		labelsCopy[k] = v
	}
	labelsCopy[roleLabelKey] = provider.Suffix
	etcd.Labels = labelsCopy
	etcd.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: labelsCopy,
	}

	stsLabelsCopy := make(map[string]string)
	for k, v := range stsLabels {
		stsLabelsCopy[k] = v
	}
	stsLabelsCopy[roleLabelKey] = provider.Suffix
	etcd.Spec.Labels = stsLabelsCopy

	etcdTLS := defaultEtcdTLS.DeepCopy()

	etcd.Spec.Etcd = v1alpha1.EtcdConfig{
		Metrics:                 (*v1alpha1.MetricsLevel)(&etcdMetrics),
		DefragmentationSchedule: &etcdDefragmentationSchedule,
		Quota:                   &etcdQuota,
		Resources:               &etcdResources,
		ClientPort:              &etcdClientPort,
		ServerPort:              &etcdServerPort,
		ClientUrlTLS:            etcdTLS,
	}

	backupStore := defaultBackupStore.DeepCopy()
	backupStore.Container = &container
	backupStore.Provider = &provider.Provider
	backupStore.Prefix = prefix

	etcd.Spec.Backup = v1alpha1.BackupSpec{
		Port:                     &backupPort,
		FullSnapshotSchedule:     &backupFullSnapshotSchedule,
		Resources:                &backupResources,
		GarbageCollectionPolicy:  &backupGarbageCollectionPolicy,
		GarbageCollectionPeriod:  &backupGarbageCollectionPeriod,
		DeltaSnapshotPeriod:      &backupDeltaSnapshotPeriod,
		DeltaSnapshotMemoryLimit: &backupDeltaSnapshotMemoryLimit,
		SnapshotCompression:      &backupCompression,
		Store:                    backupStore,
	}

	etcd.Spec.Common = sharedConfig

	etcd.Spec.Replicas = int32(replicas)
	etcd.Spec.StorageCapacity = &storageCapacity

	return etcd
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
)

func getProviders() (map[string]Provider, error) {
	providerNames := strings.Split(getEnvOrFallback(envProviders, ""), " ")

	providers := make(map[string]Provider)

	for _, p := range providerNames {
		var provider Provider
		switch p {
		case providerAWS:
			s3AccessKeyID := getEnvOrFallback(envS3AccessKeyID, "")
			s3SecretAccessKey := getEnvOrFallback(envS3SecretAccessKey, "")
			s3Region := getEnvOrFallback(envS3Region, "")
			if s3AccessKeyID != "" && s3SecretAccessKey != "" && s3Region != "" {
				provider = Provider{
					Provider:        "aws",
					Suffix:          "aws",
					StorageProvider: "S3",
					SecretData: map[string][]byte{
						"accessKeyID":     []byte(s3AccessKeyID),
						"secretAccessKey": []byte(s3SecretAccessKey),
						"region":          []byte(s3Region),
					},
				}
			}
		case providerAzure:
			absStorageAccount := getEnvOrFallback(envABSStorageAccount, "")
			absStorageKey := getEnvOrFallback(envABSStorageKey, "")
			if absStorageAccount != "" && absStorageKey != "" {
				provider = Provider{
					Provider:        "azure",
					Suffix:          "az",
					StorageProvider: "ABS",
					SecretData: map[string][]byte{
						"storageAccount": []byte(absStorageAccount),
						"storageKey":     []byte(absStorageKey),
					},
				}
			}
		case providerGCP:
			gcsServiceAccountPath := getEnvOrFallback(envGCSServiceAccount, "")
			if gcsServiceAccountPath != "" {
				gcsSA, err := os.ReadFile(gcsServiceAccountPath)
				if err != nil {
					return nil, err
				}

				provider = Provider{
					Provider:        "gcp",
					Suffix:          "gcp",
					StorageProvider: "GCS",
					SecretData: map[string][]byte{
						"serviceaccount.json": gcsSA,
					},
				}
			}
		case providerLocal:
			provider = Provider{
				Provider:        providerLocal,
				Suffix:          "events",
				StorageProvider: "Local",
			}
		}
		providers[p] = provider
	}

	return providers, nil
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

func deploySecret(logger logr.Logger, name, namespace string, labels map[string]string, secretType corev1.SecretType, secretData map[string][]byte, secretsClient typedcorev1.SecretInterface) (*corev1.Secret, error) {
	secret := corev1.Secret{}
	secret.Name = name
	secret.Namespace = namespace
	secret.Labels = labels
	secret.Type = secretType
	secret.Data = secretData

	logger.Info("creating secret", "secret", client.ObjectKeyFromObject(&secret))
	return secretsClient.Create(context.TODO(), &secret, metav1.CreateOptions{})
}

func buildAndDeployTLSSecrets(logger logr.Logger, certsPath string, providers map[string]Provider, secretsClient typedcorev1.SecretInterface) ([]*corev1.Secret, error) {
	var (
		caCertFile          = "ca.crt"
		caKeyFile           = "ca.key"
		caSecretName        = "ca-etcd"
		tlsServerCertFile   = "server.crt"
		tlsServerKeyFile    = "server.key"
		tlsServerSecretName = "etcd-server-cert"
		tlsClientCertFile   = "client.crt"
		tlsClientKeyFile    = "client.key"
		tlsClientSecretName = "etcd-client-tls"
		secretData          map[string][]byte
	)
	secrets := make([]*corev1.Secret, 3*len(providers))

	for _, provider := range providers {
		caCert, err := ioutil.ReadFile(path.Join(certsPath, caCertFile))
		if err != nil {
			return nil, err
		}
		caKey, err := ioutil.ReadFile(path.Join(certsPath, caKeyFile))
		if err != nil {
			return nil, err
		}
		secretData = map[string][]byte{
			"ca.crt": caCert,
			"ca.key": caKey,
		}
		secretName := fmt.Sprintf("%s-%s", caSecretName, provider.Suffix)
		caSecret, err := deploySecret(logger, secretName, namespace, labels, corev1.SecretTypeOpaque, secretData, secretsClient)
		if err != nil {
			return nil, err
		}
		secrets = append(secrets, caSecret)

		tlsServerCert, err := ioutil.ReadFile(path.Join(certsPath, tlsServerCertFile))
		if err != nil {
			return nil, err
		}
		tlsServerKey, err := ioutil.ReadFile(path.Join(certsPath, tlsServerKeyFile))
		if err != nil {
			return nil, err
		}
		secretData = map[string][]byte{
			"ca.crt":  caCert,
			"tls.crt": tlsServerCert,
			"tls.key": tlsServerKey,
		}
		secretName = fmt.Sprintf("%s-%s", tlsServerSecretName, provider.Suffix)
		tlsServerSecret, err := deploySecret(logger, secretName, namespace, labels, corev1.SecretTypeTLS, secretData, secretsClient)
		if err != nil {
			return nil, err
		}
		secrets = append(secrets, tlsServerSecret)

		tlsClientCert, err := ioutil.ReadFile(path.Join(certsPath, tlsClientCertFile))
		if err != nil {
			return nil, err
		}
		tlsClientKey, err := ioutil.ReadFile(path.Join(certsPath, tlsClientKeyFile))
		if err != nil {
			return nil, err
		}
		secretData = map[string][]byte{
			"ca.crt":  caCert,
			"tls.crt": tlsClientCert,
			"tls.key": tlsClientKey,
		}
		secretName = fmt.Sprintf("%s-%s", tlsClientSecretName, provider.Suffix)
		tlsClientSecret, err := deploySecret(logger, secretName, namespace, labels, corev1.SecretTypeTLS, secretData, secretsClient)
		if err != nil {
			return nil, err
		}
		secrets = append(secrets, tlsClientSecret)
	}

	return secrets, nil
}

func deployBackupSecrets(logger logr.Logger, secretsClient typedcorev1.SecretInterface, providers map[string]Provider, namespace, storageContainer string) ([]*corev1.Secret, error) {
	secrets := make([]*corev1.Secret, 0)
	for _, provider := range providers {
		if provider.SecretData == nil {
			continue
		}
		secretData := provider.SecretData
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
		secret, err := secretsClient.Create(context.TODO(), &etcdBackupSecret, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}
		secrets = append(secrets, secret)
	}

	return secrets, nil
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
func executeRemoteCommand(kubeconfigPath, namespace, podName, containerName, command string) (string, string, error) {
	exec, err := getRemoteCommandExecutor(kubeconfigPath, namespace, podName, containerName, command)
	if err != nil {
		return "", "", err
	}

	buf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: buf,
		Stderr: errBuf,
	})
	if err != nil {
		return "", "", err
	}

	return strings.TrimSpace(buf.String()), strings.TrimSpace(errBuf.String()), nil
}

func getSnapstore(storageProvider, storageContainer, storePrefix string) (brtypes.SnapStore, error) {
	snapstoreConfig := &brtypes.SnapstoreConfig{
		Provider:  storageProvider,
		Container: storageContainer,
		Prefix:    path.Join(storePrefix, "v1"),
	}
	store, err := snapstore.GetSnapstore(snapstoreConfig)
	if err != nil {
		return nil, err
	}

	return store, nil
}

func purgeSnapstore(store brtypes.SnapStore) error {
	snapList, err := store.List()
	if err != nil {
		return err
	}

	for _, snap := range snapList {
		err = store.Delete(*snap)
		if err != nil {
			return err
		}
	}

	return nil
}

func populateEtcdWithCount(logger logr.Logger, kubeconfigPath, namespace, etcdName, podName, containerName, keyPrefix, valuePrefix string, start, end int, delay time.Duration) error {
	var (
		cmd     string
		stdout  string
		stderr  string
		retries = 0
		err     error
	)

	for i := start; i <= end; {
		cmd = fmt.Sprintf("ETCDCTL_API=3 etcdctl --endpoints=https://%s-local:%d --cacert /var/etcd/ssl/ca/ca.crt --cert=/var/etcd/ssl/client/tls.crt --key=/var/etcd/ssl/client/tls.key put %s-%d %s-%d", etcdName, etcdClientPort, keyPrefix, i, valuePrefix, i)
		stdout, stderr, err = executeRemoteCommand(kubeconfigPath, namespace, podName, containerName, cmd)
		if err != nil || stderr != "" || stdout != "OK" {
			logger.Error(err, fmt.Sprintf("failed to put (%s-%d, %s-%d): stdout: %s; stderr: %s. Retrying", keyPrefix, i, valuePrefix, i, stdout, stderr))
			retries++
			if retries >= etcdCommandMaxRetries {
				return fmt.Errorf("failed to put (%s-%d, %s-%d): stdout: %s; stderr: %s; err: %v", keyPrefix, i, valuePrefix, i, stdout, stderr, err)
			}
			continue
		}
		logger.Info(fmt.Sprintf("put (%s-%d, %s-%d) successful", keyPrefix, i, valuePrefix, i))
		if i%10 == 0 {
			logger.Info(fmt.Sprintf("deleting key %s-%d", keyPrefix, i))
			cmd = fmt.Sprintf("ETCDCTL_API=3 etcdctl --endpoints=https://%s-local:%d --cacert /var/etcd/ssl/ca/ca.crt --cert=/var/etcd/ssl/client/tls.crt --key=/var/etcd/ssl/client/tls.key del %s-%d", etcdName, etcdClientPort, keyPrefix, i)
			stdout, stderr, err = executeRemoteCommand(kubeconfigPath, namespace, podName, containerName, cmd)
			if err != nil || stderr != "" || stdout != "1" {
				logger.Error(err, fmt.Sprintf("failed to delete key %s-%d: stdout: %s; stderr: %s. Retrying", keyPrefix, i, stdout, stderr))
				retries++
				if retries >= etcdCommandMaxRetries {
					return fmt.Errorf("failed to delete key %s-%d: stdout: %s; stderr: %s; err: %v", keyPrefix, i, stdout, stderr, err)
				}
				continue
			}
		}
		retries = 0
		i++
		time.Sleep(delay)
	}

	return nil
}

func getEtcdKey(kubeconfigPath, namespace, etcdName, podName, containerName, keyPrefix string, suffix int) (string, string, error) {
	var (
		cmd    string
		stdout string
		stderr string
		err    error
	)

	cmd = fmt.Sprintf("ETCDCTL_API=3 etcdctl --endpoints=https://%s-local:%d --cacert /var/etcd/ssl/ca/ca.crt --cert=/var/etcd/ssl/client/tls.crt --key=/var/etcd/ssl/client/tls.key get %s-%d", etcdName, etcdClientPort, keyPrefix, suffix)
	stdout, stderr, err = executeRemoteCommand(kubeconfigPath, namespace, podName, containerName, cmd)
	if err != nil || stderr != "" {
		return "", "", fmt.Errorf("failed to get %s-%d: stdout: %s; stderr: %s; err: %v", keyPrefix, suffix, stdout, stderr, err)
	}
	splits := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(splits) != 2 {
		return "", "", fmt.Errorf("error splitting stdout into 2 parts: stdout: %s", stdout)
	}

	return strings.TrimSpace(splits[0]), strings.TrimSpace(splits[1]), nil
}

func getEtcdKeys(logger logr.Logger, kubeconfigPath, namespace, etcdName, podName, containerName, keyPrefix string, start, end, skipMultiple int) (map[string]string, error) {
	var (
		key         string
		val         string
		retries     = 0
		keyValueMap = make(map[string]string)
		err         error
	)
	for i := start; i <= end; {
		if i%skipMultiple == 0 {
			i++
			continue
		}
		key, val, err = getEtcdKey(kubeconfigPath, namespace, etcdName, podName, containerName, keyPrefix, i)
		if err != nil {
			logger.Info(fmt.Sprintf("failed to get key %s-%d. Retrying", keyPrefix, i))
			retries++
			if retries >= etcdCommandMaxRetries {
				return nil, fmt.Errorf("failed to get key %s-%d", keyPrefix, i)
			}
			continue
		}
		retries = 0
		logger.Info(fmt.Sprintf("fetched (%s, %s) from etcd", key, val))
		keyValueMap[key] = val
		i++
	}

	return keyValueMap, nil
}

func triggerOnDemandSnapshot(kubeconfigPath, namespace, podName, containerName string, port int, snapshotKind string) (*brtypes.Snapshot, error) {
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
	cmd := fmt.Sprintf("curl https://localhost:%d/snapshot/%s -k -s", port, snapKind)
	stdout, stderr, err := executeRemoteCommand(kubeconfigPath, namespace, podName, containerName, cmd)
	if err != nil || stdout == "" {
		return nil, fmt.Errorf("failed to trigger on-demand %s snapshot for %s: stdout: %s; stderr: %s; err: %v", snapKind, podName, stdout, stderr, err)
	}
	if err = json.Unmarshal([]byte(stdout), &snapshot); err != nil {
		return nil, err
	}

	return snapshot, nil
}

func getLatestSnapshots(kubeconfigPath, namespace, etcdName, podName, containerName string, port int) (*LatestSnapshots, error) {
	var latestSnapshots *LatestSnapshots
	cmd := fmt.Sprintf("curl https://%s-local:%d/snapshot/latest -k -s", etcdName, port)
	stdout, stderr, err := executeRemoteCommand(kubeconfigPath, namespace, podName, containerName, cmd)
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

func deleteDir(kubeconfigPath, namespace, podName, containerName string, dirPath string) error {
	cmd := fmt.Sprintf("rm -rf %s", dirPath)
	stdout, stderr, err := executeRemoteCommand(kubeconfigPath, namespace, podName, containerName, cmd)
	if err != nil || stdout != "" {
		return fmt.Errorf("failed to delete directory %s for %s: stdout: %s; stderr: %s; err: %v", dirPath, podName, stdout, stderr, err)
	}
	return nil
}
