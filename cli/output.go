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
func writeOutput(result *types.Result, format, outputPath string) {
	var output []byte
	switch format {
	case "text":
		output = []byte(formatText(result))
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
