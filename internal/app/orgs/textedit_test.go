package orgs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func TestADateIsSetInTextUnderTheHeadingHoldingTheLine(t *testing.T) {
	text := "* TODO Call\nSome notes.\n* Other\n"
	res := ChangeDateInText(&common.TextDateChange{Text: text, Line: 1, Name: "scheduled", Value: "<2026-11-02 Mon>"})
	if !res.Ok || !res.Changed {
		t.Fatalf("not changed: %+v", res)
	}
	want := "* TODO Call\n  SCHEDULED: <2026-11-02 Mon>\nSome notes.\n* Other\n"
	if res.Text != want {
		t.Errorf("got\n%q\nwant\n%q", res.Text, want)
	}
	// Again with the same date: nothing to change.
	if again := ChangeDateInText(&common.TextDateChange{Text: res.Text, Line: 0, Name: "SCHEDULED", Value: "<2026-11-02 Mon>"}); again.Changed {
		t.Errorf("the same date changed the text: %q", again.Text)
	}
	// Cleared.
	cleared := ChangeDateInText(&common.TextDateChange{Text: res.Text, Line: 2, Name: "SCHEDULED"})
	if cleared.Text != text {
		t.Errorf("cleared:\n%q", cleared.Text)
	}
	if res := ChangeDateInText(&common.TextDateChange{Text: "no heading\n", Line: 0, Name: "DEADLINE", Value: "2026-11-02"}); res.Ok {
		t.Errorf("a line under no heading was dated: %+v", res)
	}
}

// The block runs on the text sent, not on the file: the file on disk is
// untouched and the answer carries the result.
func TestABlockRunsInTextAndTheFileIsLeftAlone(t *testing.T) {
	need(t, "bash")
	babelOn(t, nil)
	dir := t.TempDir()
	path := filepath.Join(dir, "r.org")
	onDisk := "* Heading\n"
	if err := os.WriteFile(path, []byte(onDisk), 0644); err != nil {
		t.Fatal(err)
	}
	text := "* Heading\n#+BEGIN_SRC sh\necho unsaved\n#+END_SRC\n"
	res := RunInText(&common.TextRun{Filename: path, Text: text, Line: 2})
	if !res.Ok || !res.Changed {
		t.Fatalf("did not run: %+v", res)
	}
	want := text + "\n#+RESULTS:\n: unsaved\n"
	if res.Text != want {
		t.Errorf("got\n%q\nwant\n%q", res.Text, want)
	}
	if b, _ := os.ReadFile(path); string(b) != onDisk {
		t.Errorf("the file was written: %q", b)
	}
	if res := RunInText(&common.TextRun{Filename: path, Text: text, Line: 0}); res.Ok {
		t.Errorf("a heading line ran: %+v", res)
	}
}

// A clock table refreshed in a buffer reads the buffer's clocks, not the
// file's: the file on disk has none.
func TestADynamicBlockIsRefreshedInText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "d.org")
	if err := os.WriteFile(path, []byte("* Work\n"), 0644); err != nil {
		t.Fatal(err)
	}
	text := "#+BEGIN: clocktable :scope file :block untilnow\n#+END:\n* Work\n  :LOGBOOK:\n  CLOCK: [2026-10-01 Thu 09:00]--[2026-10-01 Thu 10:30] =>  1:30\n  :END:\n" +
		"#+BEGIN: insertdatetime\nold\n#+END:\n"
	res := DblockInText(&common.TextRun{Filename: path, Text: text, Line: 0})
	if !res.Ok || !res.Changed || len(res.Blocks) != 1 {
		t.Fatalf("not refreshed: %+v", res)
	}
	if !strings.Contains(res.Text, "Work") || !strings.Contains(res.Text, "1:30") || !strings.Contains(res.Text, "\nold\n") {
		t.Errorf("only the clock table should have changed, got\n%s", res.Text)
	}
	all := DblockInText(&common.TextRun{Filename: path, Text: text, Line: -1})
	if !all.Ok || len(all.Blocks) != 2 || strings.Contains(all.Text, "\nold\n") {
		t.Errorf("Line -1 should refresh both: %+v", all)
	}
	if again := DblockInText(&common.TextRun{Filename: path, Text: res.Text, Line: 0}); !again.Ok || again.Changed {
		t.Errorf("a fresh table changed again: %+v", again)
	}
	if bad := DblockInText(&common.TextRun{Filename: path, Text: text, Line: 2}); bad.Ok {
		t.Errorf("a heading line is not a block: %+v", bad)
	}
	if b, _ := os.ReadFile(path); string(b) != "* Work\n" {
		t.Errorf("the file was written: %q", b)
	}
}

func TestDiagnosticsOfText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.org")
	text := strings.Join([]string{
		"* Heading",                              // 0
		"🙂 see [[*Nowhere]] and [[*Heading]]",    // 1
		"[[file:missing.org]] [[id:no-such-id]]", // 2
		":LOGBOOK:",                              // 3
		"* Other",                                // 4
		"#+BEGIN_SRC sh",                         // 5
		"echo",                                   // 6
	}, "\n") + "\n"
	got := DiagnoseText(&common.TextDiagnose{Filename: path, Text: text})
	byCode := map[string][]common.TextDiagnostic{}
	for _, d := range got {
		byCode[d.Code] = append(byCode[d.Code], d)
	}
	if b := byCode["unclosed-block"]; len(b) != 1 || b[0].Line != 5 || b[0].Severity != "error" || b[0].To != 14 {
		t.Errorf("unclosed block: %+v", b)
	}
	if d := byCode["unclosed-drawer"]; len(d) != 1 || d[0].Line != 3 {
		t.Errorf("unclosed drawer: %+v", d)
	}
	links := byCode["broken-link"]
	if len(links) != 3 {
		t.Fatalf("want 3 broken links, got %+v", links)
	}
	// The emoji is 4 bytes and 2 UTF-16 units: the link starts at unit 7.
	if l := links[0]; l.Line != 1 || l.From != 7 || l.To != 7+len("[[*Nowhere]]") {
		t.Errorf("columns are not UTF-16: %+v", l)
	}
	if links[1].Line != 2 || !strings.Contains(links[1].Message, "no file") || !strings.Contains(links[2].Message, "no-such-id") {
		t.Errorf("file and id links: %+v", links[1:])
	}
}
