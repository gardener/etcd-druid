module github.com/gardener/etcd-druid

go 1.16

require (
	github.com/gardener/etcd-druid/api v0.0.0-00010101000000-000000000000
	github.com/gardener/gardener v1.23.0
	github.com/ghodss/yaml v1.0.0
	github.com/onsi/ginkgo v1.14.2
	github.com/onsi/gomega v1.10.5
	github.com/sirupsen/logrus v1.6.0
	k8s.io/api v0.20.6
	k8s.io/apimachinery v0.21.1
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/helm v2.16.1+incompatible
	k8s.io/utils v0.0.0-20210111153108-fddb29f9d009
	sigs.k8s.io/controller-runtime v0.8.3
	sigs.k8s.io/controller-tools v0.4.1
)

replace (
	github.com/gardener/etcd-druid/api => ./api
	k8s.io/api => k8s.io/api v0.19.6
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.19.6
	k8s.io/apimachinery => k8s.io/apimachinery v0.19.6
	k8s.io/client-go => k8s.io/client-go v0.19.6
	k8s.io/code-generator => k8s.io/code-generator v0.19.6
	k8s.io/helm => k8s.io/helm v2.13.1+incompatible
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.19.6
	k8s.io/kube-openapi => github.com/gardener/kube-openapi v0.0.0-20201221124747-75e88872edcf // k8s-1.19
)
