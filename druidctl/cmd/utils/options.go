// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// GlobalOptions holds all user-configurable CLI options and flags.
// Runtime dependencies (Logger, IOStreams, Clients) are in RuntimeEnv.
type GlobalOptions struct {
	// Verbose enables verbose output
	Verbose bool
	// AllNamespaces lists resources across all namespaces
	AllNamespaces bool
	// ResourceArgs holds positional args (can be "name" or "ns/name")
	ResourceArgs []string
	// DisableBanner disables the CLI banner
	DisableBanner bool
	// LabelSelector filters resources by labels (-l flag)
	LabelSelector string
	// ConfigFlags holds kubectl-compatible flags (kubeconfig, namespace, context, etc.)
	ConfigFlags *genericclioptions.ConfigFlags
}

// newGlobalOptions creates a new GlobalOptions with the given ConfigFlags.
// This is internal - use NewCommandContext() to create the full context.
func newGlobalOptions(configFlags *genericclioptions.ConfigFlags) *GlobalOptions {
	return &GlobalOptions{
		ConfigFlags: configFlags,
	}
}

// AddFlags adds flags to the specified command
func (o *GlobalOptions) AddFlags(cmd *cobra.Command) {
	o.ConfigFlags.AddFlags(cmd.PersistentFlags())
	cmd.PersistentFlags().BoolVarP(&o.Verbose, "verbose", "v", false, "Enable verbose output")
	cmd.PersistentFlags().BoolVarP(&o.AllNamespaces, "all-namespaces", "A", false,
		"If present, list the requested object(s) across all namespaces")
	cmd.PersistentFlags().BoolVar(&o.DisableBanner, "no-banner", false, "Disable the CLI banner")
	cmd.PersistentFlags().StringVarP(&o.LabelSelector, "selector", "l", "",
		"Selector (label query) to filter on, supports '=', '==', and '!='.")
}

// Complete fills in the GlobalOptions based on command line args and flags.
func (o *GlobalOptions) Complete(_ *cobra.Command, args []string) error {
	o.ResourceArgs = args
	return nil
}

// ValidateResourceSelection validates resource selection requirements.
// kubectl-compatible behavior:
//   - Resource args: "name" (uses -n namespace) or "ns/name" (cross-namespace)
//   - -n namespace: scopes to namespace (if no args, lists all in namespace)
//   - -A: lists all resources across all namespaces
//   - -l selector: filters by labels
//
// Validation rules:
//   - Cannot combine -A with resource args
//   - Cannot combine -A with -n
//   - Cross-namespace args (ns/name) cannot be combined with -n flag
func (o *GlobalOptions) ValidateResourceSelection() error {
	hasResourceArgs := len(o.ResourceArgs) > 0
	hasCrossNamespaceArgs := o.hasCrossNamespaceArgs()
	hasNamespaceFlag := o.ConfigFlags.Namespace != nil && *o.ConfigFlags.Namespace != ""

	if o.AllNamespaces {
		if hasResourceArgs {
			return fmt.Errorf("cannot specify resource names when using --all-namespaces/-A")
		}
		if hasNamespaceFlag {
			return fmt.Errorf("cannot use --namespace/-n with --all-namespaces/-A")
		}
		return nil
	}

	// Cross-namespace args (ns/name) conflict with -n flag
	if hasCrossNamespaceArgs && hasNamespaceFlag {
		return fmt.Errorf("cannot use --namespace/-n with cross-namespace resource references (ns/name format)")
	}
	return nil
}

// hasCrossNamespaceArgs checks if any resource arg contains a namespace prefix (ns/name format)
func (o *GlobalOptions) hasCrossNamespaceArgs() bool {
	for _, arg := range o.ResourceArgs {
		if strings.Contains(arg, "/") {
			return true
		}
	}
	return false
}

// BuildEtcdRefList builds a list of NamespacedName from ResourceArgs respecting the -n flag.
// kubectl-compatible behavior:
//   - "name" : uses namespace from -n flag or default
//   - "ns/name" : uses explicit namespace (cross-namespace selection)
//   - Empty args : returns nil (caller should list all in namespace)
func (o *GlobalOptions) BuildEtcdRefList() []types.NamespacedName {
	if len(o.ResourceArgs) == 0 {
		return nil
	}

	namespace := o.GetNamespace()
	refs := make([]types.NamespacedName, 0, len(o.ResourceArgs))

	for _, arg := range o.ResourceArgs {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}

		if strings.Contains(arg, "/") {
			// Cross-namespace format: ns/name
			parts := strings.SplitN(arg, "/", 2)
			refs = append(refs, types.NamespacedName{
				Namespace: parts[0],
				Name:      parts[1],
			})
		} else {
			// Simple name - use -n namespace or default
			refs = append(refs, types.NamespacedName{
				Namespace: namespace,
				Name:      arg,
			})
		}
	}

	return refs
}

// GetNamespace returns the namespace from -n flag or "default" if not specified.
func (o *GlobalOptions) GetNamespace() string {
	if o.ConfigFlags.Namespace != nil && *o.ConfigFlags.Namespace != "" {
		return *o.ConfigFlags.Namespace
	}
	return "default"
}
