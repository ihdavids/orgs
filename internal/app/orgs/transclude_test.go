package orgs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func transclusionOnFor(t *testing.T) {
	t.Helper()
	if Conf().Server == nil {
		t.Skip("no server settings")
	}
	old := Conf().Server.Transclusion
	Conf().Server.Transclusion.Enable = true
	t.Cleanup(func() { Conf().Server.Transclusion = old })
}

// Lines without the marks, for comparing.
func unmarked(lines []string) string {
	out := []string{}
	skip := false
	for _, l := range lines {
		for _, m := range strings.Split(l, "\n") {
			if strings.HasPrefix(m, "#+BEGIN_EXPORT html") {
				skip = true
				continue
			}
			if skip {
				if strings.HasPrefix(m, "#+END_EXPORT") {
					skip = false
				}
				continue
			}
			out = append(out, m)
		}
	}
	return strings.Join(out, "\n")
}

const txSource = `* Projects
** Budget
:PROPERTIES:
:ID: budget-1
:CUSTOM_ID: budget
:END:
Spend less.
*** Rent
Too much.

** Other
Not this.
`

func TestTransclusionExpandsHeadingsAtTheirNewLevel(t *testing.T) {
	transclusionOnFor(t)
	paths := loadOrg(t, map[string]string{
		"src.org": txSource,
		"main.org": `* Plan
#+transclude: [[id:budget-1]]
* By name
#+transclude: [[file:src.org::*Budget]] :no-first-heading
* Lines
#+transclude: [[file:src.org]] :lines 7-8 :src text
`,
	})
	lines, _ := readFileLines(paths["main.org"])
	out, txs := expandTransclusions(paths["main.org"], lines, true)
	want := `* Plan
** Budget
:PROPERTIES:
:ID: budget-1
:CUSTOM_ID: budget
:END:
Spend less.
*** Rent
Too much.
* By name
Spend less.
** Rent
Too much.
* Lines
#+BEGIN_SRC text
Spend less.
*** Rent
#+END_SRC
`
	if got := unmarked(out); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if len(txs) != 3 || txs[0].File != paths["src.org"] || txs[0].From != 1 || txs[0].To != 8 || txs[0].Title != "Budget" {
		t.Errorf("transclusions: %+v", txs)
	}
	if !strings.Contains(strings.Join(out, "\n"), `data-tx="0" data-edge="start"`) {
		t.Errorf("no marks:\n%s", strings.Join(out, "\n"))
	}
}

func TestTransclusionLevelOptionAndNesting(t *testing.T) {
	transclusionOnFor(t)
	paths := loadOrg(t, map[string]string{
		"a.org": "* A\n:PROPERTIES:\n:ID: a-1\n:END:\n#+transclude: [[id:b-1]]\n",
		"b.org": "* B\n:PROPERTIES:\n:ID: b-1\n:END:\nfrom b\n#+transclude: [[id:a-1]]\n",
		"m.org": "#+transclude: [[id:a-1]] :level 3\n#+transclude: [[id:nope]]\n",
	})
	lines, _ := readFileLines(paths["m.org"])
	out, txs := expandTransclusions(paths["m.org"], lines, false)
	got := strings.Join(out, "\n")
	for _, want := range []string{"*** A", "**** B", "from b", "transcludes itself", ": transclusion: no heading has the id nope"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if len(txs) != 2 || txs[1].Error == "" {
		t.Errorf("transclusions: %+v", txs)
	}
}

func TestTransclusionIsOffUnlessTurnedOn(t *testing.T) {
	if Conf().Server == nil {
		t.Skip("no server settings")
	}
	old := Conf().Server.Transclusion
	Conf().Server.Transclusion.Enable = false
	t.Cleanup(func() { Conf().Server.Transclusion = old })
	paths := loadOrg(t, map[string]string{"src.org": txSource, "m.org": "* M\n#+transclude: [[id:budget-1]]\n"})
	if f := exportFile(paths["m.org"]); f != GetDb().FindByFile(paths["m.org"]) {
		t.Errorf("an export expanded a transclusion with transclusion off")
	}
	if res := SaveTransclusion(&TransclusionSave{File: paths["src.org"], From: 0, To: 0, Old: "* Projects", New: "* X"}); res.Ok {
		t.Errorf("a save went through with transclusion off")
	}
}

func TestExportsReadTheTranscludedText(t *testing.T) {
	transclusionOnFor(t)
	paths := loadOrg(t, map[string]string{"src.org": txSource, "m.org": "* M\n#+transclude: [[id:budget-1]]\n"})
	f, stamp := exportFileStamped(paths["m.org"])
	if f == nil || stamp == "" {
		t.Fatalf("no transcluded file: %v %q", f, stamp)
	}
	if len(f.Doc.Outline.Children) != 1 || len(f.Doc.Outline.Children[0].Children) != 1 {
		t.Fatalf("outline: %+v", f.Doc.Outline.Children)
	}
	h := f.Doc.Outline.Children[0].Children[0].Headline
	if h.Lvl != 2 || len(h.Title) == 0 || h.Title[0].String() != "Budget" {
		t.Errorf("heading: level %d %v", h.Lvl, h.Title)
	}
}

func TestAnEditGoesBackToTheSourceRows(t *testing.T) {
	transclusionOnFor(t)
	paths := loadOrg(t, map[string]string{"src.org": txSource, "m.org": "* M\n#+transclude: [[id:budget-1]]\n"})
	lines, _ := readFileLines(paths["m.org"])
	_, txs := expandTransclusions(paths["m.org"], lines, false)
	tx := txs[0]
	edited := strings.Replace(tx.Text, "Spend less.", "Spend much less.\nAnd save.", 1)
	if res := SaveTransclusion(&TransclusionSave{File: tx.File, From: tx.From, To: tx.To, Old: tx.Text, New: edited}); !res.Ok {
		t.Fatalf("save: %s", res.Msg)
	}
	want := strings.Replace(txSource, "Spend less.", "Spend much less.\nAnd save.", 1)
	wantOrg(t, "source", readOrg(t, paths["src.org"]), want)
	// The same edit again was made from text that is no longer there.
	if res := SaveTransclusion(&TransclusionSave{File: tx.File, From: tx.From, To: tx.To, Old: tx.Text, New: edited}); res.Ok {
		t.Errorf("a stale edit was written")
	}
	// Nothing outside the org files.
	if res := SaveTransclusion(&TransclusionSave{File: "/etc/hosts", From: 0, To: 0, Old: "", New: "x"}); res.Ok {
		t.Errorf("wrote outside the org files")
	}
}

func TestTransclusionOptionsAreRead(t *testing.T) {
	o := parseTxOpts(`:level :only-contents :exclude-elements "drawer keyword" :src go :rest ":exports both" :end "say \"x\"" :expand-links :disable-auto`)
	if o.level != 0 || !o.onlyContents || !o.exclude["drawer"] || !o.exclude["keyword"] || o.src != "go" ||
		o.rest != ":exports both" || o.end != `say "x"` || !o.expandLinks {
		t.Errorf("options: %+v", o)
	}
	if o := parseTxOpts(":level auto :no-first-heading :lines 3-"); o.level != 0 || !o.noFirstHeading || o.linesFrom != 3 || o.linesTo != 0 {
		t.Errorf("options: %+v", o)
	}
	if o := parseTxOpts(":level 2 :lines -4"); o.level != 2 || o.linesFrom != 0 || o.linesTo != 4 {
		t.Errorf("options: %+v", o)
	}
}

func TestTransclusionHeadingOptions(t *testing.T) {
	transclusionOnFor(t)
	paths := loadOrg(t, map[string]string{
		"src.org": txSource,
		"m.org": `* Only
#+transclude: [[id:budget-1]] :only-contents
* No first
#+transclude: [[id:budget-1]] :no-first-heading :level 3
* Bare level
#+transclude: [[id:budget-1]] :level :exclude-elements "property-drawer"
`,
	})
	lines, _ := readFileLines(paths["m.org"])
	out, _ := expandTransclusions(paths["m.org"], lines, false)
	want := `* Only
Spend less.
Too much.
* No first
Spend less.
*** Rent
Too much.
* Bare level
** Budget
Spend less.
*** Rent
Too much.
`
	if got := strings.Join(out, "\n"); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestTransclusionExcludeElements(t *testing.T) {
	transclusionOnFor(t)
	paths := loadOrg(t, map[string]string{
		"src.org": `* Log
SCHEDULED: <2026-11-03 Tue>
:LOGBOOK:
CLOCK: [2026-11-02 Mon 10:00]--[2026-11-02 Mon 11:00] =>  1:00
:END:
#+FILETAGS: x
#+NAME: tbl
| a | b |
#+TBLFM: $2=$1
# a comment
: fixed
#+BEGIN_SRC sh
echo hi
#+END_SRC
#+BEGIN_QUOTE
Said.
#+END_QUOTE
Text.
`,
		"m.org": `* M
#+transclude: [[file:src.org::*Log]] :exclude-elements "planning drawer keyword table comment fixed-width src-block"
`,
	})
	lines, _ := readFileLines(paths["m.org"])
	out, _ := expandTransclusions(paths["m.org"], lines, false)
	want := `* M
** Log
#+NAME: tbl
#+BEGIN_QUOTE
Said.
#+END_QUOTE
Text.
`
	if got := strings.Join(out, "\n"); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestTransclusionLinesEndAndSearch(t *testing.T) {
	transclusionOnFor(t)
	paths := loadOrg(t, map[string]string{
		"m.org": `* M
#+transclude: [[file:main.go::func main]] :end "}" :src go :rest ":exports both"
#+transclude: [[file:main.go::3]] :lines -1
#+transclude: [[file:main.go]] :lines 5- :end "}"
#+transclude: [[file:main.go]] :end "nowhere"
`,
	})
	// Not an org file, so it is read for being under the org directories.
	dir := filepath.Dir(paths["m.org"])
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n\nfunc other() {}\n"), 0644)
	oldDirs := Conf().Server.OrgDirs
	Conf().Server.OrgDirs = []string{dir}
	t.Cleanup(func() { Conf().Server.OrgDirs = oldDirs })
	lines, _ := readFileLines(paths["m.org"])
	out, txs := expandTransclusions(paths["m.org"], lines, false)
	got := strings.Join(out, "\n")
	want := "* M\n#+BEGIN_SRC go :exports both\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n#+END_SRC\n" +
		"import \"fmt\"\n" +
		"func main() {\n\tfmt.Println(\"hi\")\n}\n" +
		": transclusion: [[file:main.go]]: :end \"nowhere\" is not found\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if len(txs) != 4 || txs[0].From != 4 || txs[0].To != 6 {
		t.Errorf("transclusions: %+v", txs)
	}
}

func TestTransclusionExpandLinks(t *testing.T) {
	transclusionOnFor(t)
	paths := loadOrg(t, map[string]string{
		"sub/src.org": "* S\n[[file:pic.png]] [[./other.org::*H][there]] [[https://x.org]] [[file:/abs.png]]\n#+BEGIN_SRC org\n[[file:pic.png]]\n#+END_SRC\n",
		"m.org":       "* M\n#+transclude: [[file:sub/src.org::*S]] :expand-links :only-contents\n",
	})
	lines, _ := readFileLines(paths["m.org"])
	out, _ := expandTransclusions(paths["m.org"], lines, false)
	dir := filepath.Dir(paths["sub/src.org"])
	want := "* M\n[[file:" + filepath.Join(dir, "pic.png") + "]] [[" + filepath.Join(dir, "other.org") + "::*H][there]] [[https://x.org]] [[file:/abs.png]]\n" +
		"#+BEGIN_SRC org\n[[file:pic.png]]\n#+END_SRC\n"
	if got := strings.Join(out, "\n"); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
