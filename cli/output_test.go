package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/bubunyo/buildgraph/pkg/types"
)

func baseResult(hasChanges bool, changes []types.Change, services []string) *types.Result {
	return &types.Result{
		Timestamp:     time.Now(),
		CurrentCommit: "abc123",
		HasChanges:    hasChanges,
		Changes:       changes,
		Impact: types.Impact{
			ServicesToBuild: services,
		},
	}
}

func TestFormatText_NoChanges(t *testing.T) {
	result := baseResult(false, nil, nil)
	out := formatText(result)

	if !strings.Contains(out, "Has changes : false") {
		t.Errorf("expected 'Has changes : false' in output:\n%s", out)
	}
	if !strings.Contains(out, "Services to build (0)") {
		t.Errorf("expected 'Services to build (0)' in output:\n%s", out)
	}
}

func TestFormatText_WithChanges(t *testing.T) {
	changes := []types.Change{
		{Function: "core.Foo", Type: "modified", Reason: "ast_hash_changed"},
	}
	result := baseResult(true, changes, []string{"service-a"})
	out := formatText(result)

	if !strings.Contains(out, "Has changes : true") {
		t.Errorf("expected 'Has changes : true':\n%s", out)
	}
	if !strings.Contains(out, "Changes (1)") {
		t.Errorf("expected 'Changes (1)':\n%s", out)
	}
	if !strings.Contains(out, "core.Foo") {
		t.Errorf("expected function name in output:\n%s", out)
	}
	if !strings.Contains(out, "ast_hash_changed") {
		t.Errorf("expected reason in output:\n%s", out)
	}
	if !strings.Contains(out, "Services to build (1)") {
		t.Errorf("expected 'Services to build (1)':\n%s", out)
	}
	if !strings.Contains(out, "service-a") {
		t.Errorf("expected service-a in output:\n%s", out)
	}
}

func TestFormatText_ExternalDepChange(t *testing.T) {
	changes := []types.Change{
		{
			Function: "core.Foo",
			Type:     "external_dep_changed",
			Reason:   "external_dependency_version_changed",
			Package:  "github.com/pkg/foo",
			OldVer:   "v1.0.0",
			NewVer:   "v2.0.0",
		},
	}
	result := baseResult(true, changes, []string{"service-a"})
	out := formatText(result)

	if !strings.Contains(out, "github.com/pkg/foo") {
		t.Errorf("expected package name in output:\n%s", out)
	}
	if !strings.Contains(out, "v1.0.0") || !strings.Contains(out, "v2.0.0") {
		t.Errorf("expected version info in output:\n%s", out)
	}
}

func TestFormatText_ServicesSorted(t *testing.T) {
	result := baseResult(true, nil, []string{"svc-c", "svc-a", "svc-b"})
	out := formatText(result)

	idxA := strings.Index(out, "svc-a")
	idxB := strings.Index(out, "svc-b")
	idxC := strings.Index(out, "svc-c")

	if idxA < 0 || idxB < 0 || idxC < 0 {
		t.Fatalf("expected all services in output:\n%s", out)
	}
	if !(idxA < idxB && idxB < idxC) {
		t.Errorf("expected services sorted a < b < c in output:\n%s", out)
	}
}

func TestCountFiles(t *testing.T) {
	fns := map[string]*types.Function{
		"pkg.Foo": {File: "core/foo.go"},
		"pkg.Bar": {File: "core/foo.go"}, // same file
		"pkg.Baz": {File: "core/bar.go"},
	}
	if got := countFiles(fns); got != 2 {
		t.Errorf("expected 2 unique files, got %d", got)
	}
}
