package lint

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

func TestParagraphsReadOnlyProse(t *testing.T) {
	file := `* Heading words never read
:PROPERTIES:
:ID: abc
:END:
SCHEDULED: <2026-10-23 Fri>
First line, see [[https://x.org/y][the site]] and =code= here.
Second line *bold* end.

#+begin_src go
nope()
#+end_src
#+begin_quote
Quoted words.
#+end_quote
| a | table |
- item one
- [X] item two`
	lines := strings.Split(file, "\n")
	ps := paragraphs(lines, 0, len(lines))
	got := []string{}
	for _, p := range ps {
		got = append(got, string(p.text))
	}
	want := []string{
		"First line, see the site and here. Second line bold end.",
		"Quoted words.",
		"item one",
		"item two",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("paragraphs:\n got %q\nwant %q", got, want)
	}
	// "site" is on line 5 (0 based) at the byte where it is written.
	p := ps[0]
	i := strings.Index(string(p.text), "site")
	if at := p.at[i]; at.line != 5 || at.col != strings.Index(lines[5], "site") {
		t.Fatalf("site at %+v", at)
	}
	// "Second" starts line 6.
	i = strings.Index(string(p.text), "Second")
	if at := p.at[i]; at.line != 6 || at.col != 0 {
		t.Fatalf("Second at %+v", at)
	}
}

// Columns are bytes, and offsets come back in UTF-16 units.
func TestPlaceCountsUTF16AndBytes(t *testing.T) {
	lines := []string{"Café 😀 recieve"}
	ps := paragraphs(lines, 0, 1)
	// Café(4) space(1) 😀(2 units) space(1) = 8 units before "recieve".
	at := ps[0].place(8)
	if at.col != strings.Index(lines[0], "recieve") {
		t.Fatalf("col %d, want %d", at.col, strings.Index(lines[0], "recieve"))
	}
}

// LanguageTool gets paragraphs joined by a blank line; each match has to be
// walked back to the paragraph it fell in.
func TestLanguageToolOffsetsFindTheirParagraph(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		text := r.PostForm.Get("text")
		off := strings.Index(text, "teh")
		fmt.Fprintf(w, `{"matches":[{"message":"typo","offset":%d,"length":3,
			"replacements":[{"value":"the"}],"rule":{"id":"MORFOLOGIK","issueType":"misspelling"}}]}`, off)
	}))
	defer srv.Close()
	core := &commands.Core{Rest: common.Rest{Url: srv.URL, Header: http.Header{}}}
	lines := strings.Split("First paragraph.\n\nSecond has teh typo.", "\n")
	ps := paragraphs(lines, 0, len(lines))
	res, err := runLanguageTool(core, ps, "en-US", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res[0]) != 0 || len(res[1]) != 1 {
		t.Fatalf("matches per paragraph: %d, %d", len(res[0]), len(res[1]))
	}
	if at := res[1][0].at; at.line != 2 || at.col != strings.Index(lines[2], "teh") {
		t.Fatalf("teh at %+v", at)
	}
}
