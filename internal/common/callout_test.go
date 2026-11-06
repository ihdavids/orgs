package common

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common/mdorg"
)

type calloutCases struct {
	Lines []struct {
		Line string
		Want *Callout
	}
	Files []struct {
		Text string
		Want []Callout
	}
}

func readCalloutCases(t *testing.T) calloutCases {
	b, err := os.ReadFile("testdata/callout_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cs calloutCases
	if err := json.Unmarshal(b, &cs); err != nil {
		t.Fatal(err)
	}
	return cs
}

// The same cases are run by worg's callouts.test.ts and vscode's
// callouts-model.test.ts.
func TestCalloutCases(t *testing.T) {
	cs := readCalloutCases(t)
	for _, c := range cs.Lines {
		got, ok := ParseCalloutLine(c.Line)
		if c.Want == nil {
			if ok {
				t.Errorf("%q: a callout %+v, want none", c.Line, got)
			}
			continue
		}
		if !ok || got != *c.Want {
			t.Errorf("%q:\n got %+v (%v)\nwant %+v", c.Line, got, ok, *c.Want)
		}
	}
	for _, f := range cs.Files {
		got := CalloutsInLines(strings.Split(f.Text, "\n"))
		if len(got) != len(f.Want) {
			t.Fatalf("file: %d callouts, want %d: %+v", len(got), len(f.Want), got)
		}
		for i, w := range f.Want {
			g := got[i]
			if g.Type != w.Type || g.Line != w.Line || g.EndLine != w.EndLine || g.Depth != w.Depth {
				t.Errorf("file callout %d: got %+v want %+v", i, g, w)
			}
		}
	}
}

// What go-org makes of the block must read as the line does: the writers
// see the block, the endpoints and clients the line.
func TestCalloutOfBlockAgreesWithTheLine(t *testing.T) {
	for _, c := range readCalloutCases(t).Lines {
		if c.Want == nil || strings.Contains(c.Line, "begin:") {
			continue
		}
		name := strings.Fields(strings.TrimSpace(c.Line))[0][len("#+begin_"):]
		text := strings.TrimSpace(c.Line) + "\nbody\n#+end_" + name + "\n"
		d := org.New().Silent().Parse(strings.NewReader(text), "x.org")
		var b *org.Block
		for _, n := range d.Nodes {
			if v, ok := n.(*org.Block); ok {
				b = v
			}
		}
		if b == nil {
			t.Errorf("%q: go-org made no block of it: %#v", c.Line, d.Nodes)
			continue
		}
		got, ok := CalloutOfBlock(*b)
		if !ok || got != *c.Want {
			t.Errorf("%q from the block:\n got %+v (%v)\nwant %+v", c.Line, got, ok, *c.Want)
		}
	}
}

// mdorg keeps its own list of the kinds (it does not import go-org).
func TestMdorgCalloutKindsAreTheTable(t *testing.T) {
	want := map[string]bool{}
	for _, ty := range CalloutTypes {
		want[ty.Name] = true
		for _, a := range ty.Aliases {
			want[a] = true
		}
	}
	for k := range want {
		if !mdorg.CalloutKinds[k] {
			t.Errorf("mdorg.CalloutKinds lacks %q", k)
		}
	}
	for k := range mdorg.CalloutKinds {
		if !want[k] {
			t.Errorf("mdorg.CalloutKinds has %q, which the table does not", k)
		}
	}
}

// What `orgs import` writes for a markdown callout is read back as the
// same callout.
func TestImportedCalloutsReadBack(t *testing.T) {
	for _, c := range []struct {
		md         string
		ty, title  string
		fold, kind string
	}{
		{"> [!warning] Careful\n> Hot.", "warning", "Careful", "", ""},
		{"> [!faq]- Why?\n> Because.", "question", "Why?", "closed", ""},
		{"> [!example]+\n> one", "example", "", "open", ""},
		{"> [!quote]\n> words", "quote", "", "", ""},
		{"> [!recipe] Soup\n> Water.", "note", "Soup", "", "recipe"},
		{"> [!tip] Ratio 1 :2\n> x", "tip", "Ratio 1\u00a0:2", "", ""},
		{"> [!tip] :colon\n> x", "tip", ":colon", "", ""},
	} {
		text := mdorg.ConvertText("n.md", c.md)
		var begin string
		for _, l := range strings.Split(text, "\n") {
			if strings.HasPrefix(l, "#+begin_") {
				begin = l
				break
			}
		}
		got, ok := ParseCalloutLine(begin)
		if !ok || got.Type != c.ty || got.Title != c.title || got.Fold != c.fold || got.Unknown != c.kind {
			t.Errorf("%q -> %q -> %+v (%v)", c.md, begin, got, ok)
		}
	}
}
