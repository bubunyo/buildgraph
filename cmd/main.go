package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bubunyo/buildgraph/pkg/analyzer"
	"github.com/bubunyo/buildgraph/pkg/config"
	"github.com/bubunyo/buildgraph/pkg/types"
)

var (
	flagConfig   string
	flagOutput   string
	flagFormat   string
	flagVerbose  bool
	flagBaseline string
	flagNoCache  bool
)

func main() {
	flag.StringVar(&flagConfig, "c", ".buildgraph/config.yaml", "Config file path")
	flag.StringVar(&flagOutput, "o", "", "Output file (default: stdout)")
	flag.StringVar(&flagFormat, "f", "json", "Output format: json, yaml, text")
	flag.BoolVar(&flagVerbose, "v", false, "Verbose output")
	flag.StringVar(&flagBaseline, "b", ".buildgraph/cache/baseline.json", "Baseline file")
	flag.BoolVar(&flagNoCache, "no-cache", false, "Skip baseline, compute fresh")

	flag.Parse()

	if flag.NArg() < 1 {
		log.Fatal("Usage: buildgraph <command>")
	}

	cmd := flag.Arg(0)

	switch cmd {
	case "analyze":
		runAnalyze()
	case "generate":
		runGenerate()
	default:
		log.Fatalf("Unknown command: %s", cmd)
	}
}

func runAnalyze() {
	startTime := time.Now()

	cfg, err := config.Load(flagConfig)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	rootPath, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}

	rootModule, err := detectRootModule(rootPath)
	if err != nil {
		log.Fatalf("Failed to detect root module: %v", err)
	}

	log.Printf("Analyzing project: %s", rootModule)

	a := analyzer.New(cfg, rootModule)

	if err := a.DiscoverPackages(rootPath); err != nil {
		log.Fatalf("Failed to discover packages: %v", err)
	}

	functions, err := a.ParseSourceFiles(rootPath)
	if err != nil {
		log.Fatalf("Failed to parse source files: %v", err)
	}

	if err := a.ExtractDependencies(functions); err != nil {
		log.Fatalf("Failed to extract dependencies: %v", err)
	}

	graph := a.BuildIndices(functions)

	if err := a.ComputeHashes(functions); err != nil {
		log.Fatalf("Failed to compute hashes: %v", err)
	}

	result := &types.Result{
		Timestamp:     time.Now(),
		CurrentCommit: getGitCommit(),
		HasChanges:    true,
		Changes:       []types.Change{},
		Impact:        computeImpact(functions, graph),
	}

	if flagVerbose {
		result.Debug = &types.DebugInfo{
			FilesParsed:    countFiles(functions),
			FunctionsFound: len(functions),
			AnalysisTimeMs: time.Since(startTime).Milliseconds(),
		}
	}

	var output []byte
	switch flagFormat {
	case "json":
		output, _ = json.MarshalIndent(result, "", "  ")
	case "yaml":
		output, _ = json.Marshal(result)
	default:
		output = []byte(formatText(result))
	}

	if flagOutput != "" {
		os.WriteFile(flagOutput, output, 0644)
	} else {
		fmt.Println(string(output))
	}
}

func runGenerate() {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	rootPath, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}

	rootModule, err := detectRootModule(rootPath)
	if err != nil {
		log.Fatalf("Failed to detect root module: %v", err)
	}

	a := analyzer.New(cfg, rootModule)

	if err := a.DiscoverPackages(rootPath); err != nil {
		log.Fatalf("Failed to discover packages: %v", err)
	}

	functions, err := a.ParseSourceFiles(rootPath)
	if err != nil {
		log.Fatalf("Failed to parse source files: %v", err)
	}

	if err := a.ExtractDependencies(functions); err != nil {
		log.Fatalf("Failed to extract dependencies: %v", err)
	}

	graph := a.BuildIndices(functions)

	if err := a.ComputeHashes(functions); err != nil {
		log.Fatalf("Failed to compute hashes: %v", err)
	}

	baseline := &types.Baseline{
		Version:        "1.0",
		GeneratedAt:    time.Now(),
		Commit:         getGitCommit(),
		GoVersion:      getGoVersion(),
		ModulePath:     rootModule,
		Graph:          *graph,
		FunctionHashes: make(map[string]types.HashInfo),
		ExternalDeps:   make(map[string]string),
		SourceHashes:   make(map[string]string),
	}

	for name, fn := range functions {
		baseline.FunctionHashes[name] = types.HashInfo{
			ASTHash: fn.ASTHash,
		}
	}

	outputPath := flagOutput
	if outputPath == "" {
		outputPath = ".buildgraph/cache/baseline.json"
	}

	os.MkdirAll(".buildgraph/cache", 0755)

	data, _ := json.MarshalIndent(baseline, "", "  ")
	os.WriteFile(outputPath, data, 0644)

	fmt.Printf("Baseline generated: %s\n", outputPath)
}

func detectRootModule(rootPath string) (string, error) {
	content, err := os.ReadFile(rootPath + "/go.mod")
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if len(line) > 7 && strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}

	return "", fmt.Errorf("module not found")
}

func getGitCommit() string {
	return "unknown"
}

func getGoVersion() string {
	return "1.24"
}

func computeImpact(functions map[string]*types.Function, graph *types.CallGraph) types.Impact {
	impact := types.Impact{
		AffectedFunctions: make(map[string][]string),
		AffectReasons:     make(map[string][]string),
		ServicesToBuild:   []string{},
	}

	affectedOwners := make(map[string]bool)
	for _, fn := range functions {
		owner := graph.FunctionOwner[fn.FullName]
		if _, ok := impact.AffectedFunctions[owner]; !ok {
			impact.AffectedFunctions[owner] = []string{}
			impact.AffectReasons[owner] = []string{}
		}
		impact.AffectedFunctions[owner] = append(impact.AffectedFunctions[owner], fn.FullName)
		affectedOwners[owner] = true
	}

	for owner := range affectedOwners {
		impact.ServicesToBuild = append(impact.ServicesToBuild, owner)
	}

	return impact
}

func countFiles(functions map[string]*types.Function) int {
	files := make(map[string]bool)
	for _, fn := range functions {
		files[fn.File] = true
	}
	return len(files)
}

func formatText(result *types.Result) string {
	output := " formatText(result *=== BuildGraph Analysis ===\n\n"
	output += fmt.Sprintf("Changes detected: %d\n", len(result.Changes))
	output += fmt.Sprintf("Services to build: %d\n\n", len(result.Impact.ServicesToBuild))

	for _, svc := range result.Impact.ServicesToBuild {
		output += fmt.Sprintf("  - %s\n", svc)
	}

	return output
}
