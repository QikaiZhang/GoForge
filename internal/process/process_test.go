package process

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunCapturesOutput(t *testing.T) {
	var out, errOut bytes.Buffer
	code, err := Run(context.Background(), "sh", []string{"-c", "echo out; echo err 1>&2"}, Options{
		Stdout: &out,
		Stderr: &errOut,
	})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if got := strings.TrimSpace(out.String()); got != "out" {
		t.Errorf("stdout = %q, want %q", got, "out")
	}
	if got := strings.TrimSpace(errOut.String()); got != "err" {
		t.Errorf("stderr = %q, want %q", got, "err")
	}
}

func TestRunExitCodeIsAResultNotAnError(t *testing.T) {
	// The single most important contract in this package: a failing
	// child is reported through the exit code, err stays nil.
	code, err := Run(context.Background(), "sh", []string{"-c", "exit 3"}, Options{})
	if err != nil {
		t.Fatalf("err = %v, want nil (the child ran fine, it just failed)", err)
	}
	if code != 3 {
		t.Errorf("code = %d, want 3", code)
	}
}

func TestRunSignalDeathMapsTo128PlusN(t *testing.T) {
	code, err := Run(context.Background(), "sh", []string{"-c", "kill -TERM $$"}, Options{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if want := 128 + 15; code != want {
		t.Errorf("code = %d, want %d", code, want)
	}
}

func TestRunMissingBinaryIsAnError(t *testing.T) {
	_, err := Run(context.Background(), "goforge-definitely-not-a-binary", nil, Options{})
	if err == nil {
		t.Fatal("starting a missing binary must fail")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want a not-found message", err)
	}
}

func TestRunContextCancelSendsInterrupt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	// `exec` replaces the shell with sleep, so the child process IS
	// sleep and dies directly from SIGINT. Without exec, sh traps the
	// signal and keeps waiting for the foreground child — signals to a
	// shell do not reach its grandchildren (this is why `go run`
	// installs explicit signal forwarding).
	code, err := Run(ctx, "sh", []string{"-c", "exec sleep 30"}, Options{
		KillDelay: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Run took %s after cancel, want ~immediate", elapsed)
	}
	if want := 128 + 2; code != want { // SIGINT
		t.Errorf("code = %d, want %d", code, want)
	}
}

func TestRunEscalatesToSigkillWhenInterruptIsIgnored(t *testing.T) {
	if testing.Short() {
		t.Skip("timing-sensitive escalation test")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	// The trap makes the child IGNORE SIGINT; only the WaitDelay
	// escalation to SIGKILL can end it.
	code, err := Run(ctx, "sh", []string{"-c", "trap '' INT; sleep 30"}, Options{
		KillDelay: time.Second,
	})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > 6*time.Second {
		t.Errorf("Run took %s, want ~1s (the WaitDelay)", elapsed)
	}
	if want := 128 + 9; code != want { // SIGKILL
		t.Errorf("code = %d, want %d (SIGKILL escalation)", code, want)
	}
}

func TestRunInDir(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	code, err := Run(context.Background(), "sh", []string{"-c", "pwd"}, Options{
		Dir:    dir,
		Stdout: &out,
	})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if got := strings.TrimSpace(out.String()); got != dir {
		t.Errorf("pwd = %q, want %q", got, dir)
	}
}
