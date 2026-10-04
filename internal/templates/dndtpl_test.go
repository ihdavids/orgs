package templates

import (
	"os"
	"regexp"
	"strings"
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

// On a phone the html sheet shows one page at a time, and the tabs that turn
// the pages come from M_PAGES in the sheet's script. A section given a
// data-page that list does not name is still hidden with the rest, but no tab
// ever shows it: on a phone it would simply be gone. Nothing at export time
// notices, so this does.
func TestCharacterSheetPhonePagesHaveTabs(t *testing.T) {
	data, err := os.ReadFile("../../templates/dnd_character_html.tpl")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	at := strings.Index(s, "var M_PAGES = [")
	if at < 0 {
		t.Fatal("no M_PAGES in the sheet's script")
	}
	end := strings.Index(s[at:], "];")
	tabs := map[string]bool{}
	for _, m := range regexp.MustCompile(`\['([a-z]+)',`).FindAllStringSubmatch(s[at:at+end], -1) {
		tabs[m[1]] = true
	}
	used := regexp.MustCompile(`data-page="([a-z]+)"`).FindAllStringSubmatch(s, -1)
	if len(used) == 0 {
		t.Fatal("no section of the sheet carries a data-page")
	}
	for _, m := range used {
		if !tabs[m[1]] {
			t.Errorf("data-page=%q has no tab in M_PAGES", m[1])
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
