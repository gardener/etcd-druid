// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/gardener/etcd-druid/druidctl/cmd/listresources"
	"github.com/gardener/etcd-druid/druidctl/cmd/reconcile"
	"github.com/gardener/etcd-druid/druidctl/cmd/resourceprotection"
	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
	"github.com/gardener/etcd-druid/druidctl/internal/banner"

	"github.com/spf13/cobra"
)

// Global options instance
var options *cmdutils.GlobalOptions

var rootCmd = &cobra.Command{
	Use:   "druid [command] [resource] [flags]",
	Short: "CLI for etcd-druid operator",
	Long:  `This is a command line interface for Druid. It allows you to interact with Druid using various commands and flags.`,
	Run: func(cmd *cobra.Command, _ []string) {
		if options.Verbose {
			cmd.Println("Verbose mode enabled")
		}
		if err := cmd.Help(); err != nil {
			options.Logger.Warning(options.IOStreams.ErrOut, "Failed to show help: ", err.Error())
		}
	},
}

// Execute runs the root command
func Execute() error {
	options = cmdutils.NewOptions()
	options.AddFlags(rootCmd)

	originalPreRun := rootCmd.PersistentPreRun
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		banner.ShowBanner(rootCmd, cmd, options.DisableBanner)
		if err := options.Complete(cmd, args); err != nil {
			options.Logger.Error(options.IOStreams.ErrOut, "Completion failed: ", err)
			return err
		}
		if err := options.Validate(); err != nil {
			options.Logger.Error(options.IOStreams.ErrOut, "Validation failed: ", err)
			if _, werr := options.IOStreams.Out.Write([]byte("\n")); werr != nil {
				options.Logger.Warning(options.IOStreams.ErrOut, "Failed writing newline: ", werr.Error())
			}
			if herr := cmd.Help(); herr != nil {
				options.Logger.Warning(options.IOStreams.ErrOut, "Help display failed: ", herr.Error())
			}
			return err
		}
		if originalPreRun != nil {
			originalPreRun(cmd, args)
		}
		return nil
	}

	// Add subcommands
	rootCmd.AddCommand(reconcile.NewReconcileCommand(options))
	rootCmd.AddCommand(resourceprotection.NewAddProtectionCommand(options))
	rootCmd.AddCommand(resourceprotection.NewRemoveProtectionCommand(options))
	rootCmd.AddCommand(reconcile.NewSuspendReconcileCommand(options))
	rootCmd.AddCommand(reconcile.NewResumeReconcileCommand(options))
	rootCmd.AddCommand(listresources.NewListResourcesCommand(options))
	return rootCmd.Execute()
}
