package orgpkgs

// Trackers (features.tracker), from the command line.
//
//	orgs track source:property:MOOD          a year as a heatmap (the #+TRACKER: words)
//	orgs track MOOD                          the same: a bare word is a property
//	orgs track habit:Run -type sparkline     the last weeks in one line
//	orgs track clock:Writing -type bar -from -30d
//	orgs track all                           every #+TRACKER: line, drawn
//	orgs track ls                            the #+TRACKER: lines
//	orgs track log MOOD 4 [-date yesterday]  write :MOOD: 4 on the day page
//
// Needs `orgs features tracker on`.

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"golang.org/x/term"
)

type Track struct {
	base
	Type   string
	From   string
	To     string
	Date   string
	Color  string
	Height int
}

func (self *Track) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Type, "type", "", "heatmap, line, bar or sparkline (overrides the line's type:)")
	fset.StringVar(&self.From, "from", "", "the first day: 2026-01-01, -30d, -1y")
	fset.StringVar(&self.To, "to", "", "the last day (today)")
	fset.StringVar(&self.Date, "date", "", "log: the day to write for (today): yesterday, -2d, 2026-11-01")
	fset.StringVar(&self.Color, "color", "", "blue, green, orange, red, violet (the line's color:)")
	fset.IntVar(&self.Height, "height", 8, "rows of a bar or line chart")
}

func (self *Track) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 {
		verb = words[0]
	}
	switch verb {
	case "ls", "list":
		self.list(core)
		return
	case "log", "add", "set":
		self.log(core, words[1:])
		return
	case "all":
		self.all(core)
		return
	case "":
		commands.Fail("orgs track OPTIONS... | MOOD | all | ls | log PROPERTY VALUE   (options as on a #+TRACKER: line: source:property:MOOD type:line from:-30d)")
	}
	line := strings.Join(words, " ")
	if len(words) == 1 && !strings.Contains(words[0], ":") {
		line = "source:property:" + words[0]
	}
	self.draw(core, line, true)
}

// withFlags adds the flags' words to a line; a word given twice reads as the
// last one.
func (self *Track) withFlags(line string) string {
	for _, kv := range [][2]string{{"type", self.Type}, {"from", self.From}, {"to", self.To}, {"color", self.Color}} {
		if kv[1] != "" {
			line += " " + common.FormatTrackerOptions([]common.TrackerOption{{Key: kv[0], Value: kv[1]}})
		}
	}
	return line
}

func (self *Track) series(core *commands.Core, line string) common.TrackerSeries {
	res, err := commands.SendReceiveGetErr[common.TrackerSeries](core, "tracker/series", map[string]string{"line": self.withFlags(line)})
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	return res
}

func (self *Track) draw(core *commands.Core, line string, render bool) {
	s := self.series(core, line)
	print := func() {
		fmt.Printf("%s%s%s %s\n", commands.C(commands.AnsiBold), s.Title, commands.C(commands.AnsiReset), dim(s.From+" to "+s.To))
		switch s.Type {
		case "sparkline":
			fmt.Println(trackSpark(s, termWidth()-2))
		case "line", "bar":
			for _, l := range trackColumns(s, termWidth()-8, self.Height) {
				fmt.Println(l)
			}
		default:
			for _, l := range trackHeatmap(s, termWidth()) {
				fmt.Println(l)
			}
		}
		fmt.Println(dim(trackSummary(s)))
	}
	if render {
		commands.RenderOne(s, print)
	} else {
		print()
	}
}

func termWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 20 {
		return w
	}
	return 100
}

// The colours of a heatmap's steps, 256-colour, by hue: empty, then four.
var trackShades = map[string][]string{
	"blue":   {"\033[38;5;240m", "\033[38;5;153m", "\033[38;5;111m", "\033[38;5;68m", "\033[38;5;26m"},
	"green":  {"\033[38;5;240m", "\033[38;5;151m", "\033[38;5;114m", "\033[38;5;71m", "\033[38;5;28m"},
	"orange": {"\033[38;5;240m", "\033[38;5;223m", "\033[38;5;215m", "\033[38;5;208m", "\033[38;5;166m"},
	"red":    {"\033[38;5;240m", "\033[38;5;224m", "\033[38;5;210m", "\033[38;5;167m", "\033[38;5;124m"},
	"violet": {"\033[38;5;240m", "\033[38;5;189m", "\033[38;5;147m", "\033[38;5;98m", "\033[38;5;56m"},
}

// The glyph carries the step too, for a terminal without colour.
var trackGlyphs = []string{"·", "░", "▒", "▓", "█"}

func trackShade(colour string, level int) string {
	sh, ok := trackShades[strings.ToLower(colour)]
	if !ok {
		sh = trackShades["blue"]
	}
	return commands.C(sh[level]) + trackGlyphs[level] + commands.C(commands.AnsiReset)
}

func trackBounds(s common.TrackerSeries) (float64, float64) {
	lo, hi := math.Min(0, s.Summary.Min), s.Summary.Max
	written := strings.HasPrefix(s.Source, "property:") || strings.HasPrefix(s.Source, "table:")
	if written && s.Type != "bar" && s.Summary.Min > 0 && s.Summary.Max > s.Summary.Min {
		lo = s.Summary.Min
	}
	return lo, hi
}

// trackHeatmap is the calendar: a column a week (Monday at the top), the
// month names above, the newest weeks kept when it is too wide.
func trackHeatmap(s common.TrackerSeries, width int) []string {
	if len(s.Days) == 0 {
		return nil
	}
	first, _ := time.ParseInLocation("2006-01-02", s.Days[0].Date, time.Local)
	start := first.AddDate(0, 0, -((int(first.Weekday()) + 6) % 7))
	last, _ := time.ParseInLocation("2006-01-02", s.Days[len(s.Days)-1].Date, time.Local)
	cols := int(last.Sub(start).Hours()/24)/7 + 1
	skip := 0
	if room := width - 5; cols > room && room > 4 {
		skip = cols - room
	}
	grid := make([][]string, 7)
	for r := range grid {
		grid[r] = make([]string, cols-skip)
		for c := range grid[r] {
			grid[r][c] = " "
		}
	}
	lo, hi := trackBounds(s)
	months := make([]byte, cols-skip)
	for i := range months {
		months[i] = ' '
	}
	monthLine := []rune(strings.Repeat(" ", cols-skip+3))
	for _, d := range s.Days {
		t, _ := time.ParseInLocation("2006-01-02", d.Date, time.Local)
		off := int(math.Round(t.Sub(start).Hours() / 24))
		c, r := off/7-skip, off%7
		if c < 0 {
			continue
		}
		grid[r][c] = trackShade(s.Color, common.TrackerLevel(d.Value, d.N, lo, hi))
		if t.Day() == 1 && c+3 <= len(monthLine) {
			copy(monthLine[c:], []rune(t.Format("Jan")))
		}
	}
	out := []string{"    " + dim(strings.TrimRight(string(monthLine), " "))}
	names := []string{"Mon", "", "Wed", "", "Fri", "", ""}
	for r := 0; r < 7; r++ {
		out = append(out, fmt.Sprintf("%s %s", dim(fmt.Sprintf("%-3s", names[r])), strings.Join(grid[r], "")))
	}
	legend := "    " + dim("less ")
	for l := 0; l < 5; l++ {
		legend += trackShade(s.Color, l)
	}
	return append(out, legend+dim(" more"))
}

// trackSpark is the newest days that fit as one line of ▁▂▃ glyphs.
func trackSpark(s common.TrackerSeries, width int) string {
	days := s.Days
	if width > 0 && len(days) > width {
		days = days[len(days)-width:]
	}
	return commands.C(commands.AnsiBlue) + common.Sparkline(days) + commands.C(commands.AnsiReset)
}

// trackColumns is a column chart in block glyphs, height rows, a column a
// day (the newest that fit), the scale on the left.
func trackColumns(s common.TrackerSeries, width, height int) []string {
	if height < 2 {
		height = 2
	}
	days := s.Days
	if width > 0 && len(days) > width {
		days = days[len(days)-width:]
	}
	lo, hi := trackBounds(s)
	if s.Type == "bar" {
		lo = math.Min(0, lo)
	}
	if hi <= lo {
		hi = lo + 1
	}
	zero := !(strings.HasPrefix(s.Source, "property:") || strings.HasPrefix(s.Source, "table:"))
	eighths := []string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
	rows := make([][]string, height)
	for r := range rows {
		rows[r] = make([]string, len(days))
	}
	for c, d := range days {
		filled := 0
		if d.N > 0 || zero {
			filled = int(math.Round((d.Value - lo) / (hi - lo) * float64(height*8)))
			if d.N > 0 && filled == 0 {
				filled = 1
			}
		}
		for r := 0; r < height; r++ {
			fromBottom := height - 1 - r
			n := filled - fromBottom*8
			switch {
			case n >= 8:
				rows[r][c] = "█"
			case n > 0:
				rows[r][c] = eighths[n]
			default:
				rows[r][c] = " "
			}
		}
	}
	out := []string{}
	for r := 0; r < height; r++ {
		label := ""
		switch r {
		case 0:
			label = common.TrackerFormat(hi, trackKind(s))
		case height - 1:
			label = common.TrackerFormat(lo, trackKind(s))
		}
		out = append(out, dim(fmt.Sprintf("%6s ", label))+commands.C(commands.AnsiBlue)+strings.Join(rows[r], "")+commands.C(commands.AnsiReset))
	}
	if len(days) > 0 {
		a, b := days[0].Date, days[len(days)-1].Date
		gap := len(days) - len(a) - len(b)
		if gap < 1 {
			gap = 1
		}
		out = append(out, "       "+dim(a+strings.Repeat(" ", gap)+b))
	}
	return out
}

func trackKind(s common.TrackerSeries) string {
	if s.Kind == "rating" || s.Kind == "bool" {
		return "number"
	}
	return s.Kind
}

// trackSummary is the server's summary in words.
func trackSummary(s common.TrackerSeries) string {
	sum := s.Summary
	if sum.Days == 0 {
		return "nothing in that range"
	}
	parts := []string{fmt.Sprintf("%d day%s", sum.Days, plural(sum.Days))}
	k := trackKind(s)
	parts = append(parts, "total "+common.TrackerFormat(sum.Total, k), "mean "+common.TrackerFormat(sum.Mean, k),
		"min "+common.TrackerFormat(sum.Min, k), "max "+common.TrackerFormat(sum.Max, k))
	parts = append(parts, fmt.Sprintf("streak %d", sum.Current), fmt.Sprintf("longest %d", sum.Longest))
	return strings.Join(parts, " · ")
}

func (self *Track) trackers(core *commands.Core) []common.TrackerLine {
	res, err := commands.SendReceiveGetErr[common.TrackerList](core, "tracker/list", nil)
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	return res.Trackers
}

func (self *Track) list(core *commands.Core) {
	ls := self.trackers(core)
	commands.Render(ls, func() {
		if len(ls) == 0 {
			fmt.Println("No #+TRACKER: lines. Put one in any file: #+TRACKER: source:property:MOOD")
			return
		}
		for _, t := range ls {
			kind := "#+TRACKER:"
			if t.Block {
				kind = "#+BEGIN: tracker"
			}
			fmt.Printf("%s %s\n  %s %s\n", commands.C(commands.AnsiBold)+t.Title+commands.C(commands.AnsiReset),
				dim(fmt.Sprintf("%s:%d", filepath.Base(t.Filename), t.Line+1)), dim(kind), t.Options)
		}
	})
}

func (self *Track) all(core *commands.Core) {
	ls := self.trackers(core)
	if commands.Machine() {
		out := []common.TrackerSeries{}
		for _, t := range ls {
			out = append(out, self.series(core, t.Options))
		}
		commands.Render(out, nil)
		return
	}
	for i, t := range ls {
		if i > 0 {
			fmt.Println()
		}
		self.draw(core, t.Options, false)
	}
}

func (self *Track) log(core *commands.Core, words []string) {
	if len(words) < 2 {
		commands.Fail("orgs track log PROPERTY VALUE [-date yesterday]")
	}
	req := common.TrackerLog{Property: words[0], Value: strings.Join(words[1:], " "), Date: self.Date}
	reply, err := commands.SendReceivePostErr[common.TrackerLog, common.TrackerLogResult](core, "tracker/log", &req)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	if !reply.Ok {
		commands.Fail("%s", reply.Msg)
	}
	if commands.Machine() {
		commands.RenderOne(reply, nil)
		return
	}
	made := ""
	if reply.Created {
		made = dim(" (made the day page)")
	}
	commands.Ok(1, "logged", []string{fmt.Sprintf("%s %s%s", reply.Msg, dim(reply.Date), made)})
}

func init() {
	commands.AddCmd("track", "trackers: a property, tag, habit or clocked time a day, as a heatmap or a line",
		func() commands.Cmd { return &Track{} })
}
