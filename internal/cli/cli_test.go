package cli

import (
	"bytes"
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
			name:     "new with a name announces the plan",
			args:     []string{"new", "user-service"},
			wantCode: ExitOK,
			wantOut:  []string{`"user-service"`},
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
