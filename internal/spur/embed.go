package spur

//go:generate go run github.com/spurintel/cli/cmd/cleandocs

import "embed"

// Schemas is the embedded filesystem of JSON schemas copied from the docs
// repo by cmd/cleandocs. Each entry is named <name>.schema.json.
//
//go:embed schemas/*.schema.json
var schemasFS embed.FS

// Docs is the embedded filesystem of cleaned markdown documentation files
// also produced by cmd/cleandocs. Each entry is named context-api-*.md.
//
//go:embed docs/*.md
var docsFS embed.FS

// SchemasFS returns the embedded schema filesystem rooted at schemas/.
func SchemasFS() embed.FS { return schemasFS }

// DocsFS returns the embedded documentation filesystem rooted at docs/.
func DocsFS() embed.FS { return docsFS }
