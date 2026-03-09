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

func TestCountFiles(t *testing.T) {
	fns := map[string]*types.Function{
		"pkg.Foo": {File: "core/foo.go"},
		"pkg.Bar": {File: "core/foo.go"}, // same file
		"pkg.Baz": {File: "core/bar.go"},
	}
	assert.Equal(t, 2, countFiles(fns))
}
