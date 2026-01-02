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
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
)

const (
	CAKeyFileName      = "ca.key"
	CACertFileName     = "ca.crt"
	ServerKeyFileName  = "server.key"
	ServerCertFileName = "server.crt"
	ClientKeyFileName  = "client.key"
	ClientCertFileName = "client.crt"

	defaultCertExpiryDays = 30
)

// GenerateCAKeyCertToDirectory generates a CA key and certificate and saves them to the specified directory.
func GenerateCAKeyCertToDirectory(logger logr.Logger, outputDir, name string) (err error) {
	logger.Info("generating raw CA key and certificate", "name", name, "outputDir", outputDir)
	caKey, caCert, err := generateRawCAKeyCert(name)
	if err != nil {
		return fmt.Errorf("failed to generate raw CA key and cert for %s: %w", name, err)
	}

	caKeyPath := filepath.Join(outputDir, CAKeyFileName)
	logger.Info("writing CA key to file", "path", caKeyPath, "outputDir", outputDir)
	caKeyFile, err := os.Create(caKeyPath) // #nosec: G304 -- test files.
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

	caCertPath := filepath.Join(outputDir, CACertFileName)
	logger.Info("writing CA certificate to file", "path", caCertPath, "outputDir", outputDir)
	caCertFile, err := os.Create(caCertPath) // #nosec: G304 -- test files.
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

// GenerateCACert generates a PEM-encoded CA certificate.
func GenerateCACert(name string) ([]byte, error) {
	_, cert, err := generateRawCAKeyCert(name)
	if err != nil {
		return nil, fmt.Errorf("failed to generate raw CA cert for %s: %w", name, err)
	}
	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert,
	})
	return certPEM, nil
}

// generateRawCAKeyCert generates a raw CA certificate.
func generateRawCAKeyCert(name string) (*rsa.PrivateKey, []byte, error) {
	// Generate private key
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
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

	// Create self-signed certificate
	cert, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	return key, cert, nil
}

// getCACert reads and returns the CA certificate from the specified path.
func getCACert(path string) (*x509.Certificate, error) {
	caCertPEM, err := os.ReadFile(path) // #nosec: G304 -- test files.
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate %s: %v", path, err)
	}
	caCertBlock, _ := pem.Decode(caCertPEM)
	if caCertBlock == nil {
		return nil, fmt.Errorf("failed to decode CA certificate PEM from %s", path)
	}
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA certificate %s: %v", path, err)
	}
	return caCert, nil
}

// getCAKey reads and returns the CA private key from the specified path.
func getCAKey(path string) (*rsa.PrivateKey, error) {
	caKeyPEM, err := os.ReadFile(path) // #nosec: G304 -- test files.
	if err != nil {
		return nil, fmt.Errorf("failed to read CA key %s: %v", path, err)
	}
	caKeyBlock, _ := pem.Decode(caKeyPEM)
	if caKeyBlock == nil {
		return nil, fmt.Errorf("failed to decode CA key PEM from %s", path)
	}
	caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA key %s: %v", path, err)
	}
	return caKey, nil
}

// getCACertKey reads and returns the CA certificate and key from the specified paths.
func getCACertKey(certPath, keyPath string) (*x509.Certificate, *rsa.PrivateKey, error) {
	caCert, err := getCACert(certPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get CA certificate: %v", err)
	}
	caKey, err := getCAKey(keyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get CA key: %v", err)
	}
	return caCert, caKey, nil
}

// getCACertKeyFromDir reads and returns the CA certificate and key from the specified directory.
func getCACertKeyFromDir(dir string) (*x509.Certificate, *rsa.PrivateKey, error) {
	certPath := filepath.Join(dir, CACertFileName)
	keyPath := filepath.Join(dir, CAKeyFileName)
	return getCACertKey(certPath, keyPath)
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

// GenerateTLSKeyCertToDirectory generates TLS key and certificate for either server or client, specified by certType parameter
// using the given CA directory, and writes them to the specified output directory.
func GenerateTLSKeyCertToDirectory(logger logr.Logger, certType certificateType, caDir, outputDir, name, namespace string) (err error) {
	logger.Info("generating TLS key", "type", certType, "name", name, "caDir", caDir, "outputDir", outputDir)

	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		err = fmt.Errorf("failed to generate %s key %s in outputDir %s: %v", certType, name, outputDir, err)
		return
	}

	var keyPath string
	switch certType {
	case CertTypeServer:
		keyPath = filepath.Join(outputDir, ServerKeyFileName)
	case CertTypeClient:
		keyPath = filepath.Join(outputDir, ClientKeyFileName)
	}
	keyFile, err := os.Create(keyPath) // #nosec: G304 -- test files.
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

	logger.Info("generating certificate", "certType", certType, "name", name, "outputDir", outputDir)

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

	caCert, caKey, err := getCACertKeyFromDir(caDir)
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
		certPath = filepath.Join(outputDir, ServerCertFileName)
	case CertTypeClient:
		certPath = filepath.Join(outputDir, ClientCertFileName)
	}
	certFile, err := os.Create(certPath) // #nosec: G304 -- test files.
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

// GeneratePKIResourcesToDirectory generates CA, server, and client TLS key and certificate files in the specified directory, for an Etcd cluster.
// Generated files are:
// - ca.key: CA private key
// - ca.crt: CA certificate
// - server.key: Server private key
// - server.crt: Server certificate
// - client.key: Client private key
// - client.crt: Client certificate
func GeneratePKIResourcesToDirectory(logger logr.Logger, tlsDir, name, namespace string) error {
	if err := GenerateCAKeyCertToDirectory(logger, tlsDir, name); err != nil {
		return err
	}

	if err := GenerateTLSKeyCertToDirectory(logger, CertTypeServer, tlsDir, tlsDir, name, namespace); err != nil {
		return err
	}

	if err := GenerateTLSKeyCertToDirectory(logger, CertTypeClient, tlsDir, tlsDir, name, namespace); err != nil {
		return err
	}

	return nil
}
