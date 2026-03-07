package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a starter buildgraph.yaml in the current directory",
	Long:  `Writes a buildgraph.yaml with commented defaults. Does not overwrite an existing file.`,
	RunE:  runInit,
}

func runInit(_ *cobra.Command, _ []string) error {
	const target = "buildgraph.yaml"

	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("%s already exists — remove it first if you want to reinitialise", target)
	}

	const content = `# BuildGraph configuration
# https://github.com/bubunyo/buildgraph

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
`
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", target, err)
	}
	fmt.Printf("Created %s\n", target)
	fmt.Println("Add .buildgraph/ to your .gitignore.")
	return nil
}
