// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"
	"time"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/e2e/testenv"
	e2eutils "github.com/gardener/etcd-druid/test/e2e/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	timeoutEtcdDisruptionStart = 30 * time.Second
	timeoutEtcdRecovery        = 5 * time.Minute
	timeoutDeployJob           = 2 * time.Minute
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
