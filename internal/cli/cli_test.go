package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// writeTemplatesFixture creates a minimal templates/ tree in dir. At
// this stage goforge reads templates relative to its working
// directory, so the on-disk CLI tests must provide one.
func writeTemplatesFixture(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		"templates/project/go.mod.tmpl":      "module {{.Name}}",
		"templates/project/main.go.tmpl":     "package main // {{.Name}}",
		"templates/project/model.go.tmpl":    "package model",
		"templates/project/config.yaml.tmpl": "server: {}",
		"templates/project/gitignore.tmpl":   "bin/",
		"templates/project/README.md.tmpl":   "# {{.Name}}",
	}
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNewCreatesProjectOnDisk(t *testing.T) {
	// goforge new writes into the working directory, so run it inside a
	// temp dir instead of the package directory. t.Chdir restores the
	// original directory automatically (Go 1.24+).
	tmp := t.TempDir()
	writeTemplatesFixture(t, tmp)
	t.Chdir(tmp)

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
	tmp := t.TempDir()
	writeTemplatesFixture(t, tmp)
	t.Chdir(tmp)
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
