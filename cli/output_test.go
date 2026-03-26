package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bubunyo/buildgraph/pkg/types"
)

func baseResult(hasChanges bool, changes []types.Change, services []string) *types.Result {
	return &types.Result{
		Timestamp:     time.Now(),
		CurrentCommit: "abc123",
		HasChanges:    hasChanges,
		Changes:       changes,
		Impact: types.Impact{
			ServicesToBuild: services,
		},
	}
}

func TestFormatText_NoChanges(t *testing.T) {
	out := formatText(baseResult(false, nil, nil))
	assert.Contains(t, out, "Has changes : false")
	assert.Contains(t, out, "Services to build (0)")
}

func TestFormatText_WithChanges(t *testing.T) {
	changes := []types.Change{
		{Function: "core.Foo", Type: "modified", Reason: "ast_hash_changed"},
	}
	out := formatText(baseResult(true, changes, []string{"service-a"}))
	assert.Contains(t, out, "Has changes : true")
	assert.Contains(t, out, "Changes (1)")
	assert.Contains(t, out, "core.Foo")
	assert.Contains(t, out, "ast_hash_changed")
	assert.Contains(t, out, "Services to build (1)")
	assert.Contains(t, out, "service-a")
}

func TestFormatText_ExternalDepChange(t *testing.T) {
	changes := []types.Change{
		{
			Function: "core.Foo",
			Type:     "external_dep_changed",
			Reason:   "external_dependency_version_changed",
			Package:  "github.com/pkg/foo",
			OldVer:   "v1.0.0",
			NewVer:   "v2.0.0",
		},
	}
	out := formatText(baseResult(true, changes, []string{"service-a"}))
	assert.Contains(t, out, "github.com/pkg/foo")
	assert.Contains(t, out, "v1.0.0")
	assert.Contains(t, out, "v2.0.0")
}

func TestFormatText_ServicesSorted(t *testing.T) {
	out := formatText(baseResult(true, nil, []string{"svc-c", "svc-a", "svc-b"}))

	idxA := strings.Index(out, "svc-a")
	idxB := strings.Index(out, "svc-b")
	idxC := strings.Index(out, "svc-c")

	require.True(t, idxA >= 0 && idxB >= 0 && idxC >= 0, "expected all services in output:\n%s", out)
	assert.True(t, idxA < idxB && idxB < idxC, "expected services sorted a < b < c in output:\n%s", out)
}

// TestFormatDot_ServiceMarkedRebuild_WithFullPath asserts that when
// ServicesToBuild contains a full owner path (e.g. "services/svc-a"), the
// corresponding cluster in the DOT output is marked [rebuild].
//
// This test is expected to FAIL because formatDot currently extracts only the
// last path segment (shortOwner) for the rebuiltServices lookup, so a full-path
// key like "services/svc-a" never matches.
func TestFormatDot_ServiceMarkedRebuild_WithFullPath(t *testing.T) {
	result := &types.Result{
		HasChanges: true,
		Changes:    []types.Change{{Function: "core.Fn", Type: "modified"}},
		Impact: types.Impact{
			ServicesToBuild: []string{"services/svc-a"},
			AffectedFunctions: map[string][]string{
				"services/svc-a": {"services/svc-a.main"},
			},
		},
	}
	graph := &types.CallGraph{
		Nodes: map[string]types.Function{
			"services/svc-a.main": {FullName: "services/svc-a.main", IsMain: true},
		},
	}

	out := formatDot(result, graph)

	assert.Contains(t, out, "[rebuild]", "cluster for services/svc-a should be marked [rebuild]")
}

// TestFormatDot_NodeIDsAreQuoted asserts that every node ID in the DOT output
// is a double-quoted string rather than a bare identifier.  Go function keys
// contain characters that are invalid in unquoted DOT IDs (*, #, (, ), /, @)
// and must always be quoted so that `dot -Tpng` does not reject the output.
func TestFormatDot_NodeIDsAreQuoted(t *testing.T) {
	// Use function keys that contain the problematic characters.
	result := &types.Result{
		HasChanges: true,
		Changes: []types.Change{
			{Function: "(*pkg.Type).Method", Type: "modified"},
			{Function: "pkg.init#1", Type: "modified"},
		},
		Impact: types.Impact{
			ServicesToBuild: []string{"services/svc"},
			AffectedFunctions: map[string][]string{
				"services/svc": {
					"(*pkg.Type).Method",
					"pkg.init#1",
					"services/svc.main",
				},
			},
		},
	}
	graph := &types.CallGraph{
		Nodes: map[string]types.Function{
			"(*pkg.Type).Method": {FullName: "(*pkg.Type).Method"},
			"pkg.init#1":         {FullName: "pkg.init#1"},
			"services/svc.main":  {FullName: "services/svc.main", IsMain: true},
		},
	}

	out := formatDot(result, graph)

	// Every line that declares a node or edge must use quoted IDs.
	// Quoted IDs start with '"'; bare identifiers that contain special chars
	// would start directly with *, (, or a letter followed by # etc.
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		// Skip comments, subgraph lines, attribute lines, and blank lines.
		if trimmed == "" ||
			strings.HasPrefix(trimmed, "//") ||
			strings.HasPrefix(trimmed, "digraph") ||
			strings.HasPrefix(trimmed, "subgraph") ||
			strings.HasPrefix(trimmed, "rankdir") ||
			strings.HasPrefix(trimmed, "node ") ||
			strings.HasPrefix(trimmed, "edge ") ||
			strings.HasPrefix(trimmed, "label=") ||
			strings.HasPrefix(trimmed, "style=") ||
			strings.HasPrefix(trimmed, "color=") ||
			strings.HasPrefix(trimmed, "{") ||
			strings.HasPrefix(trimmed, "}") {
			continue
		}
		// Any remaining non-blank line that references a node ID must start
		// with a double-quote (quoted identifier).
		assert.True(t, strings.HasPrefix(trimmed, `"`),
			"expected quoted node ID on line: %q", trimmed)
	}

	// Spot-check: the raw special characters must not appear unquoted.
	// They may appear inside quoted strings, but not as bare tokens.
	// We verify by checking the output is accepted as valid DOT syntax —
	// proxy this by ensuring no bare `*` or `#` appears at the start of a token.
	assert.NotContains(t, out, "\n  *", "bare * must not start a node ID line")
	assert.NotContains(t, out, "\n    *", "bare * must not start a node ID line inside cluster")
}

// TestShortLabel_NormalForm checks that a plain fully-qualified function name
// loses its import path prefix and retains only "pkg.Func".
func TestShortLabel_NormalForm(t *testing.T) {
	assert.Equal(t, "collision.Run", shortLabel("github.com/org/repo/collision.Run"))
	assert.Equal(t, "sub.Func", shortLabel("github.com/org/pkg/sub.Func"))
	assert.Equal(t, "pkg.Func", shortLabel("pkg.Func")) // no slash — returned as-is
	assert.Equal(t, "sideeffect.init#1", shortLabel("github.com/org/repo/core/sideeffect.init#1"))
}

// TestShortLabel_PointerReceiverForm checks that pointer-receiver method keys
// are rendered correctly.  The import path inside the parens must be stripped
// while keeping the leading "(*" and closing ").<Method>" intact.
//
// Regression: the previous split-on-"/" approach produced "collision.A).Run"
// (dropping the "(*" prefix) instead of "(*collision.A).Run".
func TestShortLabel_PointerReceiverForm(t *testing.T) {
	assert.Equal(t, "(*collision.A).Run",
		shortLabel("(*github.com/org/repo/collision.A).Run"))
	assert.Equal(t, "(*sub.Type).Method",
		shortLabel("(*github.com/org/pkg/sub.Type).Method"))
	// No path prefix inside the parens — returned unchanged inside parens.
	assert.Equal(t, "(*pkg.Type).Method",
		shortLabel("(*pkg.Type).Method"))
}

// ── formatFullDot ─────────────────────────────────────────────────────────────

func fullDotGraph() *types.CallGraph {
	return &types.CallGraph{
		Nodes: map[string]types.Function{
			"github.com/org/repo/services/svc-a.main": {
				FullName: "github.com/org/repo/services/svc-a.main",
				IsMain:   true,
				Deps: []types.Dependency{
					{FullName: "github.com/org/repo/core/lib.Process"},
				},
			},
			"github.com/org/repo/core/lib.Process": {
				FullName: "github.com/org/repo/core/lib.Process",
				Deps: []types.Dependency{
					{FullName: "github.com/org/repo/core/lib.Helper"},
				},
			},
			"github.com/org/repo/core/lib.Helper": {
				FullName: "github.com/org/repo/core/lib.Helper",
			},
			"(*github.com/org/repo/core/lib.T).Run": {
				FullName: "(*github.com/org/repo/core/lib.T).Run",
			},
		},
		FunctionOwner: map[string]string{
			"github.com/org/repo/services/svc-a.main": "services/svc-a",
			"github.com/org/repo/core/lib.Process":    "core/lib",
			"github.com/org/repo/core/lib.Helper":     "core/lib",
			"(*github.com/org/repo/core/lib.T).Run":   "core/lib",
		},
	}
}

// TestFormatFullDot_AllNodesPresent asserts that every key in graph.Nodes
// appears as a quoted node ID somewhere in the DOT output.
func TestFormatFullDot_AllNodesPresent(t *testing.T) {
	graph := fullDotGraph()
	out := formatFullDot(graph, false)

	for key := range graph.Nodes {
		escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(key)
		quotedID := `"` + escaped + `"`
		assert.Contains(t, out, quotedID,
			"node %q must appear as a quoted ID in the DOT output", key)
	}
}

// TestFormatFullDot_EdgesEmitted asserts that every Deps entry in graph.Nodes
// produces a corresponding "->" edge line in the DOT output.
func TestFormatFullDot_EdgesEmitted(t *testing.T) {
	graph := fullDotGraph()
	out := formatFullDot(graph, false)

	// svc-a.main → lib.Process (cross-cluster edge)
	assert.Contains(t, out,
		`"github.com/org/repo/services/svc-a.main" -> "github.com/org/repo/core/lib.Process"`,
		"cross-cluster edge svc-a.main → lib.Process must be present")

	// lib.Process → lib.Helper (intra-cluster edge)
	assert.Contains(t, out,
		`"github.com/org/repo/core/lib.Process" -> "github.com/org/repo/core/lib.Helper"`,
		"intra-cluster edge lib.Process → lib.Helper must be present")
}

// TestFormatFullDot_ClustersPerOwner asserts that there is exactly one
// subgraph cluster per distinct owner in graph.FunctionOwner.
func TestFormatFullDot_ClustersPerOwner(t *testing.T) {
	graph := fullDotGraph()
	out := formatFullDot(graph, false)

	// Two distinct owners: "services/svc-a" and "core/lib".
	assert.Contains(t, out, `"services/svc-a"`, "cluster label services/svc-a must appear")
	assert.Contains(t, out, `"core/lib"`, "cluster label core/lib must appear")

	// Count subgraph declarations — must be exactly 2.
	count := strings.Count(out, "subgraph cluster_")
	assert.Equal(t, 2, count, "expected exactly 2 subgraph clusters, got %d", count)
}

// TestFormatFullDot_NodeIDsQuoted asserts that all node IDs in the DOT output
// are double-quoted, including keys containing special characters (* # /).
func TestFormatFullDot_NodeIDsQuoted(t *testing.T) {
	graph := fullDotGraph()
	out := formatFullDot(graph, false)

	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" ||
			strings.HasPrefix(trimmed, "digraph") ||
			strings.HasPrefix(trimmed, "subgraph") ||
			strings.HasPrefix(trimmed, "rankdir") ||
			strings.HasPrefix(trimmed, "node ") ||
			strings.HasPrefix(trimmed, "edge ") ||
			strings.HasPrefix(trimmed, "label=") ||
			strings.HasPrefix(trimmed, "style=") ||
			strings.HasPrefix(trimmed, "color=") ||
			strings.HasPrefix(trimmed, "//") ||
			strings.HasPrefix(trimmed, "{") ||
			strings.HasPrefix(trimmed, "}") {
			continue
		}
		assert.True(t, strings.HasPrefix(trimmed, `"`),
			"expected quoted node ID on line: %q", trimmed)
	}
}

// TestFormatFullDot_MainNodeHighlighted asserts that main-package entry points
// are rendered with the light-blue fill colour (#d0e8ff).
func TestFormatFullDot_MainNodeHighlighted(t *testing.T) {
	graph := fullDotGraph()
	out := formatFullDot(graph, false)

	assert.Contains(t, out, `fillcolor="#d0e8ff"`,
		"main function node must use light-blue fill to mark it as an entry point")
	// Non-main nodes must not carry the main highlight.
	assert.NotContains(t, out,
		`"github.com/org/repo/core/lib.Process" [label="lib.Process", fillcolor="#d0e8ff"`,
		"non-main node must not use the main-highlight fill colour")
}

// stdlibDotGraph returns a graph that has one internal dep and one stdlib dep
// (fmt.Println — no "/" in the key) so we can test stdlib filtering.
func stdlibDotGraph() *types.CallGraph {
	return &types.CallGraph{
		Nodes: map[string]types.Function{
			"github.com/org/repo/services/svc.main": {
				FullName: "github.com/org/repo/services/svc.main",
				IsMain:   true,
				Deps: []types.Dependency{
					{FullName: "github.com/org/repo/core/lib.Process"},
					{FullName: "fmt.Println"}, // stdlib — no "/"
				},
			},
			"github.com/org/repo/core/lib.Process": {
				FullName: "github.com/org/repo/core/lib.Process",
				Deps: []types.Dependency{
					{FullName: "fmt.Sprintf"}, // stdlib
				},
			},
		},
		FunctionOwner: map[string]string{
			"github.com/org/repo/services/svc.main": "services/svc",
			"github.com/org/repo/core/lib.Process":  "core/lib",
		},
	}
}

// TestFormatFullDot_StdlibFilteredByDefault asserts that stdlib function edges
// (keys with no "/") are excluded when showStdlib is false.
func TestFormatFullDot_StdlibFilteredByDefault(t *testing.T) {
	out := formatFullDot(stdlibDotGraph(), false)

	assert.NotContains(t, out, "fmt.Println",
		"stdlib fmt.Println must be absent when showStdlib=false")
	assert.NotContains(t, out, "fmt.Sprintf",
		"stdlib fmt.Sprintf must be absent when showStdlib=false")

	// Internal edges must still be present.
	assert.Contains(t, out,
		`"github.com/org/repo/services/svc.main" -> "github.com/org/repo/core/lib.Process"`,
		"internal edge must still be emitted when stdlib is filtered")
}

// TestFormatFullDot_StdlibShownWithFlag asserts that stdlib function edges are
// included when showStdlib is true.
func TestFormatFullDot_StdlibShownWithFlag(t *testing.T) {
	out := formatFullDot(stdlibDotGraph(), true)

	assert.Contains(t, out, "fmt.Println",
		"stdlib fmt.Println must appear when showStdlib=true")
	assert.Contains(t, out, "fmt.Sprintf",
		"stdlib fmt.Sprintf must appear when showStdlib=true")
}

func TestCountFiles(t *testing.T) {
	fns := map[string]*types.Function{
		"pkg.Foo": {File: "core/foo.go"},
		"pkg.Bar": {File: "core/foo.go"}, // same file
		"pkg.Baz": {File: "core/bar.go"},
	}
	assert.Equal(t, 2, countFiles(fns))
}
