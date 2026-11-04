package orgs

import (
	"strings"
	"testing"
	"time"
)

func sexpOn(t *testing.T, expr string, y int, m time.Month, d int) (bool, int) {
	t.Helper()
	x, err := readSexp(expr)
	if err != nil {
		t.Fatalf("%s: %v", expr, err)
	}
	ok, n, err := evalSexp(x, dayOfNum(agendaDayYMD(y, m, d)), "american")
	if err != nil {
		t.Fatalf("%s: %v", expr, err)
	}
	return ok, n
}

func TestDiarySexps(t *testing.T) {
	cases := []struct {
		expr    string
		y       int
		m       time.Month
		d       int
		want    bool
		counted int
	}{
		{"(diary-float t 4 2)", 2026, 11, 12, true, 0},     // second Thursday
		{"(diary-float t 4 2)", 2026, 11, 5, false, 0},     // the first
		{"(diary-float 11 4 -1)", 2026, 11, 26, true, 0},   // last Thursday of November
		{"(diary-float 11 4 -1)", 2026, 11, 19, false, 0},
		{"(diary-anniversary 10 31 1948)", 2026, 10, 31, true, 78},
		{"(org-anniversary 1948 10 31)", 2026, 10, 31, true, 78},
		{"(org-anniversary 2000 2 29)", 2026, 2, 28, true, 26}, // a common year
		{"(diary-block 11 1 2026 11 3 2026)", 2026, 11, 3, true, 0},
		{"(org-block 2026 11 1 2026 11 3)", 2026, 11, 4, false, 0},
		{"(diary-cyclic 14 11 2 2026)", 2026, 11, 16, true, 1},
		{"(diary-cyclic 14 11 2 2026)", 2026, 11, 15, false, 0},
		{"(org-class 2026 9 1 2026 12 20 2 45)", 2026, 11, 3, false, 0}, // a Tuesday of ISO week 45, skipped
		{"(org-class 2026 9 1 2026 12 20 2 45)", 2026, 11, 10, true, 0}, // week 46
		{"(org-class 2026 9 1 2026 12 20 2 45)", 2026, 11, 11, false, 0}, // a Wednesday
		{"(memq (calendar-day-of-week date) '(1 2 3 4 5))", 2026, 11, 7, false, 0},
		{"(and (diary-date t t 2026) (= (calendar-day-of-week date) 2))", 2026, 11, 3, true, 0},
		{"(diary-remind '(diary-date 11 10 2026) 3)", 2026, 11, 8, true, 0},
	}
	for _, c := range cases {
		ok, n := sexpOn(t, c.expr, c.y, c.m, c.d)
		if ok != c.want || (ok && n != c.counted) {
			t.Errorf("%s on %d-%d-%d: %v %d, want %v %d", c.expr, c.y, c.m, c.d, ok, n, c.want, c.counted)
		}
	}
	if _, err := readSexp("(diary-float t 4"); err == nil {
		t.Errorf("an unclosed expression read")
	}
}

func TestSexpDatesLandOnTheAgenda(t *testing.T) {
	loadOrg(t, map[string]string{"s.org": "* Book club <%%(diary-float t 4 2)>\n* Arthur is %d\n<%%(org-anniversary 1948 11 12)>\n"})
	from := agendaDayYMD(2026, 11, 1)
	r := agendaRules{From: from, To: from + 29, Today: from, WarnDays: 14}
	res, err := buildAgenda("", r, agendaDayTime(from))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, d := range res.Days {
		for _, e := range d.Entries {
			title := res.Nodes[e.Node].Headline
			if e.Text != "" {
				title = e.Text
			}
			got = append(got, d.Date+" "+title)
		}
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "2026-11-12 Arthur is 78") || strings.Count(joined, "Book club") != 1 || !strings.Contains(joined, "2026-11-12 Book club") {
		t.Errorf("agenda:\n%s", joined)
	}
}
