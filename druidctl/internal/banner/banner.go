// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package banner

import (
	"os"
	"strings"

	"github.com/gardener/etcd-druid/druidctl/internal/log"
	"github.com/gardener/etcd-druid/druidctl/internal/version"

	"github.com/spf13/cobra"
)

var asciiArt = `
▶  ██████╗ ██████╗ ██╗   ██╗██╗██████╗  ██████╗████████╗██╗     
▶  ██╔══██╗██╔══██╗██║   ██║██║██╔══██╗██╔════╝╚══██╔══╝██║     
▶  ██║  ██║██████╔╝██║   ██║██║██║  ██║██║        ██║   ██║     
▶  ██║  ██║██╔══██╗██║   ██║██║██║  ██║██║        ██║   ██║     
▶  ██████╔╝██║  ██║╚██████╔╝██║██████╔╝╚██████╗   ██║   ███████╗
▶  ╚═════╝ ╚═╝  ╚═╝ ╚═════╝ ╚═╝╚═════╝  ╚═════╝   ╚═╝   ╚══════╝
`

// ShowBanner renders the CLI banner when appropriate based on the current command and flags.
func ShowBanner(rootCmd, cmd *cobra.Command, disableBanner bool) {
	if disableBanner {
		return
	}

	shouldShow := false

	if cmd.Flags().Changed("help") || cmd.Flags().Changed("h") {
		shouldShow = true
	} else if rootCmd == cmd {
		shouldShow = true
	} else if !cmd.HasParent() {
		shouldShow = true
	} else if cmd.Name() == "help" {
		shouldShow = true
	}

	if !shouldShow {
		return
	}

	logger := log.NewLogger(log.LogTypeCharm)
	lines := strings.Split(strings.TrimSpace(asciiArt), "\n")
	for _, line := range lines {
		logger.RawHeader(os.Stdout, line)
	}

	versionInfo := version.Get()
	logger.RawHeader(os.Stdout, "Version: "+versionInfo.String())

	// Optional: Show development build warning in banner
	if !versionInfo.IsRelease() {
		logger.RawHeader(os.Stdout, "⚠️  Development Build")
	}
}
