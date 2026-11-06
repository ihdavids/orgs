package excalidraw

import (
	"encoding/base64"
	"fmt"
	"html"
	"math"
	"strings"
)

// Render draws a scene as SVG for exporters that cannot run Excalidraw:
// rectangles, ellipses, diamonds, lines, arrows (with their heads), text,
// freehand strokes, pictures and frames. Plain lines, not Excalidraw's
// hand-drawn roughness - legible rather than identical. Hachure and
// cross-hatch fills are patterns of thin lines in the fill colour.
//
// The scene itself rides along in a <metadata> block in the form
// excalidraw.com reads back (payload-type, version 1: base64 of the json),
// so a picture this draws still opens as a drawing there.
func Render(s *Scene, sceneJSON []byte) string {
	els := s.Live()
	minX, minY, maxX, maxY := bounds(els)
	if minX > maxX {
		minX, minY, maxX, maxY = 0, 0, 200, 100
	}
	const pad = 10.0
	minX -= pad
	minY -= pad
	w := maxX - minX + pad
	h := maxY - minY + pad

	r := &renderer{}
	var body strings.Builder
	for _, e := range els {
		r.element(&body, e, s)
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" version="1.1" viewBox="%s %s %s %s" width="%s" height="%s">`,
		num(minX), num(minY), num(w), num(h), num(w), num(h))
	b.WriteString("\n<!-- svg-source:excalidraw -->\n<!-- drawn by orgs: plain shapes, not Excalidraw's own rendering -->\n")
	if len(sceneJSON) > 0 {
		b.WriteString("<metadata><!-- payload-type:application/vnd.excalidraw+json --><!-- payload-version:1 --><!-- payload-start -->")
		b.WriteString(base64.StdEncoding.EncodeToString(sceneJSON))
		b.WriteString("<!-- payload-end --></metadata>\n")
	}
	if len(r.defs) > 0 {
		b.WriteString("<defs>")
		for _, d := range r.defs {
			b.WriteString(d)
		}
		b.WriteString("</defs>\n")
	}
	if bg := s.Background(); bg != "" && bg != "transparent" {
		fmt.Fprintf(&b, `<rect x="%s" y="%s" width="%s" height="%s" fill="%s"/>`+"\n", num(minX), num(minY), num(w), num(h), attr(bg))
	}
	b.WriteString(body.String())
	b.WriteString("</svg>\n")
	return b.String()
}

type renderer struct {
	defs     []string
	patterns map[string]string
}

func num(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "0"
	}
	s := fmt.Sprintf("%.2f", f)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" || s == "" {
		return "0"
	}
	return s
}

func attr(s string) string { return html.EscapeString(s) }

// colour is a stroke or fill as SVG has it: "transparent" and "" are none.
func colour(c string) string {
	c = strings.TrimSpace(c)
	if c == "" || c == "transparent" {
		return "none"
	}
	return c
}

func isLinear(t string) bool { return t == "line" || t == "arrow" || t == "freedraw" }

// The element's box, corners turned by its angle.
func elementBounds(e Element) (x0, y0, x1, y1 float64) {
	if isLinear(e.Type) && len(e.Points) > 0 {
		x0, y0, x1, y1 = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for _, p := range e.Points {
			if len(p) < 2 {
				continue
			}
			x0, x1 = math.Min(x0, e.X+p[0]), math.Max(x1, e.X+p[0])
			y0, y1 = math.Min(y0, e.Y+p[1]), math.Max(y1, e.Y+p[1])
		}
		sw := e.StrokeWidth * 2
		x0, y0, x1, y1 = x0-sw, y0-sw, x1+sw, y1+sw
	} else {
		x0, y0, x1, y1 = e.X, e.Y, e.X+e.Width, e.Y+e.Height
	}
	if e.Angle == 0 || math.IsInf(x0, 0) {
		return
	}
	cx, cy := (x0+x1)/2, (y0+y1)/2
	sin, cos := math.Sin(e.Angle), math.Cos(e.Angle)
	nx0, ny0, nx1, ny1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, c := range [][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}} {
		dx, dy := c[0]-cx, c[1]-cy
		px, py := cx+dx*cos-dy*sin, cy+dx*sin+dy*cos
		nx0, nx1 = math.Min(nx0, px), math.Max(nx1, px)
		ny0, ny1 = math.Min(ny0, py), math.Max(ny1, py)
	}
	return nx0, ny0, nx1, ny1
}

func bounds(els []Element) (minX, minY, maxX, maxY float64) {
	minX, minY, maxX, maxY = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, e := range els {
		if e.Type == "selection" {
			continue
		}
		x0, y0, x1, y1 := elementBounds(e)
		if math.IsInf(x0, 0) {
			continue
		}
		minX, minY = math.Min(minX, x0), math.Min(minY, y0)
		maxX, maxY = math.Max(maxX, x1), math.Max(maxY, y1)
	}
	return
}

// fill is the fill attribute for an element's background: a colour, or a
// pattern of lines for hachure and cross-hatch (zigzag drawn as hachure).
func (r *renderer) fill(e Element) string {
	c := colour(e.BackgroundColor)
	if c == "none" {
		return "none"
	}
	style := e.FillStyle
	if style != "hachure" && style != "cross-hatch" && style != "zigzag" {
		return attr(c)
	}
	key := style + "|" + c
	if r.patterns == nil {
		r.patterns = map[string]string{}
	}
	id, ok := r.patterns[key]
	if !ok {
		id = fmt.Sprintf("orgs-fill-%d", len(r.patterns))
		r.patterns[key] = id
		lines := fmt.Sprintf(`<line x1="0" y1="0" x2="0" y2="8" stroke="%s" stroke-width="1.5"/>`, attr(c))
		if style == "cross-hatch" {
			lines += fmt.Sprintf(`<line x1="0" y1="0" x2="8" y2="0" stroke="%s" stroke-width="1.5"/>`, attr(c))
		}
		r.defs = append(r.defs, fmt.Sprintf(`<pattern id="%s" patternUnits="userSpaceOnUse" width="8" height="8" patternTransform="rotate(-41)">%s</pattern>`, id, lines))
	}
	return "url(#" + id + ")"
}

func strokeAttrs(e Element, round bool) string {
	sw := e.StrokeWidth
	if sw <= 0 {
		sw = 1
	}
	s := fmt.Sprintf(` stroke="%s" stroke-width="%s"`, attr(colour(e.StrokeColor)), num(sw))
	switch e.StrokeStyle {
	case "dashed":
		s += fmt.Sprintf(` stroke-dasharray="8 %s"`, num(8+sw))
	case "dotted":
		s += fmt.Sprintf(` stroke-dasharray="1.5 %s"`, num(6+sw))
		round = true
	}
	if round {
		s += ` stroke-linecap="round" stroke-linejoin="round"`
	}
	return s
}

// groupOpen is the element's <g>: its turn about its centre and its
// opacity.
func groupOpen(b *strings.Builder, e Element) {
	b.WriteString("<g")
	if e.Angle != 0 {
		x0, y0, x1, y1 := e.X, e.Y, e.X+e.Width, e.Y+e.Height
		if isLinear(e.Type) && len(e.Points) > 0 {
			x0, y0, x1, y1 = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
			for _, p := range e.Points {
				if len(p) >= 2 {
					x0, x1 = math.Min(x0, e.X+p[0]), math.Max(x1, e.X+p[0])
					y0, y1 = math.Min(y0, e.Y+p[1]), math.Max(y1, e.Y+p[1])
				}
			}
		}
		fmt.Fprintf(b, ` transform="rotate(%s %s %s)"`, num(e.Angle*180/math.Pi), num((x0+x1)/2), num((y0+y1)/2))
	}
	if e.Opacity != nil && *e.Opacity < 100 {
		fmt.Fprintf(b, ` opacity="%s"`, num(math.Max(0, *e.Opacity)/100))
	}
	b.WriteString(">")
}

func cornerRadius(e Element) float64 {
	if e.Roundness == nil {
		return 0
	}
	m := math.Min(math.Abs(e.Width), math.Abs(e.Height))
	if e.Roundness.Type == 3 {
		// Excalidraw's adaptive radius: 32, or a quarter of a small box.
		r := 32.0
		if e.Roundness.Value != nil {
			r = *e.Roundness.Value
		}
		if m*0.25 < r {
			return m * 0.25
		}
		return r
	}
	return m * 0.25
}

func (r *renderer) element(b *strings.Builder, e Element, s *Scene) {
	switch e.Type {
	case "rectangle", "ellipse", "diamond", "line", "arrow", "freedraw", "text", "image", "frame", "magicframe", "embeddable", "iframe":
	default:
		return
	}
	groupOpen(b, e)
	defer b.WriteString("</g>\n")
	x, y, w, h := e.X, e.Y, e.Width, e.Height
	switch e.Type {
	case "rectangle":
		rx := cornerRadius(e)
		fmt.Fprintf(b, `<rect x="%s" y="%s" width="%s" height="%s"`, num(x), num(y), num(math.Abs(w)), num(math.Abs(h)))
		if rx > 0 {
			fmt.Fprintf(b, ` rx="%s"`, num(rx))
		}
		fmt.Fprintf(b, ` fill="%s"%s/>`, r.fill(e), strokeAttrs(e, false))
	case "ellipse":
		fmt.Fprintf(b, `<ellipse cx="%s" cy="%s" rx="%s" ry="%s" fill="%s"%s/>`,
			num(x+w/2), num(y+h/2), num(math.Abs(w/2)), num(math.Abs(h/2)), r.fill(e), strokeAttrs(e, false))
	case "diamond":
		fmt.Fprintf(b, `<polygon points="%s,%s %s,%s %s,%s %s,%s" fill="%s"%s stroke-linejoin="round"/>`,
			num(x+w/2), num(y), num(x+w), num(y+h/2), num(x+w/2), num(y+h), num(x), num(y+h/2), r.fill(e), strokeAttrs(e, false))
	case "line", "arrow":
		r.linear(b, e)
	case "freedraw":
		pts := absPoints(e)
		if len(pts) == 0 {
			return
		}
		sw := e.StrokeWidth
		if sw <= 0 {
			sw = 1
		}
		if len(pts) == 1 {
			fmt.Fprintf(b, `<circle cx="%s" cy="%s" r="%s" fill="%s"/>`, num(pts[0][0]), num(pts[0][1]), num(sw*1.25), attr(colour(e.StrokeColor)))
			return
		}
		fmt.Fprintf(b, `<path d="%s" fill="none" stroke="%s" stroke-width="%s" stroke-linecap="round" stroke-linejoin="round"/>`,
			smoothPath(pts, false), attr(colour(e.StrokeColor)), num(sw*2.5))
	case "text":
		r.text(b, e)
	case "image":
		if e.FileID == nil || s.Files == nil {
			fmt.Fprintf(b, `<rect x="%s" y="%s" width="%s" height="%s" fill="#eee" stroke="#bbb"/>`, num(x), num(y), num(w), num(h))
			return
		}
		f, ok := s.Files[*e.FileID]
		if !ok || !strings.HasPrefix(f.DataURL, "data:image/") {
			fmt.Fprintf(b, `<rect x="%s" y="%s" width="%s" height="%s" fill="#eee" stroke="#bbb"/>`, num(x), num(y), num(w), num(h))
			return
		}
		fmt.Fprintf(b, `<image x="%s" y="%s" width="%s" height="%s" preserveAspectRatio="none" href="%s"/>`,
			num(x), num(y), num(w), num(h), attr(f.DataURL))
	case "frame", "magicframe":
		fmt.Fprintf(b, `<rect x="%s" y="%s" width="%s" height="%s" rx="8" fill="none" stroke="#bbb" stroke-width="1" stroke-dasharray="6 4"/>`,
			num(x), num(y), num(w), num(h))
		name := "Frame"
		if e.Name != nil && *e.Name != "" {
			name = *e.Name
		}
		fmt.Fprintf(b, `<text x="%s" y="%s" font-family="Helvetica, Arial, sans-serif" font-size="14" fill="#999">%s</text>`,
			num(x), num(y-6), html.EscapeString(name))
	case "embeddable", "iframe":
		fmt.Fprintf(b, `<rect x="%s" y="%s" width="%s" height="%s" fill="#f4f4f4"%s/>`, num(x), num(y), num(w), num(h), strokeAttrs(e, false))
		if e.Link != nil && *e.Link != "" {
			fmt.Fprintf(b, `<text x="%s" y="%s" font-family="Helvetica, Arial, sans-serif" font-size="14" fill="#666" text-anchor="middle">%s</text>`,
				num(x+w/2), num(y+h/2), html.EscapeString(*e.Link))
		}
	}
}

func absPoints(e Element) [][2]float64 {
	out := make([][2]float64, 0, len(e.Points))
	for _, p := range e.Points {
		if len(p) >= 2 {
			out = append(out, [2]float64{e.X + p[0], e.Y + p[1]})
		}
	}
	return out
}

// smoothPath is a Catmull-Rom curve through the points as cubic Béziers,
// which is near enough to Excalidraw's rounded lines and freehand strokes.
func smoothPath(pts [][2]float64, closed bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "M%s %s", num(pts[0][0]), num(pts[0][1]))
	n := len(pts)
	at := func(i int) [2]float64 {
		if closed {
			return pts[(i+n)%n]
		}
		if i < 0 {
			return pts[0]
		}
		if i >= n {
			return pts[n-1]
		}
		return pts[i]
	}
	last := n - 1
	if closed {
		last = n
	}
	for i := 0; i < last; i++ {
		p0, p1, p2, p3 := at(i-1), at(i), at(i+1), at(i+2)
		c1 := [2]float64{p1[0] + (p2[0]-p0[0])/6, p1[1] + (p2[1]-p0[1])/6}
		c2 := [2]float64{p2[0] - (p3[0]-p1[0])/6, p2[1] - (p3[1]-p1[1])/6}
		fmt.Fprintf(&b, " C%s %s %s %s %s %s", num(c1[0]), num(c1[1]), num(c2[0]), num(c2[1]), num(p2[0]), num(p2[1]))
	}
	if closed {
		b.WriteString(" Z")
	}
	return b.String()
}

func straightPath(pts [][2]float64, closed bool) string {
	var b strings.Builder
	for i, p := range pts {
		if i == 0 {
			fmt.Fprintf(&b, "M%s %s", num(p[0]), num(p[1]))
		} else {
			fmt.Fprintf(&b, " L%s %s", num(p[0]), num(p[1]))
		}
	}
	if closed {
		b.WriteString(" Z")
	}
	return b.String()
}

func (r *renderer) linear(b *strings.Builder, e Element) {
	pts := absPoints(e)
	if len(pts) < 2 {
		return
	}
	closed := e.Type == "line" && (e.Polygon || (len(pts) > 2 && pts[0] == pts[len(pts)-1]))
	d := ""
	if e.Roundness != nil && !e.Elbowed && len(pts) > 2 {
		d = smoothPath(pts, false)
	} else {
		d = straightPath(pts, closed)
	}
	f := "none"
	if closed {
		f = r.fill(e)
	}
	fmt.Fprintf(b, `<path d="%s" fill="%s"%s/>`, d, f, strokeAttrs(e, true))
	if e.Type != "arrow" && e.StartArrowhead == nil && e.EndArrowhead == nil {
		return
	}
	if e.StartArrowhead != nil && *e.StartArrowhead != "" {
		arrowhead(b, e, *e.StartArrowhead, pts[0], pts[1])
	}
	if e.EndArrowhead != nil && *e.EndArrowhead != "" {
		arrowhead(b, e, *e.EndArrowhead, pts[len(pts)-1], pts[len(pts)-2])
	}
}

// arrowhead draws one head at tip, pointing away from from.
func arrowhead(b *strings.Builder, e Element, kind string, tip, from [2]float64) {
	dx, dy := tip[0]-from[0], tip[1]-from[1]
	seg := math.Hypot(dx, dy)
	if seg == 0 {
		return
	}
	ux, uy := dx/seg, dy/seg
	sw := e.StrokeWidth
	if sw <= 0 {
		sw = 1
	}
	size := math.Min(18+sw*3, seg*0.75)
	stroke := attr(colour(e.StrokeColor))
	side := func(deg float64) [2]float64 {
		a := deg * math.Pi / 180
		// Back from the tip along the line, turned by a.
		bx, by := -ux*size, -uy*size
		return [2]float64{tip[0] + bx*math.Cos(a) - by*math.Sin(a), tip[1] + bx*math.Sin(a) + by*math.Cos(a)}
	}
	switch kind {
	case "arrow":
		l, rr := side(25), side(-25)
		fmt.Fprintf(b, `<path d="M%s %s L%s %s L%s %s" fill="none" stroke="%s" stroke-width="%s" stroke-linecap="round" stroke-linejoin="round"/>`,
			num(l[0]), num(l[1]), num(tip[0]), num(tip[1]), num(rr[0]), num(rr[1]), stroke, num(sw))
	case "triangle", "triangle_outline":
		l, rr := side(25), side(-25)
		fill := stroke
		if kind == "triangle_outline" {
			fill = "none"
		}
		fmt.Fprintf(b, `<polygon points="%s,%s %s,%s %s,%s" fill="%s" stroke="%s" stroke-width="%s" stroke-linejoin="round"/>`,
			num(l[0]), num(l[1]), num(tip[0]), num(tip[1]), num(rr[0]), num(rr[1]), fill, stroke, num(sw))
	case "bar":
		half := size / 2
		fmt.Fprintf(b, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s" stroke-linecap="round"/>`,
			num(tip[0]-uy*half), num(tip[1]+ux*half), num(tip[0]+uy*half), num(tip[1]-ux*half), stroke, num(sw))
	case "dot", "circle", "circle_outline":
		rad := math.Max(sw*2, size/4)
		fill := stroke
		if kind == "circle_outline" {
			fill = "none"
		}
		fmt.Fprintf(b, `<circle cx="%s" cy="%s" r="%s" fill="%s" stroke="%s" stroke-width="%s"/>`,
			num(tip[0]-ux*rad), num(tip[1]-uy*rad), num(rad), fill, stroke, num(sw))
	case "diamond", "diamond_outline":
		half := size / 3
		back := [2]float64{tip[0] - ux*size*0.66, tip[1] - uy*size*0.66}
		mid := [2]float64{tip[0] - ux*size*0.33, tip[1] - uy*size*0.33}
		fill := stroke
		if kind == "diamond_outline" {
			fill = "none"
		}
		fmt.Fprintf(b, `<polygon points="%s,%s %s,%s %s,%s %s,%s" fill="%s" stroke="%s" stroke-width="%s"/>`,
			num(tip[0]), num(tip[1]), num(mid[0]-uy*half), num(mid[1]+ux*half), num(back[0]), num(back[1]), num(mid[0]+uy*half), num(mid[1]-ux*half), fill, stroke, num(sw))
	default:
		// Anything newer than this knows: the plain arrow.
		arrowhead(b, e, "arrow", tip, from)
	}
}

// fontStack is the CSS for Excalidraw's font numbers: 1 Virgil, 2
// Helvetica, 3 Cascadia, 5 Excalifont, 6 Nunito, 7 Lilita One, 8 Comic
// Shanns, 9 Liberation Sans. The hand-drawn fonts fall back to a cursive one
// a reader is likely to have.
func fontStack(n int) string {
	switch n {
	case 2, 6, 9:
		return "Helvetica, Nunito, Arial, sans-serif"
	case 3, 8:
		return "Cascadia, 'Comic Shanns', Menlo, Consolas, monospace"
	case 7:
		return "'Lilita One', Impact, sans-serif"
	default:
		return "Excalifont, Virgil, 'Segoe Print', 'Comic Sans MS', 'Chalkboard SE', cursive"
	}
}

func (r *renderer) text(b *strings.Builder, e Element) {
	text := e.Text
	if text == "" {
		text = e.OriginalText
	}
	if text == "" {
		return
	}
	size := e.FontSize
	if size <= 0 {
		size = 20
	}
	lh := e.LineHeight
	if lh <= 0 {
		lh = 1.25
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	anchor, x := "start", e.X
	switch e.TextAlign {
	case "center":
		anchor, x = "middle", e.X+e.Width/2
	case "right":
		anchor, x = "end", e.X+e.Width
	}
	box := size * lh
	fmt.Fprintf(b, `<text font-family="%s" font-size="%s" fill="%s" text-anchor="%s" style="white-space: pre">`,
		attr(fontStack(e.FontFamily)), num(size), attr(colour(e.StrokeColor)), anchor)
	for i, l := range lines {
		// The baseline: the line's box, the glyphs centred in it, about 4/5
		// of the size down from their top.
		base := e.Y + float64(i)*box + (box-size)/2 + size*0.8
		fmt.Fprintf(b, `<tspan x="%s" y="%s">%s</tspan>`, num(x), num(base), html.EscapeString(l))
	}
	b.WriteString("</text>")
}
