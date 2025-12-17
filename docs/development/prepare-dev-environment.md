# Prepare Dev Environment

This guide will provide with detailed instructions on installing all dependencies and tools that are required to start developing and testing `etcd-druid`.

## [macOS only] Installing Homebrew

Homebrew is a popular package manager for macOS. You can install it by executing the following command in a terminal:
```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

## Installing Go

On macOS run:

```bash
brew install go
```

Alternatively you can also follow the [Go installation documentation](https://go.dev/doc/install).

## Installing Git

We use `git` as VCS which you need to install.
On macOS run:

```bash
brew install git
```

For other OS, please check the [Git installation documentation](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git).

## Installing Docker

You need to have docker installed and running. This will allow starting a [kind](https://kind.sigs.k8s.io/) cluster or a [minikube](https://minikube.sigs.k8s.io/docs/) cluster for locally deploying `etcd-druid`.

On macOS run:

```bash
brew install docker
```

Alternatively you can also follow the [Docker installation documentation](https://docs.docker.com/get-docker/).

## Installing Kubectl

To interact with the local Kubernetes cluster you will need kubectl.
On macOS run:

```bash
brew install kubernetes-cli
```

For other OS, please check the [Kubectl installation documentation](https://kubernetes.io/docs/tasks/tools/).

## Other tools that might come in handy

To operate `etcd-druid` you do not need these tools but they usually come in handy when working with YAML/JSON files.
On macOS run:

```bash
# jq (https://jqlang.github.io/jq/) is a lightweight and flexible command-line JSON processor
brew install jq
# yq (https://mikefarah.gitbook.io/yq) is a lightweight and portable command-line YAML processor.
brew install yq
```

## Get the sources

Clone the repository from Github into your `$GOPATH`.
```bash
mkdir -p $(go env GOPATH)/src/github.com/gardener
cd $(go env GOPATH)src/github.com/gardener
git clone https://github.com/gardener/etcd-druid.git
# alternatively you can also use `git clone git@github.com:gardener/etcd-druid.git`
```
