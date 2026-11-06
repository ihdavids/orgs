package jcanvas

// A canvas drawn as one static SVG: groups, cards, edges with arrows and
// labels, in the canvas's own coordinates. Used for `GET /canvas/svg`, for
// `orgs canvas svg` and for a `[[file:board.canvas]]` link in an exported
// page. Text is wrapped by a character count (an SVG has no text layout);
// it is a picture of the board, not the board.

import (
	"fmt"
	"html"
	"math"
	"strings"
)

type SvgOpts struct {
	// Prefix for the ids inside (clip paths), so two canvases inlined in one
	// page do not share them.
	IDPrefix string
	// Widest the picture is drawn (its width attribute); 0 = natural size.
	MaxWidth float64
	// Dark draws light text on dark cards.
	Dark bool
}

type palette struct{ bg, card, stroke, text, dim, edge, labelBg string }

func paletteFor(dark bool) palette {
	if dark {
		return palette{"#1e1e1e", "#262626", "#555555", "#dadada", "#999999", "#8a8a8a", "#1e1e1e"}
	}
	return palette{"#fafafa", "#ffffff", "#c8c8c8", "#222222", "#777777", "#8a8a8a", "#fafafa"}
}

func esc(s string) string { return html.EscapeString(s) }

func f(v float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.1f", v), "0"), ".")
}

// wrap breaks a line into pieces of at most n characters, at spaces where it can.
func wrap(line string, n int) []string {
	if n < 4 {
		n = 4
	}
	words := strings.Fields(line)
	if len(words) == 0 {
		return []string{""}
	}
	out := []string{}
	cur := ""
	for _, w := range words {
		for len([]rune(w)) > n {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			r := []rune(w)
			out = append(out, string(r[:n]))
			w = string(r[n:])
		}
		switch {
		case cur == "":
			cur = w
		case len([]rune(cur))+1+len([]rune(w)) <= n:
			cur += " " + w
		default:
			out = append(out, cur)
			cur = w
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// labelWidth is the width of an edge label's box.
func labelWidth(text string) float64 { return float64(len([]rune(text)))*7 + 16 }

// At is the curve's point at t.
func (c Curve) At(t float64) Pt {
	u := 1 - t
	a, b, d, e := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
	return Pt{a*c.P1.X + b*c.C1.X + d*c.C2.X + e*c.P2.X, a*c.P1.Y + b*c.C1.Y + d*c.C2.Y + e*c.P2.Y}
}

// drawnBounds is the box around everything the picture draws: the cards
// (Bounds) and each edge's curve and label, since a curve leaving a side
// bulges out past the cards it joins (two cards side by side joined top to
// top loop above both).
func drawnBounds(c *Canvas) Rect {
	b := c.Bounds()
	if len(c.Nodes) == 0 {
		return b
	}
	minX, minY, maxX, maxY := b.X, b.Y, b.X+b.W, b.Y+b.H
	add := func(x0, y0, x1, y1 float64) {
		minX, minY = math.Min(minX, x0), math.Min(minY, y0)
		maxX, maxY = math.Max(maxX, x1), math.Max(maxY, y1)
	}
	for _, e := range c.Edges {
		from, to := c.Node(e.FromNode), c.Node(e.ToNode)
		if from == nil || to == nil {
			continue
		}
		cv := EdgeCurve(from.Rect(), to.Rect(), e.FromSide, e.ToSide)
		for i := 0; i <= 32; i++ {
			pt := cv.At(float64(i) / 32)
			add(pt.X, pt.Y, pt.X, pt.Y)
		}
		if e.Label != "" {
			m, w := cv.Mid(), labelWidth(e.Label)
			add(m.X-w/2, m.Y-11, m.X+w/2, m.Y+11)
		}
	}
	return Rect{minX, minY, maxX - minX, maxY - minY}
}

// SVG draws the canvas. cards may be nil (every card drawn from the node alone).
func SVG(c *Canvas, cards map[string]Card, o SvgOpts) string {
	p := paletteFor(o.Dark)
	b := drawnBounds(c)
	pad := 40.0
	if b.W == 0 && b.H == 0 {
		b = Rect{0, 0, 400, 200}
	}
	vx, vy, vw, vh := b.X-pad, b.Y-pad-24, b.W+2*pad, b.H+2*pad+24
	width := vw
	if o.MaxWidth > 0 && width > o.MaxWidth {
		width = o.MaxWidth
	}
	height := vh * width / vw
	var s strings.Builder
	fmt.Fprintf(&s, `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" class="org-canvas" viewBox="%s %s %s %s" width="%s" height="%s" font-family="system-ui, -apple-system, 'Segoe UI', sans-serif">`,
		f(vx), f(vy), f(vw), f(vh), f(width), f(height))
	fmt.Fprintf(&s, `<rect x="%s" y="%s" width="%s" height="%s" fill="%s"/>`, f(vx), f(vy), f(vw), f(vh), p.bg)
	if len(c.Nodes) == 0 {
		fmt.Fprintf(&s, `<text x="%s" y="%s" text-anchor="middle" fill="%s" font-size="16">An empty canvas</text>`, f(b.X+b.W/2), f(b.Y+b.H/2), p.dim)
	}

	// Groups, under everything.
	for _, n := range c.Nodes {
		if n.Type != "group" {
			continue
		}
		col := ColorHex(n.Color)
		stroke := p.stroke
		if col != "" {
			stroke = col
		}
		fill := "none"
		if col != "" {
			fill = col
		}
		fmt.Fprintf(&s, `<g class="canvas-group"><rect x="%s" y="%s" width="%s" height="%s" rx="10" fill="%s" fill-opacity="0.07" stroke="%s" stroke-width="2"/>`,
			f(n.X), f(n.Y), f(n.Width), f(n.Height), fill, stroke)
		if n.Label != "" {
			fmt.Fprintf(&s, `<text x="%s" y="%s" font-size="18" font-weight="600" fill="%s">%s</text>`, f(n.X+4), f(n.Y-8), p.text, esc(n.Label))
		}
		s.WriteString(`</g>`)
	}

	// Edges, under the cards they join.
	type lbl struct {
		at   Pt
		text string
		col  string
	}
	labels := []lbl{}
	for _, e := range c.Edges {
		from, to := c.Node(e.FromNode), c.Node(e.ToNode)
		if from == nil || to == nil {
			continue
		}
		cv := EdgeCurve(from.Rect(), to.Rect(), e.FromSide, e.ToSide)
		col := ColorHex(e.Color)
		if col == "" {
			col = p.edge
		}
		fmt.Fprintf(&s, `<path d="M%s,%s C%s,%s %s,%s %s,%s" fill="none" stroke="%s" stroke-width="2.5"/>`,
			f(cv.P1.X), f(cv.P1.Y), f(cv.C1.X), f(cv.C1.Y), f(cv.C2.X), f(cv.C2.Y), f(cv.P2.X), f(cv.P2.Y), col)
		if e.ToEndOr() == "arrow" {
			s.WriteString(arrow(cv.P2, cv.ToSide, col))
		}
		if e.FromEndOr() == "arrow" {
			s.WriteString(arrow(cv.P1, cv.FromSide, col))
		}
		if e.Label != "" {
			labels = append(labels, lbl{cv.Mid(), e.Label, col})
		}
	}

	// Cards.
	for i, n := range c.Nodes {
		if n.Type == "group" {
			continue
		}
		var card *Card
		if cd, ok := cards[n.ID]; ok {
			card = &cd
		}
		s.WriteString(cardSvg(n, card, p, fmt.Sprintf("%sc%d", o.IDPrefix, i)))
	}

	for _, l := range labels {
		w := labelWidth(l.text)
		fmt.Fprintf(&s, `<rect x="%s" y="%s" width="%s" height="22" rx="4" fill="%s"/><text x="%s" y="%s" text-anchor="middle" font-size="13" fill="%s">%s</text>`,
			f(l.at.X-w/2), f(l.at.Y-11), f(w), p.labelBg, f(l.at.X), f(l.at.Y+4.5), p.text, esc(l.text))
	}
	s.WriteString(`</svg>`)
	return s.String()
}

func arrow(tip Pt, side string, col string) string {
	n := normal(side)
	base := Pt{tip.X + n.X*14, tip.Y + n.Y*14}
	perp := Pt{-n.Y * 7, n.X * 7}
	return fmt.Sprintf(`<path d="M%s,%s L%s,%s L%s,%s Z" fill="%s"/>`,
		f(tip.X), f(tip.Y), f(base.X+perp.X), f(base.Y+perp.Y), f(base.X-perp.X), f(base.Y-perp.Y), col)
}

func cardSvg(n Node, c *Card, p palette, clipID string) string {
	var s strings.Builder
	col := ColorHex(n.Color)
	stroke := p.stroke
	if col != "" {
		stroke = col
	}
	dash := ""
	if c != nil && c.Kind == "missing" {
		stroke, dash = "#d04040", ` stroke-dasharray="6 4"`
	}
	fmt.Fprintf(&s, `<g class="canvas-card"><clipPath id="%s"><rect x="%s" y="%s" width="%s" height="%s" rx="8"/></clipPath>`,
		clipID, f(n.X), f(n.Y), f(n.Width), f(n.Height))
	fmt.Fprintf(&s, `<rect x="%s" y="%s" width="%s" height="%s" rx="8" fill="%s" stroke="%s" stroke-width="2"%s/>`,
		f(n.X), f(n.Y), f(n.Width), f(n.Height), p.card, stroke, dash)
	if col != "" {
		fmt.Fprintf(&s, `<rect x="%s" y="%s" width="%s" height="%s" rx="8" fill="%s" fill-opacity="0.08"/>`, f(n.X), f(n.Y), f(n.Width), f(n.Height), col)
	}
	fmt.Fprintf(&s, `<g clip-path="url(#%s)">`, clipID)

	// A picture fills its card.
	if c != nil && c.Kind == "image" && c.Src != "" {
		fmt.Fprintf(&s, `<image href="%s" xlink:href="%s" x="%s" y="%s" width="%s" height="%s" preserveAspectRatio="xMidYMid meet"/>`,
			esc(c.Src), esc(c.Src), f(n.X+2), f(n.Y+2), f(n.Width-4), f(n.Height-4))
		s.WriteString(`</g></g>`)
		return s.String()
	}

	type line struct {
		text   string
		size   float64
		weight string
		fill   string
	}
	lines := []line{}
	add := func(text string, size float64, weight, fill string) {
		per := int(math.Floor((n.Width - 24) / (size * 0.56)))
		for _, w := range wrap(text, per) {
			lines = append(lines, line{w, size, weight, fill})
		}
	}
	body := func(plain string) {
		for _, l := range strings.Split(plain, "\n") {
			if strings.TrimSpace(l) == "" {
				lines = append(lines, line{"", 7, "", p.text})
				continue
			}
			add(strings.TrimRight(l, " "), 13, "", p.text)
		}
	}
	switch {
	case c != nil && c.Kind == "missing":
		add("Not found: "+Label(n, nil), 14, "600", "#d04040")
		if c.Msg != "" {
			add(c.Msg, 12, "", p.dim)
		}
	case c != nil && (c.Kind == "heading" || c.Kind == "org"):
		title := c.Title
		if c.Priority != "" {
			title = "[#" + c.Priority + "] " + title
		}
		if c.Keyword != "" {
			kc := "#c0392b"
			if c.Done {
				kc = "#2e8b57"
			}
			add(c.Keyword, 12, "700", kc)
		}
		add(title, 15, "600", p.text)
		if len(c.Tags) > 0 {
			add(":"+strings.Join(c.Tags, ":")+":", 12, "", p.dim)
		}
		if c.Plain != "" {
			lines = append(lines, line{"", 5, "", p.text})
			// Links as their descriptions, as a text card's are.
			body(strings.Join(PlainText(c.Plain), "\n"))
		}
	case n.Type == "text":
		first := true
		for _, l := range strings.Split(n.Text, "\n") {
			isHead := headingLineRe.MatchString(l)
			pl := strings.Join(PlainText(l), " ")
			if strings.TrimSpace(pl) == "" {
				if !first {
					lines = append(lines, line{"", 7, "", p.text})
				}
				continue
			}
			first = false
			if isHead {
				add(pl, 15, "600", p.text)
			} else {
				add(pl, 13, "", p.text)
			}
		}
	case n.Type == "link":
		title := Host(n.URL)
		if c != nil && c.Title != "" {
			title = c.Title
		}
		add(title, 15, "600", p.text)
		add(n.URL, 12, "", "#3b73b9")
	case n.Type == "file":
		add(Label(n, nil), 15, "600", p.text)
		add(n.File, 12, "", p.dim)
	default:
		add(Label(n, c), 14, "600", p.text)
	}
	y := n.Y + 12
	for _, l := range lines {
		y += l.size * 1.35
		if y > n.Y+n.Height-4 {
			break
		}
		if l.text == "" {
			continue
		}
		weight := ""
		if l.weight != "" {
			weight = ` font-weight="` + l.weight + `"`
		}
		fmt.Fprintf(&s, `<text x="%s" y="%s" font-size="%s"%s fill="%s">%s</text>`, f(n.X+12), f(y), f(l.size), weight, l.fill, esc(l.text))
	}
	s.WriteString(`</g></g>`)
	return s.String()
}
