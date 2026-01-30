// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"encoding/json"
	"fmt"

	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
	"github.com/gardener/etcd-druid/druidctl/internal/version"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

var (
	versionExample = `
# Print version information
kubectl druid version

# Print version in JSON format
kubectl druid version --output json

# Print only the version number
kubectl druid version --short`
)

// NewVersionCommand creates the version command
func NewVersionCommand(globalOpts *cmdutils.GlobalOptions) *cobra.Command {
	opts := newVersionOptions(globalOpts, "", false)

	cmd := &cobra.Command{
		Use:     "version",
		Short:   "Print version information",
		Long:    `Print the version information for druidctl.`,
		Example: versionExample,
		// Skip the persistent pre-run hook for version command
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// Don't validate or complete for version command
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			return nil
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			return runVersion(globalOpts, opts)
		},
	}

	cmd.Flags().StringVarP(&opts.output, "output", "o", "", "Output format. One of: json|yaml")
	cmd.Flags().BoolVar(&opts.short, "short", false, "Print just the version number")

	return cmd
}

func runVersion(globalOpts *cmdutils.GlobalOptions, opts *versionOptions) error {
	versionInfo := version.Get()

	if opts.short {
		_, _ = fmt.Fprintln(globalOpts.IOStreams.Out, versionInfo.GitVersion)
		return nil
	}

	switch opts.output {
	case "json":
		data, err := json.MarshalIndent(versionInfo, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal version info to JSON: %w", err)
		}
		_, _ = fmt.Fprintln(globalOpts.IOStreams.Out, string(data))
		return nil
	case "yaml":
		data, err := yaml.Marshal(versionInfo)
		if err != nil {
			return fmt.Errorf("failed to marshal version info to YAML: %w", err)
		}
		fmt.Fprint(globalOpts.IOStreams.Out, string(data))
		return nil
	case "":
		fmt.Fprintf(globalOpts.IOStreams.Out, "druidctl version: %s\n", versionInfo.GitVersion)
		fmt.Fprintf(globalOpts.IOStreams.Out, "  Git commit:     %s\n", versionInfo.GitCommit)
		fmt.Fprintf(globalOpts.IOStreams.Out, "  Git tree state: %s\n", versionInfo.GitTreeState)
		fmt.Fprintf(globalOpts.IOStreams.Out, "  Build date:     %s\n", versionInfo.BuildDate)
		fmt.Fprintf(globalOpts.IOStreams.Out, "  Go version:     %s\n", versionInfo.GoVersion)
		fmt.Fprintf(globalOpts.IOStreams.Out, "  Compiler:       %s\n", versionInfo.Compiler)
		fmt.Fprintf(globalOpts.IOStreams.Out, "  Platform:       %s\n", versionInfo.Platform)

		if !versionInfo.IsRelease() {
			_, _ = fmt.Fprintln(globalOpts.IOStreams.Out, "\n ⚠️  This is a development build and not an official release.")
		}
		if versionInfo.IsDirty() {
			_, _ = fmt.Fprintln(globalOpts.IOStreams.Out, " ⚠️  Built from modified source (dirty git tree).")
		}
		return nil
	default:
		return fmt.Errorf("invalid output format: %s (must be json or yaml)", opts.output)
	}
}
