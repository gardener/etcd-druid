package resourceprotection

import (
	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
	"github.com/gardener/etcd-druid/druidctl/internal/client"
	"k8s.io/apimachinery/pkg/types"
)

type resourceProtectionOptions struct {
	*cmdutils.GlobalOptions
}

type resourceProtectionCmdCtx struct {
	*resourceProtectionOptions
	etcdRefList []types.NamespacedName
	etcdClient  client.EtcdClientInterface
}

func newResourceProtectionOptions(options *cmdutils.GlobalOptions) *resourceProtectionOptions {
	return &resourceProtectionOptions{
		GlobalOptions: options,
	}
}
