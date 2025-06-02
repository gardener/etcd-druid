// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package opts

import (
	"fmt"
	"os"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	configvalidation "github.com/gardener/etcd-druid/api/config/v1alpha1/validation"

	"github.com/go-logr/logr"
	flag "github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

var configDecoder runtime.Decoder

func init() {
	configScheme := runtime.NewScheme()
	utilruntime.Must(druidconfigv1alpha1.AddToScheme(configScheme))
	configDecoder = serializer.NewCodecFactory(configScheme).UniversalDecoder()
}

// CLIOptions provides convenience abstraction to initialize and validate OperatorConfiguration from CLI flags.
type CLIOptions struct {
	logger     logr.Logger
	configFile string
	// Config is the operator configuration initialized from the CLI flags.
	Config           *druidconfigv1alpha1.OperatorConfiguration
	deprecatedConfig *deprecatedOperatorConfiguration
}

// NewCLIOptions creates a new CLIOptions and adds the required CLI flags to the flag.flagSet.
func NewCLIOptions(fs *flag.FlagSet, logger logr.Logger) *CLIOptions {
	cliOpts := &CLIOptions{
		logger:           logger,
		deprecatedConfig: &deprecatedOperatorConfiguration{},
	}
	cliOpts.addFlags(fs)
	cliOpts.deprecatedConfig.addDeprecatedFlags(fs)
	return cliOpts
}

// Complete reads the configuration file and decodes it into an OperatorConfiguration.
func (o *CLIOptions) Complete() error {
	if len(o.configFile) == 0 {
		o.logger.Info("No config file specified. Falling back to deprecated CLI flags if defined.")
		// setting captured deprecated worker flags to OperatorConfiguration.
		o.Config = o.deprecatedConfig.ToOperatorConfiguration()
		druidconfigv1alpha1.SetObjectDefaults_OperatorConfiguration(o.Config)
		return nil
	}
	if err := o.initializeOperatorConfigurationFromFile(); err != nil {
		return fmt.Errorf("error initializing operator configuration from file: %w", err)
	}
	return druidconfigv1alpha1.DefaultFeatureGates.SetEnabledFeaturesFromMap(o.Config.FeatureGates)
}

func (o *CLIOptions) initializeOperatorConfigurationFromFile() error {
	data, err := os.ReadFile(o.configFile)
	if err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}
	o.Config = &druidconfigv1alpha1.OperatorConfiguration{}
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
