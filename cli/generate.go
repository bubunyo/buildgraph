package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/bubunyo/buildgraph/pkg/storage"
	"github.com/bubunyo/buildgraph/pkg/types"
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a new baseline snapshot",
	Long: `Parses the current codebase, builds the call graph, and saves a baseline
snapshot that future analyze runs will compare against.`,
	RunE: runGenerate,
}

func init() {
	generateCmd.Flags().StringP("output", "o", "", "Output path for the baseline (overrides config)")
}

func runGenerate(cmd *cobra.Command, _ []string) error {
	cfg := loadConfig()

	rootPath, err := getWorkDir()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	rootModule, err := detectRootModule(rootPath)
	if err != nil {
		return fmt.Errorf("detecting root module: %w", err)
	}

	functions, graph, extDeps, extHash, sourceHashes, err := parseProject(rootPath, rootModule, cfg, nil)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	functionHashes := make(map[string]types.HashInfo)
	for name, fn := range functions {
		var extDepsForFn []string
		for _, dep := range fn.Deps {
			if dep.Type == "external" {
				extDepsForFn = append(extDepsForFn, dep.Package.Path)
			}
		}
		functionHashes[name] = types.HashInfo{
			ASTHash:        fn.ASTHash,
			TransitiveHash: fn.TransitiveHash,
			ExternalDeps:   extDepsForFn,
		}
	}

	baseline := &types.Baseline{
		Version:          "1.0",
		GeneratedAt:      time.Now(),
		Commit:           getGitCommit(),
		GoVersion:        getGoVersion(),
		ModulePath:       rootModule,
		Graph:            *graph,
		FunctionHashes:   functionHashes,
		ExternalDeps:     extDeps,
		ExternalDepsHash: extHash,
		SourceHashes:     sourceHashes,
	}

	outputPath, _ := cmd.Flags().GetString("output")
	if outputPath == "" {
		outputPath = cfg.Baseline
	}

	store := storage.New()
	if err := store.SaveBaseline(baseline, outputPath); err != nil {
		return fmt.Errorf("saving baseline: %w", err)
	}

	fmt.Printf("Baseline generated : %s\n", outputPath)
	fmt.Printf("Commit             : %s\n", baseline.Commit)
	fmt.Printf("Functions          : %d\n", len(functions))
	fmt.Printf("External deps      : %d\n", len(extDeps))
	fmt.Printf("External deps hash : %s\n", extHash)
	return nil
}
