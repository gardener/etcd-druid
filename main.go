// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	druidmgr "github.com/gardener/etcd-druid/internal/manager"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/etcd-druid/internal/version"
	"github.com/go-logr/logr"
	flag "github.com/spf13/pflag"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"os"
	"runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	// +kubebuilder:scaffold:imports
)

var logger = ctrl.Log.WithName("druid")

func main() {
	ctx := ctrl.SetupSignalHandler()
	ctrl.SetLogger(utils.MustNewLogger(false, utils.LogFormatJSON))
	printVersionInfo()

	mgrConfig, err := initializeManagerConfig()
	if err != nil {
		logger.Error(err, "failed to initialize manager config")
		os.Exit(1)
	}
	if err = mgrConfig.Validate(); err != nil {
		logger.Error(err, "validation of manager config failed")
		os.Exit(1)
	}
	mgr, err := druidmgr.InitializeManager(&mgrConfig)
	if err != nil {
		logger.Error(err, "failed to create druid controller manager")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	logger.Info("Starting manager")
	if err := mgr.Start(ctx); err != nil {
		logger.Error(err, "Error running manager")
		os.Exit(1)
	}
}

func initializeManagerConfig() (druidmgr.Config, error) {
	mgrConfig := druidmgr.Config{}
	cmdLineFlagSet := flag.CommandLine
	// This is required when moving from v0.22 to v0.23. Some command line flags have been renamed in v0.23.
	// If the entity which deploys druid needs to handle both versions then in the caller both old and new flags can be set.
	// Older version of druid will only work with older flags while ignoring the newer ones as unknowns. It is going to be a similar case for newer versions of druid as well.
	// Once we stabilise the command line arguments then this will no longer be needed.
	cmdLineFlagSet.ParseErrorsWhitelist.UnknownFlags = true
	if err := mgrConfig.InitFromFlags(cmdLineFlagSet); err != nil {
		logger.Error(err, "failed to initialize from flags")
		return druidmgr.Config{}, err
	}

	if err := cmdLineFlagSet.Parse(os.Args[1:]); err != nil {
		logger.Error(err, "failed to parse command line flags")
		return druidmgr.Config{}, err
	}
	printFlags(cmdLineFlagSet, logger)
	return mgrConfig, nil
}

func printVersionInfo() {
	logger.Info("Etcd-druid build information", "Etcd-druid Version", version.Version, "Git SHA", version.GitSHA)
	logger.Info("Golang runtime information", "Version", runtime.Version(), "OS", runtime.GOOS, "Arch", runtime.GOARCH)
}

func printFlags(fs *flag.FlagSet, logger logr.Logger) {
	var flagKVs []interface{}
	fs.VisitAll(func(f *flag.Flag) {
		flagKVs = append(flagKVs, f.Name, f.Value.String())
	})
	logger.Info("Running with flags", flagKVs...)
}
