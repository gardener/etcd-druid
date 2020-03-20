module github.com/gardener/etcd-druid

go 1.13

require (
	github.com/gardener/controller-manager-library v0.1.1-0.20200204110458-c263b9bb97ad
	github.com/gardener/gardener v1.3.1
	github.com/ghodss/yaml v1.0.0
	github.com/golang/protobuf v1.3.5 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/googleapis/gnostic v0.4.1 // indirect
	github.com/huandu/xstrings v1.3.1 // indirect
	github.com/onsi/ginkgo v1.10.1
	github.com/onsi/gomega v1.7.0
	github.com/sirupsen/logrus v1.4.2
	golang.org/x/lint v0.0.0-20191125180803-fdd1cda4f05f
	k8s.io/api v0.17.0
	k8s.io/apimachinery v0.17.0
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/code-generator v0.16.8
	k8s.io/helm v2.16.1+incompatible
	k8s.io/kube-openapi v0.0.0-20191107075043-30be4d16710a
	k8s.io/utils v0.0.0-20200327001022-6496210b90e8
	sigs.k8s.io/controller-runtime v0.4.0
)

replace (
	k8s.io/api => k8s.io/api v0.16.8
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.16.8
	k8s.io/apimachinery => k8s.io/apimachinery v0.16.8
	k8s.io/client-go => k8s.io/client-go v0.16.8
	k8s.io/code-generator => k8s.io/code-generator v0.16.8
	k8s.io/helm => k8s.io/helm v2.13.1+incompatible
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.16.8
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20190816220812-743ec37842bf
)
