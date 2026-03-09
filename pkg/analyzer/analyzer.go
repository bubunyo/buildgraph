// Package analyzer builds a precise call graph for a Go monorepo using
// golang.org/x/tools. It uses:
//
//   - go/packages   — load packages with full type information
//   - go/ssa        — build SSA IR from the loaded packages
//   - callgraph/cha — Class Hierarchy Analysis to construct the call graph
//
// CHA is chosen because it is conservative (never misses an edge), fast, and
// does not require a single main package — which is essential for a monorepo
// that contains multiple independent services.
package analyzer

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/bubunyo/buildgraph/pkg/config"
	"github.com/bubunyo/buildgraph/pkg/types"
)

// Analyzer loads a monorepo, builds a CHA call graph, and returns the
// structured data needed by the diff and impact packages.
type Analyzer struct {
	cfg        *config.Config
	rootModule string
	rootPath   string

	// populated after Load()
	fset    *token.FileSet
	prog    *ssa.Program
	pkgs    []*ssa.Package
	cg      *callgraph.Graph
	allPkgs []*packages.Package

	// ssaIndex is a key→*ssa.Function map built once in BuildGraph() so that
	// findSSAFunc can resolve a function in O(1) instead of scanning all CHA
	// nodes on every call.
	ssaIndex map[string]*ssa.Function

	// sourceHashCache memoizes SHA-256 digests of source files within a
	// single analysis run to avoid redundant disk reads.
	sourceHashCache map[string]string
}

func New(cfg *config.Config, rootModule, rootPath string) *Analyzer {
	return &Analyzer{
		cfg:             cfg,
		rootModule:      rootModule,
		rootPath:        rootPath,
		ssaIndex:        make(map[string]*ssa.Function),
		sourceHashCache: make(map[string]string),
	}
}

// Load discovers all packages under services/ and core/, loads them with full
// type information, builds SSA, and runs CHA to produce a call graph.
func (a *Analyzer) Load() error {
	patterns := a.buildPatterns()

	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedTypes |
			packages.NeedSyntax |
			packages.NeedTypesInfo |
			packages.NeedModule,
		Fset: token.NewFileSet(),
		Dir:  a.rootPath,
		// Respect build tags, vendor, etc.
		Tests: false,
	}

	loaded, err := packages.Load(cfg, patterns...)
	if err != nil {
		return fmt.Errorf("packages.Load: %w", err)
	}

	// Collect and report any load errors but don't abort; partial graphs are
	// still useful.
	for _, p := range loaded {
		for _, e := range p.Errors {
			fmt.Printf("warning: package %s: %v\n", p.PkgPath, e)
		}
	}

	a.fset = cfg.Fset
	a.allPkgs = loaded

	// Build SSA from all transitively loaded packages.
	prog, ssaPkgs := ssautil.AllPackages(loaded, ssa.InstantiateGenerics)
	prog.Build()

	a.prog = prog

	// Keep only the directly requested packages (services + core).
	for _, p := range ssaPkgs {
		if p != nil {
			a.pkgs = append(a.pkgs, p)
		}
	}

	// Run CHA to build the call graph.
	a.cg = cha.CallGraph(prog)

	return nil
}

// BuildGraph converts the CHA call graph into the types.CallGraph used by the
// rest of buildgraph.
//
// Rules:
//   - Every function whose package path starts with rootModule is "internal".
//   - Everything else is "external" (stdlib or third-party); we record the
//     call but stop traversal there.
func (a *Analyzer) BuildGraph() (map[string]*types.Function, *types.CallGraph, error) {
	if a.cg == nil {
		return nil, nil, fmt.Errorf("call graph not built — call Load() first")
	}

	functions := make(map[string]*types.Function)
	nodes := make(map[string]types.Function)
	reverseIndex := make(map[string][]string)
	functionOwner := make(map[string]string)

	// Walk every node in the call graph.
	if err := callgraph.GraphVisitEdges(a.cg, func(edge *callgraph.Edge) error {
		caller := edge.Caller.Func
		callee := edge.Callee.Func

		// Skip synthetic / nil functions injected by the SSA builder.
		if isSynthetic(caller) || isSynthetic(callee) {
			return nil
		}

		callerKey := funcKey(caller)
		calleeKey := funcKey(callee)

		// Register caller if it belongs to the monorepo.
		if a.isInternal(caller) {
			if _, exists := functions[callerKey]; !exists {
				fn := a.toFunction(caller)
				functions[callerKey] = fn
				nodes[callerKey] = *fn
				functionOwner[callerKey] = a.owner(caller)
			}

			// Add callee as a dependency of the caller.
			dep := a.toDependency(callee)
			fn := functions[callerKey]
			if !hasDep(fn.Deps, calleeKey) {
				fn.Deps = append(fn.Deps, dep)
				functions[callerKey] = fn
				updated := nodes[callerKey]
				updated.Deps = fn.Deps
				nodes[callerKey] = updated
			}

			// Build reverse index only for internal callees so we can
			// propagate changes upward later.
			if a.isInternal(callee) {
				if !hasString(reverseIndex[calleeKey], callerKey) {
					reverseIndex[calleeKey] = append(reverseIndex[calleeKey], callerKey)
				}
			}
		}

		// Also register internal callees so isolated functions appear in the
		// graph even if nothing outside the monorepo calls them.
		if a.isInternal(callee) {
			if _, exists := functions[calleeKey]; !exists {
				fn := a.toFunction(callee)
				functions[calleeKey] = fn
				nodes[calleeKey] = *fn
				functionOwner[calleeKey] = a.owner(callee)
			}
		}

		return nil
	}); err != nil {
		return nil, nil, err
	}

	// Also capture functions that have no edges at all (leaf functions with
	// no callers and no callees) by walking every SSA function directly.
	for fn := range a.cg.Nodes {
		if fn == nil || isSynthetic(fn) || !a.isInternal(fn) {
			continue
		}
		key := funcKey(fn)
		if _, exists := functions[key]; !exists {
			f := a.toFunction(fn)
			functions[key] = f
			nodes[key] = *f
			functionOwner[key] = a.owner(fn)
		}
	}

	graph := &types.CallGraph{
		Nodes:         nodes,
		ReverseIndex:  reverseIndex,
		FunctionOwner: functionOwner,
	}

	// Build the O(1) SSA index so that ComputeHashes can look up functions
	// by key without scanning all CHA nodes on every call.
	for fn := range a.cg.Nodes {
		if fn != nil {
			a.ssaIndex[funcKey(fn)] = fn
		}
	}

	// Apply exclude patterns: remove functions whose source file matches any
	// configured glob pattern or whose file path contains /vendor/ when
	// SkipVendor is enabled, keeping the graph consistent.
	a.applyExcludeFilters(functions, graph)

	return functions, graph, nil
}

// ComputeHashes fills in ASTHash and TransitiveHash for every function.
//
// If prevSourceHashes and prevFuncHashes are non-nil (from a stored baseline),
// functions whose source file hash is unchanged have their AST hash reused from
// the baseline instead of being recomputed — a significant speedup for large
// codebases where only a few files change per commit.
func (a *Analyzer) ComputeHashes(
	functions map[string]*types.Function,
	prevSourceHashes map[string]string,
	prevFuncHashes map[string]types.HashInfo,
) error {
	// First pass: AST hash.
	// If the file containing a function has not changed (source hash match),
	// reuse the previously computed AST hash to avoid re-parsing.
	for key, fn := range functions {
		if fn.File != "" && prevSourceHashes != nil && prevFuncHashes != nil {
			if prevSrc, ok := prevSourceHashes[fn.File]; ok {
				if curSrc, ok2 := a.cachedSourceHash(fn.File); ok2 && curSrc == prevSrc {
					if stored, ok3 := prevFuncHashes[key]; ok3 && stored.ASTHash != "" {
						fn.ASTHash = stored.ASTHash
						functions[key] = fn
						continue
					}
				}
			}
		}

		h, err := a.astHash(fn)
		if err != nil {
			return err
		}
		fn.ASTHash = h
		functions[key] = fn
	}

	// Second pass: transitive hash (incorporates direct dep hashes).
	for key, fn := range functions {
		fn.TransitiveHash = transitiveHash(fn, functions)
		functions[key] = fn
	}

	return nil
}

// cachedSourceHash returns the SHA-256 of the given file path, memoised for
// the lifetime of this Analyzer instance.
func (a *Analyzer) cachedSourceHash(relPath string) (string, bool) {
	if h, ok := a.sourceHashCache[relPath]; ok {
		return h, true
	}
	absPath := filepath.Join(a.rootPath, relPath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(data)
	h := fmt.Sprintf("sha256:%x", sum)
	a.sourceHashCache[relPath] = h
	return h, true
}

// ComputeSourceHashes returns a map of relative file path → SHA-256 hash for
// every Go source file that was loaded during this analysis run.  The map is
// stored in the baseline so that subsequent runs can detect which files have
// not changed and skip re-hashing their functions.
func (a *Analyzer) ComputeSourceHashes() (map[string]string, error) {
	hashes := make(map[string]string)
	for _, pkg := range a.allPkgs {
		for _, f := range pkg.CompiledGoFiles {
			rel := a.relPath(f)
			if _, done := hashes[rel]; done {
				continue
			}
			data, err := os.ReadFile(f)
			if err != nil {
				// Non-fatal: skip unreadable files.
				continue
			}
			sum := sha256.Sum256(data)
			hashes[rel] = fmt.Sprintf("sha256:%x", sum)
		}
	}
	return hashes, nil
}

// ExtractExternalDeps parses the root go.mod and returns the require map plus
// a hash of the require block.
func (a *Analyzer) ExtractExternalDeps() (map[string]string, string, error) {
	goModPath := filepath.Join(a.rootPath, "go.mod")
	gomod, err := ParseGoMod(goModPath)
	if err != nil {
		return nil, "", err
	}
	return gomod.Require, HashGoMod(gomod), nil
}

// ── private helpers ──────────────────────────────────────────────────────────

func (a *Analyzer) isInternal(fn *ssa.Function) bool {
	if fn.Package() == nil {
		return false
	}
	return strings.HasPrefix(fn.Package().Pkg.Path(), a.rootModule)
}

func (a *Analyzer) owner(fn *ssa.Function) string {
	if fn.Package() == nil {
		return ""
	}
	pkgPath := fn.Package().Pkg.Path()
	rel := strings.TrimPrefix(pkgPath, a.rootModule+"/")
	// Return the top two path components (e.g. "services/service-a" or
	// "core/module-a") so that individual services are distinguished from
	// one another. If the package lives directly under the root (no slash),
	// return the single component as-is.
	parts := strings.SplitN(rel, "/", 3)
	if len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	return parts[0]
}

func (a *Analyzer) toFunction(fn *ssa.Function) *types.Function {
	key := funcKey(fn)
	pkg := ""
	if fn.Package() != nil {
		pkg = fn.Package().Pkg.Path()
	}

	file, startLine, endLine := fnPosition(fn, a.fset)

	return &types.Function{
		Name:       fn.Name(),
		FullName:   key,
		Package:    pkg,
		File:       a.relPath(file),
		StartLine:  startLine,
		EndLine:    endLine,
		IsExported: fn.Object() != nil && fn.Object().Exported(),
		IsMain:     fn.Name() == "main" && fn.Package() != nil && fn.Package().Pkg.Name() == "main",
	}
}

func (a *Analyzer) toDependency(fn *ssa.Function) types.Dependency {
	key := funcKey(fn)
	depType := "external"
	pkgPath := ""
	pkgName := ""
	pkgVer := ""

	if fn.Package() != nil {
		pkgPath = fn.Package().Pkg.Path()
		pkgName = fn.Package().Pkg.Name()
		if a.isInternal(fn) {
			depType = "internal"
		}
	}

	return types.Dependency{
		Name:     fn.Name(),
		FullName: key,
		Type:     depType,
		Package: types.Package{
			Path:    pkgPath,
			Name:    pkgName,
			Version: pkgVer,
			Module:  pkgPath,
		},
	}
}

func (a *Analyzer) astHash(fn *types.Function) (string, error) {
	// Find the SSA function corresponding to this types.Function.
	ssaFn := a.findSSAFunc(fn.FullName)
	if ssaFn == nil || ssaFn.Syntax() == nil {
		return "", nil
	}

	var buf bytes.Buffer
	// Use the SSA textual representation as a canonical form — it strips
	// comments, normalises whitespace, and is stable across formatting changes.
	if _, err := ssaFn.WriteTo(&buf); err != nil {
		return "", fmt.Errorf("serialising SSA for %s: %w", ssaFn.Name(), err)
	}

	h := sha256.Sum256(buf.Bytes())
	return fmt.Sprintf("sha256:%x", h), nil
}

// findSSAFunc returns the *ssa.Function for the given key using the pre-built
// ssaIndex for O(1) lookup.  Returns nil if the key is not found.
func (a *Analyzer) findSSAFunc(fullName string) *ssa.Function {
	return a.ssaIndex[fullName]
}

func (a *Analyzer) relPath(abs string) string {
	rel, err := filepath.Rel(a.rootPath, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(rel)
}

// buildPatterns constructs ./... glob patterns for go/packages.
// The whole project is scannable by default; only the service directories are
// explicitly listed in config.  We scan the entire module ("./...") and rely
// on the call graph to exclude packages that are never reachable from a service.
func (a *Analyzer) buildPatterns() []string {
	// Use the configured service directories as entry-point patterns so that
	// go/packages loads exactly the packages we care about plus their transitive
	// dependencies (which it resolves automatically).
	var patterns []string
	for _, d := range a.cfg.Services {
		// Strip any trailing slash the user may have added.
		d = strings.TrimRight(d, "/")
		patterns = append(patterns, "./"+d+"/...")
	}
	if len(patterns) == 0 {
		// Fallback: scan everything.
		patterns = []string{"./..."}
	}
	return patterns
}

// applyExcludeFilters removes from functions and graph any function whose
// source file matches a configured exclude glob pattern or whose file path
// contains /vendor/ when SkipVendor is enabled.  Entries are also purged from
// the reverse index and function-owner map to keep the graph consistent.
func (a *Analyzer) applyExcludeFilters(functions map[string]*types.Function, graph *types.CallGraph) {
	if !a.cfg.Exclude.SkipVendor && len(a.cfg.Exclude.Patterns) == 0 {
		return
	}

	excluded := make(map[string]bool)
	for key, fn := range functions {
		if a.fileMatchesExclude(fn.File) {
			excluded[key] = true
		}
	}

	for key := range excluded {
		delete(functions, key)
		delete(graph.Nodes, key)
		delete(graph.FunctionOwner, key)
		delete(graph.ReverseIndex, key)
	}

	// Also scrub excluded keys out of callers' reverse-index slices.
	for callee, callers := range graph.ReverseIndex {
		filtered := callers[:0]
		for _, caller := range callers {
			if !excluded[caller] {
				filtered = append(filtered, caller)
			}
		}
		if len(filtered) == 0 {
			delete(graph.ReverseIndex, callee)
		} else {
			graph.ReverseIndex[callee] = filtered
		}
	}
}

// fileMatchesExclude reports whether the given relative file path should be
// excluded according to the configured SkipVendor flag and Patterns list.
// Pattern matching uses path.Match (forward-slash semantics) and also handles
// the common "**/" prefix by stripping it and matching against each path
// component suffix.
func (a *Analyzer) fileMatchesExclude(relFile string) bool {
	if relFile == "" {
		return false
	}
	// Normalise to forward slashes for cross-platform consistency.
	relFile = filepath.ToSlash(relFile)

	if a.cfg.Exclude.SkipVendor && strings.Contains(relFile, "/vendor/") {
		return true
	}

	for _, pattern := range a.cfg.Exclude.Patterns {
		pattern = filepath.ToSlash(pattern)

		// Support "**/" prefix: match the pattern against every suffix of the path.
		trimmed := strings.TrimPrefix(pattern, "**/")
		if trimmed != pattern {
			// The pattern had a "**/" prefix — check all trailing sub-paths.
			parts := strings.Split(relFile, "/")
			for i := range parts {
				candidate := strings.Join(parts[i:], "/")
				if ok, _ := path.Match(trimmed, candidate); ok {
					return true
				}
			}
			continue
		}

		// Plain pattern — match against the whole relative path.
		if ok, _ := path.Match(pattern, relFile); ok {
			return true
		}
	}
	return false
}

// funcKey returns a stable, globally-unique identifier for an SSA function.
// It uses fn.String() (≡ fn.RelString(nil)) which includes the receiver type
// for methods, preventing collisions between methods of the same name on
// different receiver types within the same package.
//
// Examples:
//
//	"github.com/user/repo/pkg.FuncName"       // package-level function
//	"(*github.com/user/repo/pkg.TypeA).Save"  // pointer-receiver method
//	"(github.com/user/repo/pkg.TypeB).Save"   // value-receiver method
func funcKey(fn *ssa.Function) string {
	return fn.String()
}

func fnPosition(fn *ssa.Function, fset *token.FileSet) (file string, start, end int) {
	if fn.Pos() == token.NoPos {
		return "", 0, 0
	}
	pos := fset.Position(fn.Pos())
	// ssa.Function has no EndPos — use Pos as both start and end.
	return pos.Filename, pos.Line, pos.Line
}

func isSynthetic(fn *ssa.Function) bool {
	// Synthetic functions have no source position.
	return fn == nil || (fn.Synthetic != "" && fn.Pos() == token.NoPos)
}

func hasDep(deps []types.Dependency, fullName string) bool {
	for _, d := range deps {
		if d.FullName == fullName {
			return true
		}
	}
	return false
}

func hasString(ss []string, s string) bool {
	return slices.Contains(ss, s)
}

func transitiveHash(fn *types.Function, all map[string]*types.Function) string {
	parts := []string{fn.ASTHash}
	for _, dep := range fn.Deps {
		switch dep.Type {
		case "internal":
			if callee, ok := all[dep.FullName]; ok {
				parts = append(parts, callee.ASTHash)
			}
		case "external":
			parts = append(parts, dep.Package.Path+"@"+dep.Package.Version)
		}
	}
	// Deterministic sort.
	slices.Sort(parts)
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return fmt.Sprintf("sha256:%x", h)
}
