package diff

import (
	"github.com/bubunyo/buildgraph/pkg/types"
)

// Detector compares the current state of the codebase against a previous
// baseline and produces a list of semantic Changes.
type Detector struct {
	currentFunctions map[string]*types.Function
	currentExtDeps   map[string]string // pkg path -> version (from go.mod)
	currentExtHash   string            // hash of the require block
	previousBaseline *types.Baseline
}

func NewDetector(
	current map[string]*types.Function,
	extDeps map[string]string,
	extHash string,
	previous *types.Baseline,
) *Detector {
	return &Detector{
		currentFunctions: current,
		currentExtDeps:   extDeps,
		currentExtHash:   extHash,
		previousBaseline: previous,
	}
}

// DetectChanges returns all semantic changes since the previous baseline.
func (d *Detector) DetectChanges() []types.Change {
	// No baseline: every function is "new" – trigger full build
	if d.previousBaseline == nil {
		return d.allAdded()
	}

	var changes []types.Change

	prevNodes := d.previousBaseline.Graph.Nodes
	prevHashes := d.previousBaseline.FunctionHashes

	// 1. New functions
	for name := range d.currentFunctions {
		if _, exists := prevNodes[name]; !exists {
			changes = append(changes, types.Change{
				Function: name,
				Type:     "added",
				Reason:   "new_function",
			})
		}
	}

	// 2. Removed functions
	for name := range prevNodes {
		if _, exists := d.currentFunctions[name]; !exists {
			changes = append(changes, types.Change{
				Function: name,
				Type:     "removed",
				Reason:   "function_deleted",
			})
		}
	}

	// 3. Modified functions (AST body changed)
	for name, curr := range d.currentFunctions {
		prev, hasPrev := prevHashes[name]
		if !hasPrev {
			continue
		}
		if prev.ASTHash != curr.ASTHash {
			changes = append(changes, types.Change{
				Function: name,
				Type:     "modified",
				Reason:   "ast_hash_changed",
				OldHash:  prev.ASTHash,
				NewHash:  curr.ASTHash,
			})
		}
	}

	// 4. External dependency version changes
	//    Fast-path: compare the whole require-block hash first.
	if d.currentExtHash != "" &&
		d.previousBaseline.ExternalDepsHash != "" &&
		d.currentExtHash != d.previousBaseline.ExternalDepsHash {

		changes = append(changes, d.externalDepChanges()...)
	}

	return changes
}

// allAdded marks every current function as "added" (first run).
func (d *Detector) allAdded() []types.Change {
	changes := make([]types.Change, 0, len(d.currentFunctions))
	for name := range d.currentFunctions {
		changes = append(changes, types.Change{
			Function: name,
			Type:     "added",
			Reason:   "new_function",
		})
	}
	return changes
}

// externalDepChanges returns one Change per package whose version differs.
// It emits changes for every function that directly calls the bumped package
// so impact analysis can propagate correctly.
func (d *Detector) externalDepChanges() []types.Change {
	prev := d.previousBaseline.ExternalDeps
	var changes []types.Change

	// Find packages whose version changed
	bumped := make(map[string][2]string) // path -> [oldVer, newVer]

	for path, newVer := range d.currentExtDeps {
		oldVer, existed := prev[path]
		if !existed || oldVer != newVer {
			bumped[path] = [2]string{oldVer, newVer}
		}
	}
	// Also detect removals
	for path, oldVer := range prev {
		if _, exists := d.currentExtDeps[path]; !exists {
			bumped[path] = [2]string{oldVer, ""}
		}
	}

	if len(bumped) == 0 {
		return nil
	}

	// For each function that calls a bumped external package, emit a change
	for funcName, fn := range d.currentFunctions {
		for _, dep := range fn.Deps {
			if dep.Type != "external" {
				continue
			}
			versions, affected := bumped[dep.Package.Path]
			if !affected {
				continue
			}
			changes = append(changes, types.Change{
				Function: funcName,
				Type:     "external_dep_changed",
				Reason:   "external_dependency_version_changed",
				Package:  dep.Package.Path,
				OldVer:   versions[0],
				NewVer:   versions[1],
			})
		}
	}

	return changes
}
