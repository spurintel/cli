package main

import (
	"strings"
	"testing"
)

// Multi-line named imports are the regression this guards: the old pattern
// only removed the first line, leaking `A,\nB,\n} from '...';` fragments into
// the committed MCP docs.
func TestCleanMDXStripsImports(t *testing.T) {
	t.Parallel()

	input := "import { TwoColumn, LeftColumn } from '@/components/two-column';\n" +
		"import {\n" +
		"  ApiObject,\n" +
		"  Steps,\n" +
		"} from '@/components';\n" +
		"import './styles.css';\n" +
		"\n" +
		"# Context API\n" +
		"\n" +
		"Body text that must survive.\n"

	got := cleanMDX(input, nil)

	for _, leak := range []string{"import", "from '@/components", "ApiObject,", "} from", "./styles.css"} {
		if strings.Contains(got, leak) {
			t.Errorf("cleaned output still contains %q:\n%s", leak, got)
		}
	}
	if !strings.Contains(got, "# Context API") || !strings.Contains(got, "Body text that must survive.") {
		t.Fatalf("cleaned output lost real content:\n%s", got)
	}
}

// Metadata exports, ApiObject placeholders, and JSX wrappers keep working
// alongside the import stripping.
func TestCleanMDXEndToEnd(t *testing.T) {
	t.Parallel()

	input := "import { TwoColumn } from '@/components/two-column';\n" +
		"\n" +
		"export const metadata = {\n" +
		"  title: 'Context API',\n" +
		"};\n" +
		"\n" +
		"<TwoColumn>\n" +
		"  Some wrapped text.\n" +
		"</TwoColumn>\n" +
		"\n" +
		"<ApiObject schema={ipContextSchema} />\n"

	got := cleanMDX(input, map[string]string{"ipContextSchema": "spur://schemas/ip-context"})

	if strings.Contains(got, "metadata") || strings.Contains(got, "TwoColumn") {
		t.Errorf("JSX/metadata residue in output:\n%s", got)
	}
	if !strings.Contains(got, "Some wrapped text.") {
		t.Errorf("wrapped content lost:\n%s", got)
	}
	if !strings.Contains(got, "resource spur://schemas/ip-context") {
		t.Errorf("ApiObject placeholder missing:\n%s", got)
	}
}
