// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// defaultCACertDataKey is the default data key under which a CA bundle is stored
	// in an etcd client CA secret.
	defaultCACertDataKey = "ca.crt"
	// defaultTLSCertDataKey is the default data key for a client certificate.
	defaultTLSCertDataKey = "tls.crt"
	// defaultTLSKeyDataKey is the default data key for a client private key.
	defaultTLSKeyDataKey = "tls.key"
)

// TLS credential resolution sentinel errors. Callers may classify a wrapped error
// with errors.Is to preserve their own error taxonomy (e.g. distinct DruidError
// codes and requeue semantics) while reusing the shared resolution logic.
var (
	// ErrGetSecret indicates the CA/keypair Secret could not be fetched.
	ErrGetSecret = errors.New("failed to get secret")
	// ErrSecretNotFound indicates the CA/keypair Secret was not found (a NotFound
	// Get error). It also satisfies errors.Is(err, ErrGetSecret).
	ErrSecretNotFound = fmt.Errorf("%w: not found", ErrGetSecret)
	// ErrMissingDataKey indicates the requested data key is absent from the Secret.
	ErrMissingDataKey = errors.New("data key not found in secret")
	// ErrAppendCACerts indicates the CA bundle could not be parsed/appended to a pool.
	ErrAppendCACerts = errors.New("failed to append CA certs from secret")
)

// loadCAPoolFromVolumeSecret resolves the Secret name mounted under volumeName on sts,
// fetches that Secret, and builds an x509.CertPool from the bytes stored under caDataKey.
// It returns (nil, nil) when sts is nil or the volume is absent, signalling that the
// corresponding TLS material is not configured on the running pods.
func loadCAPoolFromVolumeSecret(ctx context.Context, cl client.Client, sts *appsv1.StatefulSet, namespace, volumeName, caDataKey string) (*x509.CertPool, error) {
	secretName, ok := GetSecretNameFromVolume(sts, volumeName)
	if !ok {
		return nil, nil
	}
	caData, err := readSecretDataKey(ctx, cl, namespace, secretName, caDataKey)
	if err != nil {
		return nil, err
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("%w %s/%s", ErrAppendCACerts, namespace, secretName)
	}
	return caPool, nil
}

// BuildBackupRestoreCATLSConfig builds a CA-only *tls.Config for connecting to the
// backup-restore HTTP server. The CA bundle is resolved from the Secret mounted under the
// "backup-restore-ca" volume, using caDataKey (callers pass the Etcd spec DataKey override
// or "bundle.crt"). Resolving the Secret name from the live StatefulSet volume keeps the
// trust anchor consistent with what the running pods serve during credential rotation.
//
// It returns (nil, nil) when the backup-restore CA volume is absent.
func BuildBackupRestoreCATLSConfig(ctx context.Context, cl client.Client, sts *appsv1.StatefulSet, namespace, caDataKey string) (*tls.Config, error) {
	caPool, err := loadCAPoolFromVolumeSecret(ctx, cl, sts, namespace, common.VolumeNameBackupRestoreCA, caDataKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load backup-restore CA: %w", err)
	}
	if caPool == nil {
		return nil, nil
	}
	return &tls.Config{
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS12,
	}, nil
}

// GetEtcdClientSchemeAndTLSConfig returns the URL scheme and TLS config for the
// etcd client endpoint. When spec.etcd.clientUrlTLS is nil it returns
// ("http", nil, nil). When TLS is configured the CA is read from the Secret
// referenced by spec.etcd.clientUrlTLS.tlsCASecretRef and, when
// clientTLSSecretRef is set, a client certificate-key pair is attached for mTLS.
// Resolving credentials directly from the Etcd CR avoids a dependency on the
// StatefulSet and works identically before and after the STS is created.
func GetEtcdClientSchemeAndTLSConfig(ctx context.Context, cl client.Client, etcd *druidv1alpha1.Etcd) (scheme string, tlsConfig *tls.Config, err error) {
	if etcd.Spec.Etcd.ClientUrlTLS == nil {
		return "http", nil, nil
	}

	caSecretRef := etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef
	caDataKey := ptr.Deref(caSecretRef.DataKey, defaultCACertDataKey)
	caData, err := readSecretDataKey(ctx, cl, etcd.Namespace, caSecretRef.Name, caDataKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to load etcd client CA for %s/%s from secret %s: %w",
			etcd.Namespace, etcd.Name, caSecretRef.Name, err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caData) {
		return "", nil, fmt.Errorf("etcd client CA secret %s/%s (key %q) contains no valid PEM data",
			etcd.Namespace, caSecretRef.Name, caDataKey)
	}

	cfg := &tls.Config{
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS12,
	}

	clientRef := etcd.Spec.Etcd.ClientUrlTLS.ClientTLSSecretRef
	if clientRef.Name != "" {
		certData, err := readSecretDataKey(ctx, cl, etcd.Namespace, clientRef.Name, defaultTLSCertDataKey)
		if err != nil {
			return "", nil, fmt.Errorf("failed to load etcd client cert for %s/%s from secret %s: %w",
				etcd.Namespace, etcd.Name, clientRef.Name, err)
		}
		keyData, err := readSecretDataKey(ctx, cl, etcd.Namespace, clientRef.Name, defaultTLSKeyDataKey)
		if err != nil {
			return "", nil, fmt.Errorf("failed to load etcd client key for %s/%s from secret %s: %w",
				etcd.Namespace, etcd.Name, clientRef.Name, err)
		}
		cert, err := tls.X509KeyPair(certData, keyData)
		if err != nil {
			return "", nil, fmt.Errorf("failed to build client key pair from secret %s/%s: %w",
				etcd.Namespace, clientRef.Name, err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}

	return "https", cfg, nil
}

// readSecretDataKey fetches a Secret and returns the bytes stored under dataKey.
func readSecretDataKey(ctx context.Context, cl client.Client, namespace, name, dataKey string) ([]byte, error) {
	secret := &corev1.Secret{}
	if err := cl.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s/%s: %w", ErrSecretNotFound, namespace, name, err)
		}
		return nil, fmt.Errorf("%w %s/%s: %w", ErrGetSecret, namespace, name, err)
	}
	data, ok := secret.Data[dataKey]
	if !ok {
		return nil, fmt.Errorf("%w: data key %q not found in secret %s/%s", ErrMissingDataKey, dataKey, namespace, name)
	}
	return data, nil
}
