package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ModulePath  string            `yaml:"module_path"`
	Directories DirectoriesConfig `yaml:"directories"`
	Exclude     ExcludeConfig     `yaml:"exclude"`
	Output      OutputConfig      `yaml:"output"`
	Cache       CacheConfig       `yaml:"cache"`
}

type DirectoriesConfig struct {
	Services   string   `yaml:"services"`
	Core       string   `yaml:"core"`
	Additional []string `yaml:"additional"`
}

type ExcludeConfig struct {
	Patterns   []string `yaml:"patterns"`
	SkipVendor bool     `yaml:"skip_vendor"`
	SkipTests  bool     `yaml:"skip_tests"`
}

type OutputConfig struct {
	Format  string `yaml:"format"`
	File    string `yaml:"file"`
	Verbose bool   `yaml:"verbose"`
}

type CacheConfig struct {
	Enabled          bool   `yaml:"enabled"`
	Directory        string `yaml:"directory"`
	IncludeGoVersion bool   `yaml:"include_go_version"`
}

func Default() *Config {
	return &Config{
		ModulePath: "",
		Directories: DirectoriesConfig{
			Services:   "services",
			Core:       "core",
			Additional: []string{},
		},
		Exclude: ExcludeConfig{
			Patterns:   []string{"**/.*"},
			SkipVendor: true,
			SkipTests:  true,
		},
		Output: OutputConfig{
			Format:  "json",
			File:    "",
			Verbose: false,
		},
		Cache: CacheConfig{
			Enabled:          true,
			Directory:        ".buildgraph/cache",
			IncludeGoVersion: true,
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()

	if path == "" {
		path = ".buildgraph/config.yaml"
	}

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

func (c *Config) Validate() error {
	return nil
}
