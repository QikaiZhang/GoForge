// Package git integrates version control by composing the git CLI.
//
// Shelling out to git (instead of linking a git library) is a
// deliberate choice: git is already installed everywhere goforge
// runs, its output is what the user already knows, and subprocess
// isolation means goforge never breaks when git's internals change.
// The trade-offs are documented in docs/adr/005-git-integration.md.
package git

import (
	"context"
	"fmt"
	"io"

	"goforge/internal/process"
)

// Status runs `git status` in dir and streams its output.
//
// The returned code is git's exit code: 0 for a clean invocation,
// 128 when dir is not inside a repository (git has already printed
// its own diagnostic to stderr). An error means git itself could not
// be run (not installed, not in PATH).
func Status(ctx context.Context, dir string, stdout, stderr io.Writer) (int, error) {
	code, err := process.Run(ctx, "git", []string{"status"}, process.Options{
		Dir:    dir,
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return 0, fmt.Errorf("git status: %w", err)
	}
	return code, nil
}
