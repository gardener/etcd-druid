package utils

import (
	"fmt"
	"os"

	"github.com/gardener/etcd-druid/druidctl/internal/client"
	"github.com/gardener/etcd-druid/druidctl/internal/log"
	"github.com/gardener/etcd-druid/druidctl/internal/utils"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

const namespace = "default"

// GlobalOptions holds all global options and configuration for the CLI
type GlobalOptions struct {
	// Common options
	Verbose       bool
	AllNamespaces bool
	ResourcesRef  string
	DisableBanner bool

	// IO options
	LogType   log.LogType
	Logger    log.Logger
	IOStreams genericiooptions.IOStreams

	// client options
	ConfigFlags   *genericclioptions.ConfigFlags
	ClientFactory client.Factory
	Clients       *ClientBundle
}

// NewOptions returns a new Options instance with default values
func NewOptions() *GlobalOptions {
	configFlags := utils.GetConfigFlags()
	factory := client.NewClientFactory(configFlags)
	return &GlobalOptions{
		LogType:       log.LogTypeCharm,
		ConfigFlags:   configFlags,
		ClientFactory: factory,
		Clients:       NewClientBundle(factory),
		IOStreams:     genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr},
	}
}

// AddFlags adds flags to the specified command
func (o *GlobalOptions) AddFlags(cmd *cobra.Command) {
	o.ConfigFlags.AddFlags(cmd.PersistentFlags())
	cmd.PersistentFlags().BoolVarP(&o.Verbose, "verbose", "v", false, "Enable verbose output")
	cmd.PersistentFlags().BoolVarP(&o.AllNamespaces, "all-namespaces", "A", false,
		"If present, list the requested object(s) across all namespaces")
	cmd.PersistentFlags().BoolVar(&o.DisableBanner, "no-banner", false, "Disable the CLI banner")
}

// Complete fills in the GlobalOptions based on command line args and flags
func (o *GlobalOptions) Complete(cmd *cobra.Command, args []string) error {
	// Initialize Logger
	o.Logger = log.NewLogger(o.LogType)
	o.Logger.SetVerbose(o.Verbose)

	// Fill in ResourceName and Namespace
	if len(args) > 0 {
		o.ResourcesRef = args[0]
	}
	return nil
}

// Validate validates the GlobalOptions
func (o *GlobalOptions) Validate() error {
	if o.AllNamespaces {
		if o.ResourcesRef != "" {
			return fmt.Errorf("cannot specify a resource name with --all-namespaces/-A")
		}
	} else {
		if o.ResourcesRef == "" {
			return fmt.Errorf("etcd resource name is required when not using --all-namespaces")
		}
	}
	return nil
}
