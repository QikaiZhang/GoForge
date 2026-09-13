// Package project scaffolds new Go service skeletons.
//
// Create knows the filesystem layout of a generated project; the
// content of every file lives in templates/project and is rendered
// through the template package. The template FS is injected so tests
// can supply their own.
package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"goforge/internal/template"
)

// ErrDirExists is returned by Create when the target directory is
// already there. Callers detect it with errors.Is to produce a
// friendlier message (and a different exit code) than a generic
// filesystem failure.
var ErrDirExists = errors.New("project directory already exists")

// Data is the template data available to project templates.
type Data struct {
	// Name is the project name and the Go module name.
	Name string
}

// scaffoldDirs are created empty. The layer directories start with a
// .gitkeep because git does not track empty directories; real files
// land there when `goforge generate` runs.
var scaffoldDirs = []string{
	"cmd/server",
	"internal/handler",
	"internal/service",
	"internal/repository",
	"internal/model",
	"api",
	"configs",
}

// fileSpec is one rendered file in the skeleton: the path relative to
// the project root and the template (relative to the template FS) it
// is rendered from. An empty template name writes an empty file.
type fileSpec struct {
	path string
	tmpl string
}

var scaffoldFiles = []fileSpec{
	{"go.mod", "project/go.mod.tmpl"},
	{filepath.Join("cmd", "server", "main.go"), "project/main.go.tmpl"},
	{filepath.Join("internal", "model", "model.go"), "project/model.go.tmpl"},
	{filepath.Join("internal", "handler", "respond.go"), "project/respond.go.tmpl"},
	{filepath.Join("configs", "config.yaml"), "project/config.yaml.tmpl"},
	{"goforge.yaml", "project/goforge.yaml.tmpl"},
	{filepath.Join("api", ".gitkeep"), ""},
	{filepath.Join("internal", "service", ".gitkeep"), ""},
	{filepath.Join("internal", "repository", ".gitkeep"), ""},
	{".gitignore", "project/gitignore.tmpl"},
	{"README.md", "project/README.md.tmpl"},
}

// ValidateName reports whether name can be used as a project (and
// therefore Go module) name. The rules are deliberately stricter than
// what the Go module system allows: one lowercase word with digits and
// inner dashes is enough for scaffolding and keeps every downstream
// consumer (module path, directory, process name) unambiguous.
func ValidateName(name string) error {
	switch {
	case name == "":
		return errors.New("project name must not be empty")
	case strings.ContainsAny(name, `/\`) || name == "." || name == "..":
		return fmt.Errorf("%q must be a plain name, not a path", name)
	}
	for i, r := range name {
		switch {
		case i == 0 && (r < 'a' || r > 'z'):
			// A leading digit would make some Go identifiers
			// (e.g. derived type names) illegal.
			return fmt.Errorf("%q must start with a lowercase letter", name)
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-':
			// ok
		default:
			return fmt.Errorf("%q may only contain lowercase letters, digits and dashes", name)
		}
	}
	if strings.HasSuffix(name, "-") {
		return fmt.Errorf("%q must not end with a dash", name)
	}
	return nil
}

// Create scaffolds a project skeleton rooted at dir. dir must not
// exist; name becomes the Go module name. templates is the FS the
// project templates are rendered from (the embedded FS in production,
// a fixture FS in tests).
func Create(dir, name string, templates fs.FS) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("%s: %w", dir, ErrDirExists)
	} else if !errors.Is(err, fs.ErrNotExist) {
		// Not "missing" but unreadable/permission-denied: report as-is.
		return fmt.Errorf("stat %s: %w", dir, err)
	}

	data := Data{Name: name}

	for _, d := range scaffoldDirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", d, err)
		}
	}
	for _, f := range scaffoldFiles {
		content := []byte{}
		if f.tmpl != "" {
			var err error
			if content, err = template.Render(templates, f.tmpl, data); err != nil {
				return fmt.Errorf("render %s: %w", f.tmpl, err)
			}
		}
		path := filepath.Join(dir, f.path)
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", f.path, err)
		}
	}
	return nil
}
