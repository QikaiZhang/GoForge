// Package template renders goforge's built-in templates.
//
// It is the single place in goforge that knows about text/template.
// Callers hand in an fs.FS (embedded in production, fstest.MapFS in
// tests, os.DirFS in tools), a template name relative to that FS, and
// the data; they get back rendered bytes. Deciding WHAT to generate
// is the generator's job, not this package's.
package template

import (
	"bytes"
	"fmt"
	"io/fs"
	"text/template"
)

// Render executes the named template from fsys with data.
func Render(fsys fs.FS, name string, data any) ([]byte, error) {
	tmpl, err := template.ParseFS(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template %s: %w", name, err)
	}
	return buf.Bytes(), nil
}
