module github.com/gardener/etcd-druid

go 1.12

require (
	github.com/gardener/gardener v0.33.0
	github.com/hashicorp/go-multierror v1.0.0 // indirect
	github.com/onsi/ginkgo v1.10.3
	github.com/onsi/gomega v1.7.1
	github.com/sirupsen/logrus v1.4.2
	golang.org/x/net v0.0.0-20191209160850-c0dbc17a3553
	k8s.io/api v0.0.0-20191004102349-159aefb8556b
	k8s.io/apimachinery v0.0.0-20191004074956-c5d2f014d689
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/helm v2.14.2+incompatible
	k8s.io/utils v0.0.0-20190712204705-3dccf664f023 // indirect
	sigs.k8s.io/controller-runtime v0.2.2
)

replace (
	github.com/gardener/gardener-extensions => github.com/gardener/gardener-extensions v0.0.0-20191028142629-438a3dcf5eca
	k8s.io/api => k8s.io/api v0.0.0-20191004102349-159aefb8556b // kubernetes-1.14.8
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190315093550-53c4693659ed // kubernetes-1.14.8
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20191004074956-c5d2f014d689 // kubernetes-1.14.8
	k8s.io/apiserver => k8s.io/apiserver v0.0.0-20191010014313-3893be10d307 // kubernetes-1.14.8
	k8s.io/client-go => k8s.io/client-go v11.0.0+incompatible // kubernetes-1.14.0
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.0.0-20190816225014-88e17f53ad9d // kubernetes-1.14.8
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20190704094409-6c2a4329ac29 // kubernetes-1.14.8
	k8s.io/component-base => k8s.io/component-base v0.0.0-20190816222507-f3799749b6b7 // kubernetes-1.14.8
	k8s.io/helm => k8s.io/helm v2.13.1+incompatible
	k8s.io/klog => k8s.io/klog v0.1.0
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.0.0-20191004104030-d9d5f0cc7532 // kubernetes-1.14.8
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20190320154901-5e45bb682580
)
