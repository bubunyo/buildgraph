package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bubunyo/buildgraph/pkg/analyzer"
	"github.com/bubunyo/buildgraph/pkg/config"
	"github.com/bubunyo/buildgraph/pkg/types"
)

// parseProject runs the full analysis pipeline shared by analyze and generate.
// prevBaseline may be nil (first run or --no-cache); when provided, its source
// and function hashes are used to skip re-hashing unchanged functions.
func parseProject(
	rootPath, rootModule string,
	cfg *config.Config,
	prevBaseline *types.Baseline,
) (
	functions map[string]*types.Function,
	graph *types.CallGraph,
	extDeps map[string]string,
	extHash string,
	sourceHashes map[string]string,
	err error,
) {
	a := analyzer.New(cfg, rootModule, rootPath)

	fmt.Fprintln(os.Stderr, "Loading packages…")
	if err = a.Load(); err != nil {
		return
	}

	fmt.Fprintln(os.Stderr, "Building call graph…")
	functions, graph, err = a.BuildGraph()
	if err != nil {
		return
	}

	var prevSrcHashes map[string]string
	var prevFuncHashes map[string]types.HashInfo
	if prevBaseline != nil {
		prevSrcHashes = prevBaseline.SourceHashes
		prevFuncHashes = prevBaseline.FunctionHashes
	}

	fmt.Fprintf(os.Stderr, "Computing hashes for %d functions…\n", len(functions))
	if err = a.ComputeHashes(functions, prevSrcHashes, prevFuncHashes); err != nil {
		return
	}

	extDeps, extHash, err = a.ExtractExternalDeps()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not extract external deps: %v\n", err)
		extDeps = map[string]string{}
		extHash = ""
		err = nil //nolint:ineffassign
	}

	sourceHashes, err = a.ComputeSourceHashes()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not compute source hashes: %v\n", err)
		sourceHashes = map[string]string{}
		err = nil //nolint:ineffassign
	}

	return
}

// detectRootModule reads the module path from the go.mod in rootPath.
func detectRootModule(rootPath string) (string, error) {
	content, err := os.ReadFile(filepath.Join(rootPath, "go.mod"))
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(string(content), "\n") {
		after, found := strings.CutPrefix(line, "module ")
		if found {
			return strings.TrimSpace(after), nil
		}
	}
	return "", fmt.Errorf("module directive not found in go.mod")
}

// getWorkDir returns the current working directory.
func getWorkDir() (string, error) {
	return os.Getwd()
}

func getGitCommit() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func getGoVersion() string {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		return "unknown"
	}
	parts := strings.Fields(string(out))
	if len(parts) >= 3 {
		return parts[2]
	}
	return "unknown"
}

func countFiles(functions map[string]*types.Function) int {
	files := make(map[string]bool)
	for _, fn := range functions {
		files[fn.File] = true
	}
	return len(files)
}
