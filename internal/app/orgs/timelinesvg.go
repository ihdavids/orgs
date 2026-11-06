package orgs

// A timeline's picture, for exports, the CLI and anything that cannot run
// worg's live view: lanes of events over an axis whose ticks suit any scale
// (common.TimelineTicks, minutes to millennia). Spans are rounded bars,
// points dots, eras bands behind every lane with their name in a strip at
// the top. Labels sit above their mark and are packed into rows so none
// overlap; past the rows a lane may take, a mark is drawn without its label
// (its <title> still names it).
//
// Ink is currentColor at lower strengths, so the picture reads on a light
// page, worg's dark html theme and paper alike; events wear their lane's
// colour from the chart palette. Every event is a <g> with data-hash,
// data-file, data-line and data-i (its place in the events), which worg's
// rendered view and VS Code use for their own hover and click.

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"html"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ihdavids/orgs/internal/common"
)

const (
	tlsPad      = 16.0
	tlsRow      = 30.0
	tlsLaneHead = 20.0
	tlsLaneGap  = 8.0
	tlsAxis     = 30.0
	tlsFont     = 12.0
	tlsMaxRows  = 6
)

// tlsTextWidth is about how wide a label is at the label size.
func tlsTextWidth(s string, size float64) float64 {
	w := 0.0
	for _, r := range s {
		switch {
		case r == ' ' || r == 'i' || r == 'l' || r == 'j' || r == '.' || r == ',' || r == '\'' || r == 't' || r == 'f' || r == 'I':
			w += 0.3
		case r == 'm' || r == 'w' || r == 'M' || r == 'W':
			w += 0.85
		case r >= 'A' && r <= 'Z':
			w += 0.66
		case r > 0x2e80:
			w += 1.0
		default:
			w += 0.55
		}
	}
	return w * size
}

func tlsShort(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// tlsRange is the span drawn: the asked for range, else the events' own with
// a margin.
func tlsRange(d common.TimelineData) (int64, int64) {
	lo, hi := int64(math.MaxInt64), int64(math.MinInt64)
	for _, ev := range d.Events {
		if ev.S < lo {
			lo = ev.S
		}
		end := ev.S
		if ev.Kind != "point" && ev.E > end {
			end = ev.E
		}
		if end > hi {
			hi = end
		}
	}
	if len(d.Events) == 0 {
		now := common.TimelineWall(time.Now())
		lo, hi = now-30*86400000, now+30*86400000
	}
	if hi <= lo {
		lo, hi = lo-86400000, hi+86400000
	}
	pad := (hi - lo) / 25
	if d.S != 0 || d.From != "" {
		lo = d.S
	} else {
		lo -= pad
	}
	if d.E != 0 || d.To != "" {
		hi = d.E
	} else {
		hi += pad
	}
	if hi <= lo {
		hi = lo + 86400000
	}
	return lo, hi
}

type tlsPlaced struct {
	i        int
	x0, x1   float64 // the mark
	lx       float64 // the label's left
	label    string
	row      int
	labelled bool
}

// tlsPack puts a lane's events in rows, earliest first, each in the first
// row whose last occupant ends before it starts.
func tlsPack(evs []tlsPlaced, maxRows int, right float64) ([]tlsPlaced, int) {
	ends := []float64{}
	rows := 0
	for k := range evs {
		p := &evs[k]
		lw := tlsTextWidth(p.label, tlsFont)
		p.lx = p.x0
		if p.x1-p.x0 < 1 {
			p.lx = p.x0 - 4.5
		}
		if p.lx+lw > right {
			p.lx = math.Max(tlsPad, right-lw)
		}
		start := math.Min(p.lx, p.x0-5)
		end := math.Max(p.x1+5, p.lx+lw) + 10
		placed := false
		for r := range ends {
			if ends[r] <= start {
				p.row, ends[r], p.labelled, placed = r, end, true, true
				break
			}
		}
		if !placed && len(ends) < maxRows {
			p.row, p.labelled = len(ends), true
			ends = append(ends, end)
			placed = true
		}
		if !placed {
			// No room: the mark goes in the row that frees up first, unlabelled.
			best := 0
			for r := range ends {
				if ends[r] < ends[best] {
					best = r
				}
			}
			p.row, p.labelled = best, false
			ends[best] = math.Max(ends[best], p.x1+8)
		}
		if p.row+1 > rows {
			rows = p.row + 1
		}
	}
	return evs, rows
}

func tlsNum(f float64) string { return tkNum(f) }

// timelineSvg draws a timeline width pixels wide (960 when 0).
func timelineSvg(d common.TimelineData, width int, now time.Time) string {
	W := float64(width)
	if W <= 0 {
		W = 960
	}
	W = math.Max(320, math.Min(W, 4000))
	lo, hi := tlsRange(d)
	left, right := tlsPad, W-tlsPad
	xOf := func(ms int64) float64 { return left + float64(ms-lo)/float64(hi-lo)*(right-left) }
	clamp := func(x float64) float64 { return math.Max(left, math.Min(right, x)) }

	// Which events go where.
	laneOf := map[string]int{}
	for i, l := range d.Lanes {
		laneOf[l.Name] = i
	}
	perLane := make([][]tlsPlaced, len(d.Lanes))
	eras := []tlsPlaced{}
	for i, ev := range d.Events {
		end := ev.S
		if ev.Kind != "point" && ev.E > end {
			end = ev.E
		}
		if end < lo || ev.S > hi {
			continue
		}
		p := tlsPlaced{i: i, x0: clamp(xOf(ev.S)), label: tlsShort(ev.Title, 48)}
		p.x1 = p.x0
		if ev.Kind != "point" && ev.E > ev.S {
			p.x1 = clamp(xOf(ev.E))
			if p.x1-p.x0 < 8 {
				p.x1 = p.x0 + 8
			}
		}
		if ev.Kind == "era" {
			eras = append(eras, p)
			continue
		}
		if li, ok := laneOf[ev.Lane]; ok {
			perLane[li] = append(perLane[li], p)
		}
	}
	maxRows := tlsMaxRows
	if d.Height > 0 && len(d.Lanes) > 0 {
		avail := float64(d.Height) - tlsAxis - 40 - float64(len(d.Lanes))*(tlsLaneHead+tlsLaneGap)
		maxRows = int(math.Max(1, math.Min(12, math.Floor(avail/float64(len(d.Lanes))/tlsRow))))
	}
	rowsOf := make([]int, len(d.Lanes))
	for li := range perLane {
		perLane[li], rowsOf[li] = tlsPack(perLane[li], maxRows, right)
		if rowsOf[li] == 0 {
			rowsOf[li] = 1
		}
	}
	eras, eraRows := tlsPack(eras, 2, right)

	// Heights.
	y := 8.0
	titleY := 0.0
	if d.Title != "" {
		titleY = y + 15
		y += 28
	}
	eraY := y
	if len(eras) > 0 {
		y += float64(eraRows)*24 + 6
	}
	plotTop := y
	laneY := make([]float64, len(d.Lanes))
	for li := range d.Lanes {
		laneY[li] = y
		y += tlsLaneHead + float64(rowsOf[li])*tlsRow + tlsLaneGap
	}
	if len(d.Lanes) == 0 {
		y += 40
	}
	axisY := y
	H := axisY + tlsAxis

	sum := sha1.Sum([]byte(fmt.Sprintf("%d %d %d %s", lo, hi, len(d.Events), d.Title)))
	id := "tl" + hex.EncodeToString(sum[:4])
	sel := "#" + id
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" id="%s" class="org-timeline-svg" viewBox="0 0 %s %s" width="%s" height="%s" role="img" aria-label="%s" style="max-width:100%%;height:auto;font-family:system-ui,-apple-system,'Segoe UI',sans-serif">`,
		id, tlsNum(W), tlsNum(H), tlsNum(W), tlsNum(H), html.EscapeString(timelineCaption(d)))
	fmt.Fprintf(&b, `<style>%[1]s .ink{fill:currentColor}%[1]s .ink2{fill:currentColor;fill-opacity:.62}%[1]s .ink3{fill:currentColor;fill-opacity:.45}`+
		`%[1]s .grid{stroke:currentColor;stroke-opacity:.08;stroke-width:1}%[1]s .gridM{stroke:currentColor;stroke-opacity:.16;stroke-width:1}`+
		`%[1]s .axis{stroke:currentColor;stroke-opacity:.3;stroke-width:1}%[1]s .lane{fill:currentColor;fill-opacity:.025}`+
		`%[1]s .era{fill:currentColor;fill-opacity:.045}%[1]s .erabar{fill:currentColor;fill-opacity:.09}`+
		`%[1]s .today{stroke:#d03b3b;stroke-width:1.5}%[1]s .todayt{fill:#d03b3b;font-weight:600}`+
		`%[1]s .ev{cursor:pointer}%[1]s .ev:hover .lbl{text-decoration:underline}%[1]s .ev:hover .mk{filter:brightness(1.12)}%[1]s .hit{fill:transparent}</style>`, sel)

	if d.Title != "" {
		fmt.Fprintf(&b, `<text class="ink" x="%s" y="%s" font-size="14" font-weight="600">%s</text>`, tlsNum(left), tlsNum(titleY), html.EscapeString(d.Title))
	}
	// Lane backgrounds.
	for li := range d.Lanes {
		h := tlsLaneHead + float64(rowsOf[li])*tlsRow
		if li%2 == 0 {
			fmt.Fprintf(&b, `<rect class="lane" x="%s" y="%s" width="%s" height="%s" rx="6"/>`, tlsNum(left-8), tlsNum(laneY[li]-2), tlsNum(right-left+16), tlsNum(h+4))
		}
	}
	// Era washes behind everything.
	for _, p := range eras {
		ev := d.Events[p.i]
		style := ""
		if ev.Colour != "" {
			style = fmt.Sprintf(` style="fill:%s;fill-opacity:.08"`, ev.Colour)
		}
		fmt.Fprintf(&b, `<rect class="era" x="%s" y="%s" width="%s" height="%s"%s/>`, tlsNum(p.x0), tlsNum(plotTop), tlsNum(p.x1-p.x0), tlsNum(axisY-plotTop), style)
	}
	// Gridlines and the axis.
	ticks := common.TimelineTicks(lo, hi, right-left, 84)
	for _, t := range ticks {
		x := xOf(t.T)
		cls := "grid"
		if t.Major {
			cls = "gridM"
		}
		fmt.Fprintf(&b, `<line class="%s" x1="%s" y1="%s" x2="%s" y2="%s"/>`, cls, tlsNum(x), tlsNum(plotTop), tlsNum(x), tlsNum(axisY))
	}
	fmt.Fprintf(&b, `<line class="axis" x1="%s" y1="%s" x2="%s" y2="%s"/>`, tlsNum(left), tlsNum(axisY), tlsNum(right), tlsNum(axisY))
	for _, t := range ticks {
		x := xOf(t.T)
		anchor := "middle"
		if x-left < 20 {
			anchor = "start"
		} else if right-x < 20 {
			anchor = "end"
		}
		cls, wt := "ink2", ""
		if t.Major {
			cls, wt = "ink", ` font-weight="600"`
		}
		fmt.Fprintf(&b, `<line class="axis" x1="%s" y1="%s" x2="%s" y2="%s"/>`, tlsNum(x), tlsNum(axisY), tlsNum(x), tlsNum(axisY+4))
		fmt.Fprintf(&b, `<text class="%s" x="%s" y="%s" font-size="11" text-anchor="%s"%s style="font-variant-numeric:tabular-nums">%s</text>`, cls, tlsNum(x), tlsNum(axisY+17), anchor, wt, html.EscapeString(t.Label))
	}
	// Today.
	if t := common.TimelineWall(now); t >= lo && t <= hi {
		x := xOf(t)
		fmt.Fprintf(&b, `<line class="today" x1="%s" y1="%s" x2="%s" y2="%s"/>`, tlsNum(x), tlsNum(plotTop-4), tlsNum(x), tlsNum(axisY))
		anchor := "middle"
		if right-x < 24 {
			anchor = "end"
		}
		fmt.Fprintf(&b, `<text class="todayt" x="%s" y="%s" font-size="10" text-anchor="%s">Today</text>`, tlsNum(x), tlsNum(axisY+28), anchor)
	}
	data := func(i int) string {
		ev := d.Events[i]
		return fmt.Sprintf(` data-i="%d" data-hash="%s" data-file="%s" data-line="%d"`, i, html.EscapeString(ev.Hash), html.EscapeString(ev.File), ev.Line)
	}
	tip := func(ev common.TimelineEvent) string {
		s := ev.Label + " — " + ev.Title
		if ev.Lane != "" {
			s += " (" + ev.Lane + ")"
		}
		return "<title>" + html.EscapeString(s) + "</title>"
	}
	// The era strip.
	for _, p := range eras {
		ev := d.Events[p.i]
		yy := eraY + float64(p.row)*24
		fill := ""
		if ev.Colour != "" {
			fill = fmt.Sprintf(` style="fill:%s;fill-opacity:.22"`, ev.Colour)
		}
		fmt.Fprintf(&b, `<g class="ev"%s>%s<rect class="erabar mk" x="%s" y="%s" width="%s" height="20" rx="4"%s/>`, data(p.i), tip(ev), tlsNum(p.x0), tlsNum(yy), tlsNum(p.x1-p.x0), fill)
		if p.labelled {
			lx := math.Max(p.x0+6, left+2)
			fmt.Fprintf(&b, `<text class="ink lbl" x="%s" y="%s" font-size="11" font-weight="600">%s</text>`, tlsNum(lx), tlsNum(yy+14), html.EscapeString(p.label))
		}
		b.WriteString("</g>")
	}
	// The lanes.
	for li, l := range d.Lanes {
		ly := laneY[li]
		if !(len(d.Lanes) == 1 && l.Name == "") {
			name := l.Name
			if name == "" {
				name = "Events"
			}
			fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="4" fill="%s"/>`, tlsNum(left+4), tlsNum(ly+10), l.Colour)
			fmt.Fprintf(&b, `<text class="ink" x="%s" y="%s" font-size="11" font-weight="600">%s <tspan class="ink3" font-weight="400">%d</tspan></text>`,
				tlsNum(left+14), tlsNum(ly+14), html.EscapeString(tlsShort(name, 60)), l.Count)
		}
		for _, p := range perLane[li] {
			ev := d.Events[p.i]
			ry := ly + tlsLaneHead + float64(p.row)*tlsRow
			fmt.Fprintf(&b, `<g class="ev"%s>%s`, data(p.i), tip(ev))
			hx0 := math.Min(p.x0-6, p.lx)
			hx1 := math.Max(p.x1+6, p.lx+tlsTextWidth(p.label, tlsFont))
			if !p.labelled {
				hx0, hx1 = p.x0-6, p.x1+6
			}
			fmt.Fprintf(&b, `<rect class="hit" x="%s" y="%s" width="%s" height="%s"/>`, tlsNum(hx0), tlsNum(ry), tlsNum(hx1-hx0), tlsNum(tlsRow))
			if ev.Kind != "point" && ev.E > ev.S {
				op := ""
				if ev.Approx {
					op = ` fill-opacity=".55"`
				}
				fmt.Fprintf(&b, `<rect class="mk" x="%s" y="%s" width="%s" height="8" rx="4" fill="%s"%s/>`, tlsNum(p.x0), tlsNum(ry+17), tlsNum(p.x1-p.x0), ev.Colour, op)
			} else if ev.Approx {
				fmt.Fprintf(&b, `<circle class="mk" cx="%s" cy="%s" r="4" fill="%s" fill-opacity=".35" stroke="%s" stroke-width="1.5"/>`, tlsNum(p.x0), tlsNum(ry+21), ev.Colour, ev.Colour)
			} else {
				fmt.Fprintf(&b, `<circle class="mk" cx="%s" cy="%s" r="4.5" fill="%s"/>`, tlsNum(p.x0), tlsNum(ry+21), ev.Colour)
			}
			if p.labelled {
				fmt.Fprintf(&b, `<text class="ink lbl" x="%s" y="%s" font-size="%s">%s</text>`, tlsNum(p.lx), tlsNum(ry+11), tlsNum(tlsFont), html.EscapeString(p.label))
			}
			b.WriteString("</g>")
		}
	}
	if len(d.Events) == 0 {
		fmt.Fprintf(&b, `<text class="ink2" x="%s" y="%s" font-size="12" text-anchor="middle">No dated headings for this timeline</text>`, tlsNum(W/2), tlsNum(plotTop+24))
	}
	b.WriteString("</svg>")
	return b.String()
}
