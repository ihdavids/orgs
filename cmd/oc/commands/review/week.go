package review

// orgs review week - the weekly review, walked.
//
//	orgs review week            the seven steps, one at a time
//	orgs review week -restart   from the top, whatever step was left
//	orgs review week -days 14   waiting items older than a fortnight
//	orgs review week -json      what each step would ask about, as json
//
// Plain `orgs review` lists what wants a decision; this is where the decisions
// get made, in the order GTD makes them: empty the inbox, give every project a
// next action, chase what is waiting, look at where the time went, deal with
// what slipped, sweep the someday pile. Each step is the triage screen (one
// heading, one key per decision, `u` takes it back) over a different list.
//
// The step a review is on is kept on the server per user (POST /review/step),
// so stopping half way - here or in worg's Weekly Review tab - resumes in
// either. Finishing records the review (POST /review/done), and the first
// screen says how long ago the last one was.

import (
	"fmt"
	"math"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/cmd/oc/commands/triage"
	"github.com/ihdavids/orgs/internal/common"
)

var weekSteps = []string{
	"Empty the inbox",
	"Projects with no next action",
	"Waiting for",
	"Last week's time",
	"Stale scheduled items",
	"Someday sweep",
	"Done",
}

// Everything the wizard would walk, for -json.
type weekPlan struct {
	Last    common.ReviewLast
	Inbox   []common.ReviewItem
	Weekly  common.WeeklyReview
	Someday []common.ReviewItem
}

func (self *Review) weekly(core *commands.Core) common.WeeklyReview {
	ps := map[string]string{}
	if self.Days > 0 {
		ps["waitingDays"] = fmt.Sprint(self.Days)
	}
	wr, err := commands.SendReceiveGetErr[common.WeeklyReview](core, "review/weekly", ps)
	if err != nil {
		commands.Fail("orgs review week: %v", err)
	}
	if !wr.Ok {
		commands.Fail("orgs review week: %s", wr.Msg)
	}
	return wr
}

func (self *Review) week(core *commands.Core) {
	last, err := commands.SendReceiveGetErr[common.ReviewLast](core, "review/last", nil)
	if err != nil {
		commands.Fail("orgs review week: %v", err)
	}
	if commands.Machine() {
		plan := weekPlan{Last: last, Weekly: self.weekly(core)}
		if l, err := commands.SendReceiveGetErr[common.ReviewList](core, "review/inbox", nil); err == nil {
			plan.Inbox = l.Items
		}
		if l, err := commands.SendReceiveGetErr[common.ReviewList](core, "review/someday", nil); err == nil {
			plan.Someday = l.Items
		}
		commands.RenderOne(plan, nil)
		return
	}
	if !commands.Interactive() {
		commands.Fail("orgs review week walks you through the review and needs a terminal; -json lists what it would ask about")
	}

	tally := map[string]int{}
	w := triage.NewWalker(core, tally)
	w.Pause = true
	start := 1
	if st := last.State.Step; st > 1 && st <= len(weekSteps) && !self.Restart {
		fmt.Printf("\n  A review was left at step %d, %s.\n", st, strings.ToLower(weekSteps[st-1]))
		if w.Choose("carry on from there?", []string{"y yes", "n start again"}) != "n" {
			start = st
			// What it had done so far, so the end still adds up.
			for k, v := range last.State.Tally {
				tally[k] = v
			}
		}
	}
	if start == 1 && !self.intro(w, last) {
		return
	}

	for step := start; step <= len(weekSteps); step++ {
		// Where the review is, so stopping here resumes here - in this
		// terminal or in worg.
		saveStep(core, step, tally)
		quit := false
		switch step {
		case 1:
			quit = w.Walk(titled(triage.InboxDeck(core), 1))
		case 2:
			quit = w.Walk(self.stuckDeck(core))
		case 3:
			quit = w.Walk(self.waitingDeck(core))
		case 4:
			quit = self.timeStep(core, w)
		case 5:
			quit = w.Walk(self.staleDeck(core))
		case 6:
			quit = w.Walk(titled(triage.SomedayDeck(core), 6))
		case 7:
			self.finish(core, w)
		}
		if quit {
			saveStep(core, step, tally)
			fmt.Printf("\n  Stopped at step %d, %s. %sorgs review week%s carries on from there.\n",
				step, strings.ToLower(weekSteps[step-1]), commands.C(commands.AnsiBold), commands.C(commands.AnsiReset))
			w.Summary()
			return
		}
	}
}

type stepReq struct {
	Step  int
	Tally map[string]int
}

func saveStep(core *commands.Core, step int, tally map[string]int) {
	commands.SendReceivePostErr[stepReq, common.ReviewLast](core, "review/step", &stepReq{step, tally})
}

// A deck's title with where it is in the review.
func titled(d triage.Deck, step int) triage.Deck {
	d.Title = fmt.Sprintf("%d/%d  %s", step, len(weekSteps), weekSteps[step-1])
	return d
}

func (self *Review) intro(w *triage.Walker, last common.ReviewLast) bool {
	fmt.Print(map[bool]string{true: "\033[H\033[2J", false: "\n"}[commands.Colour()])
	fmt.Printf("  %sWeekly review%s\n", commands.C(commands.AnsiBold), commands.C(commands.AnsiReset))
	switch {
	case last.DaysAgo < 0:
		fmt.Printf("  %sThe first one. Welcome.%s\n\n", commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	case last.DaysAgo == 0:
		fmt.Printf("  %sLast done earlier today.%s\n\n", commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	default:
		tone := commands.AnsiDim
		if last.DaysAgo > 10 {
			tone = commands.AnsiGold
		}
		fmt.Printf("  %sLast done %d day%s ago.%s\n\n", commands.C(tone), last.DaysAgo,
			map[bool]string{true: "", false: "s"}[last.DaysAgo == 1], commands.C(commands.AnsiReset))
	}
	for i, s := range weekSteps {
		fmt.Printf("    %s%d%s  %s\n", commands.C(commands.AnsiDim), i+1, commands.C(commands.AnsiReset), s)
	}
	fmt.Printf("\n  %sany key starts · q stops · in each step u undoes the last decision%s\n",
		commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	switch w.ReadKey() {
	case "q", "esc", "ctrl-c":
		return false
	}
	return true
}

// Step 2: every project with nothing to do next. Its open tasks are listed
// under it, numbered: pressing a number makes that one NEXT, which is nearly
// always the whole answer. `a` writes a new one.
func (self *Review) stuckDeck(core *commands.Core) triage.Deck {
	open := map[string][]common.ReviewItem{}
	key := func(it common.ReviewItem) string { return it.Filename + "\x00" + it.Hash }
	return triage.Deck{
		Title: fmt.Sprintf("2/%d  %s", len(weekSteps), weekSteps[1]),
		Empty: "Every project has a next action.",
		Fetch: func() ([]common.ReviewItem, error) {
			wr := self.weekly(core)
			out := []common.ReviewItem{}
			for k := range open {
				delete(open, k)
			}
			for _, p := range wr.Stuck {
				out = append(out, p.Project)
				open[key(p.Project)] = p.Open
			}
			return out, nil
		},
		Extra: func(it common.ReviewItem) []string {
			kids := open[key(it)]
			if len(kids) == 0 {
				return []string{commands.C(commands.AnsiDim) + "nothing open under it" + commands.C(commands.AnsiReset)}
			}
			lines := []string{"", commands.C(commands.AnsiDim) + "open under it:" + commands.C(commands.AnsiReset)}
			for i, k := range kids {
				if i >= 9 {
					lines = append(lines, fmt.Sprintf("   … and %d more", len(kids)-9))
					break
				}
				lines = append(lines, fmt.Sprintf(" %s%d%s  %s%s%s %s", commands.C(commands.AnsiBold), i+1,
					commands.C(commands.AnsiReset), commands.C(commands.AnsiGold), k.Status, commands.C(commands.AnsiReset), k.Headline))
			}
			return lines
		},
		Keys: []triage.Key{
			{Key: "1-9", Label: "make that one NEXT"},
			{Key: "a", Label: "add a NEXT", Do: func(w *triage.Walker, it common.ReviewItem) triage.Outcome {
				text := w.Ask("next action:", "what is the very next thing to do on \""+commands.Ellipsis(it.Headline, 40)+"\"?  ·  enter cancels")
				if text == "" {
					return triage.Stay
				}
				return w.Act(it, common.ReviewAct{Action: "next", Text: text}, true)
			}},
		},
		Other: func(w *triage.Walker, it common.ReviewItem, k string) triage.Outcome {
			if len(k) != 1 || k[0] < '1' || k[0] > '9' {
				return triage.Stay
			}
			kids := open[key(it)]
			i := int(k[0] - '1')
			if i >= len(kids) {
				return triage.Stay
			}
			return w.Act(it, common.ReviewAct{Hash: kids[i].Hash, Action: "status", Keyword: "NEXT"}, true)
		},
	}
}

// Step 3: what has been waiting on somebody else for too long.
func (self *Review) waitingDeck(core *commands.Core) triage.Deck {
	return triage.Deck{
		Title: fmt.Sprintf("3/%d  %s", len(weekSteps), weekSteps[2]),
		Empty: "Nothing has waited too long.",
		Fetch: func() ([]common.ReviewItem, error) { return self.weekly(core).Waiting, nil },
		Extra: func(it common.ReviewItem) []string {
			return []string{commands.C(commands.AnsiGold) + WaitedFor(it) + commands.C(commands.AnsiReset)}
		},
		Keys: []triage.Key{
			{Key: "c", Label: "chase", Do: func(w *triage.Walker, it common.ReviewItem) triage.Outcome {
				note := w.Ask("chased:", "a note to keep with it (who, how), or enter for none")
				return w.Act(it, common.ReviewAct{Action: "chase", Text: note}, true)
			}},
			{Key: "d", Label: "done", Do: func(w *triage.Walker, it common.ReviewItem) triage.Outcome {
				return w.Act(it, common.ReviewAct{Action: "done"}, true)
			}},
			{Key: "s", Label: "reschedule", Do: func(w *triage.Walker, it common.ReviewItem) triage.Outcome {
				if w.Schedule(it, "schedule") == triage.Stay {
					return triage.Next
				}
				return triage.Stay
			}},
		},
	}
}

// WaitedFor is how long an item has waited, in words, saying how sure that is.
func WaitedFor(it common.ReviewItem) string {
	if it.Days < 0 {
		return "waiting, with nothing to say since when"
	}
	s := fmt.Sprintf("waiting %d day%s", it.Days, map[bool]string{true: "", false: "s"}[it.Days == 1])
	if it.Since != "" {
		how := map[string]string{"logbook": "", "created": " (when it was made)", "date": " (its date)"}[it.SinceFrom]
		s += " · since " + it.Since + how
	}
	if it.Chased != "" {
		s += " · last chased " + it.Chased
	}
	return s
}

// Step 4: last week's time. Nothing to decide, only to look at, so it is a
// table and a keypress.
func (self *Review) timeStep(core *commands.Core, w *triage.Walker) bool {
	t := self.weekly(core).Time
	fmt.Print(map[bool]string{true: "\033[H\033[2J", false: "\n"}[commands.Colour()])
	fmt.Printf("  %s4/%d  %s%s  %s%s – %s%s\n\n", commands.C(commands.AnsiBold), len(weekSteps), weekSteps[3],
		commands.C(commands.AnsiReset), commands.C(commands.AnsiDim), t.From, t.To, commands.C(commands.AnsiReset))
	if t.Clocked == 0 && t.Planned == 0 {
		fmt.Printf("  %sNothing was clocked and nothing with an EFFORT was planned for that week.%s\n",
			commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	} else {
		fmt.Printf("  clocked %s%s%s of %s%s%s planned\n\n", commands.C(commands.AnsiBold), HM(t.Clocked), commands.C(commands.AnsiReset),
			commands.C(commands.AnsiBold), HM(t.Planned), commands.C(commands.AnsiReset))
		timeRows("by project", t.Projects)
		timeRows("by tag", t.Tags)
	}
	fmt.Printf("\n  %sany key goes on · q stops%s\n", commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	switch w.ReadKey() {
	case "q", "esc", "ctrl-c":
		return true
	}
	return false
}

func timeRows(title string, rows []common.ReviewTimeRow) {
	if len(rows) == 0 {
		return
	}
	fmt.Printf("  %s%s%s\n", commands.C(commands.AnsiDim), title, commands.C(commands.AnsiReset))
	most := 0.0
	for _, r := range rows {
		most = math.Max(most, math.Max(r.Clocked, r.Planned))
	}
	shown := rows
	if len(shown) > 12 {
		shown = shown[:12]
	}
	for _, r := range shown {
		name := r.Name
		if r.Kind == "file" {
			name += " (no project)"
		}
		fmt.Printf("    %-28s %6s / %-6s %s %s\n", commands.Ellipsis(name, 28), HM(r.Clocked), HM(r.Planned),
			TimeBar(r.Clocked, r.Planned, most, 24), verdict(r))
	}
	if len(shown) < len(rows) {
		fmt.Printf("    %s… and %d more%s\n", commands.C(commands.AnsiDim), len(rows)-len(shown), commands.C(commands.AnsiReset))
	}
	fmt.Println()
}

// TimeBar draws clocked against planned on one scale: the clocked part solid,
// the planned part still to go faint, and anything over the plan in gold.
func TimeBar(clocked, planned, most float64, width int) string {
	if most <= 0 {
		return ""
	}
	cells := func(m float64) int { return int(math.Round(m / most * float64(width))) }
	c, p := cells(clocked), cells(planned)
	if clocked > 0 && c == 0 {
		c = 1
	}
	if c <= p {
		return commands.C(commands.AnsiCyan) + strings.Repeat("█", c) + commands.C(commands.AnsiDim) +
			strings.Repeat("░", p-c) + commands.C(commands.AnsiReset) + strings.Repeat(" ", width-p)
	}
	return commands.C(commands.AnsiCyan) + strings.Repeat("█", p) + commands.C(commands.AnsiGold) +
		strings.Repeat("█", c-p) + commands.C(commands.AnsiReset) + strings.Repeat(" ", max(0, width-c))
}

func verdict(r common.ReviewTimeRow) string {
	d := commands.C(commands.AnsiDim)
	e := commands.C(commands.AnsiReset)
	switch {
	case r.Planned == 0:
		return d + "unplanned" + e
	case r.Clocked == 0:
		return d + "not touched" + e
	}
	pct := int(math.Round(r.Clocked / r.Planned * 100))
	s := fmt.Sprintf("%d%%", pct)
	if r.Tasks > 0 {
		s += fmt.Sprintf(" · %d/%d done", r.Done, r.Tasks)
	}
	return d + s + e
}

// HM is minutes as org writes a duration.
func HM(mins float64) string {
	m := int(math.Round(mins))
	return fmt.Sprintf("%d:%02d", m/60, m%60)
}

// Step 5: scheduled, gone past, and nothing has happened.
func (self *Review) staleDeck(core *commands.Core) triage.Deck {
	return triage.Deck{
		Title: fmt.Sprintf("5/%d  %s", len(weekSteps), weekSteps[4]),
		Empty: "Nothing scheduled has slipped by untouched.",
		Fetch: func() ([]common.ReviewItem, error) { return self.weekly(core).Stale, nil },
		Extra: func(it common.ReviewItem) []string {
			return []string{commands.C(commands.AnsiGold) + fmt.Sprintf("scheduled %d day%s ago, untouched since", it.Days,
				map[bool]string{true: "", false: "s"}[it.Days == 1]) + commands.C(commands.AnsiReset)}
		},
		Keys: []triage.Key{
			{Key: "s", Label: "reschedule", Do: func(w *triage.Walker, it common.ReviewItem) triage.Outcome {
				return w.Schedule(it, "schedule")
			}},
			{Key: "x", Label: "unschedule", Do: func(w *triage.Walker, it common.ReviewItem) triage.Outcome {
				return w.Act(it, common.ReviewAct{Action: "unschedule"}, true)
			}},
			{Key: "d", Label: "done", Do: func(w *triage.Walker, it common.ReviewItem) triage.Outcome {
				return w.Act(it, common.ReviewAct{Action: "done"}, true)
			}},
			{Key: "a", Label: "archive", Do: func(w *triage.Walker, it common.ReviewItem) triage.Outcome {
				return w.Act(it, common.ReviewAct{Action: "archive"}, true)
			}},
		},
	}
}

// Step 7: say what happened, and write the review down.
func (self *Review) finish(core *commands.Core, w *triage.Walker) {
	res, err := commands.SendReceivePostErr[struct{ Summary map[string]int }, common.ReviewLast](core, "review/done",
		&struct{ Summary map[string]int }{w.Tally})
	fmt.Print(map[bool]string{true: "\033[H\033[2J", false: "\n"}[commands.Colour()])
	fmt.Printf("  %s✓ Weekly review done.%s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset))
	if line := triage.TallyLine(w.Tally); line != "" {
		fmt.Printf("  %s%s%s\n", commands.C(commands.AnsiDim), line, commands.C(commands.AnsiReset))
	} else {
		fmt.Printf("  %sNothing needed changing.%s\n", commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	}
	switch {
	case err == commands.ErrDryRun:
	case err != nil:
		fmt.Printf("  %scould not record it: %v%s\n", commands.C(commands.AnsiRed), err, commands.C(commands.AnsiReset))
	case !res.Ok:
		fmt.Printf("  %scould not record it: %s%s\n", commands.C(commands.AnsiRed), res.Msg, commands.C(commands.AnsiReset))
	}
	fmt.Println()
}
