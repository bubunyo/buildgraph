# BuildGraph

BuildGraph is a static analysis tool for Go monorepos that determines exactly which microservices need to be rebuilt when code changes — using a precise call graph rather than naive file-level diffing.

## The problem

In a Go monorepo with multiple services sharing internal packages, the standard CI approach is to rebuild everything on every commit. BuildGraph replaces that with a targeted rebuild: if you change a function in a shared module, only the services that actually call that function get rebuilt.

```
repo/
├── services/
│   ├── service-a/   # calls module-a and module-b
│   ├── service-b/   # calls module-a only
│   └── service-c/   # calls module-c only
└── core/
    ├── module-a/
    ├── module-b/
    └── module-c/
```

Change `module-b` → rebuild `service-a` only. Not `service-b`, not `service-c`.

BuildGraph achieves this by:

1. Building a full call graph using [Class Hierarchy Analysis](https://pkg.go.dev/golang.org/x/tools/go/callgraph/cha) — conservative, no false negatives
2. Hashing every function's SSA representation — detects logic changes, ignores formatting and comments
3. Storing a baseline snapshot after each successful build
4. On the next run, diffing the current call graph against the baseline and propagating changes upward through the reverse index to find affected services

## Installation

```bash
go install github.com/bubunyo/buildgraph/cmd/buildgraph@latest
```

Or build from source:

```bash
git clone https://github.com/bubunyo/buildgraph
cd buildgraph
go build -o buildgraph ./cmd/main.go
```

**Requires Go 1.24+**

## Quick start

```bash
# 1. Initialise config in your monorepo root
cd your-monorepo
buildgraph init

# 2. Generate the initial baseline (do this after a clean build)
buildgraph generate

# 3. On subsequent commits, analyse what changed
buildgraph analyze
```

## Commands

### `buildgraph init`

Writes a `buildgraph.yaml` in the current directory with commented defaults. Does not overwrite an existing file.

```bash
buildgraph init
```

### `buildgraph generate`

Parses the current codebase, builds the call graph, and saves a baseline snapshot. Run this after a successful build so the next `analyze` has something to compare against.

```bash
buildgraph generate

# Custom output path
buildgraph generate --output /tmp/baseline.json
```

### `buildgraph analyze`

Compares the current call graph against the stored baseline and outputs which services are affected.

```bash
# JSON output (default)
buildgraph analyze

# Human-readable
buildgraph analyze --format text

# Write to file
buildgraph analyze --format json --output impact.json

# Verbose — includes timing and cache info
buildgraph analyze --verbose

# Ignore baseline, treat everything as changed
buildgraph analyze --no-cache
```

**JSON output:**

```json
{
  "has_changes": true,
  "changes": [
    {
      "function": "github.com/your-org/repo/core/module-b.Save",
      "type": "modified",
      "reason": "ast_hash_changed"
    }
  ],
  "impact": {
    "affected_functions": {
      "services/service-a": ["github.com/your-org/repo/services/service-a.main"]
    },
    "services_to_build": ["service-a"]
  }
}
```

## Configuration

`buildgraph.yaml` lives at the repo root. Every key is also available as a CLI flag or `BUILDGRAPH_*` environment variable.

```yaml
# Directories whose immediate subdirectories are deployable services.
# Each subdirectory must contain a main package.
services:
  - services

# Files to skip during analysis.
exclude:
  skip_vendor: true
  skip_tests: true
  patterns:
    - "**/*_gen.go"
    - "**/mock_*.go"

# Where the baseline snapshot is stored.
baseline: .buildgraph/baseline.json
```

### Flag / env override examples

```bash
# Override services directory
buildgraph analyze --services apps

# Override via environment variable
BUILDGRAPH_SERVICES=apps buildgraph analyze

# Point to a different baseline
buildgraph analyze --baseline /ci/cache/baseline.json
```

### Priority order

CLI flag > environment variable > `buildgraph.yaml` > built-in default

## Gitignore

Add `.buildgraph/` to your `.gitignore` — the baseline is a build artifact, not source:

```
.buildgraph/
```

Commit `buildgraph.yaml`.

## CI integration

### GitHub Actions

```yaml
jobs:
  analyze:
    runs-on: ubuntu-latest
    outputs:
      services: ${{ steps.buildgraph.outputs.services }}
      has_changes: ${{ steps.buildgraph.outputs.has_changes }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Download previous baseline
        uses: actions/download-artifact@v4
        with:
          name: buildgraph-baseline
          path: .buildgraph
        continue-on-error: true  # first run has no artifact yet

      - name: Install buildgraph
        run: go install github.com/bubunyo/buildgraph/cmd/buildgraph@latest

      - name: Analyze
        id: buildgraph
        run: |
          buildgraph analyze --format json --output impact.json
          HAS_CHANGES=$(jq -r '.has_changes' impact.json)
          SERVICES=$(jq -c '.impact.services_to_build' impact.json)
          echo "has_changes=${HAS_CHANGES}" >> "$GITHUB_OUTPUT"
          echo "services=${SERVICES}"       >> "$GITHUB_OUTPUT"

      - name: Generate new baseline
        if: success()
        run: buildgraph generate

      - uses: actions/upload-artifact@v4
        if: success()
        with:
          name: buildgraph-baseline
          path: .buildgraph/baseline.json
          retention-days: 90

  build:
    needs: analyze
    if: needs.analyze.outputs.has_changes == 'true' && needs.analyze.outputs.services != '[]'
    strategy:
      matrix:
        service: ${{ fromJson(needs.analyze.outputs.services) }}
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - run: go build -o bin/${{ matrix.service }} ./services/${{ matrix.service }}/...
      - run: go test ./services/${{ matrix.service }}/...
```

A complete workflow is available at [`.github/workflows/buildgraph.yml`](.github/workflows/buildgraph.yml).

### GitLab CI

A complete pipeline is available at [`.gitlab-ci.yml`](.gitlab-ci.yml). It uses a branch-keyed cache for the baseline and exports `SERVICES_TO_BUILD` as a dotenv artifact for downstream jobs.

## How it works

### Call graph construction

BuildGraph uses [`golang.org/x/tools/go/callgraph/cha`](https://pkg.go.dev/golang.org/x/tools/go/callgraph/cha) (Class Hierarchy Analysis) to build the call graph. CHA is conservative — it never misses an edge — and works without a single `main` entry point, which is essential for a monorepo with multiple independent services.

### Change detection

Each function gets two hashes:

| Hash | What it captures |
|---|---|
| `ast_hash` | SHA-256 of the function's SSA textual representation. Strips comments, normalises whitespace. Moving a function to a different file or reformatting it produces the same hash. |
| `transitive_hash` | Folds in the AST hashes of direct dependencies. Detects indirect changes. |

External dependency changes are detected by hashing the `require` block of `go.mod`.

### Impact propagation

Changes are propagated upward through the reverse index (callee → callers) using iterative BFS. A service is included in `services_to_build` if any function it owns — directly or transitively — is affected.

### Source hash caching

On each run, BuildGraph hashes every loaded `.go` file and stores the results in the baseline. On the next run, functions whose source file hash is unchanged reuse their stored AST hash, skipping SSA serialisation for unmodified code.

## Edge cases

| Scenario | Result |
|---|---|
| Function moved to a different file, logic unchanged | No rebuild — AST hash is file-position-independent |
| Only comments changed | No rebuild — SSA representation strips comments |
| Function renamed | Rebuild — old function is "removed", callers become "new" |
| Interface method changed | Conservative rebuild of all callers (CHA over-approximates) |
| External dependency version bumped in `go.mod` | Rebuild of all services that transitively call into that package |
| No baseline (first run) | All services built |

## Project structure

```
buildgraph/
├── buildgraph.yaml              # Config (created by `buildgraph init`)
├── cmd/
│   └── main.go                  # CLI entry point (cobra + viper)
└── pkg/
    ├── analyzer/
    │   ├── analyzer.go           # Package loading, CHA call graph, hashing
    │   └── gomod.go              # go.mod parsing and require-block hashing
    ├── config/
    │   └── config.go             # Config struct and defaults
    ├── diff/
    │   └── detector.go           # Change detection
    ├── impact/
    │   └── impact.go             # BFS impact propagation, service grouping
    ├── storage/
    │   └── storage.go            # Baseline JSON read/write
    └── types/
        └── types.go              # Shared data structures
```

## License

MIT
