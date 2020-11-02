module github.com/gardener/etcd-druid

go 1.13

require (
	github.com/gardener/etcd-druid/api v0.0.0-00010101000000-000000000000
	github.com/gardener/gardener v1.11.3
	github.com/ghodss/yaml v1.0.0
	github.com/onsi/ginkgo v1.14.0
	github.com/onsi/gomega v1.10.1
	github.com/sirupsen/logrus v1.6.0
	k8s.io/api v0.18.8
	k8s.io/apimachinery v0.18.8
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/helm v2.16.1+incompatible
	k8s.io/utils v0.0.0-20200603063816-c1c6865ac451
	sigs.k8s.io/controller-runtime v0.6.3
)

replace (
	github.com/gardener/etcd-druid/api => ./api
	k8s.io/api => k8s.io/api v0.18.8
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.18.8
	k8s.io/apimachinery => k8s.io/apimachinery v0.18.8
	k8s.io/client-go => k8s.io/client-go v0.18.8
	k8s.io/code-generator => k8s.io/code-generator v0.18.8
	k8s.io/helm => k8s.io/helm v2.13.1+incompatible
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.18.8
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20190816220812-743ec37842bf
	sigs.k8s.io/controller-runtime => github.com/gardener/controller-runtime v0.6.3-gardener.1
	sigs.k8s.io/structured-merge-diff/v3 => github.com/gardener/structured-merge-diff/v3 v3.0.0-gardener.1
)
