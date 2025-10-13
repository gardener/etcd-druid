// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	"github.com/gardener/etcd-druid/cmd/opts"
	druidmgr "github.com/gardener/etcd-druid/internal/manager"
	"github.com/gardener/etcd-druid/internal/utils"
	druidversion "github.com/gardener/etcd-druid/internal/version"

	flag "github.com/spf13/pflag"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	logger = ctrl.Log.WithName("druid")
)

func main() {
	ctx := ctrl.SetupSignalHandler()

	cliFs := flag.CommandLine
	druidversion.AddFlags(cliFs)
	// This is required when moving from v0.22 to v0.23. Some command line flags have been renamed in v0.23.
	// If the entity which deploys druid needs to handle both versions then in the caller both old and new flags can be set.
	// Older version of druid will only work with older flags while ignoring the newer ones as unknowns. It is going to be a similar case for newer versions of druid as well.
	// Once we stabilise the command line arguments then this will no longer be needed.
	cliFs.ParseErrorsWhitelist.UnknownFlags = true
	cliOpts := opts.NewCLIOptions(cliFs, logger)

	// parse and print command line flags
	flag.Parse()
	druidversion.PrintVersionAndExitIfRequested()

	operatorConfig, err := initializeAndGetOperatorConfig(cliOpts)
	if err != nil {
		logger.Error(err, "failed to initialize operator configuration")
		os.Exit(1)
	}

	ctrl.SetLogger(utils.MustNewLogger(false, operatorConfig.Logging.LogLevel, operatorConfig.Logging.LogFormat))
	printRuntimeInfo()
	logger.Info("Using operator configuration", "config", *operatorConfig)

	logger.Info("Initializing etcd-druid controller manager")
	mrg, err := druidmgr.InitializeManager(operatorConfig)
	if err != nil {
		logger.Error(err, "failed to create druid controller manager")
		os.Exit(1)
	}
	logger.Info("Starting etcd-druid controller manager")
	if err = mrg.Start(ctx); err != nil {
		logger.Error(err, "failed to start druid controller manager")
		os.Exit(1)
	}
}

func initializeAndGetOperatorConfig(cliOpts *opts.CLIOptions) (*druidconfigv1alpha1.OperatorConfiguration, error) {
	if err := cliOpts.Complete(); err != nil {
		return nil, fmt.Errorf("failed to complete cli options: %w", err)
	}
	if err := cliOpts.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate cli options: %w", err)
	}
	return cliOpts.Config, nil
}

func printRuntimeInfo() {
	versionInfo := druidversion.Get()
	logger.Info("etcd-druid runtime info",
		"version", versionInfo.String(),
		"go Version", versionInfo.GoVersion,
		"platform", versionInfo.Platform,
		"git commit", versionInfo.GitCommit,
		"git tree state", versionInfo.GitTreeState,
		"build date", versionInfo.BuildDate,
	)
}
