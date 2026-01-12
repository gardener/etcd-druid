// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/gardener/etcd-druid/druidctl/cmd/listresources"
	"github.com/gardener/etcd-druid/druidctl/cmd/reconcile"
	"github.com/gardener/etcd-druid/druidctl/cmd/resourceprotection"
	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
	versioncmd "github.com/gardener/etcd-druid/druidctl/cmd/version"
	"github.com/gardener/etcd-druid/druidctl/internal/banner"
	"github.com/gardener/etcd-druid/druidctl/internal/version"

	"github.com/spf13/cobra"
)

// NewDruidCommand creates the root druid command with all subcommands.
func NewDruidCommand() *cobra.Command {
	options := cmdutils.NewOptions()

	rootCmd := &cobra.Command{
		Use:     "druid [command] [resource] [flags]",
		Short:   "CLI for etcd-druid operator",
		Long:    `This is a command line interface for Druid. It allows you to interact with Druid using various commands and flags.`,
		Version: version.Get().String(),
		Run: func(cmd *cobra.Command, _ []string) {
			if options.Verbose {
				cmd.Println("Verbose mode enabled")
			}
			if err := cmd.Help(); err != nil {
				options.Logger.Warning(options.IOStreams.ErrOut, "Failed to show help: ", err.Error())
			}
		},
	}

	// Customize the version template for --version flag
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	// Add global flags
	options.AddFlags(rootCmd)

	// Setup persistent pre-run hook for completion and validation
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

		// Skip for shell completion commands as they don't need any setup
		cmdName := cmd.Name()
		if cmdName == cobra.ShellCompRequestCmd || cmdName == cobra.ShellCompNoDescRequestCmd ||
			cmdName == "completion" || cmdName == "bash" || cmdName == "zsh" ||
			cmdName == "fish" || cmdName == "powershell" {
			return nil
		}

		banner.ShowBanner(rootCmd, cmd, options.DisableBanner)
		if err := options.Complete(cmd, args); err != nil {
			options.Logger.Error(options.IOStreams.ErrOut, "Completion failed: ", err)
			return err
		}
		return nil
	}

	// Add subcommands
	rootCmd.AddCommand(versioncmd.NewVersionCommand(options))
	rootCmd.AddCommand(reconcile.NewReconcileCommand(options))
	rootCmd.AddCommand(resourceprotection.NewComponentProtectionCommand(options))
	rootCmd.AddCommand(listresources.NewListResourcesCommand(options))

	return rootCmd
}

// Execute creates and executes the root druid command.
func Execute() error {
	return NewDruidCommand().Execute()
}
