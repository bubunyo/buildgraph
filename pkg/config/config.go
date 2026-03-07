package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the in-memory representation of buildgraph.yaml.
// Every field can be overridden by a CLI flag of the same name
// (via viper's automatic flag-binding in cmd/).
type Config struct {
	// Services is a list of directories whose immediate subdirectories are
	// treated as deployable services.  Each subdirectory must contain a
	// main package.  Defaults to ["services"].
	Services []string `mapstructure:"services" yaml:"services"`

	// Exclude controls which files and directories are skipped during analysis.
	Exclude ExcludeConfig `mapstructure:"exclude" yaml:"exclude"`

	// Baseline is the path (relative to the project root) where buildgraph
	// reads and writes the baseline snapshot.
	// Default: .buildgraph/baseline.json
	Baseline string `mapstructure:"baseline" yaml:"baseline"`
}

// ExcludeConfig describes what to skip during package loading and analysis.
type ExcludeConfig struct {
	// SkipVendor skips the vendor/ directory. Default: true.
	SkipVendor bool `mapstructure:"skip_vendor" yaml:"skip_vendor"`

	// SkipTests skips *_test.go files. Default: true.
	SkipTests bool `mapstructure:"skip_tests" yaml:"skip_tests"`

	// Patterns is a list of glob patterns to exclude (e.g. "**/*_gen.go").
	Patterns []string `mapstructure:"patterns" yaml:"patterns"`
}

// Load reads a YAML config file at path and merges it over the defaults.
// If the file does not exist, the defaults are returned without error.
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Default returns a Config with sensible defaults for a standard Go monorepo.
func Default() *Config {
	return &Config{
		Services: []string{"services"},
		Exclude: ExcludeConfig{
			SkipVendor: true,
			SkipTests:  true,
			Patterns:   []string{"**/*_gen.go", "**/mock_*.go"},
		},
		Baseline: ".buildgraph/baseline.json",
	}
}
