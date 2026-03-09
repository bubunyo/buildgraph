package analyzer

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bubunyo/buildgraph/pkg/config"
)

// testprojectDirInternal returns the absolute path to the testproject fixture.
// It is separate from the one in analyzer_test.go because that file lives in
// the external test package (analyzer_test) while this file is in the internal
// package (analyzer).
func testprojectDirInternal() string {
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(repoRoot, "testproject")
}

func loadedAnalyzerInternal(t *testing.T) *Analyzer {
	t.Helper()
	cfg := &config.Config{Services: []string{"services"}}
	a := New(cfg, "github.com/bubunyo/buildgraph/testproject", testprojectDirInternal())
	require.NoError(t, a.Load())
	return a
}

// ── ssaIndex ──────────────────────────────────────────────────────────────────

// TestSSAIndex_PopulatedAfterBuildGraph verifies that BuildGraph() populates
// ssaIndex with at least one entry.
func TestSSAIndex_PopulatedAfterBuildGraph(t *testing.T) {
	a := loadedAnalyzerInternal(t)
	_, _, err := a.BuildGraph()
	require.NoError(t, err)

	assert.NotEmpty(t, a.ssaIndex, "ssaIndex must be non-empty after BuildGraph")
}

// TestSSAIndex_FindSSAFunc_ReturnsNilForUnknown verifies that findSSAFunc
// returns nil when the key does not exist in the index.
func TestSSAIndex_FindSSAFunc_ReturnsNilForUnknown(t *testing.T) {
	a := loadedAnalyzerInternal(t)
	_, _, err := a.BuildGraph()
	require.NoError(t, err)

	result := a.findSSAFunc("definitely.not.a.real.function.key")
	assert.Nil(t, result, "findSSAFunc must return nil for unknown keys")
}

// TestSSAIndex_FindSSAFunc_ReturnsCorrectFunction verifies that findSSAFunc
// returns a non-nil *ssa.Function for a key that is known to be in the graph.
func TestSSAIndex_FindSSAFunc_ReturnsCorrectFunction(t *testing.T) {
	a := loadedAnalyzerInternal(t)
	fns, _, err := a.BuildGraph()
	require.NoError(t, err)
	require.NotEmpty(t, fns)

	// Pick any key from the functions map — it must resolve via findSSAFunc.
	var knownKey string
	for k := range fns {
		knownKey = k
		break
	}

	ssaFn := a.findSSAFunc(knownKey)
	assert.NotNil(t, ssaFn, "findSSAFunc(%q) must return a non-nil *ssa.Function", knownKey)
}

// TestSSAIndex_KeyConsistency verifies that every key in the functions map
// resolves to an *ssa.Function via findSSAFunc, proving the index is complete.
func TestSSAIndex_KeyConsistency(t *testing.T) {
	a := loadedAnalyzerInternal(t)
	fns, _, err := a.BuildGraph()
	require.NoError(t, err)

	missing := 0
	for k, fn := range fns {
		ssaFn := a.findSSAFunc(k)
		if ssaFn == nil && fn.File == "" {
			// Functions with no source file are external/synthetic wrappers that
			// the CHA graph may include without a real SSA node — allow absence.
			continue
		}
		if ssaFn == nil {
			missing++
			t.Logf("findSSAFunc returned nil for key %q (File=%q)", k, fn.File)
		}
	}
	assert.Equal(t, 0, missing, "%d functions in the map could not be resolved via findSSAFunc", missing)
}
