// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/go-logr/logr"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

const (
	caKeyFileName      = "ca.key"
	caCertFileName     = "ca.crt"
	serverKeyFileName  = "server.key"
	serverCertFileName = "server.crt"
	clientKeyFileName  = "client.key"
	clientCertFileName = "client.crt"

	defaultCertExpiryDays = 30
)

// generateCAKeyCert generates a CA key and certificate and saves them to the specified directory.
func generateCAKeyCert(logger logr.Logger, dir, name string) (err error) {
	logger.Info("generating CA key", "name", name, "dir", dir)
	caKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		err = fmt.Errorf("failed to generate CA key %s in dir %s: %v", name, dir, err)
		return
	}
	caKeyPath := filepath.Join(dir, caKeyFileName)
	caKeyFile, err := os.Create(caKeyPath)
	if err != nil {
		err = fmt.Errorf("failed to create CA key file %s: %v", caKeyPath, err)
		return
	}
	defer func() {
		if cErr := caKeyFile.Close(); cErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close CA key file %s: %v", caKeyPath, cErr))
		}
	}()
	if err = pem.Encode(caKeyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(caKey)}); err != nil {
		err = fmt.Errorf("failed to encode CA key %s: %v", caKeyPath, err)
		return
	}

	logger.Info("generating CA certificate", "name", name, "dir", dir)
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("%s-ca", name),
			Organization: []string{"Gardener"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, defaultCertExpiryDays),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}
	caCert, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		err = fmt.Errorf("failed to create CA certificate for %s in dir %s: %v", name, dir, err)
		return
	}
	caCertPath := filepath.Join(dir, caCertFileName)
	caCertFile, err := os.Create(caCertPath)
	if err != nil {
		err = fmt.Errorf("failed to create CA certificate file %s: %v", caCertPath, err)
		return
	}
	defer func() {
		if cErr := caCertFile.Close(); cErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close CA certificate file %s: %v", caCertPath, cErr))
		}
	}()
	if err = pem.Encode(caCertFile, &pem.Block{Type: "CERTIFICATE", Bytes: caCert}); err != nil {
		err = fmt.Errorf("failed to encode CA certificate %s: %v", caCertPath, err)
		return
	}

	return
}

// getCAKeyCert reads and returns the CA certificate and key from the specified directory.
func getCAKeyCert(dir string) (*x509.Certificate, *rsa.PrivateKey, error) {
	caCertPath := filepath.Join(dir, caCertFileName)
	caCertPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read CA certificate %s: %v", caCertPath, err)
	}
	caCertBlock, _ := pem.Decode(caCertPEM)
	if caCertBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode CA certificate PEM from %s", caCertPath)
	}
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA certificate %s: %v", caCertPath, err)
	}
	caKeyPath := filepath.Join(dir, caKeyFileName)
	caKeyPEM, err := os.ReadFile(caKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read CA key %s: %v", caKeyPath, err)
	}
	caKeyBlock, _ := pem.Decode(caKeyPEM)
	if caKeyBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode CA key PEM from %s", caKeyPath)
	}
	caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA key %s: %v", caKeyPath, err)
	}

	return caCert, caKey, nil
}

// getDNSNames generates a list of DNS names for the given Etcd name and namespace.
func getDNSNames(name, namespace string) []string {
	return []string{
		fmt.Sprintf("%s-local", name),
		fmt.Sprintf("%s-client", name),
		fmt.Sprintf("%s-client.%s", name, namespace),
		fmt.Sprintf("%s-client.%s.svc", name, namespace),
		fmt.Sprintf("%s-client.%s.svc.cluster.local", name, namespace),
		fmt.Sprintf("*.%s-peer", name),
		fmt.Sprintf("*.%s-peer.%s", name, namespace),
		fmt.Sprintf("*.%s-peer.%s.svc", name, namespace),
		fmt.Sprintf("*.%s-peer.%s.svc.cluster.local", name, namespace),
	}
}

// certificateType represents the type of TLS certificate to be generated.
type certificateType string

const (
	// CertTypeServer represents a server TLS certificate.
	CertTypeServer certificateType = "server"
	// CertTypeClient represents a client TLS certificate.
	CertTypeClient certificateType = "client"
)

// generateTLSKeyCert generates TLS key and certificate for either server or client, specified by certType parameter.
func generateTLSKeyCert(logger logr.Logger, certType certificateType, dir, name, namespace string) (err error) {
	logger.Info("generating TLS key", "type", certType, "name", name, "dir", dir)

	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		err = fmt.Errorf("failed to generate %s key %s in dir %s: %v", certType, name, dir, err)
		return
	}

	var keyPath string
	switch certType {
	case CertTypeServer:
		keyPath = filepath.Join(dir, serverKeyFileName)
	case CertTypeClient:
		keyPath = filepath.Join(dir, clientKeyFileName)
	}
	keyFile, err := os.Create(keyPath)
	if err != nil {
		err = fmt.Errorf("failed to create %s key file %s: %v", certType, keyPath, err)
		return
	}
	defer func() {
		if cErr := keyFile.Close(); cErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close %s key file %s: %v", certType, keyPath, cErr))
		}
	}()

	if err = pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}); err != nil {
		err = fmt.Errorf("failed to encode %s key %s: %v", certType, keyPath, err)
		return
	}

	logger.Info("generating certificate", "certType", certType, "name", name, "dir", dir)

	certTemplate := &x509.Certificate{
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("%s-%s", name, certType),
			Organization: []string{"Gardener"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, defaultCertExpiryDays),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	switch certType {
	case CertTypeServer:
		certTemplate.SerialNumber = big.NewInt(2)
		certTemplate.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
		certTemplate.DNSNames = getDNSNames(name, namespace)
	case CertTypeClient:
		certTemplate.SerialNumber = big.NewInt(3)
		certTemplate.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}

	caCert, caKey, err := getCAKeyCert(dir)
	if err != nil {
		err = fmt.Errorf("failed to get CA key and cert: %v", err)
		return
	}

	cert, err := x509.CreateCertificate(rand.Reader, certTemplate, caCert, &key.PublicKey, caKey)
	if err != nil {
		err = fmt.Errorf("failed to create %s certificate for %s: %v", certType, name, err)
		return
	}

	var certPath string
	switch certType {
	case CertTypeServer:
		certPath = filepath.Join(dir, serverCertFileName)
	case CertTypeClient:
		certPath = filepath.Join(dir, clientCertFileName)
	}
	certFile, err := os.Create(certPath)
	if err != nil {
		err = fmt.Errorf("failed to create %s certificate file %s: %v", certType, certPath, err)
	}
	defer func() {
		if cErr := certFile.Close(); cErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close %s certificate file %s: %v", certType, certPath, cErr))
		}
	}()

	if err = pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: cert}); err != nil {
		err = fmt.Errorf("failed to encode %s certificate %s: %v", certType, certPath, err)
		return
	}

	return
}

// GeneratePKIResources generates CA, server, and client TLS key and certificate files in the specified directory, for an Etcd cluster.
// Generated files are:
// - ca.key: CA private key
// - ca.crt: CA certificate
// - server.key: Server private key
// - server.crt: Server certificate
// - client.key: Client private key
// - client.crt: Client certificate
func GeneratePKIResources(logger logr.Logger, tlsDir, name, namespace string) error {
	if err := generateCAKeyCert(logger, tlsDir, name); err != nil {
		return err
	}

	if err := generateTLSKeyCert(logger, CertTypeServer, tlsDir, name, namespace); err != nil {
		return err
	}

	if err := generateTLSKeyCert(logger, CertTypeClient, tlsDir, name, namespace); err != nil {
		return err
	}

	return nil
}
