package diff

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	changes := NewDetector(current, nil, "", nil).DetectChanges()

	require.Len(t, changes, 2)
	for _, c := range changes {
		assert.Equal(t, "added", c.Type)
		assert.Equal(t, "new_function", c.Reason)
	}
}

func TestDetectChanges_NoChanges(t *testing.T) {
	current := map[string]*types.Function{
		"pkg.Foo": makeFunction("pkg.Foo", "hash-foo"),
	}
	nodes := map[string]types.Function{"pkg.Foo": *makeFunction("pkg.Foo", "hash-foo")}
	hashes := map[string]types.HashInfo{"pkg.Foo": {ASTHash: "hash-foo"}}

	changes := NewDetector(current, nil, "", makeBaseline(nodes, hashes)).DetectChanges()

	assert.Empty(t, changes)
}

func TestDetectChanges_NewFunction(t *testing.T) {
	current := map[string]*types.Function{
		"pkg.Foo": makeFunction("pkg.Foo", "hash-foo"),
		"pkg.New": makeFunction("pkg.New", "hash-new"),
	}
	nodes := map[string]types.Function{"pkg.Foo": *makeFunction("pkg.Foo", "hash-foo")}
	hashes := map[string]types.HashInfo{"pkg.Foo": {ASTHash: "hash-foo"}}

	changes := NewDetector(current, nil, "", makeBaseline(nodes, hashes)).DetectChanges()

	added := changesByType(changes, "added")
	require.Len(t, added, 1)
	assert.Equal(t, "pkg.New", added[0].Function)
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

	changes := NewDetector(current, nil, "", makeBaseline(nodes, hashes)).DetectChanges()

	removed := changesByType(changes, "removed")
	require.Len(t, removed, 1)
	assert.Equal(t, "pkg.Old", removed[0].Function)
}

func TestDetectChanges_ModifiedFunction(t *testing.T) {
	current := map[string]*types.Function{
		"pkg.Foo": makeFunction("pkg.Foo", "hash-foo-new"),
	}
	nodes := map[string]types.Function{"pkg.Foo": *makeFunction("pkg.Foo", "hash-foo-old")}
	hashes := map[string]types.HashInfo{"pkg.Foo": {ASTHash: "hash-foo-old"}}

	changes := NewDetector(current, nil, "", makeBaseline(nodes, hashes)).DetectChanges()

	modified := changesByType(changes, "modified")
	require.Len(t, modified, 1)
	c := modified[0]
	assert.Equal(t, "pkg.Foo", c.Function)
	assert.Equal(t, "ast_hash_changed", c.Reason)
	assert.Equal(t, "hash-foo-old", c.OldHash)
	assert.Equal(t, "hash-foo-new", c.NewHash)
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
	nodes := map[string]types.Function{"pkg.Foo": *makeFunction("pkg.Foo", "hash-foo", extDep)}
	hashes := map[string]types.HashInfo{
		"pkg.Foo": {ASTHash: "hash-foo", ExternalDeps: []string{"github.com/foo/bar"}},
	}

	baseline := makeBaseline(nodes, hashes)
	baseline.ExternalDeps = map[string]string{"github.com/foo/bar": "v1.0.0"}
	baseline.ExternalDepsHash = "old-hash"

	changes := NewDetector(current, map[string]string{"github.com/foo/bar": "v2.0.0"}, "new-hash", baseline).DetectChanges()

	extChanges := changesByType(changes, "external_dep_changed")
	require.Len(t, extChanges, 1)
	assert.Equal(t, "v1.0.0", extChanges[0].OldVer)
	assert.Equal(t, "v2.0.0", extChanges[0].NewVer)
}

// TestDetectChanges_ExternalDepRemoved covers the "removal" branch in
// externalDepChanges where a package present in the baseline disappears from
// the current go.mod.
func TestDetectChanges_ExternalDepRemoved(t *testing.T) {
	extDep := types.Dependency{
		Type:     "external",
		FullName: "github.com/foo/bar.DoThing",
		Package:  types.Package{Path: "github.com/foo/bar"},
	}
	current := map[string]*types.Function{
		"pkg.Foo": makeFunction("pkg.Foo", "hash-foo", extDep),
	}
	nodes := map[string]types.Function{"pkg.Foo": *makeFunction("pkg.Foo", "hash-foo", extDep)}
	hashes := map[string]types.HashInfo{"pkg.Foo": {ASTHash: "hash-foo"}}

	baseline := makeBaseline(nodes, hashes)
	baseline.ExternalDeps = map[string]string{"github.com/foo/bar": "v1.0.0"}
	baseline.ExternalDepsHash = "old-hash"

	// Current: the dep is gone from go.mod entirely.
	changes := NewDetector(current, map[string]string{}, "new-hash", baseline).DetectChanges()

	extChanges := changesByType(changes, "external_dep_changed")
	require.Len(t, extChanges, 1)
	assert.Equal(t, "v1.0.0", extChanges[0].OldVer)
	assert.Equal(t, "", extChanges[0].NewVer)
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
		"pkg.Foo": makeFunction("pkg.Foo", "hash-foo", internalDep),
	}
	nodes := map[string]types.Function{"pkg.Foo": *makeFunction("pkg.Foo", "hash-foo")}
	hashes := map[string]types.HashInfo{"pkg.Foo": {ASTHash: "hash-foo"}}

	baseline := makeBaseline(nodes, hashes)
	baseline.ExternalDeps = map[string]string{"github.com/foo/bar": "v1.0.0"}
	baseline.ExternalDepsHash = "old-hash"

	changes := NewDetector(current, map[string]string{"github.com/foo/bar": "v2.0.0"}, "new-hash", baseline).DetectChanges()

	// pkg.Foo has no external dep, so no external_dep_changed change for it.
	assert.Empty(t, changesByType(changes, "external_dep_changed"))
}

func TestDetectChanges_ExternalDepUnchanged_NoChange(t *testing.T) {
	extDep := types.Dependency{
		Type:    "external",
		Package: types.Package{Path: "github.com/foo/bar"},
	}
	current := map[string]*types.Function{
		"pkg.Foo": makeFunction("pkg.Foo", "hash-foo", extDep),
	}
	nodes := map[string]types.Function{"pkg.Foo": *makeFunction("pkg.Foo", "hash-foo", extDep)}
	hashes := map[string]types.HashInfo{"pkg.Foo": {ASTHash: "hash-foo"}}

	baseline := makeBaseline(nodes, hashes)
	baseline.ExternalDeps = map[string]string{"github.com/foo/bar": "v1.0.0"}
	baseline.ExternalDepsHash = "same-hash"

	changes := NewDetector(current, map[string]string{"github.com/foo/bar": "v1.0.0"}, "same-hash", baseline).DetectChanges()

	assert.Empty(t, changes)
}
