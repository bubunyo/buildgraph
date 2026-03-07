package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bubunyo/buildgraph/pkg/types"
)

func TestNew(t *testing.T) {
	assert.NotNil(t, New())
}

func TestLoadBaseline_MissingFile_ReturnsNil(t *testing.T) {
	s := New()
	baseline, err := s.LoadBaseline("/nonexistent/path/baseline.json")
	require.NoError(t, err)
	assert.Nil(t, baseline)
}

func TestLoadBaseline_ValidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	want := &types.Baseline{
		Version:    "1.0",
		ModulePath: "github.com/example/repo",
		Commit:     "abc123",
	}

	s := New()
	require.NoError(t, s.SaveBaseline(want, path))

	got, err := s.LoadBaseline(path)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, want.Version, got.Version)
	assert.Equal(t, want.ModulePath, got.ModulePath)
	assert.Equal(t, want.Commit, got.Commit)
}

func TestLoadBaseline_MalformedJSON_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	require.NoError(t, os.WriteFile(path, []byte("not valid json {{{"), 0644))

	_, err := New().LoadBaseline(path)
	assert.Error(t, err)
}

func TestSaveBaseline_WritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	baseline := &types.Baseline{
		Version:     "1.0",
		GeneratedAt: time.Now(),
		Commit:      "deadbeef",
		ModulePath:  "github.com/example/repo",
	}

	require.NoError(t, New().SaveBaseline(baseline, path))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestSaveBaseline_CreatesParentDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deep", "baseline.json")

	require.NoError(t, New().SaveBaseline(&types.Baseline{Version: "1.0"}, path))

	_, err := os.Stat(path)
	assert.NoError(t, err)
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
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
	require.NoError(t, s.SaveBaseline(want, path))

	got, err := s.LoadBaseline(path)
	require.NoError(t, err)

	assert.Equal(t, want.Commit, got.Commit)
	assert.Equal(t, want.GoVersion, got.GoVersion)
	assert.Equal(t, want.ExternalDepsHash, got.ExternalDepsHash)
	assert.Equal(t, want.FunctionHashes["pkg.Foo"].ASTHash, got.FunctionHashes["pkg.Foo"].ASTHash)
	assert.Equal(t, want.SourceHashes["foo.go"], got.SourceHashes["foo.go"])
}
