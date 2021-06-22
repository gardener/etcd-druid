module github.com/gardener/etcd-druid

go 1.16

require (
	github.com/gardener/etcd-backup-restore v0.12.1
	github.com/gardener/etcd-druid/api v0.0.0-00010101000000-000000000000
	github.com/gardener/gardener v1.23.0
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr v0.3.0
	github.com/golang/mock v1.5.0
	github.com/onsi/ginkgo v1.14.2
	github.com/onsi/gomega v1.10.5
	github.com/sirupsen/logrus v1.7.0
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
	helm.sh/helm/v3 v3.2.4
	k8s.io/api v0.20.6
	k8s.io/apiextensions-apiserver v0.20.6
	k8s.io/apimachinery v0.20.6
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/helm v2.16.1+incompatible
	k8s.io/utils v0.0.0-20210111153108-fddb29f9d009
	sigs.k8s.io/controller-runtime v0.8.3
	sigs.k8s.io/controller-tools v0.4.1
)

replace (
	// Ref: https://github.com/Azure/go-autorest/issues/414
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible
	github.com/gardener/etcd-druid/api => ./api
	// Ref: https://github.com/helm/helm/commit/4faeedd98b03e5af7733317a84e77ebff28c55f7
	helm.sh/helm/v3 => helm.sh/helm/v3 v3.4.2
	k8s.io/api => k8s.io/api v0.19.6
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.19.6
	k8s.io/apimachinery => k8s.io/apimachinery v0.19.6
	k8s.io/client-go => k8s.io/client-go v0.19.6
	k8s.io/code-generator => k8s.io/code-generator v0.19.6
	k8s.io/helm => k8s.io/helm v2.13.1+incompatible
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.19.6
	k8s.io/kube-openapi => github.com/gardener/kube-openapi v0.0.0-20201221124747-75e88872edcf // k8s-1.19
)
