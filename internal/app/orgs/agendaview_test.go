package orgs

import (
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

func TestAgendaViewsAnswerEachSection(t *testing.T) {
	now := time.Now()
	today := now.Format("2006-01-02 Mon")
	loadOrg(t, map[string]string{"work.org": `#+CATEGORY: work
#+TODO: TODO NEXT WAITING | DONE
* TODO Write the report                                  :urgent:
SCHEDULED: <` + today + ` 10:00>
* NEXT Call Sam
* WAITING Parts
* DONE Old thing
* Kitchen                                                :project:
Remember the grout.
** TODO Pick tiles
`})
	n, ok := findAgendaView("", "n")
	if !ok {
		t.Fatal("no built-in n")
	}
	res := runAgendaView(n, now)
	if len(res.Parts) != 2 || res.Parts[0].Agenda == nil || len(res.Parts[0].Agenda.Days) != 7 {
		t.Fatalf("n: %+v", res)
	}
	titles := func(p common.AgendaViewPart) string {
		out := []string{}
		for _, x := range p.Nodes {
			out = append(out, x.Headline)
		}
		return strings.Join(out, ",")
	}
	if got := titles(res.Parts[1]); got != "Write the report,Call Sam,Parts,Pick tiles" {
		t.Errorf("open tasks: %s", got)
	}
	v := common.AgendaView{Name: "Mine", Sections: []common.AgendaViewSection{
		{Kind: "todo", Keywords: "NEXT|WAITING"},
		{Kind: "tags", Query: "+urgent"},
		{Kind: "search", Query: "grout"},
		{Kind: "stuck"},
	}}
	res = runAgendaView(v, now)
	want := []string{"Call Sam,Parts", "Write the report", "Kitchen", "Kitchen"}
	for i, w := range want {
		if got := titles(res.Parts[i]); got != w || res.Parts[i].Msg != "" {
			t.Errorf("section %d (%s): %s %s, want %s", i, res.Parts[i].Title, got, res.Parts[i].Msg, w)
		}
	}
	txt := agendaViewText(runAgendaView(common.AgendaView{Name: "Day", Sections: []common.AgendaViewSection{{Kind: "agenda"}, {Kind: "todo", Keywords: "NEXT"}}}, now))
	for _, w := range []string{"Day agenda\n" + now.Format("Monday"), "  work:       10:00 TODO Write the report  :urgent:", "Tasks: NEXT\n  work:       NEXT Call Sam"} {
		if !strings.Contains(txt, w) {
			t.Errorf("no %q in\n%s", w, txt)
		}
	}
	ics := agendaViewICS(runAgendaView(n, now), now)
	if !strings.Contains(ics, "BEGIN:VEVENT") || !strings.Contains(ics, "SUMMARY:Write the report") {
		t.Errorf("ics:\n%s", ics)
	}
	if html := agendaViewHTML(runAgendaView(n, now)); !strings.Contains(html, "<h2>Week agenda</h2>") || !strings.Contains(html, `<div class="day">`) {
		t.Errorf("html:\n%s", html)
	}
}
