package text

import (
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
)

func TestPlainText(t *testing.T) {
	doc := org.New().Silent().Parse(strings.NewReader(`#+TITLE: Notes
#+AUTHOR: Sam
* TODO Introduction                                                 :work:
SCHEDULED: <2026-11-03 Tue>
:PROPERTIES:
:ID: x
:END:
This is *bold*, /italic/ and ~code~, with [[https://orgmode.org][a link]] and a very long sentence that keeps on going past the edge of the page so it has to wrap.
** Detail
#+BEGIN_SRC sh
echo hi
#+END_SRC
*** Deep
**** Deeper still
text under it
* Second
`), "notes.org")
	got := (&Exporter{}).Text(doc)
	for _, want := range []string{
		"                                 Notes\n                                 =====\n                                  Sam",
		"1 TODO Introduction  :work:\n===========================",
		"This is *bold*, /italic/ and `code', with a link <https://orgmode.org>\nand a very long",
		"1.1 Detail\n~~~~~~~~~~",
		"  echo hi",
		"1.1.1 Deep\n----------",
		"- Deeper still\n  text under it",
		"2 Second\n========",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	for _, not := range []string{"SCHEDULED", ":ID:", "#+TITLE", "#+BEGIN_SRC"} {
		if strings.Contains(got, not) {
			t.Errorf("%q should be left out:\n%s", not, got)
		}
	}
}
