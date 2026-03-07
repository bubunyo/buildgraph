package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGoMod(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestParseGoMod_ValidBlockRequire(t *testing.T) {
	path := writeGoMod(t, `module github.com/example/repo

go 1.24

require (
	github.com/foo/bar v1.2.3
	github.com/baz/qux v0.0.1 // indirect
)
`)
	gm, err := ParseGoMod(path)
	if err != nil {
		t.Fatalf("ParseGoMod: %v", err)
	}
	if gm.Module != "github.com/example/repo" {
		t.Errorf("Module: got %q", gm.Module)
	}
	if gm.GoVer != "1.24" {
		t.Errorf("GoVer: got %q", gm.GoVer)
	}
	if gm.Require["github.com/foo/bar"] != "v1.2.3" {
		t.Errorf("foo/bar version: got %q", gm.Require["github.com/foo/bar"])
	}
	if gm.Require["github.com/baz/qux"] != "v0.0.1" {
		t.Errorf("baz/qux version: got %q", gm.Require["github.com/baz/qux"])
	}
}

func TestParseGoMod_SingleLineRequire(t *testing.T) {
	path := writeGoMod(t, `module github.com/example/repo

go 1.21

require github.com/spf13/cobra v1.10.2
`)
	gm, err := ParseGoMod(path)
	if err != nil {
		t.Fatalf("ParseGoMod: %v", err)
	}
	if gm.Require["github.com/spf13/cobra"] != "v1.10.2" {
		t.Errorf("cobra version: got %q", gm.Require["github.com/spf13/cobra"])
	}
}

func TestParseGoMod_SkipsBlankLinesAndComments(t *testing.T) {
	path := writeGoMod(t, `// top comment
module github.com/example/repo

// another comment
go 1.22

require (
	// inline comment
	github.com/foo/bar v1.0.0
)
`)
	gm, err := ParseGoMod(path)
	if err != nil {
		t.Fatalf("ParseGoMod: %v", err)
	}
	if gm.Module != "github.com/example/repo" {
		t.Errorf("Module: got %q", gm.Module)
	}
	if gm.Require["github.com/foo/bar"] != "v1.0.0" {
		t.Errorf("foo/bar: got %q", gm.Require["github.com/foo/bar"])
	}
}

func TestParseGoMod_EmptyRequire(t *testing.T) {
	path := writeGoMod(t, `module github.com/example/repo

go 1.21
`)
	gm, err := ParseGoMod(path)
	if err != nil {
		t.Fatalf("ParseGoMod: %v", err)
	}
	if len(gm.Require) != 0 {
		t.Errorf("expected empty Require, got %v", gm.Require)
	}
}

func TestParseGoMod_MissingFile_ReturnsError(t *testing.T) {
	_, err := ParseGoMod("/nonexistent/go.mod")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestParseGoMod_MultipleRequireBlocks(t *testing.T) {
	path := writeGoMod(t, `module github.com/example/repo

go 1.21

require (
	github.com/foo/bar v1.0.0
)

require (
	github.com/baz/qux v2.0.0 // indirect
)
`)
	gm, err := ParseGoMod(path)
	if err != nil {
		t.Fatalf("ParseGoMod: %v", err)
	}
	if gm.Require["github.com/foo/bar"] != "v1.0.0" {
		t.Errorf("foo/bar: got %q", gm.Require["github.com/foo/bar"])
	}
	if gm.Require["github.com/baz/qux"] != "v2.0.0" {
		t.Errorf("baz/qux: got %q", gm.Require["github.com/baz/qux"])
	}
}

func TestHashGoMod_Deterministic(t *testing.T) {
	gm := &GoMod{
		Require: map[string]string{
			"github.com/foo/bar": "v1.2.3",
			"github.com/baz/qux": "v0.0.1",
		},
	}
	h1 := HashGoMod(gm)
	h2 := HashGoMod(gm)
	if h1 != h2 {
		t.Errorf("HashGoMod not deterministic: %q vs %q", h1, h2)
	}
}

func TestHashGoMod_DifferentVersionProducesDifferentHash(t *testing.T) {
	gm1 := &GoMod{Require: map[string]string{"github.com/foo/bar": "v1.0.0"}}
	gm2 := &GoMod{Require: map[string]string{"github.com/foo/bar": "v2.0.0"}}
	if HashGoMod(gm1) == HashGoMod(gm2) {
		t.Error("expected different hashes for different versions")
	}
}

func TestHashGoMod_OrderIndependent(t *testing.T) {
	// Same deps in different map iteration orders should yield the same hash.
	gm1 := &GoMod{Require: map[string]string{
		"github.com/aaa/aaa": "v1.0.0",
		"github.com/zzz/zzz": "v2.0.0",
	}}
	gm2 := &GoMod{Require: map[string]string{
		"github.com/zzz/zzz": "v2.0.0",
		"github.com/aaa/aaa": "v1.0.0",
	}}
	if HashGoMod(gm1) != HashGoMod(gm2) {
		t.Error("expected same hash regardless of map iteration order")
	}
}

func TestHashGoMod_EmptyRequire(t *testing.T) {
	gm := &GoMod{Require: map[string]string{}}
	h := HashGoMod(gm)
	if h == "" {
		t.Error("expected non-empty hash for empty require")
	}
}
