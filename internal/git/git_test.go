package git

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initRepo creates a real git repository (git is a hard assumption of
// this package, same as `go` for the process tests).
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func TestStatusInsideRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: shells out to git")
	}
	dir := initRepo(t)

	var out, errOut bytes.Buffer
	code, err := Status(context.Background(), dir, &out, &errOut)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if code != 0 {
		t.Errorf("code = %d, want 0\n%s%s", code, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	// `git status` on a fresh repo reports the current branch.
	if !strings.Contains(combined, "On branch") {
		t.Errorf("output = %q, want git's own status output", combined)
	}
}

func TestStatusOutsideRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: shells out to git")
	}
	dir := t.TempDir() // no git init here

	var out, errOut bytes.Buffer
	code, err := Status(context.Background(), dir, &out, &errOut)
	if err != nil {
		t.Fatalf("Status error = %v, want nil (git ran, it just failed)", err)
	}
	if code != 128 {
		t.Errorf("code = %d, want 128 (git's not-a-repository code)", code)
	}
	if !strings.Contains(errOut.String(), "not a git repository") {
		t.Errorf("stderr = %q, want git's diagnostic", errOut.String())
	}
}

func TestStatusGitMissing(t *testing.T) {
	// An empty PATH makes exec fail to find git: that is goforge's
	// error to report, distinct from git's own exit codes.
	t.Setenv("PATH", "")
	dir := filepath.Join(t.TempDir(), "anywhere")

	_, err := Status(context.Background(), dir, nil, nil)
	if err == nil {
		t.Fatal("Status without git in PATH should fail")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want a not-found message", err)
	}
}
