// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package reconciliation

import (
	"time"

	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"

	"github.com/spf13/cobra"
)

const (
	defaultTimeout = 5 * time.Minute
)

var (
	triggerExample = `
# Trigger reconciliation for an Etcd resource named "my-etcd" in the test namespace
kubectl druid reconciliation trigger test/my-etcd

# Trigger reconciliation for multiple Etcd resources using space-separated list
kubectl druid reconciliation trigger test/my-etcd dev/my-etcd

# Trigger reconciliation for all Etcd resources across all namespaces
kubectl druid reconciliation trigger -A

# Trigger reconciliation for an Etcd resource and wait until it's ready
kubectl druid reconciliation trigger test/my-etcd --wait-till-ready

# Trigger reconciliation and watch until ready (indefinite timeout)
kubectl druid reconciliation trigger test/my-etcd --watch
`
	suspendExample = `
# Suspend reconciliation for an Etcd resource
kubectl druid reconciliation suspend test/my-etcd

# Suspend reconciliation for multiple Etcd resources
kubectl druid reconciliation suspend test/my-etcd dev/my-etcd

# Suspend reconciliation for all Etcd resources
kubectl druid reconciliation suspend -A
`
	resumeExample = `
# Resume reconciliation for an Etcd resource
kubectl druid reconciliation resume test/my-etcd

# Resume reconciliation for multiple Etcd resources
kubectl druid reconciliation resume test/my-etcd dev/my-etcd

# Resume reconciliation for all Etcd resources
kubectl druid reconciliation resume -A
`
)

// NewReconciliationCommand creates the 'reconciliation' command with nested subcommands
// Structure:
//   - `kubectl druid reconciliation trigger <resources>` - trigger reconciliation
//   - `kubectl druid reconciliation suspend <resources>` - suspend reconciliation
//   - `kubectl druid reconciliation resume <resources>` - resume reconciliation
func NewReconciliationCommand(cmdCtx *cmdutils.CommandContext) *cobra.Command {
	reconciliationCmd := &cobra.Command{
		Use:   "reconciliation",
		Short: "Manage etcd resource reconciliation",
		Long: `Manage etcd resource reconciliation.
Use subcommands 'trigger', 'suspend', and 'resume' to control reconciliation state.`,
	}

	// Add subcommands
	reconciliationCmd.AddCommand(NewTriggerCommand(cmdCtx))
	reconciliationCmd.AddCommand(NewSuspendCommand(cmdCtx))
	reconciliationCmd.AddCommand(NewResumeCommand(cmdCtx))

	return reconciliationCmd
}

// NewTriggerCommand creates the 'reconciliation trigger' subcommand
func NewTriggerCommand(cmdCtx *cmdutils.CommandContext) *cobra.Command {
	var waitTillReady bool
	var watch bool
	var timeout time.Duration = defaultTimeout

	cmd := &cobra.Command{
		Use:     "trigger [resources] [flags]",
		Short:   "Trigger reconciliation for etcd resources",
		Long:    "Trigger reconciliation for the specified etcd resources by adding a reconcile annotation.",
		Example: triggerExample,
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := cmdCtx.Options
			runtime := cmdCtx.Runtime

			// If no args and no -A flag, show help
			if len(opts.ResourceArgs) == 0 && !opts.AllNamespaces {
				return cmd.Help()
			}

			// Create reconcile context
			reconcileOpts := newReconcileOptions(opts, waitTillReady, watch, timeout)
			reconcileRuntime := newReconcileRuntime(runtime)
			ctx := &reconcileCmdCtx{
				reconcileOptions: reconcileOpts,
				reconcileRuntime: reconcileRuntime,
			}

			if err := ctx.validate(); err != nil {
				runtime.Logger.Error(runtime.IOStreams.ErrOut, "Trigger reconciliation validation failed", err)
				if herr := cmd.Help(); herr != nil {
					runtime.Logger.Warning(runtime.IOStreams.ErrOut, "Failed to show help: ", herr.Error())
				}
				return err
			}

			if err := ctx.complete(); err != nil {
				return err
			}

			if opts.Verbose {
				if opts.AllNamespaces {
					runtime.Logger.Info(runtime.IOStreams.Out, "Triggering reconciliation for Etcd resources across all namespaces")
				} else {
					runtime.Logger.Info(runtime.IOStreams.Out, "Triggering reconciliation for selected Etcd resources")
				}
			}

			if err := ctx.execute(cmdutils.CmdContext(cmd)); err != nil {
				runtime.Logger.Error(runtime.IOStreams.ErrOut, "Trigger reconciliation failed", err)
				return err
			}
			return nil
		},
	}

	// Add trigger-specific flags
	cmd.Flags().BoolVarP(&waitTillReady, "wait-till-ready", "w", false,
		"Wait until the Etcd resource is ready after triggering reconciliation")
	cmd.Flags().BoolVarP(&watch, "watch", "W", false,
		"Watch the Etcd resources until they are ready after reconciliation (no timeout)")
	cmd.Flags().DurationVarP(&timeout, "timeout", "t", defaultTimeout,
		"Timeout for waiting (only valid with --wait-till-ready)")

	return cmd
}

// NewSuspendCommand creates the 'reconciliation suspend' subcommand
func NewSuspendCommand(cmdCtx *cmdutils.CommandContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "suspend [resources] [flags]",
		Short:   "Suspend reconciliation for etcd resources",
		Long:    "Suspend reconciliation for the specified etcd resources by adding a suspend annotation.",
		Example: suspendExample,
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := cmdCtx.Options
			runtime := cmdCtx.Runtime

			suspendOpts := newSuspendReconcileOptions(opts)
			suspendRuntime := newSuspendReconcileRuntime(runtime)
			ctx := &suspendReconcileCmdCtx{
				suspendReconcileOptions: suspendOpts,
				suspendReconcileRuntime: suspendRuntime,
			}

			if err := ctx.validate(); err != nil {
				runtime.Logger.Error(runtime.IOStreams.ErrOut, "Suspending reconciliation validation failed", err)
				if herr := cmd.Help(); herr != nil {
					runtime.Logger.Warning(runtime.IOStreams.ErrOut, "Failed to show help: ", herr.Error())
				}
				return err
			}

			if err := ctx.complete(); err != nil {
				return err
			}

			if opts.Verbose {
				if opts.AllNamespaces {
					runtime.Logger.Info(runtime.IOStreams.Out, "Suspending reconciliation for Etcd resources across all namespaces")
				} else {
					runtime.Logger.Info(runtime.IOStreams.Out, "Suspending reconciliation for selected Etcd resources")
				}
			}

			if err := ctx.execute(cmdutils.CmdContext(cmd)); err != nil {
				runtime.Logger.Error(runtime.IOStreams.ErrOut, "Suspending reconciliation failed", err)
				return err
			}
			return nil
		},
	}

	return cmd
}

// NewResumeCommand creates the 'reconciliation resume' subcommand
func NewResumeCommand(cmdCtx *cmdutils.CommandContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "resume [resources] [flags]",
		Short:   "Resume reconciliation for etcd resources",
		Long:    "Resume reconciliation for the specified etcd resources by removing the suspend annotation.",
		Example: resumeExample,
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := cmdCtx.Options
			runtime := cmdCtx.Runtime

			resumeOpts := newResumeReconcileOptions(opts)
			resumeRuntime := newResumeReconcileRuntime(runtime)
			ctx := &resumeReconcileCmdCtx{
				resumeReconcileOptions: resumeOpts,
				resumeReconcileRuntime: resumeRuntime,
			}

			if err := ctx.validate(); err != nil {
				runtime.Logger.Error(runtime.IOStreams.ErrOut, "Resuming reconciliation validation failed", err)
				if herr := cmd.Help(); herr != nil {
					runtime.Logger.Warning(runtime.IOStreams.ErrOut, "Failed to show help: ", herr.Error())
				}
				return err
			}

			if err := ctx.complete(); err != nil {
				return err
			}

			if opts.Verbose {
				if opts.AllNamespaces {
					runtime.Logger.Info(runtime.IOStreams.Out, "Resuming reconciliation for Etcd resources across all namespaces")
				} else {
					runtime.Logger.Info(runtime.IOStreams.Out, "Resuming reconciliation for selected Etcd resources")
				}
			}

			if err := ctx.execute(cmdutils.CmdContext(cmd)); err != nil {
				runtime.Logger.Error(runtime.IOStreams.ErrOut, "Resuming reconciliation failed", err)
				return err
			}
			return nil
		},
	}

	return cmd
}
