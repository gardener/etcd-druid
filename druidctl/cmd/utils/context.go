// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"github.com/gardener/etcd-druid/druidctl/internal/log"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// CommandContext is the top-level context passed to all commands.
// It composes GlobalOptions (user-configurable flags) and RuntimeEnv (infrastructure dependencies).
type CommandContext struct {
	// Options contains all user-configurable CLI options and flags
	Options *GlobalOptions
	// Runtime contains infrastructure dependencies (logger, IO, clients)
	Runtime *RuntimeEnv
}

// NewCommandContext creates a new CommandContext with default values.
func NewCommandContext() *CommandContext {
	configFlags := genericclioptions.NewConfigFlags(true)
	return &CommandContext{
		Options: newGlobalOptions(configFlags),
		Runtime: NewRuntimeEnv(configFlags, log.LoggerKindCharm),
	}
}

// AddFlags adds all global flags to the specified command.
func (c *CommandContext) AddFlags(cmd *cobra.Command) {
	c.Options.AddFlags(cmd)
}

// Complete fills in the CommandContext based on command line args and flags.
// This should be called after the flags are parsed.
func (c *CommandContext) Complete(cmd *cobra.Command, args []string) error {
	if err := c.Options.Complete(cmd, args); err != nil {
		return err
	}
	// Configure runtime based on options
	c.Runtime.SetVerbose(c.Options.Verbose)
	return nil
}
