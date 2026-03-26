package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Dump the full call graph as a DOT diagram",
	Long: `Analyses the current source tree, builds the complete call graph, and
emits it as a Graphviz DOT digraph.

Pipe the output into dot(1) to render an image:

  buildgraph graph | dot -Tpng -o graph.png
  buildgraph graph | dot -Tsvg -o graph.svg

The diagram groups functions into clusters by owner (service / library).
Main-package entry points are highlighted in light blue.

Standard library functions (fmt.Println, os.Exit, etc.) are excluded by
default to keep the graph focused on your own code. Use --stdlib to include
them.`,
	RunE: runGraph,
}

func init() {
	graphCmd.Flags().StringP("format", "f", "dot", "Output format (dot)")
	graphCmd.Flags().StringP("output", "o", "", "Output file (default: stdout)")
	graphCmd.Flags().Bool("stdlib", false, "Include standard library functions in the graph")
}

func runGraph(cmd *cobra.Command, _ []string) error {
	cfg := loadConfig()

	rootPath, err := getWorkDir()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	rootModule, err := detectRootModule(rootPath)
	if err != nil {
		return fmt.Errorf("detecting root module: %w", err)
	}

	// Parse the project without a previous baseline — we want the full current
	// graph, not a diff against anything.
	_, graph, _, _, _, err := parseProject(rootPath, rootModule, cfg, nil)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	format, _ := cmd.Flags().GetString("format")
	output, _ := cmd.Flags().GetString("output")
	showStdlib, _ := cmd.Flags().GetBool("stdlib")
	writeGraphOutput(graph, format, output, showStdlib)

	return nil
}
