package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if len(cfg.Services) == 0 {
		t.Error("expected at least one default service directory")
	}
	if cfg.Services[0] != "services" {
		t.Errorf("expected default services directory 'services', got %q", cfg.Services[0])
	}
	if cfg.Baseline == "" {
		t.Error("expected a non-empty default baseline path")
	}
	if !cfg.Exclude.SkipVendor {
		t.Error("expected skip_vendor to default to true")
	}
	if !cfg.Exclude.SkipTests {
		t.Error("expected skip_tests to default to true")
	}
}

func TestLoad_MissingFile_ReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	defaults := Default()
	if cfg.Baseline != defaults.Baseline {
		t.Errorf("expected default baseline %q, got %q", defaults.Baseline, cfg.Baseline)
	}
}

func TestLoad_ValidFile(t *testing.T) {
	content := `
services:
  - apps
  - cmd
baseline: /tmp/baseline.json
exclude:
  skip_vendor: false
  skip_tests: false
`
	path := filepath.Join(t.TempDir(), "buildgraph.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Services) != 2 {
		t.Errorf("expected 2 services, got %d: %v", len(cfg.Services), cfg.Services)
	}
	if cfg.Services[0] != "apps" {
		t.Errorf("expected first service 'apps', got %q", cfg.Services[0])
	}
	if cfg.Baseline != "/tmp/baseline.json" {
		t.Errorf("expected baseline '/tmp/baseline.json', got %q", cfg.Baseline)
	}
	if cfg.Exclude.SkipVendor {
		t.Error("expected skip_vendor false")
	}
	if cfg.Exclude.SkipTests {
		t.Error("expected skip_tests false")
	}
}

func TestLoad_UnreadableFile_ReturnsError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test unreadable files as root")
	}

	path := filepath.Join(t.TempDir(), "buildgraph.yaml")
	if err := os.WriteFile(path, []byte("services:\n  - apps\n"), 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0644) })

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for unreadable file, got nil")
	}
}

func TestLoad_MalformedFile_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "buildgraph.yaml")
	if err := os.WriteFile(path, []byte("services: [unclosed"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for malformed YAML, got nil")
	}
}
