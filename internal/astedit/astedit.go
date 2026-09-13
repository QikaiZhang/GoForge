// Package astedit edits existing Go source files.
//
// The design is a deliberate hybrid, chosen after the pure-AST route
// misbehaved:
//
//   - AST (go/parser) answers semantic questions — "is this type
//     declared?", "is the wiring already in the function body?",
//     "where exactly does the function body end?" — robust against
//     formatting and comments.
//   - The actual insertion is done as text at AST-computed offsets.
//     Printing a mutated AST through go/printer mixes the positions
//     of synthetic nodes with the real file's comment positions and
//     shuffles comments into the middle of statements; text splicing
//     never touches existing code, so comments survive untouched.
//   - go/format canonicalizes and validates the result: if the splice
//     produced anything syntactically broken, format.Source fails and
//     nothing is written.
//
// Only append-style edits are supported (imports, wiring statements,
// type declarations). That covers goforge's needs and keeps the
// failure modes simple: either the file is unchanged, or it is the
// old file plus new, gofmt-clean code.
package astedit

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
)

// EnsureFuncWiring appends stmtSrc to the body of funcName in the Go
// file at path and makes sure every path in importPaths is imported.
// It is idempotent: when a statement in the body already contains
// key (e.g. the constructor call being wired), the file is left
// untouched and (false, nil) is returned.
//
// It fails without writing when funcName does not exist, when a
// snippet does not parse, or when the result is not valid Go.
func EnsureFuncWiring(path, funcName string, importPaths []string, stmtSrc, key string) (bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	fset, f, err := parse(path, src)
	if err != nil {
		return false, err
	}
	fn := FuncDecl(f, funcName)
	if fn == nil {
		return false, fmt.Errorf("wire %s: function %s not found", path, funcName)
	}

	wired, err := BodyContainsStmt(fset, f, funcName, key)
	if err != nil {
		return false, err
	}
	if wired {
		return false, nil
	}

	out := string(src)

	// Splice order matters: every offset below is computed against the
	// ORIGINAL source. Splice from the end towards the beginning so
	// each edit only shifts content after its own insertion point:
	// first the statements (near the end of the file), then the
	// imports (near the top).
	out, err = appendStmtsBeforeBodyEnd(out, fset, fn, stmtSrc)
	if err != nil {
		return false, err
	}
	if missing := missingImports(f, importPaths); len(missing) > 0 {
		out, err = addImports(out, fset, f, missing)
		if err != nil {
			return false, err
		}
	}

	if err := writeFormatted(path, []byte(out)); err != nil {
		return false, err
	}
	return true, nil
}

// EnsureType appends declSrc to the Go file at path unless a type
// named typeName is already declared there. It returns whether the
// file changed.
func EnsureType(path, typeName, declSrc string) (bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	_, f, err := parse(path, src)
	if err != nil {
		return false, err
	}
	if HasType(f, typeName) {
		return false, nil
	}

	out := strings.TrimRight(string(src), "\n") + "\n\n" + strings.TrimRight(declSrc, "\n") + "\n"
	if err := writeFormatted(path, []byte(out)); err != nil {
		return false, err
	}
	return true, nil
}

// --- queries ---

// FuncDecl returns the top-level function named name, or nil.
func FuncDecl(f *ast.File, name string) *ast.FuncDecl {
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name {
			return fn
		}
	}
	return nil
}

// HasType reports whether a top-level type named name is declared.
func HasType(f *ast.File, name string) bool {
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.Name == name {
				return true
			}
		}
	}
	return false
}

// HasImport reports whether path is imported by f.
func HasImport(f *ast.File, path string) bool {
	for _, imp := range f.Imports {
		if p, err := strconv.Unquote(imp.Path.Value); err == nil && p == path {
			return true
		}
	}
	return false
}

// BodyContainsStmt reports whether the body of the named function has
// a statement whose canonical source contains substr. Answering the
// question on the syntax tree (not on raw text) is what makes the
// wiring idempotent regardless of formatting.
func BodyContainsStmt(fset *token.FileSet, f *ast.File, funcName, substr string) (bool, error) {
	fn := FuncDecl(f, funcName)
	if fn == nil || fn.Body == nil {
		return false, fmt.Errorf("function %s not found in %s", funcName, f.Name.Name)
	}
	for _, stmt := range fn.Body.List {
		var buf bytes.Buffer
		if err := format.Node(&buf, fset, stmt); err != nil {
			return false, err
		}
		if bytes.Contains(buf.Bytes(), []byte(substr)) {
			return true, nil
		}
	}
	return false, nil
}

// --- internals ---

func parse(path string, src []byte) (*token.FileSet, *ast.File, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return fset, f, nil
}

// writeFormatted canonicalizes formatted src and writes it to path.
// format.Source doubles as the validation gate: broken output never
// reaches the disk.
func writeFormatted(path string, src []byte) error {
	clean, err := format.Source(src)
	if err != nil {
		return fmt.Errorf("format %s: %w", path, err)
	}
	return os.WriteFile(path, clean, 0o644)
}

func missingImports(f *ast.File, paths []string) []string {
	var missing []string
	for _, p := range paths {
		if !HasImport(f, p) {
			missing = append(missing, p)
		}
	}
	return missing
}

// addImports returns src with every path in imports added to the
// import block. It handles the three shapes a file can have: a
// parenthesized block, a single unparenthesized import, and no import
// declaration at all. Splicing text keeps every existing comment in
// place; format.Source (in writeFormatted) then sorts the block.
func addImports(src string, fset *token.FileSet, f *ast.File, imports []string) (string, error) {
	var newLines strings.Builder
	for _, p := range imports {
		newLines.WriteString("\t" + strconv.Quote(p) + "\n")
	}
	block := newLines.String()

	var decl *ast.GenDecl
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
			decl = gd
			break
		}
	}

	switch {
	case decl == nil:
		// Insert an import block right after the package clause.
		pkgEnd := fset.Position(f.Name.End()).Offset
		lineEnd := strings.IndexByte(src[pkgEnd:], '\n')
		if lineEnd < 0 {
			return "", errors.New("cannot find a newline after the package clause")
		}
		at := pkgEnd + lineEnd + 1
		return src[:at] + "\nimport (\n" + block + ")\n" + src[at:], nil

	case decl.Lparen.IsValid():
		// Insert as the first lines inside the parentheses; the
		// format pass sorts the block afterwards.
		at := fset.Position(decl.Lparen).Offset + 1 // past the "("
		if at < len(src) && src[at] == '\n' {
			at++
		}
		return src[:at] + block + src[at:], nil

	default:
		// Single import ("import \"fmt\""): rewrite the declaration
		// as a parenthesized block containing both imports.
		from := fset.Position(decl.Pos()).Offset
		to := fset.Position(decl.End()).Offset
		first := src[from:to]
		block = "\t" + strings.TrimPrefix(strings.TrimSpace(first), "import ") + "\n" + block
		return src[:from] + "import (\n" + block + ")" + src[to:], nil
	}
}

// appendStmtsBeforeBodyEnd splices stmtSrc into the function body,
// right before its closing brace. The splice offset comes from the
// AST, so formatting differences and comments in the original body
// cannot redirect the insertion.
func appendStmtsBeforeBodyEnd(src string, fset *token.FileSet, fn *ast.FuncDecl, stmtSrc string) (string, error) {
	if !fn.Body.Rbrace.IsValid() {
		return "", errors.New("function body has no closing brace")
	}
	rbrace := fset.Position(fn.Body.Rbrace).Offset
	lineStart := strings.LastIndexByte(src[:rbrace], '\n') + 1

	renderStmts := func(indent string) string {
		var b strings.Builder
		for _, line := range strings.Split(strings.TrimRight(stmtSrc, "\n"), "\n") {
			if strings.TrimSpace(line) == "" {
				b.WriteString("\n")
				continue
			}
			b.WriteString(indent + line + "\n")
		}
		return b.String()
	}

	if ws := src[lineStart:rbrace]; strings.TrimLeft(ws, " \t") == "" {
		// The closing brace owns its line (the gofmt-formatted case):
		// insert whole lines before the brace line, one level deeper
		// than the brace.
		return src[:lineStart] + renderStmts(ws+"\t") + ws + src[rbrace:], nil
	}
	// Braces share a line with code (`func f() {}`): open the block on
	// a new line and let format.Source canonicalize indentation.
	return src[:rbrace] + "\n" + renderStmts("\t") + src[rbrace:], nil
}
