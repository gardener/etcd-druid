// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	schemev1 "k8s.io/client-go/kubernetes/scheme"
)

// Scheme  is the kubernetes runtime scheme
var Scheme = runtime.NewScheme()

func init() {
	localSchemeBuilder := runtime.NewSchemeBuilder(
		druidv1alpha1.AddToScheme,
		schemev1.AddToScheme,
	)

	utilruntime.Must(localSchemeBuilder.AddToScheme(Scheme))
}
