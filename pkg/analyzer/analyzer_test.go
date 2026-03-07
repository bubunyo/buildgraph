package analyzer_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bubunyo/buildgraph/pkg/analyzer"
	"github.com/bubunyo/buildgraph/pkg/config"
	"github.com/bubunyo/buildgraph/pkg/types"
)

// testprojectDir returns the absolute path to the testproject fixture.
func testprojectDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(repoRoot, "testproject")
}

func newTestAnalyzer() *analyzer.Analyzer {
	cfg := &config.Config{Services: []string{"services"}}
	return analyzer.New(cfg, "github.com/bubunyo/buildgraph/testproject", testprojectDir())
}

// loadedAnalyzer returns an Analyzer that has already called Load().
func loadedAnalyzer(t *testing.T) *analyzer.Analyzer {
	t.Helper()
	a := newTestAnalyzer()
	require.NoError(t, a.Load())
	return a
}

// ── New ───────────────────────────────────────────────────────────────────────

func TestNew_ReturnsNonNil(t *testing.T) {
	assert.NotNil(t, newTestAnalyzer())
}

// ── Load ──────────────────────────────────────────────────────────────────────

func TestLoad_Succeeds(t *testing.T) {
	assert.NoError(t, newTestAnalyzer().Load())
}

// ── BuildGraph ────────────────────────────────────────────────────────────────

func TestBuildGraph_WithoutLoad_ReturnsError(t *testing.T) {
	_, _, err := newTestAnalyzer().BuildGraph()
	assert.Error(t, err)
}

func TestBuildGraph_ReturnsNonEmptyGraph(t *testing.T) {
	a := loadedAnalyzer(t)

	fns, graph, err := a.BuildGraph()
	require.NoError(t, err)
	assert.NotEmpty(t, fns)
	require.NotNil(t, graph)
	assert.NotEmpty(t, graph.Nodes)
}

func TestBuildGraph_OnlyContainsInternalFunctions(t *testing.T) {
	a := loadedAnalyzer(t)

	fns, _, err := a.BuildGraph()
	require.NoError(t, err)

	for key := range fns {
		assert.Contains(t, key, "github.com/bubunyo/buildgraph/testproject",
			"unexpected external function in map: %s", key)
	}
}

func TestBuildGraph_FunctionOwnerIsPopulated(t *testing.T) {
	a := loadedAnalyzer(t)

	_, graph, err := a.BuildGraph()
	require.NoError(t, err)

	assert.NotEmpty(t, graph.FunctionOwner)
	for key, owner := range graph.FunctionOwner {
		assert.NotEmpty(t, owner, "function %s has empty owner", key)
	}
}

func TestBuildGraph_ReverseIndexPopulated(t *testing.T) {
	a := loadedAnalyzer(t)

	_, graph, err := a.BuildGraph()
	require.NoError(t, err)

	// module-a is called by both services; its functions should appear in the
	// reverse index.
	found := false
	for key := range graph.ReverseIndex {
		if strings.Contains(key, "module") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected reverse index to contain entries for shared module functions")
}

// ── ComputeHashes ─────────────────────────────────────────────────────────────

func TestComputeHashes_Succeeds(t *testing.T) {
	a := loadedAnalyzer(t)
	fns, _, err := a.BuildGraph()
	require.NoError(t, err)
	assert.NoError(t, a.ComputeHashes(fns, nil, nil))
}

func TestComputeHashes_FillsASTHash(t *testing.T) {
	a := loadedAnalyzer(t)
	fns, _, err := a.BuildGraph()
	require.NoError(t, err)
	require.NoError(t, a.ComputeHashes(fns, nil, nil))

	filled := 0
	for _, fn := range fns {
		if fn.ASTHash != "" {
			filled++
		}
	}
	assert.Greater(t, filled, 0, "expected at least one function with a non-empty ASTHash")
}

func TestComputeHashes_FillsTransitiveHash(t *testing.T) {
	a := loadedAnalyzer(t)
	fns, _, err := a.BuildGraph()
	require.NoError(t, err)
	require.NoError(t, a.ComputeHashes(fns, nil, nil))

	for _, fn := range fns {
		assert.NotEmpty(t, fn.TransitiveHash, "function %s has empty TransitiveHash", fn.FullName)
	}
}

func TestComputeHashes_Deterministic(t *testing.T) {
	load := func() map[string]string {
		a := loadedAnalyzer(t)
		fns, _, err := a.BuildGraph()
		require.NoError(t, err)
		require.NoError(t, a.ComputeHashes(fns, nil, nil))
		hashes := make(map[string]string, len(fns))
		for k, fn := range fns {
			hashes[k] = fn.ASTHash
		}
		return hashes
	}

	h1, h2 := load(), load()
	for k, v := range h1 {
		assert.Equal(t, v, h2[k], "hash for %s is not deterministic", k)
	}
}

func TestComputeHashes_ReusesPrevHashWhenFileUnchanged(t *testing.T) {
	// First run — compute the baseline hashes.
	a1 := loadedAnalyzer(t)
	fns1, _, err := a1.BuildGraph()
	require.NoError(t, err)
	require.NoError(t, a1.ComputeHashes(fns1, nil, nil))

	prevSource, err := a1.ComputeSourceHashes()
	require.NoError(t, err)

	prevFuncHashes := make(map[string]types.HashInfo, len(fns1))
	for k, fn := range fns1 {
		prevFuncHashes[k] = types.HashInfo{ASTHash: fn.ASTHash, TransitiveHash: fn.TransitiveHash}
	}

	// Second run — pass previous hashes so the analyzer can reuse cached values.
	a2 := loadedAnalyzer(t)
	fns2, _, err := a2.BuildGraph()
	require.NoError(t, err)
	require.NoError(t, a2.ComputeHashes(fns2, prevSource, prevFuncHashes))

	for k, fn2 := range fns2 {
		if fn1, ok := fns1[k]; ok && fn1.ASTHash != "" {
			assert.Equal(t, fn1.ASTHash, fn2.ASTHash, "ASTHash mismatch for %s", k)
		}
	}
}

// ── ComputeSourceHashes ───────────────────────────────────────────────────────

func TestComputeSourceHashes_ReturnsNonEmpty(t *testing.T) {
	a := loadedAnalyzer(t)
	hashes, err := a.ComputeSourceHashes()
	require.NoError(t, err)
	assert.NotEmpty(t, hashes)
	for path, hash := range hashes {
		assert.True(t, strings.HasPrefix(hash, "sha256:"),
			"hash for %s does not start with sha256: got %q", path, hash)
	}
}

func TestComputeSourceHashes_Deterministic(t *testing.T) {
	run := func() map[string]string {
		a := loadedAnalyzer(t)
		h, err := a.ComputeSourceHashes()
		require.NoError(t, err)
		return h
	}
	h1, h2 := run(), run()
	for k, v := range h1 {
		assert.Equal(t, v, h2[k], "source hash for %s is not deterministic", k)
	}
}

// ── ExtractExternalDeps ───────────────────────────────────────────────────────

func TestExtractExternalDeps_Succeeds(t *testing.T) {
	_, _, err := newTestAnalyzer().ExtractExternalDeps()
	assert.NoError(t, err)
}

func TestExtractExternalDeps_ReturnsNonEmptyHash(t *testing.T) {
	_, hash, err := newTestAnalyzer().ExtractExternalDeps()
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}
