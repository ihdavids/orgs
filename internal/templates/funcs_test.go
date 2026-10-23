package templates

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func render(t *testing.T, text string) string {
	t.Helper()
	registerOrgFilters()
	m := &TemplateManager{}
	out, e := m.RenderDraft(filepath.Join(t.TempDir(), "new", "x.tpl"), text, nil)
	if e != nil {
		t.Fatalf("%q: %s", text, e.Msg)
	}
	return out
}

func TestDays(t *testing.T) {
	now := time.Date(2026, 10, 22, 9, 30, 0, 0, time.Local) // a Thursday
	ds := days(now)("mon", 3)
	if len(ds) != 3 {
		t.Fatalf("%d days", len(ds))
	}
	if ds[0].String() != "<2026-10-26 Mon>" || ds[2]["weekday"] != "Wednesday" || ds[1]["date"] != "2026-10-27" {
		t.Errorf("got %v", ds)
	}
	if got := days(now)("", 1); got[0]["date"] != "2026-10-22" {
		t.Errorf("no start is today: %v", got)
	}
	if len(days(now)("today", 100000)) != maxDays {
		t.Error("not capped")
	}
	if got := days(now)("<2026-10-19 Mon>", 2); got[1]["date"] != "2026-10-20" {
		t.Errorf("from a timestamp: %v", got)
	}
	if days(now)("not a date", 3) != nil {
		t.Error("a bad start made days")
	}
	// In a template: each day prints as its timestamp and has its fields.
	out := render(t, `{% for d in days("2026-10-26", 2) %}* {{ d.dayname }} {{ d }}
{% endfor %}`)
	if out != "* Mon <2026-10-26 Mon>\n* Tue <2026-10-27 Tue>\n" {
		t.Errorf("got %q", out)
	}
}

func TestPlusAndFormat(t *testing.T) {
	cases := map[string]string{
		`{{ "<2026-10-22 Thu>"|plus:"2w" }}`:          "<2026-11-05 Thu>",
		`{{ "[2026-10-22 Thu 09:00]"|plus:"90min" }}`: "[2026-10-22 Thu 10:30]",
		`{{ "2026-01-31"|plus:"1m" }}`:                "2026-03-03",
		`{{ "<2026-10-22 Thu +1w>"|plus:"-1d" }}`:     "<2026-10-21 Wed +1w>",
		`{{ "<2026-10-22 Thu>"|format:"weekday" }}`:   "Thursday",
		`{{ "2026-10-22"|plus:"3d"|format:"today" }}`: "<2026-10-25 Sun>",
		`{{ "Q4 Plans: Draft!"|slug }}`:               "q4-plans-draft",
		`{{ "Åsa's notes"|slug }}`:                    "åsa-s-notes",
		`{{ "work, q4 plans, work"|tags }}`:           ":work:q4_plans:",
		`{{ "home errands"|tags }}`:                   ":home:errands:",
		`{{ ""|tags }}`:                               "",
	}
	for in, want := range cases {
		if got := render(t, in); got != want {
			t.Errorf("%s = %q, want %q", in, got, want)
		}
	}
	m := &TemplateManager{}
	for _, bad := range []string{`{{ "soon"|plus:"1d" }}`, `{{ "2026-10-22"|plus:"a while" }}`, `{{ "2026-10-22"|format:"uuid" }}`} {
		if _, e := m.RenderDraft(filepath.Join(t.TempDir(), "x.tpl"), bad, nil); e == nil || !strings.Contains(e.Msg, "") {
			t.Errorf("%s ran", bad)
		}
	}
}

func TestAskFields(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "new"), 0o755)
	os.WriteFile(filepath.Join(dir, "new", "head.tpl"), []byte(`#+CLIENT: {{ ask("Client") }} {{ ask('Rate', "100") }}`), 0o644)
	text := `{{ ask("Project", "Q4") }}
{% include "head.tpl" %}
{{ ask("Client") }} {{ ask("Due date") }} {{ ask("nothing"`
	got := AskFields(text, filepath.Join(dir, "new", "mine.tpl"))
	want := []AskField{{Name: "Project", Default: "Q4"}, {Name: "Client"}, {Name: "Rate", Default: "100"}, {Name: "Due date"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
}
