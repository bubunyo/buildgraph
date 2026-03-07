package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	require.NotEmpty(t, cfg.Services)
	assert.Equal(t, "services", cfg.Services[0])
	assert.NotEmpty(t, cfg.Baseline)
	assert.True(t, cfg.Exclude.SkipVendor)
	assert.True(t, cfg.Exclude.SkipTests)
}

func TestLoad_MissingFile_ReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	require.NoError(t, err)
	assert.Equal(t, Default().Baseline, cfg.Baseline)
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
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Len(t, cfg.Services, 2)
	assert.Equal(t, "apps", cfg.Services[0])
	assert.Equal(t, "/tmp/baseline.json", cfg.Baseline)
	assert.False(t, cfg.Exclude.SkipVendor)
	assert.False(t, cfg.Exclude.SkipTests)
}

func TestLoad_UnreadableFile_ReturnsError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test unreadable files as root")
	}

	path := filepath.Join(t.TempDir(), "buildgraph.yaml")
	require.NoError(t, os.WriteFile(path, []byte("services:\n  - apps\n"), 0000))
	t.Cleanup(func() { _ = os.Chmod(path, 0644) })

	_, err := Load(path)
	assert.Error(t, err)
}

func TestLoad_MalformedFile_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "buildgraph.yaml")
	require.NoError(t, os.WriteFile(path, []byte("services: [unclosed"), 0644))

	_, err := Load(path)
	assert.Error(t, err)
}
