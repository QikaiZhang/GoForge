package template

import (
	"strings"
	"testing"
	"testing/fstest"
)

func fixtureFS() fstest.MapFS {
	return fstest.MapFS{
		"greeting.tmpl": &fstest.MapFile{Data: []byte("hello {{.Name}}")},
	}
}

func TestRender(t *testing.T) {
	out, err := Render(fixtureFS(), "greeting.tmpl", struct{ Name string }{"user-service"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := string(out), "hello user-service"; got != want {
		t.Errorf("Render output = %q, want %q", got, want)
	}
}

func TestRenderMissingTemplate(t *testing.T) {
	_, err := Render(fixtureFS(), "nope.tmpl", nil)
	if err == nil {
		t.Fatal("Render of a missing template should fail")
	}
	if !strings.Contains(err.Error(), "nope.tmpl") {
		t.Errorf("error = %v, want it to name the missing template", err)
	}
}

func TestRenderBrokenTemplate(t *testing.T) {
	broken := fstest.MapFS{
		"broken.tmpl": &fstest.MapFile{Data: []byte("{{.Name")},
	}
	_, err := Render(broken, "broken.tmpl", nil)
	if err == nil {
		t.Fatal("Render of a syntactically broken template should fail")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("error = %v, want it to mention the parse step", err)
	}
}

func TestRenderDataMismatch(t *testing.T) {
	// Rendering with a struct that lacks the field the template uses
	// must fail loudly, not silently emit "<no value>".
	_, err := Render(fixtureFS(), "greeting.tmpl", struct{ Wrong string }{"x"})
	if err == nil {
		t.Fatal("Render with mismatched data should fail")
	}
}
