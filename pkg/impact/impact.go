package impact

import (
	"slices"
	"sort"
	"strings"

	"github.com/bubunyo/buildgraph/pkg/types"
)

type Analyzer struct {
	graph          *types.CallGraph
	functionOwners map[string]string
	reverseIndex   map[string][]string
	// serviceSet is a pre-computed set of owners (e.g. "services/service-a")
	// that contain a main function, built once in NewAnalyzer for O(1) lookup.
	serviceSet map[string]bool
}

func NewAnalyzer(graph *types.CallGraph) *Analyzer {
	// Pre-compute the service set by scanning graph nodes once.
	serviceSet := make(map[string]bool)
	for key, fn := range graph.Nodes {
		if fn.IsMain {
			if owner, ok := graph.FunctionOwner[key]; ok && owner != "" {
				serviceSet[owner] = true
			}
		}
	}

	return &Analyzer{
		graph:          graph,
		functionOwners: graph.FunctionOwner,
		reverseIndex:   graph.ReverseIndex,
		serviceSet:     serviceSet,
	}
}

func (a *Analyzer) ComputeImpact(changes []types.Change) types.Impact {
	impact := types.Impact{
		AffectedFunctions: make(map[string][]string),
		AffectReasons:     make(map[string][]string),
		ServicesToBuild:   []string{},
	}

	// Get initial changed functions
	changedFuncs := make(map[string]bool)
	for _, change := range changes {
		changedFuncs[change.Function] = true
	}

	// If no changes, return empty impact
	if len(changedFuncs) == 0 {
		return impact
	}

	// Propagate changes through call graph (find all callers)
	visited := make(map[string]bool)
	queue := make([]string, 0)

	// Initialize queue with changed functions
	for f := range changedFuncs {
		queue = append(queue, f)
	}

	// Iterative BFS traversal
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current] {
			continue
		}
		visited[current] = true

		// Find all functions that call this function
		callers := a.reverseIndex[current]
		for _, caller := range callers {
			if !visited[caller] {
				queue = append(queue, caller)

				// Record impact
				owner := a.functionOwners[caller]
				if owner != "" {
					impact.AffectedFunctions[owner] = append(impact.AffectedFunctions[owner], caller)
					impact.AffectReasons[owner] = append(impact.AffectReasons[owner], "calls "+current)
				}
			}
		}
	}

	// Also include the directly changed functions
	for funcName := range changedFuncs {
		owner := a.functionOwners[funcName]
		if owner != "" {
			// Check if already in affected
			if !slices.Contains(impact.AffectedFunctions[owner], funcName) {
				impact.AffectedFunctions[owner] = append(impact.AffectedFunctions[owner], funcName)
				impact.AffectReasons[owner] = append(impact.AffectReasons[owner], "directly_changed")
			}
		}
	}

	// Extract unique services to build
	serviceSet := make(map[string]bool)
	for owner := range impact.AffectedFunctions {
		// Check if this owner is a service (has main function)
		if a.isService(owner) {
			serviceSet[owner] = true
		}
	}

	for svc := range serviceSet {
		// Emit the short service name (last path component) rather than the
		// full relative path (e.g. "service-a" instead of "services/service-a").
		parts := strings.SplitN(svc, "/", 2)
		name := svc
		if len(parts) == 2 {
			name = parts[1]
		}
		impact.ServicesToBuild = append(impact.ServicesToBuild, name)
	}
	sort.Strings(impact.ServicesToBuild)

	// If no services found in impact, fall back to listing every known service
	// so that changes to orphaned functions (no callers in the graph) still
	// trigger a conservative full rebuild.
	if len(impact.ServicesToBuild) == 0 {
		seen := make(map[string]bool)
		for _, owner := range a.functionOwners {
			if seen[owner] || !a.isService(owner) {
				continue
			}
			seen[owner] = true
			parts := strings.SplitN(owner, "/", 2)
			name := owner
			if len(parts) == 2 {
				name = parts[1]
			}
			impact.ServicesToBuild = append(impact.ServicesToBuild, name)
		}
		sort.Strings(impact.ServicesToBuild)
	}

	return impact
}

// isService reports whether the given owner has a main function, using the
// pre-computed serviceSet for O(1) lookup.
func (a *Analyzer) isService(owner string) bool {
	return a.serviceSet[owner]
}
