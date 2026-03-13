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
	"github.com/bubunyo/buildgraph/pkg/impact"
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

// ── funcKey collision ─────────────────────────────────────────────────────────

// TestBuildGraph_ReceiverMethodsHaveDistinctKeys verifies that two methods
// with the same name on different receiver types in the same package produce
// distinct keys in the functions map. The testproject/core/collision package
// defines (*A).Run and (*B).Run for exactly this purpose.
func TestBuildGraph_ReceiverMethodsHaveDistinctKeys(t *testing.T) {
	a := loadedAnalyzer(t)

	fns, _, err := a.BuildGraph()
	require.NoError(t, err)

	// Collect all keys that contain the collision package and the method name "Run".
	var runKeys []string
	for key := range fns {
		if strings.Contains(key, "collision") && strings.HasSuffix(key, ".Run") {
			runKeys = append(runKeys, key)
		}
	}

	// We expect exactly two distinct keys: one for (*A).Run and one for (*B).Run.
	require.Len(t, runKeys, 2,
		"expected two distinct keys for (*A).Run and (*B).Run, got: %v", runKeys)
	assert.NotEqual(t, runKeys[0], runKeys[1],
		"(*A).Run and (*B).Run must not share the same funcKey")
}

// ── Exclude patterns ─────────────────────────────────────────────────────────

// TestBuildGraph_ExcludePatternsRemoveFunctions verifies that functions whose
// source file matches a configured exclude glob pattern are omitted from the
// returned functions map and call graph.
func TestBuildGraph_ExcludePatternsRemoveFunctions(t *testing.T) {
	// The collision package lives in core/collision/collision.go.
	// Excluding "**/collision.go" should remove (*A).Run and (*B).Run.
	cfg := &config.Config{
		Services: []string{"services"},
		Exclude: config.ExcludeConfig{
			Patterns: []string{"**/collision.go"},
		},
	}
	a := analyzer.New(cfg, "github.com/bubunyo/buildgraph/testproject", testprojectDir())
	require.NoError(t, a.Load())

	fns, graph, err := a.BuildGraph()
	require.NoError(t, err)

	for key := range fns {
		assert.NotContains(t, key, "collision",
			"function from excluded file must not appear in functions map: %s", key)
	}
	for key := range graph.Nodes {
		assert.NotContains(t, key, "collision",
			"function from excluded file must not appear in graph nodes: %s", key)
	}
}

// TestBuildGraph_ExcludePatternsPreserveOtherFunctions verifies that only the
// matched functions are removed and the rest of the graph is intact.
func TestBuildGraph_ExcludePatternsPreserveOtherFunctions(t *testing.T) {
	cfg := &config.Config{
		Services: []string{"services"},
		Exclude: config.ExcludeConfig{
			Patterns: []string{"**/collision.go"},
		},
	}
	a := analyzer.New(cfg, "github.com/bubunyo/buildgraph/testproject", testprojectDir())
	require.NoError(t, a.Load())

	fns, _, err := a.BuildGraph()
	require.NoError(t, err)

	// Functions from module-a and module-b must still be present.
	found := false
	for key := range fns {
		if strings.Contains(key, "module") {
			found = true
			break
		}
	}
	assert.True(t, found, "non-excluded functions from module-a/module-b must still be present")
}

// ── blank import / side-effect dependency ────────────────────────────────────

// TestBuildGraph_BlankImport_SideEffectTrackedInReverseIndex asserts that a
// package imported only for its side effects (import _ "pkg") creates a
// dependency edge in the call graph so that changes to it trigger a rebuild of
// the importing service.
//
// This test is expected to FAIL until the analyzer synthesises a dependency
// edge from the importer to the blank-imported package's init.
func TestBuildGraph_BlankImport_SideEffectTrackedInReverseIndex(t *testing.T) {
	a := loadedAnalyzer(t)
	_, graph, err := a.BuildGraph()
	require.NoError(t, err)

	const sideeffectPkg = "sideeffect"
	const serviceAPkg = "service-a"

	// sideeffect.init must appear in the reverse index — service-a blank-imports it.
	var sideeffectKey string
	for k := range graph.ReverseIndex {
		if strings.Contains(k, sideeffectPkg) {
			sideeffectKey = k
			break
		}
	}
	require.NotEmpty(t, sideeffectKey,
		"sideeffect.init must appear in the reverse index (service-a blank-imports it)")

	// service-a must be listed as a caller of sideeffect.
	callers := graph.ReverseIndex[sideeffectKey]
	found := false
	for _, caller := range callers {
		if strings.Contains(caller, serviceAPkg) {
			found = true
			break
		}
	}
	assert.True(t, found,
		"service-a must appear as a caller of sideeffect via blank import; callers=%v", callers)
}

// TestBuildGraph_ToolsLoadedButNotServices verifies that when the testproject
// has a tools/tool-a package (which imports a shared core module and has a
// main function), the analyzer loads it as part of the graph — but that it is
// NOT treated as a deployable service when serviceDirs is restricted to
// ["services"].
//
// service-c lives under services/ but is added to the exclude patterns, so it
// must also be absent from ServicesToBuild.
//
// This test is expected to FAIL until impact.NewAnalyzer accepts a serviceDirs
// parameter and filters the serviceSet accordingly, and until ServicesToBuild
// emits full owner paths. It asserts the currently-broken behaviour: tool-a
// appears in ServicesToBuild even though it lives under tools/, not services/.
func TestBuildGraph_ToolsLoadedButNotServices(t *testing.T) {
	// Load with both "services" and "tools" so tool-a is in the graph.
	// Exclude service-c so it must not appear in ServicesToBuild either.
	cfg := &config.Config{
		Services: []string{"services", "tools"},
		Exclude: config.ExcludeConfig{
			Patterns: []string{"**/service-c/**"},
		},
	}
	a := analyzer.New(cfg, "github.com/bubunyo/buildgraph/testproject", testprojectDir())
	require.NoError(t, a.Load())

	_, graph, err := a.BuildGraph()
	require.NoError(t, err)

	// tool-a's main must be present in the graph since we loaded tools/.
	toolMainKey := ""
	for key := range graph.Nodes {
		if strings.Contains(key, "tool-a") && strings.HasSuffix(key, ".main") {
			toolMainKey = key
			break
		}
	}
	require.NotEmpty(t, toolMainKey, "tools/tool-a main function must be present in the graph")

	// service-c must have been excluded from the graph.
	for key := range graph.Nodes {
		assert.NotContains(t, key, "service-c",
			"service-c was excluded and must not appear in graph nodes")
	}

	// Now run impact analysis restricted to serviceDirs=["services"] only.
	// tool-a's owner ("tools/tool-a") must NOT appear in ServicesToBuild even
	// though it has a main function reachable in the graph.
	//
	// Currently FAILS because impact.NewAnalyzer has no serviceDirs param and
	// treats every main as a service, emitting "tool-a" (path.Base) in the result.
	impactAnalyzer := impact.NewAnalyzer(graph, []string{"services"})

	// Simulate a change to module-a which tool-a also depends on.
	var moduleAFunc string
	for key := range graph.Nodes {
		if strings.Contains(key, "module-a") {
			moduleAFunc = key
			break
		}
	}
	require.NotEmpty(t, moduleAFunc, "module-a function must exist in graph")

	result := impactAnalyzer.ComputeImpact([]types.Change{{Function: moduleAFunc, Type: "modified"}})

	// After the fix: ServicesToBuild should contain full paths like
	// "services/service-a", "services/service-b" and NOT "tools/tool-a" or
	// "services/service-c" (excluded).
	for _, svc := range result.ServicesToBuild {
		assert.True(t,
			strings.HasPrefix(svc, "services/"),
			"ServicesToBuild must only contain services/ entries, got %q", svc,
		)
		assert.NotContains(t, svc, "service-c",
			"excluded service-c must not appear in ServicesToBuild")
	}
}
