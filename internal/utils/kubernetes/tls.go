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
)

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
