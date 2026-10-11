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
