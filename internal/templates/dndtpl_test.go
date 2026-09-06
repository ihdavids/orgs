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
