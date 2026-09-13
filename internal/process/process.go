// Package process runs child processes and reports how they ended.
//
// Wrapping exec.Command is the most re-invented wheel in CLI tools,
// and the re-implementations usually get the same three things wrong.
// This package gets them right, once:
//
//  1. A non-zero exit code is a RESULT, not a tool failure. `go test`
//     exits 1 when tests fail — that is the child doing its job. Run
//     returns (exitCode, nil) for any child that ran to completion,
//     and reserves error for "could not start or manage the child".
//  2. Context cancellation must not skip grace. exec.CommandContext
//     kills with SIGKILL by default, which gives a server no chance
//     to finish in-flight requests. Run sends SIGINT instead, waits
//     KillDelay for a graceful exit, and only then escalates to SIGKILL.
//  3. Streams are injected. Child output goes to the io.Writers the
//     caller provides, so tests capture it without pipes or a pty.
package process

import (
	"context"
	"errors"
	"fmt"
	"io"

	"os/exec"
	"syscall"
	"time"
)

// DefaultKillDelay is how long a child gets to exit gracefully after
// the interrupt before Run escalates to SIGKILL.
const DefaultKillDelay = 10 * time.Second

// Options configures one child process run. All fields are optional.
type Options struct {
	Dir       string        // working directory; empty = inherit
	Env       []string      // environment; nil = inherit
	Stdin     io.Reader     // child's stdin; nil = /dev/null
	Stdout    io.Writer     // child's stdout; nil = discarded
	Stderr    io.Writer     // child's stderr; nil = discarded
	KillDelay time.Duration // interrupt→SIGKILL grace period; 0 = DefaultKillDelay
}

// Run starts name(args), streams its output to the writers in opts,
// and waits for it to finish.
//
// The returned int is the child's exit code (POSIX convention: 128+n
// when the child was killed by signal n). The returned error is
// non-nil only when goforge itself failed to run or manage the child:
// "binary not found" is an error, "tests failed (exit 1)" is not.
func Run(ctx context.Context, name string, args []string, opts Options) (int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = opts.Dir
	cmd.Env = opts.Env
	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr

	// Run the child in its own process group so signals can reach the
	// whole tree: `go run` compiles a grandchild binary, and the
	// graceful-shutdown handler lives THERE, not in go run itself.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Upgrade the context cancellation from SIGKILL to a group SIGINT
	// so every descendant gets the chance to shut down gracefully;
	// WaitDelay escalates to SIGKILL if they ignore it.
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return errors.New("process not started")
		}
		// Negative pid = signal the whole process group.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
	}
	cmd.WaitDelay = opts.KillDelay
	if cmd.WaitDelay == 0 {
		cmd.WaitDelay = DefaultKillDelay
	}

	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return 0, fmt.Errorf("%s: command not found (is it installed and in PATH?)", name)
		}
		return 0, fmt.Errorf("start %s: %w", name, err)
	}
	if err := cmd.Wait(); err != nil {
		return exitCode(name, err)
	}
	return 0, nil
}

// exitCode translates the error from cmd.Wait into (code, nil) when
// the child itself determined the outcome, or (0, err) when goforge
// failed around the child.
func exitCode(name string, err error) (int, error) {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if status, ok := ee.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal()), nil
		}
		return ee.ExitCode(), nil
	}
	return 0, fmt.Errorf("run %s: %w", name, err)
}
