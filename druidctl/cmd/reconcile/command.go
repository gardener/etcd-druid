package reconcile

import (
	"context"
	"fmt"
	"strings"
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
		druidctl reconcile 'test/my-etcd'

		# Reconcile all Etcd resources in the test namespace
		druidctl reconcile 'test/*'

		# Reconcile different Etcd resources in different namespaces
		druidctl reconcile 'test/my-etcd,dev/my-etcd'

		# Reconcile all Etcd resources across all namespaces
		druidctl reconcile --all-namespaces(-A)

		# Reconcile an Etcd resource named "my-etcd" in the test namespace and wait until it's ready with default timeout
		druidctl reconcile 'test/my-etcd' --wait-till-ready

		# Reconcile an Etcd resource named "my-etcd" in the test namespace with a custom timeout
		druidctl reconcile 'test/my-etcd' --wait-till-ready --timeout=10m
		
		# Reconcile an Etcd resource named "my-etcd" in the test namespace and watch until it's ready, i.e, --wait-till-ready with indefinite timeout
		druidctl reconcile 'test/my-etcd' --watch(-W)`

	suspendReconcileExample = `
		# Suspend reconciliation for an Etcd resource named "my-etcd" in the test namespace
		druidctl suspend-reconcile 'test/my-etcd'

		# Suspend reconciliation for all Etcd resources in the test namespace
		druidctl suspend-reconcile 'test/*'
		
		# Suspend reconciliation for different Etcd resources in different namespaces
		druidctl suspend-reconcile 'test/my-etcd,dev/my-etcd'

		# Suspend reconciliation for all Etcd resources in all namespaces
		druidctl suspend-reconcile --all-namespaces`

	resumeReconcileExample = `
		# Resume reconciliation for an Etcd resource named "my-etcd" in the test namespace
		druidctl resume-reconcile 'test/my-etcd'

		# Resume reconciliation for all Etcd resources in the test namespace
		druidctl resume-reconcile 'test/*'

		# Resume reconciliation for different Etcd resources in different namespaces
		druidctl resume-reconcile 'test/my-etcd,dev/my-etcd'

		# Resume reconciliation for all Etcd resources in all namespaces
		druidctl resume-reconcile --all-namespaces`
)

// group the Use, Short, Long and Example for the reconcile commands into a structure
type reconcileCommandInfo struct {
	use     string
	short   string
	long    string
	example string
}

func newReconcileBaseCommand(
	cmdInfo *reconcileCommandInfo,
	options *cmdutils.GlobalOptions,
	reconcileCmdCtx func(*cmdutils.GlobalOptions) reconcileCmdCtxInterface,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:     cmdInfo.use,
		Short:   cmdInfo.short,
		Long:    cmdInfo.long,
		Example: cmdInfo.example,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reconcileCmdCtx := reconcileCmdCtx(options)
			if err := reconcileCmdCtx.validate(); err != nil {
				options.Logger.Error(options.IOStreams.ErrOut, fmt.Sprintf("%s validation failed", getOperationName(cmdInfo.use)), err)
				cmd.Help()
				return err
			}

			if err := reconcileCmdCtx.complete(options); err != nil {
				return err
			}

			if options.AllNamespaces {
				options.Logger.Info(options.IOStreams.Out, fmt.Sprintf("%s Etcd resources across all namespaces", getOperationName(cmdInfo.use)))
			} else {
				options.Logger.Info(options.IOStreams.Out, fmt.Sprintf("%s for selected Etcd resources", getOperationName(cmdInfo.use)))
			}

			if err := reconcileCmdCtx.execute(context.TODO()); err != nil {
				options.Logger.Error(options.IOStreams.ErrOut, fmt.Sprintf("%s failed", getOperationName(cmdInfo.use)), err)
				return err
			}
			options.Logger.Success(options.IOStreams.Out, fmt.Sprintf("%s completed successfully", getOperationName(cmdInfo.use)))
			return nil
		},
	}
	return cmd
}

// NewReconcileCommand creates the 'reconcile' command
func NewReconcileCommand(options *cmdutils.GlobalOptions) *cobra.Command {
	var waitTillReady bool
	var watch bool
	var timeout time.Duration = defaultTimeout

	cmdInfo := &reconcileCommandInfo{
		use:     "reconcile <etcd-resource-name> --wait-till-ready(optional flag) --timeout(optional flag)",
		short:   "Reconcile the mentioned etcd resource",
		long:    "Reconcile the mentioned etcd resource. If the flag --wait-till-ready is set, then reconcile only after the Etcd CR is considered ready",
		example: reconcileExample,
	}

	reconcileCmd := newReconcileBaseCommand(
		cmdInfo,
		options,
		func(options *cmdutils.GlobalOptions) reconcileCmdCtxInterface {
			reconcileOptions := newReconcileOptions(options, waitTillReady, watch, timeout)
			return &reconcileCmdCtx{
				reconcileOptions: reconcileOptions,
			}
		},
	)

	// Add command-specific flags
	reconcileCmd.Flags().BoolVarP(&waitTillReady, "wait-till-ready", "w", false,
		"Wait until the Etcd resource is ready before reconciling")
	reconcileCmd.Flags().BoolVarP(&watch, "watch", "W", false,
		"Watch the Etcd resources until they are ready after reconciliation")
	reconcileCmd.Flags().DurationVarP(&timeout, "timeout", "t", defaultTimeout,
		"Timeout for the reconciliation process")

	return reconcileCmd
}

// NewSuspendReconcileCommand creates a new 'suspend-reconcile' command.
func NewSuspendReconcileCommand(options *cmdutils.GlobalOptions) *cobra.Command {
	cmdInfo := &reconcileCommandInfo{
		use:     "suspend-reconcile <etcd-resource-name>",
		short:   "Suspend reconciliation for the mentioned etcd resource",
		long:    "Suspend reconciliation for the mentioned etcd resource.",
		example: suspendReconcileExample,
	}
	suspendReconcileCmd := newReconcileBaseCommand(
		cmdInfo,
		options,
		func(options *cmdutils.GlobalOptions) reconcileCmdCtxInterface {
			suspendReconcileOptions := newSuspendReconcileOptions(options)
			return &suspendReconcileCmdCtx{
				suspendReconcileOptions: suspendReconcileOptions,
			}
		},
	)

	return suspendReconcileCmd
}

// NewResumeReconcileCommand creates a new 'resume-reconcile' command.
func NewResumeReconcileCommand(options *cmdutils.GlobalOptions) *cobra.Command {
	cmdInfo := &reconcileCommandInfo{
		use:     "resume-reconcile <etcd-resource-name>",
		short:   "Resume reconciliation for the mentioned etcd resource",
		long:    "Resume reconciliation for the mentioned etcd resource.",
		example: resumeReconcileExample,
	}
	resumeReconcileCmd := newReconcileBaseCommand(
		cmdInfo,
		options,
		func(options *cmdutils.GlobalOptions) reconcileCmdCtxInterface {
			resumeReconcileOptions := newResumeReconcileOptions(options)
			return &resumeReconcileCmdCtx{
				resumeReconcileOptions: resumeReconcileOptions,
			}
		},
	)

	return resumeReconcileCmd
}

func getOperationName(commandUse string) string {
	command := strings.Split(commandUse, " ")[0]
	switch command {
	case "suspend-reconcile":
		return "Suspending reconciliation"
	case "resume-reconcile":
		return "Resuming reconciliation"
	case "reconcile":
		return "Reconciling"
	}
	return "Processing"
}
