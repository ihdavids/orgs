package orgs

import (
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

func TestOrgReviewArithmetic(t *testing.T) {
	day := func(s string) int {
		d, ok := reviewDayOf(s)
		if !ok {
			t.Fatalf("not a day: %s", s)
		}
		return d
	}
	today := day("2026-11-04")
	cases := []struct {
		name      string
		p         reviewProps
		unrev     bool
		ok, never bool
		due       string
	}{
		{"next wins", reviewProps{Last: "[2026-01-01 Thu]", Next: "<2026-11-10 Tue>", Delay: "+1m"}, false, true, false, "2026-11-10"},
		{"last plus delay", reviewProps{Last: "[2026-10-04 Sun]", Delay: "+1m"}, false, true, false, "2026-11-04"},
		{"weeks", reviewProps{Last: "2026-10-21", Delay: "+2w"}, false, true, false, "2026-11-04"},
		{"bare number is days", reviewProps{Last: "[2026-11-01 Sun 10:00]", Delay: "3"}, false, true, false, "2026-11-04"},
		{"month end normalises", reviewProps{Last: "[2026-01-31 Sat]", Delay: "+1m"}, false, true, false, "2026-03-03"},
		{"year", reviewProps{Last: "[2025-11-04 Tue]", Delay: "+1y"}, false, true, false, "2026-11-04"},
		{"bad delay is a month", reviewProps{Last: "[2026-10-04 Sun]", Delay: "soon"}, false, true, false, "2026-11-04"},
		{"own delay only: never reviewed", reviewProps{Delay: "+2w", OwnDelay: true}, false, true, true, "2026-11-04"},
		{"inherited delay only: not in review", reviewProps{Delay: "+2w", InheritedDelay: true}, false, false, false, ""},
		{"inherited delay with unreviewed", reviewProps{Delay: "+2w", InheritedDelay: true}, true, true, true, "2026-11-04"},
		{"nothing", reviewProps{Delay: "+1m"}, true, false, false, ""},
	}
	for _, c := range cases {
		w, ok := reviewDueAtOf(c.p, c.unrev, today)
		if ok != c.ok || w.Never != c.never {
			t.Errorf("%s: ok %v never %v", c.name, ok, w.Never)
			continue
		}
		if ok && reviewDayString(w.Due) != c.due {
			t.Errorf("%s: due %s want %s", c.name, reviewDayString(w.Due), c.due)
		}
	}

	// org-review-sets-next-date
	if n, ok := reviewNextAfterMark(today, "+1w", "always", "<2027-01-01 Fri>"); !ok || reviewDayString(n) != "2026-11-11" {
		t.Errorf("always: %s %v", reviewDayString(n), ok)
	}
	if _, ok := reviewNextAfterMark(today, "+1w", "only-future", "<2027-01-01 Fri>"); ok {
		t.Errorf("only-future wrote an earlier date")
	}
	if _, ok := reviewNextAfterMark(today, "+1w", "only-future", ""); !ok {
		t.Errorf("only-future with none set did not write")
	}
	if _, ok := reviewNextAfterMark(today, "+1w", "never", ""); ok {
		t.Errorf("never wrote")
	}

	now := time.Date(2026, 11, 4, 9, 0, 0, 0, time.Local)
	for in, want := range map[string]string{"": "2026-11-04", "+1w": "2026-11-11", "tomorrow": "2026-11-05",
		"2026-12-01": "2026-12-01", "[2026-12-02 Wed]": "2026-12-02", "fri": "2026-11-06"} {
		d, err := reviewDayFrom(in, today, now)
		if err != nil || reviewDayString(d) != want {
			t.Errorf("reviewDayFrom(%q) = %s, %v; want %s", in, reviewDayString(d), err, want)
		}
	}
	if _, err := reviewDayFrom("whenever", today, now); err == nil {
		t.Errorf("whenever read as a date")
	}
}

func TestOrgReviewDueAndMark(t *testing.T) {
	text := `* Areas
:PROPERTIES:
:REVIEW_DELAY: +2w
:END:
** Health
:PROPERTIES:
:LAST_REVIEW: [2026-10-01 Thu]
:END:
** Money
:PROPERTIES:
:LAST_REVIEW: [2026-11-01 Sun]
:END:
** Garden
* Project X
:PROPERTIES:
:NEXT_REVIEW: [2026-11-06 Fri]
:END:
Body text.
* DONE Finished
:PROPERTIES:
:NEXT_REVIEW: [2026-01-01 Thu]
:END:
* Fresh
:PROPERTIES:
:REVIEW_DELAY: +1w
:END:
`
	withFeatures(t, func(f *common.FeatureSettings) { f.Review = false })
	paths := loadOrg(t, map[string]string{"rv.org": text})
	p := paths["rv.org"]
	hf, health := secAt(t, p, "Areas", "Health")
	if reviewDue(hf, health) {
		t.Fatalf("ReviewDue with the feature off")
	}

	withFeatures(t, func(f *common.FeatureSettings) { f.Review = true })
	Conf().Server.OrgReview = common.OrgReviewSettings{}
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local)
	got := map[string]string{}
	list := orgReviewItems(now, 7, false)
	for _, it := range list.Items {
		got[it.Headline] = it.Due
	}
	// Areas itself has its own delay and no dates: never reviewed, due now.
	want := map[string]string{"Areas": "", "Health": "2026-10-15", "Money": "2026-11-15", "Project X": "2026-11-06", "Fresh": ""}
	delete(want, "Money") // 11 days out, past within=7
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if d, ok := got[k]; !ok || d != v {
			t.Errorf("%s: due %q want %q (all %v)", k, d, v, got)
		}
	}
	if list.Due != 3 || (list.Items[0].Headline != "Areas" && list.Items[0].Headline != "Fresh") {
		t.Errorf("due count %d, first %s", list.Due, list.Items[0].Headline)
	}

	f, health := secAt(t, p, "Areas", "Health")
	if !reviewDue(f, health) {
		t.Errorf("Health not ReviewDue")
	}
	if _, err := reviewMark(f, health, agendaDayNum(now), "looked fine", now); err != nil {
		t.Fatal(err)
	}
	out := readOrg(t, p)
	if !strings.Contains(out, ":LAST_REVIEW: [2026-11-04 Wed]") || !strings.Contains(out, ":NEXT_REVIEW: [2026-11-18 Wed]") ||
		!strings.Contains(out, "Reviewed: looked fine") || !strings.Contains(out, "** Money") {
		t.Fatalf("after mark:\n%s", out)
	}

	// A cadence on a heading not yet in review starts its clock today.
	gf, garden := secAt(t, p, "Areas", "Garden")
	d := "1m"
	if _, err := reviewSetCadence(gf, garden, common.OrgReviewSet{Delay: &d}, now); err != nil {
		t.Fatal(err)
	}
	xf, x := secAt(t, p, "Project X")
	if _, err := reviewSnooze(xf, x, "+1w", now); err != nil {
		t.Fatal(err)
	}
	ff, fresh := secAt(t, p, "Fresh")
	if _, err := reviewSetCadence(ff, fresh, common.OrgReviewSet{Clear: true}, now); err != nil {
		t.Fatal(err)
	}
	out = readOrg(t, p)
	for _, want := range []string{"** Garden\n:PROPERTIES:\n:REVIEW_DELAY: +1m\n:NEXT_REVIEW:  [2026-12-04 Fri]\n:END:",
		"* Project X\n:PROPERTIES:\n:NEXT_REVIEW: [2026-11-11 Wed]\n:END:\nBody text.", "* Fresh"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "* Fresh\n:PROPERTIES:") || !strings.HasSuffix(strings.TrimSpace(out), "* Fresh") {
		t.Errorf("clear left the drawer:\n%s", out)
	}
}
