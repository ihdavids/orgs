package orgpkgs

// Timelines (features.timeline), from the command line.
//
//	orgs timeline                         every dated heading, by year and month
//	orgs timeline 'HasTags("history")'       a query's
//	orgs timeline group:tag from:-500     a #+TIMELINE: line's words
//	orgs timeline -group file -from -1y -to today
//	orgs timeline ls                      the #+TIMELINE: lines
//	orgs timeline svg -out t.svg          the picture the html exporter draws
//	orgs timeline date 'c. 1200'          how a :DATE: value reads
//
// Needs `orgs features timeline on`.

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Timeline struct {
	base
	Group  string
	From   string
	To     string
	File   string
	Line   string
	Clocks bool
	Out    string
	Width  int
}

func (self *Timeline) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Group, "group", "", "lanes by: tag, file, parent, keyword, property:NAME, none (auto)")
	fset.StringVar(&self.From, "from", "", "the start: 1066, -500, 2026-01-01, -1y")
	fset.StringVar(&self.To, "to", "", "the end: 1945, today, +2w")
	fset.StringVar(&self.File, "file", "", "only files matching this (a glob or part of a path)")
	fset.StringVar(&self.Line, "line", "", "a #+TIMELINE: line's options")
	fset.BoolVar(&self.Clocks, "clocks", false, "clocked time as spans too")
	fset.StringVar(&self.Out, "out", "", "svg: write here (default stdout)")
	fset.IntVar(&self.Width, "width", 960, "svg: the picture's width in pixels")
}

// timelineOptionWord is a word of a #+TIMELINE: line (key:value with a key
// the line knows), rather than a query.
func timelineOptionWord(w string) bool {
	k, _, ok := strings.Cut(w, ":")
	if !ok {
		return false
	}
	switch strings.ToLower(k) {
	case "query", "group", "lanes", "from", "to", "file", "title", "height", "clocks", "limit":
		return true
	}
	return false
}

// params is the request: the line's words, then a query, then the flags.
func (self *Timeline) params(words []string) map[string]string {
	opts, query := []string{}, []string{}
	if self.Line != "" {
		opts = append(opts, self.Line)
	}
	for _, w := range words {
		if timelineOptionWord(w) {
			opts = append(opts, common.FormatTrackerOptions(common.ParseTrackerOptions(w)))
		} else {
			query = append(query, w)
		}
	}
	ps := map[string]string{"line": strings.Join(opts, " ")}
	if len(query) > 0 {
		ps["query"] = strings.Join(query, " ")
	}
	for k, v := range map[string]string{"group": self.Group, "from": self.From, "to": self.To, "file": self.File} {
		if v != "" {
			ps[k] = v
		}
	}
	if self.Clocks {
		ps["clocks"] = "t"
	}
	if self.File == "" && !strings.Contains(ps["line"], "file:") {
		ps["file"] = "all"
	}
	return ps
}

func (self *Timeline) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 {
		verb = words[0]
	}
	switch verb {
	case "ls", "list":
		self.list(core)
		return
	case "svg":
		self.svg(core, words[1:])
		return
	case "date":
		self.date(core, strings.Join(words[1:], " "))
		return
	}
	res, err := commands.SendReceiveGetErr[common.TimelineData](core, "timeline/events", self.params(words))
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	commands.RenderOne(res, func() { timelinePrint(res) })
}

// The lanes' colours in a terminal (256-colour), the chart palette's order.
var timelineAnsi = []string{"\033[38;5;32m", "\033[38;5;202m", "\033[38;5;36m", "\033[38;5;214m", "\033[38;5;211m", "\033[38;5;28m", "\033[38;5;62m", "\033[38;5;167m"}

func timelineLaneColour(slot int) string {
	if slot >= 0 && slot < len(timelineAnsi) {
		return commands.C(timelineAnsi[slot])
	}
	return commands.C(commands.AnsiDim)
}

// timelineBar is a span as a short bar: longer spans longer, on a log scale
// so a day and a century both fit.
func timelineBar(s, e int64) string {
	days := float64(e-s) / 86400000
	n := int(math.Round(math.Log2(days+1) * 1.6))
	if n < 1 {
		n = 1
	}
	if n > 18 {
		n = 18
	}
	return strings.Repeat("━", n)
}

func timelinePrint(d common.TimelineData) {
	if d.Title != "" {
		fmt.Printf("%s%s%s\n", commands.C(commands.AnsiBold), d.Title, commands.C(commands.AnsiReset))
	}
	if len(d.Events) == 0 {
		fmt.Println(dim("No dated headings."))
		return
	}
	slots := map[string]int{}
	for _, l := range d.Lanes {
		slots[l.Name] = l.Slot
	}
	// Months under a year only when the year is busy enough to need them.
	perYear := map[string]int{}
	for _, ev := range d.Events {
		perYear[common.TimelineFormat(ev.S, "year")]++
	}
	width := 0
	for _, ev := range d.Events {
		if n := len([]rune(ev.Label)); n > width && n < 34 {
			width = n
		}
	}
	// A long history is headed by century, so a line a year does not take
	// a heading a year.
	first, last := d.Events[0].S, d.Events[len(d.Events)-1].S
	byCentury := float64(last-first)/86400000/365.25 > 300
	year, month := "", ""
	for _, ev := range d.Events {
		y := common.TimelineFormat(ev.S, "year")
		if byCentury {
			y = common.TimelineFormat(ev.S, "century")
		}
		if y != year {
			year, month = y, ""
			fmt.Printf("\n%s%s%s\n", commands.C(commands.AnsiBold), y, commands.C(commands.AnsiReset))
		}
		if !byCentury && perYear[y] > 8 && ev.Precision != "year" && ev.Precision != "decade" && ev.Precision != "century" {
			if m := common.TimelineFormat(ev.S, "month"); m != month {
				month = m
				fmt.Printf("  %s\n", dim(strings.Fields(m)[0]))
			}
		}
		mark := "●"
		if ev.Approx {
			mark = "○"
		}
		col := timelineLaneColour(slots[ev.Lane])
		extra := ""
		switch ev.Kind {
		case "era":
			mark = "▒"
			col = commands.C(commands.AnsiDim)
			extra = " " + dim("era")
		case "span":
			extra = " " + col + timelineBar(ev.S, ev.E) + commands.C(commands.AnsiReset) + " " + dim(common.TimelineSpanWords(ev.S, ev.E))
		}
		lane := ""
		if len(d.Lanes) > 1 && ev.Lane != "" {
			lane = "  " + dim(ev.Lane)
		}
		kw := ""
		if ev.Keyword != "" {
			kw = commands.C(commands.AnsiBold) + ev.Keyword + commands.C(commands.AnsiReset) + " "
		}
		label := ev.Label
		pad := width - len([]rune(label))
		if pad < 0 {
			pad = 0
		}
		fmt.Printf("  %s%s  %s%s%s %s%s%s%s\n", label, strings.Repeat(" ", pad), col, mark, commands.C(commands.AnsiReset), kw, ev.Title, extra, lane)
	}
	if d.Truncated {
		fmt.Println(dim("\n(the most recent " + strconv.Itoa(len(d.Events)) + " - narrow it with a query, -from or -to)"))
	}
}

func (self *Timeline) list(core *commands.Core) {
	res, err := commands.SendReceiveGetErr[common.TimelineList](core, "timeline/list", nil)
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	commands.Render(res.Timelines, func() {
		if len(res.Timelines) == 0 {
			fmt.Println(dim("No #+TIMELINE: lines. Write one in any file: #+TIMELINE: query:\"HasTags(\\\"history\\\")\" group:tag"))
		}
		for _, t := range res.Timelines {
			kind := "#+TIMELINE:"
			if t.Block {
				kind = "#+BEGIN: timeline"
			}
			fmt.Printf("%s:%d  %s%s%s  %s\n", filepath.Base(t.Filename), t.Line+1, commands.C(commands.AnsiBold), t.Title, commands.C(commands.AnsiReset), dim(kind+" "+t.Options))
		}
	})
}

func (self *Timeline) svg(core *commands.Core, words []string) {
	ps := self.params(words)
	ps["width"] = strconv.Itoa(self.Width)
	body, _, status, err := core.Rest.GetRaw("timeline/svg", ps)
	if err != nil {
		commands.Fail("%v", err)
	}
	if status != 200 {
		commands.Fail("the server answered %d: %s", status, strings.TrimSpace(string(body)))
	}
	if self.Out == "" {
		os.Stdout.Write(body)
		fmt.Println()
		return
	}
	if commands.Wrote("write "+self.Out, nil) {
		return
	}
	if err := os.WriteFile(self.Out, append(body, '\n'), 0644); err != nil {
		commands.Fail("%v", err)
	}
	fmt.Fprintln(os.Stderr, "wrote "+self.Out)
}

func (self *Timeline) date(core *commands.Core, text string) {
	if strings.TrimSpace(text) == "" {
		commands.Fail("orgs timeline date TEXT   (1066, -500, 44 BC, c. 1200, 1950s, 12th century, 1939-1945)")
	}
	// Read here: the reading is common's, the server would say the same.
	d, ok := common.ParseTimelineDate(text)
	r := common.TimelineDateReading{Ok: ok, TimelineDate: d}
	if !ok {
		r.Msg = fmt.Sprintf("%q does not read as a date", text)
	}
	commands.RenderOne(r, func() {
		if !ok {
			commands.Fail("%s", r.Msg)
		}
		end := ""
		if d.HasEnd {
			end = " to " + d.End + " (" + common.TimelineSpanWords(d.S, d.E) + ")"
		}
		approx := ""
		if d.Approx {
			approx = ", approximate"
		}
		fmt.Printf("%s%s%s  %s\n", commands.C(commands.AnsiBold), d.Label, commands.C(commands.AnsiReset), dim(d.Start+end+" - by the "+d.Precision+approx))
	})
}

func init() {
	commands.AddCmd("timeline", "dated headings on a timeline: by year, ls, svg, date",
		func() commands.Cmd { return &Timeline{} })
}
