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

// GenerateCAKeyCert generates a CA key and certificate and saves them to the specified directory.
func GenerateCAKeyCert(logger logr.Logger, outputDir, name string) (err error) {
	logger.Info("generating CA key", "name", name, "outputDir", outputDir)
	caKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		err = fmt.Errorf("failed to generate CA key %s in outputDir %s: %v", name, outputDir, err)
		return
	}
	caKeyPath := filepath.Join(outputDir, caKeyFileName)
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

	logger.Info("generating CA certificate", "name", name, "outputDir", outputDir)
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
		err = fmt.Errorf("failed to create CA certificate for %s in outputDir %s: %v", name, outputDir, err)
		return
	}
	caCertPath := filepath.Join(outputDir, caCertFileName)
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

// getCACert reads and returns the CA certificate from the specified path.
func getCACert(path string) (*x509.Certificate, error) {
	caCertPEM, err := os.ReadFile(path)
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
	caKeyPEM, err := os.ReadFile(path)
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
	certPath := filepath.Join(dir, caCertFileName)
	keyPath := filepath.Join(dir, caKeyFileName)
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

// GenerateTLSKeyCert generates TLS key and certificate for either server or client, specified by certType parameter
// using the given CA directory, and writes them to the specified output directory.
func GenerateTLSKeyCert(logger logr.Logger, certType certificateType, caDir, outputDir, name, namespace string) (err error) {
	logger.Info("generating TLS key", "type", certType, "name", name, "caDir", caDir, "outputDir", outputDir)

	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		err = fmt.Errorf("failed to generate %s key %s in outputDir %s: %v", certType, name, outputDir, err)
		return
	}

	var keyPath string
	switch certType {
	case CertTypeServer:
		keyPath = filepath.Join(outputDir, serverKeyFileName)
	case CertTypeClient:
		keyPath = filepath.Join(outputDir, clientKeyFileName)
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
		certPath = filepath.Join(outputDir, serverCertFileName)
	case CertTypeClient:
		certPath = filepath.Join(outputDir, clientCertFileName)
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
	if err := GenerateCAKeyCert(logger, tlsDir, name); err != nil {
		return err
	}

	if err := GenerateTLSKeyCert(logger, CertTypeServer, tlsDir, tlsDir, name, namespace); err != nil {
		return err
	}

	if err := GenerateTLSKeyCert(logger, CertTypeClient, tlsDir, tlsDir, name, namespace); err != nil {
		return err
	}

	return nil
}
