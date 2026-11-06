package orgs

// A tracker's picture. The chart is laid out once as a list of shapes in
// pixels (trackerLayout), then written as SVG (html, worg, VS Code, typst) or
// as TikZ (LaTeX), so the two cannot disagree about where a day is.
//
// Colours follow the house chart rules: one hue for magnitude, its steps as
// the hue's strength over the page, so a heatmap reads light to dark on paper
// and recedes toward the surface on a dark page with no theme switch; text,
// axes and empty days are the page's own ink (currentColor) at lower
// strength, never the series colour. Gridlines are solid hairlines.
//
// Each day carries data-date, data-value, data-n, data-file, data-hash and
// data-line, and a <title> (the browser's tooltip in an exported page), so a
// client can put its own tooltip and click on the server's picture rather
// than drawing the chart again.

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// The hues a tracker can be drawn in: the chart palette's categorical slots.
var trackerHues = map[string]string{
	"blue": "#2a78d6", "orange": "#eb6834", "aqua": "#1baf7a", "teal": "#1baf7a", "yellow": "#eda100",
	"magenta": "#e87ba4", "pink": "#e87ba4", "green": "#008300", "violet": "#6250d6", "purple": "#6250d6", "red": "#e34948",
}

func trackerHue(c string) string {
	c = strings.ToLower(strings.TrimSpace(c))
	if h, ok := trackerHues[c]; ok {
		return h
	}
	if len(c) == 7 && c[0] == '#' {
		if _, err := strconv.ParseUint(c[1:], 16, 32); err == nil {
			return c
		}
	}
	return trackerHues["blue"]
}

// How strong each heatmap step is.
var trackerLevelAlpha = []float64{0, 0.25, 0.5, 0.75, 1}

type tkShape struct {
	kind       string // rect, text, path, line
	x, y, w, h float64
	r          float64
	pts        [][2]float64
	class      string
	text       string
	anchor     string // start, middle, end
	bold       bool
	size       float64
	title      string
	day        *common.TrackerDay
}

type tkPicture struct {
	w, h   float64
	title  string
	shapes []tkShape
}

func (p *tkPicture) add(s tkShape) { p.shapes = append(p.shapes, s) }

func (p *tkPicture) label(x, y float64, s, anchor, class string, size float64, bold bool) {
	p.add(tkShape{kind: "text", x: x, y: y, text: s, anchor: anchor, class: class, size: size, bold: bold})
}

func trackerDayTitle(d common.TrackerDay, kind, unit string) string {
	t, _ := time.ParseInLocation("2006-01-02", d.Date, time.Local)
	head := t.Format("Mon 2 Jan 2006")
	if d.N == 0 {
		if kind == "count" || kind == "duration" {
			return head + ": 0"
		}
		return head + ": nothing"
	}
	v := common.TrackerFormat(d.Value, kind)
	if unit != "" {
		v += " " + unit
	}
	return head + ": " + v
}

// trackerLayout lays a series out in pixels.
func trackerLayout(s common.TrackerSeries, spec trackerSpec, width int, now time.Time) *tkPicture {
	switch s.Type {
	case "line", "bar":
		return trackerChartLayout(s, spec, width, now)
	case "sparkline":
		return trackerSparkLayout(s, spec)
	}
	return trackerHeatLayout(s, spec, now)
}

const (
	tkCell = 11.0
	tkStep = 13.0
)

func trackerHeatLayout(s common.TrackerSeries, spec trackerSpec, now time.Time) *tkPicture {
	p := &tkPicture{title: s.Title}
	from, _ := time.ParseInLocation("2006-01-02", s.From, time.Local)
	to, _ := time.ParseInLocation("2006-01-02", s.To, time.Local)
	weekStart := from.AddDate(0, 0, -((int(from.Weekday()) + 6) % 7))
	cols := int(to.Sub(weekStart).Hours()/24)/7 + 1
	left, top := 30.0, 4.0
	if s.Title != "" {
		p.label(0, 14, s.Title, "start", "ink", 13, true)
		top = 26
	}
	gy := top + 16
	gridW := float64(cols) * tkStep
	p.w = math.Max(left+gridW+4, 400)
	// Months along the top: at the column holding the 1st, the first
	// column's only when the next 1st is a way off.
	firstOne := -1
	labels := []struct {
		col int
		s   string
	}{}
	for c := 0; c < cols; c++ {
		for r := 0; r < 7; r++ {
			d := weekStart.AddDate(0, 0, c*7+r)
			if d.Day() == 1 && !d.Before(from) && !d.After(to) {
				if firstOne < 0 {
					firstOne = c
				}
				name := d.Format("Jan")
				if d.Month() == time.January {
					name = d.Format("Jan 2006")
				}
				labels = append(labels, struct {
					col int
					s   string
				}{c, name})
			}
		}
	}
	if firstOne < 0 || firstOne >= 3 {
		labels = append([]struct {
			col int
			s   string
		}{{0, from.Format("Jan")}}, labels...)
	}
	last := -10
	for _, l := range labels {
		if l.col-last < 3 {
			continue
		}
		p.label(left+float64(l.col)*tkStep, gy-5, l.s, "start", "ink2", 10, false)
		last = l.col
	}
	for _, r := range []int{0, 2, 4} {
		p.label(left-6, gy+float64(r)*tkStep+tkCell-2, []string{"Mon", "", "Wed", "", "Fri"}[r], "end", "ink2", 10, false)
	}
	lo, hi := trackerBounds(s, spec)
	today := trackerDay(now).Format("2006-01-02")
	for i := range s.Days {
		d := &s.Days[i]
		t, _ := time.ParseInLocation("2006-01-02", d.Date, time.Local)
		off := int(math.Round(t.Sub(weekStart).Hours() / 24))
		c, r := off/7, off%7
		lvl := common.TrackerLevel(d.Value, d.N, lo, hi)
		x, y := left+float64(c)*tkStep, gy+float64(r)*tkStep
		p.add(tkShape{kind: "rect", x: x, y: y, w: tkCell, h: tkCell, r: 2, class: "l" + strconv.Itoa(lvl),
			title: trackerDayTitle(*d, s.Kind, s.Unit), day: d})
		if d.Date == today {
			p.add(tkShape{kind: "rect", x: x - 1, y: y - 1, w: tkCell + 2, h: tkCell + 2, r: 3, class: "today"})
		}
	}
	// Legend and summary.
	ly := gy + 7*tkStep + 6
	lx := p.w - 4 - 5*tkStep - 30
	p.label(lx-4, ly+9, "Less", "end", "ink2", 10, false)
	for i := 0; i < 5; i++ {
		p.add(tkShape{kind: "rect", x: lx + float64(i)*tkStep, y: ly, w: tkCell, h: tkCell, r: 2, class: "l" + strconv.Itoa(i)})
	}
	p.label(lx+5*tkStep+2, ly+9, "More", "start", "ink2", 10, false)
	if !spec.Bare {
		p.label(0, ly+9, trackerSummaryText(s), "start", "ink2", 10, false)
	}
	p.h = ly + tkCell + 4
	return p
}

// trackerBounds is the value scale: min:/max: when given, else from zero (or
// the least value, if below it) to the most.
func trackerBounds(s common.TrackerSeries, spec trackerSpec) (float64, float64) {
	lo, hi := 0.0, 0.0
	if s.Summary.Days > 0 {
		lo = math.Min(0, s.Summary.Min)
		hi = s.Summary.Max
		// Values somebody wrote down (a mood of 1 to 5) are scaled from the
		// least of them; a line of them need not start at zero either. Counts
		// and bars do.
		if !spec.zeroFill() && s.Type != "bar" && s.Summary.Min > 0 && s.Summary.Max > s.Summary.Min {
			lo = s.Summary.Min
		}
	}
	if spec.Min != nil {
		lo = *spec.Min
	}
	if spec.Max != nil {
		hi = *spec.Max
	}
	return lo, hi
}

// trackerNiceStep is a round step near span/n: 1, 2, 2.5 or 5 times a power
// of ten (minutes go by 15, 30, 60...).
func trackerNiceStep(span float64, n int, duration bool) float64 {
	if span <= 0 {
		return 1
	}
	raw := span / float64(n)
	if duration && raw >= 10 {
		for _, s := range []float64{15, 30, 60, 120, 180, 240, 360, 480, 720, 1440} {
			if s >= raw {
				return s
			}
		}
	}
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if m*mag >= raw {
			return m * mag
		}
	}
	return 10 * mag
}

func trackerTick(v float64, kind string) string {
	if kind == "duration" {
		return common.TrackerFormat(v, "duration")
	}
	a := math.Abs(v)
	switch {
	case a >= 1e6:
		return strconv.FormatFloat(v/1e6, 'f', -1, 64) + "M"
	case a >= 1e3:
		return strconv.FormatFloat(math.Round(v/100)/10, 'f', -1, 64) + "K"
	}
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

func trackerChartLayout(s common.TrackerSeries, spec trackerSpec, width int, now time.Time) *tkPicture {
	p := &tkPicture{title: s.Title}
	if width < 240 {
		width = 640
	}
	p.w = float64(width)
	top := 10.0
	if s.Title != "" {
		p.label(0, 14, s.Title, "start", "ink", 13, true)
		top = 30
	}
	left, right := 44.0, 10.0
	if spec.Goal != nil {
		right = 58 // the goal's label sits past the plot's end
	}
	plotH := 150.0
	bottom := top + plotH
	n := len(s.Days)
	if n == 0 {
		n = 1
	}
	step := (p.w - left - right) / float64(n)
	lo, hi := trackerBounds(s, spec)
	if spec.Goal != nil && spec.Max == nil {
		hi = math.Max(hi, *spec.Goal)
		lo = math.Min(lo, *spec.Goal)
	}
	tick := trackerNiceStep(hi-lo, 4, s.Kind == "duration")
	if (s.Kind == "count" || s.Kind == "bool" || s.Kind == "rating") && tick < 1 {
		tick = 1 // no half a run
	}
	lo = math.Floor(lo/tick) * tick
	hi = math.Ceil(hi/tick) * tick
	if hi <= lo {
		hi = lo + tick
	}
	yOf := func(v float64) float64 { return bottom - (v-lo)/(hi-lo)*plotH }
	for v := lo; v <= hi+tick/1000; v += tick {
		y := yOf(v)
		p.add(tkShape{kind: "line", pts: [][2]float64{{left, y}, {p.w - right, y}}, class: "grid"})
		p.label(left-6, y+3.5, trackerTick(v, s.Kind), "end", "ink2", 10, false)
	}
	// Dates along the bottom.
	lastX := -100.0
	for i, d := range s.Days {
		t, _ := time.ParseInLocation("2006-01-02", d.Date, time.Local)
		label := ""
		switch {
		case len(s.Days) <= 16:
			label = t.Format("2 Jan")
			if i > 0 && t.Day() != 1 {
				label = t.Format("2")
			}
		case len(s.Days) <= 120:
			if t.Weekday() == time.Monday {
				label = t.Format("2 Jan")
			}
		default:
			if t.Day() == 1 {
				label = t.Format("Jan")
				if t.Month() == time.January {
					label = t.Format("Jan 2006")
				}
			}
		}
		x := left + (float64(i)+0.5)*step
		if label != "" && x-lastX > 44 {
			p.label(x, bottom+14, label, "middle", "ink2", 10, false)
			lastX = x
		}
	}
	zero := spec.zeroFill()
	if s.Type == "bar" {
		bw := math.Max(1, math.Min(step-2, 24))
		for i := range s.Days {
			d := &s.Days[i]
			if d.N == 0 && !zero {
				continue
			}
			x := left + (float64(i)+0.5)*step - bw/2
			y0, y1 := yOf(math.Max(lo, 0)), yOf(d.Value)
			if math.Abs(y0-y1) < 0.5 {
				continue
			}
			p.add(tkShape{kind: "bar", x: x, y: math.Min(y0, y1), w: bw, h: math.Abs(y0 - y1), r: math.Min(4, bw/2), class: "bar"})
		}
	} else {
		pts := [][2]float64{}
		for i, d := range s.Days {
			if d.N == 0 && !zero {
				continue
			}
			pts = append(pts, [2]float64{left + (float64(i)+0.5)*step, yOf(d.Value)})
		}
		if len(pts) > 1 {
			p.add(tkShape{kind: "path", pts: pts, class: "mark"})
		}
		if len(pts) <= 45 || len(pts) == 1 {
			for _, pt := range pts {
				p.add(tkShape{kind: "dot", x: pt[0], y: pt[1], r: 4, class: "dot"})
			}
		}
	}
	if spec.Goal != nil {
		y := yOf(*spec.Goal)
		p.add(tkShape{kind: "line", pts: [][2]float64{{left, y}, {p.w - right, y}}, class: "goal"})
		p.label(p.w-right+6, y+3.5, "goal "+common.TrackerFormat(*spec.Goal, s.Kind), "start", "ink2", 10, false)
	}
	p.add(tkShape{kind: "line", pts: [][2]float64{{left, bottom}, {p.w - right, bottom}}, class: "axis"})
	// A column a day to hover and click, over the whole height.
	for i := range s.Days {
		d := &s.Days[i]
		p.add(tkShape{kind: "rect", x: left + float64(i)*step, y: top, w: step, h: plotH, class: "hit",
			title: trackerDayTitle(*d, s.Kind, s.Unit), day: d})
	}
	p.h = bottom + 22
	if !spec.Bare {
		p.label(0, bottom+34, trackerSummaryText(s), "start", "ink2", 10, false)
		p.h = bottom + 40
	}
	return p
}

func trackerSparkLayout(s common.TrackerSeries, spec trackerSpec) *tkPicture {
	p := &tkPicture{w: 160, h: 28, title: s.Title}
	n := len(s.Days)
	if n == 0 {
		return p
	}
	lo, hi := trackerBounds(s, spec)
	if hi <= lo {
		hi = lo + 1
	}
	step := p.w / float64(n)
	pts := [][2]float64{}
	for i, d := range s.Days {
		if d.N == 0 && !spec.zeroFill() {
			continue
		}
		pts = append(pts, [2]float64{(float64(i) + 0.5) * step, 24 - (d.Value-lo)/(hi-lo)*20})
	}
	if len(pts) > 1 {
		p.add(tkShape{kind: "path", pts: pts, class: "mark"})
	}
	if len(pts) > 0 {
		last := pts[len(pts)-1]
		p.add(tkShape{kind: "dot", x: last[0], y: last[1], r: 2.5, class: "dot"})
	}
	for i := range s.Days {
		d := &s.Days[i]
		p.add(tkShape{kind: "rect", x: float64(i) * step, y: 0, w: step, h: p.h, class: "hit", title: trackerDayTitle(*d, s.Kind, s.Unit), day: d})
	}
	return p
}

// ----------------------------------------------------------------------------
// SVG
// ----------------------------------------------------------------------------

func tkNum(f float64) string { return strconv.FormatFloat(math.Round(f*10)/10, 'f', -1, 64) }

func trackerSvg(s common.TrackerSeries, spec trackerSpec, width int, now time.Time) string {
	p := trackerLayout(s, spec, width, now)
	hue := trackerHue(spec.Color)
	sum := sha1.Sum([]byte(hue))
	id := "trk" + hex.EncodeToString(sum[:])[:6]
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" class="org-tracker-svg %s" viewBox="0 0 %s %s" width="%s" height="%s" style="max-width:100%%;height:auto" role="img" aria-label="%s" data-type="%s" data-kind="%s">`,
		id, tkNum(p.w), tkNum(p.h), tkNum(p.w), tkNum(p.h), html.EscapeString(s.Title), s.Type, s.Kind)
	sel := "." + id
	fmt.Fprintf(&b, `<style>%[1]s{font-family:system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;font-variant-numeric:tabular-nums;overflow:visible}`+
		`%[1]s .l0{fill:currentColor;fill-opacity:.08}`+
		`%[1]s .l1,%[1]s .l2,%[1]s .l3,%[1]s .l4,%[1]s .bar,%[1]s .dot{fill:%[2]s}`+
		`%[1]s .l1{fill-opacity:%[3]s}%[1]s .l2{fill-opacity:%[4]s}%[1]s .l3{fill-opacity:%[5]s}`+
		`%[1]s .ink{fill:currentColor}%[1]s .ink2{fill:currentColor;fill-opacity:.68}`+
		`%[1]s .grid{stroke:currentColor;stroke-opacity:.13;stroke-width:1}`+
		`%[1]s .axis{stroke:currentColor;stroke-opacity:.35;stroke-width:1}`+
		`%[1]s .goal{stroke:currentColor;stroke-opacity:.55;stroke-width:1}`+
		`%[1]s .mark{fill:none;stroke:%[2]s;stroke-width:2;stroke-linejoin:round;stroke-linecap:round}`+
		`%[1]s .today{fill:none;stroke:currentColor;stroke-opacity:.6;stroke-width:1}`+
		`%[1]s .hit{fill:transparent}%[1]s [data-date]{cursor:pointer}`+
		`%[1]s rect[data-date]:hover:not(.hit){stroke:currentColor;stroke-width:1}`+
		`%[1]s .hit:hover{fill:currentColor;fill-opacity:.06}</style>`,
		sel, hue, tkNum(trackerLevelAlpha[1]), tkNum(trackerLevelAlpha[2]), tkNum(trackerLevelAlpha[3]))
	for _, sh := range p.shapes {
		data := ""
		if sh.day != nil {
			d := sh.day
			data = fmt.Sprintf(` data-date="%s" data-value="%s" data-n="%d"`, d.Date, html.EscapeString(common.TrackerFormat(d.Value, s.Kind)), d.N)
			if d.File != "" {
				data += fmt.Sprintf(` data-file="%s" data-line="%d"`, html.EscapeString(d.File), d.Line)
			}
			if d.Hash != "" {
				data += fmt.Sprintf(` data-hash="%s"`, html.EscapeString(d.Hash))
			}
		}
		title := ""
		if sh.title != "" {
			title = "<title>" + html.EscapeString(sh.title) + "</title>"
		}
		switch sh.kind {
		case "rect":
			rx := ""
			if sh.r > 0 {
				rx = ` rx="` + tkNum(sh.r) + `"`
			}
			if title == "" {
				fmt.Fprintf(&b, `<rect class="%s" x="%s" y="%s" width="%s" height="%s"%s%s/>`, sh.class, tkNum(sh.x), tkNum(sh.y), tkNum(sh.w), tkNum(sh.h), rx, data)
			} else {
				fmt.Fprintf(&b, `<rect class="%s" x="%s" y="%s" width="%s" height="%s"%s%s>%s</rect>`, sh.class, tkNum(sh.x), tkNum(sh.y), tkNum(sh.w), tkNum(sh.h), rx, data, title)
			}
		case "bar":
			// Rounded at the end away from the baseline only.
			r := sh.r
			x0, x1, y0, y1 := sh.x, sh.x+sh.w, sh.y, sh.y+sh.h
			r = math.Min(r, sh.h)
			fmt.Fprintf(&b, `<path class="%s" d="M%s %sV%sQ%s %s %s %sH%sQ%s %s %s %sV%sZ"/>`, sh.class,
				tkNum(x0), tkNum(y1), tkNum(y0+r), tkNum(x0), tkNum(y0), tkNum(x0+r), tkNum(y0),
				tkNum(x1-r), tkNum(x1), tkNum(y0), tkNum(x1), tkNum(y0+r), tkNum(y1))
		case "dot":
			fmt.Fprintf(&b, `<circle class="%s" cx="%s" cy="%s" r="%s"/>`, sh.class, tkNum(sh.x), tkNum(sh.y), tkNum(sh.r))
		case "line", "path":
			pts := []string{}
			for _, pt := range sh.pts {
				pts = append(pts, tkNum(pt[0])+","+tkNum(pt[1]))
			}
			fmt.Fprintf(&b, `<polyline class="%s" points="%s"/>`, sh.class, strings.Join(pts, " "))
		case "text":
			w := ""
			if sh.bold {
				w = ` font-weight="600"`
			}
			fmt.Fprintf(&b, `<text class="%s" x="%s" y="%s" font-size="%s" text-anchor="%s"%s>%s</text>`, sh.class, tkNum(sh.x), tkNum(sh.y), tkNum(sh.size), sh.anchor, w, html.EscapeString(sh.text))
		}
	}
	b.WriteString("</svg>")
	return b.String()
}

func trackerErrorSvg(msg string) string {
	w := math.Max(200, float64(len(msg))*6.2+16)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" class="org-tracker-svg org-tracker-error" viewBox="0 0 %s 28" width="%s" height="28" role="img"><text x="8" y="18" font-size="11" fill="currentColor" fill-opacity=".75" font-family="system-ui,sans-serif">%s</text></svg>`,
		tkNum(w), tkNum(w), html.EscapeString(msg))
}

// ----------------------------------------------------------------------------
// TikZ
// ----------------------------------------------------------------------------

func tikzEscape(s string) string {
	r := strings.NewReplacer(`\`, `\textbackslash{}`, `&`, `\&`, `%`, `\%`, `$`, `\$`, `#`, `\#`, `_`, `\_`, `{`, `\{`, `}`, `\}`, `~`, `\textasciitilde{}`, `^`, `\textasciicircum{}`)
	return r.Replace(s)
}

// trackerTikz is the picture as a tikzpicture, scaled to a text width of
// about 340pt, y downwards as the layout has it.
func trackerTikz(s common.TrackerSeries, spec trackerSpec, now time.Time) []string {
	p := trackerLayout(s, spec, 0, now)
	hue := strings.ToUpper(strings.TrimPrefix(trackerHue(spec.Color), "#"))
	scale := math.Min(0.75, 340/p.w)
	out := []string{
		`\begin{center}`,
		`\definecolor{trackerhue}{HTML}{` + hue + `}`,
		fmt.Sprintf(`\begin{tikzpicture}[x=%.3fpt,y=-%.3fpt,font=\sffamily\fontsize{%.1f}{%.1f}\selectfont]`, scale, scale, math.Max(5.5, 10*scale), math.Max(6.5, 12*scale)),
	}
	fills := map[string]string{"l0": "black!7", "l1": "trackerhue!25", "l2": "trackerhue!50", "l3": "trackerhue!75", "l4": "trackerhue", "bar": "trackerhue", "dot": "trackerhue"}
	for _, sh := range p.shapes {
		switch sh.kind {
		case "rect":
			if f, ok := fills[sh.class]; ok {
				out = append(out, fmt.Sprintf(`\fill[%s,rounded corners=%.2fpt] (%s,%s) rectangle (%s,%s);`, f, sh.r*scale, tkNum(sh.x), tkNum(sh.y), tkNum(sh.x+sh.w), tkNum(sh.y+sh.h)))
			} else if sh.class == "today" {
				out = append(out, fmt.Sprintf(`\draw[black!60,line width=0.4pt,rounded corners=%.2fpt] (%s,%s) rectangle (%s,%s);`, sh.r*scale, tkNum(sh.x), tkNum(sh.y), tkNum(sh.x+sh.w), tkNum(sh.y+sh.h)))
			}
		case "bar":
			out = append(out, fmt.Sprintf(`\fill[trackerhue] (%s,%s) rectangle (%s,%s);`, tkNum(sh.x), tkNum(sh.y), tkNum(sh.x+sh.w), tkNum(sh.y+sh.h)))
		case "dot":
			out = append(out, fmt.Sprintf(`\fill[trackerhue] (%s,%s) circle (%.2fpt);`, tkNum(sh.x), tkNum(sh.y), sh.r*scale))
		case "line", "path":
			style := map[string]string{"grid": "black!15,line width=0.3pt", "axis": "black!40,line width=0.3pt", "goal": "black!55,line width=0.3pt",
				"mark": "trackerhue,line width=1pt,line join=round,line cap=round"}[sh.class]
			pts := []string{}
			for _, pt := range sh.pts {
				pts = append(pts, "("+tkNum(pt[0])+","+tkNum(pt[1])+")")
			}
			out = append(out, fmt.Sprintf(`\draw[%s] %s;`, style, strings.Join(pts, " -- ")))
		case "text":
			anchor := map[string]string{"start": "base west", "middle": "base", "end": "base east"}[sh.anchor]
			colour := "black"
			if sh.class == "ink2" {
				colour = "black!68"
			}
			t := tikzEscape(sh.text)
			if sh.bold {
				t = `\textbf{` + t + `}`
			}
			out = append(out, fmt.Sprintf(`\node[%s,anchor=%s,inner sep=0pt] at (%s,%s) {%s};`, colour, anchor, tkNum(sh.x), tkNum(sh.y), t))
		}
	}
	return append(out, `\end{tikzpicture}`, `\end{center}`)
}
