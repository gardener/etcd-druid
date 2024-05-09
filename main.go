// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"runtime"

	"github.com/go-logr/logr"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/gardener/etcd-druid/internal/controller"
	"github.com/gardener/etcd-druid/internal/version"
	flag "github.com/spf13/pflag"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	// +kubebuilder:scaffold:imports
)

var logger = ctrl.Log.WithName("druid")

func main() {
	ctx := ctrl.SetupSignalHandler()

	ctrl.SetLogger(zap.New(buildDefaultLoggerOpts()...))

	printVersionInfo()

	mgrConfig := controller.ManagerConfig{}
	if err := mgrConfig.InitFromFlags(flag.CommandLine); err != nil {
		logger.Error(err, "failed to initialize from flags")
		os.Exit(1)
	}

	flag.Parse()

	printFlags(logger)

	if err := mgrConfig.Validate(); err != nil {
		logger.Error(err, "validation of manager config failed")
		os.Exit(1)
	}

	mgr, err := controller.CreateManagerWithControllersAndWebhooks(&mgrConfig)
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

func printVersionInfo() {
	logger.Info("Etcd-druid build information", "Etcd-druid Version", version.Version, "Git SHA", version.GitSHA)
	logger.Info("Golang runtime information", "Version", runtime.Version(), "OS", runtime.GOOS, "Arch", runtime.GOARCH)
}

func printFlags(logger logr.Logger) {
	var flagKVs []interface{}
	flag.VisitAll(func(f *flag.Flag) {
		flagKVs = append(flagKVs, f.Name, f.Value.String())
	})

	logger.Info("Running with flags", flagKVs...)
}

func buildDefaultLoggerOpts() []zap.Opts {
	var opts []zap.Opts
	opts = append(opts, zap.UseDevMode(false))
	opts = append(opts, zap.JSONEncoder(func(encoderConfig *zapcore.EncoderConfig) {
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		encoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	}))
	return opts
}
