// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package listresources

import (
	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
	client "github.com/gardener/etcd-druid/druidctl/internal/client"
	"github.com/gardener/etcd-druid/druidctl/internal/printer"

	"k8s.io/apimachinery/pkg/types"
)

// listResourcesOptions holds command-specific options for the list-resources command
type listResourcesOptions struct {
	*cmdutils.GlobalOptions
	Filter       string
	OutputFormat string
}

// listResourcesRuntime holds runtime state for the list-resources command
type listResourcesRuntime struct {
	*cmdutils.RuntimeEnv
	etcdRefList   []types.NamespacedName
	EtcdClient    client.EtcdClientInterface
	GenericClient client.GenericClientInterface
	Printer       printer.Printer
}

// listResourcesCmdCtx composes options and runtime for the list-resources command
type listResourcesCmdCtx struct {
	*listResourcesOptions
	*listResourcesRuntime
}

func newListResourcesOptions(options *cmdutils.GlobalOptions, filter string, outputFormat string) *listResourcesOptions {
	return &listResourcesOptions{
		GlobalOptions: options,
		Filter:        filter,
		OutputFormat:  outputFormat,
	}
}

func newListResourcesRuntime(runtime *cmdutils.RuntimeEnv) *listResourcesRuntime {
	return &listResourcesRuntime{
		RuntimeEnv: runtime,
	}
}
