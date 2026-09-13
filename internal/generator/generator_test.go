package generator

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"goforge/internal/project"
)

// realTemplates serves the repository's real templates; `go test`
// runs with the package directory as cwd, so the relative path is
// stable (same convention as internal/project).
func realTemplates() fs.FS {
	return os.DirFS(filepath.Join("..", "..", "templates"))
}

// scaffoldProject creates a throwaway goforge project to generate into.
func scaffoldProject(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "user-service")
	if err := project.Create(dir, "user-service", realTemplates()); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestValidateEntityName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"user", false},
		{"user_profile", false},
		{"profile2", false},
		{"", true},
		{"User", true},
		{"user-profile", true},
		{"2user", true},
		{"user__profile", true},
		{"user_", true},
		{"_user", true},
		{"us er", true},
		{"用户", true},
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

func TestNaming(t *testing.T) {
	tests := []struct{ in, pascal, camel string }{
		{"user", "User", "user"},
		{"user_profile", "UserProfile", "userProfile"},
		{"a1_b2", "A1B2", "a1B2"},
	}
	for _, tt := range tests {
		if got := Pascal(tt.in); got != tt.pascal {
			t.Errorf("Pascal(%q) = %q, want %q", tt.in, got, tt.pascal)
		}
		if got := Camel(tt.in); got != tt.camel {
			t.Errorf("Camel(%q) = %q, want %q", tt.in, got, tt.camel)
		}
	}
}

func TestGenerateHandler(t *testing.T) {
	dir := scaffoldProject(t)

	path, err := Generate(dir, Request{Kind: KindHandler, Name: "user"}, realTemplates())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := filepath.Join(dir, "internal", "handler", "user.go"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"package handler",
		"type UserHandler struct",
		"func NewUserHandler(",
		`user-service/internal/model`,
	} {
		if !strings.Contains(string(content), want) {
			t.Errorf("generated file does not contain %q:\n%s", want, content)
		}
	}
}

func TestGenerateAllKinds(t *testing.T) {
	dir := scaffoldProject(t)
	for _, kind := range []Kind{KindHandler, KindService, KindRepository} {
		if _, err := Generate(dir, Request{Kind: kind, Name: "user"}, realTemplates()); err != nil {
			t.Errorf("Generate %s: %v", kind, err)
		}
	}
	for _, p := range []string{
		filepath.Join("internal", "handler", "user.go"),
		filepath.Join("internal", "service", "user.go"),
		filepath.Join("internal", "repository", "user.go"),
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("%s missing: %v", p, err)
		}
	}
}

func TestGenerateRefusesExistingFileWithoutForce(t *testing.T) {
	dir := scaffoldProject(t)
	if _, err := Generate(dir, Request{Kind: KindService, Name: "user"}, realTemplates()); err != nil {
		t.Fatal(err)
	}

	_, err := Generate(dir, Request{Kind: KindService, Name: "user"}, realTemplates())
	if !errors.Is(err, ErrFileExists) {
		t.Errorf("error = %v, want ErrFileExists", err)
	}

	// --force overwrites.
	if _, err := Generate(dir, Request{Kind: KindService, Name: "user", Force: true}, realTemplates()); err != nil {
		t.Errorf("Generate with Force: %v", err)
	}
}

func TestGenerateOutsideProject(t *testing.T) {
	dir := t.TempDir() // no go.mod here
	_, err := Generate(dir, Request{Kind: KindHandler, Name: "user"}, realTemplates())
	if err == nil || !strings.Contains(err.Error(), "go.mod") {
		t.Errorf("error = %v, want a message naming the missing go.mod", err)
	}
}

func TestGenerateUnknownKindAndBadName(t *testing.T) {
	dir := scaffoldProject(t)
	if _, err := Generate(dir, Request{Kind: "controller", Name: "user"}, realTemplates()); err == nil {
		t.Error("unknown kind should fail")
	}
	if _, err := Generate(dir, Request{Kind: KindHandler, Name: "user-profile"}, realTemplates()); err == nil {
		t.Error("invalid entity name should fail")
	}
}

func TestModulePathParsing(t *testing.T) {
	dir := t.TempDir()
	goMod := "module example.com/quoted\n\ngo 1.22\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := modulePath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "example.com/quoted" {
		t.Errorf("modulePath = %q, want %q", got, "example.com/quoted")
	}
}

// TestGeneratedProjectCompiles is the end-to-end guarantee: scaffolding
// plus all three layers of the sample entity must produce a project
// that still builds. (Other entity names do not compile at this stage
// — the generated code references model types that only exist for the
// scaffold sample. The AST stage fixes that properly.)
func TestGeneratedProjectCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: builds the generated project")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}

	dir := scaffoldProject(t)
	for _, kind := range []Kind{KindHandler, KindService, KindRepository} {
		if _, err := Generate(dir, Request{Kind: kind, Name: "user"}, realTemplates()); err != nil {
			t.Fatalf("Generate %s: %v", kind, err)
		}
	}

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated project does not build: %v\n%s", err, out)
	}
}
