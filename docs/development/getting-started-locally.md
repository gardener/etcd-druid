# Developing etcd-druid locally

You can setup `etcd-druid` locally by following detailed instructions in [this document](../deployment/getting-started-locally/getting-started-locally.md). 

* For best development experience you should use `make deploy-dev` - this helps during development where you wish to make changes to the code base and with a key-press allow automatic re-deployment of the application to the target Kubernetes cluster.
* In case you wish to start a debugging session then use `make deploy-debug` - this will additionally disable leader election and prevent leases to expire and process to stop.

> **Note:** We leverage [skaffold debug](https://skaffold.dev/docs/workflows/debug/) and [skaffold dev](https://skaffold.dev/docs/workflows/dev/) features. 