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
	"os"

	"go.uber.org/zap/zapcore"

	"github.com/gardener/etcd-druid/controllers"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	config "github.com/gardener/etcd-druid/pkg/config"
	"github.com/gardener/etcd-druid/pkg/flags"

	coordinationv1 "k8s.io/api/coordination/v1"
	coordinationv1beta1 "k8s.io/api/coordination/v1beta1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var setupLog = ctrl.Log.WithName("setup")

func main() {
	var (
		controllerManagerConfig             = config.NewControllerManagerConfig()
		etcdControllerConfig                = config.NewEtcdControllerConfig()
		custodianControllerConfig           = config.NewCustodianControllerConfig()
		compactionLeaseControllerConfig     = config.NewCompactionLeaseControllerConfig()
		secretControllerConfig              = config.NewSecretControllerConfig()
		etcdCopyBackupsTaskControllerConfig = config.NewEtcdCopyBackupsTaskControllerConfig()
	)

	flags.AddAllFlags(
		&controllerManagerConfig,
		&etcdControllerConfig,
		&custodianControllerConfig,
		&compactionLeaseControllerConfig,
		&secretControllerConfig,
		&etcdCopyBackupsTaskControllerConfig,
	)

	flags.ParseFlags()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	flags.PrintFlags(setupLog)

	ctx := ctrl.SetupSignalHandler()

	// TODO: this can be removed once we have an improved informer, see https://github.com/gardener/etcd-druid/issues/215
	uncachedObjects := controllers.UncachedObjectList
	if controllerManagerConfig.DisableLeaseCache {
		uncachedObjects = append(uncachedObjects, &coordinationv1.Lease{}, &coordinationv1beta1.Lease{})
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		ClientDisableCacheFor:      uncachedObjects,
		Scheme:                     kubernetes.Scheme,
		MetricsBindAddress:         controllerManagerConfig.MetricsAddr,
		LeaderElection:             controllerManagerConfig.EnableLeaderElection,
		LeaderElectionID:           controllerManagerConfig.LeaderElectionID,
		LeaderElectionResourceLock: controllerManagerConfig.LeaderElectionResourceLock,
	})
	if err != nil {
		setupLog.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	etcd, err := controllers.NewEtcdReconcilerWithImageVector(mgr, controllerManagerConfig.DisableEtcdServiceAccountAutomount)
	if err != nil {
		setupLog.Error(err, "Unable to initialize etcd controller with image vector")
		os.Exit(1)
	}

	if err := etcd.SetupWithManager(mgr, etcdControllerConfig.Workers, controllerManagerConfig.IgnoreOperationAnnotation); err != nil {
		setupLog.Error(err, "Unable to create controller", "Controller", "Etcd")
		os.Exit(1)
	}

	custodian := controllers.NewEtcdCustodian(mgr, custodianControllerConfig)

	if err := custodian.SetupWithManager(ctx, mgr, custodianControllerConfig.Workers, controllerManagerConfig.IgnoreOperationAnnotation); err != nil {
		setupLog.Error(err, "Unable to create controller", "Controller", "Etcd Custodian")
		os.Exit(1)
	}

	lc, err := controllers.NewCompactionLeaseControllerWithImageVector(mgr, compactionLeaseControllerConfig)
	if err != nil {
		setupLog.Error(err, "Unable to initialize lease controller")
		os.Exit(1)
	}

	if err := lc.SetupWithManager(mgr, compactionLeaseControllerConfig.Workers); err != nil {
		setupLog.Error(err, "Unable to create controller", "Controller", "Lease")
		os.Exit(1)
	}

	secret := controllers.NewSecret(mgr)

	if err := secret.SetupWithManager(mgr, secretControllerConfig.Workers); err != nil {
		setupLog.Error(err, "Unable to create controller", "Controller", "Secret")
		os.Exit(1)
	}

	etcdCopyBackupsTask, err := controllers.NewEtcdCopyBackupsTaskReconcilerWithImageVector(mgr)
	if err != nil {
		setupLog.Error(err, "Unable to initialize controller with image vector")
		os.Exit(1)
	}

	if err := etcdCopyBackupsTask.SetupWithManager(mgr, etcdControllerConfig.Workers); err != nil {
		setupLog.Error(err, "Unable to create controller", "Controller", "EtcdCopyBackupsTask")
	}

	// +kubebuilder:scaffold:builder

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "Error running manager")
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
