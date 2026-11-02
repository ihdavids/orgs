package orgs

import (
	"os"
	"path/filepath"
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
