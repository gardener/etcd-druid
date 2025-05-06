// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package opts

import (
	"fmt"
	configv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	configvalidation "github.com/gardener/etcd-druid/api/config/v1alpha1/validation"
	flag "github.com/spf13/pflag"
	"golang.org/x/exp/slog"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"os"
)

var configDecoder runtime.Decoder

func init() {
	configScheme := runtime.NewScheme()
	utilruntime.Must(configv1alpha1.AddToScheme(configScheme))
	configDecoder = serializer.NewCodecFactory(configScheme).UniversalDecoder()
}

// CLIOptions provides convenience abstraction to initialize and validate OperatorConfiguration from CLI flags.
type CLIOptions struct {
	configFile string
	// Config is the operator configuration initialized from the CLI flags.
	Config           *configv1alpha1.OperatorConfiguration
	deprecatedConfig *deprecatedOperatorConfiguration
}

// NewCLIOptions creates a new CLIOptions and adds the required CLI flags to the flag.flagSet.
func NewCLIOptions(fs *flag.FlagSet) *CLIOptions {
	cliOpts := &CLIOptions{
		deprecatedConfig: &deprecatedOperatorConfiguration{},
	}
	cliOpts.addFlags(fs)
	cliOpts.deprecatedConfig.addDeprecatedFlags(fs)
	return cliOpts
}

// Complete reads the configuration file and decodes it into an OperatorConfiguration.
func (o *CLIOptions) Complete() error {
	if len(o.configFile) == 0 {
		slog.Info("No config file specified. Falling back to deprecated CLI flags if defined.")
		// setting captured deprecated worker flags to OperatorConfiguration.
		o.Config = o.deprecatedConfig.ToOperatorConfiguration()
		configv1alpha1.SetObjectDefaults_OperatorConfiguration(o.Config)
		return nil
	}
	if err := o.initializeOperatorConfigurationFromFile(); err != nil {
		return fmt.Errorf("error initializing operator configuration from file: %w", err)
	}
	return configv1alpha1.DefaultFeatureGates.SetEnabledFeaturesFromMap(o.Config.FeatureGates)
}

func (o *CLIOptions) initializeOperatorConfigurationFromFile() error {
	data, err := os.ReadFile(o.configFile)
	if err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}
	o.Config = &configv1alpha1.OperatorConfiguration{}
	if err = runtime.DecodeInto(configDecoder, data, o.Config); err != nil {
		return fmt.Errorf("error decoding config: %w", err)
	}
	return nil
}

func (o *CLIOptions) Validate() error {
	if errs := configvalidation.ValidateOperatorConfiguration(o.Config); len(errs) > 0 {
		return errs.ToAggregate()
	}
	return nil
}

func (o *CLIOptions) addFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.configFile, "config", o.configFile, "Path to configuration file.")
}

// In the new OperatorConfiguration all concurrentSyncs are defined as pointer to int.
// This cannot be used when parsing flags as it results in a nil pointer dereference error.
// We did not want to change the type to a non-pointer type as it will be hard to differentiate between
// default value (which will be 0) and the user explicitly setting it to 0.
// NOTE: Once we stop supporting deprecated flags then this can be removed.
type deprecatedWorkers struct {
	etcdWorkers                int
	compactionWorkers          int
	etcdCopyBackupsTaskWorkers int
	secretWorkers              int
}

func (d deprecatedWorkers) updateOperatorConfiguration(operatorConfig *configv1alpha1.OperatorConfiguration) {
	operatorConfig.Controllers.Etcd.ConcurrentSyncs = &d.etcdWorkers
	operatorConfig.Controllers.Compaction.ConcurrentSyncs = &d.compactionWorkers
	operatorConfig.Controllers.EtcdCopyBackupsTask.ConcurrentSyncs = &d.etcdCopyBackupsTaskWorkers
	operatorConfig.Controllers.Secret.ConcurrentSyncs = &d.secretWorkers
}

// createEmptyOperatorConfig creates an empty OperatorConfiguration with default values.
// As long as we support deprecated flags we need to initialize pointer fields to empty structs.
// This prevents nil pointer dereference errors.
func createEmptyOperatorConfig() *configv1alpha1.OperatorConfiguration {
	return &configv1alpha1.OperatorConfiguration{
		Server: configv1alpha1.ServerConfiguration{
			Webhooks: &configv1alpha1.HTTPSServer{},
			Metrics:  &configv1alpha1.Server{},
		},
	}
}
