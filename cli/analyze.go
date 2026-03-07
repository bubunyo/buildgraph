package cli

import (
	"fmt"
	"log"
	"time"

	"github.com/bubunyo/buildgraph/pkg/diff"
	"github.com/bubunyo/buildgraph/pkg/impact"
	"github.com/bubunyo/buildgraph/pkg/storage"
	"github.com/bubunyo/buildgraph/pkg/types"
	"github.com/spf13/cobra"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Detect which services need to be rebuilt",
	Long: `Loads the previous baseline, compares the current call graph against it,
and outputs which services are affected by the detected changes.`,
	RunE: runAnalyze,
}

func init() {
	analyzeCmd.Flags().StringP("format", "f", "json", "Output format: json, text")
	analyzeCmd.Flags().StringP("output", "o", "", "Output file (default: stdout)")
	analyzeCmd.Flags().BoolP("verbose", "v", false, "Include debug info in output")
	analyzeCmd.Flags().Bool("no-cache", false, "Ignore baseline, treat everything as new")
}

func runAnalyze(cmd *cobra.Command, _ []string) error {
	startTime := time.Now()
	cfg := loadConfig()

	rootPath, err := getWorkDir()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	rootModule, err := detectRootModule(rootPath)
	if err != nil {
		return fmt.Errorf("detecting root module: %w", err)
	}

	log.Printf("Analyzing project: %s", rootModule)

	// Load baseline before parsing so unchanged functions can reuse stored hashes.
	var previousBaseline *types.Baseline
	noCache, _ := cmd.Flags().GetBool("no-cache")
	if !noCache {
		store := storage.New()
		previousBaseline, _ = store.LoadBaseline(cfg.Baseline)
	}

	functions, graph, extDeps, extHash, _, err := parseProject(rootPath, rootModule, cfg, previousBaseline)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	detector := diff.NewDetector(functions, extDeps, extHash, previousBaseline)
	changes := detector.DetectChanges()

	impactAnalyzer := impact.NewAnalyzer(graph)
	impactResult := impactAnalyzer.ComputeImpact(changes)

	previousCommit := ""
	if previousBaseline != nil {
		previousCommit = previousBaseline.Commit
	}

	result := &types.Result{
		Timestamp:        time.Now(),
		PreviousCommit:   previousCommit,
		CurrentCommit:    getGitCommit(),
		PreviousBaseline: cfg.Baseline,
		HasChanges:       len(changes) > 0,
		Changes:          changes,
		Impact:           impactResult,
	}

	verbose, _ := cmd.Flags().GetBool("verbose")
	if verbose {
		result.Debug = &types.DebugInfo{
			FilesParsed:    countFiles(functions),
			FunctionsFound: len(functions),
			AnalysisTimeMs: time.Since(startTime).Milliseconds(),
			CacheHit:       previousBaseline != nil && !noCache,
		}
	}

	format, _ := cmd.Flags().GetString("format")
	output, _ := cmd.Flags().GetString("output")
	writeOutput(result, format, output)

	if len(changes) > 0 {
		log.Printf("Changes detected: %d", len(changes))
		log.Printf("Services to build: %v", impactResult.ServicesToBuild)
	} else {
		log.Printf("No changes detected")
	}
	return nil
}
