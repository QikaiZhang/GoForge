// Package generator adds feature code — handler, service, repository
// layers — to an existing goforge project.
//
// Generate decides WHAT gets written and WHERE; template rendering
// stays in the template package. The generator owns naming, target
// paths, the overwrite policy, the "is this actually a goforge
// project" check, and wiring the new code into the app.
package generator

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"goforge/internal/template"
)

// Kind selects the layer to generate.
type Kind string

const (
	KindHandler    Kind = "handler"
	KindService    Kind = "service"
	KindRepository Kind = "repository"
)

// kindSpec maps a kind to its output directory (under internal/) and
// its template (under the template FS root).
var kindSpecs = map[Kind]struct{ dir, tmpl string }{
	KindHandler:    {dir: "handler", tmpl: "handler/handler.go.tmpl"},
	KindService:    {dir: "service", tmpl: "service/service.go.tmpl"},
	KindRepository: {dir: "repository", tmpl: "repository/repository.go.tmpl"},
}

// Valid reports whether k is one of the three known kinds.
func (k Kind) Valid() bool {
	_, ok := kindSpecs[k]
	return ok
}

// Request is one generate invocation.
type Request struct {
	Kind  Kind
	Name  string // entity name in snake_case, e.g. user, user_profile
	Force bool   // overwrite an existing file
}

// ErrFileExists is returned when the target file is already there and
// Request.Force is false.
var ErrFileExists = errors.New("file already exists")

// Data is the template data for generated layer files.
type Data struct {
	Name   string // user
	Pascal string // User (used in type and constructor names)
	Module string // the module path from the project's go.mod
}

// Generate renders the file for req into the project rooted at dir
// (usually ".") and returns the path it created.
//
// A handler generation additionally wires the feature into the app
// (see wiring.go): the model type is ensured and registerRoutes in
// cmd/server/main.go starts serving the new routes. When only the
// wiring fails, the returned path still names the created file and
// the error explains what did not happen.
func Generate(dir string, req Request, templates fs.FS) (string, error) {
	if !req.Kind.Valid() {
		return "", fmt.Errorf("unknown kind %q (want handler, service or repository)", req.Kind)
	}
	if err := ValidateName(req.Name); err != nil {
		return "", err
	}
	module, err := modulePath(dir)
	if err != nil {
		return "", err
	}

	data := Data{Name: req.Name, Pascal: Pascal(req.Name), Module: module}
	path, err := renderLayer(dir, req, data, templates)
	if err != nil {
		return "", err
	}
	if err := wireFeature(dir, data, req.Kind, templates); err != nil {
		return path, fmt.Errorf("%s created, but wiring failed: %w", path, err)
	}
	return path, nil
}

// renderLayer renders and writes the file for one kind, enforcing the
// overwrite policy.
func renderLayer(dir string, req Request, data Data, templates fs.FS) (string, error) {
	spec := kindSpecs[req.Kind]
	outDir := filepath.Join(dir, "internal", spec.dir)
	outPath := filepath.Join(outDir, req.Name+".go")

	if _, err := os.Stat(outPath); err == nil && !req.Force {
		return "", fmt.Errorf("%s: %w (use --force to overwrite)", outPath, ErrFileExists)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("stat %s: %w", outPath, err)
	}

	content, err := template.Render(templates, spec.tmpl, data)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("create directory %s: %w", outDir, err)
	}
	if err := os.WriteFile(outPath, content, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", outPath, err)
	}
	return outPath, nil
}

// modulePath extracts the module path from the go.mod in dir.
//
// Parsing go.mod with string operations is a deliberate simplification:
// the file's grammar is small and stable, and the alternative
// (golang.org/x/mod/modfile) pulls in a dependency for one line. If
// goforge ever needs to MODIFY go.mod, switch to x/mod — write access
// is where hand parsing stops being defensible.
func modulePath(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%s is not inside a goforge project (go.mod not found)", dir)
	} else if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`), nil
		}
	}
	return "", fmt.Errorf("go.mod in %s has no module line", dir)
}
