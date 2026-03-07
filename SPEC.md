# BuildGraph - Intelligent Build Pipeline Optimization

## Table of Contents

1. [Project Overview](#project-overview)
2. [Problem Statement](#problem-statement)
3. [Architecture](#architecture)
4. [Data Structures](#data-structures)
5. [Algorithms](#algorithms)
6. [CLI Interface](#cli-interface)
7. [Configuration](#configuration)
8. [Storage Format](#storage-format)
9. [Pipeline Integration](#pipeline-integration)
10. [Edge Cases](#edge-cases)
11. [Performance Considerations](#performance-considerations)
12. [Bottlenecks and Mitigations](#bottlenecks-and-mitigations)
13. [Implementation Status](#implementation-status)
14. [Project Structure](#project-structure)
15. [Implementation Roadmap](#implementation-roadmap)

---

## Project Overview

### Purpose

BuildGraph is a static analysis tool that analyzes Go codebases containing multiple microservices and core modules. It builds a call graph of all internal code (services and core modules), detects semantic changes between builds, and determines which services need to be rebuilt based on their dependency relationships.

### Goals

1. **Reduce CI build time** by only building services affected by code changes
2. **Provide accurate impact analysis** using semantic (AST-based) diffing
3. **Handle complex dependency chains** including internal and external dependencies
4. **Integrate seamlessly** with existing CI/CD pipelines

### Terminology

| Term | Definition |
|------|------------|
| **Service** | A deployable microservice in the `services/` directory that has a `main()` function |
| **Core Module** | A shared library/module in the `core/` directory used by services |
| **Internal Dependency** | A dependency on code within the same repository (service or core module) |
| **External Dependency** | A dependency on code outside the repository (third-party Go modules) |
| **Call Graph** | A directed graph where nodes are functions and edges represent call relationships |
| **Impact Analysis** | The process of determining which services are affected by changed functions |
| **Baseline** | The previously stored state (function hashes, call graph) used for comparison |

---

## Problem Statement

### Current State

In a Go monorepo with multiple microservices:

```
repo/
├── services/
│   ├── service-a/
│   │   ├── main.go
│   │   └── go.mod
│   ├── service-b/
│   │   ├── main.go
│   │   └── go.mod
│   └── service-c/
│       ├── main.go
│       └── go.mod
├── core/
│   ├── module-a/
│   │   ├── foo.go
│   │   └── go.mod
│   ├── module-b/
│   │   ├── bar.go
│   │   └── go.mod
│   └── module-c/
│       ├── baz.go
│       └── go.mod
└── go.work (optional, for workspace)
```

**Dependency Example:**
- `service-a` imports `module-a`, `module-b`
- `service-b` imports `module-a`
- `service-c` imports `module-c` only

**Problem:** When `module-a` changes:
- Naive approach: Rebuild ALL services (slow)
- Desired: Rebuild only `service-a` and `service-b` (fast)

### Additional Complexity

1. **Function-level granularity**: Changing one function in a module shouldn't rebuild services that don't call it
2. **External dependency changes**: If an external library version changes, affected services must rebuild even if their internal code didn't change
3. **Moved/renamed code**: Moving a function to another file shouldn't trigger rebuilds if logic is unchanged

---

## Architecture

### High-Level Pipeline

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           BUILDGRAPH PIPELINE                                │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────┐     ┌─────────────────┐     ┌─────────────────────────┐
│   1. LOAD       │────▶│   2. DETECT     │────▶│   3. IMPACT             │
│   BASELINE      │     │   CHANGES       │     │   ANALYSIS              │
│                 │     │                 │     │                         │
│ - Load cached   │     │ - Compare AST   │     │ - Walk call graph       │
│   graph         │     │   hashes        │     │ - Find downstream       │
│ - Load previous │     │ - Compare ext   │     │   functions             │
│   hashes        │     │   deps          │     │ - Group by service      │
│                 │     │ - Find new/rm   │     │                         │
└─────────────────┘     └─────────────────┘     └─────────────────────────┘
         │                                               │
         │                                               ▼
         │                                    ┌─────────────────────────┐
         │                                    │   4. OUTPUT             │
         │                                    │                         │
         │                                    │ - List of services     │
         │                                    │   to build              │
         │                                    │ - Change details       │
         │                                    │ - Impact mapping       │
         │                                    └─────────────────────────┘
         │
         ▼
┌─────────────────────────────┐
│   5. BUILD & CACHE          │
│                             │
│ - Build affected services  │
│ - On success: upload       │
│   new baseline/cache       │
└─────────────────────────────┘
```

### Component Architecture

```
buildgraph/
├── cmd/
│   └── buildgraph/
│       └── main.go           # CLI entry point
├── pkg/
│   ├── analyzer/
│   │   ├── ast.go            # AST parsing utilities
│   │   ├── callgraph.go     # Call graph builder
│   │   └── deps.go          # Dependency extraction
│   ├── diff/
│   │   ├── detector.go      # Change detection logic
│   │   └── hasher.go        # Function hashing
│   ├── impact/
│   │   ├── walker.go        # Graph traversal
│   │   └── service.go       # Service grouping
│   ├── storage/
│   │   ├── baseline.go      # Baseline I/O
│   │   └── cache.go         # Cache management
│   └── types/
│       └── types.go         # All data structures
├── config/
│   └── config.go            # Configuration
├── .buildgraph/
│   ├── config.yaml          # Project config
│   └── cache/               # Cache directory
└── SPEC.md                  # This document
```

---

## Data Structures

### Core Types

```go
// Package represents a Go package with its module information
type Package struct {
    Path    string `json:"path"`    // Full import path (e.g., "github.com/user/repo/core/a")
    Name    string `json:"name"`    // Package name (e.g., "a")
    Version string `json:"version"` // Version from go.mod (e.g., "v1.2.3")
    Module  string `json:"module"`  // Module path (e.g., "github.com/user/repo")
}

// Dependency represents a function or method that another function calls
type Dependency struct {
    Package Package `json:"package"` // The package containing the dependency
    Type    string  `json:"type"`    // "internal" or "external"
    Name    string  `json:"name"`    // Function/method name (e.g., "Foo")
    FullName string `json:"full_name"` // Full qualified name (e.g., "core/a.Foo")
}

// Function represents a single function or method in the codebase
type Function struct {
    Name          string       `json:"name"`          // Function name (e.g., "Foo")
    FullName      string       `json:"full_name"`     // Fully qualified name (e.g., "core/a.Foo")
    Package       string       `json:"package"`       // Package path (e.g., "core/a")
    File          string       `json:"file"`          // Source file relative path
    StartLine     int          `json:"start_line"`   // Line number where function starts
    EndLine       int          `json:"end_line"`      // Line number where function ends
    IsExported    bool         `json:"is_exported"`   // Whether function is exported (capitalized)
    IsMain        bool         `json:"is_main"`       // Whether this is a main() function
    
    // Hashing
    ASTHash         string       `json:"ast_hash"`        // SHA256 of normalized AST
    TransitiveHash  string       `json:"transitive_hash"` // Hash including dependencies
    Deps            []Dependency `json:"deps"`            // Direct dependencies (calls)
}

// CallGraph represents the entire call graph of the project
type CallGraph struct {
    // Map of function full name -> Function
    // Key examples: "service-a.main", "core/a.Foo", "core/a.Bar"
    Nodes map[string]Function `json:"nodes"`
    
    // Reverse index: function -> callers (who calls this function)
    // Used for impact analysis: given X changed, find all who call X
    ReverseIndex map[string][]string `json:"reverse_index"`
    
    // Service membership: which service/core module a function belongs to
    // Key: function full name, Value: service/module name
    FunctionOwner map[string]string `json:"function_owner"`
}
```

### Baseline Storage

```go
// Baseline represents the stored state from a previous successful build
type Baseline struct {
    Version       string            `json:"version"`        // "1.0" - for schema migration
    GeneratedAt   time.Time          `json:"generated_at"`  // When this baseline was created
    Commit        string            `json:"commit"`         // Git commit SHA
    GoVersion     string            `json:"go_version"`     // Go version used
    ModulePath    string            `json:"module_path"`    // Root module path
    
    // The call graph
    Graph CallGraph `json:"graph"`
    
    // Function hashes for quick comparison
    // Key: function full name, Value: HashInfo
    FunctionHashes map[string]HashInfo `json:"function_hashes"`
    
    // External dependency versions at time of baseline
    ExternalDeps map[string]string `json:"external_deps"` // package path -> version
    
    // Source file hashes (to detect file moves/additions)
    SourceHashes map[string]string `json:"source_hashes"` // file path -> hash
}

// HashInfo stores all hash-related information for a function
type HashInfo struct {
    ASTHash        string   `json:"ast_hash"`        // Hash of function body AST
    TransitiveHash string   `json:"transitive_hash"` // Hash including dependency hashes
    DepsHash       string   `json:"deps_hash"`       // Hash of dependency list (for external dep changes)
    ExternalDeps   []string `json:"external_deps"`  // List of external packages this function calls
}
```

### Change Detection Output

```go
// Change represents a single change detected between builds
type Change struct {
    Function string `json:"function"` // Full function name
    Type     string `json:"type"`     // "modified", "added", "removed", "external_dep_changed"
    Reason   string `json:"reason"`   // Human-readable reason
    
    // For "modified"
    OldHash string `json:"old_hash,omitempty"`
    NewHash string `json:"new_hash,omitempty"`
    
    // For "external_dep_changed"
    Package  string `json:"package,omitempty"`
    OldVer   string `json:"old_version,omitempty"`
    NewVer   string `json:"new_version,omitempty"`
}

// Impact represents the impact analysis result
type Impact struct {
    // Functions changed directly or transitively, grouped by owner
    // Key: service/module name, Value: list of affected functions
    AffectedFunctions map[string][]string `json:"affected_functions"`
    
    // Reason why each service is affected
    // Key: service name, Value: human-readable explanation
    AffectReasons map[string][]string `json:"affect_reasons"`
    
    // Final list of services that need to be built
    ServicesToBuild []string `json:"services_to_build"`
    
    // Detailed change information
    Changes []Change `json:"changes"`
}

// Result is the final output of the buildgraph analysis
type Result struct {
    Timestamp        time.Time `json:"timestamp"`
    PreviousCommit   string     `json:"previous_commit"`
    CurrentCommit    string     `json:"current_commit"`
    PreviousBaseline string    `json:"previous_baseline"` // timestamp
    
    // Change detection
    HasChanges       bool       `json:"has_changes"`
    Changes          []Change   `json:"changes"`
    
    // Impact analysis
    Impact           Impact     `json:"impact"`
    
    // Debug info (optional, for troubleshooting)
    Debug *DebugInfo `json:"debug,omitempty"`
}

type DebugInfo struct {
    FilesParsed     int      `json:"files_parsed"`
    FunctionsFound  int      `json:"functions_found"`
    AnalysisTimeMs  int64    `json:"analysis_time_ms"`
    CacheHit        bool     `json:"cache_hit"`
}
```

### Configuration

```go
// Config represents buildgraph configuration
type Config struct {
    // Root module path (auto-detected if not set)
    ModulePath string `yaml:"module_path"`
    
    // Directories to scan
    Directories DirectoriesConfig `yaml:"directories"`
    
    // Files and patterns to exclude
    Exclude ExcludeConfig `yaml:"exclude"`
    
    // Output settings
    Output OutputConfig `yaml:"output"`
    
    // Cache settings
    Cache CacheConfig `yaml:"cache"`
}

type DirectoriesConfig struct {
    // Services directory (where microservices are located)
    Services string `yaml:"services"` // default: "services"
    
    // Core modules directory (shared libraries)
    Core string `yaml:"core"` // default: "core"
    
    // Additional directories to include
    Additional []string `yaml:"additional"`
}

type ExcludeConfig struct {
    // Patterns to exclude (standard filepath.Match)
    Patterns []string `yaml:"patterns"`
    
    // Vendor directories
    SkipVendor bool `yaml:"skip_vendor"` // default: true
    
    // Test files
    SkipTests bool `yaml:"skip_tests"` // default: true
}

type OutputConfig struct {
    // Output format: "json", "yaml", "text"
    Format string `yaml:"format"`
    
    // Output file (empty = stdout)
    File string `yaml:"file"`
    
    // Include debug information
    Verbose bool `yaml:"verbose"`
}

type CacheConfig struct {
    // Enable caching
    Enabled bool `yaml:"enabled"`
    
    // Cache directory (relative to project root)
    Directory string `yaml:"directory"` // default: ".buildgraph/cache"
    
    // Cache key components
    IncludeGoVersion bool `yaml:"include_go_version"` // default: true
}
```

---

## Algorithms

### Algorithm 1: Build Call Graph

```
BUILD_CALL_GRAPH(rootModulePath) -> CallGraph

Input: 
  - rootModulePath: The root Go module path (e.g., "github.com/user/repo")
  
Output:
  - CallGraph with all nodes, edges, reverse index, and function owners

Steps:

1. DISCOVER_PACKAGES()
   a. Scan directories defined in config (services/*/, core/*/)
   b. For each directory with go.mod, create Package struct
   c. Load module version from go.mod
   d. Return: Map of package path -> Package

2. PARSE_SOURCE_FILES(packages)
   a. For each package:
      i. List all .go files (excluding *_test.go if configured)
      ii. For each file:
          - Parse using go/parser.ParseFile()
          - AST.Inspect() to find all FuncDecl and FuncLit nodes
          - For each function:
              * Extract name, package, file, line numbers
              * Determine if exported (IsExported)
              * Determine if main function (Name == "main" && package == "main")
   b. Return: Map of function full name -> Function (without deps yet)

3. EXTRACT_DEPENDENCIES(functions, packages, rootModulePath)
   a. For each function in functions:
      i. Get AST node for the function
      ii. Walk the AST:
          - Handle *ast.CallExpr: extract function being called
          - Handle *ast.SelectorExpr: resolve package + function name
          - Handle *ast.Ident: possible local function call
      iii. For each call found:
          - Resolve to full function name (e.g., "core/a.Foo")
          - Determine if internal or external:
              * Internal: package path starts with rootModulePath
              * External: otherwise
          - Look up version for external deps from packages map
      iv. Build Deps list
   b. Return: functions with populated Deps field

4. BUILD_INDICES(functions)
   a. Create nodes map (functions as-is)
   b. Create reverse index:
      for each function f in functions:
        for each dep d in f.Deps:
          if d.Type == "internal":
            reverseIndex[d.FullName].append(f.FullName)
   c. Create function owner map:
      for each function f in functions:
        owner = extractOwner(f.Package, config)
        functionOwner[f.FullName] = owner
   d. Return: CallGraph with all indices

5. RETURN CallGraph
```

### Algorithm 2: Compute Function Hash

```
COMPUTE_FUNCTION_HASH(function, baselineGraph) -> HashInfo

Input:
  - function: The Function to hash
  - baselineGraph: Previous CallGraph (for dependency hashes)
  
Output:
  - HashInfo with all hash values

Steps:

1. COMPUTE_AST_HASH(function)
   a. Get AST node for function body
   b. Normalize AST:
      - Sort import declarations
      - Remove comments
      - Print using go/ast.Inspect with consistent formatting
   c. hash = sha256(normalizedString)
   d. Return hash

2. COMPUTE_DEPS_HASH(function)
   a. For each dependency d in function.Deps:
      i. If d.Type == "internal":
         - Get hash from baselineGraph.FunctionHashes[d.FullName].TransitiveHash
      ii. If d.Type == "external":
         - Use d.Package.Version as string
   b. Sort dependency identifiers consistently
   c. combined = join(allIdentifiers, "|")
   d. depsHash = sha256(combined)
   e. Return depsHash

3. COMPUTE_TRANSITIVE_HASH(astHash, depsHash)
   a. combined = astHash + "|" + depsHash
   b. transitiveHash = sha256(combined)
   c. Return transitiveHash

4. EXTRACT_EXTERNAL_DEPS(function)
   a. For each dep in function.Deps:
      if dep.Type == "external":
        externalDeps.append(dep.Package.Path)
   b. Return externalDeps

5. RETURN HashInfo{
     ASTHash: astHash,
     TransitiveHash: transitiveHash,
     DepsHash: depsHash,
     ExternalDeps: externalDeps
   }
```

### Algorithm 3: Detect Changes

```
DETECT_CHANGES(currentBaseline, previousBaseline) -> []Change

Input:
  - currentBaseline: Newly computed baseline
  - previousBaseline: Previously stored baseline
  
Output:
  - List of changes between the two baselines

Steps:

1. CHECK_FOR_NEW_FUNCTIONS()
   a. For each function f in currentBaseline.Graph.Nodes:
      if f NOT in previousBaseline.Graph.Nodes:
        Change{
          Function: f.FullName,
          Type: "added",
          Reason: "new_function"
        }

2. CHECK_FOR_REMOVED_FUNCTIONS()
   a. For each function f in previousBaseline.Graph.Nodes:
      if f NOT in currentBaseline.Graph.Nodes:
        Change{
          Function: f.FullName,
          Type: "removed",
          Reason: "function_deleted"
        }

3. CHECK_FOR_MODIFIED_FUNCTIONS()
   a. For each function f in currentBaseline.Graph.Nodes:
      if f in previousBaseline.Graph.Nodes:
        oldInfo = previousBaseline.FunctionHashes[f.FullName]
        newInfo = currentBaseline.FunctionHashes[f.FullName]
        
        if newInfo.ASTHash != oldInfo.ASTHash:
          Change{
            Function: f.FullName,
            Type: "modified",
            Reason: "ast_hash_changed",
            OldHash: oldInfo.ASTHash,
            NewHash: newInfo.ASTHash
          }
        
        if newInfo.DepsHash != oldInfo.DepsHash:
          // External dependency changed
          // Find which external dep changed
          oldExt = set(oldInfo.ExternalDeps)
          newExt = set(newInfo.ExternalDeps)
          
          added = newExt - oldExt
          removed = oldExt - newExt
          
          for pkg := range added:
            // Get version from current baseline
            newVer := currentBaseline.ExternalDeps[pkg]
            oldVer := previousBaseline.ExternalDeps[pkg]
            
            Change{
              Function: f.FullName,
              Type: "external_dep_changed",
              Reason: "external_dependency_version_changed",
              Package: pkg,
              OldVer: oldVer,
              NewVer: newVer
            }

4. RETURN allChanges
```

### Algorithm 4: Impact Analysis

```
COMPUTE_IMPACT(changes, callGraph) -> Impact

Input:
  - changes: List of Change from DETECT_CHANGES
  - callGraph: Current CallGraph
  
Output:
  - Impact with affected services and services to build

Steps:

1. INITIALIZE()
   a. affectedFunctions = {} // map of owner -> list of functions
   b. affectReasons = {} // map of owner -> list of reasons
   c. changedFunctions = set of all function names from changes

2. PROPAGATE_CHANGES(changedFunctions, visited)
   a. For each function f in changedFunctions:
      if f in visited:
        continue
      
      visited.add(f)
      
      // Find who calls this function
      callers = callGraph.ReverseIndex[f]
      
      for each caller in callers:
        // Add caller to changed set (it needs to be rebuilt)
        changedFunctions.add(caller)
        
        // Record impact
        owner = callGraph.FunctionOwner[caller]
        reason = "calls " + f
        
        affectedFunctions[owner].append(caller)
        affectReasons[owner].append(reason)
        
        // Continue propagation
        PROPAGATE_CHANGES({caller}, visited)

3. IDENTIFY_SERVICES_TO_BUILD(affectedFunctions)
   a. servicesToBuild = []
   b. For each owner := range affectedFunctions:
      // Check if this owner is a service (not core module)
      if isService(owner):
        servicesToBuild.append(owner)
   c. Sort servicesToBuild for deterministic output

4. RETURN Impact{
     AffectedFunctions: affectedFunctions,
     AffectReasons: affectReasons,
     ServicesToBuild: servicesToBuild,
     Changes: changes
   }
```

### Algorithm 5: Full Pipeline

```
RUN_BUILGRAPH() -> Result

Steps:

1. LOAD_CONFIG()
   a. Read .buildgraph/config.yaml or use defaults
   b. Detect root module path from go.mod

2. LOAD_PREVIOUS_BASELINE()
   a. Check cache directory for baseline.json
   b. If exists, unmarshal into PreviousBaseline
   c. If not exists, return nil (first run)

3. PARSE_CURRENT_CODE()
   a. Discover all packages in services/ and core/ directories
   b. Parse all Go source files
   c. Extract all functions
   d. Build call graph

4. COMPUTE_HASHES(callGraph, previousBaseline)
   a. For each function in callGraph.Nodes:
      hashInfo = COMPUTE_FUNCTION_HASH(function, previousBaseline?.Graph)
      functionHashes[f.FullName] = hashInfo
   
   b. Collect all external dependency versions from packages

5. DETECT_CHANGES()
   a. If previousBaseline exists:
      changes = DETECT_CHANGES(currentBaseline, previousBaseline)
   b. If no previous baseline:
      // First run - all functions are "new"
      changes = all functions marked as "added"

6. COMPUTE_IMPACT()
   a. impact = COMPUTE_IMPACT(changes, callGraph)

7. BUILD_RESULT()
   a. result = Result{
      Timestamp: now(),
      PreviousCommit: previousBaseline?.Commit,
      CurrentCommit: getCurrentGitCommit(),
      HasChanges: len(changes) > 0,
      Changes: changes,
      Impact: impact
    }

8. OUTPUT(result)
   a. Write result to configured output (file or stdout)
   b. If configured, write JSON for machine consumption

9. RETURN result
```

---

## CLI Interface

### Command Structure

```
buildgraph [global flags] <command> [command flags]
```

All config file fields can be overridden by flags.  Priority order (highest to lowest):

1. CLI flag
2. Environment variable (`BUILDGRAPH_<KEY>`)
3. `buildgraph.yaml`
4. Built-in default

### Commands

#### 1. `analyze` - Detect which services need to be rebuilt

```bash
buildgraph analyze [flags]

Flags:
  -f, --format string     Output format: json, text (default: json)
  -o, --output string     Output file (default: stdout)
  -v, --verbose           Include debug info in output
      --no-cache          Ignore baseline, treat everything as new

Global flags (also settable in buildgraph.yaml):
      --config string     Config file path (default: buildgraph.yaml)
      --services strings  Service directories (default: [services])
      --baseline string   Baseline file path (default: .buildgraph/baseline.json)
      --skip-vendor       Skip vendor/ directory (default: true)
      --skip-tests        Skip *_test.go files (default: true)
```

**Example:**
```bash
# Standard run — reads buildgraph.yaml, outputs JSON to stdout
buildgraph analyze

# Output to file
buildgraph analyze --format json --output impact.json

# Override services directory for a non-standard layout
buildgraph analyze --services apps

# Force fresh analysis ignoring any stored baseline
buildgraph analyze --no-cache
```

#### 2. `generate` - Save a new baseline snapshot

```bash
buildgraph generate [flags]

Flags:
  -o, --output string     Output path for the baseline (overrides config)
```

**Example:**
```bash
# Generate and save baseline (path from buildgraph.yaml or default)
buildgraph generate

# Save to a custom path
buildgraph generate --output /tmp/baseline.json
```

#### 3. `init` - Create a starter config file

```bash
buildgraph init
```

Writes a commented `buildgraph.yaml` in the current directory.
Exits with an error if the file already exists.

### Output Formats

#### JSON Output

```json
{
  "timestamp": "2026-03-07T10:30:00Z",
  "previous_commit": "abc123",
  "current_commit": "def456",
  "has_changes": true,
  "changes": [
    {
      "function": "core/a.Foo",
      "type": "modified",
      "reason": "ast_hash_changed"
    }
  ],
  "impact": {
    "affected_functions": {
      "service-a": ["core/a.Foo"],
      "service-b": ["core/a.Foo"]
    },
    "affect_reasons": {
      "service-a": ["calls core/a.Foo"],
      "service-b": ["calls core/a.Foo"]
    },
    "services_to_build": ["service-a", "service-b"]
  }
}
```

#### Text Output

```
=== BuildGraph Analysis ===

Changes detected: 1
  core/a.Foo: modified (ast_hash_changed)

Impact Analysis:
  service-a: affected (calls core/a.Foo)
  service-b: affected (calls core/a.Foo)

Services to build:
  - service-a
  - service-b
```

---

## Configuration

### Config File

Location: `buildgraph.yaml` (repo root).  Created by `buildgraph init`.

```yaml
# BuildGraph configuration

# Directories whose immediate subdirectories are deployable services.
# Each subdirectory must contain a main package.
services:
  - services

# Files and directories to skip during analysis.
exclude:
  skip_vendor: true
  skip_tests: true
  patterns:
    - "**/*_gen.go"
    - "**/mock_*.go"

# Path where the baseline snapshot is stored.
# Add .buildgraph/ to your .gitignore.
baseline: .buildgraph/baseline.json
```

### Environment Variables

Every config key can be overridden via an environment variable of the form
`BUILDGRAPH_<KEY>` (dots and dashes replaced with underscores, uppercased).

| Variable | Description | Default |
|----------|-------------|---------|
| `BUILDGRAPH_SERVICES` | Service directories | `services` |
| `BUILDGRAPH_BASELINE` | Baseline file path | `.buildgraph/baseline.json` |
| `BUILDGRAPH_EXCLUDE_SKIP_VENDOR` | Skip vendor/ | `true` |
| `BUILDGRAPH_EXCLUDE_SKIP_TESTS` | Skip test files | `true` |

---

## Storage Format

### Baseline File

Location: `.buildgraph/baseline.json` (configurable via `baseline` key or `--baseline` flag)

```json
{
  "version": "1.0",
  "generated_at": "2026-03-07T10:30:00Z",
  "commit": "abc123def456",
  "go_version": "1.24.2",
  "module_path": "github.com/user/repo",
  
  "graph": {
    "nodes": {
      "service-a.main": {
        "name": "main",
        "full_name": "service-a.main",
        "package": "service-a",
        "file": "services/service-a/main.go",
        "start_line": 1,
        "end_line": 30,
        "is_exported": false,
        "is_main": true,
        "ast_hash": "sha256:a1b2c3...",
        "transitive_hash": "sha256:d4e5f6...",
        "deps": [
          {
            "package": {
              "path": "github.com/user/repo/core/a",
              "name": "a",
              "version": "",
              "module": "github.com/user/repo"
            },
            "type": "internal",
            "name": "Foo",
            "full_name": "core/a.Foo"
          },
          {
            "package": {
              "path": "github.com/pkg/foo",
              "name": "foo",
              "version": "v1.2.3",
              "module": "github.com/pkg/foo"
            },
            "type": "external",
            "name": "Bar",
            "full_name": "pkg/foo.Bar"
          }
        ]
      },
      "core/a.Foo": {
        "name": "Foo",
        "full_name": "core/a.Foo",
        "package": "core/a",
        "file": "core/a/foo.go",
        "start_line": 10,
        "end_line": 25,
        "is_exported": true,
        "is_main": false,
        "ast_hash": "sha256:...",
        "transitive_hash": "sha256:...",
        "deps": [
          {
            "package": {
              "path": "github.com/user/repo/core/b",
              "name": "b",
              "version": "",
              "module": "github.com/user/repo"
            },
            "type": "internal",
            "name": "Baz",
            "full_name": "core/b.Baz"
          }
        ]
      }
    },
    "reverse_index": {
      "core/a.Foo": ["service-a.main"],
      "core/b.Baz": ["core/a.Foo"]
    },
    "function_owner": {
      "service-a.main": "service-a",
      "core/a.Foo": "core/a",
      "core/b.Baz": "core/b"
    }
  },
  
  "function_hashes": {
    "service-a.main": {
      "ast_hash": "sha256:abc...",
      "transitive_hash": "sha256:def...",
      "deps_hash": "sha256:ghi...",
      "external_deps": ["github.com/pkg/foo"]
    },
    "core/a.Foo": {
      "ast_hash": "sha256:jkl...",
      "transitive_hash": "sha256:mno...",
      "deps_hash": "sha256:pqr...",
      "external_deps": []
    }
  },
  
  "external_deps": {
    "github.com/pkg/foo": "v1.2.3",
    "github.com/pkg/bar": "v2.0.0"
  },
  
  "source_hashes": {
    "services/service-a/main.go": "sha256:...",
    "core/a/foo.go": "sha256:...",
    "core/b/baz.go": "sha256:..."
  }
}
```

### Cache Metadata

Location: `.buildgraph/cache/metadata.json`

```json
{
  "version": "1.0",
  "created_at": "2026-03-07T10:30:00Z",
  "last_used_at": "2026-03-07T10:30:00Z",
  "git_commit": "abc123",
  "go_version": "1.24.2",
  "source_hash": "sha256:...",
  "function_count": 150,
  "service_count": 3,
  "module_count": 5
}
```

---

## Pipeline Integration

### GitHub Actions Example

```yaml
name: Build and Test

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  analyze:
    runs-on: ubuntu-latest
    outputs:
      services: ${{ steps.buildgraph.outputs.services }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.24'

      - name: Download previous baseline
        uses: actions/download-artifact@v4
        with:
          name: buildgraph-baseline
          path: .buildgraph/cache
        continue-on-error: true

      - name: Run BuildGraph analysis
        id: buildgraph
        run: |
          go run ./cmd/buildgraph analyze -f json -o impact.json
          
          # Extract services to build
          SERVICES=$(jq -r '.impact.services_to_build[]' impact.json)
          echo "services=$SERVICES" >> $GITHUB_OUTPUT
          
          # If no services to build, build all
          if [ -z "$SERVICES" ]; then
            echo "services=service-a,service-b,service-c" >> $GITHUB_OUTPUT
          fi

      - name: Upload baseline
        if: success()
        uses: actions/upload-artifact@v4
        with:
          name: buildgraph-baseline
          path: .buildgraph/cache/
          retention-days: 30

  build:
    needs: analyze
    runs-on: ubuntu-latest
    strategy:
      matrix:
        service: ${{ fromJson(needs.analyze.outputs.services) }}
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5

      - name: Build ${{ matrix.service }}
        run: |
          go build -o bin/${{ matrix.service }} ./services/${{ matrix.service }}
```

### GitLab CI Example

```yaml
stages:
  - analyze
  - build
  - test

variables:
  BUILDGGRAPH_VERSION: "v1.0.0"

buildgraph:
  stage: analyze
  image: golang:1.24
  cache:
    key: buildgraph
    paths:
      - .buildgraph/cache/
  script:
    - go install github.com/user/buildgraph@latest
    - buildgraph analyze -f json -o impact.json
  artifacts:
    paths:
      - impact.json
    when: always
  rules:
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
    - if: '$CI_COMMIT_BRANCH == "main"'

build-services:
  stage: build
  image: golang:1.24
  script:
    - |
      SERVICES=$(jq -r '.impact.services_to_build // [] | join(",")' impact.json)
      if [ -z "$SERVICES" ]; then
        echo "No services to build"
      else
        for service in $(echo "$SERVICES" | tr ',' ' '); do
          echo "Building $service"
          go build -o bin/$service ./services/$service
        done
      fi
  needs:
    - job: buildgraph
      artifacts: true
```

### Makefile Integration

```makefile
.PHONY: analyze build-services

ANALYZE_JSON := impact.json

analyze:
	go run ./cmd/buildgraph analyze -f json -o $(ANALYZE_JSON)

build-services: analyze
	@SERVICES=$$(jq -r '.impact.services_to_build // [] | join(" ")' $(ANALYZE_JSON)) && \
	if [ -z "$$SERVICES" ]; then \
		echo "No services to build"; \
		for svc in $(SERVICES); do \
			echo "Building $$svc"; \
			go build -o bin/$$svc ./services/$$svc; \
		done \
	else \
		echo "Building all services"; \
		for svc in $(SERVICES); do \
			echo "Building $$svc"; \
			go build -o bin/$$svc ./services/$$svc; \
		done \
	fi

generate-baseline:
	go run ./cmd/buildgraph generate -o .buildgraph/cache/baseline.json
```

---

## Edge Cases

### 1. Function Moved to Different File

**Scenario:** Function `Foo` is moved from `core/a/foo.go` to `core/a/bar.go`

**Detection:**
- Source hashes differ for old file and new file
- But AST content is the same → AST hash unchanged
- Transitive hash unchanged → no impact

**Result:** No rebuild triggered ✓

### 2. Function Renamed

**Scenario:** Function `Foo` renamed to `Bar`

**Detection:**
- Old function `core/a.Foo` removed from nodes
- New function `core/a.Bar` added to nodes
- Callers are updated in their imports (assume they compile)

**Result:** Callers become "new" functions → rebuild triggered

### 3. Only Comments Changed

**Scenario:** Comments added/removed from function

**Detection:**
- AST parsing removes comments before hashing
- AST hash unchanged

**Result:** No rebuild triggered ✓

### 4. Interface Usage

**Scenario:**
```go
// core/a/handler.go
type Handler interface {
    Process(data string) error
}

// service-a/main.go  
var h Handler = &ConcreteHandler{}
h.Process("test")
```

**Detection:**
- Static analysis sees `h.Process()` call
- Cannot determine concrete implementation at compile time
- Must assume all implementations could be affected

**Result:** Conservative - may over-build, but won't miss changes ✓

### 5. Reflection Usage

**Scenario:**
```go
func CallViaReflection(fn interface{}, args ...interface{}) {
    reflect.ValueOf(fn).Call(args)
}
```

**Detection:**
- `go/ast` cannot see what function is passed at runtime

**Result:** Cannot detect → user must manually trigger full rebuild

### 6. Build Tags

**Scenario:**
```go
//go:build prod
func ProdOnly() { ... }

//go:build dev
func DevOnly() { ... }
```

**Detection:**
- Parser must decide which to include
- Use `go/packages` with build constraints

**Result:** Configurable - can choose which tags to analyze

### 7. Circular Dependencies

**Scenario:** Package A imports Package B, Package B imports Package A

**Detection:**
- Go doesn't allow circular imports at package level
- Call graph handles function-level cycles

**Result:** DAG is acyclic by Go's nature ✓

### 8. First Run (No Baseline)

**Scenario:** Running buildgraph for the first time

**Detection:**
- No previous baseline to compare
- All functions marked as "added"

**Result:** All services marked for build ✓

### 9. External Dependency Version Changed

**Scenario:** `github.com/pkg/foo` updated from `v1.2.3` to `v1.3.0` in go.mod

**Detection:**
- External deps map shows version change
- Any function calling external dep has `DepsHash` changed

**Result:** Those functions marked as changed → impact propagation ✓

### 10. Services in Same Repository

**Scenario:** `service-a` imports `service-b` directly

**Detection:**
- Both are internal (module path matches)
- Both have main() functions
- Call graph treats service-b as internal function from service-a's perspective

**Result:** If service-b changes, service-a is marked for rebuild

---

## Performance Considerations

### Parsing Performance

| Project Size | Estimated Time |
|--------------|-----------------|
| 50 functions | < 1 second |
| 500 functions | 2-5 seconds |
| 5000 functions | 30-60 seconds |

### Caching Strategy

1. **Source file hash**: Skip re-parsing unchanged files
2. **Function AST hash**: Skip re-hashing unchanged functions
3. **Graph structure**: Cache call graph, rebuild only if structure changes

### Parallelization

- Parse multiple packages in parallel
- Hash multiple functions in parallel
- Use worker pool for large codebases

### Memory Usage

- Call graph for 5000 functions: ~50MB
- Baseline JSON for 5000 functions: ~5MB

---

## Bottlenecks and Mitigations

### Potential Bottlenecks

This section identifies performance bottlenecks in the implementation and provides strategies to overcome them.

### 1. Parsing Large Codebases

| Problem | Impact | Mitigation |
|---------|--------|------------|
| Parsing 5000+ Go files | 30-60 seconds per run | Incremental parsing + caching |
| Repeated parsing on every run | Wasteful, slow CI | Skip files with unchanged source hash |

**Mitigation: Source Hash Caching**

```
Before: Parse all files every run
After:  
  1. Hash each source file → store in SourceHashes
  2. On next run, compare source hashes
  3. Skip parsing files where hash unchanged
  4. Only parse changed/new files
```

**Data Structure Addition:**

```go
type SourceFileCache struct {
    File    string `json:"file"`     // Relative path
    Hash    string `json:"hash"`     // SHA256 of content
    Parsed  bool   `json:"parsed"`   // Whether AST was parsed
}
```

### 2. AST Hash Computation

| Problem | Impact | Mitigation |
|---------|--------|------------|
| Hashing large functions | CPU-intensive | Hash only significant nodes |
| Hash normalization complexity | Hard to maintain | Use go fmt + hash |
| Computing transitive hashes | O(n²) potential | Topological sort + memoization |

**Mitigation: Optimized Hashing**

```go
// 1. Normalize AST: use go/format to get canonical form
func normalizeAST(node ast.Node) []byte {
    return format.Node(nil, fset, node)
}

// 2. Memoize: cache hash results during computation
var hashCache = make(map[string]string)

func computeHashWithMemoization(node ast.Node, deps []string) string {
    key := hashKey(node, deps)
    if cached, ok := hashCache[key]; ok {
        return cached
    }
    // ... compute ...
    hashCache[key] = result
    return result
}

// 3. Transitive hash: process in topological order
// Since call graph is DAG, process in reverse topological order
// Leaf functions first → their hashes are available for parents
```

### 3. Dependency Resolution

| Problem | Impact | Mitigation |
|---------|--------|------------|
| Resolving import paths | Slow, complex | Use `go/packages` (calls `go list`) |
| External dep version lookup | Repeated work | Cache package→version mapping |

**Mitigation: Pre-built Dependency Map**

```go
type DepResolver struct {
    // Pre-built map: package path → version
    packageVersions map[string]string
    
    // Memoization: path → resolved full name
    resolutionCache map[string]string
}

func NewDepResolver(rootModule string) *DepResolver {
    // Parse all go.mod files once
    // Build package → version map
    // Store in resolver for reuse
}
```

### 4. Call Graph Size

| Problem | Impact | Mitigation |
|---------|--------|------------|
| 10,000+ functions in graph | Memory pressure | Use efficient data structures |
| Slow lookups during impact analysis | Delays CI | Pre-build indexes |

**Mitigation: Pre-computed Indexes**

The baseline already includes `ReverseIndex`, but we can add an **Impact Index**:

```go
type ImpactIndex struct {
    // For every function, pre-compute which services depend on it
    // Key: function full name, Value: list of service names
    FunctionToServices map[string][]string
    
    // For every service, pre-compute which core modules it depends on
    // Key: service name, Value: list of module names
    ServiceToModules map[string][]string
}
```

**Impact Analysis with Index:**

```
Before (on every run):
  changed = {core/a.Foo}
  for each func in changed:
    callers = reverseIndex[func]    // O(1) lookup
    for each caller in callers:
      changed.add(caller)           // Add to set
      owner = functionOwner[caller] // O(1) lookup
      affected[owner].append(caller)
  Repeat until fixed point

After (pre-computed, stored in baseline):
  changed = {core/a.Foo}
  affectedServices = impactIndex[core/a.Foo]  // O(1)!
  servicesToBuild = filterToServices(affectedServices)
```

### 5. Graph Traversal for Impact

| Problem | Impact | Mitigation |
|---------|--------|------------|
| Deep call chains | Stack overflow risk | Use iterative (queue-based) traversal |
| Repeated traversals | Wasteful | Pre-compute impact index |

**Mitigation: Iterative Traversal**

```go
func computeImpact(changedFunctions map[string]bool, callGraph *CallGraph) map[string][]string {
    affected := make(map[string][]string)
    visited := make(map[string]bool)
    queue := make([]string, 0)
    
    // Initialize queue with changed functions
    for f := range changedFunctions {
        queue = append(queue, f)
    }
    
    // Iterative BFS
    for len(queue) > 0 {
        current := queue[0]
        queue = queue[1:]
        
        if visited[current] {
            continue
        }
        visited[current] = true
        
        // Find callers (reverse index)
        callers := callGraph.ReverseIndex[current]
        for _, caller := range callers {
            if !visited[caller] {
                queue = append(queue, caller)
                
                // Record impact
                owner := callGraph.FunctionOwner[caller]
                affected[owner] = append(affected[owner], caller)
            }
        }
    }
    
    return affected
}
```

### 6. I/O Bottlenecks

| Problem | Impact | Mitigation |
|---------|--------|------------|
| Reading many small files | Disk I/O | Batch reads, parallel file reads |
| JSON serialization of large graph | CPU, large files | Binary format or compression |

**Mitigation: Storage Optimization**

```go
type StorageFormat int

const (
    FormatJSON StorageFormat = iota
    FormatJSONGzip
    FormatMessagePack
    FormatSQLite
)

// For large graphs: use gzip compression
func saveBaselineGz(baseline *Baseline, path string) error {
    data, err := json.Marshal(baseline)
    if err != nil {
        return err
    }
    
    var buf bytes.Buffer
    w := gzip.NewWriter(&buf)
    w.Write(data)
    w.Close()
    
    return os.WriteFile(path, buf.Bytes(), 0644)
}
```

### 7. Memory Usage

| Problem | Impact | Mitigation |
|---------|--------|------------|
| Loading entire graph into memory | RAM pressure | Stream processing, lazy loading |
| Baseline JSON too large | Slow parse/serialize | Compress, split files |

**Mitigation: Lazy Loading**

```go
type LazyCallGraph struct {
    nodes         map[string]*Function
    reverseIndex  map[string][]string
    functionOwner map[string]string
    
    // Lazy: only load when needed
    fullGraphPath string
}

func (g *LazyCallGraph) GetFunction(name string) (*Function, error) {
    if fn, ok := g.nodes[name]; ok {
        return fn, nil
    }
    // Lazy load from storage
    return g.loadFunction(name)
}
```

### 8. External Dependency Version Tracking

| Problem | Impact | Mitigation |
|---------|--------|------------|
| Need to detect go.mod changes | Extra parsing | Track go.mod hash separately |

**Mitigation: go.mod Change Detection**

```go
type ExternalDepTracker struct {
    goModHash string          // Hash of go.mod content
    versions  map[string]string // package → version
    
    // Computed: functions whose deps changed
    affectedFunctions map[string][]string
}

func (e *ExternalDepTracker) DetectChanges() []string {
    // If go.mod hash changed, recompute affected functions
    // Otherwise, use cached result
}
```

### Optimization Summary

| Phase | Naive Approach | Optimized Approach | Speedup |
|-------|----------------|-------------------|---------|
| **Parsing** | Parse all files | Source hash check → skip unchanged | ~10x |
| **Hashing** | Hash every function | Incremental: only changed + dependents | ~5x |
| **Graph Lookup** | Build on demand | Pre-built indexes in baseline | ~100x |
| **Impact** | Graph traversal | O(1) impact index lookup | ~1000x |
| **Storage** | Plain JSON | Compressed binary | ~3x |
| **Compute** | Sequential | Parallel workers | ~4x (4 cores) |

### Recommended Implementation Priority

1. **Source hash caching** (Week 1-2) - Biggest win, simplest to implement
2. **Impact index pre-computation** (Week 2) - Enables O(1) lookups
3. **Parallel parsing** (Week 3) - Linear speedup with worker pool
4. **Binary storage** (Week 4) - Reduce I/O, faster loads
5. **Lazy loading** (Week 5) - Memory optimization for large codebases

### Performance Targets

| Metric | Naive Target | Optimized Target |
|--------|--------------|------------------|
| First run (5000 functions) | 60 seconds | 30 seconds |
| Incremental run (1% changed) | 60 seconds | 2 seconds |
| Memory usage | 500MB | 200MB |
| Baseline file size | 10MB | 3MB |

---

## Implementation Status

### Phase 1: Core Functionality ✅ COMPLETE

- [x] Project setup with Go modules
- [x] Basic AST parsing for functions
- [x] Simple call graph building (direct calls only)
- [x] JSON baseline storage (generate command)
- [x] Basic CLI with `analyze` command

### Phase 2: Dependency Analysis ✅ COMPLETE

- [x] Resolve package imports to full paths
- [x] Distinguish internal vs external dependencies
- [x] Extract external dependency versions from go.mod
- [x] Build reverse index for impact analysis

### Phase 3: Change Detection ✅ COMPLETE

- [x] AST normalization and hashing
- [x] Transitive hash computation
- [x] External dependency change detection
- [x] Diff output with change reasons

### Phase 4: Impact Analysis ✅ COMPLETE

- [x] Graph traversal for impact propagation
- [x] Service grouping and filtering
- [x] Output formatting (JSON, YAML, text)

### Phase 5: Optimization ✅ COMPLETE

- [x] Source hash caching — `ComputeSourceHashes()` hashes every loaded `.go`
  file; stored in `Baseline.SourceHashes`. On re-runs, unchanged files skip
  AST re-hashing and reuse stored `HashInfo` values (fast-path in
  `ComputeHashes`).
- [x] `services_to_build` sorted for deterministic output
- [ ] Parallel package loading (deferred — `go/packages` already parallelises
  internally; manual worker pool adds complexity for marginal gain at current
  scale)
- [ ] Compressed/binary baseline storage (deferred — JSON is sufficient for
  the test project; opt-in gzip can be added when baseline files exceed ~5 MB)

### Phase 6: Integration ✅ COMPLETE

- [x] GitHub Actions workflow (`.github/workflows/buildgraph.yml`) — two-job
  pipeline: `analyze` exports `services` JSON array; `build` fans out with a
  matrix over affected services only
- [x] GitLab CI workflow (`.gitlab-ci.yml`) — `analyze` stage exports
  `SERVICES_TO_BUILD`; `build` stage uses `parallel:matrix`; dynamic child
  pipeline template included as commented alternative
- [x] Makefile examples (in SPEC.md)
- [x] Documentation

### Phase 7: CLI & Config Refactor ✅ COMPLETE

- [x] Replaced `flag` package with `cobra` + `viper` (spf13)
- [x] Config restructured: `services[]`, `exclude`, `baseline` only — no
  redundant or speculative fields
- [x] Config file moved to `buildgraph.yaml` at repo root
- [x] All config fields overridable via CLI flag or `BUILDGRAPH_*` env var
- [x] `.buildgraph/cache/baseline.json` flattened to `.buildgraph/baseline.json`
- [x] `buildgraph init` command generates a starter `buildgraph.yaml`
- [x] `storage.New()` simplified — no longer holds a cache directory; path is
  always passed explicitly by the caller

---

## Project Structure

```
buildgraph/
├── SPEC.md                              # This specification
├── buildgraph.yaml                      # Config (created by `buildgraph init`)
├── .github/
│   └── workflows/
│       └── buildgraph.yml              # GitHub Actions CI workflow
├── .gitlab-ci.yml                      # GitLab CI workflow
├── cmd/
│   └── main.go                         # CLI entry point (cobra + viper)
├── pkg/
│   ├── types/
│   │   └── types.go                    # Core data types
│   ├── config/
│   │   └── config.go                   # Config struct + defaults
│   ├── analyzer/
│   │   ├── analyzer.go                 # CHA call graph builder + hash computation
│   │   └── gomod.go                    # go.mod parser & external dep hashing
│   ├── diff/
│   │   └── detector.go                 # Change detection
│   ├── impact/
│   │   └── impact.go                   # Impact analysis (BFS, service grouping)
│   └── storage/
│       └── storage.go                  # Baseline JSON read/write
├── .buildgraph/
│   └── baseline.json                   # Generated — add .buildgraph/ to .gitignore
└── testproject/                        # Sample test project
    ├── go.mod                          # Single root module
    ├── buildgraph.yaml                 # Created by `buildgraph init`
    ├── services/
    │   ├── service-a/
    │   └── service-b/
    └── core/
        ├── module-a/
        └── module-b/
```

---

## Implementation Roadmap

### Phase 1: Core Functionality (Week 1-2)

- [ ] Project setup with Go modules
- [ ] Basic AST parsing for functions
- [ ] Simple call graph building (direct calls only)
- [ ] JSON baseline storage
- [ ] Basic CLI with `analyze` command

### Phase 2: Dependency Analysis (Week 3)

- [ ] Resolve package imports to full paths
- [ ] Distinguish internal vs external dependencies
- [ ] Extract external dependency versions from go.mod
- [ ] Build reverse index for impact analysis

### Phase 3: Change Detection (Week 4)

- [ ] AST normalization and hashing
- [ ] Transitive hash computation
- [ ] External dependency change detection
- [ ] Diff output with change reasons

### Phase 4: Impact Analysis (Week 5)

- [ ] Graph traversal for impact propagation
- [ ] Service grouping and filtering
- [ ] Output formatting (JSON, YAML, text)

### Phase 5: Optimization (Week 6)

- [ ] Incremental parsing (skip unchanged files)
- [ ] Caching improvements
- [ ] Parallel processing

### Phase 6: Integration (Week 7)

- [ ] GitHub Actions integration examples
- [ ] GitLab CI integration examples
- [ ] Makefile examples
- [ ] Documentation

---

## Appendix

### A. Go Analysis Libraries

| Library | Purpose |
|---------|---------|
| `go/parser` | Parse Go source to AST |
| `go/ast` | AST types and traversal |
| `go/types` | Type checking |
| `golang.org/x/tools/go/packages` | Package loading with deps |
| `golang.org/x/tools/go/callgraph` | Call graph construction |
| `golang.org/x/tools/go/analysis` | Analysis framework |

### B. Example Project Structure

```
github.com/user/repo/
├── .buildgraph/
│   ├── config.yaml
│   └── cache/
│       ├── baseline.json
│       └── metadata.json
├── services/
│   ├── service-a/
│   │   ├── main.go
│   │   └── go.mod
│   ├── service-b/
│   │   ├── main.go
│   │   └── go.mod
│   └── service-c/
│       ├── main.go
│       └── go.mod
├── core/
│   ├── module-a/
│   │   ├── foo.go
│   │   └── go.mod
│   ├── module-b/
│   │   ├── bar.go
│   │   └── go.mod
│   └── module-c/
│       ├── baz.go
│       └── go.mod
├── go.mod
├── go.sum
└── buildgraph
    ├── cmd/
    │   └── buildgraph/
    │       └── main.go
    ├── pkg/
    │   ├── analyzer/
    │   ├── diff/
    │   ├── impact/
    │   ├── storage/
    │   └── types/
    ├── config/
    └── go.mod
```

### C. Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success - analysis complete |
| 1 | Error - analysis failed |
| 2 | No changes detected |
| 3 | Invalid configuration |

### D. Testing Strategy

1. **Unit tests** for each algorithm component
2. **Integration tests** with sample monorepo
3. **Golden files** for expected outputs
4. **Benchmark tests** for performance regression
