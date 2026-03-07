package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bubunyo/buildgraph/pkg/types"
)

// ── detectRootModule ──────────────────────────────────────────────────────────

func TestDetectRootModule_Valid(t *testing.T) {
	dir := t.TempDir()
	content := "module github.com/example/repo\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := detectRootModule(dir)
	if err != nil {
		t.Fatalf("detectRootModule: %v", err)
	}
	if got != "github.com/example/repo" {
		t.Errorf("got %q, want %q", got, "github.com/example/repo")
	}
}

func TestDetectRootModule_MissingModuleDirective_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("go 1.21\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := detectRootModule(dir)
	if err == nil {
		t.Fatal("expected error for missing module directive, got nil")
	}
}

func TestDetectRootModule_MissingFile_ReturnsError(t *testing.T) {
	_, err := detectRootModule("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing go.mod, got nil")
	}
}

// ── writeOutput ───────────────────────────────────────────────────────────────

func TestWriteOutput_JSON_ToStdout(t *testing.T) {
	// Just ensure it doesn't panic — stdout capture is not needed.
	result := &types.Result{
		HasChanges: false,
		Impact:     types.Impact{ServicesToBuild: []string{}},
	}
	// Should not panic.
	writeOutput(result, "json", "")
}

func TestWriteOutput_Text_ToStdout(t *testing.T) {
	result := &types.Result{
		HasChanges: false,
		Impact:     types.Impact{ServicesToBuild: []string{}},
	}
	writeOutput(result, "text", "")
}

func TestWriteOutput_JSON_ToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.json")

	result := &types.Result{
		HasChanges:    true,
		CurrentCommit: "abc123",
		Changes: []types.Change{
			{Function: "pkg.Foo", Type: "modified"},
		},
		Impact: types.Impact{ServicesToBuild: []string{"service-a"}},
	}

	writeOutput(result, "json", path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "abc123") {
		t.Errorf("expected commit in output, got: %s", string(data))
	}
	if !strings.Contains(string(data), "service-a") {
		t.Errorf("expected service-a in output, got: %s", string(data))
	}
}

func TestWriteOutput_Text_ToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")

	result := &types.Result{
		HasChanges:    false,
		CurrentCommit: "deadbeef",
		Impact:        types.Impact{ServicesToBuild: []string{}},
	}

	writeOutput(result, "text", path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "deadbeef") {
		t.Errorf("expected commit in output, got: %s", string(data))
	}
}

func TestWriteOutput_UnknownFormat_FallsBackToJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.out")

	result := &types.Result{
		HasChanges:    false,
		CurrentCommit: "cafebabe",
		Impact:        types.Impact{ServicesToBuild: []string{}},
	}

	writeOutput(result, "unknown", path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// Should fall back to JSON — check for JSON structure.
	if !strings.Contains(string(data), "has_changes") {
		t.Errorf("expected JSON output, got: %s", string(data))
	}
}

// ── runInit ───────────────────────────────────────────────────────────────────

func TestRunInit_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer func() { _ = os.Chdir(orig) }()

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	if err := runInit(nil, nil); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	data, err := os.ReadFile("buildgraph.yaml")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "services:") {
		t.Errorf("expected 'services:' in buildgraph.yaml, got: %s", string(data))
	}
	if !strings.Contains(string(data), "baseline:") {
		t.Errorf("expected 'baseline:' in buildgraph.yaml, got: %s", string(data))
	}
}

func TestRunInit_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer func() { _ = os.Chdir(orig) }()

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	// Create the file first.
	if err := os.WriteFile("buildgraph.yaml", []byte("existing"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err = runInit(nil, nil)
	if err == nil {
		t.Fatal("expected error when file already exists, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' in error, got: %v", err)
	}
}

// ── getGitCommit / getGoVersion ───────────────────────────────────────────────

func TestGetGitCommit_ReturnsString(t *testing.T) {
	// Either returns a real commit hash or "unknown" — just ensure no panic.
	got := getGitCommit()
	if got == "" {
		t.Error("expected non-empty result from getGitCommit")
	}
}

func TestGetGoVersion_ReturnsString(t *testing.T) {
	got := getGoVersion()
	if got == "" {
		t.Error("expected non-empty result from getGoVersion")
	}
	// Should start with "go" or be "unknown".
	if got != "unknown" && !strings.HasPrefix(got, "go") {
		t.Errorf("unexpected go version format: %q", got)
	}
}
