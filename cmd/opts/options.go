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
	Config *configv1alpha1.OperatorConfiguration
}

// NewCLIOptions creates a new CLIOptions and adds the required CLI flags to the flag.flagSet.
func NewCLIOptions(fs *flag.FlagSet) *CLIOptions {
	cliOpts := &CLIOptions{
		Config: &configv1alpha1.OperatorConfiguration{},
	}
	cliOpts.addFlags(fs)
	cliOpts.addDeprecatedFlags(fs)
	return cliOpts
}

// Complete reads the configuration file and decodes it into an OperatorConfiguration.
func (o *CLIOptions) Complete() error {
	if len(o.configFile) == 0 {
		slog.Info("No config file specified. Falling back to deprecated CLI flags if defined.")
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
	if err = runtime.DecodeInto(configDecoder, data, o.Config); err != nil {
		return fmt.Errorf("error decoding config: %w", err)
	}
	return nil
}

func (o *CLIOptions) Validate() error {
	if errs := configvalidation.ValidateOperatorConfiguration(o.Config); errs != nil {
		return errs.ToAggregate()
	}
	return nil
}

func (o *CLIOptions) addFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.configFile, "config", o.configFile, "Path to configuration file.")
}
