package project

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// realTemplates points at the repository's template directory; `go
// test` runs with the package directory as cwd, so the relative path
// is stable. Tests that need synthetic templates use testing/fstest
// instead (see internal/template).
func realTemplates() fs.FS {
	return os.DirFS(filepath.Join("..", "..", "templates"))
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"user-service", false},
		{"a", false},
		{"svc2", false},
		{"user-service-2", false},
		{"", true},
		{"User", true},           // no uppercase
		{"user_service", true},   // no underscore: keep it one strict rule
		{"-user", true},          // must start with a letter
		{"user-", true},          // no trailing dash (derived identifiers get ugly)
		{"us er", true},          // no spaces
		{"用户", true},             // ascii only
		{"user/service", true},   // path separator
		{`user\service`, true},   // windows separator
		{"..", true},             // path traversal
		{".", true},              // current directory
		{"/tmp/elsewhere", true}, // absolute path
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestCreateScaffoldsProject(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "user-service")

	if err := Create(dir, "user-service", realTemplates()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Every scaffolded directory and file must exist.
	for _, d := range scaffoldDirs {
		if fi, err := os.Stat(filepath.Join(dir, d)); err != nil {
			t.Errorf("directory %s missing: %v", d, err)
		} else if !fi.IsDir() {
			t.Errorf("%s is not a directory", d)
		}
	}
	for _, f := range scaffoldFiles {
		if _, err := os.Stat(filepath.Join(dir, f.path)); err != nil {
			t.Errorf("file %s missing: %v", f.path, err)
		}
	}

	goMod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if !strings.Contains(string(goMod), "module user-service") {
		t.Errorf("go.mod = %q, want it to declare the module name", goMod)
	}

	mainGo, err := os.ReadFile(filepath.Join(dir, "cmd", "server", "main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	for _, want := range []string{"package main", "registerRoutes", "healthz"} {
		if !strings.Contains(string(mainGo), want) {
			t.Errorf("main.go = %q, want it to contain %q", mainGo, want)
		}
	}
}

func TestCreateRefusesExistingDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "user-service")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	err := Create(dir, "user-service", realTemplates())
	if !errors.Is(err, ErrDirExists) {
		t.Errorf("Create error = %v, want ErrDirExists", err)
	}
}

func TestCreateReportsPermissionErrors(t *testing.T) {
	root := t.TempDir()
	cage := filepath.Join(root, "cage")
	if err := os.MkdirAll(cage, 0o555); err != nil { // read-only parent
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cage, 0o755) }) // let TempDir cleanup delete it

	err := Create(filepath.Join(cage, "user-service"), "user-service", realTemplates())
	if err == nil {
		t.Fatal("Create in read-only parent should fail")
	}
	if errors.Is(err, ErrDirExists) {
		t.Errorf("error = %v, want a filesystem error, not ErrDirExists", err)
	}
}

// TestCreatedProjectCompiles is the safety net that makes every later
// stage brave: the code we generate must always be valid Go. It shells
// out to `go build`, so it is skipped in -short mode.
func TestCreatedProjectCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: builds the generated project")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}

	dir := filepath.Join(t.TempDir(), "user-service")
	if err := Create(dir, "user-service", realTemplates()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated project does not build: %v\n%s", err, out)
	}
}
