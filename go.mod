module github.com/gardener/etcd-druid

go 1.12

require (
	github.com/Masterminds/goutils v1.1.0 // indirect
	github.com/cyphar/filepath-securejoin v0.2.2 // indirect
	github.com/gardener/external-dns-management v0.0.0-20190722114702-f6b12f6e4b43 // indirect
	github.com/gardener/gardener v0.0.0-20191001105106-32539124646f
	github.com/gobuffalo/logger v1.0.1 // indirect
	github.com/gobuffalo/packd v0.3.0 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/hashicorp/go-multierror v1.0.0 // indirect
	github.com/karrick/godirwalk v1.10.12 // indirect
	github.com/onsi/ginkgo v1.10.3
	github.com/onsi/gomega v1.5.0
	github.com/sirupsen/logrus v1.4.2
	github.com/ugorji/go/codec v1.1.7 // indirect
	golang.org/x/crypto v0.0.0-20191202143827-86a70503ff7e // indirect
	golang.org/x/lint v0.0.0-20191125180803-fdd1cda4f05f // indirect
	golang.org/x/net v0.0.0-20191204025024-5ee1b9f4859a
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e // indirect
	golang.org/x/sys v0.0.0-20191204072324-ce4227a45e2e // indirect
	golang.org/x/tools v0.0.0-20191204193430-660eba4da30b // indirect
	golang.org/x/xerrors v0.0.0-20191204190536-9bdfabe68543 // indirect
	gopkg.in/ini.v1 v1.44.0 // indirect
	k8s.io/api v0.0.0-20190826194732-9f642ccb7a30
	k8s.io/apimachinery v0.0.0-20190827074644-f378a67c6af3
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/helm v2.14.3+incompatible
	k8s.io/kube-openapi v0.0.0-20190722073852-5e22f3d471e6 // indirect
	k8s.io/utils v0.0.0-20190712204705-3dccf664f023 // indirect
	sigs.k8s.io/controller-runtime v0.2.0-beta.4
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
