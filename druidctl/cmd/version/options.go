// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package version

import (
	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
)

type versionOptions struct {
	*cmdutils.GlobalOptions
	output string
	short  bool
}

func newVersionOptions(globalOpts *cmdutils.GlobalOptions, output string, short bool) *versionOptions {
	return &versionOptions{
		GlobalOptions: globalOpts,
		output:        output,
		short:         short,
	}
}
