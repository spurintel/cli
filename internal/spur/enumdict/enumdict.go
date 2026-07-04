// Package enumdict walks the embedded Spur JSON Schemas and builds a flat
// lookup table of enum values to their (category, description) pairs. It is
// used by the explain_enum tool to answer questions like
// "what does CALLBACK_PROXY mean?".
//
// The walker handles the three shapes the Spur schemas use:
//   - "enum": [ "A", "B" ] (flat enum, no per-value description)
//   - "oneOf": [ { "const": "A", "description": "..." } ] (preferred; per-value descriptions)
//   - "anyOf": [ ... ] (same as oneOf; used for open-ended patterns)
package enumdict

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// Entry is a single occurrence of an enum value in one of the schemas.
type Entry struct {
	Value       string `json:"value"`
	Category    string `json:"category"`
	Description string `json:"description,omitempty"`
	AppearsIn   string `json:"appearsIn"`
	Schema      string `json:"schema"`
}

// Dict is an immutable, case-insensitive lookup table.
type Dict struct {
	byKey map[string][]Entry
}

// Build parses every *.schema.json in fsys and returns a Dict. The fsys
// argument is typically an embedded filesystem containing the Spur schemas.
func Build(fsys fs.FS) (*Dict, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	d := &Dict{byKey: map[string][]Entry{}}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".schema.json") {
			continue
		}
		b, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		var root map[string]any
		if err := json.Unmarshal(b, &root); err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		schemaName := strings.TrimSuffix(e.Name(), ".schema.json")
		walk(root, schemaName, "", d)
	}
	return d, nil
}

// Lookup returns all entries that match value (case-insensitive). Multiple
// entries mean the value appears in more than one field in the schemas.
func (d *Dict) Lookup(value string) []Entry {
	if d == nil {
		return nil
	}
	return d.byKey[strings.ToUpper(strings.TrimSpace(value))]
}

// Categories returns the distinct category names, sorted. Useful for hinting
// when an unknown value is looked up.
func (d *Dict) Categories() []string {
	seen := map[string]struct{}{}
	for _, entries := range d.byKey {
		for _, e := range entries {
			seen[e.Category] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// walk recursively traverses a JSON Schema node, recording any const+description
// entries it finds under oneOf/anyOf, and plain enum arrays. The path parameter
// tracks the dotted property path (e.g., "properties.risks.items") so callers
// can see where a value appeared.
func walk(node any, schema, path string, d *Dict) {
	switch n := node.(type) {
	case map[string]any:
		// A node may be a schema with direct oneOf/anyOf children whose
		// members have "const" + "description".
		if category, ok := categoryForPath(path); ok {
			for _, key := range []string{"oneOf", "anyOf"} {
				if arr, ok := n[key].([]any); ok {
					harvestOneOf(arr, schema, category, path, d)
				}
			}
			if enum, ok := n["enum"].([]any); ok {
				harvestPlainEnum(enum, schema, category, path, d)
			}
		}
		// Recurse into every property to keep crawling.
		for k, v := range n {
			walk(v, schema, joinPath(path, k), d)
		}
	case []any:
		for i, item := range n {
			walk(item, schema, fmt.Sprintf("%s[%d]", path, i), d)
		}
	}
}

// harvestOneOf records entries from a oneOf/anyOf array of const+description
// nodes.
func harvestOneOf(arr []any, schema, category, path string, d *Dict) {
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		value, _ := m["const"].(string)
		if value == "" {
			continue
		}
		desc, _ := m["description"].(string)
		d.add(Entry{
			Value:       value,
			Category:    category,
			Description: desc,
			AppearsIn:   path,
			Schema:      schema,
		})
	}
}

// harvestPlainEnum records entries from an enum array when no per-value
// descriptions are available.
func harvestPlainEnum(arr []any, schema, category, path string, d *Dict) {
	for _, item := range arr {
		value, ok := item.(string)
		if !ok || value == "" {
			continue
		}
		d.add(Entry{
			Value:     value,
			Category:  category,
			AppearsIn: path,
			Schema:    schema,
		})
	}
}

func (d *Dict) add(e Entry) {
	key := strings.ToUpper(e.Value)
	// Dedupe: if we already recorded this (value, category, schema) skip.
	for _, existing := range d.byKey[key] {
		if existing.Category == e.Category && existing.Schema == e.Schema && existing.AppearsIn == e.AppearsIn {
			return
		}
	}
	d.byKey[key] = append(d.byKey[key], e)
}

// categoryForPath derives a human category (e.g. "risks") from a schema path.
// We only treat leaf-like paths ending in "items" or at a top-level property
// as enum-carrying.
func categoryForPath(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	parts := strings.Split(path, ".")
	// Trim any items / [N] artefacts when identifying the owning field.
	var cleaned []string
	for _, p := range parts {
		if p == "items" {
			continue
		}
		if idx := strings.Index(p, "["); idx != -1 {
			p = p[:idx]
		}
		cleaned = append(cleaned, p)
	}
	if len(cleaned) == 0 {
		return "", false
	}
	// The field name is the last "properties" child.
	for i := len(cleaned) - 1; i >= 0; i-- {
		if i > 0 && cleaned[i-1] == "properties" {
			return cleaned[i], true
		}
	}
	return "", false
}

func joinPath(a, b string) string {
	if a == "" {
		return b
	}
	return a + "." + b
}
