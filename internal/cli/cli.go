// Package cli implements the goforge command line interface.
//
// A CLI, reduced to its essence, is a function from argv to an exit
// code with two side effects: writing normal output to stdout and
// diagnostics to stderr. Run is exactly that function. Keeping the
// signature free of globals is what makes the whole CLI testable.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"goforge"
	"goforge/internal/config"
	"goforge/internal/generator"
	"goforge/internal/git"
	"goforge/internal/process"
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
var version = "1.0.0"

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

// command is one goforge subcommand: a name, a one-line help string,
// and the function that implements it.
//
// The registry below replaced a dispatch switch plus a hand-written
// usage string during the refactor stage. The help text is derived
// from this table, so a newly registered command can no longer be
// forgotten in the help output, and dispatch is one lookup instead of
// a growing switch. A struct with a func field is deliberately chosen
// over a Command interface: there is exactly one implementation kind,
// so an interface would be ceremony, not abstraction.
type command struct {
	name  string
	short string
	run   func(args []string, stdout, stderr io.Writer) int
}

// commands is the full command registry, in help order.
var commands = []command{
	{name: "new", short: "create a new project skeleton", run: runNew},
	{name: "generate", short: "generate handler/service/repository code into the current project", run: runGenerate},
	{name: "dev", short: "run the current project's server (go run ./cmd/server)", run: runDev},
	{name: "test", short: "run the current project's tests (go test ./...)", run: runTest},
	{name: "git", short: "run git status in the current project", run: runGit},
	{name: "version", short: "print the goforge version", run: runVersion},
}

// usageText renders the help from the command registry, so the list of
// commands exists exactly once in this file.
func usageText() string {
	var b strings.Builder
	b.WriteString("goforge is a scaffold for Go backend services.\n\n")
	b.WriteString("Usage:\n  goforge <command> [args]\n\n")
	b.WriteString("Commands:\n")
	for _, c := range commands {
		fmt.Fprintf(&b, "  %-10s %s\n", c.name, c.short)
	}
	return b.String()
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, usageText())
}

// runVersion handles "goforge version" and the "--version" flag.
func runVersion(_ []string, stdout, _ io.Writer) int {
	fmt.Fprintf(stdout, "goforge version %s\n", version)
	return ExitOK
}

// Run executes one goforge invocation and returns the process exit code.
// args is argv without the program name; stdout/stderr are injected so
// tests can capture output without touching the real process streams.
func Run(args []string, stdout, stderr io.Writer) int {
	switch {
	case len(args) == 0:
		printUsage(stdout)
		return ExitOK
	case args[0] == "-h" || args[0] == "--help" || args[0] == "help":
		printUsage(stdout)
		return ExitOK
	case args[0] == "--version":
		return runVersion(nil, stdout, stderr)
	}
	for _, c := range commands {
		if c.name == args[0] {
			return c.run(args[1:], stdout, stderr)
		}
	}
	fmt.Fprintf(stderr, "goforge: unknown command %q\n\n", args[0])
	printUsage(stderr)
	return ExitUsage
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

// runGenerate handles "goforge generate <kind> <name> [--force]".
// The command must be run inside a goforge project; generator.Generate
// verifies that and returns a descriptive error when not.
func runGenerate(args []string, stdout, stderr io.Writer) int {
	var force bool
	var rest []string
	for _, a := range args {
		if a == "--force" {
			force = true
			continue
		}
		rest = append(rest, a)
	}
	if len(rest) != 2 {
		fmt.Fprintln(stderr, "goforge generate: a kind and an entity name are required")
		printGenerateUsage(stderr)
		return ExitUsage
	}

	req := generator.Request{Kind: generator.Kind(rest[0]), Name: rest[1], Force: force}

	// Input problems are usage errors; the generator re-validates them
	// (it is a library function and cannot assume the CLI checked).
	switch {
	case !req.Kind.Valid():
		fmt.Fprintf(stderr, "goforge generate: unknown kind %q (want handler, service or repository)\n", req.Kind)
		printGenerateUsage(stderr)
		return ExitUsage
	case generator.ValidateName(req.Name) != nil:
		fmt.Fprintf(stderr, "goforge generate: %v\n", generator.ValidateName(req.Name))
		printGenerateUsage(stderr)
		return ExitUsage
	}

	// Everything Generate reports from here on is a runtime failure:
	// not inside a project, file exists without --force, disk errors.
	path, err := generator.Generate(".", req, templatesFS)
	if err != nil {
		fmt.Fprintf(stderr, "goforge generate: %v\n", err)
		if errors.Is(err, generator.ErrFileExists) {
			fmt.Fprintln(stderr, "keep the existing file, or re-run with --force to overwrite it")
		}
		return ExitError
	}
	// A broken goforge.yaml does not block generation (this command can
	// run without it), but it must not pass silently — the same broken
	// file aborts `goforge dev`.
	if _, err := config.Load("."); err != nil {
		fmt.Fprintf(stderr, "goforge generate: warning: %v\n", err)
	}
	fmt.Fprintf(stdout, "created %s\n", path)
	return ExitOK
}

// runDev handles "goforge dev [--port N]": run the current project's
// server as a child process, stream its output, and forward Ctrl+C.
func runDev(args []string, stdout, stderr io.Writer) int {
	// Ctrl+C / SIGTERM cancel ctx, which interrupts the child
	// gracefully (SIGINT, then SIGKILL after the grace period —
	// see internal/process).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runDevContext(ctx, args, stdout, stderr)
}

// runDevContext is the testable core of dev: it takes its context as
// a parameter instead of installing signal handlers, so tests can
// cancel it directly.
func runDevContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	port, ok := parseDevPort(args, stderr)
	if !ok {
		return ExitUsage
	}

	// Unlike generate (which only warns), dev is the command whose
	// whole behavior is configured: a broken goforge.yaml is fatal here.
	cfg, err := config.Load(".")
	if err != nil {
		fmt.Fprintf(stderr, "goforge dev: %v\n", err)
		return ExitError
	}
	// Precedence: flag > environment > file > default.
	config.ApplyEnv(&cfg, os.Getenv)
	if port != 0 {
		cfg.Server.Port = port
	}

	// Fail fast with a goforge-shaped message when this is not a
	// project; without the check the user would see go's raw error.
	if _, err := os.Stat("go.mod"); err != nil {
		fmt.Fprintln(stderr, "goforge dev: not inside a goforge project (go.mod not found)")
		return ExitError
	}
	if _, err := os.Stat(filepath.Join("cmd", "server")); err != nil {
		fmt.Fprintln(stderr, "goforge dev: no cmd/server in this project (is it a goforge project?)")
		return ExitError
	}

	fmt.Fprintf(stdout, "starting server on :%d (Ctrl+C to stop)\n", cfg.Server.Port)
	env := append(os.Environ(), "PORT="+strconv.Itoa(cfg.Server.Port))
	code, err := process.Run(ctx, "go", []string{"run", "./cmd/server"}, process.Options{
		Stdin:  os.Stdin,
		Stdout: stdout,
		Stderr: stderr,
		Env:    env,
	})
	if err != nil {
		fmt.Fprintf(stderr, "goforge dev: %v\n", err)
		return ExitError
	}
	if ctx.Err() != nil {
		// goforge requested the stop (Ctrl+C / SIGTERM). The child's
		// exit status does not measure graceful shutdown — `go run`
		// exits 1 or 130 when interrupted, even though the server it
		// hosts shuts down cleanly. A requested stop is a success.
		fmt.Fprintln(stdout, "server stopped")
		return ExitOK
	}
	fmt.Fprintf(stdout, "server exited (code %d)\n", code)
	// Exit-code passthrough: when the child decides to die, its code
	// is the command's result.
	return code
}

// parseDevPort extracts the optional --port flag. Returns 0 when the
// flag is absent (meaning "no override").
func parseDevPort(args []string, stderr io.Writer) (int, bool) {
	raw := ""
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--port":
			i++
			if i >= len(args) {
				fmt.Fprintln(stderr, "goforge dev: --port requires a value")
				return 0, false
			}
			raw = args[i]
		case strings.HasPrefix(args[i], "--port="):
			raw = strings.TrimPrefix(args[i], "--port=")
		default:
			fmt.Fprintf(stderr, "goforge dev: unknown argument %q\n", args[i])
			fmt.Fprintln(stderr)
			fmt.Fprintln(stderr, "usage: goforge dev [--port <n>]")
			return 0, false
		}
	}
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		fmt.Fprintf(stderr, "goforge dev: --port %q is not a number\n", raw)
		return 0, false
	}
	return n, true
}

// runTest handles "goforge test [-- extra args]": run the current
// project's tests via `go test ./...`.
//
// This command is the smallest consumer of the process package, and
// exists mainly to make the exit-code contract visible: failing tests
// exit 1 and goforge reports exactly that — not an error, not a
// message about goforge itself.
func runTest(args []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runTestContext(ctx, args, stdout, stderr)
}

// runTestContext is the testable core of test.
func runTestContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	// Everything before "--" belongs to goforge (and goforge accepts
	// nothing here); everything after "--" goes to `go test` verbatim.
	// This is the standard argv convention for nested CLIs.
	passthrough := []string{}
	rest := args
	for i, a := range rest {
		if a == "--" {
			passthrough = args[i+1:]
			args = args[:i]
			break
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(stderr, "goforge test: unknown argument %q\\n", args[0])
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "usage: goforge test [-- <go test flags>]")
		return ExitUsage
	}

	if _, err := os.Stat("go.mod"); err != nil {
		fmt.Fprintln(stderr, "goforge test: not inside a goforge project (go.mod not found)")
		return ExitError
	}

	goArgs := append([]string{"test", "./..."}, passthrough...)
	code, err := process.Run(ctx, "go", goArgs, process.Options{
		Stdin:  os.Stdin,
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		fmt.Fprintf(stderr, "goforge test: %v\\n", err)
		return ExitError
	}
	return code
}

// runGit handles "goforge git status". The git subcommand is the
// project's demonstration of CLI composition: goforge gains a
// capability by invoking another CLI, not by linking a library.
func runGit(args []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runGitContext(ctx, args, stdout, stderr)
}

// runGitContext is the testable core of git.
func runGitContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || args[0] != "status" {
		if len(args) > 0 {
			fmt.Fprintf(stderr, "goforge git: only 'status' is supported; use the git binary directly for %q\n", args[0])
		} else {
			fmt.Fprintln(stderr, "goforge git: a subcommand is required")
		}
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "usage: goforge git status")
		return ExitUsage
	}

	code, err := git.Status(ctx, ".", stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "goforge git: %v\n", err)
		return ExitError
	}
	if code == 128 {
		// git already printed "fatal: not a git repository..."; add
		// the actionable hint goforge knows and git doesn't.
		fmt.Fprintln(stderr, "this directory is not versioned; run 'git init' first")
		return ExitError
	}
	return code
}

func printGenerateUsage(stderr io.Writer) {
	fmt.Fprintln(stderr)
	fmt.Fprintln(stderr, "usage: goforge generate handler|service|repository <name> [--force]")
}
