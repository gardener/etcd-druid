// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"os"

	"github.com/gardener/etcd-druid/druidctl/internal/client"
	"github.com/gardener/etcd-druid/druidctl/internal/log"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

// RuntimeEnv holds runtime dependencies that are not user-configurable options.
// These are infrastructure components needed to execute commands.
type RuntimeEnv struct {
	// Logger is the logger instance for the CLI
	Logger log.Logger
	// IOStreams provides standard input/output/error streams
	IOStreams genericiooptions.IOStreams
	// Clients provides lazy-loaded Kubernetes clients
	Clients *ClientBundle
}

// NewRuntimeEnv creates a new RuntimeEnv with default values.
// The logger is created based on the provided loggerKind.
func NewRuntimeEnv(configFlags *genericclioptions.ConfigFlags, loggerKind log.LoggerKind) *RuntimeEnv {
	factory := client.NewClientFactory(configFlags)
	return &RuntimeEnv{
		Logger:    log.NewLogger(loggerKind),
		IOStreams: genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr},
		Clients:   NewClientBundle(factory),
	}
}

// SetVerbose configures the logger's verbosity level.
func (r *RuntimeEnv) SetVerbose(verbose bool) {
	r.Logger.SetVerbose(verbose)
}
