// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import "sigs.k8s.io/controller-runtime/pkg/client"

// patchFn returns a client.Patch with the given client.Object as the base object.
type patchFn func(client.Object, ...client.MergeFromOption) client.Patch

func mergeFrom(obj client.Object, opts ...client.MergeFromOption) client.Patch {
	return client.MergeFromWithOptions(obj, opts...)
}

func mergeFromWithOptimisticLock(obj client.Object, opts ...client.MergeFromOption) client.Patch {
	return client.MergeFromWithOptions(obj, append(opts, client.MergeFromWithOptimisticLock{})...)
}
