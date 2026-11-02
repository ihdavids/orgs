package orgs

import (
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
#+transclude: [[file:src.org::*Budget]] :only-contents
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
