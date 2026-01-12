// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package resourceprotection

import (
	"context"

	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"

	"github.com/spf13/cobra"
)

var (
	addExample = `
		# Add component protection to an Etcd resource
		kubectl druid component-protection add test/my-etcd

		# Add component protection to multiple Etcd resources
		kubectl druid component-protection add ns1/etcd1 ns2/etcd2

		# Add component protection to all Etcd resources
		kubectl druid component-protection add -A`

	removeExample = `
		# Remove component protection from an Etcd resource
		kubectl druid component-protection remove test/my-etcd

		# Remove component protection from multiple Etcd resources
		kubectl druid component-protection remove ns1/etcd1 ns2/etcd2

		# Remove component protection from all Etcd resources
		kubectl druid component-protection remove -A`
)

// NewComponentProtectionCommand creates the 'component-protection' parent command with nested subcommands
// Structure:
//   - `kubectl druid component-protection add <resources>` - enable protection
//   - `kubectl druid component-protection remove <resources>` - disable protection
func NewComponentProtectionCommand(options *cmdutils.GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "component-protection",
		Short: "Manage component protection for etcd resources",
		Long: `Manage component protection for etcd resources.

Component protection prevents accidental deletion of managed Kubernetes resources
(StatefulSets, Services, ConfigMaps, etc.) by etcd-druid.

Use 'add' to enable protection and 'remove' to disable protection.
NOTE: This will only have effect if resource protection webhook has been enabled when deploying etcd-druid.`,
	}

	// Add subcommands
	cmd.AddCommand(NewAddCommand(options))
	cmd.AddCommand(NewRemoveCommand(options))

	return cmd
}

// NewAddCommand creates the 'component-protection add' subcommand
func NewAddCommand(options *cmdutils.GlobalOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "add [resources] [flags]",
		Short:   "Enable component protection for etcd resources",
		Long:    "Enable component protection for the specified etcd resources by removing the disable-protection annotation.",
		Example: addExample,
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resourceProtectionOptions := newResourceProtectionOptions(options)
			ctx := &resourceProtectionCmdCtx{
				resourceProtectionOptions: resourceProtectionOptions,
			}

			if err := ctx.validate(); err != nil {
				options.Logger.Error(options.IOStreams.ErrOut, "Add component protection validation failed", err)
				if err := cmd.Help(); err != nil {
					options.Logger.Warning(options.IOStreams.ErrOut, "Failed to show help: ", err.Error())
				}
				return err
			}

			if err := ctx.complete(options); err != nil {
				return err
			}

			if err := ctx.removeDisableProtectionAnnotation(context.TODO()); err != nil {
				options.Logger.Error(options.IOStreams.ErrOut, "Add component protection failed", err)
				return err
			}

			options.Logger.Success(options.IOStreams.Out, "Component protection added successfully")
			return nil
		},
	}
}

// NewRemoveCommand creates the 'component-protection remove' subcommand
func NewRemoveCommand(options *cmdutils.GlobalOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "remove [resources] [flags]",
		Short:   "Disable component protection for etcd resources",
		Long:    "Disable component protection for the specified etcd resources by adding the disable-protection annotation.",
		Example: removeExample,
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resourceProtectionOptions := newResourceProtectionOptions(options)
			ctx := &resourceProtectionCmdCtx{
				resourceProtectionOptions: resourceProtectionOptions,
			}

			if err := ctx.validate(); err != nil {
				options.Logger.Error(options.IOStreams.ErrOut, "Remove component protection validation failed", err)
				if err := cmd.Help(); err != nil {
					options.Logger.Warning(options.IOStreams.ErrOut, "Failed to show help: ", err.Error())
				}
				return err
			}

			if err := ctx.complete(options); err != nil {
				return err
			}

			if err := ctx.addDisableProtectionAnnotation(context.TODO()); err != nil {
				options.Logger.Error(options.IOStreams.ErrOut, "Remove component protection failed", err)
				return err
			}

			options.Logger.Success(options.IOStreams.Out, "Component protection removed successfully")
			return nil
		},
	}
}
