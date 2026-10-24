package help

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/docex"
)

// The embedded docs.json must be what the sources say now, or `orgs help`
// shows last month's documentation. Run go generate in this folder (or
// tools/builddocs.sh) to write it again.
func TestDocsBundleIsCurrent(t *testing.T) {
	want, err := docex.BundleJSON("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, docsJSON) {
		t.Fatalf("cmd/oc/commands/help/docs.json is out of date: run `go generate ./cmd/oc/commands/help/` (%d bytes now, %d embedded)", len(want), len(docsJSON))
	}
}

func TestInline(t *testing.T) {
	cases := []struct {
		in   string
		want []span
	}{
		{"run =orgs search= now", []span{{spText, "run "}, {spCode, "orgs search"}, {spText, " now"}}},
		{"a *bold* word, ~code~.", []span{{spText, "a "}, {spBold, "bold"}, {spText, " word, "}, {spCode, "code"}, {spText, "."}}},
		{"see [[https://x.org][the site]] or [[file:a.png]]", []span{{spText, "see the site or a.png"}}},
		{"x=1 and y=2 stay", []span{{spText, "x=1 and y=2 stay"}}},
		{"2*3*4 is not bold", []span{{spText, "2*3*4 is not bold"}}},
	}
	for _, c := range cases {
		got := inline(c.in)
		if len(got) != len(c.want) {
			t.Errorf("%q: got %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q: got %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}

func TestRoffEscapesAndHeadings(t *testing.T) {
	out := tidyRoff(renderRoff("* Asking: =orgs search=\n\n  .dot first -flag \\ back\n\n  - one\n    more\n  - two\n", 1))
	for _, want := range []string{
		`.SH "ASKING: \fBorgs search\fR"`,
		`\&.dot first \-flag \e back`,
		".IP \\(bu 2\none more\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("roff lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, ".SH \"ASKING: \\fBorgs search\\fR\"\n.PP") {
		t.Errorf("a paragraph break straight after a heading:\n%s", out)
	}
}
