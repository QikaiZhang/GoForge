package goforge

import (
	"io/fs"
	"testing"
	"text/template/parse"
)

// TestAllEmbeddedTemplatesParse is the contract that keeps the binary
// shippable: every template embedded into the binary must at least be
// syntactically valid. Content correctness is asserted where each
// template is consumed (internal/project, internal/generator).
func TestAllEmbeddedTemplatesParse(t *testing.T) {
	names, err := fs.Glob(Templates, "templates/*/*.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("no templates embedded — check the //go:embed pattern")
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			data, err := fs.ReadFile(Templates, name)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if _, err := parse.Parse(name, string(data), "{{", "}}", nil); err != nil {
				t.Errorf("template %s does not parse: %v", name, err)
			}
		})
	}
}
