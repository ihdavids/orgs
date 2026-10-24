package writing

// `orgs write`: worg's writing mode, the parts of it that are not an editor.
//
//	orgs write                      today against the goals, the streak, the last 14 days
//	orgs write -short               one line for a status bar: ✍ 412/500 · 6d
//	orgs write log -days 30         words per day, and the files they went into
//	orgs write goals                what they are
//	orgs write goals -daily 500 -rest 1
//	orgs write ideas                the idea generators
//	orgs write ideas character -n 3 -genre sf -set role=villain
//	orgs write snapshot 'Chapter 3' -title 'Before the rewrite'
//	orgs write snapshots draft.org  (or a heading) newest first
//	orgs write snapshots draft.org -delete 3fa2c1d09b7e
//
// The writing log is only read here. worg posts a file's word count as it is
// edited, counted by its own prose rules; see rhythm.go for why this side
// never posts one. A snapshot is taken the way writing mode takes one, so it
// shows up beside the heading in writing mode's Snapshots tab (outline.go).

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/storygen"
)

type Write struct {
	Short   bool
	File    string
	Days    int
	Daily   int
	Weekly  int
	Monthly int
	Rest    int
	N       int
	Seed    uint64
	Genre   string
	Opts    map[string]string
	Org     bool
	Title   string
	Delete  string
	Target  commands.TargetFlags
	fset    *flag.FlagSet
}

func (self *Write) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Write) StartPlugin(manager *common.PluginManager)         {}
func (self *Write) HelpGroup() string                                 { return "Writing" }

func (self *Write) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	self.Opts = map[string]string{}
	fset.BoolVar(&self.Short, "short", false, "one line for a status bar: ✍ words/goal · streak")
	fset.StringVar(&self.File, "file", "", "only this file's part of the log (a path or a base name)")
	fset.IntVar(&self.Days, "days", 14, "days the spark line (or log) goes back")
	fset.IntVar(&self.Daily, "daily", 0, "goals: words a day (0 for none)")
	fset.IntVar(&self.Weekly, "weekly", 0, "goals: words a week, Monday on (0 for none)")
	fset.IntVar(&self.Monthly, "monthly", 0, "goals: words a month (0 for none)")
	fset.IntVar(&self.Rest, "rest", 0, "goals: days a week a streak may miss (0 to 6)")
	fset.IntVar(&self.N, "n", 1, "ideas: how many to make")
	fset.Uint64Var(&self.Seed, "seed", 0, "ideas: the same seed makes the same ideas")
	fset.StringVar(&self.Genre, "genre", "", "ideas: fantasy, sf, modern or any")
	fset.Func("set", "ideas: a generator option, name=value (repeatable)", func(s string) error {
		k, v, ok := strings.Cut(s, "=")
		if !ok || k == "" {
			return fmt.Errorf("-set wants name=value")
		}
		self.Opts[k] = v
		return nil
	})
	fset.BoolVar(&self.Org, "org", false, "ideas: print each as an org heading, ready to paste or capture")
	fset.StringVar(&self.Title, "title", "", "snapshot: what it is a snapshot before")
	fset.StringVar(&self.Delete, "delete", "", "snapshots: take this one away, by its id")
	commands.AddTargetFlags(fset, &self.Target)
}

func (self *Write) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	sub := "today"
	if len(words) > 0 {
		sub = words[0]
		words = words[1:]
	}
	switch sub {
	case "today", "status":
		self.today(core)
	case "log":
		self.log(core)
	case "goals", "goal":
		self.goals(core)
	case "ideas", "idea":
		self.ideas(core, words)
	case "snapshot", "snap":
		self.snapshot(core, words)
	case "snapshots", "snaps":
		self.snapshots(core, words)
	default:
		commands.Fail("write: no %q - today, log, goals, ideas, snapshot or snapshots", sub)
	}
}

func init() {
	commands.AddCmd("write", "words written against your goals, idea generators and snapshots (worg's writing mode)",
		func() commands.Cmd { return &Write{} })
}

// ---------------------------------------------------------------------------
// The log
// ---------------------------------------------------------------------------

type logResp struct {
	Ok    bool
	Msg   string
	Files map[string]map[string]Day
	Goals *Goals
}

func (self *Write) readLog(core *commands.Core) logResp {
	res, err := commands.SendReceiveGetErr[logResp](core, "writing/log", nil)
	if err != nil {
		commands.Fail("write: %v", err)
	}
	if res.Files == nil {
		res.Files = map[string]map[string]Day{}
	}
	if res.Goals == nil {
		res.Goals = &Goals{}
	}
	// -file narrows here rather than through the endpoint's own ?file=, which
	// wants the server's exact path; a base name is what somebody types.
	if self.File != "" {
		kept := map[string]map[string]Day{}
		for f, d := range res.Files {
			if f == self.File || filepath.Base(f) == self.File || filepath.Base(f) == self.File+".org" {
				kept[f] = d
			}
		}
		if len(kept) == 0 {
			commands.Fail("write: nothing in the log for %s", self.File)
		}
		res.Files = kept
	}
	return res
}

// DayWords is one day of the spark line.
type DayWords struct {
	Day   string
	Words int
	Met   bool `json:",omitempty"`
}

// Today is the answer `orgs write -json` gives.
type Today struct {
	Date   string
	Goals  Goals
	Rhythm Rhythm
	Days   []DayWords // oldest first, today last
	Files  []Entry    // written in today, most first
}

func (self *Write) today(core *commands.Core) {
	lg := self.readLog(core)
	now := time.Now()
	byDay := DailyWords(lg.Files)
	g := *lg.Goals
	out := Today{Date: IsoDay(now), Goals: g, Rhythm: Compute(byDay, now, g), Files: EntriesOn(lg.Files, IsoDay(now))}
	n := self.Days
	if n < 1 {
		n = 14
	}
	for k := n - 1; k >= 0; k-- {
		d := IsoDay(addDays(dayOnly(now), -k))
		out.Days = append(out.Days, DayWords{Day: d, Words: byDay[d], Met: g.Daily > 0 && byDay[d] >= g.Daily})
	}
	if commands.RenderOne(out, nil) {
		return
	}
	if self.Short {
		fmt.Println(shortLine(out))
		return
	}
	if len(lg.Files) == 0 {
		fmt.Println("Nothing in the writing log yet: worg's writing mode keeps it as you write.")
		return
	}
	r := out.Rhythm
	row := func(name string, done, goal, left int, period string) {
		fmt.Printf("%-11s %s", name, num(done))
		if goal <= 0 {
			fmt.Println()
			return
		}
		fmt.Printf(" / %s  %s %3d%%", num(goal), bar(done, goal, 16), pct(done, goal))
		switch {
		case done >= goal:
			fmt.Printf("  %smade it%s", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset))
		case left > 1:
			fmt.Printf("  %s%s a day to make the %s%s", commands.C(commands.AnsiDim), num(Pace(done, goal, left)), period, commands.C(commands.AnsiReset))
		default:
			fmt.Printf("  %s%s to go%s", commands.C(commands.AnsiDim), num(goal-done), commands.C(commands.AnsiReset))
		}
		fmt.Println()
	}
	row("Today", r.Today, g.Daily, 1, "day")
	row("This week", r.Week, g.Weekly, r.WeekLeft, "week")
	row("This month", r.Month, g.Monthly, r.MonthLeft, "month")
	streak := fmt.Sprintf("%d day%s", r.Streak, plural(r.Streak))
	if r.Streak > 0 {
		streak = commands.C(commands.AnsiGold) + streak + commands.C(commands.AnsiReset)
	}
	fmt.Printf("%-11s %s %s(longest %d", "Streak", streak, commands.C(commands.AnsiDim), r.Longest)
	if g.RestDays > 0 {
		fmt.Printf(", %d rest day%s a week", g.RestDays, plural(g.RestDays))
	}
	fmt.Printf(")%s\n", commands.C(commands.AnsiReset))
	fmt.Printf("%-11s %s %s%s%s\n", fmt.Sprintf("Last %d", len(out.Days)), spark(out.Days, g.Daily),
		commands.C(commands.AnsiDim), sparkNote(r), commands.C(commands.AnsiReset))
	if len(out.Files) > 0 {
		fmt.Println()
		for _, e := range out.Files {
			fmt.Printf("  %6s  %s\n", num(e.Words), filepath.Base(e.File))
		}
	}
}

func sparkNote(r Rhythm) string {
	if r.DaysWritten30 == 0 {
		return ""
	}
	return fmt.Sprintf("  %s a day written, %d of the last 30", num(r.Average), r.DaysWritten30)
}

// The status bar line. Words over a goal when there is one, the streak when
// there is one; nothing else, because a status bar is a glance.
func shortLine(t Today) string {
	s := "✍ " + num(t.Rhythm.Today)
	if t.Goals.Daily > 0 {
		s += "/" + num(t.Goals.Daily)
	}
	if t.Rhythm.Streak > 0 {
		s += fmt.Sprintf(" · %dd", t.Rhythm.Streak)
	}
	return s
}

func (self *Write) log(core *commands.Core) {
	lg := self.readLog(core)
	byDay := DailyWords(lg.Files)
	n := self.Days
	if n < 1 {
		n = 14
	}
	type logRow struct {
		Day   string
		Words int
		Files []Entry
	}
	rows := []logRow{}
	today := dayOnly(time.Now())
	for k := 0; k < n; k++ {
		d := IsoDay(addDays(today, -k))
		if byDay[d] == 0 && len(EntriesOn(lg.Files, d)) == 0 {
			continue
		}
		rows = append(rows, logRow{d, byDay[d], EntriesOn(lg.Files, d)})
	}
	if commands.Render(rows, nil) {
		return
	}
	if len(rows) == 0 {
		fmt.Printf("Nothing written in the last %d days.\n", n)
		return
	}
	g := lg.Goals.Daily
	for _, r := range rows {
		t, _ := time.ParseInLocation("2006-01-02", r.Day, time.Local)
		col := ""
		if g > 0 && r.Words >= g {
			col = commands.AnsiGreen
		}
		fmt.Printf("%s %s%7s%s", t.Format("Mon 02 Jan"), commands.C(col), num(r.Words), commands.C(commands.AnsiReset))
		names := []string{}
		for _, e := range r.Files {
			names = append(names, fmt.Sprintf("%s %s", filepath.Base(e.File), num(e.Words)))
		}
		fmt.Printf("  %s%s%s\n", commands.C(commands.AnsiDim), strings.Join(names, ", "), commands.C(commands.AnsiReset))
	}
}

// ---------------------------------------------------------------------------
// Goals
// ---------------------------------------------------------------------------

func (self *Write) goals(core *commands.Core) {
	lg := self.readLog(core)
	g := *lg.Goals
	// Only the flags given change anything; a value out of range is sent as
	// typed so the server's refusal says why, rather than being ignored here.
	set := map[string]bool{}
	self.fset.Visit(func(f *flag.Flag) { set[f.Name] = true })
	changed := false
	for _, f := range []struct {
		name string
		v    int
		dst  *int
	}{{"daily", self.Daily, &g.Daily}, {"weekly", self.Weekly, &g.Weekly}, {"monthly", self.Monthly, &g.Monthly}, {"rest", self.Rest, &g.RestDays}} {
		if set[f.name] {
			*f.dst = f.v
			changed = true
		}
	}
	if changed {
		// The endpoint keeps the whole set, so what was not named is sent back
		// as it was rather than as zero.
		res, err := commands.SendReceivePostErr[Goals, common.ResultMsg](core, "writing/goals", &g)
		if err == commands.ErrDryRun {
			return
		}
		if err != nil {
			commands.Fail("write goals: %v", err)
		}
		if !res.Ok {
			commands.Fail("write goals: %s", res.Msg)
		}
	}
	if commands.RenderOne(g, nil) {
		return
	}
	show := func(name string, v int, unit string) {
		s := "none"
		if v > 0 {
			s = num(v) + " " + unit
		}
		fmt.Printf("%-9s %s\n", name, s)
	}
	show("Daily", g.Daily, "words")
	show("Weekly", g.Weekly, "words")
	show("Monthly", g.Monthly, "words")
	fmt.Printf("%-9s %d a week\n", "Rest days", g.RestDays)
}

// ---------------------------------------------------------------------------
// Ideas
// ---------------------------------------------------------------------------

type ideasList struct {
	Ok         bool            `json:"ok"`
	Generators []storygen.Info `json:"generators"`
}

func (self *Write) ideas(core *commands.Core, words []string) {
	list, err := commands.SendReceiveGetErr[ideasList](core, "writing/ideas", nil)
	if err != nil {
		commands.Fail("write ideas: %v", err)
	}
	if len(words) == 0 {
		if commands.Render(list.Generators, nil) {
			return
		}
		group := ""
		for _, g := range list.Generators {
			if g.Group != group {
				group = g.Group
				fmt.Printf("%s%s%s\n", commands.C(commands.AnsiBold), group, commands.C(commands.AnsiReset))
			}
			fmt.Printf("  %-16s %s%s%s\n", g.Id, commands.C(commands.AnsiDim), g.Summary, commands.C(commands.AnsiReset))
		}
		return
	}
	want := strings.ToLower(strings.Join(words, " "))
	var info *storygen.Info
	for i, g := range list.Generators {
		if strings.ToLower(g.Id) == want || strings.ToLower(g.Name) == want {
			info = &list.Generators[i]
			break
		}
	}
	if info == nil {
		// A prefix is enough when it names one generator.
		var hits []int
		for i, g := range list.Generators {
			if strings.HasPrefix(strings.ToLower(g.Id), want) || strings.HasPrefix(strings.ToLower(g.Name), want) {
				hits = append(hits, i)
			}
		}
		if len(hits) != 1 {
			commands.Fail("write ideas: no one generator called %q - `orgs write ideas` lists them", want)
		}
		info = &list.Generators[hits[0]]
	}
	opts := map[string]string{}
	for _, f := range info.Fields {
		if f.Default != "" {
			opts[f.Id] = f.Default
		}
	}
	if self.Genre != "" {
		opts["genre"] = self.Genre
	}
	for k, v := range self.Opts {
		opts[k] = v
	}
	n := self.N
	if n < 1 {
		n = 1
	}
	req := storygen.Request{Type: info.Id, Options: opts, Count: n, Seed: self.Seed}
	// Making ideas writes nothing, so it does not go through the dry run.
	res, err := common.RestPost[storygen.Response](&core.Rest, "writing/ideas", &req)
	if err != nil {
		commands.Fail("write ideas: %v", err)
	}
	if !res.Ok {
		commands.Fail("write ideas: %s", res.Msg)
	}
	if commands.Render(res.Items, nil) {
		return
	}
	for i, it := range res.Items {
		if i > 0 {
			fmt.Println()
		}
		if self.Org {
			fmt.Printf("* %s", it.Title)
			if len(it.Tags) > 0 {
				fmt.Printf(" :%s:", strings.Join(it.Tags, ":"))
			}
			fmt.Println()
			if it.Body != "" {
				fmt.Println(strings.TrimRight(it.Body, "\n"))
			}
			if it.Prose != "" {
				fmt.Println()
				fmt.Println(it.Prose)
			}
			continue
		}
		fmt.Printf("%s%s%s", commands.C(commands.AnsiBold), it.Title, commands.C(commands.AnsiReset))
		if it.Summary != "" {
			fmt.Printf("  %s%s%s", commands.C(commands.AnsiDim), it.Summary, commands.C(commands.AnsiReset))
		}
		fmt.Println()
		w := 0
		for _, f := range it.Facts {
			if len(f.Key) > w {
				w = len(f.Key)
			}
		}
		for _, f := range it.Facts {
			fmt.Printf("  %s%-*s%s  %s\n", commands.C(commands.AnsiCyan), w, f.Key, commands.C(commands.AnsiReset), f.Value)
		}
		if it.Prose != "" {
			fmt.Printf("  %s\n", it.Prose)
		}
	}
	if !commands.Machine() && self.Seed == 0 {
		fmt.Fprintf(os.Stderr, "%s-seed %d makes these again%s\n", commands.C(commands.AnsiDim), res.Seed, commands.C(commands.AnsiReset))
	}
}

// ---------------------------------------------------------------------------
// Snapshots
// ---------------------------------------------------------------------------

// Snapshot is the server's (writing.go).
type Snapshot struct {
	Id      string
	Heading string
	Title   string
	Time    string
	Text    string
	Words   int
}

type snapReq struct {
	File     string
	Snapshot *Snapshot `json:",omitempty"`
	Delete   string    `json:",omitempty"`
}

type snapResp struct {
	Ok        bool
	Msg       string
	Snapshot  *Snapshot
	Snapshots []Snapshot
}

// The heading, the file it is in and the file's lines, found the way every
// write verb finds one.
func (self *Write) heading(core *commands.Core, words []string) (common.Todo, Node, []string) {
	ts := commands.Resolve(core, &self.Target, asQuery(core, words), commands.TargetOpts{Prompt: "Snapshot> ", Quiet: true})
	if len(ts) != 1 {
		commands.Fail("write snapshot: one heading at a time")
	}
	t := ts[0]
	file, err := commands.SendReceiveGetErr[common.ResultMsg](core, "orgfile", map[string]string{"filename": t.Filename})
	if err != nil || !file.Ok {
		commands.Fail("write snapshot: could not read %s: %v %s", t.Filename, err, file.Msg)
	}
	lines := strings.Split(file.Msg, "\n")
	// The file's own keywords, so a heading in a file with its own #+TODO
	// loses its keyword from the path the way writing mode's does.
	kw := map[string]bool{}
	if ks, err := commands.SendReceiveGetErr[[]string](core, "status/"+commands.HashPath(t.Hash), nil); err == nil {
		for _, k := range ks {
			kw[k] = true
		}
	}
	st := commands.RestGetStates(core)
	for _, k := range append(st.Active, st.Done...) {
		kw[k] = true
	}
	for _, n := range Outline(lines, kw) {
		if n.Line == t.LineNum {
			return t, n, lines
		}
	}
	commands.Fail("write snapshot: %s:%d is not a heading any more; has the file changed?", t.Filename, t.LineNum)
	return t, Node{}, nil
}

// Words with no call or operator in them are a heading's text: the heading
// called exactly that when there is one, else any with the words in it. The
// same rule as `orgs mentions` (mentions.go asQuery).
func asQuery(core *commands.Core, words []string) []string {
	text := strings.TrimSpace(strings.Join(words, " "))
	if text == "" || text == "-" || commands.LooksLikeHash(text) || strings.ContainsAny(text, "()&|!=<>") {
		return words
	}
	pat := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			return r
		}
		return '.'
	}, text)
	exact := fmt.Sprintf(`MatchHeadline("(?i)^ *%s *$")`, pat)
	if found, err := commands.SendReceiveGetErr[common.Todos](core, "search", map[string]string{"query": exact}); err == nil && len(found) > 0 {
		return []string{exact}
	}
	return []string{fmt.Sprintf(`MatchHeadline("(?i)%s")`, pat)}
}

func (self *Write) snapshot(core *commands.Core, words []string) {
	t, n, lines := self.heading(core, words)
	text := strings.Join(lines[n.Line:n.End+1], "\n")
	title := strings.TrimSpace(self.Title)
	if title == "" {
		title = "Before " + time.Now().Format("2006-01-02")
	}
	s := Snapshot{Heading: n.Path, Title: title, Text: text, Words: SnapWords(text)}
	res, err := commands.SendReceivePostErr[snapReq, snapResp](core, "writing/snapshots", &snapReq{File: t.Filename, Snapshot: &s})
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("write snapshot: %v", err)
	}
	if !res.Ok || res.Snapshot == nil {
		commands.Fail("write snapshot: %s", res.Msg)
	}
	if commands.RenderOne(*res.Snapshot, nil) {
		return
	}
	fmt.Printf("%skept%s %s %s(%s words of %s, id %s)%s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset),
		res.Snapshot.Title, commands.C(commands.AnsiDim), num(res.Snapshot.Words), n.Title, res.Snapshot.Id, commands.C(commands.AnsiReset))
}

// A word naming a file rather than a heading: one ending .org, matched to a
// file the server reads by path or by base name.
func serverFile(core *commands.Core, word string) string {
	if !strings.HasSuffix(word, ".org") {
		return ""
	}
	files, err := commands.SendReceiveGetErr[[]string](core, "files", nil)
	if err != nil {
		commands.Fail("write snapshots: %v", err)
	}
	abs, _ := filepath.Abs(word)
	var hits []string
	for _, f := range files {
		if f == word || f == abs {
			return f
		}
		if filepath.Base(f) == filepath.Base(word) {
			hits = append(hits, f)
		}
	}
	if len(hits) == 1 {
		return hits[0]
	}
	if len(hits) > 1 {
		commands.Fail("write snapshots: %d files are called %s; give more of the path", len(hits), filepath.Base(word))
	}
	commands.Fail("write snapshots: the server reads no file called %s", word)
	return ""
}

func (self *Write) snapshots(core *commands.Core, words []string) {
	file, path := "", ""
	if len(words) == 1 {
		file = serverFile(core, words[0])
	}
	if file == "" {
		t, n, _ := self.heading(core, words)
		file, path = t.Filename, n.Path
	}
	if self.Delete != "" {
		res, err := commands.SendReceivePostErr[snapReq, snapResp](core, "writing/snapshots", &snapReq{File: file, Delete: self.Delete})
		if err == commands.ErrDryRun {
			return
		}
		if err != nil {
			commands.Fail("write snapshots: %v", err)
		}
		if !res.Ok {
			commands.Fail("write snapshots: %s", res.Msg)
		}
		if !commands.Machine() {
			fmt.Printf("deleted %s\n", self.Delete)
		}
		return
	}
	res, err := commands.SendReceiveGetErr[snapResp](core, "writing/snapshots", map[string]string{"file": file})
	if err != nil {
		commands.Fail("write snapshots: %v", err)
	}
	if !res.Ok {
		commands.Fail("write snapshots: %s", res.Msg)
	}
	rows := []Snapshot{}
	for _, s := range res.Snapshots {
		if path == "" || s.Heading == path {
			rows = append(rows, s)
		}
	}
	if commands.Render(rows, nil) {
		return
	}
	if len(rows) == 0 {
		fmt.Println("No snapshots.")
		return
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Time > rows[j].Time })
	for _, s := range rows {
		when := s.Time
		if t, err := time.Parse(time.RFC3339, s.Time); err == nil {
			when = t.Local().Format("2006-01-02 15:04")
		}
		where := strings.ReplaceAll(strings.Trim(s.Heading, "\x00"), "\x00", " › ")
		fmt.Printf("%s%s%s  %s  %s%s · %s words · %s%s\n", commands.C(commands.AnsiDim), s.Id, commands.C(commands.AnsiReset),
			when, s.Title, commands.C(commands.AnsiDim), num(s.Words), where, commands.C(commands.AnsiReset))
	}
}

// ---------------------------------------------------------------------------
// Drawing
// ---------------------------------------------------------------------------

func num(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		return "-" + s
	}
	return s
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func pct(done, goal int) int {
	if goal <= 0 {
		return 0
	}
	return done * 100 / goal
}

// A progress bar, green once the goal is made; ░ is what is left.
func bar(done, goal, width int) string {
	f := width
	if done < goal {
		f = done * width / goal
	}
	if f < 0 {
		f = 0
	}
	col := commands.AnsiBlue
	if done >= goal {
		col = commands.AnsiGreen
	}
	return commands.C(col) + strings.Repeat("█", f) + commands.C(commands.AnsiReset) +
		commands.C(commands.AnsiDim) + strings.Repeat("░", width-f) + commands.C(commands.AnsiReset)
}

var sparks = []rune("▁▂▃▄▅▆▇█")

// One cell a day, scaled to the busiest day shown (or the daily goal, when no
// day reached it); a day with nothing is a dot, a day that made the goal
// green. Scaling to the goal alone drew every good day as the same full block.
func spark(days []DayWords, goal int) string {
	top := goal
	for _, d := range days {
		if d.Words > top {
			top = d.Words
		}
	}
	var b strings.Builder
	for _, d := range days {
		if d.Words <= 0 {
			b.WriteString(commands.C(commands.AnsiDim) + "·" + commands.C(commands.AnsiReset))
			continue
		}
		i := len(sparks) - 1
		if top > 0 && d.Words < top {
			i = d.Words * (len(sparks) - 1) / top
		}
		col := commands.AnsiBlue
		if d.Met {
			col = commands.AnsiGreen
		}
		b.WriteString(commands.C(col) + string(sparks[i]) + commands.C(commands.AnsiReset))
	}
	return b.String()
}
