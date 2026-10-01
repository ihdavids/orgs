package templates

import (
	"os"
	"testing"

	"github.com/flosch/pongo2/v5"
)

// The character sheets are large pongo2 templates edited by hand, and a
// mistyped tag in one only shows up at export time. Parsing them here is the
// cheapest way to find that out.
func TestCharacterSheetTemplatesParse(t *testing.T) {
	for _, f := range []string{
		"../../templates/dnd_character_html.tpl",
		"../../templates/dnd_character.tpl",
	} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if _, err := pongo2.FromBytes(data); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

// The three documentation themes are one application in three shells, and the
// shells pull the application in with {% include %}. Two things can break
// silently there and neither shows up until somebody exports a page: a
// mistyped pongo2 tag, and an include that does not resolve - the path is
// taken relative to the template that writes it, so moving either file breaks
// it. FromFile rather than FromBytes, because that is what gives pongo2 a base
// directory to resolve the include against.
func TestDocThemeTemplatesParse(t *testing.T) {
	for _, f := range []string{
		"../../templates/html_docs.tpl",
		"../../templates/html_rtd.tpl",
		"../../templates/html_furo.tpl",
	} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if _, err := pongo2.FromFile(f); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}
