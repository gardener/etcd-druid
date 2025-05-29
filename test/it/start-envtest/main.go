// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/etcd-druid/test/it/setup"

	"github.com/go-logr/logr"
	flag "github.com/spf13/pflag"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	kubeConfigPath string
)

func main() {
	ctx := ctrl.SetupSignalHandler()
	logger := utils.MustNewLogger(true, druidconfigv1alpha1.LogLevelInfo, druidconfigv1alpha1.LogFormatText)
	logf.SetLogger(logger)
	logger = logf.Log.WithName("start-envtest")

	addFlags(flag.CommandLine)
	flag.Parse()

	envCloser, err := launchEnvTest(logger)
	if err != nil {
		logger.Error(err, "failed to launch envtest")
		os.Exit(1)
	}
	defer func() {
		logger.Info("Shutting down test environment")
		envCloser()
	}()
	logger.Info("Test environment ready!")
	<-ctx.Done()
}

func addFlags(fs *flag.FlagSet) {
	configDir := os.TempDir()
	defaultKubeConfigPath := filepath.Join(configDir, "envtest-kubeconfig.yaml")
	fs.StringVar(&kubeConfigPath, "kubeconfig", defaultKubeConfigPath, "Path to the kubeconfig file to use for the test environment")
}

func launchEnvTest(logger logr.Logger) (setup.DruidTestEnvCloser, error) {
	logger.Info("Starting test environment")
	druidEnv, druidEnvCloser, err := setup.NewDruidTestEnvironment("start-envtest", []string{})
	if err != nil {
		return nil, fmt.Errorf("failed to create integration test environment: %w", err)
	}
	k8sTestEnv := druidEnv.GetEnvTestEnvironment()
	adminUser, err := k8sTestEnv.ControlPlane.AddUser(envtest.User{
		Name:   "envtest-admin",
		Groups: []string{"system:masters"},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to add envtest-admin user: %w", err)
	}
	kubeConfigBytes, err := adminUser.KubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig for envtest-admin user: %w", err)
	}
	if err := os.WriteFile(kubeConfigPath, kubeConfigBytes, 0600); err != nil {
		return nil, fmt.Errorf("failed to write kubeconfig file: %w", err)
	}
	logger.Info("Successfully written kubeconfig", "path", kubeConfigPath)
	return druidEnvCloser, nil
}
