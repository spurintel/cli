// cleandocs reads Spur mdx documentation and JSON schemas from the Spur
// documentation source tree, strips JSX/import/metadata noise, and writes plain
// markdown plus verbatim schema copies into the spur CLI repo. It is the
// generator behind `go generate ./internal/spur`.
//
// Invocation (from anywhere in the module):
//
//	SPUR_DOCS_DIR=/path/to/docs/src go run github.com/spurintel/cli/cmd/cleandocs
//
// SPUR_DOCS_DIR must point at the documentation source tree; the generator
// requires that source and is not runnable without it. Output paths are
// resolved against the module root (the directory holding go.mod) so the tool
// behaves the same whether it is run by hand or by `go generate`. The generated
// outputs under internal/spur/docs and internal/spur/schemas are committed so
// regular builds do not depend on the docs source.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// moduleRoot walks up from the current working directory until it finds the
// directory containing go.mod, so path resolution does not depend on where the
// generator is invoked from.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

type cleanConfig struct {
	docsRoot      string
	outDocsDir    string
	outSchemasDir string
	mdxRelDir     string
	schemasRelDir string
	schemaVarURI  map[string]string
}

func defaultConfig(root string) cleanConfig {
	return cleanConfig{
		outDocsDir:    filepath.Join(root, "internal", "spur", "docs"),
		outSchemasDir: filepath.Join(root, "internal", "spur", "schemas"),
		mdxRelDir:     filepath.Join("app", "(docs)", "context-api"),
		schemasRelDir: "types",
		schemaVarURI: map[string]string{
			"ipContextSchema":   "spur://schemas/ip-context",
			"tagMetadataSchema": "spur://schemas/tag-metadata",
		},
	}
}

func resolveDocsRoot(moduleRoot string) (string, error) {
	if dir := os.Getenv("SPUR_DOCS_DIR"); dir != "" {
		return dir, nil
	}
	abs := filepath.Join(filepath.Dir(moduleRoot), "web", "apps", "docs", "src")
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("SPUR_DOCS_DIR not set and fallback %s not found: %w", abs, err)
	}
	return abs, nil
}

func main() {
	root, err := moduleRoot()
	if err != nil {
		fail(err)
	}
	cfg := defaultConfig(root)
	docsRoot, err := resolveDocsRoot(root)
	if err != nil {
		fail(err)
	}
	cfg.docsRoot = docsRoot

	if err := ensureDirs(cfg); err != nil {
		fail(err)
	}

	mdxFiles, err := listMDX(filepath.Join(cfg.docsRoot, cfg.mdxRelDir))
	if err != nil {
		fail(err)
	}
	if len(mdxFiles) == 0 {
		fail(fmt.Errorf("no mdx files found under %s", filepath.Join(cfg.docsRoot, cfg.mdxRelDir)))
	}

	var writtenDocs []string
	// Output names derive from each page's directory, so two mdx files in one
	// directory would silently overwrite each other; fail loudly instead.
	byDst := map[string]string{}
	for _, src := range mdxFiles {
		dst, err := cleanMDXFile(src, cfg)
		if err != nil {
			fail(fmt.Errorf("clean %s: %w", src, err))
		}
		if prev, ok := byDst[dst]; ok {
			fail(fmt.Errorf("output name collision: %s and %s both map to %s", prev, src, dst))
		}
		byDst[dst] = src
		writtenDocs = append(writtenDocs, dst)
	}

	schemas := []string{"ip-context.schema.json", "tag-metadata.schema.json"}
	var writtenSchemas []string
	for _, name := range schemas {
		src := filepath.Join(cfg.docsRoot, cfg.schemasRelDir, name)
		dst := filepath.Join(cfg.outSchemasDir, name)
		if err := copyFile(src, dst); err != nil {
			fail(fmt.Errorf("copy schema %s: %w", name, err))
		}
		writtenSchemas = append(writtenSchemas, dst)
	}

	fmt.Printf("cleandocs: wrote %d markdown files and %d schemas\n",
		len(writtenDocs), len(writtenSchemas))
	for _, p := range writtenDocs {
		fmt.Printf("  md  %s\n", p)
	}
	for _, p := range writtenSchemas {
		fmt.Printf("  js  %s\n", p)
	}
}

func ensureDirs(cfg cleanConfig) error {
	for _, d := range []string{cfg.outDocsDir, cfg.outSchemasDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func listMDX(dir string) ([]string, error) {
	var files []string
	walker := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".mdx") {
			files = append(files, path)
		}
		return nil
	}
	if err := filepath.WalkDir(dir, walker); err != nil {
		return nil, err
	}
	return files, nil
}

func cleanMDXFile(src string, cfg cleanConfig) (string, error) {
	raw, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	cleaned := cleanMDX(string(raw), cfg.schemaVarURI)

	// The docs repo lays out one page.mdx per directory, so the directory name
	// is the page's identity; the root context-api page becomes "overview".
	base := filepath.Base(filepath.Dir(src))
	name := "context-api-" + base + ".md"
	if base == "context-api" {
		name = "context-api-overview.md"
	}
	dst := filepath.Join(cfg.outDocsDir, name)
	if err := os.WriteFile(dst, []byte(cleaned), 0o644); err != nil {
		return "", err
	}
	return dst, nil
}

// reImport strips `import ... from '...'` statements, including multi-line
// named-import blocks (the negated character class crosses newlines), and bare
// side-effect imports (`import './x.css'`). Anchored to a line start and to a
// line end right after the module specifier so it never eats past the
// statement.
var reImport = regexp.MustCompile(`(?m)^import\s(?:[^'"]*?from\s*)?['"][^'"]+['"];?[ \t]*$`)

// reExportMetadata strips `export const metadata = { ... }` across newlines.
var reExportMetadata = regexp.MustCompile(`(?s)export\s+const\s+metadata\s*=\s*\{.*?\}\s*;?`)

// reExportConst strips other `export const X = ...;` lines.
var reExportConstLine = regexp.MustCompile(`(?m)^export\s+const\s+\w+.*$`)

// jsxUnwrapTags are paired JSX wrappers whose open/close tags we strip
// while keeping and dedenting the content in between.
var jsxUnwrapTags = []string{"TwoColumn", "LeftColumn", "RightColumn", "Steps"}

// reApiObject matches <ApiObject schema={schemaVar} /> and its paired form.
var reApiObject = regexp.MustCompile(`<ApiObject\s+schema=\{(\w+)\}\s*/?>\s*</ApiObject>|<ApiObject\s+schema=\{(\w+)\}\s*/>`)

// reCodeFence matches a fenced code-block boundary (``` ...).
var reCodeFence = regexp.MustCompile("^\\s*```")

func cleanMDX(input string, schemaURIs map[string]string) string {
	s := input

	s = reImport.ReplaceAllString(s, "")
	s = reExportMetadata.ReplaceAllString(s, "")
	s = reExportConstLine.ReplaceAllString(s, "")

	s = reApiObject.ReplaceAllStringFunc(s, func(match string) string {
		m := reApiObject.FindStringSubmatch(match)
		varName := ""
		for _, g := range m[1:] {
			if g != "" {
				varName = g
				break
			}
		}
		uri, ok := schemaURIs[varName]
		if !ok {
			return fmt.Sprintf("> Full field reference: schema %s", varName)
		}
		return fmt.Sprintf("> Full field reference: resource %s", uri)
	})

	s = unwrapJSXWrappers(s, jsxUnwrapTags)
	s = collapseBlankLines(s)
	return strings.TrimSpace(s) + "\n"
}

// unwrapJSXWrappers removes lines that are sole open/close tags for the given
// tag names and dedents any lines between them by up to four spaces. Content
// inside fenced code blocks is left untouched so code indentation is
// preserved.
func unwrapJSXWrappers(input string, tags []string) string {
	openPatterns := make([]*regexp.Regexp, len(tags))
	closePatterns := make([]*regexp.Regexp, len(tags))
	for i, t := range tags {
		openPatterns[i] = regexp.MustCompile(`^\s*<` + t + `\b[^>]*>\s*$`)
		closePatterns[i] = regexp.MustCompile(`^\s*</` + t + `>\s*$`)
	}

	lines := strings.Split(input, "\n")
	var depth int
	inCode := false
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if reCodeFence.MatchString(line) {
			inCode = !inCode
			out = append(out, maybeDedent(line, depth))
			continue
		}
		if !inCode {
			matchedOpen := false
			for _, p := range openPatterns {
				if p.MatchString(line) {
					depth++
					matchedOpen = true
					break
				}
			}
			if matchedOpen {
				continue
			}
			matchedClose := false
			for _, p := range closePatterns {
				if p.MatchString(line) {
					if depth > 0 {
						depth--
					}
					matchedClose = true
					break
				}
			}
			if matchedClose {
				continue
			}
		}
		out = append(out, maybeDedent(line, depth))
	}
	return strings.Join(out, "\n")
}

// maybeDedent strips up to (depth * 2) leading spaces from a line when we are
// inside a JSX wrapper. MDX indents wrapper contents by two spaces per level;
// the docs use four-space indent in practice so we cap at four for safety.
// Code lines are dedented the same way so fences start at column zero;
// otherwise markdown treats them as indented blocks.
func maybeDedent(line string, depth int) string {
	if depth <= 0 {
		return line
	}
	strip := depth * 2
	if strip > 4 {
		strip = 4
	}
	removed := 0
	for removed < strip && len(line) > 0 && line[0] == ' ' {
		line = line[1:]
		removed++
	}
	return line
}

var reBlankLines = regexp.MustCompile(`\n{3,}`)

func collapseBlankLines(s string) string {
	return reBlankLines.ReplaceAllString(s, "\n\n")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	// The close flushes the write; ignoring its error could commit a
	// truncated schema with a success exit.
	return out.Close()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "cleandocs:", err)
	os.Exit(1)
}
