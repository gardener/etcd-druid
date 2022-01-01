module github.com/gardener/etcd-druid

go 1.16

require (
	github.com/gardener/gardener v1.36.0
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr v1.2.0
	github.com/golang/mock v1.6.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.17.0
	k8s.io/api v0.23.0
	k8s.io/apimachinery v0.23.0
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/utils v0.0.0-20210930125809-cb0fa318a74b
	sigs.k8s.io/controller-runtime v0.11.0
	sigs.k8s.io/controller-tools v0.7.0
)

replace (
	k8s.io/api => k8s.io/api v0.22.2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.22.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.22.2
	k8s.io/client-go => k8s.io/client-go v0.22.2
	k8s.io/code-generator => k8s.io/code-generator v0.22.2
	k8s.io/helm => k8s.io/helm v2.13.1+incompatible
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.22.2
)
