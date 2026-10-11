package plan

// `orgs plan`: time-blocking from the terminal.
//
//	orgs plan                          today: the hours as a timeline, then what is left to place
//	orgs plan -date tomorrow           another day (any date `orgs sched` reads)
//	orgs plan report 10:00             put the task matching "report" at 10:00, for its EFFORT (else an hour)
//	orgs plan report 10:00-11:30       ... for exactly that long
//	orgs plan report next              ... in the first free gap long enough
//	orgs plan 10:00                    pick the task (fzf), then place it
//	orgs plan 'IsStatus("NEXT")' 14    a query or a hash names the task, as for every write verb
//	orgs plan unblock report           take it off the day (a repeating task keeps its date)
//
// Placing writes SCHEDULED: <2026-10-12 Mon 10:00-11:00> through POST
// /plan/block (internal/app/orgs/planner.go), the same write worg's planner
// makes when a task is dragged onto an hour, so the two always agree about
// the day.
//
// Which task: words with no query punctuation are looked for in the day's
// own lists (the tasks waiting to be placed and the blocks already placed),
// every word somewhere in the headline. That is what a person typing
// `orgs plan report 10` means, and the plain query language would read
// "report" as an unknown name. A hash, a query or - for stdin go through
// commands.Resolve like every other verb.

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Plan struct {
	Date     string
	Query    string
	Hours    string
	For      string
	KeepDate bool
	Target   commands.TargetFlags
	fset     *flag.FlagSet
}

func init() {
	commands.AddCmd("plan", "time-block the day: place tasks on hours (SCHEDULED with a time range)",
		func() commands.Cmd { return &Plan{} })
}

func (self *Plan) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }

func (self *Plan) StartPlugin(manager *common.PluginManager) {}

func (self *Plan) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Date, "date", "", "the day: today, tomorrow, fri, +2d, 2026-10-12 (default today)")
	fset.StringVar(&self.Query, "query", "", "the tasks to offer (default IsTask())")
	fset.StringVar(&self.Hours, "hours", "", "the working day, e.g. 9-17 or 8:30-18:00 (default 9-17)")
	fset.StringVar(&self.For, "for", "", "how long a block placed with only a start runs: 45m, 1h30 (default its EFFORT, else an hour)")
	fset.BoolVar(&self.KeepDate, "keep-date", false, "unblock: take only the time off, keep the date")
	commands.AddTargetFlags(fset, &self.Target)
}

const usage = `  orgs plan                        today's hours and what is left to place
  orgs plan -date tomorrow         another day
  orgs plan report 10:00           place the task matching "report" at 10:00 (its EFFORT, else 1h)
  orgs plan report 10:00-11:30     ... for exactly that long
  orgs plan report next            ... in the first free gap
  orgs plan 14:30                  pick the task, then place it
  orgs plan unblock report         take it off the day`

func (self *Plan) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	day := self.day()
	ds, de := self.window()

	if len(words) > 0 && (words[0] == "unblock" || words[0] == "unplan" || words[0] == "off") {
		self.unblock(core, day, ds, de, words[1:])
		return
	}
	if len(words) > 0 && words[0] == "help" {
		fmt.Fprintln(os.Stderr, usage)
		return
	}

	// The time is the last word that reads as one; everything else names the
	// task.
	var slot *Slot
	rest := words
	for i := len(words) - 1; i >= 0; i-- {
		if s, ok := ParseSlot(words[i]); ok {
			slot = &s
			rest = append(append([]string{}, words[:i]...), words[i+1:]...)
			break
		}
	}
	if slot == nil {
		if len(words) > 0 {
			commands.Fail("orgs plan: when? Give a time (10:00, 10:00-11:30, 2pm) or next.\n\n%s", usage)
		}
		self.show(core, day, ds, de)
		return
	}
	pd := self.fetch(core, day, ds, de)
	task := self.pick(core, pd, rest, false)
	self.place(core, pd, task, *slot)
}

// The day asked for, as YYYY-MM-DD; today when none was.
func (self *Plan) day() string {
	if strings.TrimSpace(self.Date) == "" {
		return time.Now().Format("2006-01-02")
	}
	d, _, err := commands.ParseDate(self.Date, time.Now())
	if err != nil {
		commands.Fail("orgs plan -date: %v", err)
	}
	return d.Day.Format("2006-01-02")
}

func (self *Plan) window() (string, string) {
	if strings.TrimSpace(self.Hours) == "" {
		return "", ""
	}
	s, ok := ParseSlot(self.Hours)
	if !ok || s.Next || s.End < 0 {
		commands.Fail("orgs plan -hours: %q is not a span of the day, e.g. 9-17 or 8:30-18:00", self.Hours)
	}
	return Clock(s.Start), Clock(s.End)
}

func (self *Plan) fetch(core *commands.Core, day, ds, de string) common.PlanDay {
	ps := map[string]string{"date": day}
	if self.Query != "" {
		ps["query"] = self.Query
	}
	if ds != "" {
		ps["dayStart"], ps["dayEnd"] = ds, de
	}
	pd, err := commands.SendReceiveGetErr[common.PlanDay](core, "plan/day", ps)
	if err != nil {
		commands.Fail("%v", err)
	}
	if !pd.Ok {
		commands.Fail("%s", pd.Msg)
	}
	return pd
}

func (self *Plan) show(core *commands.Core, day, ds, de string) {
	pd := self.fetch(core, day, ds, de)
	commands.RenderOne(pd, func() {
		fmt.Print(Draw(pd, time.Now(), commands.C))
	})
}

// A candidate for "which task": something on the day's lists.
type choice struct {
	Hash, Headline, Status, Filename, What string
	LineNum                                int
}

// pick names the one heading a write is about. blocksOnly is for unblock,
// which can only be about something already on the day.
func (self *Plan) pick(core *commands.Core, pd common.PlanDay, words []string, blocksOnly bool) choice {
	plain := len(words) > 0 && self.Target.Hash == ""
	for _, w := range words {
		if w == "-" || commands.LooksLikeHash(w) || strings.ContainsAny(w, `()&|!"'{}=<>`) {
			plain = false
		}
	}
	if !plain && (len(words) > 0 || self.Target.Hash != "" || commands.StdinIsPipe()) {
		found := commands.Resolve(core, &self.Target, words, commands.TargetOpts{
			Prompt: "Plan> ", Query: "IsTask()", Quiet: true,
		})
		t := found[0]
		return choice{Hash: t.Hash, Headline: t.Headline, Status: t.Status, Filename: t.Filename, LineNum: t.LineNum}
	}

	all := []choice{}
	for _, b := range pd.Blocks {
		if b.Movable {
			all = append(all, choice{b.Hash, b.Headline, b.Status, b.Filename, b.Start + "-" + b.End, b.LineNum})
		}
	}
	if !blocksOnly {
		for _, t := range pd.Tasks {
			all = append(all, choice{t.Hash, t.Headline, t.Status, t.Filename, taskNote(t), t.LineNum})
		}
	}
	found := all
	if len(words) > 0 {
		found = []choice{}
		for _, c := range all {
			if matchWords(c.Headline, words) {
				found = append(found, c)
			}
		}
	}
	switch {
	case len(found) == 1:
		return found[0]
	case len(found) == 0 && len(words) > 0:
		commands.Fail("nothing on %s's lists matches %q", pd.Date, strings.Join(words, " "))
	case len(found) == 0:
		commands.Fail("nothing to place on %s", pd.Date)
	case self.Target.At > 0:
		if self.Target.At > len(found) {
			commands.Fail("-at %d, but only %d matched", self.Target.At, len(found))
		}
		return found[self.Target.At-1]
	case !commands.Interactive():
		if len(words) == 0 {
			commands.Fail("say which task: words from its headline, a query, or a hash")
		}
		commands.Fail("%d tasks match %q. -at N for one of them, or more words", len(found), strings.Join(words, " "))
	}
	lines := make([]string, len(found))
	for i, c := range found {
		lines[i] = commands.PickLine([]string{strconv.Itoa(i), c.Hash},
			fmt.Sprintf("%s%-12s%s %s%s", commands.C(commands.AnsiDim), c.What, commands.C(commands.AnsiReset), kw(c.Status), c.Headline))
	}
	preview := ""
	if selfCmd, err := commands.SelfCommand(core); err == nil {
		preview = selfCmd + " show -hash {2} -pane"
	}
	sel := commands.Pick(commands.PickOpts{Lines: lines, AddressFields: 2, Prompt: "Plan> ", Preview: preview,
		Header: "enter to place · ctrl-/ hides the pane"})
	if len(sel) == 0 {
		commands.Fail("nothing chosen")
	}
	addr, ok := commands.Address(sel[0], 2)
	if !ok {
		commands.Fail("nothing chosen")
	}
	i, err := strconv.Atoi(addr[0])
	if err != nil || i < 0 || i >= len(found) {
		commands.Fail("nothing chosen")
	}
	return found[i]
}

func matchWords(headline string, words []string) bool {
	h := strings.ToLower(headline)
	for _, w := range words {
		if !strings.Contains(h, strings.ToLower(w)) {
			return false
		}
	}
	return true
}

func (self *Plan) place(core *commands.Core, pd common.PlanDay, task choice, slot Slot) {
	req := common.PlanBlockReq{Hash: task.Hash, Date: pd.Date, DayStart: pd.DayStart, DayEnd: pd.DayEnd}
	if !slot.Next {
		req.Start = Clock(slot.Start)
		if slot.End >= 0 {
			req.End = Clock(slot.End)
		} else if self.For != "" {
			d := common.ParseDuration(self.For)
			if d == nil || d.Mins <= 0 {
				commands.Fail("orgs plan -for: %q is not a length (45m, 1h30, 2h)", self.For)
			}
			req.End = Clock(slot.Start + int(d.Mins+0.5))
		}
	}
	res, err := commands.SendReceivePostErr[common.PlanBlockReq, common.PlanWrite](core, "plan/block", &req)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		commands.Fail("could not place %s: %s", task.Headline, res.Msg)
	}
	commands.RenderOne(res, func() {
		fmt.Printf("%s%s-%s%s %s  %s%s%s\n", commands.C(commands.AnsiGreen), res.Start, res.End, commands.C(commands.AnsiReset),
			task.Headline, commands.C(commands.AnsiDim), res.Stamp, commands.C(commands.AnsiReset))
	})
}

func (self *Plan) unblock(core *commands.Core, day, ds, de string, words []string) {
	pd := self.fetch(core, day, ds, de)
	task := self.pick(core, pd, words, true)
	req := common.PlanUnblockReq{Hash: task.Hash, KeepDate: self.KeepDate}
	res, err := commands.SendReceivePostErr[common.PlanUnblockReq, common.PlanWrite](core, "plan/unblock", &req)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		commands.Fail("could not unblock %s: %s", task.Headline, res.Msg)
	}
	commands.RenderOne(res, func() {
		what := "unscheduled"
		if res.Stamp != "" {
			what = "now " + res.Stamp
		}
		fmt.Printf("%s %s(%s)%s\n", task.Headline, commands.C(commands.AnsiDim), what, commands.C(commands.AnsiReset))
	})
}

// ---------------------------------------------------------------------------
// Times
// ---------------------------------------------------------------------------

// Slot is a time typed on the command line: a start, maybe an end, or "next"
// (the first free gap). Minutes after midnight; End is -1 when not given.
type Slot struct {
	Start, End int
	Next       bool
}

var slotRe = regexp.MustCompile(`^(\d{1,2})(?::(\d{2}))?\s*(am|pm)?(?:-(\d{1,2})(?::(\d{2}))?\s*(am|pm)?)?$`)

// ParseSlot reads 10:00, 10, 2pm, 9:30am, 10:00-11:30, 14-15:30 or next.
// A bare number is an hour only up to 24, so a word like "2026" is never
// taken for a time.
func ParseSlot(s string) (Slot, bool) {
	t := strings.ToLower(strings.TrimSpace(s))
	if t == "next" || t == "now" {
		return Slot{Next: true, End: -1}, true
	}
	m := slotRe.FindStringSubmatch(t)
	if m == nil {
		return Slot{}, false
	}
	clock := func(h, mi, ampm string) (int, bool) {
		hh, _ := strconv.Atoi(h)
		mm := 0
		if mi != "" {
			mm, _ = strconv.Atoi(mi)
		}
		switch ampm {
		case "am":
			if hh < 1 || hh > 12 {
				return 0, false
			}
			if hh == 12 {
				hh = 0
			}
		case "pm":
			if hh < 1 || hh > 12 {
				return 0, false
			}
			if hh != 12 {
				hh += 12
			}
		}
		if hh > 24 || mm > 59 || (hh == 24 && mm != 0) {
			return 0, false
		}
		return hh*60 + mm, true
	}
	start, ok := clock(m[1], m[2], m[3])
	if !ok {
		return Slot{}, false
	}
	out := Slot{Start: start, End: -1}
	if m[4] != "" {
		ampm := m[6]
		end, ok := clock(m[4], m[5], ampm)
		if !ok {
			return Slot{}, false
		}
		// 2-4pm: the pm belongs to both ends.
		if m[3] == "" && ampm == "pm" && start < 12*60 && start+12*60 < end {
			out.Start += 12 * 60
		}
		out.End = end
	}
	return out, true
}

func Clock(m int) string {
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

func clockMins(s string) int {
	sl, ok := ParseSlot(s)
	if !ok || sl.Next {
		return -1
	}
	return sl.Start
}
