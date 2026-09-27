package orgs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

// A block that printed its own org link has already answered the question, and
// wrapping it a second time gives [[file:[[file:plot.png]]]] - a link naming no
// file, which then reads back as a file called "[[file:plot.png". Printing the
// link is what emacs babel puts in the buffer, so it is the common case rather
// than an odd one.
func TestFileResultIsNotWrappedTwice(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"plot.png", "[[file:plot.png]]"},
		{"[[file:plot.png]]", "[[file:plot.png]]"},
		{"file:plot.png", "[[file:plot.png]]"},
		{"  [[file:out/chart.svg]]\n", "[[file:out/chart.svg]]"},
		{"", ""},
	}
	for _, c := range cases {
		if got := babelFormat("file", c.in); got != c.want {
			t.Errorf("babelFormat(file, %q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// And the reader of that link has to get the name back out of it whole.
func TestResultLinkName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"[[file:plot.png]]", "plot.png"},
		{"[[file:out/android-profiling-schedule.mermaid]]", "out/android-profiling-schedule.mermaid"},
		{"nothing here", ""},
		// A double wrap is not written any more, but one read off an old sheet
		// must not come back as a filename with brackets in it - the inner
		// link is the one that names a file.
		{"[[file:[[file:plot.png]]]]", "plot.png"},
	}
	for _, c := range cases {
		got := ""
		if m := resultLinkRe.FindStringSubmatch(c.in); m != nil {
			got = m[1]
		}
		if got != c.want {
			t.Errorf("name of %q = %q, want %q", c.in, got, c.want)
		}
	}
}

// What the client is told to do with the file. The client never guesses at an
// extension - this is the only list, because the html exporter and the kanban
// cards already needed one.
func TestMediaKindOf(t *testing.T) {
	cases := []struct{ name, media, lang string }{
		{"plot.png", "image", ""},
		{"diagram.svg", "image", ""},
		{"take.wav", "audio", ""},
		{"clip.mp4", "video", ""},
		{"report.pdf", "pdf", ""},
		{"schedule.mermaid", "text", "mermaid"},
		{"schedule.mmd", "text", "mermaid"},
		{"timings.json", "text", "json"},
		{"notes.org", "text", "org"},
		{"run.py", "text", "python"},
		// Nothing sensible to say leaves it plain rather than half coloured.
		{"README", "text", ""},
		{"data.bin", "text", ""},
	}
	for _, c := range cases {
		media, lang := mediaKindOf(c.name)
		if media != c.media || lang != c.lang {
			t.Errorf("mediaKindOf(%q) = %q/%q, want %q/%q", c.name, media, lang, c.media, c.lang)
		}
	}
}

// A file the block named is looked for beside the org file first, because that
// is where the block ran - and a name that turns out to be something other than
// text is said to be binary rather than printed into the page.
func TestDescribeResultFile(t *testing.T) {
	dir := t.TempDir()
	org := filepath.Join(dir, "notes.org")
	if err := os.WriteFile(org, []byte("* hi\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "chart.mermaid"), []byte("graph TD\n  a-->b\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Named like text, is not: a NUL in the first 8k.
	if err := os.WriteFile(filepath.Join(dir, "odd.json"), []byte("{\x00\x00}"), 0600); err != nil {
		t.Fatal(err)
	}

	res := common.CodeResult{Result: "[[file:chart.mermaid]]"}
	describeResultFile(&res, org)
	if res.File != "chart.mermaid" || !res.Exists || res.Media != "text" || res.TextLang != "mermaid" {
		t.Fatalf("mermaid: %+v", res)
	}
	// The trailing newline is taken off, the way it is off every other result:
	// a box whose last line is blank looks like a file with something missing.
	if res.Text != "graph TD\n  a-->b" {
		t.Errorf("contents = %q", res.Text)
	}
	if res.Bytes != 17 {
		t.Errorf("bytes = %d, want 17", res.Bytes)
	}

	res = common.CodeResult{Result: "[[file:odd.json]]"}
	describeResultFile(&res, org)
	if res.Media != "binary" || res.Text != "" {
		t.Errorf("a file named like text and full of bytes should be binary and unprinted: %+v", res)
	}

	// A block can name a file it never wrote. Saying so is the whole point of
	// looking: the link on its own reads like an answer.
	res = common.CodeResult{Result: "[[file:never-written.mermaid]]"}
	describeResultFile(&res, org)
	if res.Exists || res.File != "never-written.mermaid" {
		t.Errorf("missing file: %+v", res)
	}

	// Not a file link at all - nothing is touched.
	res = common.CodeResult{Result: "42"}
	describeResultFile(&res, org)
	if res.File != "" || res.Media != "" {
		t.Errorf("plain result: %+v", res)
	}
}
