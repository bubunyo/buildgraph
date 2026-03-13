package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/bubunyo/buildgraph/pkg/types"
)

// writeOutput serialises result in the requested format and writes it to
// outputPath (or stdout if outputPath is empty).
func writeOutput(result *types.Result, graph *types.CallGraph, format, outputPath string) {
	var output []byte
	switch format {
	case "text":
		output = []byte(formatText(result))
	case "dot":
		output = []byte(formatDot(result, graph))
	default:
		var marshalErr error
		output, marshalErr = json.MarshalIndent(result, "", "  ")
		if marshalErr != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal result to JSON: %v\n", marshalErr)
			os.Exit(1)
		}
	}

	if outputPath != "" {
		if err := os.WriteFile(outputPath, output, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write output: %v\n", err)
			os.Exit(1)
		}
		return
	}
	fmt.Println(string(output))
}

// formatText renders result as a human-readable summary.
func formatText(result *types.Result) string {
	sb := &strings.Builder{}
	fmt.Fprintln(sb, "=== BuildGraph Analysis ===")
	fmt.Fprintf(sb, "Commit      : %s\n", result.CurrentCommit)
	fmt.Fprintf(sb, "Has changes : %v\n\n", result.HasChanges)

	if len(result.Changes) > 0 {
		fmt.Fprintf(sb, "Changes (%d):\n", len(result.Changes))
		for _, c := range result.Changes {
			fmt.Fprintf(sb, "  [%s] %s\n", c.Type, c.Function)
			if c.Reason != "" {
				fmt.Fprintf(sb, "    reason : %s\n", c.Reason)
			}
			if c.Type == "external_dep_changed" {
				fmt.Fprintf(sb, "    pkg    : %s  %s -> %s\n", c.Package, c.OldVer, c.NewVer)
			}
		}
		fmt.Fprintln(sb)
	}

	svcs := make([]string, len(result.Impact.ServicesToBuild))
	copy(svcs, result.Impact.ServicesToBuild)
	sort.Strings(svcs)

	fmt.Fprintf(sb, "Services to build (%d):\n", len(svcs))
	for _, svc := range svcs {
		fmt.Fprintf(sb, "  - %s\n", svc)
	}
	return sb.String()
}

// formatDot renders the impact as a Graphviz DOT digraph.
//
// Layout:
//   - One cluster (subgraph) per service that needs to be rebuilt.
//   - Nodes are short function names; changed functions are filled red,
//     transitively-affected functions are filled orange.
//   - Edges represent caller → callee relationships drawn from the call graph,
//     restricted to nodes that appear in the impact set.
func formatDot(result *types.Result, graph *types.CallGraph) string {
	// Index changed functions for quick lookup.
	changed := make(map[string]bool, len(result.Changes))
	for _, c := range result.Changes {
		changed[c.Function] = true
	}

	// Collect all affected functions across all owners.
	affected := make(map[string]bool)
	for _, fns := range result.Impact.AffectedFunctions {
		for _, fn := range fns {
			affected[fn] = true
		}
	}
	// Changed functions are implicitly affected too.
	for fn := range changed {
		affected[fn] = true
	}

	// Build a set of services to rebuild for quick lookup.
	rebuiltServices := make(map[string]bool, len(result.Impact.ServicesToBuild))
	for _, s := range result.Impact.ServicesToBuild {
		rebuiltServices[s] = true
	}

	// dotID converts a fully-qualified function name to a safe DOT node ID.
	dotID := func(fn string) string {
		r := strings.NewReplacer(".", "_", "/", "_", "-", "_", "(", "_", ")", "_")
		return r.Replace(fn)
	}

	// shortLabel strips the module prefix for readability.
	shortLabel := func(fn string) string {
		// Keep only "package.Func" — last two dot-separated segments.
		parts := strings.Split(fn, "/")
		if len(parts) == 0 {
			return fn
		}
		last := parts[len(parts)-1]
		return last
	}

	sb := &strings.Builder{}
	fmt.Fprintln(sb, "digraph buildgraph {")
	fmt.Fprintln(sb, `  rankdir=LR;`)
	fmt.Fprintln(sb, `  node [fontname="Helvetica", fontsize=11, style=filled, fillcolor=white];`)
	fmt.Fprintln(sb, `  edge [fontsize=9];`)
	fmt.Fprintln(sb)

	// Emit one cluster per owner that has affected functions.
	// Sort owners for deterministic output.
	owners := make([]string, 0, len(result.Impact.AffectedFunctions))
	for owner := range result.Impact.AffectedFunctions {
		owners = append(owners, owner)
	}
	sort.Strings(owners)

	clusterIdx := 0
	for _, owner := range owners {
		fns := result.Impact.AffectedFunctions[owner]
		if len(fns) == 0 {
			continue
		}

		// Determine cluster label — append "(rebuild)" for services being rebuilt.
		label := owner
		if rebuiltServices[owner] {
			label = owner + "  [rebuild]"
		}

		fmt.Fprintf(sb, "  subgraph cluster_%d {\n", clusterIdx)
		fmt.Fprintf(sb, "    label=%q;\n", label)
		fmt.Fprintln(sb, `    style=rounded;`)
		if rebuiltServices[owner] {
			fmt.Fprintln(sb, `    color=red;`)
		} else {
			fmt.Fprintln(sb, `    color=orange;`)
		}
		fmt.Fprintln(sb)

		seen := make(map[string]bool)
		for _, fn := range fns {
			if seen[fn] {
				continue
			}
			seen[fn] = true
			id := dotID(fn)
			lbl := shortLabel(fn)
			if changed[fn] {
				fmt.Fprintf(sb, "    %s [label=%q, fillcolor=\"#ff6b6b\", fontcolor=white];\n", id, lbl)
			} else {
				fmt.Fprintf(sb, "    %s [label=%q, fillcolor=\"#ffd580\"];\n", id, lbl)
			}
		}
		fmt.Fprintln(sb, "  }")
		fmt.Fprintln(sb)
		clusterIdx++
	}

	// Emit edges: for every affected function, draw edges to its callees
	// that are also in the affected set, using the call graph.
	fmt.Fprintln(sb, "  // edges")
	edgesSeen := make(map[string]bool)
	for fn := range affected {
		node, ok := graph.Nodes[fn]
		if !ok {
			continue
		}
		for _, dep := range node.Deps {
			if !affected[dep.FullName] {
				continue
			}
			key := dotID(fn) + "->" + dotID(dep.FullName)
			if edgesSeen[key] {
				continue
			}
			edgesSeen[key] = true
			fmt.Fprintf(sb, "  %s -> %s;\n", dotID(fn), dotID(dep.FullName))
		}
	}

	fmt.Fprintln(sb, "}")
	return sb.String()
}
