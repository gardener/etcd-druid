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
	addProtectionExample = `
		# Add component protection to an Etcd resource named "my-etcd" in the test namespace
		druidctl add-component-protection 'test/my-etcd'

		# Add component protection to all Etcd resources in the test namespace
		druidctl add-component-protection 'test/*'
		
		# Add component protection to different Etcd resources in different namespaces
		druidctl add-component-protection 'test/my-etcd,dev/my-etcd'
		
		# Add component protection to all Etcd resources in all namespaces
		druidctl add-component-protection --all-namespaces`

	removeProtectionExample = `
		# Remove component protection from an Etcd resource named "my-etcd" in the test namespace
		druidctl remove-component-protection 'test/my-etcd'

		# Remove component protection from all Etcd resources in the test namespace
		druidctl remove-component-protection 'test/*'
		
		# Remove component protection from different Etcd resources in different namespaces
		druidctl remove-component-protection 'test/my-etcd,dev/my-etcd'
		
		# Remove component protection from all Etcd resources in all namespaces
		druidctl remove-component-protection --all-namespaces`
)

// Create add-component-protection subcommand
func NewAddProtectionCommand(options *cmdutils.GlobalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "add-component-protection <etcd-resource-name>",
		Short: "Adds resource protection to all managed components for a given etcd cluster",
		Long: `Adds resource protection to all managed components for a given etcd cluster.
			   NOTE: This will only have effect if resource protection webhook has been enabled when deploying etcd-druid.`,
		Example: addProtectionExample,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resourceProtectionOptions := newResourceProtectionOptions(options)
			resourceProtectionCmdCtx := &resourceProtectionCmdCtx{
				resourceProtectionOptions: resourceProtectionOptions,
			}

			if err := resourceProtectionCmdCtx.validate(); err != nil {
				cmd.Help()
				return err
			}

			if err := resourceProtectionCmdCtx.complete(options); err != nil {
				return err
			}

			if err := resourceProtectionCmdCtx.removeDisableProtectionAnnotation(context.TODO()); err != nil {
				options.Logger.Error(options.IOStreams.ErrOut, "Add component protection failed", err)
				return err
			}

			options.Logger.Success(options.IOStreams.Out, "Component protection added successfully")
			return nil
		},
	}
}

// Create remove-component-protection subcommand
func NewRemoveProtectionCommand(options *cmdutils.GlobalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "remove-component-protection <etcd-resource-name>",
		Short: "Removes resource protection for all managed components for a given etcd cluster",
		Long: `Removes resource protection for all managed components for a given etcd cluster.
			   NOTE: This will only have effect if resource protection webhook has been enabled when deploying etcd-druid.`,
		Example: removeProtectionExample,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resourceProtectionOptions := newResourceProtectionOptions(options)
			resourceProtectionCmdCtx := &resourceProtectionCmdCtx{
				resourceProtectionOptions: resourceProtectionOptions,
			}

			if err := resourceProtectionCmdCtx.validate(); err != nil {
				return err
			}

			if err := resourceProtectionCmdCtx.complete(options); err != nil {
				return err
			}

			if err := resourceProtectionCmdCtx.addDisableProtectionAnnotation(context.TODO()); err != nil {
				options.Logger.Error(options.IOStreams.ErrOut, "Remove component protection failed", err)
				return err
			}

			options.Logger.Success(options.IOStreams.Out, "Component protection removed successfully")
			return nil
		},
	}
}
