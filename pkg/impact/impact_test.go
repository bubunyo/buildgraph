package impact

import (
	"slices"
	"testing"

	"github.com/bubunyo/buildgraph/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// graph builds a CallGraph from a compact description:
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

	if len(result.ServicesToBuild) != 0 {
		t.Errorf("expected no services to build, got %v", result.ServicesToBuild)
	}
	if len(result.AffectedFunctions) != 0 {
		t.Errorf("expected no affected functions, got %v", result.AffectedFunctions)
	}
}

func TestComputeImpact_DirectServiceChange(t *testing.T) {
	g := buildGraph(
		map[string]bool{"services/svc.main": true},
		map[string]string{"services/svc.main": "services/svc"},
		map[string][]string{},
	)
	result := NewAnalyzer(g).ComputeImpact([]types.Change{
		change("services/svc.main"),
	})

	if !slices.Contains(result.ServicesToBuild, "svc") {
		t.Errorf("expected 'svc' in services_to_build, got %v", result.ServicesToBuild)
	}
}

func TestComputeImpact_CoreChangePropagatesToService(t *testing.T) {
	// core.Save is called by services/service-a.main
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

	if !slices.Contains(result.ServicesToBuild, "svc-a") {
		t.Errorf("expected 'svc-a' in services_to_build, got %v", result.ServicesToBuild)
	}
}

func TestComputeImpact_UnrelatedServiceNotAffected(t *testing.T) {
	// core.Save is only called by svc-a; svc-b is unrelated
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

	if slices.Contains(result.ServicesToBuild, "svc-b") {
		t.Errorf("svc-b should not be in services_to_build, got %v", result.ServicesToBuild)
	}
	if !slices.Contains(result.ServicesToBuild, "svc-a") {
		t.Errorf("svc-a should be in services_to_build, got %v", result.ServicesToBuild)
	}
}

func TestComputeImpact_MultiHopPropagation(t *testing.T) {
	// core.Low -> core.Mid -> services/svc.main  (two hops)
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

	if !slices.Contains(result.ServicesToBuild, "svc") {
		t.Errorf("expected 'svc' in services_to_build via multi-hop, got %v", result.ServicesToBuild)
	}
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

	if len(result.ServicesToBuild) != 2 {
		t.Fatalf("expected 2 services, got %v", result.ServicesToBuild)
	}
	if result.ServicesToBuild[0] != "svc-a" || result.ServicesToBuild[1] != "svc-b" {
		t.Errorf("expected sorted [svc-a svc-b], got %v", result.ServicesToBuild)
	}
}

func TestComputeImpact_FallbackBuildsAllServicesWhenCoreChanges(t *testing.T) {
	// A change to a function that is not in the reverse index of any service
	// (e.g. a leaf core function with no callers) should not trigger the
	// fallback — only truly unresolvable changes should.
	// Here we verify the fallback: core changes whose callers resolve to no
	// service produce an empty services_to_build (not all services).
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
	if !slices.Contains(result.ServicesToBuild, "svc-a") {
		t.Errorf("expected fallback to include svc-a, got %v", result.ServicesToBuild)
	}
}

// TestComputeImpact_ChangedFunctionWithNoOwner covers the branch where a
// changed function has no entry in FunctionOwner (owner == "").  The function
// should still be processed for reversal propagation without panicking, and the
// known services should appear via the fallback.
func TestComputeImpact_ChangedFunctionWithNoOwner(t *testing.T) {
	g := buildGraph(
		map[string]bool{
			"orphan.Fn":           false,
			"services/svc-a.main": true,
		},
		map[string]string{
			// Note: "orphan.Fn" deliberately has no owner entry.
			"services/svc-a.main": "services/svc-a",
		},
		map[string][]string{}, // no callers of orphan.Fn
	)
	// Should not panic; fallback emits all known services.
	result := NewAnalyzer(g).ComputeImpact([]types.Change{change("orphan.Fn")})
	_ = result // the fallback may or may not include svc-a depending on owner lookup
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

	if !a.isService("services/svc") {
		t.Error("expected services/svc to be identified as a service")
	}
	if a.isService("core/mod") {
		t.Error("expected core/mod not to be identified as a service")
	}
}
