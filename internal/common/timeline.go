package common

// Timelines (features.timeline): the
// wire types, and the two pieces of the timeline both the server and its
// clients need - reading a free date (the :DATE: property history notes are
// dated with) and the axis's ticks at any scale, from minutes to millennia.
//
// Dates on a timeline are wall-clock fields, never instants (see the root
// CLAUDE.md, "Org dates cross the wire as wall-clock fields"): a date's
// position is the milliseconds from 1970-01-01 of its fields read as if they
// were UTC. Years are astronomical underneath (year 0 is 1 BC, -43 is 44 BC),
// which Go's time and JavaScript's Date both count in, and historians' years
// on the way in and out: `-44` and `44 BC` are both 44 BC.
//
// ParseTimelineDate ↔ parseHistDate, TimelineTicks ↔ timelineTicks in
// worg/src/orgtimeline.ts, and ParseTimelineDate ↔ parseHistDate in
// vscode-org-everywhere/src/timeline-model.ts: the same cases in
// timeline_test.go, orgtimeline.test.ts and test/timeline-model.test.ts.

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TimelineEvent is one dated heading on a timeline.
type TimelineEvent struct {
	Title   string
	Keyword string   `json:",omitempty"`
	Tags    []string `json:",omitempty"`
	Hash    string
	File    string
	Line    int
	Snippet string `json:",omitempty"`
	// point, span or era.
	Kind string
	// Where the date came from: date (the :DATE: property), range (an org
	// date range), planned (SCHEDULED to DEADLINE), timestamp, closed,
	// scheduled, deadline, created, datetree, clock.
	Source string
	// Start and End as ISO fields (astronomical years, -0043-03-15), End
	// empty for a point.
	Start, End string
	// The same as milliseconds of wall-clock time read as UTC, for drawing.
	S, E int64
	// How finely the date is known: minute, day, month, year, decade, century.
	Precision string
	Approx    bool `json:",omitempty"`
	// The date as a person reads it: "14 Oct 1066", "c. 1200", "1939 – 1945".
	Label  string
	Lane   string
	Colour string
	// The lane's place in the chart palette (0-7), -1 for a colour of the
	// heading's own or a lane past the eighth.
	Slot int
}

// TimelineLane is one row of a timeline.
type TimelineLane struct {
	Name   string
	Colour string
	Slot   int
	Count  int
}

// TimelineData is GET /timeline/events.
type TimelineData struct {
	Ok        bool
	Off       bool   `json:",omitempty"`
	Msg       string `json:",omitempty"`
	Title     string `json:",omitempty"`
	Query     string `json:",omitempty"`
	Group     string
	From, To  string `json:",omitempty"`
	S, E      int64  `json:",omitempty"` // the asked for range, as positions; 0 when open
	Height    int    `json:",omitempty"`
	Events    []TimelineEvent
	Lanes     []TimelineLane
	Truncated bool `json:",omitempty"`
}

// TimelineLine is a #+TIMELINE: line (or a #+BEGIN: timeline block) in a file.
type TimelineLine struct {
	Filename string
	Line     int
	Options  string
	Block    bool   `json:",omitempty"`
	Title    string `json:",omitempty"`
	Hash     string `json:",omitempty"`
	Headline string `json:",omitempty"`
}

type TimelineList struct {
	Ok        bool
	Off       bool   `json:",omitempty"`
	Msg       string `json:",omitempty"`
	Timelines []TimelineLine
}

// TimelineDateReading is GET /timeline/date: how a :DATE: value reads.
type TimelineDateReading struct {
	Ok bool
	TimelineDate
	Msg string `json:",omitempty"`
}

// TimelineGroups are the ways events are put into lanes.
var TimelineGroups = []string{"auto", "tag", "file", "parent", "keyword", "property:NAME", "none"}

// ---------------------------------------------------------------------------
// Free dates
// ---------------------------------------------------------------------------

// TimelineDate is a free date as read.
type TimelineDate struct {
	Start, End string
	S, E       int64
	// The end of the last period it names, exclusive: 1946-01-01 for 1945
	// (what to:1945 means), E for a period.
	Until     int64
	HasEnd    bool
	Precision  string
	Approx     bool
	Label      string
}

var (
	tdApproxRe  = regexp.MustCompile(`(?i)^(?:circa\s*|ca\.\s*|ca\s+|c\.\s*|c\s+|about\s+|around\s+|approx\.?\s*|~\s*)`)
	tdEraRe     = regexp.MustCompile(`(?i)\s*(b\.?\s?c\.?\s?e?\.?|a\.?\s?d\.?|c\.?\s?e\.?)$`)
	tdEraPreRe  = regexp.MustCompile(`(?i)^(a\.?\s?d\.?)\s*`)
	tdStampRe   = regexp.MustCompile(`^[<\[](-?\d{1,6})-(\d{2})-(\d{2})(?:\s+[^\s\d>\]]+)?(?:\s+(\d{1,2}):(\d{2}))?[^>\]]*[>\]]$`)
	tdIsoRe     = regexp.MustCompile(`^(-?)(\d{1,6})(?:-(\d{1,2})(?:-(\d{1,2})(?:[T\s]+(\d{1,2}):(\d{2}))?)?)?$`)
	tdDecadeRe  = regexp.MustCompile(`^(\d{1,5}0)'?s$`)
	tdCenturyRe = regexp.MustCompile(`(?i)^(\d{1,3})(?:st|nd|rd|th)\s+(?:century|cent\.?|c\.?)$`)
	tdDMYRe     = regexp.MustCompile(`(?i)^(\d{1,2})\s+([a-z]{3,9})\.?,?\s+(\d{1,6})$`)
	tdMDYRe     = regexp.MustCompile(`(?i)^([a-z]{3,9})\.?\s+(\d{1,2}),?\s+(\d{1,6})$`)
	tdMYRe      = regexp.MustCompile(`(?i)^([a-z]{3,9})\.?\s+(\d{1,6})$`)
	tdYearRngRe = regexp.MustCompile(`^(-?\d{3,6})-(\d{3,6})$`)
)

var tdMonths = []string{"jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"}
var tdMonthNames = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

func tdMonth(s string) int {
	s = strings.ToLower(s)
	if len(s) < 3 {
		return 0
	}
	for i, m := range tdMonths {
		if strings.HasPrefix(s, m) {
			full := strings.ToLower(time.Month(i + 1).String())
			if strings.HasPrefix(full, s) || (s == "sept") {
				return i + 1
			}
		}
	}
	return 0
}

// TimelineMaxYear bounds a date's year: JavaScript's Date ends near 275760.
const TimelineMaxYear = 270000

// tdPart is one end of a date: astronomical fields and how fine.
type tdPart struct {
	y, mo, d, h, mi int
	prec            string
	approx          bool
}

func (p tdPart) ms() int64 {
	return time.Date(p.y, time.Month(p.mo), p.d, p.h, p.mi, 0, 0, time.UTC).UnixMilli()
}

// next is the start of the period after this one, at its precision: the
// exclusive end of "1066", "Oct 1066", "1950s".
func (p tdPart) next() tdPart {
	q := p
	switch p.prec {
	case "minute":
		t := time.Date(p.y, time.Month(p.mo), p.d, p.h, p.mi+1, 0, 0, time.UTC)
		q.y, q.mo, q.d, q.h, q.mi = t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute()
	case "day":
		t := time.Date(p.y, time.Month(p.mo), p.d+1, 0, 0, 0, 0, time.UTC)
		q.y, q.mo, q.d = t.Year(), int(t.Month()), t.Day()
	case "month":
		t := time.Date(p.y, time.Month(p.mo+1), 1, 0, 0, 0, 0, time.UTC)
		q.y, q.mo, q.d = t.Year(), int(t.Month()), 1
	case "year":
		q.y, q.mo, q.d = p.y+1, 1, 1
	case "decade":
		q.y, q.mo, q.d = p.y+10, 1, 1
	case "century":
		q.y, q.mo, q.d = p.y+100, 1, 1
	}
	return q
}

// histYear is an astronomical year as a historian writes it: 44 BC for -43.
func tdHistYear(y int) string {
	if y <= 0 {
		return strconv.Itoa(1-y) + " BC"
	}
	return strconv.Itoa(y)
}

// isoYear is an astronomical year in an ISO date: -0043, 1066.
func tdIsoYear(y int) string {
	if y < 0 {
		return fmt.Sprintf("-%04d", -y)
	}
	return fmt.Sprintf("%04d", y)
}

func tdOrdinal(n int) string {
	suf := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suf = "st"
		case 2:
			suf = "nd"
		case 3:
			suf = "rd"
		}
	}
	return strconv.Itoa(n) + suf
}

func (p tdPart) label() string {
	var s string
	switch p.prec {
	case "minute":
		s = fmt.Sprintf("%d %s %s %02d:%02d", p.d, tdMonthNames[p.mo-1], tdHistYear(p.y), p.h, p.mi)
	case "day":
		s = fmt.Sprintf("%d %s %s", p.d, tdMonthNames[p.mo-1], tdHistYear(p.y))
	case "month":
		s = tdMonthNames[p.mo-1] + " " + tdHistYear(p.y)
	case "decade":
		s = strconv.Itoa(p.y) + "s"
	case "century":
		if p.y <= 0 {
			s = tdOrdinal((-p.y)/100+1) + " century BC"
		} else {
			s = tdOrdinal((p.y-1)/100+1) + " century"
		}
	default:
		s = tdHistYear(p.y)
	}
	if p.approx {
		s = "c. " + s
	}
	return s
}

func (p tdPart) iso() string {
	s := tdIsoYear(p.y)
	switch p.prec {
	case "minute":
		return fmt.Sprintf("%s-%02d-%02dT%02d:%02d", s, p.mo, p.d, p.h, p.mi)
	case "day":
		return fmt.Sprintf("%s-%02d-%02d", s, p.mo, p.d)
	case "month":
		return fmt.Sprintf("%s-%02d", s, p.mo)
	}
	return s
}

// astro is a historian's year as an astronomical one: era BC (or a minus
// sign) counts back from 1 BC = 0.
func tdAstro(n int, bc bool) int {
	if bc {
		return 1 - n
	}
	return n
}

// parseTdPart reads one end of a date.
func parseTdPart(s string) (tdPart, bool) {
	s = strings.TrimSpace(s)
	p := tdPart{mo: 1, d: 1}
	if m := tdApproxRe.FindString(s); m != "" {
		p.approx = true
		s = strings.TrimSpace(s[len(m):])
	}
	if strings.HasSuffix(s, "?") {
		p.approx = true
		s = strings.TrimSpace(strings.TrimSuffix(s, "?"))
	}
	if m := tdStampRe.FindStringSubmatch(s); m != nil {
		p.y, _ = strconv.Atoi(m[1])
		p.mo, _ = strconv.Atoi(m[2])
		p.d, _ = strconv.Atoi(m[3])
		p.prec = "day"
		if m[4] != "" {
			p.h, _ = strconv.Atoi(m[4])
			p.mi, _ = strconv.Atoi(m[5])
			p.prec = "minute"
		}
		return p, p.valid()
	}
	bc := false
	if m := tdEraRe.FindStringSubmatch(s); m != nil && s != m[0] {
		e := strings.ToLower(strings.NewReplacer(".", "", " ", "").Replace(m[1]))
		bc = strings.HasPrefix(e, "b")
		s = strings.TrimSpace(s[:len(s)-len(m[0])])
	} else if m := tdEraPreRe.FindString(s); m != "" && s != m {
		s = strings.TrimSpace(s[len(m):])
	}
	if m := tdCenturyRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		if n < 1 {
			return p, false
		}
		p.prec = "century"
		if bc {
			// The 5th century BC is 500 BC to 401 BC.
			p.y = tdAstro(n*100, true)
		} else {
			p.y = (n-1)*100 + 1
		}
		return p, p.valid()
	}
	if m := tdDecadeRe.FindStringSubmatch(s); m != nil && !bc {
		p.y, _ = strconv.Atoi(m[1])
		p.prec = "decade"
		return p, p.valid()
	}
	if m := tdDMYRe.FindStringSubmatch(s); m != nil {
		p.d, _ = strconv.Atoi(m[1])
		p.mo = tdMonth(m[2])
		y, _ := strconv.Atoi(m[3])
		p.y, p.prec = tdAstro(y, bc), "day"
		return p, p.mo > 0 && p.valid()
	}
	if m := tdMDYRe.FindStringSubmatch(s); m != nil {
		p.mo = tdMonth(m[1])
		p.d, _ = strconv.Atoi(m[2])
		y, _ := strconv.Atoi(m[3])
		p.y, p.prec = tdAstro(y, bc), "day"
		return p, p.mo > 0 && p.valid()
	}
	if m := tdMYRe.FindStringSubmatch(s); m != nil {
		p.mo = tdMonth(m[1])
		y, _ := strconv.Atoi(m[2])
		p.y, p.prec = tdAstro(y, bc), "month"
		return p, p.mo > 0 && p.valid()
	}
	if m := tdIsoRe.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[2])
		if m[1] == "-" {
			if bc {
				return p, false
			}
			bc = true
		}
		p.y, p.prec = tdAstro(y, bc), "year"
		if m[3] != "" {
			p.mo, _ = strconv.Atoi(m[3])
			p.prec = "month"
		}
		if m[4] != "" {
			p.d, _ = strconv.Atoi(m[4])
			p.prec = "day"
		}
		if m[5] != "" {
			p.h, _ = strconv.Atoi(m[5])
			p.mi, _ = strconv.Atoi(m[6])
			p.prec = "minute"
		}
		return p, p.valid()
	}
	return p, false
}

func (p tdPart) valid() bool {
	if p.y > TimelineMaxYear || p.y < -TimelineMaxYear || p.mo < 1 || p.mo > 12 || p.d < 1 || p.h > 23 || p.mi > 59 {
		return false
	}
	// No 31 Feb.
	t := time.Date(p.y, time.Month(p.mo), p.d, 0, 0, 0, 0, time.UTC)
	return t.Day() == p.d
}

var tdRangeSeps = []string{"--", "–", "—", " to ", "..", " - ", " until "}

// ParseTimelineDate reads a free date: an org timestamp, 1066, 1066-10,
// 1066-10-14, 1066-10-14 09:00, 14 October 1066, Oct 14, 1066, -500 or 500
// BC (there is no year 0: -1 is 1 BC), AD 800, c. 1200 / ca. / circa / ~ /
// 1200?, 1950s, 12th century (BC), and a range of any two of them joined by
// --, –, " to ", .. or " - " (1939-1945 too). A period - a decade, a
// century, a range - has an end; a single day or year is a point at its
// start.
func ParseTimelineDate(s string) (TimelineDate, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return TimelineDate{}, false
	}
	var a, b tdPart
	ranged := false
	if m := tdYearRngRe.FindStringSubmatch(s); m != nil {
		if n, _ := strconv.Atoi(m[2]); n > 12 {
			var ok1, ok2 bool
			a, ok1 = parseTdPart(m[1])
			b, ok2 = parseTdPart(m[2])
			if !ok1 || !ok2 {
				return TimelineDate{}, false
			}
			ranged = true
		}
	}
	if !ranged {
		for _, sep := range tdRangeSeps {
			low := strings.ToLower(s)
			i := strings.Index(low, sep)
			if i <= 0 {
				continue
			}
			l, r := strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+len(sep):])
			pa, ok1 := parseTdPart(l)
			pb, ok2 := parseTdPart(r)
			if !ok1 || !ok2 {
				// "500 - 400 BC": the era written once, at the end.
				if ok2 && tdEraRe.MatchString(r) {
					if m := tdEraRe.FindStringSubmatch(r); m != nil {
						pa, ok1 = parseTdPart(l + " " + m[1])
					}
				}
				if !ok1 || !ok2 {
					continue
				}
			} else if m := tdEraRe.FindStringSubmatch(r); m != nil && !tdEraRe.MatchString(l) && strings.HasPrefix(strings.ToLower(strings.TrimSpace(m[1])), "b") {
				pa, _ = parseTdPart(l + " " + m[1])
			}
			a, b, ranged = pa, pb, true
			break
		}
	}
	if !ranged {
		p, ok := parseTdPart(s)
		if !ok {
			return TimelineDate{}, false
		}
		d := TimelineDate{Start: p.iso(), S: p.ms(), Until: p.next().ms(), Precision: p.prec, Approx: p.approx, Label: p.label()}
		if p.prec == "decade" || p.prec == "century" {
			e := p.next()
			d.HasEnd, d.E = true, e.ms()
			d.End = e.iso()
		}
		return d, true
	}
	e := b.next()
	if e.ms() <= a.ms() {
		return TimelineDate{}, false
	}
	prec := a.prec
	if tdCoarser(b.prec, prec) {
		prec = b.prec
	}
	d := TimelineDate{Start: a.iso(), End: e.iso(), S: a.ms(), E: e.ms(), Until: e.ms(), HasEnd: true, Precision: prec,
		Approx: a.approx || b.approx, Label: a.label() + " – " + b.label()}
	return d, true
}

var tdPrecOrder = map[string]int{"minute": 0, "day": 1, "month": 2, "year": 3, "decade": 4, "century": 5}

func tdCoarser(a, b string) bool { return tdPrecOrder[a] > tdPrecOrder[b] }

// TimelineWall is a time's wall-clock fields as a position (read as UTC).
func TimelineWall(t time.Time) int64 {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC).UnixMilli()
}

// TimelineFields is a position's fields again.
func TimelineFields(ms int64) time.Time {
	return time.UnixMilli(ms).UTC()
}

// TimelineFormat is a position as a date label at a precision.
func TimelineFormat(ms int64, prec string) string {
	t := TimelineFields(ms)
	return tdPart{y: t.Year(), mo: int(t.Month()), d: t.Day(), h: t.Hour(), mi: t.Minute(), prec: prec}.label()
}

// TimelineISO is a position as ISO fields at a precision.
func TimelineISO(ms int64, prec string) string {
	t := TimelineFields(ms)
	return tdPart{y: t.Year(), mo: int(t.Month()), d: t.Day(), h: t.Hour(), mi: t.Minute(), prec: prec}.iso()
}

// ---------------------------------------------------------------------------
// The axis
// ---------------------------------------------------------------------------

// TimelineTick is one mark on the axis.
type TimelineTick struct {
	T     int64
	Label string
	Major bool
}

type tlStep struct {
	unit string // minute, hour, day, week, month, year
	n    int
	ms   float64 // about how long
}

const tlDay = 86400000.0

var tlSteps = func() []tlStep {
	s := []tlStep{
		{"minute", 1, 60e3}, {"minute", 5, 300e3}, {"minute", 15, 900e3}, {"minute", 30, 1800e3},
		{"hour", 1, 3600e3}, {"hour", 3, 3 * 3600e3}, {"hour", 6, 6 * 3600e3}, {"hour", 12, 12 * 3600e3},
		{"day", 1, tlDay}, {"day", 2, 2 * tlDay}, {"week", 1, 7 * tlDay},
		{"month", 1, 30.44 * tlDay}, {"month", 3, 91.3 * tlDay}, {"month", 6, 182.6 * tlDay},
	}
	for _, y := range []int{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000, 20000, 50000, 100000} {
		s = append(s, tlStep{"year", y, float64(y) * 365.25 * tlDay})
	}
	return s
}()

// TimelineStepFor is the step ticks are drawn at for a scale: the finest
// whose ticks are at least minPx apart. Unit and count, e.g. "month", 3.
func TimelineStepFor(pxPerMs, minPx float64) (string, int) {
	for _, s := range tlSteps {
		if s.ms*pxPerMs >= minPx {
			return s.unit, s.n
		}
	}
	last := tlSteps[len(tlSteps)-1]
	return last.unit, last.n
}

// The year a historian labels an astronomical year with (-1 for 1 BC), and
// back: a tick at label value 0 is drawn at AD 1.
func tlLabelYear(y int) int {
	if y <= 0 {
		return y - 1
	}
	return y
}

func tlAstroYear(v int) int {
	switch {
	case v > 0:
		return v
	case v == 0:
		return 1
	}
	return v + 1
}

func tlYearLabel(v int) string {
	if v < 0 {
		return strconv.Itoa(-v) + " BC"
	}
	if v == 0 {
		return "1"
	}
	return strconv.Itoa(v)
}

func tlFloorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// TimelineTicks is the axis between two positions drawn width pixels wide,
// ticks at least minPx apart. A tick's label says only what changed: 09:00,
// then Nov 4 at midnight; Nov 4 by the day, 2026 on New Year's Day; Mar by the
// month, 2026 in January; years as years, BC before 1.
func TimelineTicks(from, to int64, width, minPx float64) []TimelineTick {
	out := []TimelineTick{}
	if to <= from || width <= 0 {
		return out
	}
	if minPx <= 0 {
		minPx = 80
	}
	unit, n := TimelineStepFor(width/float64(to-from), minPx)
	start := TimelineFields(from)
	end := TimelineFields(to)
	add := func(t time.Time, label string, major bool) {
		ms := t.UnixMilli()
		if ms >= from && ms <= to {
			out = append(out, TimelineTick{ms, label, major})
		}
	}
	dayLabel := func(t time.Time) string {
		if t.Month() == 1 && t.Day() == 1 {
			return tlYearLabel(tlLabelYear(t.Year()))
		}
		return tdMonthNames[t.Month()-1] + " " + strconv.Itoa(t.Day())
	}
	const limit = 2000
	switch unit {
	case "minute", "hour":
		stepMin := n
		if unit == "hour" {
			stepMin = n * 60
		}
		t := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		for i := 0; !t.After(end) && i < 100000; i++ {
			mins := t.Hour()*60 + t.Minute()
			if mins%stepMin == 0 && !t.Before(start) {
				if mins == 0 {
					add(t, dayLabel(t), true)
				} else {
					add(t, fmt.Sprintf("%02d:%02d", t.Hour(), t.Minute()), false)
				}
				if len(out) > limit {
					break
				}
			}
			// Jump straight to the next multiple.
			next := (mins/stepMin + 1) * stepMin
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, next, 0, 0, time.UTC)
		}
	case "day", "week":
		t := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		for i := 0; !t.After(end) && len(out) <= limit && i < 100000; i++ {
			ok := true
			if unit == "week" {
				ok = t.Weekday() == time.Monday
			} else if n == 2 {
				ok = t.Day()%2 == 1 && t.Day() != 31
			}
			if ok {
				add(t, dayLabel(t), t.Day() == 1)
			}
			t = t.AddDate(0, 0, 1)
		}
	case "month":
		t := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
		for !t.After(end) && len(out) <= limit {
			if (int(t.Month())-1)%n == 0 {
				if t.Month() == 1 {
					add(t, tlYearLabel(tlLabelYear(t.Year())), true)
				} else {
					add(t, tdMonthNames[t.Month()-1], false)
				}
			}
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		}
	default:
		// Major ticks at the next power of ten: every 10 years for 1, 2 or
		// 5 a step, every 100 for 10, 20 or 50.
		major := 10
		for major <= n {
			major *= 10
		}
		v := tlFloorDiv(tlLabelYear(start.Year()), n) * n
		last := tlLabelYear(end.Year())
		for ; v <= last && len(out) <= limit; v += n {
			t := time.Date(tlAstroYear(v), 1, 1, 0, 0, 0, 0, time.UTC)
			add(t, tlYearLabel(v), v%major == 0)
		}
	}
	return out
}

// TimelineSpanWords is how long a span is, roughly: 3 days, 6 years.
func TimelineSpanWords(s, e int64) string {
	d := float64(e-s) / tlDay
	pl := func(n float64, u string) string {
		r := int(math.Round(n))
		if r == 1 {
			return "1 " + u
		}
		return strconv.Itoa(r) + " " + u + "s"
	}
	switch {
	case d < 1.0/24:
		return pl(d*24*60, "minute")
	case d < 1:
		return pl(d*24, "hour")
	case d < 60:
		return pl(d, "day")
	case d < 730:
		return pl(d/30.44, "month")
	}
	return pl(d/365.25, "year")
}
