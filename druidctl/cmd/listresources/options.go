// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package listresources

import (
	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
	client "github.com/gardener/etcd-druid/druidctl/internal/client"
	"github.com/gardener/etcd-druid/druidctl/internal/printer"
	"k8s.io/apimachinery/pkg/types"
)

type listResourcesOptions struct {
	*cmdutils.GlobalOptions
	Filter       string
	OutputFormat string
}

type listResourcesCmdCtx struct {
	*listResourcesOptions
	etcdRefList   []types.NamespacedName
	EtcdClient    client.EtcdClientInterface
	GenericClient client.GenericClientInterface
	Printer       printer.Printer
}

func newListResourcesOptions(options *cmdutils.GlobalOptions, filter string, outputFormat string) *listResourcesOptions {
	return &listResourcesOptions{
		GlobalOptions: options,
		Filter:        filter,
		OutputFormat:  outputFormat,
	}
}
