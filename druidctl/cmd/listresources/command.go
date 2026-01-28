// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package listresources

import (
	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
	"github.com/gardener/etcd-druid/druidctl/internal/printer"

	"github.com/spf13/cobra"
)

const defaultFilter = "all"

var (
	example = `
# List all managed resources for an etcd resource in the default namespace
kubectl druid list-resources my-etcd

# List all managed resources for an etcd resource in a specific namespace
kubectl druid list-resources my-etcd -n test

# List all managed resources for all etcd resources in a namespace
kubectl druid list-resources -n test

# List all managed resources for all etcd resources across all namespaces
kubectl druid list-resources -A

# List resources with label selector in a namespace
kubectl druid list-resources -l app=etcd-statefulset -n test

# List all managed resources for multiple etcd resources in a namespace
kubectl druid list-resources etcd1 etcd2 -n test

# Cross-namespace selection (explicit ns/name format)
kubectl druid list-resources ns1/etcd1 ns2/etcd2

# Filter by resource type
kubectl druid list-resources my-etcd --filter=pods,services

# Output in JSON format
kubectl druid list-resources my-etcd -n test --output=json
`
)

// NewListResourcesCommand creates the list-resources command
func NewListResourcesCommand(options *cmdutils.GlobalOptions) *cobra.Command {
	listResourcesOptions := newListResourcesOptions(options, defaultFilter, string(printer.OutputTypeNone))

	listResourcesCmd := &cobra.Command{
		Use:     "list-resources <etcd-resource-name> --filter=<comma separated types> (optional flag) --output=<output-format> (optional flag)",
		Short:   "List managed resources for an etcd cluster filtered by the specified types",
		Long:    `List managed resources for an etcd cluster filtered by the specified types. If no types are specified, all managed resources will be listed.`,
		Args:    cobra.ArbitraryArgs,
		Example: example,
		RunE: func(cmd *cobra.Command, _ []string) error {
			listResourcesCmdCtx := &listResourcesCmdCtx{
				listResourcesOptions: listResourcesOptions,
			}
			if err := listResourcesCmdCtx.validate(); err != nil {
				if herr := cmd.Help(); herr != nil {
					options.Logger.Warning(options.IOStreams.ErrOut, "Failed to show help: ", herr.Error())
				}
				return err
			}

			if err := listResourcesCmdCtx.complete(options); err != nil {
				return err
			}

			if err := listResourcesCmdCtx.execute(cmdutils.CmdContext(cmd)); err != nil {
				options.Logger.Error(options.IOStreams.ErrOut, "Listing Managed resources for Etcds failed", err)
				return err
			}

			return nil
		},
	}

	listResourcesCmd.Flags().StringVarP(&listResourcesOptions.Filter, "filter", "f", defaultFilter, "Comma-separated list of resource types to include (short or full names). Use 'all' for a curated default set.")
	listResourcesCmd.Flags().StringVarP(&listResourcesOptions.OutputFormat, "output", "o", string(printer.OutputTypeNone), "Output format. One of: json, yaml")

	return listResourcesCmd
}
