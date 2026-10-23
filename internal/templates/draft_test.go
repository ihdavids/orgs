package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnsafeTags(t *testing.T) {
	refused := []string{
		`{% ssi "/etc/passwd" %}`,
		`{% include "/etc/passwd" %}`,
		`{% include "../../secret.txt" %}`,
		`{% include "~/notes.org" %}`,
		`{% include somevar %}`,
		`{%- extends '../base.tpl' -%}`,
	}
	for _, s := range refused {
		if UnsafeTags("x\n"+s) == nil {
			t.Errorf("%s was allowed", s)
		} else if e := UnsafeTags("x\n" + s); e.Line != 2 {
			t.Errorf("%s: line %d, want 2", s, e.Line)
		}
	}
	allowed := []string{
		`{% include "header.tpl" %}`,
		`{% include 'parts/footer.tpl' with x=1 %}`,
		`{{ title }} {% if author %}{{author}}{% endif %}`,
	}
	for _, s := range allowed {
		if e := UnsafeTags(s); e != nil {
			t.Errorf("%s was refused: %s", s, e.Msg)
		}
	}
}

func TestRenderDraft(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "new"), 0o755)
	os.WriteFile(filepath.Join(dir, "new", "head.tpl"), []byte("#+TITLE: {{title}}"), 0o644)
	m := &TemplateManager{}
	path := filepath.Join(dir, "new", "mine.tpl")

	// An include beside it resolves as it will once the draft is saved there.
	out, e := m.RenderDraft(path, "{% include \"head.tpl\" %}\n* {{ basename|upper }}", map[string]any{"title": "Plans", "basename": "plans"})
	if e != nil {
		t.Fatalf("render: %s", e.Msg)
	}
	if out != "#+TITLE: Plans\n* PLANS" {
		t.Errorf("got %q", out)
	}

	// A broken tag says where.
	_, e = m.RenderDraft(path, "#+TITLE: x\n\n{% if title %}\nno end", nil)
	if e == nil || e.Line == 0 {
		t.Fatalf("an unclosed if: %+v", e)
	}
	_, e = m.RenderDraft(path, "a\n{{ title|nosuchfilter }}", nil)
	if e == nil || e.Line != 2 || !strings.Contains(e.Msg, "nosuchfilter") {
		t.Errorf("an unknown filter: %+v", e)
	}
}
