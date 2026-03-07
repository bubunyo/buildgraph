package diff

import (
	"testing"
	"time"

	"github.com/bubunyo/buildgraph/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func makeFunction(name, astHash string, deps ...types.Dependency) *types.Function {
	return &types.Function{
		FullName: name,
		ASTHash:  astHash,
		Deps:     deps,
	}
}

func makeBaseline(nodes map[string]types.Function, hashes map[string]types.HashInfo) *types.Baseline {
	return &types.Baseline{
		Version:     "1.0",
		GeneratedAt: time.Now(),
		Graph: types.CallGraph{
			Nodes:         nodes,
			ReverseIndex:  map[string][]string{},
			FunctionOwner: map[string]string{},
		},
		FunctionHashes: hashes,
		ExternalDeps:   map[string]string{},
	}
}

func changesByType(changes []types.Change, typ string) []types.Change {
	var out []types.Change
	for _, c := range changes {
		if c.Type == typ {
			out = append(out, c)
		}
	}
	return out
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestDetectChanges_NoBaseline_AllAdded(t *testing.T) {
	current := map[string]*types.Function{
		"pkg.Foo": makeFunction("pkg.Foo", "hash-foo"),
		"pkg.Bar": makeFunction("pkg.Bar", "hash-bar"),
	}

	d := NewDetector(current, nil, "", nil)
	changes := d.DetectChanges()

	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
	for _, c := range changes {
		if c.Type != "added" {
			t.Errorf("expected type 'added', got %q", c.Type)
		}
		if c.Reason != "new_function" {
			t.Errorf("expected reason 'new_function', got %q", c.Reason)
		}
	}
}

func TestDetectChanges_NoChanges(t *testing.T) {
	current := map[string]*types.Function{
		"pkg.Foo": makeFunction("pkg.Foo", "hash-foo"),
	}
	nodes := map[string]types.Function{
		"pkg.Foo": *makeFunction("pkg.Foo", "hash-foo"),
	}
	hashes := map[string]types.HashInfo{
		"pkg.Foo": {ASTHash: "hash-foo"},
	}

	d := NewDetector(current, nil, "", makeBaseline(nodes, hashes))
	changes := d.DetectChanges()

	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %d: %+v", len(changes), changes)
	}
}

func TestDetectChanges_NewFunction(t *testing.T) {
	current := map[string]*types.Function{
		"pkg.Foo": makeFunction("pkg.Foo", "hash-foo"),
		"pkg.New": makeFunction("pkg.New", "hash-new"),
	}
	nodes := map[string]types.Function{
		"pkg.Foo": *makeFunction("pkg.Foo", "hash-foo"),
	}
	hashes := map[string]types.HashInfo{
		"pkg.Foo": {ASTHash: "hash-foo"},
	}

	d := NewDetector(current, nil, "", makeBaseline(nodes, hashes))
	changes := d.DetectChanges()

	added := changesByType(changes, "added")
	if len(added) != 1 {
		t.Fatalf("expected 1 added change, got %d", len(added))
	}
	if added[0].Function != "pkg.New" {
		t.Errorf("expected pkg.New to be added, got %q", added[0].Function)
	}
}

func TestDetectChanges_RemovedFunction(t *testing.T) {
	current := map[string]*types.Function{
		"pkg.Foo": makeFunction("pkg.Foo", "hash-foo"),
	}
	nodes := map[string]types.Function{
		"pkg.Foo": *makeFunction("pkg.Foo", "hash-foo"),
		"pkg.Old": *makeFunction("pkg.Old", "hash-old"),
	}
	hashes := map[string]types.HashInfo{
		"pkg.Foo": {ASTHash: "hash-foo"},
		"pkg.Old": {ASTHash: "hash-old"},
	}

	d := NewDetector(current, nil, "", makeBaseline(nodes, hashes))
	changes := d.DetectChanges()

	removed := changesByType(changes, "removed")
	if len(removed) != 1 {
		t.Fatalf("expected 1 removed change, got %d", len(removed))
	}
	if removed[0].Function != "pkg.Old" {
		t.Errorf("expected pkg.Old to be removed, got %q", removed[0].Function)
	}
}

func TestDetectChanges_ModifiedFunction(t *testing.T) {
	current := map[string]*types.Function{
		"pkg.Foo": makeFunction("pkg.Foo", "hash-foo-new"),
	}
	nodes := map[string]types.Function{
		"pkg.Foo": *makeFunction("pkg.Foo", "hash-foo-old"),
	}
	hashes := map[string]types.HashInfo{
		"pkg.Foo": {ASTHash: "hash-foo-old"},
	}

	d := NewDetector(current, nil, "", makeBaseline(nodes, hashes))
	changes := d.DetectChanges()

	modified := changesByType(changes, "modified")
	if len(modified) != 1 {
		t.Fatalf("expected 1 modified change, got %d", len(modified))
	}
	c := modified[0]
	if c.Function != "pkg.Foo" {
		t.Errorf("expected pkg.Foo, got %q", c.Function)
	}
	if c.Reason != "ast_hash_changed" {
		t.Errorf("expected reason 'ast_hash_changed', got %q", c.Reason)
	}
	if c.OldHash != "hash-foo-old" {
		t.Errorf("unexpected OldHash: %q", c.OldHash)
	}
	if c.NewHash != "hash-foo-new" {
		t.Errorf("unexpected NewHash: %q", c.NewHash)
	}
}

func TestDetectChanges_ExternalDepChanged(t *testing.T) {
	extDep := types.Dependency{
		Type:     "external",
		FullName: "github.com/foo/bar.DoThing",
		Package:  types.Package{Path: "github.com/foo/bar"},
	}
	current := map[string]*types.Function{
		"pkg.Foo": makeFunction("pkg.Foo", "hash-foo", extDep),
	}
	nodes := map[string]types.Function{
		"pkg.Foo": *makeFunction("pkg.Foo", "hash-foo", extDep),
	}
	hashes := map[string]types.HashInfo{
		"pkg.Foo": {ASTHash: "hash-foo", ExternalDeps: []string{"github.com/foo/bar"}},
	}

	baseline := makeBaseline(nodes, hashes)
	baseline.ExternalDeps = map[string]string{"github.com/foo/bar": "v1.0.0"}
	baseline.ExternalDepsHash = "old-hash"

	currentExtDeps := map[string]string{"github.com/foo/bar": "v2.0.0"}

	d := NewDetector(current, currentExtDeps, "new-hash", baseline)
	changes := d.DetectChanges()

	extChanges := changesByType(changes, "external_dep_changed")
	if len(extChanges) != 1 {
		t.Fatalf("expected 1 external_dep_changed, got %d", len(extChanges))
	}
	c := extChanges[0]
	if c.OldVer != "v1.0.0" {
		t.Errorf("expected OldVer v1.0.0, got %q", c.OldVer)
	}
	if c.NewVer != "v2.0.0" {
		t.Errorf("expected NewVer v2.0.0, got %q", c.NewVer)
	}
}

// TestDetectChanges_ExternalDepRemoved covers the "removal" branch in
// externalDepChanges where a package present in the baseline disappears from
// the current go.mod.
func TestDetectChanges_ExternalDepRemoved(t *testing.T) {
	// A function that used to depend on github.com/foo/bar (now removed).
	extDep := types.Dependency{
		Type:     "external",
		FullName: "github.com/foo/bar.DoThing",
		Package:  types.Package{Path: "github.com/foo/bar"},
	}
	current := map[string]*types.Function{
		"pkg.Foo": makeFunction("pkg.Foo", "hash-foo", extDep),
	}
	nodes := map[string]types.Function{
		"pkg.Foo": *makeFunction("pkg.Foo", "hash-foo", extDep),
	}
	hashes := map[string]types.HashInfo{
		"pkg.Foo": {ASTHash: "hash-foo"},
	}

	baseline := makeBaseline(nodes, hashes)
	baseline.ExternalDeps = map[string]string{"github.com/foo/bar": "v1.0.0"}
	baseline.ExternalDepsHash = "old-hash"

	// Current: the dep is gone from go.mod entirely.
	currentExtDeps := map[string]string{}

	d := NewDetector(current, currentExtDeps, "new-hash", baseline)
	changes := d.DetectChanges()

	extChanges := changesByType(changes, "external_dep_changed")
	if len(extChanges) != 1 {
		t.Fatalf("expected 1 external_dep_changed for removed dep, got %d: %+v", len(extChanges), changes)
	}
	c := extChanges[0]
	if c.OldVer != "v1.0.0" {
		t.Errorf("expected OldVer v1.0.0, got %q", c.OldVer)
	}
	if c.NewVer != "" {
		t.Errorf("expected empty NewVer for removed dep, got %q", c.NewVer)
	}
}

// TestDetectChanges_ExternalDepChanged_InternalDepSkipped covers the branch
// inside externalDepChanges where a function's dep has Type != "external" and
// is therefore skipped.
func TestDetectChanges_ExternalDepChanged_InternalDepSkipped(t *testing.T) {
	internalDep := types.Dependency{
		Type:     "internal",
		FullName: "mypkg.Helper",
		Package:  types.Package{Path: "github.com/me/repo/mypkg"},
	}
	current := map[string]*types.Function{
		// This function only has an internal dep — no external dep to match.
		"pkg.Foo": makeFunction("pkg.Foo", "hash-foo", internalDep),
	}
	nodes := map[string]types.Function{
		"pkg.Foo": *makeFunction("pkg.Foo", "hash-foo"),
	}
	hashes := map[string]types.HashInfo{
		"pkg.Foo": {ASTHash: "hash-foo"},
	}

	baseline := makeBaseline(nodes, hashes)
	// Simulate an external dep hash change so externalDepChanges is called.
	baseline.ExternalDeps = map[string]string{"github.com/foo/bar": "v1.0.0"}
	baseline.ExternalDepsHash = "old-hash"
	currentExtDeps := map[string]string{"github.com/foo/bar": "v2.0.0"}

	d := NewDetector(current, currentExtDeps, "new-hash", baseline)
	changes := d.DetectChanges()

	// pkg.Foo has no external dep, so no external_dep_changed change for it.
	extChanges := changesByType(changes, "external_dep_changed")
	if len(extChanges) != 0 {
		t.Errorf("expected 0 external_dep_changed (only internal dep), got %d: %+v", len(extChanges), extChanges)
	}
}

func TestDetectChanges_ExternalDepUnchanged_NoChange(t *testing.T) {
	extDep := types.Dependency{
		Type:    "external",
		Package: types.Package{Path: "github.com/foo/bar"},
	}
	current := map[string]*types.Function{
		"pkg.Foo": makeFunction("pkg.Foo", "hash-foo", extDep),
	}
	nodes := map[string]types.Function{
		"pkg.Foo": *makeFunction("pkg.Foo", "hash-foo", extDep),
	}
	hashes := map[string]types.HashInfo{
		"pkg.Foo": {ASTHash: "hash-foo"},
	}

	baseline := makeBaseline(nodes, hashes)
	baseline.ExternalDeps = map[string]string{"github.com/foo/bar": "v1.0.0"}
	baseline.ExternalDepsHash = "same-hash"

	d := NewDetector(current, map[string]string{"github.com/foo/bar": "v1.0.0"}, "same-hash", baseline)
	changes := d.DetectChanges()

	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %d: %+v", len(changes), changes)
	}
}
