package orgs

// Timelines (features.timeline), in org's own spelling.
// Dated headings become events on a timeline:
//
//   - points: a timestamp, CLOSED, SCHEDULED, DEADLINE, CREATED, a datetree day;
//   - spans: an org date range <a>--<b>, SCHEDULED to DEADLINE, clocked time
//     (clocks:t), a :DATE: range;
//   - eras: a heading with a range and :TIMELINE: era (or an :era: tag),
//     drawn as a band behind every lane.
//
// History notes date a heading with a :DATE: property in free words a
// historian writes - -500, 44 BC, 1066-10-14, c. 1200, 1950s, 12th century,
// 1939-1945 - read by common.ParseTimelineDate. A #+TIMELINE: line says what
// to draw, in the #+TRACKER: line's key:value words:
//
//	#+TIMELINE: query:"HasTags(\"history\")" group:tag from:-500 to:1500 title:"Antiquity"
//
// and a #+BEGIN: timeline dynamic block takes the same words and writes the
// events as an org table (date, event, lane). While the feature is on the
// export copy has each #+TIMELINE: line drawn: the svg for html (and typst),
// a chronological list for LaTeX and plain text. A file never changes for it.

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/orgdate"
)

const timelineFeature = "timeline"

func init() {
	dynGenerators["timeline"] = dynTimeline
}

var (
	timelineLineRe   = regexp.MustCompile(`(?i)^(\s*)#\+TIMELINE:\s*(.*)$`)
	timelineBeginRe  = regexp.MustCompile(`(?i)^\s*#\+BEGIN:\s+timeline\b\s*(.*)$`)
	timelineDayRe    = regexp.MustCompile(`^\s*[<\[]?(\d{4}-\d{2}-\d{2})`)
	timelineLinkRe   = regexp.MustCompile(`\[\[([^\]]*)\](?:\[([^\]]*)\])?\]`)
	timelinePlanRe   = regexp.MustCompile(`^\s*(SCHEDULED|DEADLINE|CLOSED):`)
	timelineMarkupRe = regexp.MustCompile(`(^|\s)[*/=~+_]([^\s*/=~+_][^*/=~+_]*?)[*/=~+_]($|[\s.,;:!?)])`)
)

// The chart palette's categorical slots, in its order (light steps; worg
// steps them for a dark page itself). A lane past the eighth is ink grey.
var timelineHues = []string{"#2a78d6", "#eb6834", "#1baf7a", "#eda100", "#e87ba4", "#008300", "#6250d6", "#e34948"}

const timelineGrey = "#898781"

// ----------------------------------------------------------------------------
// A timeline's options
// ----------------------------------------------------------------------------

type timelineSpec struct {
	Line   string
	Opts   []common.TrackerOption
	Query  string
	Group  string
	File   string // a glob or part of a path; "" every file
	Title  string
	From   int64
	To     int64 // exclusive; 0 open
	FromS  string
	ToS    string
	Height int
	Clocks bool
	Limit  int
}

func (s timelineSpec) get(key, def string) string { return common.TrackerGet(s.Opts, key, def) }

// timelineBound reads from:/to: - a free date (1066, -500, 2026-11-04) or a
// day as orgs reads one typed (-1y, today, +2w). For to: the end of the
// period it names.
func timelineBound(v string, now time.Time, end bool) (int64, error) {
	v = strings.TrimSpace(v)
	if d, ok := common.ParseTimelineDate(v); ok {
		if end {
			return d.Until, nil
		}
		return d.S, nil
	}
	d, ok, err := orgdate.ParseDate(v, now)
	if err != nil || !ok || d.Day.IsZero() {
		return 0, fmt.Errorf("%q is not a date (1066, -500, 2026-11-04, -1y, today)", v)
	}
	day := time.Date(d.Day.Year(), d.Day.Month(), d.Day.Day(), 0, 0, 0, 0, time.UTC)
	if end {
		day = day.AddDate(0, 0, 1)
	}
	return day.UnixMilli(), nil
}

// parseTimelineSpec reads a #+TIMELINE: line's options. here is the file the
// line is in: with no query: and no file:, a line draws its own file.
func parseTimelineSpec(line, here string, now time.Time) (timelineSpec, error) {
	if m := timelineLineRe.FindStringSubmatch(line); m != nil {
		line = m[2]
	}
	opts := common.ParseTrackerOptions(line)
	s := timelineSpec{Line: strings.TrimSpace(line), Opts: opts}
	s.Query = strings.TrimSpace(s.get("query", ""))
	s.Group = strings.TrimSpace(s.get("group", s.get("lanes", "auto")))
	switch g := strings.ToLower(s.Group); {
	case g == "" || g == "auto":
		s.Group = "auto"
	case g == "tag" || g == "tags":
		s.Group = "tag"
	case g == "file" || g == "files":
		s.Group = "file"
	case g == "parent" || g == "heading":
		s.Group = "parent"
	case g == "keyword" || g == "todo" || g == "state":
		s.Group = "keyword"
	case g == "none" || g == "nil" || g == "no":
		s.Group = "none"
	case strings.HasPrefix(g, "property:") || strings.HasPrefix(g, "prop:"):
		_, name, _ := strings.Cut(s.Group, ":")
		if strings.TrimSpace(name) == "" {
			return s, fmt.Errorf("group:property: needs a property name: group:property:PLACE")
		}
		s.Group = "property:" + strings.ToUpper(strings.TrimSpace(name))
	default:
		return s, fmt.Errorf("group:%s is not a way to make lanes (auto, tag, file, parent, keyword, property:NAME, none)", s.Group)
	}
	s.File = s.get("file", "")
	switch strings.ToLower(s.File) {
	case "", "this", "here":
		s.File = ""
		if s.Query == "" && here != "" && strings.ToLower(s.get("file", "this")) != "all" {
			s.File = here
		}
	case "all", "*":
		s.File = ""
	}
	s.Title = s.get("title", "")
	var err error
	if v := s.get("from", ""); v != "" {
		if s.From, err = timelineBound(v, now, false); err != nil {
			return s, fmt.Errorf("from:%s: %v", v, err)
		}
		s.FromS = v
	}
	if v := s.get("to", ""); v != "" {
		if s.To, err = timelineBound(v, now, true); err != nil {
			return s, fmt.Errorf("to:%s: %v", v, err)
		}
		s.ToS = v
	}
	if s.FromS != "" && s.ToS != "" && s.To <= s.From {
		return s, fmt.Errorf("to:%s is before from:%s", s.ToS, s.FromS)
	}
	if v := s.get("height", ""); v != "" {
		n, err := strconv.Atoi(strings.TrimSuffix(v, "px"))
		if err != nil || n < 80 || n > 4000 {
			return s, fmt.Errorf("height:%s is not a height in pixels (80 to 4000)", v)
		}
		s.Height = n
	}
	s.Clocks = dynTrue(s.get("clocks", s.get("clock", "nil")))
	s.Limit = 2000
	if v := s.get("limit", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			s.Limit = n
		}
	}
	return s, nil
}

// ----------------------------------------------------------------------------
// A heading's dates
// ----------------------------------------------------------------------------

func timelineSdcDate(s *org.SDC) *org.OrgDate {
	if s == nil || s.Date == nil || s.Date.Start.IsZero() {
		return nil
	}
	return s.Date
}

// timelineOrgSpan is an org date as positions: a day without a time is the
// whole day.
func timelineOrgSpan(d *org.OrgDate) (s, e int64, prec string) {
	s = common.TimelineWall(d.Start)
	prec = "day"
	if d.HaveTime {
		prec = "minute"
	}
	if !d.End.IsZero() && d.End.After(d.Start) {
		e = common.TimelineWall(d.End)
		if !d.HaveTime {
			e = common.TimelineWall(time.Date(d.End.Year(), d.End.Month(), d.End.Day()+1, 0, 0, 0, 0, time.UTC))
		}
	}
	return
}

// timelinePoint fills an event's start from a position.
func timelineSet(ev *common.TimelineEvent, s, e int64, prec, source string) {
	ev.S, ev.E, ev.Precision, ev.Source = s, e, prec, source
	ev.Start = common.TimelineISO(s, prec)
	ev.Label = common.TimelineFormat(s, prec)
	ev.Kind = "point"
	if e > s {
		ev.Kind = "span"
		endPrec := prec
		last := e
		if prec == "day" {
			last = e - 86400000 // the last day, not the morning after
		}
		ev.End = common.TimelineISO(e, prec)
		if last > s {
			end := common.TimelineFormat(last, endPrec)
			if endPrec == "minute" && common.TimelineFormat(s, "day") == common.TimelineFormat(last, "day") {
				end = end[strings.LastIndex(end, " ")+1:] // 14:00 – 15:30 on one day
			}
			ev.Label += " – " + end
		}
	} else {
		ev.E = 0
	}
}

// timelineDayOf is a datetree day: the heading's title, or a heading above it.
func timelineDayOf(sec *org.Section) (time.Time, bool, bool) {
	for s, own := sec, true; s != nil && s.Headline != nil; s, own = s.Parent, false {
		if m := timelineDayRe.FindStringSubmatch(headlineTitle(s)); m != nil {
			if t, err := time.ParseInLocation("2006-01-02", m[1], time.UTC); err == nil {
				return t, own, true
			}
		}
	}
	return time.Time{}, false, false
}

func timelineHidden(sec *org.Section) bool {
	if v, ok := ownProp(sec, "TIMELINE"); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "no", "nil", "none", "hide", "hidden", "off":
			return true
		}
	}
	return ednaHasTag(sec, "notimeline")
}

func timelineIsEra(sec *org.Section) bool {
	if v, ok := ownProp(sec, "TIMELINE"); ok && strings.EqualFold(strings.TrimSpace(v), "era") {
		return true
	}
	return ednaHasTag(sec, "era")
}

// timelineDated is the date a heading goes on the timeline with, in order:
// :DATE:, an org date range in its title or body, SCHEDULED to DEADLINE, an
// active timestamp, CLOSED, SCHEDULED, DEADLINE, CREATED, a datetree day.
func timelineDated(sec *org.Section, ev *common.TimelineEvent) bool {
	h := sec.Headline
	if v, ok := ownProp(sec, "DATE"); ok {
		if d, ok := common.ParseTimelineDate(v); ok {
			ev.S, ev.E, ev.Precision, ev.Approx, ev.Label, ev.Start, ev.End, ev.Source = d.S, d.E, d.Precision, d.Approx, d.Label, d.Start, d.End, "date"
			ev.Kind = "point"
			if d.HasEnd {
				ev.Kind = "span"
			}
			return true
		}
	}
	for _, ts := range h.Timestamps {
		if ts != nil && ts.Time != nil && !ts.Time.Start.IsZero() && !ts.Time.End.IsZero() && ts.Time.End.After(ts.Time.Start) {
			s, e, p := timelineOrgSpan(ts.Time)
			timelineSet(ev, s, e, p, "range")
			return true
		}
	}
	if sd, dd := timelineSdcDate(h.Scheduled), timelineSdcDate(h.Deadline); sd != nil && dd != nil && dd.Start.After(sd.Start) {
		s, _, p := timelineOrgSpan(sd)
		e, _, p2 := timelineOrgSpan(dd)
		if p2 == "day" {
			e += 86400000
		}
		timelineSet(ev, s, e, p, "planned")
		return true
	}
	for _, ts := range h.Timestamps {
		if ts != nil && ts.Time != nil && !ts.Time.Start.IsZero() {
			s, _, p := timelineOrgSpan(ts.Time)
			timelineSet(ev, s, 0, p, "timestamp")
			return true
		}
	}
	for _, c := range []struct {
		d   *org.SDC
		src string
	}{{h.Closed, "closed"}, {h.Scheduled, "scheduled"}, {h.Deadline, "deadline"}} {
		if d := timelineSdcDate(c.d); d != nil {
			s, _, p := timelineOrgSpan(d)
			timelineSet(ev, s, 0, p, c.src)
			return true
		}
	}
	if v, ok := ownProp(sec, "CREATED"); ok {
		if d, ok := common.ParseTimelineDate(v); ok {
			timelineSet(ev, d.S, 0, d.Precision, "created")
			return true
		}
	}
	if t, _, ok := timelineDayOf(sec); ok {
		timelineSet(ev, t.UnixMilli(), 0, "day", "datetree")
		return true
	}
	return false
}

// timelineSnippet is the first words of a heading's own text.
func timelineSnippet(lines []string, row int) string {
	if row < 0 || row >= len(lines) {
		return ""
	}
	end := ownLinesEnd(lines, row, len(lines)-1)
	words := []string{}
	n := 0
	inDrawer, inBlock := false, false
	for i := row + 1; i <= end && n < 200; i++ {
		t := strings.TrimSpace(lines[i])
		low := strings.ToLower(t)
		switch {
		case inDrawer:
			if strings.EqualFold(t, ":END:") {
				inDrawer = false
			}
			continue
		case inBlock:
			if strings.HasPrefix(low, "#+end_") {
				inBlock = false
			}
			continue
		case strings.HasPrefix(low, "#+begin_"):
			inBlock = true
			continue
		case regexp.MustCompile(`^:[A-Za-z_-]+:\s*$`).MatchString(t):
			inDrawer = true
			continue
		case t == "" || strings.HasPrefix(t, "#") || timelinePlanRe.MatchString(t) || strings.HasPrefix(t, "|"):
			if t == "" && n > 0 {
				i = end + 1 // the first paragraph only
			}
			continue
		}
		t = timelineLinkRe.ReplaceAllStringFunc(t, func(m string) string {
			p := timelineLinkRe.FindStringSubmatch(m)
			if p[2] != "" {
				return p[2]
			}
			return p[1]
		})
		t = timelineMarkupRe.ReplaceAllString(t, "$1$2$3")
		t = strings.TrimLeft(t, "-+ ")
		words = append(words, t)
		n += len(t)
	}
	s := strings.Join(words, " ")
	if r := []rune(s); len(r) > 180 {
		s = strings.TrimSpace(string(r[:177])) + "…"
	}
	return s
}

// timelineColourOf is a heading's own colour: :COLOR: (a palette name or #hex).
func timelineColourOf(sec *org.Section) string {
	for _, k := range []string{"TIMELINE_COLOR", "TIMELINE_COLOUR", "COLOR", "COLOUR"} {
		if v, ok := ownProp(sec, k); ok && strings.TrimSpace(v) != "" {
			c := strings.ToLower(strings.TrimSpace(v))
			if h, ok := trackerHues[c]; ok {
				return h
			}
			if len(c) == 7 && c[0] == '#' {
				if _, err := strconv.ParseUint(c[1:], 16, 32); err == nil {
					return c
				}
			}
		}
	}
	return ""
}

// ----------------------------------------------------------------------------
// Every event
// ----------------------------------------------------------------------------

// timelineCand is an event with what its lane could be.
type timelineCand struct {
	ev     common.TimelineEvent
	file   string
	parent string
	tags   []string
	prop   string
	own    string // its own colour
}

func timelineFileWanted(ff, name string) bool {
	if ff == "" {
		return true
	}
	if ff == name {
		return true
	}
	if ok, _ := filepath.Match(ff, filepath.Base(name)); ok {
		return true
	}
	return strings.Contains(filepath.ToSlash(name), filepath.ToSlash(ff))
}

func timelineCollect(spec timelineSpec, now time.Time) ([]timelineCand, error) {
	cache := lineCache{}
	titles := map[string]string{}
	out := []timelineCand{}
	propName := strings.TrimPrefix(spec.Group, "property:")
	visit := func(f *common.OrgFile, sec *org.Section) {
		if !isRealHeading(sec) || !timelineFileWanted(spec.File, f.Filename) || timelineHidden(sec) {
			return
		}
		lines := cache.of(f.Filename)
		base := common.TimelineEvent{
			Title: strings.TrimSpace(common.GetSectionTitle(sec)), Keyword: sec.Headline.Status, Tags: sec.Headline.Tags,
			Hash: sec.Hash, File: f.Filename, Line: sec.Headline.Pos.Row, Slot: -1,
		}
		if base.Title == "" {
			base.Title = strings.TrimSpace(headlineTitle(sec))
		}
		if _, ok := titles[f.Filename]; !ok {
			titles[f.Filename] = timelineFileTitle(f.Filename, lines)
		}
		// Own tags first, in the order written (inheritedTags sorts them).
		tags := append([]string{}, sec.Headline.Tags...)
		for _, t := range inheritedTags(sec) {
			if !timelineHas(tags, t) {
				tags = append(tags, t)
			}
		}
		c := timelineCand{file: titles[f.Filename], tags: tags, own: timelineColourOf(sec)}
		if sec.Parent != nil && sec.Parent.Headline != nil {
			c.parent = strings.TrimSpace(common.GetSectionTitle(sec.Parent))
		}
		if strings.HasPrefix(spec.Group, "property:") {
			for s := sec; s != nil && s.Headline != nil; s = s.Parent {
				if v, ok := ownProp(s, propName); ok && strings.TrimSpace(v) != "" {
					c.prop = strings.TrimSpace(v)
					break
				}
			}
		}
		ev := base
		if timelineDated(sec, &ev) {
			if timelineIsEra(sec) && ev.Kind == "span" {
				ev.Kind = "era"
			}
			ev.Snippet = timelineSnippet(lines, ev.Line)
			c.ev = ev
			out = append(out, c)
		}
		if spec.Clocks {
			for _, clk := range sec.Headline.Clocks {
				if clk == nil || clk.Date == nil || clk.Date.Start.IsZero() || clk.Date.End.IsZero() || !clk.Date.End.After(clk.Date.Start) {
					continue
				}
				ce := base
				ce.Line = clk.Pos.Row
				timelineSet(&ce, common.TimelineWall(clk.Date.Start), common.TimelineWall(clk.Date.End), "minute", "clock")
				cc := c
				cc.ev = ce
				out = append(out, cc)
			}
		}
	}
	if spec.Query != "" {
		err := queryEachSection(&common.StringQuery{Query: spec.Query}, func(sec *org.Section, f *common.OrgFile) { visit(f, sec) })
		if err != nil {
			return nil, fmt.Errorf("query %q: %v", spec.Query, err)
		}
	} else {
		walkSections(visit)
	}
	return out, nil
}

// timelineLaneOf is an event's lane under a grouping.
func timelineLaneOf(c timelineCand, group string) string {
	switch {
	case group == "file":
		return c.file
	case group == "parent":
		if c.parent == "" {
			return c.file
		}
		return c.parent
	case group == "tag":
		if len(c.tags) == 0 {
			return "untagged"
		}
		return c.tags[0]
	case group == "keyword":
		if c.ev.Keyword == "" {
			return "no keyword"
		}
		return c.ev.Keyword
	case strings.HasPrefix(group, "property:"):
		if c.prop == "" {
			return "no " + strings.ToLower(strings.TrimPrefix(group, "property:"))
		}
		return c.prop
	}
	return ""
}

// timelineAuto is the grouping auto picks: files when there are several,
// else the parent headings when they differ, else one lane.
func timelineAuto(cs []timelineCand) string {
	files, parents := map[string]bool{}, map[string]bool{}
	for _, c := range cs {
		if c.ev.Kind == "era" {
			continue
		}
		files[c.ev.File] = true
		parents[c.parent] = true
	}
	switch {
	case len(files) > 1:
		return "file"
	case len(parents) > 1:
		return "parent"
	}
	return "none"
}

func timelineEvents(spec timelineSpec, now time.Time) (common.TimelineData, error) {
	d := common.TimelineData{Ok: true, Title: spec.Title, Query: spec.Query, From: spec.FromS, To: spec.ToS, S: spec.From, E: spec.To, Height: spec.Height}
	cs, err := timelineCollect(spec, now)
	if err != nil {
		return d, err
	}
	// The range.
	kept := cs[:0]
	for _, c := range cs {
		last := c.ev.S
		if c.ev.Kind != "point" && c.ev.E > last {
			last = c.ev.E
		}
		if (spec.From != 0 || spec.FromS != "") && last < spec.From {
			continue
		}
		if (spec.To != 0 || spec.ToS != "") && c.ev.S >= spec.To {
			continue
		}
		kept = append(kept, c)
	}
	cs = kept
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].ev.S != cs[j].ev.S {
			return cs[i].ev.S < cs[j].ev.S
		}
		return cs[i].ev.Title < cs[j].ev.Title
	})
	if len(cs) > spec.Limit {
		// The most recent are kept.
		cs = cs[len(cs)-spec.Limit:]
		d.Truncated = true
	}
	group := spec.Group
	if group == "auto" {
		group = timelineAuto(cs)
	}
	d.Group = group
	lanes := map[string]int{}
	d.Events = make([]common.TimelineEvent, 0, len(cs))
	d.Lanes = []common.TimelineLane{}
	for _, c := range cs {
		ev := c.ev
		if ev.Kind == "era" {
			ev.Lane = ""
			ev.Colour = c.own
			d.Events = append(d.Events, ev)
			continue
		}
		ev.Lane = timelineLaneOf(c, group)
		i, ok := lanes[ev.Lane]
		if !ok {
			i = len(d.Lanes)
			lanes[ev.Lane] = i
			l := common.TimelineLane{Name: ev.Lane, Slot: -1, Colour: timelineGrey}
			if i < len(timelineHues) {
				l.Slot, l.Colour = i, timelineHues[i]
			}
			d.Lanes = append(d.Lanes, l)
		}
		d.Lanes[i].Count++
		ev.Slot, ev.Colour = d.Lanes[i].Slot, d.Lanes[i].Colour
		if c.own != "" {
			ev.Slot, ev.Colour = -1, c.own
		}
		d.Events = append(d.Events, ev)
	}
	return d, nil
}

// timelineFor reads a line and finds its events.
func timelineFor(line, here string, now time.Time) (common.TimelineData, timelineSpec, error) {
	spec, err := parseTimelineSpec(line, here, now)
	if err != nil {
		return common.TimelineData{Events: []common.TimelineEvent{}, Lanes: []common.TimelineLane{}}, spec, err
	}
	d, err := timelineEvents(spec, now)
	return d, spec, err
}

// ----------------------------------------------------------------------------
// Every #+TIMELINE: line
// ----------------------------------------------------------------------------

func timelineLinesIn(lines []string) []common.TimelineLine {
	out := []common.TimelineLine{}
	var inBlock string
	for i, l := range lines {
		t := strings.ToLower(strings.TrimSpace(l))
		if inBlock != "" {
			if strings.HasPrefix(t, "#+end_"+inBlock) {
				inBlock = ""
			}
			continue
		}
		if strings.HasPrefix(t, "#+begin_") {
			inBlock = strings.Fields(strings.TrimPrefix(t, "#+begin_") + " ")[0]
			continue
		}
		if m := timelineLineRe.FindStringSubmatch(l); m != nil {
			out = append(out, common.TimelineLine{Line: i, Options: strings.TrimSpace(m[2])})
		} else if m := timelineBeginRe.FindStringSubmatch(l); m != nil {
			out = append(out, common.TimelineLine{Line: i, Options: strings.TrimSpace(m[1]), Block: true})
		}
	}
	return out
}

func timelineList() []common.TimelineLine {
	out := []common.TimelineLine{}
	cache := lineCache{}
	for _, name := range GetDb().GetFiles() {
		lines := cache.of(name)
		if !strings.Contains(strings.ToLower(strings.Join(lines, "\n")), "timeline") {
			continue
		}
		f := GetDb().FindByFile(name)
		for _, t := range timelineLinesIn(lines) {
			t.Filename = name
			t.Title = common.TrackerGet(common.ParseTrackerOptions(t.Options), "title", "")
			if f != nil {
				if sec := sectionAtRow(f, t.Line); sec != nil && sec.Headline != nil {
					t.Hash, t.Headline = sec.Hash, strings.TrimSpace(headlineTitle(sec))
				}
			}
			if t.Title == "" {
				t.Title = t.Headline
			}
			if t.Title == "" {
				t.Title = timelineFileTitle(name, lines)
			}
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Filename != out[j].Filename {
			return out[i].Filename < out[j].Filename
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// ----------------------------------------------------------------------------
// Endpoints
// ----------------------------------------------------------------------------

func registerTimelineRoutes(api *mux.Router) {
	api.HandleFunc("/timeline/events", RequestTimelineEvents).Methods("GET")
	api.HandleFunc("/timeline/list", RequestTimelineList).Methods("GET")
	api.HandleFunc("/timeline/svg", RequestTimelineSvg).Methods("GET")
	api.HandleFunc("/timeline/date", RequestTimelineDate).Methods("GET")
}

// timelineLineOf is a request's options: line= (a #+TIMELINE: line's), with
// query=, group=, from=, to=, file=, title=, clocks= on top; here= the file
// the line is in.
func timelineLineOf(r *http.Request) (string, string) {
	q := r.URL.Query()
	opts := common.ParseTrackerOptions(strings.TrimSpace(q.Get("line")))
	if m := timelineLineRe.FindStringSubmatch(q.Get("line")); m != nil {
		opts = common.ParseTrackerOptions(m[2])
	}
	for _, k := range []string{"query", "group", "from", "to", "file", "title", "clocks", "height", "limit"} {
		if v := q.Get(k); v != "" {
			opts = append(opts, common.TrackerOption{Key: k, Value: v})
		}
	}
	return common.FormatTrackerOptions(opts), q.Get("here")
}

/* SDOC: API
* GET /timeline/events — Dated Headings As Events
* GET /timeline/list — Every #+TIMELINE: Line
* GET /timeline/svg — A Timeline As A Picture
* GET /timeline/date — How A :DATE: Reads

	#+BEGIN_EXAMPLE
	GET /timeline/events?query=HasTags(%22history%22)&group=tag&from=-500&to=1500
	    {"Ok": true, "Group": "tag", "S": -77914137600000, "E": ...,
	     "Events": [{"Title": "Battle of Hastings", "Kind": "point", "Source": "date",
	                 "Start": "1066-10-14", "End": "", "S": -28502755200000, "E": 0,
	                 "Precision": "day", "Label": "14 Oct 1066", "Lane": "england",
	                 "Colour": "#2a78d6", "Slot": 0, "Hash": "...", "File": "...", "Line": 12,
	                 "Keyword": "", "Tags": ["england"], "Snippet": "William of Normandy ..."}, ...],
	     "Lanes": [{"Name": "england", "Colour": "#2a78d6", "Slot": 0, "Count": 14}, ...]}
	GET /timeline/list   {"Ok": true, "Timelines": [{Filename, Line, Options, Block, Title, Hash, Headline}]}
	GET /timeline/svg?line=group:tag from:1900&width=960     image/svg+xml
	GET /timeline/date?text=c. 1200
	    {"Ok": true, "Start": "1200", "S": ..., "Until": ..., "Precision": "year", "Approx": true, "Label": "c. 1200"}
	#+END_EXAMPLE

	=line= is what follows =#+TIMELINE:=; =query=, =group=, =from=, =to=,
	=file=, =title=, =clocks= as parameters go on top of it, and =here= is
	the file the line is in (a line with no =query:= draws its own file).
	With neither, every dated heading in every file is an event (the most
	recent 2000, =Truncated= saying when there were more; =limit= changes it).
	=S=/=E= are positions: milliseconds of the date's wall-clock fields read
	as UTC (never an instant), astronomical years (=-0043= is 44 BC), so a
	client draws them without a time zone. =Kind= is =point=, =span= or
	=era= (eras have no lane; they are drawn behind every lane). =Source= is
	where the date came from: =date= (the =:DATE:= property), =range= (an
	org date range), =planned= (SCHEDULED to DEADLINE), =timestamp=,
	=closed=, =scheduled=, =deadline=, =created=, =datetree=, =clock=. =Slot=
	is the lane's place in the chart palette, -1 for a heading's own
	=:COLOR:= or a lane past the eighth. =/timeline/date= reads a free date
	as =:DATE:= does, for an editor's hover. Needs =features: timeline:=.
EDOC */
func RequestTimelineEvents(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, timelineFeature) {
		return
	}
	line, here := timelineLineOf(r)
	d, _, err := timelineFor(line, here, time.Now())
	if err != nil {
		d.Ok, d.Msg = false, err.Error()
	}
	if d.Events == nil {
		d.Events = []common.TimelineEvent{}
	}
	if d.Lanes == nil {
		d.Lanes = []common.TimelineLane{}
	}
	dynJson(w, d)
}

func RequestTimelineList(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, timelineFeature) {
		return
	}
	dynJson(w, common.TimelineList{Ok: true, Timelines: timelineList()})
}

func RequestTimelineDate(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, timelineFeature) {
		return
	}
	text := r.URL.Query().Get("text")
	d, ok := common.ParseTimelineDate(text)
	if !ok {
		dynJson(w, common.TimelineDateReading{Msg: fmt.Sprintf("%q does not read as a date", text)})
		return
	}
	dynJson(w, common.TimelineDateReading{Ok: true, TimelineDate: d})
}

func RequestTimelineSvg(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store")
	if !featureOn(timelineFeature) {
		io.WriteString(w, trackerErrorSvg("Timelines are turned off: orgs features timeline on"))
		return
	}
	width, _ := strconv.Atoi(r.URL.Query().Get("width"))
	line, here := timelineLineOf(r)
	now := time.Now()
	d, _, err := timelineFor(line, here, now)
	if err != nil {
		io.WriteString(w, trackerErrorSvg(err.Error()))
		return
	}
	io.WriteString(w, timelineSvg(d, width, now))
}

// ----------------------------------------------------------------------------
// #+BEGIN: timeline - the events as an org table
// ----------------------------------------------------------------------------

// timelineOrgLink is a link to a heading for a table in file from.
func timelineOrgLink(ev common.TimelineEvent, from string) string {
	desc := strings.NewReplacer("[", "(", "]", ")").Replace(ev.Title)
	if sec, f := timelineSection(ev); sec != nil {
		if id, ok := ownProp(sec, "ID"); ok && strings.TrimSpace(id) != "" {
			return "[[id:" + strings.TrimSpace(id) + "][" + desc + "]]"
		}
		_ = f
	}
	target := "*" + ev.Title
	if ev.File != from {
		rel := ev.File
		if r, err := filepath.Rel(filepath.Dir(from), ev.File); err == nil {
			rel = r
		}
		return "[[file:" + rel + "::" + target + "][" + desc + "]]"
	}
	return "[[" + target + "][" + desc + "]]"
}

func timelineSection(ev common.TimelineEvent) (*org.Section, *common.OrgFile) {
	f := GetDb().FindByFile(ev.File)
	if f == nil {
		return nil, nil
	}
	return sectionAtRow(f, ev.Line), f
}

func dynTimeline(c *dynCtx) ([]string, error) {
	if !featureOn(timelineFeature) {
		return nil, fmt.Errorf("timelines are turned off: `orgs features timeline on`")
	}
	d, _, err := timelineFor(c.Block.Header, c.File.Filename, c.Now)
	if err != nil {
		return nil, err
	}
	cell := func(s string) string { return strings.ReplaceAll(s, "|", "\\vert{}") }
	table := [][]string{{"Date", "Event", "Lane"}, nil}
	for _, ev := range d.Events {
		lane := ev.Lane
		if ev.Kind == "era" {
			lane = "era"
		}
		table = append(table, []string{cell(ev.Label), cell(timelineOrgLink(ev, c.File.Filename)), cell(lane)})
	}
	caption := fmt.Sprintf("#+CAPTION: %s, %d event%s, at [%s].", timelineCaption(d), len(d.Events), plural(len(d.Events)), c.Now.Format("2006-01-02 Mon 15:04"))
	return append([]string{caption}, orgTableLines(table)...), nil
}

func timelineCaption(d common.TimelineData) string {
	if d.Title != "" {
		return d.Title
	}
	return "Timeline"
}

// ----------------------------------------------------------------------------
// The export copy: each #+TIMELINE: line becomes its picture
// ----------------------------------------------------------------------------

func timelineMayApply(text string) bool {
	return featureOn(timelineFeature) && strings.Contains(strings.ToLower(text), "#+timeline:")
}

// timelineExportLines replaces each #+TIMELINE: line with what the backend
// can show: the svg in html (in a figure carrying the options, which worg's
// rendered view makes live) and typst, an org list of the events otherwise.
func timelineExportLines(path string, lines []string, backend string) ([]string, bool) {
	kind := trackerBackendKind(backend)
	at := map[int]common.TimelineLine{}
	for _, t := range timelineLinesIn(lines) {
		if !t.Block {
			at[t.Line] = t
		}
	}
	if len(at) == 0 {
		return lines, false
	}
	now := time.Now()
	out := make([]string, 0, len(lines))
	for i, l := range lines {
		t, ok := at[i]
		if !ok {
			out = append(out, l)
			continue
		}
		ind := timelineLineRe.FindStringSubmatch(l)[1]
		d, _, err := timelineFor(t.Options, path, now)
		if err != nil {
			fmt.Fprintf(os.Stderr, "timeline: %s:%d: %v\n", filepath.Base(path), i+1, err)
			out = append(out, ind+"/Timeline: "+strings.ReplaceAll(err.Error(), "/", "")+"/")
			continue
		}
		switch kind {
		case "html":
			fig := fmt.Sprintf(`<figure class="org-timeline" data-timeline="%s" data-timeline-file="%s">`, htmlAttr(t.Options), htmlAttr(path))
			out = append(out, ind+"#+BEGIN_EXPORT html", fig+timelineSvg(d, 0, now)+`</figure>`, ind+"#+END_EXPORT")
		case "typst":
			svg := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(timelineSvg(d, 0, now))
			out = append(out, ind+"#+BEGIN_EXPORT typst", `#figure(image(bytes("`+svg+`"), format: "svg", width: 100%))`, ind+"#+END_EXPORT")
		default:
			out = append(out, timelineListLines(d, ind)...)
		}
	}
	return out, true
}

func htmlAttr(s string) string {
	return strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// timelineListLines is a timeline as an org list, oldest first: the date, the
// event, its lane.
func timelineListLines(d common.TimelineData, ind string) []string {
	out := []string{}
	if d.Title != "" {
		out = append(out, ind+"*"+d.Title+"*", "")
	}
	if len(d.Events) == 0 {
		return append(out, ind+"/No dated headings for this timeline./")
	}
	for _, ev := range d.Events {
		item := ind + "- " + ev.Label + " :: " + ev.Title
		switch {
		case ev.Kind == "era":
			item += " (era)"
		case ev.Lane != "" && len(d.Lanes) > 1:
			item += " (" + ev.Lane + ")"
		}
		out = append(out, item)
	}
	return out
}

// timelineFileTitle is a file's #+TITLE:, else its name without the extension.
func timelineFileTitle(path string, lines []string) string {
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if len(t) > 8 && strings.EqualFold(t[:8], "#+title:") {
			if v := strings.TrimSpace(t[8:]); v != "" {
				return v
			}
		}
		if t != "" && !strings.HasPrefix(t, "#") && !strings.HasPrefix(t, ":") {
			break
		}
	}
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func timelineHas(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
