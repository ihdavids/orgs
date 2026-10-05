package pandoc

import "testing"

func TestFormatNames(t *testing.T) {
	cases := []struct{ asked, writer, ext string; binary bool }{
		{"docx", "docx", "docx", true},
		{"Word", "docx", "docx", true},
		{"epub", "epub3", "epub", true},
		{"md", "gfm", "md", false},
		{"txt", "plain", "txt", false},
		{"adoc", "asciidoc", "adoc", false},
		{"rst", "rst", "rst", false},
	}
	for _, c := range cases {
		if w := Writer(c.asked); w != c.writer {
			t.Errorf("%s: writer %s, want %s", c.asked, w, c.writer)
		}
		if e := Extension(c.asked); e != c.ext {
			t.Errorf("%s: extension %s, want %s", c.asked, e, c.ext)
		}
		if b := IsBinary(c.asked); b != c.binary {
			t.Errorf("%s: binary %v", c.asked, b)
		}
	}
}
