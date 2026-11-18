package orgs

import (
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// A drawer written in column zero (what Emacs writes) keeps the rest of its
// heading under it: a link after the drawer belongs to that heading, an id in
// it names that heading, and a link above the first heading belongs to the
// file.
func TestLinksPastColumnZeroDrawers(t *testing.T) {
	src := "See [[https://file.example]].\n* One\n:PROPERTIES:\n:ID: one-id\n:CUSTOM_ID: first\n:END:\nRead [[https://one.example]].\n** Two\n:PROPERTIES:\n:ID: two-id\n:END:\n[[id:one-id][back]]\n"
	doc := org.New().Parse(strings.NewReader(src), "notes.org")
	f := &common.OrgFile{Filename: "notes.org", Doc: doc}

	owner := map[string]string{}
	for _, l := range collectFileLinks(f) {
		owner[l.raw] = ""
		if l.sec != nil {
			owner[l.raw] = sectionTitle(l.sec)
		}
	}
	for raw, want := range map[string]string{"https://file.example": "", "https://one.example": "One", "id:one-id": "Two"} {
		if got, ok := owner[raw]; !ok || got != want {
			t.Errorf("%s: under %q (found %v), want %q", raw, got, ok, want)
		}
	}

	ids := newIdIndex()
	ids.addFile(f, flattenSections(f))
	if r, ok := ids.findId("two-id"); !ok || sectionTitle(r.sec) != "Two" {
		t.Errorf("two-id: %v %v", ok, r.sec)
	}
	if r, ok := ids.findCustomId(f, "#first"); !ok || sectionTitle(r.sec) != "One" {
		t.Errorf("custom id: %v", ok)
	}
}
