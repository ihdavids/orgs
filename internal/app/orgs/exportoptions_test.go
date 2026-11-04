package orgs

import (
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
)

func optRun(t *testing.T, opts, body string) string {
	t.Helper()
	paths := loadOrg(t, map[string]string{"o.org": "#+OPTIONS: " + opts + "\n" + body})
	f := GetDb().FindByFile(paths["o.org"])
	lines, _ := readFileLines(paths["o.org"])
	out, _ := exportOptionLines(f, lines)
	return strings.TrimRight(strings.Join(out[1:], "\n"), "\n")
}

const optBody = `* TODO Task [1/2]
SCHEDULED: <2026-11-03 Tue>
:PROPERTIES:
:ID: x
:END:
:LOGBOOK:
CLOCK: [2026-11-02 Mon 10:00]--[2026-11-02 Mon 11:00] =>  1:00
:END:
:NOTES:
kept?
:END:
CLOCK: [2026-11-01 Sun 10:00]--[2026-11-01 Sun 11:00] =>  1:00
- [X] one
- [ ] two
: fixed
| a | b |
#+TBLFM: $2=1
* DONE Old
** Child of old
* Plain
`

func TestExportOptionsInTheCopy(t *testing.T) {
	cases := []struct{ opts, has, hasNot string }{
		{"p:nil", ":PROPERTIES:", "SCHEDULED:"},
		{"c:nil", ":LOGBOOK:", "CLOCK:"},
		{"prop:nil", ":LOGBOOK:", ":PROPERTIES:"},
		{`d:(not "LOGBOOK")`, ":NOTES:", ":LOGBOOK:"},
		{`d:("NOTES")`, ":NOTES:", ":LOGBOOK:"},
		{"d:nil", ":PROPERTIES:", ":NOTES:"},
		{"tasks:nil", "* Plain", "Task"},
		{"tasks:todo", "* TODO Task", "Child of old"},
		{"stat:nil", "* TODO Task\n", "[1/2]"},
		{"::nil", "| a | b |", ": fixed"},
		{"|:nil", ": fixed", "| a | b |"},
	}
	for _, c := range cases {
		got := optRun(t, c.opts, optBody)
		if !strings.Contains(got, c.has) || strings.Contains(got, c.hasNot) {
			t.Errorf("%s: want %q and not %q in\n%s", c.opts, c.has, c.hasNot, got)
		}
	}
	if got := optRun(t, "H:1", "* Top\n** Deep\ntext\n*** Deeper\nmore\n"); got != "* Top\n- Deep\n  text\n  - Deeper\n    more" {
		t.Errorf("H:1:\n%s", got)
	}
	if got := optRun(t, `\n:t`, "one\ntwo\nthree\n\nnext\n"); got != "one \\\\\ntwo \\\\\nthree\n\nnext" {
		t.Errorf("breaks:\n%q", got)
	}
	// Nothing said, nothing done.
	if got := optRun(t, "toc:nil", optBody); got != strings.TrimSuffix(optBody, "\n") {
		t.Errorf("toc:nil changed the text:\n%s", got)
	}
}

func TestEmphasisTimestampAndBrokenLinkOptions(t *testing.T) {
	got := optRun(t, "*:nil", "* A *heading* here\nSome *bold* and /it/ and =code= stay.\n")
	if !strings.Contains(got, "* A \u200b*heading* here") || !strings.Contains(got, "Some \u200b*bold* and \u200b/it/ and =code= stay.") {
		t.Errorf("*:nil:\n%q", got)
	}
	// go-org reads it as text now.
	doc := GetConfig().Silent().Parse(strings.NewReader(got), "x.org")
	if html, _ := doc.Write(org.NewHTMLWriter()); strings.Contains(html, "<strong>") || strings.Contains(html, "<em>") {
		t.Errorf("still emphasis:\n%s", html)
	}
	if got := optRun(t, "timestamp:t", "text\n"); !strings.Contains(got, `<p class="date">Created: `) || !strings.Contains(got, "% Created ") {
		t.Errorf("timestamp:t:\n%s", got)
	}
	body := "* Here\n<<spot>>\nSee [[*Here]], [[*Nowhere][gone]], [[spot]] and [[#nope]].\n"
	if got := optRun(t, "broken-links:mark", body); !strings.Contains(got, "See [[*Here]], [BROKEN LINK: *Nowhere], [[spot]] and [BROKEN LINK: #nope].") {
		t.Errorf("mark:\n%s", got)
	}
	if got := optRun(t, "broken-links:t", body); !strings.Contains(got, "See [[*Here]], gone, [[spot]] and #nope.") {
		t.Errorf("t:\n%s", got)
	}
}
