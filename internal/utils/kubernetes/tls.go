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

	"github.com/gardener/etcd-druid/internal/common"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
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

// LoadCAPoolFromVolumeSecret resolves the Secret name mounted under volumeName on sts,
// fetches that Secret, and builds an x509.CertPool from the bytes stored under caDataKey.
// It returns (nil, nil) when sts is nil or the volume is absent, signalling that the
// corresponding TLS material is not configured on the running pods.
func LoadCAPoolFromVolumeSecret(ctx context.Context, cl client.Client, sts *appsv1.StatefulSet, namespace, volumeName, caDataKey string) (*x509.CertPool, error) {
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

// LoadClientKeyPairFromVolumeSecret resolves the Secret name mounted under volumeName on sts,
// fetches that Secret, and builds a tls.Certificate from the bytes stored under certDataKey
// and keyDataKey. It returns (tls.Certificate{}, false, nil) when sts is nil or the volume is
// absent, signalling that no client keypair is configured on the running pods.
func LoadClientKeyPairFromVolumeSecret(ctx context.Context, cl client.Client, sts *appsv1.StatefulSet, namespace, volumeName, certDataKey, keyDataKey string) (tls.Certificate, bool, error) {
	secretName, ok := GetSecretNameFromVolume(sts, volumeName)
	if !ok {
		return tls.Certificate{}, false, nil
	}
	certData, err := readSecretDataKey(ctx, cl, namespace, secretName, certDataKey)
	if err != nil {
		return tls.Certificate{}, false, err
	}
	keyData, err := readSecretDataKey(ctx, cl, namespace, secretName, keyDataKey)
	if err != nil {
		return tls.Certificate{}, false, err
	}
	cert, err := tls.X509KeyPair(certData, keyData)
	if err != nil {
		return tls.Certificate{}, false, fmt.Errorf("failed to build client key pair from secret %s/%s: %w", namespace, secretName, err)
	}
	return cert, true, nil
}

// BuildEtcdClientTLSConfig builds an mTLS *tls.Config for connecting to the etcd client
// service. The CA bundle is resolved from the Secret mounted under the "etcd-ca" volume
// (using caDataKey, defaulting to "ca.crt" when empty) and, when present, a client
// certificate-key pair is resolved from the Secret mounted under the "etcd-client-tls"
// volume. Resolving the Secret names from the live StatefulSet volumes (rather than from
// the Etcd spec refs) keeps the credentials consistent with what the running pods serve
// and trust, which is required for correctness during credential rotation.
//
// It returns (nil, nil) when the CA volume is absent (TLS not configured on the pods),
// leaving it to the caller to decide whether that is an error in their context.
func BuildEtcdClientTLSConfig(ctx context.Context, cl client.Client, sts *appsv1.StatefulSet, namespace, caDataKey string) (*tls.Config, error) {
	if caDataKey == "" {
		caDataKey = defaultCACertDataKey
	}
	caPool, err := LoadCAPoolFromVolumeSecret(ctx, cl, sts, namespace, common.VolumeNameEtcdCA, caDataKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load etcd client CA: %w", err)
	}
	if caPool == nil {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS12,
	}

	cert, ok, err := LoadClientKeyPairFromVolumeSecret(ctx, cl, sts, namespace, common.VolumeNameEtcdClientTLS, defaultTLSCertDataKey, defaultTLSKeyDataKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load etcd client key pair: %w", err)
	}
	if ok {
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// BuildBackupRestoreCATLSConfig builds a CA-only *tls.Config for connecting to the
// backup-restore HTTP server. The CA bundle is resolved from the Secret mounted under the
// "backup-restore-ca" volume, using caDataKey (callers pass the Etcd spec DataKey override
// or "bundle.crt"). Resolving the Secret name from the live StatefulSet volume keeps the
// trust anchor consistent with what the running pods serve during credential rotation.
//
// It returns (nil, nil) when the backup-restore CA volume is absent.
func BuildBackupRestoreCATLSConfig(ctx context.Context, cl client.Client, sts *appsv1.StatefulSet, namespace, caDataKey string) (*tls.Config, error) {
	caPool, err := LoadCAPoolFromVolumeSecret(ctx, cl, sts, namespace, common.VolumeNameBackupRestoreCA, caDataKey)
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
