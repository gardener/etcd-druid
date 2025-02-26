#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


set -o errexit
set -o nounset
set -o pipefail


TLS_ARTIFACT_REQUESTS_DIR=""
TLS_DIR=""
CERT_EXPIRY_DAYS=""

function pki::check_prerequisites() {
  if ! command -v openssl &> /dev/null; then
    echo "openssl is not installed. Please install openssl to continue."
    exit 1
  fi
}

function pki::generate_ca_key_cert() {
  echo "Generating CA key..."
  # generate the CA key
  openssl genrsa -out ${TLS_DIR}/ca.key 4096
  # create the configuration to generate the CA cert
  pki::create_ca_config
  echo "Generating CA certificate..."
  # generate the CA cert
  openssl req \
    -new \
    -x509 \
    -config ${TLS_ARTIFACT_REQUESTS_DIR}/ca.cnf \
    -key ${TLS_DIR}/ca.key \
    -out ${TLS_DIR}/ca.crt \
    -days ${CERT_EXPIRY_DAYS} \
    -batch
}

function pki::create_ca_config() {
  echo "Creating CA configuration at ${TLS_ARTIFACT_REQUESTS_DIR}/ca.cnf..."
  cat >${TLS_ARTIFACT_REQUESTS_DIR}/ca.cnf <<EOF
  [ ca ]
  default_ca = CA_default

  [ CA_default ]
  default_days = ${CERT_EXPIRY_DAYS}
  default_md = sha512
  unique_subject = no
  copy_extensions = copy

  # used to create the CA certificate
  [ req ]
  prompt = no
  distinguished_name = dn
  x509_extensions = extensions

  [ dn ]
  CN = etcd-druid-ca
  O = Gardener

  # https://docs.openssl.org/3.4/man5/x509v3_config/#standard-extensions
  [ extensions ]
  keyUsage = critical,digitalSignature,keyEncipherment,keyCertSign,cRLSign
  basicConstraints = critical,CA:true,pathlen:1
  subjectKeyIdentifier=hash
EOF
}

function pki::generate_server_key_cert() {
  if [[ $# -ne 1 ]]; then
    echo -e "${FUNCNAME[0]} expects a namespace"
    exit 1
  fi
  local namespace="$1"
  echo "Generating server key..."
  openssl genrsa -out ${TLS_DIR}/server.key 4096
  # create the configuration to generate the server cert
  pki::create_server_config ${namespace}
  echo "Generating server CSR..."
  openssl req \
    -new \
    -config ${TLS_ARTIFACT_REQUESTS_DIR}/server.cnf \
    -key ${TLS_DIR}/server.key \
    -out ${TLS_DIR}/server.csr \
    -batch
  echo "Generating server certificate..."
  openssl x509 \
    -req \
    -in ${TLS_DIR}/server.csr \
    -CA ${TLS_DIR}/ca.crt \
    -CAkey ${TLS_DIR}/ca.key \
    -CAcreateserial \
    -out ${TLS_DIR}/server.crt \
    -days ${CERT_EXPIRY_DAYS} \
    -extfile ${TLS_ARTIFACT_REQUESTS_DIR}/server.cnf \
    -extensions extensions
}

function pki::create_server_config() {
  if [[ $# -ne 1 ]]; then
    echo -e "${FUNCNAME[0]} expects a namespace"
    exit 1
  fi
  local namespace="$1"

  echo "Creating server configuration at ${TLS_ARTIFACT_REQUESTS_DIR}/server.cnf..."
  cat >${TLS_ARTIFACT_REQUESTS_DIR}/server.cnf <<EOF
  [ req ]
  prompt = no
  distinguished_name = dn
  req_extensions = extensions

  [ dn ]
  CN = etcd-druid-server
  O = Gardener

  [ extensions ]
  keyUsage = critical,digitalSignature,keyEncipherment
  extendedKeyUsage = serverAuth
  basicConstraints = critical,CA:FALSE
  subjectAltName = @sans

  [ sans ]
  DNS.0 = etcd-druid
  DNS.1 = etcd-druid.default
  DNS.2 = etcd-druid.default.svc
  DNS.3 = etcd-druid.default.svc.cluster.local
  DNS.4 = etcd-druid.${namespace}
  DNS.5 = etcd-druid.${namespace}.svc
  DNS.6 = etcd-druid.${namespace}.svc.cluster.local

EOF
}

function pki::generate_resources() {
  pki::check_prerequisites
  if [[ $# -ne 3 ]]; then
    echo -e "${FUNCNAME[0]} requires expects a target directory, namespace and a certificate expiry in days"
    exit 1
  fi
  TLS_DIR="$1"
  TLS_ARTIFACT_REQUESTS_DIR="${TLS_DIR}/requests"
  mkdir -p ${TLS_ARTIFACT_REQUESTS_DIR}
  local namespace="$2"
  CERT_EXPIRY_DAYS="$3"

  pki::generate_ca_key_cert
  pki::generate_server_key_cert ${namespace}
}