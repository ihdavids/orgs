package orgs

import (
	"os"
	"testing"

	"github.com/ihdavids/go-org/org"
)

// Every heading's hash in a loaded file, by title (the last one wins for a
// title written twice, which the tests below never ask for).
func hashesByTitle(t *testing.T, path string) map[string]string {
	t.Helper()
	f := GetDb().FindByFile(path)
	if f == nil || f.Doc == nil {
		t.Fatalf("%s is not loaded", path)
	}
	out := map[string]string{}
	var walk func(secs []*org.Section)
	walk = func(secs []*org.Section) {
		for _, s := range secs {
			if s != nil && s.Headline != nil {
				out[sectionTitle(s)] = s.Hash
				walk(s.Children)
			}
		}
	}
	walk(f.Doc.Outline.Section.Children)
	return out
}

func rewriteOrg(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	GetDb().LoadFile(path, true)
}

// The title a hash finds now, or "" for none.
func titleFor(hash string) string {
	if s := GetDb().FindByHash(hash); s != nil {
		return sectionTitle(s)
	}
	return ""
}

func TestStaleHashFindsItsHeading(t *testing.T) {
	paths := loadOrg(t, map[string]string{"h.org": `* Alpha
** Child
* Beta
** Under beta
* Same
* Same
* Gamma
* Moved
:PROPERTIES:
:ID: moved-1
:END:
`})
	p := paths["h.org"]
	before := hashesByTitle(t, p)

	// A heading added at the top changes every hash below it. Each heading
	// whose title is unique among its siblings is found by title; Moved also
	// by its ID.
	rewriteOrg(t, p, `* New first
* Alpha
** Child
* Beta
** Under beta
* Same
* Same
* Moved
:PROPERTIES:
:ID: moved-1
:END:
`)
	after := hashesByTitle(t, p)
	if before["Beta"] == after["Beta"] {
		t.Fatal("adding a heading above did not change Beta's hash; the test proves nothing")
	}
	for _, title := range []string{"Alpha", "Child", "Beta", "Under beta", "Moved"} {
		if got := titleFor(before[title]); got != title {
			t.Errorf("old hash of %q finds %q", title, got)
		}
	}
	// Two siblings called Same: which is which cannot be known.
	if got := titleFor(before["Same"]); got != "" {
		t.Errorf("an ambiguous title was forwarded to %q", got)
	}
	// Gamma was deleted.
	if got := titleFor(before["Gamma"]); got != "" {
		t.Errorf("a deleted heading's hash finds %q", got)
	}

	// Renamed in place (same row, same level), with its child: both forward,
	// and a hash from two edits ago still gets there.
	rewriteOrg(t, p, `* New first
* Alpha renamed
** Child
* Beta
** Under beta
* Same
* Same
* Moved
:PROPERTIES:
:ID: moved-1
:END:
`)
	if got := titleFor(after["Alpha"]); got != "Alpha renamed" {
		t.Errorf("renamed in place: old hash finds %q", got)
	}
	if got := titleFor(after["Child"]); got != "Child" {
		t.Errorf("child of a renamed heading: old hash finds %q", got)
	}
	if got := titleFor(before["Beta"]); got != "Beta" {
		t.Errorf("hash from two edits ago finds %q", got)
	}

	// Moved under another parent: found by its ID.
	mid := hashesByTitle(t, p)
	rewriteOrg(t, p, `* New first
* Alpha renamed
** Child
** Moved
:PROPERTIES:
:ID: moved-1
:END:
* Beta
** Under beta
* Same
* Same
`)
	if got := titleFor(mid["Moved"]); got != "Moved" {
		t.Errorf("moved by ID: old hash finds %q", got)
	}
	// FileFromSection works for what a forwarded hash finds, which a raw
	// ByHashToFile[hash] read would not.
	if s := GetDb().FindByHash(before["Beta"]); s == nil || GetDb().FileFromSection(s) == nil {
		t.Error("no file for a forwarded heading")
	}
}
