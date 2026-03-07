package analyzer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeGoMod(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "go.mod")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
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
	require.NoError(t, err)
	assert.Equal(t, "github.com/example/repo", gm.Module)
	assert.Equal(t, "1.24", gm.GoVer)
	assert.Equal(t, "v1.2.3", gm.Require["github.com/foo/bar"])
	assert.Equal(t, "v0.0.1", gm.Require["github.com/baz/qux"])
}

func TestParseGoMod_SingleLineRequire(t *testing.T) {
	path := writeGoMod(t, `module github.com/example/repo

go 1.21

require github.com/spf13/cobra v1.10.2
`)
	gm, err := ParseGoMod(path)
	require.NoError(t, err)
	assert.Equal(t, "v1.10.2", gm.Require["github.com/spf13/cobra"])
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
	require.NoError(t, err)
	assert.Equal(t, "github.com/example/repo", gm.Module)
	assert.Equal(t, "v1.0.0", gm.Require["github.com/foo/bar"])
}

func TestParseGoMod_EmptyRequire(t *testing.T) {
	path := writeGoMod(t, `module github.com/example/repo

go 1.21
`)
	gm, err := ParseGoMod(path)
	require.NoError(t, err)
	assert.Empty(t, gm.Require)
}

func TestParseGoMod_MissingFile_ReturnsError(t *testing.T) {
	_, err := ParseGoMod("/nonexistent/go.mod")
	assert.Error(t, err)
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
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", gm.Require["github.com/foo/bar"])
	assert.Equal(t, "v2.0.0", gm.Require["github.com/baz/qux"])
}

func TestHashGoMod_Deterministic(t *testing.T) {
	gm := &GoMod{
		Require: map[string]string{
			"github.com/foo/bar": "v1.2.3",
			"github.com/baz/qux": "v0.0.1",
		},
	}
	assert.Equal(t, HashGoMod(gm), HashGoMod(gm))
}

func TestHashGoMod_DifferentVersionProducesDifferentHash(t *testing.T) {
	gm1 := &GoMod{Require: map[string]string{"github.com/foo/bar": "v1.0.0"}}
	gm2 := &GoMod{Require: map[string]string{"github.com/foo/bar": "v2.0.0"}}
	assert.NotEqual(t, HashGoMod(gm1), HashGoMod(gm2))
}

func TestHashGoMod_OrderIndependent(t *testing.T) {
	gm1 := &GoMod{Require: map[string]string{
		"github.com/aaa/aaa": "v1.0.0",
		"github.com/zzz/zzz": "v2.0.0",
	}}
	gm2 := &GoMod{Require: map[string]string{
		"github.com/zzz/zzz": "v2.0.0",
		"github.com/aaa/aaa": "v1.0.0",
	}}
	assert.Equal(t, HashGoMod(gm1), HashGoMod(gm2))
}

func TestHashGoMod_EmptyRequire(t *testing.T) {
	assert.NotEmpty(t, HashGoMod(&GoMod{Require: map[string]string{}}))
}
