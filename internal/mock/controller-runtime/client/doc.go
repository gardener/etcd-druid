// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

//go:generate mockgen -package client -destination=mocks.go sigs.k8s.io/controller-runtime/pkg/client Client,StatusWriter,Reader,Writer
package client
