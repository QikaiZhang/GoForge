package astedit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleMain = `package main

import (
	"log"
	"net/http"
)

func registerRoutes(mux *http.ServeMux) {
	// existing wiring stays untouched
}
`

// edit applies fn to a real temp file and returns the file contents
// afterwards.
func edit(t *testing.T, src string, fn func(path string) (bool, error)) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := fn(path); err != nil {
		t.Fatalf("edit: %v", err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestEnsureFuncWiring(t *testing.T) {
	out := edit(t, sampleMain, func(path string) (bool, error) {
		return EnsureFuncWiring(path, "registerRoutes",
			[]string{"example.com/demo/internal/handler"},
			"h := handler.NewUserHandler()\nh.Register(mux)",
			"NewUserHandler(")
	})

	for _, want := range []string{
		`"example.com/demo/internal/handler"`,
		"h := handler.NewUserHandler()",
		"h.Register(mux)",
		"// existing wiring stays untouched",
		`"net/http"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
	// The import block must stay sorted ("example.com/..." sorts
	// before "log" and "net/http").
	if strings.Index(out, "example.com/demo") > strings.Index(out, `"log"`) {
		t.Errorf("imports are not sorted:\n%s", out)
	}
}

func TestEnsureFuncWiringIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.go")
	if err := os.WriteFile(path, []byte(sampleMain), 0o644); err != nil {
		t.Fatal(err)
	}

	wire := func() (bool, error) {
		return EnsureFuncWiring(path, "registerRoutes",
			[]string{"example.com/demo/internal/handler"},
			"h := handler.NewUserHandler()\nh.Register(mux)",
			"NewUserHandler(")
	}
	if wired, err := wire(); err != nil || !wired {
		t.Fatalf("first wiring: wired=%v err=%v", wired, err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	wired, err := wire()
	if err != nil {
		t.Fatalf("second wiring: %v", err)
	}
	if wired {
		t.Error("second wiring should report false")
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("second wiring changed the file:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestEnsureFuncWiringMissingFunction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.go")
	if err := os.WriteFile(path, []byte(sampleMain), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureFuncWiring(path, "nope", nil, "x := 1", "x :="); err == nil {
		t.Error("wiring into a missing function should fail")
	}
}

func TestEnsureFuncWiringWithoutImportDecl(t *testing.T) {
	out := edit(t, "package main\n\nfunc setup() {}\n", func(path string) (bool, error) {
		return EnsureFuncWiring(path, "setup",
			[]string{"fmt", "example.com/demo/internal/handler"},
			"h := handler.New()\nfmt.Println(h)",
			"handler.New(")
	})
	if !strings.Contains(out, `"fmt"`) || !strings.Contains(out, `"example.com/demo/internal/handler"`) {
		t.Errorf("imports missing:\n%s", out)
	}
	if !strings.Contains(out, "h := handler.New()") {
		t.Errorf("statements missing:\n%s", out)
	}
}

func TestEnsureFuncWiringSingleImport(t *testing.T) {
	src := "package main\n\nimport \"fmt\"\n\nfunc setup() {}\n"
	out := edit(t, src, func(path string) (bool, error) {
		return EnsureFuncWiring(path, "setup",
			[]string{"example.com/demo/internal/handler"},
			"h := handler.New()",
			"handler.New(")
	})
	if !strings.Contains(out, `"fmt"`) || !strings.Contains(out, `"example.com/demo/internal/handler"`) {
		t.Errorf("both imports must be present:\n%s", out)
	}
	if !strings.Contains(out, "h := handler.New()") {
		t.Errorf("statements missing:\n%s", out)
	}
}

func TestEnsureType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model.go")
	src := "// Package model holds domain types.\npackage model\n\ntype User struct{}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	decl := "// Order is a generated domain type.\ntype Order struct {\n\tID   string\n\tName string\n}"
	added, err := EnsureType(path, "Order", decl)
	if err != nil || !added {
		t.Fatalf("EnsureType: added=%v err=%v", added, err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"package model", "type User struct", "type Order struct", "ID   string"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}

	added, err = EnsureType(path, "Order", decl)
	if err != nil || added {
		t.Errorf("second EnsureType: added=%v err=%v, want false nil", added, err)
	}
}
