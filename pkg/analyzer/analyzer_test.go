package analyzer_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/bubunyo/buildgraph/pkg/analyzer"
	"github.com/bubunyo/buildgraph/pkg/config"
	"github.com/bubunyo/buildgraph/pkg/types"
)

// testprojectDir returns the absolute path to the testproject fixture.
func testprojectDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile = .../buildgraph/pkg/analyzer/analyzer_test.go
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(repoRoot, "testproject")
}

func newTestAnalyzer() *analyzer.Analyzer {
	cfg := &config.Config{
		Services: []string{"services"},
	}
	return analyzer.New(cfg, "github.com/bubunyo/buildgraph/testproject", testprojectDir())
}

// loadedAnalyzer returns an Analyzer that has already called Load().
func loadedAnalyzer(t *testing.T) *analyzer.Analyzer {
	t.Helper()
	a := newTestAnalyzer()
	if err := a.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	return a
}

// ── New ───────────────────────────────────────────────────────────────────────

func TestNew_ReturnsNonNil(t *testing.T) {
	a := newTestAnalyzer()
	if a == nil {
		t.Fatal("expected non-nil Analyzer")
	}
}

// ── Load ──────────────────────────────────────────────────────────────────────

func TestLoad_Succeeds(t *testing.T) {
	if err := newTestAnalyzer().Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
}

// ── BuildGraph ────────────────────────────────────────────────────────────────

func TestBuildGraph_WithoutLoad_ReturnsError(t *testing.T) {
	_, _, err := newTestAnalyzer().BuildGraph()
	if err == nil {
		t.Fatal("expected error when calling BuildGraph before Load")
	}
}

func TestBuildGraph_ReturnsNonEmptyGraph(t *testing.T) {
	a := loadedAnalyzer(t)

	fns, graph, err := a.BuildGraph()
	if err != nil {
		t.Fatalf("BuildGraph() error: %v", err)
	}
	if len(fns) == 0 {
		t.Error("expected at least one function in the graph")
	}
	if graph == nil {
		t.Fatal("expected non-nil CallGraph")
	}
	if len(graph.Nodes) == 0 {
		t.Error("expected at least one node in the call graph")
	}
}

func TestBuildGraph_OnlyContainsInternalFunctions(t *testing.T) {
	a := loadedAnalyzer(t)

	fns, _, err := a.BuildGraph()
	if err != nil {
		t.Fatalf("BuildGraph() error: %v", err)
	}

	for key := range fns {
		if !strings.Contains(key, "github.com/bubunyo/buildgraph/testproject") {
			t.Errorf("unexpected external function in map: %s", key)
		}
	}
}

func TestBuildGraph_FunctionOwnerIsPopulated(t *testing.T) {
	a := loadedAnalyzer(t)

	_, graph, err := a.BuildGraph()
	if err != nil {
		t.Fatalf("BuildGraph() error: %v", err)
	}

	if len(graph.FunctionOwner) == 0 {
		t.Error("expected FunctionOwner map to be populated")
	}
	for key, owner := range graph.FunctionOwner {
		if owner == "" {
			t.Errorf("function %s has empty owner", key)
		}
	}
}

func TestBuildGraph_ReverseIndexPopulated(t *testing.T) {
	a := loadedAnalyzer(t)

	_, graph, err := a.BuildGraph()
	if err != nil {
		t.Fatalf("BuildGraph() error: %v", err)
	}

	// module-a is called by both services; its functions should appear in
	// the reverse index.
	found := false
	for key := range graph.ReverseIndex {
		if strings.Contains(key, "module") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected reverse index to contain entries for shared module functions")
	}
}

// ── ComputeHashes ─────────────────────────────────────────────────────────────

func TestComputeHashes_Succeeds(t *testing.T) {
	a := loadedAnalyzer(t)

	fns, _, err := a.BuildGraph()
	if err != nil {
		t.Fatalf("BuildGraph() error: %v", err)
	}
	if err := a.ComputeHashes(fns, nil, nil); err != nil {
		t.Fatalf("ComputeHashes() error: %v", err)
	}
}

func TestComputeHashes_FillsASTHash(t *testing.T) {
	a := loadedAnalyzer(t)

	fns, _, err := a.BuildGraph()
	if err != nil {
		t.Fatalf("BuildGraph() error: %v", err)
	}
	if err := a.ComputeHashes(fns, nil, nil); err != nil {
		t.Fatalf("ComputeHashes() error: %v", err)
	}

	filled := 0
	for _, fn := range fns {
		if fn.ASTHash != "" {
			filled++
		}
	}
	if filled == 0 {
		t.Error("expected at least one function with a non-empty ASTHash")
	}
}

func TestComputeHashes_FillsTransitiveHash(t *testing.T) {
	a := loadedAnalyzer(t)

	fns, _, err := a.BuildGraph()
	if err != nil {
		t.Fatalf("BuildGraph() error: %v", err)
	}
	if err := a.ComputeHashes(fns, nil, nil); err != nil {
		t.Fatalf("ComputeHashes() error: %v", err)
	}

	for _, fn := range fns {
		if fn.TransitiveHash == "" {
			t.Errorf("function %s has empty TransitiveHash", fn.FullName)
		}
	}
}

func TestComputeHashes_Deterministic(t *testing.T) {
	load := func() map[string]string {
		a := loadedAnalyzer(t)
		fns, _, err := a.BuildGraph()
		if err != nil {
			t.Fatalf("BuildGraph() error: %v", err)
		}
		if err := a.ComputeHashes(fns, nil, nil); err != nil {
			t.Fatalf("ComputeHashes() error: %v", err)
		}
		hashes := make(map[string]string, len(fns))
		for k, fn := range fns {
			hashes[k] = fn.ASTHash
		}
		return hashes
	}

	h1, h2 := load(), load()
	for k, v := range h1 {
		if h2[k] != v {
			t.Errorf("hash for %s is not deterministic: %q vs %q", k, v, h2[k])
		}
	}
}

func TestComputeHashes_ReusesPrevHashWhenFileUnchanged(t *testing.T) {
	// First run — compute the baseline hashes.
	a1 := loadedAnalyzer(t)
	fns1, _, err := a1.BuildGraph()
	if err != nil {
		t.Fatalf("BuildGraph() error: %v", err)
	}
	if err := a1.ComputeHashes(fns1, nil, nil); err != nil {
		t.Fatalf("ComputeHashes() error: %v", err)
	}
	prevSource, err := a1.ComputeSourceHashes()
	if err != nil {
		t.Fatalf("ComputeSourceHashes() error: %v", err)
	}

	// Build prevFuncHashes from the first run.
	prevFuncHashes := make(map[string]types.HashInfo, len(fns1))
	for k, fn := range fns1 {
		prevFuncHashes[k] = types.HashInfo{ASTHash: fn.ASTHash, TransitiveHash: fn.TransitiveHash}
	}

	// Second run — pass the previous source+func hashes so the analyzer can
	// reuse cached ASTHashes for unchanged files.
	a2 := loadedAnalyzer(t)
	fns2, _, err := a2.BuildGraph()
	if err != nil {
		t.Fatalf("BuildGraph() (2nd) error: %v", err)
	}
	if err := a2.ComputeHashes(fns2, prevSource, prevFuncHashes); err != nil {
		t.Fatalf("ComputeHashes() (2nd) error: %v", err)
	}

	// Hashes must match — the cache should have been hit for every function.
	for k, fn2 := range fns2 {
		if fn1, ok := fns1[k]; ok && fn1.ASTHash != "" {
			if fn2.ASTHash != fn1.ASTHash {
				t.Errorf("ASTHash mismatch for %s: first=%q second=%q", k, fn1.ASTHash, fn2.ASTHash)
			}
		}
	}
}

// ── ComputeSourceHashes ───────────────────────────────────────────────────────

func TestComputeSourceHashes_ReturnsNonEmpty(t *testing.T) {
	a := loadedAnalyzer(t)

	hashes, err := a.ComputeSourceHashes()
	if err != nil {
		t.Fatalf("ComputeSourceHashes() error: %v", err)
	}
	if len(hashes) == 0 {
		t.Error("expected non-empty source hash map")
	}
	for path, hash := range hashes {
		if !strings.HasPrefix(hash, "sha256:") {
			t.Errorf("hash for %s does not start with sha256: got %q", path, hash)
		}
	}
}

func TestComputeSourceHashes_Deterministic(t *testing.T) {
	run := func() map[string]string {
		a := loadedAnalyzer(t)
		h, err := a.ComputeSourceHashes()
		if err != nil {
			t.Fatalf("ComputeSourceHashes() error: %v", err)
		}
		return h
	}
	h1, h2 := run(), run()
	for k, v := range h1 {
		if h2[k] != v {
			t.Errorf("source hash for %s is not deterministic", k)
		}
	}
}

// ── ExtractExternalDeps ───────────────────────────────────────────────────────

func TestExtractExternalDeps_Succeeds(t *testing.T) {
	// ExtractExternalDeps does not require Load().
	_, _, err := newTestAnalyzer().ExtractExternalDeps()
	if err != nil {
		t.Fatalf("ExtractExternalDeps() error: %v", err)
	}
}

func TestExtractExternalDeps_ReturnsNonEmptyHash(t *testing.T) {
	_, hash, err := newTestAnalyzer().ExtractExternalDeps()
	if err != nil {
		t.Fatalf("ExtractExternalDeps() error: %v", err)
	}
	// testproject/go.mod has no require block, but the hash must be a
	// non-empty deterministic string produced by HashGoMod.
	if hash == "" {
		t.Error("expected non-empty hash from ExtractExternalDeps")
	}
}
