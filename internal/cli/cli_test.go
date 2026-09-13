package cli

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// runCLI is a helper that runs one invocation and captures both streams.
func runCLI(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = Run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  []string // substrings required in stdout
		wantErr  []string // substrings required in stderr
	}{
		{
			name:     "no args prints usage to stdout",
			args:     nil,
			wantCode: ExitOK,
			wantOut:  []string{"Usage:", "new", "version"},
		},
		{
			name:     "help command",
			args:     []string{"help"},
			wantCode: ExitOK,
			wantOut:  []string{"Usage:"},
		},
		{
			name:     "-h flag",
			args:     []string{"-h"},
			wantCode: ExitOK,
			wantOut:  []string{"Usage:"},
		},
		{
			name:     "--help flag",
			args:     []string{"--help"},
			wantCode: ExitOK,
			wantOut:  []string{"Usage:"},
		},
		{
			name:     "version command",
			args:     []string{"version"},
			wantCode: ExitOK,
			wantOut:  []string{"goforge version " + version},
		},
		{
			name:     "--version flag",
			args:     []string{"--version"},
			wantCode: ExitOK,
			wantOut:  []string{"goforge version " + version},
		},
		{
			name:     "new without a name is a usage error",
			args:     []string{"new"},
			wantCode: ExitUsage,
			wantErr:  []string{"exactly one project name"},
		},
		{
			name:     "new with too many names is a usage error",
			args:     []string{"new", "a", "b"},
			wantCode: ExitUsage,
			wantErr:  []string{"exactly one project name"},
		},
		{
			name:     "invalid name is a usage error",
			args:     []string{"new", "User"},
			wantCode: ExitUsage,
			wantErr:  []string{"lowercase"},
		},
		{
			name:     "unknown command is a usage error on stderr",
			args:     []string{"frobnicate"},
			wantCode: ExitUsage,
			wantErr:  []string{`unknown command "frobnicate"`, "Usage:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := runCLI(tt.args...)
			if code != tt.wantCode {
				t.Errorf("Run(%v) exit code = %d, want %d", tt.args, code, tt.wantCode)
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(out, want) {
					t.Errorf("Run(%v) stdout = %q, want it to contain %q", tt.args, out, want)
				}
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(errOut, want) {
					t.Errorf("Run(%v) stderr = %q, want it to contain %q", tt.args, errOut, want)
				}
			}
		})
	}
}

func TestStreamsAreSeparated(t *testing.T) {
	// Errors must go to stderr and normal output to stdout; a CLI that
	// mixes them breaks shell pipelines for every user.
	code, out, errOut := runCLI("frobnicate")
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, ExitUsage)
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty (errors belong on stderr)", out)
	}
	if !strings.Contains(errOut, "unknown command") {
		t.Errorf("stderr = %q, want it to contain the error", errOut)
	}
}

// The tests below exercise "goforge new" against the real embedded
// templates. They run inside a temp working directory (t.Chdir
// restores the original directory automatically) because new writes
// relative to the cwd.

func TestNewCreatesProjectOnDisk(t *testing.T) {
	t.Chdir(t.TempDir())

	code, out, errOut := runCLI("new", "user-service")
	if code != ExitOK {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut)
	}
	if !strings.Contains(out, "next steps") {
		t.Errorf("stdout = %q, want next-step hints", out)
	}
	for _, want := range []string{
		filepath.Join("user-service", "go.mod"),
		filepath.Join("user-service", "cmd", "server", "main.go"),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Errorf("%s missing after goforge new: %v", want, err)
		}
	}
}

func TestNewRefusesExistingProject(t *testing.T) {
	t.Chdir(t.TempDir())
	if code, _, _ := runCLI("new", "user-service"); code != ExitOK {
		t.Fatalf("first goforge new failed")
	}

	code, _, errOut := runCLI("new", "user-service")
	if code != ExitError {
		t.Errorf("second goforge new exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut, "already exists") {
		t.Errorf("stderr = %q, want an exists message", errOut)
	}
}

// scaffoldHere scaffolds a project into a temp cwd and cds into it —
// the environment "goforge generate" expects.
func scaffoldHere(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	if code, _, errOut := runCLI("new", "user-service"); code != ExitOK {
		t.Fatalf("goforge new failed: %s", errOut)
	}
	if err := os.Chdir("user-service"); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateCreatesLayerFile(t *testing.T) {
	scaffoldHere(t)

	code, out, errOut := runCLI("generate", "service", "user")
	if code != ExitOK {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut)
	}
	if !strings.Contains(out, filepath.Join("internal", "service", "user.go")) {
		t.Errorf("stdout = %q, want it to name the created file", out)
	}
	content, err := os.ReadFile(filepath.Join("internal", "service", "user.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "type UserService struct") {
		t.Errorf("generated service does not contain the service type:\n%s", content)
	}
}

func TestGenerateErrorsAndForce(t *testing.T) {
	scaffoldHere(t)

	if code, _, _ := runCLI("generate", "controller", "user"); code != ExitUsage {
		t.Errorf("unknown kind: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI("generate", "handler"); code != ExitUsage {
		t.Errorf("missing name: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI("generate", "handler", "user-profile"); code != ExitUsage {
		t.Errorf("invalid name: exit = %d, want %d", code, ExitUsage)
	}

	if code, _, _ := runCLI("generate", "handler", "user"); code != ExitOK {
		t.Fatalf("first generate failed")
	}
	code, _, errOut := runCLI("generate", "handler", "user")
	if code != ExitError || !strings.Contains(errOut, "--force") {
		t.Errorf("duplicate generate: exit = %d, stderr = %q, want exit %d and a --force hint",
			code, errOut, ExitError)
	}
	if code, _, _ := runCLI("generate", "handler", "user", "--force"); code != ExitOK {
		t.Errorf("generate --force: exit != 0")
	}
}

func TestGenerateOutsideProject(t *testing.T) {
	t.Chdir(t.TempDir()) // no go.mod here

	code, _, errOut := runCLI("generate", "handler", "user")
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut, "go.mod") {
		t.Errorf("stderr = %q, want it to name the missing go.mod", errOut)
	}
}

func TestDevFlagAndProjectChecks(t *testing.T) {
	if code, _, _ := runCLI("dev", "--port"); code != ExitUsage {
		t.Errorf("--port without value: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI("dev", "--port=abc"); code != ExitUsage {
		t.Errorf("--port=abc: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI("dev", "--wat"); code != ExitUsage {
		t.Errorf("unknown flag: exit = %d, want %d", code, ExitUsage)
	}

	// Outside a project: the go.mod check fires before anything else.
	t.Chdir(t.TempDir())
	code, _, errOut := runCLI("dev")
	if code != ExitError {
		t.Errorf("dev outside project: exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut, "go.mod") {
		t.Errorf("stderr = %q, want it to name the missing go.mod", errOut)
	}
}

func TestDevBrokenConfigIsFatal(t *testing.T) {
	// The counterpart of generate's warning: dev's whole behavior is
	// configured, so a broken goforge.yaml must stop it.
	scaffoldHere(t)
	if err := os.WriteFile("goforge.yaml", []byte("server:\n  port: bogus\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runCLI("dev")
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut, "goforge.yaml") {
		t.Errorf("stderr = %q, want it to name the config file", errOut)
	}
}

// TestDevRunsServerGracefully is the crown integration test: dev must
// boot the generated server, the server must answer /healthz, and
// cancelling the context must shut everything down with exit code 0
// (the generated server handles SIGINT with a graceful shutdown).
func TestDevRunsServerGracefully(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: compiles and runs the generated server")
	}
	scaffoldHere(t)
	if code, _, errOut := runCLI("generate", "handler", "user"); code != ExitOK {
		t.Fatalf("generate handler failed: %s", errOut)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type devResult struct {
		code int
	}
	done := make(chan devResult, 1)
	out := &syncWriter{buf: new(bytes.Buffer)}
	go func() {
		// runDevContext instead of runDev: the test supplies the
		// context instead of installing real signal handlers.
		code := runDevContext(ctx, []string{"--port", "18123"}, out, out)
		done <- devResult{code: code}
	}()

	// Wait for the server to come up (go run compiles first).
	healthy := false
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://127.0.0.1:18123/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				healthy = true
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !healthy {
		cancel()
		t.Fatalf("server never became healthy; dev output:\n%s", out.String())
	}

	// Simulate Ctrl+C.
	cancel()

	select {
	case r := <-done:
		if r.code != ExitOK {
			t.Errorf("dev exit code after Ctrl+C = %d, want 0\ndev output:\n%s", r.code, out.String())
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("dev did not exit after cancel; output:\n%s", out.String())
	}
	// The generated server logs this line from its SIGINT handler, so
	// its presence proves the shutdown was graceful, not a SIGKILL.
	// The log races with go run's exit, so poll briefly.
	deadline = time.Now().Add(10 * time.Second)
	for !strings.Contains(out.String(), "shutting down") {
		if time.Now().After(deadline) {
			t.Fatalf("output does not show a graceful shutdown:\n%s", out.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// syncWriter is safe for the two output streams a child writes through
// (plain bytes.Buffer is not).
type syncWriter struct {
	mu  sync.Mutex
	buf *bytes.Buffer
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *syncWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}
