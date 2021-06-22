// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
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

package integration

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

	"github.com/gardener/etcd-backup-restore/pkg/snapstore"
	"github.com/gardener/etcd-druid/api/v1alpha1"

	"github.com/sirupsen/logrus"
	helmaction "helm.sh/helm/v3/pkg/action"
	helmchart "helm.sh/helm/v3/pkg/chart"
	helmloader "helm.sh/helm/v3/pkg/chart/loader"
	helmkube "helm.sh/helm/v3/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	apiextension "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	envS3AccessKeyID     = "ACCESS_KEY_ID"
	envS3SecretAccessKey = "SECRET_ACCESS_KEY"
	envS3Region          = "REGION"

	envABSStorageAccount = "STORAGE_ACCOUNT"
	envABSStorageKey     = "STORAGE_KEY"

	envGCSServiceAccount = "SERVICEACCOUNT_JSON"

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
	decode func(data []byte, defaults *schema.GroupVersionKind, into runtime.Object) (runtime.Object, *schema.GroupVersionKind, error)

	name = "etcd-main"
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
	annotations = map[string]string{}

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
		TLSCASecretRef: corev1.SecretReference{
			Name:      "ca-etcd",
			Namespace: namespace,
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
		Enabled: true,
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
	storageClass    = "default"
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
		TLS:                     etcdTLS,
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

	etcd.Spec.Replicas = replicas
	etcd.Spec.StorageClass = &storageClass
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
	Snapshots snapstore.SnapList `json:"snapshots"`
	Error     error              `json:"error"`
}

// LatestSnapshots stores the result from output of /snapshot/latest http call
type LatestSnapshots struct {
	FullSnapshot   *snapstore.Snapshot   `json:"fullSnapshot"`
	DeltaSnapshots []*snapstore.Snapshot `json:"deltaSnapshots"`
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

func getCloudProviders() map[string]Provider {
	providers := make(map[string]Provider)

	s3AccessKeyID := getEnvOrFallback(envS3AccessKeyID, "")
	s3SecretAccessKey := getEnvOrFallback(envS3SecretAccessKey, "")
	s3Region := getEnvOrFallback(envS3Region, "")
	if s3AccessKeyID != "" && s3SecretAccessKey != "" && s3Region != "" {
		providers["AWS"] = Provider{
			Provider:        v1alpha1.StorageProvider("aws"),
			Suffix:          "aws",
			StorageProvider: "S3",
			SecretData: map[string][]byte{
				"accessKeyID":     []byte(s3AccessKeyID),
				"secretAccessKey": []byte(s3SecretAccessKey),
				"region":          []byte(s3Region),
			},
		}
	}

	absStorageAccount := getEnvOrFallback(envABSStorageAccount, "")
	absStorageKey := getEnvOrFallback(envABSStorageKey, "")
	if absStorageAccount != "" && absStorageKey != "" {
		providers["Azure"] = Provider{
			Provider:        v1alpha1.StorageProvider("azure"),
			Suffix:          "az",
			StorageProvider: "ABS",
			SecretData: map[string][]byte{
				"storageAccount": []byte(absStorageAccount),
				"storageKey":     []byte(absStorageKey),
			},
		}
	}

	gcsServiceAccount := getEnvOrFallback(envGCSServiceAccount, "")
	if gcsServiceAccount != "" {
		providers["GCP"] = Provider{
			Provider:        v1alpha1.StorageProvider("gcp"),
			Suffix:          "gcp",
			StorageProvider: "GCS",
			SecretData: map[string][]byte{
				"serviceaccount.json": []byte(gcsServiceAccount),
			},
		}
	}

	return providers
}

func getProviders() map[string]Provider {
	providers := getCloudProviders()

	providers["Local"] = Provider{
		Provider:        v1alpha1.StorageProvider("Local"),
		Suffix:          "events",
		StorageProvider: "Local",
	}

	return providers
}

func getKubeconfig(kubeconfigPath string) (*rest.Config, error) {
	return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
}

func getKubernetesTypedClient(logger *logrus.Logger, kubeconfigPath string) (*kubernetes.Clientset, error) {
	config, err := getKubeconfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

func getKubernetesAPIExtensionClient(logger *logrus.Logger, kubeconfigPath string) (*apiextension.Clientset, error) {
	config, err := getKubeconfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	return apiextension.NewForConfig(config)
}

func getKubernetesClient(logger *logrus.Logger, kubeconfigPath string) (client.Client, error) {
	config, err := getKubeconfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	return client.New(config, client.Options{})
}

func deploySecret(logger *logrus.Logger, name, namespace string, labels map[string]string, secretType corev1.SecretType, secretData map[string][]byte, secretsClient typedcorev1.SecretInterface) (*corev1.Secret, error) {
	secret := corev1.Secret{}
	secret.Name = name
	secret.Namespace = namespace
	secret.Labels = labels
	secret.Type = secretType
	secret.Data = secretData

	logger.Infof("creating secret %s", secret.Name)
	return secretsClient.Create(context.TODO(), &secret, metav1.CreateOptions{})
}

func buildAndDeployTLSSecrets(logger *logrus.Logger, certsPath string, providers map[string]Provider, secretsClient typedcorev1.SecretInterface) ([]*corev1.Secret, error) {
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

func deployBackupSecrets(logger *logrus.Logger, secretsClient typedcorev1.SecretInterface, providers map[string]Provider, namespace, storageContainer string) ([]*corev1.Secret, error) {
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

		logger.Infof("creating secret %s", etcdBackupSecret.Name)
		secret, err := secretsClient.Create(context.TODO(), &etcdBackupSecret, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}
		logger.Infof("deployed secret %s", secret.Name)
		secrets = append(secrets, secret)
	}

	return secrets, nil
}

func helmDeployChart(logger *logrus.Logger, timeout time.Duration, actionType, kubeconfigPath, chartPath, releaseName, releaseNamespace string, chartValues map[string]interface{}, waitForResourcesToBeReady bool) error {
	var (
		chart     *helmchart.Chart
		err       error
		helmErrCh = make(chan error)
	)

	chart, err = helmloader.Load(chartPath)
	if err != nil {
		return err
	}

	actionConfig := new(helmaction.Configuration)
	if err = actionConfig.Init(helmkube.GetConfig(kubeconfigPath, "", releaseNamespace), releaseNamespace, "configmap", func(format string, v ...interface{}) {}); err != nil {
		return err
	}

	if actionType == "install" {
		installClient := helmaction.NewInstall(actionConfig)
		installClient.Namespace = releaseNamespace
		installClient.ReleaseName = releaseName
		installClient.Wait = waitForResourcesToBeReady

		go func(errCh chan<- error) {
			_, err := installClient.Run(chart, chartValues)
			errCh <- err
		}(helmErrCh)

	} else if actionType == "upgrade" {
		upgradeClient := helmaction.NewUpgrade(actionConfig)
		upgradeClient.Namespace = releaseNamespace
		upgradeClient.Wait = waitForResourcesToBeReady

		go func(errCh chan<- error) {
			_, err := upgradeClient.Run(releaseName, chart, chartValues)
			errCh <- err
		}(helmErrCh)

	} else {
		return fmt.Errorf("invalid actionType %s", actionType)
	}

	helmDeployTimer := time.NewTimer(timeout)
	for {
		select {
		case err := <-helmErrCh:
			if err != nil {
				logger.Errorf("helm chart installation to release '%s' failed", releaseName)
				return err
			}
			return nil
		case <-helmDeployTimer.C:
			logger.Errorf("helm chart installation to release '%s' failed", releaseName)
			return fmt.Errorf("operation timed out")
		}
	}
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

func getSnapstore(storageProvider, storageContainer, storePrefix string) (snapstore.SnapStore, error) {
	snapstoreConfig := &snapstore.Config{
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

func purgeSnapstore(store snapstore.SnapStore) error {
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

func populateEtcdWithCount(logger *logrus.Logger, kubeconfigPath, namespace, etcdName, podName, containerName, keyPrefix, valuePrefix string, start, end int, delay time.Duration) error {
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
			logger.Infof("failed to put (%s-%d, %s-%d): stdout: %s; stderr: %s; err: %v. Retrying", keyPrefix, i, valuePrefix, i, stdout, stderr, err)
			retries++
			if retries >= etcdCommandMaxRetries {
				return fmt.Errorf("failed to put (%s-%d, %s-%d): stdout: %s; stderr: %s; err: %v", keyPrefix, i, valuePrefix, i, stdout, stderr, err)
			}
			continue
		}
		logger.Infof("put (%s-%d, %s-%d) successful", keyPrefix, i, valuePrefix, i)
		if i%10 == 0 {
			logger.Infof("deleting key %s-%d", keyPrefix, i)
			cmd = fmt.Sprintf("ETCDCTL_API=3 etcdctl --endpoints=https://%s-local:%d --cacert /var/etcd/ssl/ca/ca.crt --cert=/var/etcd/ssl/client/tls.crt --key=/var/etcd/ssl/client/tls.key del %s-%d", etcdName, etcdClientPort, keyPrefix, i)
			stdout, stderr, err = executeRemoteCommand(kubeconfigPath, namespace, podName, containerName, cmd)
			if err != nil || stderr != "" || stdout != "1" {
				logger.Infof("failed to delete key %s-%d: stdout: %s; stderr: %s; err: %v. Retrying", keyPrefix, i, stdout, stderr, err)
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

func getEtcdKey(logger *logrus.Logger, kubeconfigPath, namespace, etcdName, podName, containerName, keyPrefix string, suffix int) (string, string, error) {
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

func getEtcdKeys(logger *logrus.Logger, kubeconfigPath, namespace, etcdName, podName, containerName, keyPrefix string, start, end, skipMultiple int) (map[string]string, error) {
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
		key, val, err = getEtcdKey(logger, kubeconfigPath, namespace, etcdName, podName, containerName, keyPrefix, i)
		if err != nil {
			logger.Infof("failed to get key %s-%d. Retrying", keyPrefix, i)
			retries++
			if retries >= etcdCommandMaxRetries {
				return nil, fmt.Errorf("failed to get key %s-%d", keyPrefix, i)
			}
			continue
		}
		retries = 0
		logger.Infof("fetched (%s, %s) from etcd", key, val)
		keyValueMap[key] = val
		i++
	}

	return keyValueMap, nil
}

func triggerOnDemandSnapshot(kubeconfigPath, namespace, etcdName, podName, containerName string, port int, snapshotKind string) (*snapstore.Snapshot, error) {
	var (
		snapshot *snapstore.Snapshot
		snapKind string
	)

	switch snapshotKind {
	case snapstore.SnapshotKindFull:
		snapKind = "full"
	case snapstore.SnapshotKindDelta:
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

func deleteDir(kubeconfigPath, namespace, podName, containerName string, port int, dirPath string) error {
	cmd := fmt.Sprintf("rm -rf %s", dirPath)
	stdout, stderr, err := executeRemoteCommand(kubeconfigPath, namespace, podName, containerName, cmd)
	if err != nil || stdout != "" {
		return fmt.Errorf("failed to delete directory %s for %s: stdout: %s; stderr: %s; err: %v", dirPath, podName, stdout, stderr, err)
	}
	return nil
}
