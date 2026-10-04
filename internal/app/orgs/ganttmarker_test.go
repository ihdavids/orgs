package orgs

import (
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func TestMarkerStampSpellings(t *testing.T) {
	cases := []struct {
		e    common.GanttMarkerEdit
		want string
	}{
		{common.GanttMarkerEdit{Start: common.GanttMarkerDay{Date: "2004-08-23"}}, "<2004-08-23 Mon>"},
		{common.GanttMarkerEdit{Start: common.GanttMarkerDay{Date: "2004-08-23", Time: "10:00"}}, "<2004-08-23 Mon 10:00>"},
		{common.GanttMarkerEdit{Start: common.GanttMarkerDay{Date: "2004-08-23"}, End: &common.GanttMarkerDay{Date: "2004-08-26"}},
			"<2004-08-23 Mon>--<2004-08-26 Thu>"},
		{common.GanttMarkerEdit{
			Start: common.GanttMarkerDay{Date: "2004-08-23", Time: "10:00", Until: "11:00"},
			End:   &common.GanttMarkerDay{Date: "2004-08-26", Time: "10:00", Until: "11:00"}},
			"<2004-08-23 Mon 10:00-11:00>--<2004-08-26 Thu 10:00-11:00>"},
	}
	for _, c := range cases {
		got, err := markerDateText(&c.e)
		if err != nil || got != c.want {
			t.Errorf("got %q (%v), want %q", got, err, c.want)
		}
	}
	bad := common.GanttMarkerEdit{Start: common.GanttMarkerDay{Date: "2004-08-26"}, End: &common.GanttMarkerDay{Date: "2004-08-23"}}
	if _, err := markerDateText(&bad); err == nil {
		t.Error("a range ending before it starts was accepted")
	}
	if _, err := markerDateText(&common.GanttMarkerEdit{Start: common.GanttMarkerDay{Date: "2004-08-23", Time: "10am"}}); err == nil {
		t.Error("a time that is not HH:MM was accepted")
	}
}

func markerRun(src string, kind, date string, isRange bool) string {
	lines := strings.Split(src, "\n")
	own := ownLinesEnd(lines, 0, len(lines)-1)
	return strings.Join(setMarkerDate(lines, 0, own, "   ", kind, date, isRange), "\n")
}

func TestMarkerDateEdits(t *testing.T) {
	cases := []struct {
		name, src, kind, date string
		isRange               bool
		want                  string
	}{
		{"replaces its own stamp", "** Beta  :M1:\n   <2026-10-20 Tue>\n** Next",
			"date", "<2026-10-22 Thu>", false, "** Beta  :M1:\n   <2026-10-22 Thu>\n** Next"},
		{"replaces a whole range", "** Freeze\n<2026-10-14 Wed>--<2026-10-16 Fri>",
			"date", "<2026-10-15 Thu>--<2026-10-19 Mon>", true, "** Freeze\n<2026-10-15 Thu>--<2026-10-19 Mon>"},
		{"a single day moves SCHEDULED", "** Launch\n   SCHEDULED: <2026-10-20 Tue>",
			"date", "<2026-10-21 Wed>", false, "** Launch\n   SCHEDULED: <2026-10-21 Wed>"},
		{"a range takes SCHEDULED's place", "** Launch\n   SCHEDULED: <2026-10-20 Tue>\n   :PROPERTIES:\n   :ID: x\n   :END:",
			"date", "<2026-10-20 Tue>--<2026-10-22 Thu>", true,
			"** Launch\n   :PROPERTIES:\n   :ID: x\n   :END:\n   <2026-10-20 Tue>--<2026-10-22 Thu>"},
		{"a range keeps a DEADLINE beside SCHEDULED", "** Launch\n   SCHEDULED: <2026-10-20 Tue> DEADLINE: <2026-10-30 Fri>",
			"date", "<2026-10-20 Tue>--<2026-10-22 Thu>", true,
			"** Launch\n   DEADLINE: <2026-10-30 Fri>\n   <2026-10-20 Tue>--<2026-10-22 Thu>"},
		{"a stamp in a drawer is not its date", "** Trip\n:PROPERTIES:\n:WHEN: <2026-01-01 Thu>\n:END:",
			"date", "<2026-10-26 Mon>", false, "** Trip\n:PROPERTIES:\n:WHEN: <2026-01-01 Thu>\n:END:\n   <2026-10-26 Mon>"},
		{"deadline replaced in place", "** GA  :M2:\n   SCHEDULED: <2026-10-01 Thu> DEADLINE: <2026-10-28 Wed>",
			"deadline", "<2026-10-29 Thu>", false, "** GA  :M2:\n   SCHEDULED: <2026-10-01 Thu> DEADLINE: <2026-10-29 Thu>"},
		{"deadline added to a planning line", "** GA\n   SCHEDULED: <2026-10-01 Thu>",
			"deadline", "<2026-10-28 Wed>", false, "** GA\n   SCHEDULED: <2026-10-01 Thu> DEADLINE: <2026-10-28 Wed>"},
		{"deadline added with no planning line", "** GA\nsome notes",
			"deadline", "<2026-10-28 Wed>", false, "** GA\n   DEADLINE: <2026-10-28 Wed>\nsome notes"},
		{"a child's stamp is not touched", "** Parent\n*** Child\n    <2026-01-01 Thu>",
			"date", "<2026-10-26 Mon>", false, "** Parent\n   <2026-10-26 Mon>\n*** Child\n    <2026-01-01 Thu>"},
	}
	for _, c := range cases {
		if got := markerRun(c.src, c.kind, c.date, c.isRange); got != c.want {
			t.Errorf("%s:\n got:\n%s\nwant:\n%s", c.name, got, c.want)
		}
	}
}

func TestMarkerHeadlineKeepsKeywordAndPriority(t *testing.T) {
	got := markerHeadline("** TODO [#A] Old name  :a:b:", "TODO", "A", "New name", nil, false)
	if got != "** TODO [#A] New name  :a:b:" {
		t.Errorf("rename lost something: %q", got)
	}
	got = markerHeadline("** Beta  :M1:", "", "", "Beta", []string{"M1", " milestone "}, true)
	if got != "** Beta  :M1:milestone:" {
		t.Errorf("tags not written: %q", got)
	}
}
