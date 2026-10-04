package gantt

// What a bar's colour means, and how to say a colour to a terminal.
//
// The schemes and the palettes are worg's (worg/src/ganttcolor.ts and the
// LIGHT_PALETTE / DARK_PALETTE / CATEGORICAL in ganttstyle.ts), so a plan
// coloured by assignee in the browser is the same colours here and the legend
// can be learned once.

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
)

type rgb struct{ r, g, b int }

type palette struct {
	bg, todo, active, done, crit, lane, text rgb
}

var darkPalette = palette{
	bg: hex("#12161c"), todo: hex("#5b9bd5"), active: hex("#3fb984"),
	done: hex("#6b7480"), crit: hex("#e0655b"), lane: hex("#1c222b"), text: hex("#dfe4ea"),
}

var lightPalette = palette{
	bg: hex("#ffffff"), todo: hex("#4a90d9"), active: hex("#2ecc71"),
	done: hex("#95a5a6"), crit: hex("#e74c3c"), lane: hex("#f4f4f4"), text: hex("#333333"),
}

var categorical = []rgb{
	hex("#4a90d9"), // blue
	hex("#e2703a"), // orange
	hex("#4aa96c"), // green
	hex("#7a5ca4"), // violet
	hex("#d4a12a"), // gold
	hex("#3aa8a8"), // teal
	hex("#d1577e"), // rose
	hex("#7d8d3f"), // olive
	hex("#5f7fd4"), // indigo
	hex("#a54d40"), // brick
	hex("#59a1c9"), // sky
	hex("#9b6a4f"), // brown
}

var (
	todayColor    = hex("#e74c3c")
	overdueColor  = hex("#e74c3c")
	slippingColor = hex("#e67e22")
	markColor     = hex("#8e44ad")
)

func hex(s string) rgb {
	c, _ := parseColor(s)
	return c
}

// A few names, for the GANTT_COLOR somebody wrote as a word. A name the
// terminal cannot be told about falls back to the scheme's own colour.
var named = map[string]string{
	"red": "#e74c3c", "orange": "#e67e22", "yellow": "#f1c40f", "gold": "#d4a12a",
	"green": "#2ecc71", "teal": "#3aa8a8", "cyan": "#1abc9c", "blue": "#4a90d9",
	"navy": "#34495e", "indigo": "#5f7fd4", "purple": "#8e44ad", "violet": "#7a5ca4",
	"pink": "#d1577e", "magenta": "#c0398e", "brown": "#9b6a4f", "grey": "#95a5a6",
	"gray": "#95a5a6", "black": "#2c3e50", "white": "#ecf0f1", "olive": "#7d8d3f",
}

func parseColor(s string) (rgb, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if v, ok := named[s]; ok {
		s = v
	}
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return rgb{}, false
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return rgb{}, false
	}
	return rgb{int(n >> 16 & 255), int(n >> 8 & 255), int(n & 255)}, true
}

// mix is a colour part of the way from one to another.
func mix(a, b rgb, k float64) rgb {
	if k < 0 {
		k = 0
	}
	if k > 1 {
		k = 1
	}
	f := func(x, y int) int { return int(float64(x) + float64(y-x)*k + 0.5) }
	return rgb{f(a.r, b.r), f(a.g, b.g), f(a.b, b.b)}
}

func (c rgb) luminance() float64 {
	return (0.2126*float64(c.r) + 0.7152*float64(c.g) + 0.0722*float64(c.b)) / 255
}

// ink is the text colour that reads on top of a bar.
func (c rgb) ink() rgb {
	if c.luminance() > 0.6 {
		return rgb{18, 20, 26}
	}
	return rgb{255, 255, 255}
}

// --- saying it to a terminal -------------------------------------------------

// trueColour is whether the terminal takes 24-bit colour. Most do now; the ones
// that do not (Apple's Terminal before macOS 26, a bare linux console) get the
// nearest of xterm's 256, which is close enough to tell blue from orange.
var trueColour = func() bool {
	switch strings.ToLower(os.Getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return true
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app", "WezTerm", "vscode", "ghostty", "Hyper", "Tabby", "WarpTerminal":
		return true
	}
	t := os.Getenv("TERM")
	return strings.Contains(t, "kitty") || strings.Contains(t, "alacritty") ||
		strings.Contains(t, "ghostty") || strings.Contains(t, "direct")
}()

func cube(v int) int {
	if v < 48 {
		return 0
	}
	if v < 115 {
		return 1
	}
	return (v - 35) / 40
}

func ansi256(c rgb) int {
	r, g, b := cube(c.r), cube(c.g), cube(c.b)
	levels := []int{0, 95, 135, 175, 215, 255}
	cr, cg, cb := levels[r], levels[g], levels[b]
	// A grey is better said from the grey ramp than the cube's six greys.
	avg := (c.r + c.g + c.b) / 3
	gi := (avg - 3) / 10
	if gi < 0 {
		gi = 0
	}
	if gi > 23 {
		gi = 23
	}
	gv := 8 + gi*10
	dist := func(x, y, z int) int {
		return (c.r-x)*(c.r-x) + (c.g-y)*(c.g-y) + (c.b-z)*(c.b-z)
	}
	if dist(gv, gv, gv) < dist(cr, cg, cb) {
		return 232 + gi
	}
	return 16 + 36*r + 6*g + b
}

func fg(c rgb) string {
	if !commands.Colour() {
		return ""
	}
	if trueColour {
		return fmt.Sprintf("\033[38;2;%d;%d;%dm", c.r, c.g, c.b)
	}
	return fmt.Sprintf("\033[38;5;%dm", ansi256(c))
}

func bg(c rgb) string {
	if !commands.Colour() {
		return ""
	}
	if trueColour {
		return fmt.Sprintf("\033[48;2;%d;%d;%dm", c.r, c.g, c.b)
	}
	return fmt.Sprintf("\033[48;5;%dm", ansi256(c))
}

// --- schemes -------------------------------------------------------------------

const unset = "(none)"

var colorModes = []string{"state", "status", "resource", "section", "progress"}

type legendEntry struct {
	label string
	color rgb
	count int
	own   bool // "a heading's own GANTT_COLOR", which has no one colour
}

type scheme struct {
	title  string
	color  func(t *Task) rgb
	legend []legendEntry
}

func stateColor(t *Task, p palette) rgb {
	switch t.State {
	case "active":
		return p.active
	case "done":
		return p.done
	case "crit":
		return p.crit
	}
	return p.todo
}

func valueOf(t *Task, mode, prop string) string {
	v := ""
	switch mode {
	case "status":
		v = t.Src.Status
	case "resource":
		v = t.Src.Resource
	case "section":
		v = t.Section
	case "property":
		v = propOf(t.Src.Props, prop)
	}
	if v = strings.TrimSpace(v); v == "" {
		return unset
	}
	return v
}

// buildScheme is worg's buildScheme. mode is one of colorModes, or the name of
// a property to colour by.
func buildScheme(tasks []*Task, mode string, p palette) scheme {
	switch mode {
	case "state":
		used := map[string]bool{}
		own := false
		for _, t := range tasks {
			if !t.Mark {
				used[t.State] = true
				if _, ok := parseColor(t.Src.Color); ok && t.Src.Color != "" {
					own = true
				}
			}
		}
		legend := []legendEntry{}
		add := func(label string, c rgb, state string) {
			if used[state] {
				legend = append(legend, legendEntry{label: label, color: c})
			}
		}
		add("To do", p.todo, "default")
		add("In progress", p.active, "active")
		add("Done", p.done, "done")
		add("Blocked", p.crit, "crit")
		if own {
			legend = append(legend, legendEntry{label: "Own GANTT_COLOR", own: true})
		}
		return scheme{
			title: "State",
			// A heading that names its own colour keeps it - but only here.
			// Under any other scheme the colour has a meaning the legend
			// states, and one bar quietly opting out would make it a lie.
			color: func(t *Task) rgb {
				if c, ok := parseColor(t.Src.Color); ok && t.Src.Color != "" {
					return c
				}
				return stateColor(t, p)
			},
			legend: legend,
		}
	case "progress":
		ramp := func(pct int) rgb {
			if pct >= 100 {
				return p.done
			}
			if pct < 0 {
				pct = 0
			}
			return mix(p.crit, p.active, float64(pct)/100)
		}
		return scheme{
			title: "Progress",
			color: func(t *Task) rgb { return ramp(t.Src.Percent) },
			legend: []legendEntry{
				{label: "Not started", color: ramp(0)},
				{label: "Half", color: ramp(50)},
				{label: "Nearly there", color: ramp(90)},
				{label: "Done", color: ramp(100)},
			},
		}
	}

	prop := ""
	title := map[string]string{"status": "Keyword", "resource": "Assignee", "section": "Lane"}[mode]
	if title == "" {
		prop = strings.ToUpper(strings.TrimSpace(mode))
		title = prop
		mode = "property"
	}
	// Distinct values in a stable order: alphabetical, the unset ones last. A
	// colour that moves when a task is added is a colour nobody can learn.
	counts := map[string]int{}
	for _, t := range tasks {
		if !t.Mark {
			counts[valueOf(t, mode, prop)]++
		}
	}
	values := []string{}
	for v := range counts {
		if v != unset {
			values = append(values, v)
		}
	}
	sort.Slice(values, func(i, j int) bool { return strings.ToLower(values[i]) < strings.ToLower(values[j]) })
	if counts[unset] > 0 {
		values = append(values, unset)
	}
	index := map[string]int{}
	for i, v := range values {
		index[v] = i
	}
	colorFor := func(v string) rgb {
		if v == unset {
			return p.done
		}
		return categorical[index[v]%len(categorical)]
	}
	legend := []legendEntry{}
	for _, v := range values {
		legend = append(legend, legendEntry{label: v, color: colorFor(v), count: counts[v]})
	}
	return scheme{
		title:  title,
		color:  func(t *Task) rgb { return colorFor(valueOf(t, mode, prop)) },
		legend: legend,
	}
}
