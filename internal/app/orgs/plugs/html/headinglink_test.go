package htmlexp

import (
	"regexp"
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// `[[*Title]]` goes to that heading's anchor, whether the heading comes
// before the link or after it, and whatever case or spacing the link uses.
func TestHeadingLinksGoToTheHeading(t *testing.T) {
	src := "* Intro\nSee [[*Keys][the keys]] and [[*every  KEY binding]].\n" +
		"* TODO [#A] Keys :ref:\nBack to [[*Intro]]. Not there: [[*Nowhere]].\n" +
		"** Every key binding\n"
	doc := org.New().Parse(strings.NewReader(src), "test.org")
	w := NewOrgHtmlWriter(&OrgHtmlExporter{Props: map[string]interface{}{}, pm: &common.PluginManager{}})
	w.SrcDoc = doc
	org.WriteNodes(w, doc.Nodes...)
	out := w.String()

	id := func(title string) string {
		m := regexp.MustCompile(`<div id="([^"]+)" class="heading-wrapper"><div id="[^"]+"[^>]*><h\d[^>]*><span[^>]*></span>(?:<span[^>]*>[^<]*</span> )?` + regexp.QuoteMeta(title)).FindStringSubmatch(out)
		if m == nil {
			t.Fatalf("no heading %q in:\n%s", title, out)
		}
		return m[1]
	}
	for _, want := range []string{
		`<a href="#` + id("Keys") + `" class="heading-link">the keys</a>`,
		`<a href="#` + id("Every key binding") + `" class="heading-link">every  KEY binding</a>`,
		`<a href="#` + id("Intro") + `" class="heading-link">Intro</a>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
	if strings.Contains(out, `href="#`+id("Intro")+`" class="heading-link">Nowhere`) {
		t.Errorf("a link to no heading was resolved")
	}
}

// The links a table of contents writes (toc-org, org-make-toc): github-style
// `[[#anchor]]` (a CUSTOM_ID first, twins numbered), and `[[*Title]]` with a
// link in the title shown as its description and the cookie gone.
func TestTocLinksGoToTheHeading(t *testing.T) {
	src := "* Contents :TOC:\n- [[#about-this][About this]]\n- [[#about-this-1][About this]]\n" +
		"- [[#the-keys][Keys]]\n- [[*Three link here][Three link here]]\n- [[#nowhere][Nowhere]]\n" +
		"* About this [1/2]\n* About this\n* Keys\n:PROPERTIES:\n:CUSTOM_ID: the-keys\n:END:\n" +
		"* Three [[https://x.org][link]] here\n"
	doc := org.New().Parse(strings.NewReader(src), "test.org")
	w := NewOrgHtmlWriter(&OrgHtmlExporter{Props: map[string]interface{}{}, pm: &common.PluginManager{}})
	w.SrcDoc = doc
	org.WriteNodes(w, doc.Nodes...)
	out := w.String()
	ids := regexp.MustCompile(`<div id="([^"]+)" class="heading-wrapper">`).FindAllStringSubmatch(out, -1)
	if len(ids) != 5 {
		t.Fatalf("want 5 headings, got %d in:\n%s", len(ids), out)
	}
	for _, want := range []string{
		`<a href="#` + ids[1][1] + `" class="heading-link">About this</a>`,
		`<a href="#` + ids[2][1] + `" class="heading-link">About this</a>`,
		`<a href="#` + ids[3][1] + `" class="heading-link">Keys</a>`,
		`<a href="#` + ids[4][1] + `" class="heading-link">Three link here</a>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
	if strings.Contains(out, `class="heading-link">Nowhere`) {
		t.Errorf("a link to no heading was resolved")
	}
}
