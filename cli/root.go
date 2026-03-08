// Package cli wires together all buildgraph subcommands and viper config
// initialisation. The binary entry point in cmd/main.go calls Execute().
package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/bubunyo/buildgraph/pkg/config"
)

var rootCmd = &cobra.Command{
	Use:   "buildgraph",
	Short: "Intelligent selective rebuild for Go monorepos",
	Long: `BuildGraph analyses your Go monorepo's call graph to determine exactly
which services need to be rebuilt when code changes.`,
	SilenceUsage: true,
}

// Execute is the single entry point called by cmd/main.go.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().String("config", "buildgraph.yaml", "Config file path")
	rootCmd.PersistentFlags().StringSlice("services", nil, "Directories containing services (overrides config)")
	rootCmd.PersistentFlags().String("baseline", "", "Baseline file path (overrides config)")
	rootCmd.PersistentFlags().Bool("skip-vendor", true, "Skip vendor/ directory")
	rootCmd.PersistentFlags().Bool("skip-tests", true, "Skip *_test.go files")

	_ = viper.BindPFlag("baseline", rootCmd.PersistentFlags().Lookup("baseline"))
	_ = viper.BindPFlag("services", rootCmd.PersistentFlags().Lookup("services"))
	_ = viper.BindPFlag("exclude.skip_vendor", rootCmd.PersistentFlags().Lookup("skip-vendor"))
	_ = viper.BindPFlag("exclude.skip_tests", rootCmd.PersistentFlags().Lookup("skip-tests"))

	rootCmd.AddCommand(analyzeCmd, generateCmd, initCmd)
}

// initConfig reads buildgraph.yaml (or the path from --config) into viper.
func initConfig() {
	cfgFile, _ := rootCmd.PersistentFlags().GetString("config")

	viper.SetConfigFile(cfgFile)
	viper.SetConfigType("yaml")

	viper.SetEnvPrefix("BUILDGRAPH")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	defaults := config.Default()
	viper.SetDefault("services", defaults.Services)
	viper.SetDefault("baseline", defaults.Baseline)
	viper.SetDefault("exclude.skip_vendor", defaults.Exclude.SkipVendor)
	viper.SetDefault("exclude.skip_tests", defaults.Exclude.SkipTests)
	viper.SetDefault("exclude.patterns", defaults.Exclude.Patterns)

	if err := viper.ReadInConfig(); err != nil {
		// When SetConfigFile is used, viper returns a plain *fs.PathError for
		// a missing file — not viper.ConfigFileNotFoundError (which is only
		// returned when searching via SetConfigName/AddConfigPath). Use
		// errors.Is(err, os.ErrNotExist) to correctly catch the missing-file
		// case and only fatal on genuine read errors (bad permissions, etc.).
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "error reading config file %s: %v\n", cfgFile, err)
			os.Exit(1)
		}
	}
}

// loadConfig unmarshals the current viper state into a *config.Config.
func loadConfig() *config.Config {
	cfg := config.Default()
	if err := viper.Unmarshal(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		os.Exit(1)
	}
	if len(cfg.Services) == 0 {
		cfg.Services = config.Default().Services
	}
	return cfg
}
