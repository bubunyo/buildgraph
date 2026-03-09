package impact

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bubunyo/buildgraph/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// buildGraph builds a CallGraph from a compact description:
//
//	nodes:   map of fullName -> isMain
//	owners:  map of fullName -> owner (e.g. "services/service-a")
//	reverse: map of callee  -> []caller
func buildGraph(
	nodes map[string]bool,
	owners map[string]string,
	reverse map[string][]string,
) *types.CallGraph {
	graphNodes := make(map[string]types.Function, len(nodes))
	for name, isMain := range nodes {
		graphNodes[name] = types.Function{FullName: name, IsMain: isMain}
	}
	return &types.CallGraph{
		Nodes:         graphNodes,
		FunctionOwner: owners,
		ReverseIndex:  reverse,
	}
}

func change(fn string) types.Change {
	return types.Change{Function: fn, Type: "modified"}
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestComputeImpact_NoChanges(t *testing.T) {
	g := buildGraph(
		map[string]bool{"svc.main": true},
		map[string]string{"svc.main": "services/svc"},
		map[string][]string{},
	)
	result := NewAnalyzer(g).ComputeImpact(nil)

	assert.Empty(t, result.ServicesToBuild)
	assert.Empty(t, result.AffectedFunctions)
}

func TestComputeImpact_DirectServiceChange(t *testing.T) {
	g := buildGraph(
		map[string]bool{"services/svc.main": true},
		map[string]string{"services/svc.main": "services/svc"},
		map[string][]string{},
	)
	result := NewAnalyzer(g).ComputeImpact([]types.Change{change("services/svc.main")})

	assert.Contains(t, result.ServicesToBuild, "svc")
}

func TestComputeImpact_CoreChangePropagatesToService(t *testing.T) {
	g := buildGraph(
		map[string]bool{
			"core.Save":           false,
			"services/svc-a.main": true,
		},
		map[string]string{
			"core.Save":           "core/module",
			"services/svc-a.main": "services/svc-a",
		},
		map[string][]string{
			"core.Save": {"services/svc-a.main"},
		},
	)
	result := NewAnalyzer(g).ComputeImpact([]types.Change{change("core.Save")})

	assert.Contains(t, result.ServicesToBuild, "svc-a")
}

func TestComputeImpact_UnrelatedServiceNotAffected(t *testing.T) {
	g := buildGraph(
		map[string]bool{
			"core.Save":           false,
			"services/svc-a.main": true,
			"services/svc-b.main": true,
		},
		map[string]string{
			"core.Save":           "core/module",
			"services/svc-a.main": "services/svc-a",
			"services/svc-b.main": "services/svc-b",
		},
		map[string][]string{
			"core.Save": {"services/svc-a.main"},
		},
	)
	result := NewAnalyzer(g).ComputeImpact([]types.Change{change("core.Save")})

	assert.NotContains(t, result.ServicesToBuild, "svc-b")
	assert.Contains(t, result.ServicesToBuild, "svc-a")
}

func TestComputeImpact_MultiHopPropagation(t *testing.T) {
	g := buildGraph(
		map[string]bool{
			"core.Low":          false,
			"core.Mid":          false,
			"services/svc.main": true,
		},
		map[string]string{
			"core.Low":          "core/low",
			"core.Mid":          "core/mid",
			"services/svc.main": "services/svc",
		},
		map[string][]string{
			"core.Low": {"core.Mid"},
			"core.Mid": {"services/svc.main"},
		},
	)
	result := NewAnalyzer(g).ComputeImpact([]types.Change{change("core.Low")})

	assert.Contains(t, result.ServicesToBuild, "svc")
}

func TestComputeImpact_ServicesToBuildSorted(t *testing.T) {
	g := buildGraph(
		map[string]bool{
			"core.Fn":             false,
			"services/svc-b.main": true,
			"services/svc-a.main": true,
		},
		map[string]string{
			"core.Fn":             "core/mod",
			"services/svc-b.main": "services/svc-b",
			"services/svc-a.main": "services/svc-a",
		},
		map[string][]string{
			"core.Fn": {"services/svc-b.main", "services/svc-a.main"},
		},
	)
	result := NewAnalyzer(g).ComputeImpact([]types.Change{change("core.Fn")})

	assert.Equal(t, []string{"svc-a", "svc-b"}, result.ServicesToBuild)
}

func TestComputeImpact_FallbackBuildsAllServicesWhenCoreChanges(t *testing.T) {
	g := buildGraph(
		map[string]bool{
			"core.Orphan":         false,
			"services/svc-a.main": true,
		},
		map[string]string{
			"core.Orphan":         "core/mod",
			"services/svc-a.main": "services/svc-a",
		},
		map[string][]string{}, // no callers of core.Orphan
	)
	result := NewAnalyzer(g).ComputeImpact([]types.Change{change("core.Orphan")})

	// The fallback kicks in: since no service was reached, all known services
	// should be included.
	assert.Contains(t, result.ServicesToBuild, "svc-a")
}

// TestComputeImpact_ChangedFunctionWithNoOwner covers the branch where a
// changed function has no entry in FunctionOwner (owner == "").
func TestComputeImpact_ChangedFunctionWithNoOwner(t *testing.T) {
	g := buildGraph(
		map[string]bool{
			"orphan.Fn":           false,
			"services/svc-a.main": true,
		},
		map[string]string{
			// "orphan.Fn" deliberately has no owner entry.
			"services/svc-a.main": "services/svc-a",
		},
		map[string][]string{},
	)
	// Should not panic.
	assert.NotPanics(t, func() {
		NewAnalyzer(g).ComputeImpact([]types.Change{change("orphan.Fn")})
	})
}

func TestIsService_IdentifiesByMainFunction(t *testing.T) {
	g := buildGraph(
		map[string]bool{
			"services/svc.main": true,
			"core.Helper":       false,
		},
		map[string]string{
			"services/svc.main": "services/svc",
			"core.Helper":       "core/mod",
		},
		map[string][]string{},
	)
	a := NewAnalyzer(g)

	assert.True(t, a.isService("services/svc"))
	assert.False(t, a.isService("core/mod"))
}

// ── serviceSet precomputation ─────────────────────────────────────────────────

// TestNewAnalyzer_ServiceSetPrecomputed verifies that NewAnalyzer builds
// the serviceSet from graph nodes so that isService is an O(1) map lookup.
func TestNewAnalyzer_ServiceSetPrecomputed(t *testing.T) {
	g := buildGraph(
		map[string]bool{
			"services/svc-a.main": true,
			"services/svc-b.main": true,
			"core/mod.Helper":     false,
		},
		map[string]string{
			"services/svc-a.main": "services/svc-a",
			"services/svc-b.main": "services/svc-b",
			"core/mod.Helper":     "core/mod",
		},
		map[string][]string{},
	)
	a := NewAnalyzer(g)

	// Both services must be pre-indexed.
	assert.True(t, a.serviceSet["services/svc-a"], "svc-a must be in serviceSet")
	assert.True(t, a.serviceSet["services/svc-b"], "svc-b must be in serviceSet")
	// Non-service owners must not appear.
	assert.False(t, a.serviceSet["core/mod"], "core/mod must not be in serviceSet")
}

// TestNewAnalyzer_ServiceSet_EmptyGraphProducesEmptySet verifies that an
// empty graph does not panic and produces an empty serviceSet.
func TestNewAnalyzer_ServiceSet_EmptyGraphProducesEmptySet(t *testing.T) {
	g := buildGraph(map[string]bool{}, map[string]string{}, map[string][]string{})
	a := NewAnalyzer(g)

	assert.Empty(t, a.serviceSet)
	assert.False(t, a.isService("services/anything"))
}
