// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/gardener/etcd-druid/druidctl/cmd/listresources"
	"github.com/gardener/etcd-druid/druidctl/cmd/reconciliation"
	"github.com/gardener/etcd-druid/druidctl/cmd/resourceprotection"
	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
	versioncmd "github.com/gardener/etcd-druid/druidctl/cmd/version"
	"github.com/gardener/etcd-druid/druidctl/internal/banner"
	"github.com/gardener/etcd-druid/druidctl/internal/version"

	"github.com/spf13/cobra"
)

// NewDruidCommand creates the root druid command with all subcommands.
func NewDruidCommand() *cobra.Command {
	cmdCtx := cmdutils.NewCommandContext()

	rootCmd := &cobra.Command{
		Use:     "kubectl druid [command] [resource] [flags]",
		Short:   "CLI for etcd-druid operator",
		Long:    `This is a command line interface for Druid. It allows you to interact with Druid using various commands and flags.`,
		Version: version.Get().String(),
		Run: func(cmd *cobra.Command, _ []string) {
			if cmdCtx.Options.Verbose {
				cmd.Println("Verbose mode enabled")
			}
			if err := cmd.Help(); err != nil {
				cmdCtx.Runtime.Logger.Warning(cmdCtx.Runtime.IOStreams.ErrOut, "Failed to show help: ", err.Error())
			}
		},
	}

	// Customize the version template for --version flag
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	// Add global flags
	cmdCtx.AddFlags(rootCmd)

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

		banner.ShowBanner(rootCmd, cmd, cmdCtx.Options.DisableBanner)
		if err := cmdCtx.Complete(cmd, args); err != nil {
			cmdCtx.Runtime.Logger.Error(cmdCtx.Runtime.IOStreams.ErrOut, "Completion failed: ", err)
			return err
		}
		return nil
	}

	// Add subcommands
	rootCmd.AddCommand(versioncmd.NewVersionCommand(cmdCtx))
	rootCmd.AddCommand(reconciliation.NewReconciliationCommand(cmdCtx))
	rootCmd.AddCommand(resourceprotection.NewComponentProtectionCommand(cmdCtx))
	rootCmd.AddCommand(listresources.NewListResourcesCommand(cmdCtx))

	return rootCmd
}

// Execute creates and executes the root druid command.
func Execute() error {
	return NewDruidCommand().Execute()
}
