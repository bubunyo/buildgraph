package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bubunyo/buildgraph/pkg/types"
)

func TestNew(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("expected non-nil Storage")
	}
}

func TestLoadBaseline_MissingFile_ReturnsNil(t *testing.T) {
	s := New()
	baseline, err := s.LoadBaseline("/nonexistent/path/baseline.json")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if baseline != nil {
		t.Fatal("expected nil baseline for missing file")
	}
}

func TestLoadBaseline_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	want := &types.Baseline{
		Version:    "1.0",
		ModulePath: "github.com/example/repo",
		Commit:     "abc123",
	}

	s := New()
	if err := s.SaveBaseline(want, path); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}

	got, err := s.LoadBaseline(path)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil baseline")
	}
	if got.Version != want.Version {
		t.Errorf("Version: got %q, want %q", got.Version, want.Version)
	}
	if got.ModulePath != want.ModulePath {
		t.Errorf("ModulePath: got %q, want %q", got.ModulePath, want.ModulePath)
	}
	if got.Commit != want.Commit {
		t.Errorf("Commit: got %q, want %q", got.Commit, want.Commit)
	}
}

func TestLoadBaseline_MalformedJSON_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	if err := os.WriteFile(path, []byte("not valid json {{{"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := New()
	_, err := s.LoadBaseline(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestSaveBaseline_WritesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	baseline := &types.Baseline{
		Version:     "1.0",
		GeneratedAt: time.Now(),
		Commit:      "deadbeef",
		ModulePath:  "github.com/example/repo",
	}

	s := New()
	if err := s.SaveBaseline(baseline, path); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty file")
	}
}

func TestSaveBaseline_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "baseline.json")

	baseline := &types.Baseline{Version: "1.0"}

	s := New()
	if err := s.SaveBaseline(baseline, path); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	want := &types.Baseline{
		Version:    "1.0",
		Commit:     "cafebabe",
		ModulePath: "github.com/example/repo",
		GoVersion:  "go1.24.2",
		FunctionHashes: map[string]types.HashInfo{
			"pkg.Foo": {ASTHash: "sha256:aabbcc", TransitiveHash: "sha256:ddeeff"},
		},
		ExternalDeps:     map[string]string{"github.com/some/dep": "v1.2.3"},
		ExternalDepsHash: "sha256:112233",
		SourceHashes:     map[string]string{"foo.go": "sha256:aabbcc"},
	}

	s := New()
	if err := s.SaveBaseline(want, path); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}

	got, err := s.LoadBaseline(path)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}

	if got.Commit != want.Commit {
		t.Errorf("Commit: got %q want %q", got.Commit, want.Commit)
	}
	if got.GoVersion != want.GoVersion {
		t.Errorf("GoVersion: got %q want %q", got.GoVersion, want.GoVersion)
	}
	if got.ExternalDepsHash != want.ExternalDepsHash {
		t.Errorf("ExternalDepsHash: got %q want %q", got.ExternalDepsHash, want.ExternalDepsHash)
	}
	if got.FunctionHashes["pkg.Foo"].ASTHash != want.FunctionHashes["pkg.Foo"].ASTHash {
		t.Errorf("FunctionHashes[pkg.Foo].ASTHash mismatch")
	}
	if got.SourceHashes["foo.go"] != want.SourceHashes["foo.go"] {
		t.Errorf("SourceHashes[foo.go] mismatch")
	}
}
