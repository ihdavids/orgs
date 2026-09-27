package orgs

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSearchOneFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.org")
	body := "* Plans\n  buy milk\n  BUY BREAD\n  nothing here\n  buy eggs\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}

	hits, err := searchOneFile(path, regexp.MustCompile(`buy`), 40)
	if err != nil {
		t.Fatal(err)
	}
	if hits.Count != 2 || len(hits.Matches) != 2 {
		t.Fatalf("case sensitive: %+v", hits)
	}
	// Lines are counted from zero, the way the source view wants them.
	if hits.Matches[0].Line != 1 || hits.Matches[1].Line != 4 {
		t.Errorf("lines = %d, %d; want 1, 4", hits.Matches[0].Line, hits.Matches[1].Line)
	}
	if hits.Matches[0].Text != "  buy milk" {
		t.Errorf("text = %q", hits.Matches[0].Text)
	}
	// The offsets have to find the match in the text that was sent.
	m := hits.Matches[0]
	if m.Text[m.Start:m.End] != "buy" {
		t.Errorf("offsets pick out %q", m.Text[m.Start:m.End])
	}

	hits, _ = searchOneFile(path, regexp.MustCompile(`(?i)buy`), 40)
	if hits.Count != 3 {
		t.Errorf("case insensitive found %d, want 3", hits.Count)
	}

	// The cap is on the lines kept, never on the count: an answer that said it
	// found two when it found three would be worse than one that found none.
	hits, _ = searchOneFile(path, regexp.MustCompile(`(?i)buy`), 2)
	if hits.Count != 3 || len(hits.Matches) != 2 || !hits.Truncated {
		t.Errorf("capped: %+v", hits)
	}
}

// A single very long line - a pasted image, a minified blob - used to stop the
// scan dead and silently lose every match after it.
func TestSearchPastAVeryLongLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.org")
	body := "first\n" + strings.Repeat("x", 200*1024) + "\nfindme\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	hits, err := searchOneFile(path, regexp.MustCompile(`findme`), 40)
	if err != nil {
		t.Fatal(err)
	}
	if hits.Count != 1 || hits.Matches[0].Line != 2 {
		t.Fatalf("%+v", hits)
	}
}

func TestTrimAround(t *testing.T) {
	// A short line is sent exactly as it is, which is nearly every line.
	text, s, e := trimAround("  buy milk", 2, 5)
	if text != "  buy milk" || s != 2 || e != 5 {
		t.Errorf("short line: %q %d %d", text, s, e)
	}

	long := strings.Repeat("a", 500) + "NEEDLE" + strings.Repeat("b", 500)
	text, s, e = trimAround(long, 500, 506)
	if len(text) > lineWindow+8 {
		t.Errorf("window is %d long", len(text))
	}
	if text[s:e] != "NEEDLE" {
		t.Errorf("offsets pick out %q", text[s:e])
	}
	if !strings.HasPrefix(text, "…") || !strings.HasSuffix(text, "…") {
		t.Errorf("a trimmed line should say so: %q", text[:4])
	}

	// A match longer than the window is never cut in half.
	huge := strings.Repeat("c", 100) + strings.Repeat("N", 400)
	text, s, e = trimAround(huge, 100, 500)
	if text[s:e] != strings.Repeat("N", 400) {
		t.Errorf("the match was cut: %d..%d of %d", s, e, len(text))
	}
}
