// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package utils

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	eventsv1beta1 "k8s.io/api/events/v1beta1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	crdDirectoryPath = filepath.Join("config", "crd", "bases")
)

// SetupTestEnvironment sets up the test environment for the manager, setting the CRD path relative
// to the caller directory level, denoted by callerDirLevel
func SetupTestEnvironment(callerDirLevel int) (*envtest.Environment, error) {
	pathPrefix := ""
	for i := 0; i < callerDirLevel; i++ {
		pathPrefix = filepath.Join(pathPrefix, "..")
	}
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join(pathPrefix, crdDirectoryPath)},
	}

	if _, err := testEnv.Start(); err != nil {
		return nil, err
	}

	return testEnv, nil
}

func GetManager(cfg *rest.Config) (manager.Manager, error) {
	uncachedObjects := []client.Object{
		&corev1.Event{},
		&eventsv1beta1.Event{},
		&eventsv1.Event{},
	}

	return manager.New(cfg, manager.Options{
		MetricsBindAddress:    "0",
		ClientDisableCacheFor: uncachedObjects,
	})
}

func StartManager(ctx context.Context, mgr manager.Manager) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		Expect(mgr.Start(ctx)).NotTo(HaveOccurred())
		wg.Done()
	}()
	syncCtx, syncCancel := context.WithTimeout(ctx, 1*time.Minute)
	defer syncCancel()
	mgr.GetCache().WaitForCacheSync(syncCtx)
	return wg
}

func StopManager(mgrCancel context.CancelFunc, mgrStopped *sync.WaitGroup, testEnv *envtest.Environment, revertFunc func()) {
	mgrCancel()
	mgrStopped.Wait()
	Expect(testEnv.Stop()).To(Succeed())
	revertFunc()
}

// SwitchDirectory sets the working directory and returns a function to revert to the previous one.
func SwitchDirectory(path string) func() {
	oldPath, err := os.Getwd()
	if err != nil {
		Expect(err).NotTo(HaveOccurred())
	}

	if err := os.Chdir(path); err != nil {
		Expect(err).NotTo(HaveOccurred())
	}

	return func() {
		if err := os.Chdir(oldPath); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
	}
}
