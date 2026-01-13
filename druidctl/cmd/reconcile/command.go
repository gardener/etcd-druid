// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package reconcile

import (
	"context"
	"time"

	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"

	"github.com/spf13/cobra"
)

const (
	defaultTimeout = 5 * time.Minute
)

var (
	reconcileExample = `
		# Reconcile an Etcd resource named "my-etcd" in the test namespace
		kubectl druid reconcile test/my-etcd

		# Reconcile multiple Etcd resources using space-separated list
		kubectl druid reconcile test/my-etcd dev/my-etcd

		# Reconcile all Etcd resources across all namespaces
		kubectl druid reconcile -A

		# Reconcile an Etcd resource and wait until it's ready
		kubectl druid reconcile test/my-etcd --wait-till-ready

		# Reconcile and watch until ready (indefinite timeout)
		kubectl druid reconcile test/my-etcd --watch`

	suspendExample = `
		# Suspend reconciliation for an Etcd resource
		kubectl druid reconcile suspend test/my-etcd

		# Suspend reconciliation for multiple Etcd resources
		kubectl druid reconcile suspend test/my-etcd dev/my-etcd

		# Suspend reconciliation for all Etcd resources
		kubectl druid reconcile suspend -A`

	resumeExample = `
		# Resume reconciliation for an Etcd resource
		kubectl druid reconcile resume test/my-etcd

		# Resume reconciliation for multiple Etcd resources
		kubectl druid reconcile resume test/my-etcd dev/my-etcd

		# Resume reconciliation for all Etcd resources
		kubectl druid reconcile resume -A`
)

// NewReconcileCommand creates the 'reconcile' command with nested subcommands
// Structure:
//   - `kubectl druid reconcile <resources>` - trigger reconciliation
//   - `kubectl druid reconcile suspend <resources>` - suspend reconciliation
//   - `kubectl druid reconcile resume <resources>` - resume reconciliation
func NewReconcileCommand(options *cmdutils.GlobalOptions) *cobra.Command {
	var waitTillReady bool
	var watch bool
	var timeout time.Duration = defaultTimeout

	reconcileCmd := &cobra.Command{
		Use:   "reconcile [resources] [flags]",
		Short: "Manage etcd resource reconciliation",
		Long: `Manage etcd resource reconciliation.

When called directly with resource names, triggers a reconciliation of the specified etcd resources.
Use subcommands 'suspend' and 'resume' to control reconciliation state.`,
		Example: reconcileExample,
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// If no args and no -A flag, show help
			if len(options.ResourceArgs) == 0 && !options.AllNamespaces {
				return cmd.Help()
			}

			// Create reconcile context
			reconcileOpts := newReconcileOptions(options, waitTillReady, watch, timeout)
			ctx := &reconcileCmdCtx{reconcileOptions: reconcileOpts}

			if err := ctx.validate(); err != nil {
				options.Logger.Error(options.IOStreams.ErrOut, "Reconciling validation failed", err)
				if herr := cmd.Help(); herr != nil {
					options.Logger.Warning(options.IOStreams.ErrOut, "Failed to show help: ", herr.Error())
				}
				return err
			}

			if err := ctx.complete(options); err != nil {
				return err
			}

			if options.AllNamespaces {
				options.Logger.Info(options.IOStreams.Out, "Reconciling Etcd resources across all namespaces")
			} else {
				options.Logger.Info(options.IOStreams.Out, "Reconciling selected Etcd resources")
			}

			if err := ctx.execute(context.TODO()); err != nil {
				options.Logger.Error(options.IOStreams.ErrOut, "Reconciling failed", err)
				return err
			}
			options.Logger.Success(options.IOStreams.Out, "Reconciling completed successfully")
			return nil
		},
	}

	// Add reconcile-specific flags
	reconcileCmd.Flags().BoolVarP(&waitTillReady, "wait-till-ready", "w", false,
		"Wait until the Etcd resource is ready before reconciling")
	reconcileCmd.Flags().BoolVarP(&watch, "watch", "W", false,
		"Watch the Etcd resources until they are ready after reconciliation")
	reconcileCmd.Flags().DurationVarP(&timeout, "timeout", "t", defaultTimeout,
		"Timeout for the reconciliation process")

	// Add subcommands
	reconcileCmd.AddCommand(NewSuspendCommand(options))
	reconcileCmd.AddCommand(NewResumeCommand(options))

	return reconcileCmd
}

// NewSuspendCommand creates the 'reconcile suspend' subcommand
func NewSuspendCommand(options *cmdutils.GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "suspend [resources] [flags]",
		Short:   "Suspend reconciliation for etcd resources",
		Long:    "Suspend reconciliation for the specified etcd resources by adding a suspend annotation.",
		Example: suspendExample,
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			suspendOpts := newSuspendReconcileOptions(options)
			ctx := &suspendReconcileCmdCtx{suspendReconcileOptions: suspendOpts}

			if err := ctx.validate(); err != nil {
				options.Logger.Error(options.IOStreams.ErrOut, "Suspending reconciliation validation failed", err)
				if herr := cmd.Help(); herr != nil {
					options.Logger.Warning(options.IOStreams.ErrOut, "Failed to show help: ", herr.Error())
				}
				return err
			}

			if err := ctx.complete(options); err != nil {
				return err
			}

			if options.AllNamespaces {
				options.Logger.Info(options.IOStreams.Out, "Suspending reconciliation for Etcd resources across all namespaces")
			} else {
				options.Logger.Info(options.IOStreams.Out, "Suspending reconciliation for selected Etcd resources")
			}

			if err := ctx.execute(context.TODO()); err != nil {
				options.Logger.Error(options.IOStreams.ErrOut, "Suspending reconciliation failed", err)
				return err
			}
			options.Logger.Success(options.IOStreams.Out, "Suspending reconciliation completed successfully")
			return nil
		},
	}

	return cmd
}

// NewResumeCommand creates the 'reconcile resume' subcommand
func NewResumeCommand(options *cmdutils.GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "resume [resources] [flags]",
		Short:   "Resume reconciliation for etcd resources",
		Long:    "Resume reconciliation for the specified etcd resources by removing the suspend annotation.",
		Example: resumeExample,
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resumeOpts := newResumeReconcileOptions(options)
			ctx := &resumeReconcileCmdCtx{resumeReconcileOptions: resumeOpts}

			if err := ctx.validate(); err != nil {
				options.Logger.Error(options.IOStreams.ErrOut, "Resuming reconciliation validation failed", err)
				if herr := cmd.Help(); herr != nil {
					options.Logger.Warning(options.IOStreams.ErrOut, "Failed to show help: ", herr.Error())
				}
				return err
			}

			if err := ctx.complete(options); err != nil {
				return err
			}

			if options.AllNamespaces {
				options.Logger.Info(options.IOStreams.Out, "Resuming reconciliation for Etcd resources across all namespaces")
			} else {
				options.Logger.Info(options.IOStreams.Out, "Resuming reconciliation for selected Etcd resources")
			}

			if err := ctx.execute(context.TODO()); err != nil {
				options.Logger.Error(options.IOStreams.ErrOut, "Resuming reconciliation failed", err)
				return err
			}
			options.Logger.Success(options.IOStreams.Out, "Resuming reconciliation completed successfully")
			return nil
		},
	}

	return cmd
}
