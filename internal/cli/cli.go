// Package cli implements the goforge command line interface.
//
// A CLI, reduced to its essence, is a function from argv to an exit
// code with two side effects: writing normal output to stdout and
// diagnostics to stderr. Run is exactly that function. Keeping the
// signature free of globals is what makes the whole CLI testable.
package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"

	"goforge"
	"goforge/internal/project"
)

// Exit codes goforge terminates with. Keeping them named in one place
// makes the contract explicit and greppable.
const (
	ExitOK    = 0 // command succeeded
	ExitError = 1 // the command ran and failed (runtime failure)
	ExitUsage = 2 // the command line itself was wrong (bad args)
)

// version is overridden at build time with:
//
//	go build -ldflags "-X goforge/internal/cli.version=v1.2.3"
var version = "0.3.0"

const usage = `goforge is a scaffold for Go backend services.

Usage:
  goforge <command> [args]

Commands:
  new        create a new project skeleton
  version    print the goforge version
  help       show this help

Use "goforge help" to see this text again.`

// templatesFS is the embedded template tree rooted at templates/, so
// template names stay layout-independent ("project/go.mod.tmpl").
//
// embed.FS keeps the root directory name in every path
// ("templates/project/go.mod.tmpl"); fs.Sub strips it. The //go:embed
// directive guarantees the directory exists, so the error here can
// only be a programming mistake — hence the panic.
var templatesFS = func() fs.FS {
	sub, err := fs.Sub(goforge.Templates, "templates")
	if err != nil {
		panic(fmt.Sprintf("embedded templates missing: %v", err))
	}
	return sub
}()

// Run executes one goforge invocation and returns the process exit code.
// args is argv without the program name; stdout/stderr are injected so
// tests can capture output without touching the real process streams.
func Run(args []string, stdout, stderr io.Writer) int {
	switch {
	case len(args) == 0:
		fmt.Fprint(stdout, usage)
		return ExitOK
	case args[0] == "-h" || args[0] == "--help" || args[0] == "help":
		fmt.Fprint(stdout, usage)
		return ExitOK
	case args[0] == "--version" || args[0] == "version":
		fmt.Fprintf(stdout, "goforge version %s\n", version)
		return ExitOK
	case args[0] == "new":
		return runNew(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "goforge: unknown command %q\n\n", args[0])
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
}

// runNew handles "goforge new <name>": validate the name, scaffold the
// project into ./<name>, and print next steps.
//
// Exit code policy: a bad name is a usage error (the user typed
// something invalid), a filesystem failure is a runtime error (the
// command was fine, the world wasn't).
func runNew(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "goforge new: exactly one project name is required")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "usage: goforge new <name>")
		return ExitUsage
	}
	name := args[0]

	if err := project.ValidateName(name); err != nil {
		fmt.Fprintf(stderr, "goforge new: %v\n", err)
		return ExitUsage
	}
	// Templates are embedded in the binary, so goforge works from any
	// directory without a source checkout next to it.
	if err := project.Create(name, name, templatesFS); err != nil {
		if errors.Is(err, project.ErrDirExists) {
			fmt.Fprintf(stderr, "goforge new: %v\n", err)
			fmt.Fprintln(stderr, "pick another name or remove the existing directory")
		} else {
			fmt.Fprintf(stderr, "goforge new: %v\n", err)
		}
		return ExitError
	}

	fmt.Fprintf(stdout, "created project %q\n\n", name)
	fmt.Fprintln(stdout, "next steps:")
	fmt.Fprintf(stdout, "  cd %s\n", name)
	fmt.Fprintln(stdout, "  go run ./cmd/server")
	return ExitOK
}
