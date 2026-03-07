package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bubunyo/buildgraph/pkg/types"
)

// ── detectRootModule ──────────────────────────────────────────────────────────

func TestDetectRootModule_Valid(t *testing.T) {
	dir := t.TempDir()
	content := "module github.com/example/repo\n\ngo 1.21\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0644))

	got, err := detectRootModule(dir)
	require.NoError(t, err)
	assert.Equal(t, "github.com/example/repo", got)
}

func TestDetectRootModule_MissingModuleDirective_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("go 1.21\n"), 0644))

	_, err := detectRootModule(dir)
	assert.Error(t, err)
}

func TestDetectRootModule_MissingFile_ReturnsError(t *testing.T) {
	_, err := detectRootModule("/nonexistent/path")
	assert.Error(t, err)
}

// ── writeOutput ───────────────────────────────────────────────────────────────

func TestWriteOutput_JSON_ToStdout(t *testing.T) {
	result := &types.Result{
		HasChanges: false,
		Impact:     types.Impact{ServicesToBuild: []string{}},
	}
	// Should not panic.
	assert.NotPanics(t, func() { writeOutput(result, "json", "") })
}

func TestWriteOutput_Text_ToStdout(t *testing.T) {
	result := &types.Result{
		HasChanges: false,
		Impact:     types.Impact{ServicesToBuild: []string{}},
	}
	assert.NotPanics(t, func() { writeOutput(result, "text", "") })
}

func TestWriteOutput_JSON_ToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.json")
	result := &types.Result{
		HasChanges:    true,
		CurrentCommit: "abc123",
		Changes:       []types.Change{{Function: "pkg.Foo", Type: "modified"}},
		Impact:        types.Impact{ServicesToBuild: []string{"service-a"}},
	}

	writeOutput(result, "json", path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "abc123")
	assert.Contains(t, string(data), "service-a")
}

func TestWriteOutput_Text_ToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.txt")
	result := &types.Result{
		HasChanges:    false,
		CurrentCommit: "deadbeef",
		Impact:        types.Impact{ServicesToBuild: []string{}},
	}

	writeOutput(result, "text", path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "deadbeef")
}

func TestWriteOutput_UnknownFormat_FallsBackToJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.out")
	result := &types.Result{
		HasChanges:    false,
		CurrentCommit: "cafebabe",
		Impact:        types.Impact{ServicesToBuild: []string{}},
	}

	writeOutput(result, "unknown", path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "has_changes")
}

// ── runInit ───────────────────────────────────────────────────────────────────

func TestRunInit_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	orig, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(orig) }()
	require.NoError(t, os.Chdir(dir))

	require.NoError(t, runInit(nil, nil))

	data, err := os.ReadFile("buildgraph.yaml")
	require.NoError(t, err)
	assert.Contains(t, string(data), "services:")
	assert.Contains(t, string(data), "baseline:")
}

func TestRunInit_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	orig, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(orig) }()
	require.NoError(t, os.Chdir(dir))

	require.NoError(t, os.WriteFile("buildgraph.yaml", []byte("existing"), 0644))

	err = runInit(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// ── getGitCommit / getGoVersion ───────────────────────────────────────────────

func TestGetGitCommit_ReturnsString(t *testing.T) {
	got := getGitCommit()
	assert.NotEmpty(t, got)
}

func TestGetGoVersion_ReturnsString(t *testing.T) {
	got := getGoVersion()
	assert.NotEmpty(t, got)
	assert.True(t, got == "unknown" || strings.HasPrefix(got, "go"),
		"unexpected go version format: %q", got)
}
