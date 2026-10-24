package standup

import (
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

func TestLastWorkingDay(t *testing.T) {
	cases := map[string]string{
		"2026-10-19": "2026-10-16", // Monday: Friday
		"2026-10-18": "2026-10-16", // Sunday: Friday
		"2026-10-17": "2026-10-16", // Saturday: Friday
		"2026-10-21": "2026-10-20", // Wednesday: Tuesday
	}
	for today, want := range cases {
		d, _ := time.ParseInLocation("2006-01-02", today, time.Local)
		if got := lastWorkingDay(d).Format("2006-01-02"); got != want {
			t.Errorf("%s: %s, want %s", today, got, want)
		}
	}
}

func sample() common.Recap {
	return common.Recap{
		Ok: true, From: "2026-10-16", To: "2026-10-18", Today: "2026-10-19",
		Closed: []common.RecapItem{
			{Headline: "Ship the landing_page", Project: "Website", When: "2026-10-16"},
			{Headline: "Water the plants", Repeat: true, Count: 2},
			{Headline: "Animated hero", Cancelled: true},
		},
		Clocked: []common.RecapGroup{{Name: "Website", Kind: "project", Mins: 210,
			Items: []common.RecapItem{{Headline: "Ship the landing_page", Mins: 150}, {Headline: "Pricing copy", Mins: 60}}}},
		ClockedMins: 210,
		Running:     &common.RecapItem{Headline: "Pricing copy", Hash: "h1", Mins: 65, When: "11:00"},
		Agenda: []common.RecapItem{
			{Headline: "Dentist", Kind: "timestamp", When: "15:00"},
			{Headline: "Tax return", Kind: "overdue", Late: 3},
			{Headline: "Pricing copy", Hash: "h1", Kind: "next", Status: "NEXT"},
		},
		Waiting: []common.RecapItem{{Headline: "Logo from the designer", Days: 4, SinceFrom: "logbook"}},
		Blocked: []common.DepProblem{{Kind: "blocked-chain", Title: "3 headings held up behind Logo from the designer"}},
		Habits:  []common.RecapHabit{{Headline: "Exercise", Done: 2, Days: 3}},
	}
}

func TestStandupSlack(t *testing.T) {
	today, _ := time.ParseInLocation("2006-01-02", "2026-10-19", time.Local)
	from, _ := time.ParseInLocation("2006-01-02", "2026-10-16", time.Local)
	s := &Standup{Max: 8}
	got := s.render(sample(), from, today)
	for _, want := range []string{
		"*Standup · Mon Oct 19*",
		"✅ *Done since Friday*",
		"• Ship the landing_page _(Website)_",
		"• Water the plants _(×2)_",
		"• ~Animated hero~ _cancelled_",
		"• Habits kept: Exercise",
		"⏱ *Clocked* 3h 30m",
		"• *Website* 3h 30m · Ship the landing_page 2h 30m, Pricing copy 1h",
		"⏳ On the clock: Pricing copy _(since 11:00, 1h 5m)_",
		"• 15:00 Dentist",
		"• Tax return _overdue 3d_",
		"• Pricing copy _(NEXT)_ ⏳",
		"🚧 *Blockers*",
		"• Waiting on Logo from the designer _(4 days)_",
		"• 3 headings held up behind Logo from the designer",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "**") || strings.Contains(got, "###") {
		t.Errorf("Slack text has Markdown in it:\n%s", got)
	}
}

func TestRecapMarkdown(t *testing.T) {
	r := sample()
	r.From, r.To, r.Today = "2026-10-12", "2026-10-18", "2026-10-19"
	r.Writing = &common.RecapWriting{Words: 3240, Days: 5, GoalDays: 3, Goal: 4000, GoalKind: "weekly",
		Files: []common.RecapWords{{File: "/n/novel.org", Words: 3240}}}
	s := &Recap{Max: 10}
	s.Md = true
	got := s.render(r)
	for _, want := range []string{
		"## Recap · Week 42 · Oct 12 – Oct 18",
		"_2 done · 3h 30m clocked · 3,240 words (81% of goal) · habits 2/3 · 2 blockers_",
		"### ✅ Done (2, 1 cancelled)",
		"**Website**",
		"- Ship the landing\\_page",
		"- ~~Animated hero~~ _cancelled_",
		"### ⏱ Clocked 3h 30m",
		"- Website 3h 30m _100%_",
		"### ✍️ Writing 3,240 words of a 4,000 weekly goal",
		"- 5 days with words, 3 met the daily goal",
		"- novel 3,240",
		"### 🔁 Habits",
		"- Exercise 2/3",
		"### 🚧 Blocked",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}
