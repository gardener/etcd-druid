// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// FakeManager fakes a manager.Manager.
type FakeManager struct {
	manager.Manager
	Client        client.Client
	Cache         cache.Cache
	EventRecorder record.EventRecorder
	APIReader     client.Reader
	Logger        logr.Logger
	Scheme        *runtime.Scheme
}

func (f FakeManager) GetClient() client.Client {
	return f.Client
}

func (f FakeManager) GetCache() cache.Cache {
	return f.Cache
}

func (f FakeManager) GetEventRecorderFor(_ string) record.EventRecorder {
	return f.EventRecorder
}

func (f FakeManager) GetAPIReader() client.Reader {
	return f.APIReader
}

func (f FakeManager) GetLogger() logr.Logger {
	return f.Logger
}

func (f FakeManager) GetScheme() *runtime.Scheme {
	return f.Scheme
}
