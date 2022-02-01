/*
Copyright (c) SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by lister-gen. DO NOT EDIT.

package internalversion

import (
	core "github.com/gardener/gardener/pkg/apis/core"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// ControllerInstallationLister helps list ControllerInstallations.
// All objects returned here must be treated as read-only.
type ControllerInstallationLister interface {
	// List lists all ControllerInstallations in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*core.ControllerInstallation, err error)
	// Get retrieves the ControllerInstallation from the index for a given name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*core.ControllerInstallation, error)
	ControllerInstallationListerExpansion
}

// controllerInstallationLister implements the ControllerInstallationLister interface.
type controllerInstallationLister struct {
	indexer cache.Indexer
}

// NewControllerInstallationLister returns a new ControllerInstallationLister.
func NewControllerInstallationLister(indexer cache.Indexer) ControllerInstallationLister {
	return &controllerInstallationLister{indexer: indexer}
}

// List lists all ControllerInstallations in the indexer.
func (s *controllerInstallationLister) List(selector labels.Selector) (ret []*core.ControllerInstallation, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*core.ControllerInstallation))
	})
	return ret, err
}

// Get retrieves the ControllerInstallation from the index for a given name.
func (s *controllerInstallationLister) Get(name string) (*core.ControllerInstallation, error) {
	obj, exists, err := s.indexer.GetByKey(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(core.Resource("controllerinstallation"), name)
	}
	return obj.(*core.ControllerInstallation), nil
}
