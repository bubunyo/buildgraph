package analyzer

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"slices"
	"strings"
)

// GoMod holds the parsed content of a go.mod file.
type GoMod struct {
	Module  string
	GoVer   string
	Require map[string]string // import path -> version
}

// ParseGoMod parses a go.mod file and returns structured dependency info.
func ParseGoMod(goModPath string) (*GoMod, error) {
	f, err := os.Open(goModPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gomod := &GoMod{
		Require: make(map[string]string),
	}

	scanner := bufio.NewScanner(f)
	inRequire := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines and comments
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		switch {
		case strings.HasPrefix(line, "module "):
			gomod.Module = strings.TrimSpace(strings.TrimPrefix(line, "module "))

		case strings.HasPrefix(line, "go "):
			gomod.GoVer = strings.TrimSpace(strings.TrimPrefix(line, "go "))

		case line == "require (":
			inRequire = true

		case line == ")":
			inRequire = false

		case inRequire:
			// e.g. github.com/foo/bar v1.2.3
			// or   github.com/foo/bar v1.2.3 // indirect
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				path := parts[0]
				ver := parts[1]
				gomod.Require[path] = ver
			}

		case strings.HasPrefix(line, "require "):
			// single-line require: require github.com/foo/bar v1.2.3
			rest := strings.TrimPrefix(line, "require ")
			parts := strings.Fields(rest)
			if len(parts) >= 2 {
				gomod.Require[parts[0]] = parts[1]
			}
		}
	}

	return gomod, scanner.Err()
}

// HashGoMod returns a sha256 of all require entries, sorted, so any version
// bump changes the hash.
func HashGoMod(gomod *GoMod) string {
	// Collect sorted entries
	entries := make([]string, 0, len(gomod.Require))
	for path, ver := range gomod.Require {
		entries = append(entries, path+"@"+ver)
	}

	// Simple deterministic sort
	slices.Sort(entries)

	combined := strings.Join(entries, "\n")
	h := sha256.Sum256([]byte(combined))
	return fmt.Sprintf("sha256:%x", h)
}
