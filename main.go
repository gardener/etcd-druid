// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"os"

	"go.uber.org/zap/zapcore"

	"github.com/gardener/etcd-druid/controllers"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	// +kubebuilder:scaffold:imports
)

var logger = ctrl.Log.WithName("druid")

func main() {
	//var (
	//	controllerManagerConfig             = config.NewControllerManagerConfig()
	//	etcdControllerConfig                = config.NewEtcdControllerConfig()
	//	custodianControllerConfig           = config.NewCustodianControllerConfig()
	//	compactionLeaseControllerConfig     = config.NewCompactionLeaseControllerConfig()
	//	secretControllerConfig              = config.NewSecretControllerConfig()
	//	etcdCopyBackupsTaskControllerConfig = config.NewEtcdCopyBackupsTaskControllerConfig()
	//)
	//
	//flags.AddAllFlags(
	//	&controllerManagerConfig,
	//	&etcdControllerConfig,
	//	&custodianControllerConfig,
	//	&compactionLeaseControllerConfig,
	//	&secretControllerConfig,
	//	&etcdCopyBackupsTaskControllerConfig,
	//)

	ctx := ctrl.SetupSignalHandler()
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgrConfig := controllers.ManagerConfig{}
	controllers.InitFromFlags(flag.CommandLine, &mgrConfig)

	flag.Parse()
	printFlags(logger)

	mgr, err := controllers.CreateAndAddToManager(&mgrConfig)
	if err != nil {
		logger.Error(err, "failed to create druid controller manager")
		os.Exit(1)
	}

	//etcd, err := controllers.NewEtcdReconcilerWithImageVector(mgr, controllerManagerConfig.DisableEtcdServiceAccountAutomount)
	//if err != nil {
	//	logger.Error(err, "Unable to initialize etcd controller with image vector")
	//	os.Exit(1)
	//}
	//
	//if err := etcd.SetupWithManager(mgr, etcdControllerConfig.Workers, controllerManagerConfig.IgnoreOperationAnnotation); err != nil {
	//	logger.Error(err, "Unable to create controller", "Controller", "Etcd")
	//	os.Exit(1)
	//}
	//
	//custodian := controllers.NewEtcdCustodian(mgr, custodianControllerConfig)
	//
	//if err := custodian.SetupWithManager(ctx, mgr, custodianControllerConfig.Workers, controllerManagerConfig.IgnoreOperationAnnotation); err != nil {
	//	logger.Error(err, "Unable to create controller", "Controller", "Etcd Custodian")
	//	os.Exit(1)
	//}
	//
	//lc, err := controllers.NewCompactionLeaseControllerWithImageVector(mgr, compactionLeaseControllerConfig)
	//if err != nil {
	//	logger.Error(err, "Unable to initialize lease controller")
	//	os.Exit(1)
	//}
	//
	//if err := lc.SetupWithManager(mgr, compactionLeaseControllerConfig.Workers); err != nil {
	//	logger.Error(err, "Unable to create controller", "Controller", "Lease")
	//	os.Exit(1)
	//}
	//
	//secret := controllers.NewSecret(mgr)
	//
	//if err := secret.SetupWithManager(mgr, secretControllerConfig.Workers); err != nil {
	//	logger.Error(err, "Unable to create controller", "Controller", "Secret")
	//	os.Exit(1)
	//}
	//
	//etcdCopyBackupsTask, err := controllers.NewEtcdCopyBackupsTaskReconcilerWithImageVector(mgr)
	//if err != nil {
	//	logger.Error(err, "Unable to initialize controller with image vector")
	//	os.Exit(1)
	//}
	//
	//if err := etcdCopyBackupsTask.SetupWithManager(mgr, etcdControllerConfig.Workers); err != nil {
	//	logger.Error(err, "Unable to create controller", "Controller", "EtcdCopyBackupsTask")
	//}

	// +kubebuilder:scaffold:builder

	logger.Info("Starting manager")
	if err := mgr.Start(ctx); err != nil {
		logger.Error(err, "Error running manager")
		os.Exit(1)
	}
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

func printFlags(logger logr.Logger) {
	var flagsToPrint string
	flag.VisitAll(func(f *flag.Flag) {
		flagsToPrint += fmt.Sprintf("%s: %s, ", f.Name, f.Value)
	})

	logger.Info(fmt.Sprintf("Running with flags: %s", flagsToPrint[:len(flagsToPrint)-2]))
}
