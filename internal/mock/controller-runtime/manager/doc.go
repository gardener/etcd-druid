// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

//go:generate mockgen -package manager -destination=mocks.go sigs.k8s.io/controller-runtime/pkg/manager Manager
package manager
