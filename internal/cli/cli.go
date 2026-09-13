// Package cli implements the goforge command line interface.
//
// A CLI, reduced to its essence, is a function from argv to an exit
// code with two side effects: writing normal output to stdout and
// diagnostics to stderr. Run is exactly that function. Keeping the
// signature free of globals is what makes the whole CLI testable.
package cli

import (
	"fmt"
	"io"
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
var version = "0.1.0"

const usage = `goforge is a scaffold for Go backend services.

Usage:
  goforge <command> [args]

Commands:
  new        create a new project skeleton (not implemented yet)
  version    print the goforge version
  help       show this help

Use "goforge help" to see this text again.`

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

// runNew handles "goforge new <name>". At this stage it only validates
// its arguments and prints what it would do; real scaffolding arrives
// in the project-init stage.
func runNew(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "goforge new: exactly one project name is required")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "usage: goforge new <name>")
		return ExitUsage
	}
	name := args[0]
	fmt.Fprintf(stdout, "would scaffold project %q (scaffolding not implemented yet)\n", name)
	return ExitOK
}
