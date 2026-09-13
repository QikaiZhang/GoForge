// Package goforge holds assets that must ship inside the goforge
// binary. go:embed cannot reference parent directories, so the
// embedded templates live at the repository root next to this file.
//
// Consumers receive it as an io/fs.FS (that is what project.Create
// and template.Render accept), which keeps them independent of where
// the bytes actually come from.
package goforge

import "embed"

// Templates is the embedded template tree (templates/project,
// templates/handler, templates/service, templates/repository).
//
// The template for .gitignore is named gitignore.tmpl because
// go:embed excludes dotfiles from plain patterns; the project package
// renders it to the real .gitignore path.
//
//go:embed templates
var Templates embed.FS
