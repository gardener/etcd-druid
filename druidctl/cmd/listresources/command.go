package listresources

import (
	"context"

	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
	"github.com/gardener/etcd-druid/druidctl/internal/printer"
	"github.com/spf13/cobra"
)

const defaultFilter = "all"

var (
	example = `
		# List all managed resources for the etcd resource named 'my-etcd' in the 'test' namespace
		druidctl list-resources 'test/my-etcd'

		# List all managed resources for all etcd resources in the 'test' namespace
		druidctl list-resources 'test/*'

		# List all managed resources for different etcd resources in different namespaces
		druidctl list-resources 'test/my-etcd,dev/my-etcd'

		# List all managed resources for all etcd resources across all namespaces
		druidctl list-resources --all-namespaces

		# List only the Secrets and ConfigMaps managed resources for the etcd resource named 'my-etcd' in the 'test' namespace
		druidctl list-resources 'test/my-etcd' --filter=secrets,configmaps

		# List all managed resources for the etcd resource named 'my-etcd' in the 'test' namespace in JSON format
		druidctl list-resources 'test/my-etcd' --output=json

		# List all managed resources for all etcd resources across all namespaces in YAML format
		druidctl list-resources --all-namespaces --output=yaml
	`
)

// NewListResourcesCommand creates the list-resources command
func NewListResourcesCommand(options *cmdutils.GlobalOptions) *cobra.Command {
	listResourcesOptions := newListResourcesOptions(options, defaultFilter, string(printer.OutputTypeNone))

	listResourcesCmd := &cobra.Command{
		Use:     "list-resources <etcd-resource-name> --filter=<comma separated types> (optional flag) --output=<output-format> (optional flag)",
		Short:   "List managed resources for an etcd cluster filtered by the specified types",
		Long:    `List managed resources for an etcd cluster filtered by the specified types. If no types are specified, all managed resources will be listed.`,
		Args:    cobra.MaximumNArgs(1),
		Example: example,
		RunE: func(cmd *cobra.Command, args []string) error {
			listResourcesCmdCtx := &listResourcesCmdCtx{
				listResourcesOptions: listResourcesOptions,
			}
			if err := listResourcesCmdCtx.validate(); err != nil {
				cmd.Help()
				return err
			}

			if err := listResourcesCmdCtx.complete(options); err != nil {
				return err
			}

			if err := listResourcesCmdCtx.execute(context.TODO()); err != nil {
				options.Logger.Error(options.IOStreams.ErrOut, "Listing Managed resources for Etcds failed", err)
				return err
			}

			options.Logger.Success(options.IOStreams.Out, "Listing Managed resources for Etcds completed successfully")
			return nil
		},
	}

	listResourcesCmd.Flags().StringVarP(&listResourcesOptions.Filter, "filter", "f", defaultFilter, "Comma-separated list of resource types to include (short or full names). Use 'all' for a curated default set.")
	listResourcesCmd.Flags().StringVarP(&listResourcesOptions.OutputFormat, "output", "o", string(printer.OutputTypeNone), "Output format. One of: json, yaml")

	return listResourcesCmd
}
