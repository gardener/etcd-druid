module github.com/gardener/etcd-druid

go 1.12

require (
	github.com/Azure/go-autorest/autorest/adal v0.5.0 // indirect
	github.com/Azure/go-autorest/autorest/mocks v0.2.0 // indirect
	github.com/gardener/gardener v0.0.0-20190828091125-4cf281185eef // indirect
	github.com/go-logr/logr v0.1.0
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	golang.org/x/net v0.0.0-20190812203447-cdfb69ac37fc
	k8s.io/api v0.0.0-20190826194732-9f642ccb7a30
	k8s.io/apimachinery v0.0.0-20190827074644-f378a67c6af3
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/helm v2.14.3+incompatible
	k8s.io/kube-aggregator v0.0.0-20190827195343-ca50d88afdbc // indirect
	sigs.k8s.io/controller-runtime v0.2.0-beta.2
	sigs.k8s.io/controller-tools v0.2.0-beta.2 // indirect
)

replace (
	k8s.io/api => k8s.io/api v0.0.0-20190313235455-40a48860b5ab //kubernetes-1.14.0
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190315093550-53c4693659ed // kubernetes-1.14.0
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190313205120-d7deff9243b1 // kubernetes-1.14.0
	k8s.io/apiserver => k8s.io/apiserver v0.0.0-20190313205120-8b27c41bdbb1 // kubernetes-1.14.0
	k8s.io/client-go => k8s.io/client-go v11.0.0+incompatible // kubernetes-1.14.0
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.0.0-20190314002537-50662da99b70 // kubernetes-1.14.0
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20190311093542-50b561225d70 // kubernetes-1.14.4
	k8s.io/component-base => k8s.io/component-base v0.0.0-20190314000054-4a91899592f4 // kubernetes-1.14.0
	k8s.io/helm => k8s.io/helm v2.13.1+incompatible
	k8s.io/klog => k8s.io/klog v0.1.0
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.0.0-20190314000639-da8327669ac5 // kubernetes-1.14.0
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20190320154901-5e45bb682580
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.2.0-beta.2
)
