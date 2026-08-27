// SPDX-FileCopyrightText: Contributors to the Gardener project
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
	defaultCACertDataKey  = "ca.crt"
	defaultTLSCertDataKey = "tls.crt"
	defaultTLSKeyDataKey  = "tls.key"
)

// TLS credential resolution sentinel errors. Callers may use errors.Is to
// distinguish resolution failures and apply their own requeue semantics.
var (
	// ErrGetSecret indicates the CA/keypair Secret could not be fetched.
	ErrGetSecret = errors.New("failed to get secret")
	// ErrSecretNotFound wraps ErrGetSecret for a NotFound response.
	ErrSecretNotFound = fmt.Errorf("%w: not found", ErrGetSecret)
	// ErrMissingDataKey indicates the requested data key is absent from the Secret.
	ErrMissingDataKey = errors.New("data key not found in secret")
	// ErrAppendCACerts indicates the CA bundle could not be parsed into a cert pool.
	ErrAppendCACerts = errors.New("failed to append CA certs from secret")
)

// LoadCAPoolFromVolumeSecret resolves the Secret mounted under volumeName on sts,
// fetches it, and builds an x509.CertPool from the bytes stored under caDataKey.
// Returns (nil, nil) when sts is nil or the volume is absent.
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

// LoadClientKeyPairFromVolumeSecret resolves the Secret mounted under volumeName on sts,
// fetches it, and builds a tls.Certificate from certDataKey and keyDataKey.
// Returns (tls.Certificate{}, false, nil) when sts is nil or the volume is absent.
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

// BuildEtcdClientTLSConfig builds an mTLS *tls.Config for the etcd client service.
// The CA is resolved from the Secret under the "etcd-ca" volume (caDataKey defaults
// to "ca.crt"). A client keypair is attached when the "etcd-client-tls" volume is
// present. Returns (nil, nil) when the CA volume is absent.
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

	// Attach a client certificate-key pair for mTLS when the spec references one.
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

// BuildBackupRestoreCATLSConfig builds a CA-only *tls.Config for the
// backup-restore HTTP server. The CA is resolved from the Secret mounted under
// the "backup-restore-ca" volume. Returns (nil, nil) when the volume is absent.
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
