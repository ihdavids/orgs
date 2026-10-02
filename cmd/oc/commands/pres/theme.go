package pres

// Colours.
//
// A terminal deck takes its palette from the same file the browser decks do:
// `templates/slides_theme_<name>.css`. A theme there is a dozen custom
// properties (`--slide-bg`, `--slide-ink`, `--slide-accent`, ...) that the
// mapping stylesheet spreads over four frameworks, and they are exactly the
// dozen things a terminal can show too. So `#+REVEAL_THEME: nocturne` - or the
// neutral `#+SLIDE_THEME:` - is the same indigo and the same cyan whether the
// talk is given from worg's Presentations tab or from `orgs pres`, and a new
// theme is still one file.
//
// What css can say and a terminal cannot (fonts, gradients, shadows) is simply
// not read. An `rgba()` is blended onto the slide's ground, since a cell has no
// alpha. When there is no theme file to be had - no templates folder where this
// command is run - the built-in palette below is used, and `term` uses the
// terminal's own colours for people whose terminal is already the way they like
// it.

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2/styles"
	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/slides"
)

// palette is one theme, resolved to terminal colours.
type palette struct {
	name    string
	dark    bool
	bg      tcell.Color
	ink     tcell.Color
	soft    tcell.Color // secondary text: subtitles, footers, captions
	head    tcell.Color
	accent  tcell.Color
	link    tcell.Color
	rule    tcell.Color
	codeBg  tcell.Color
	codeInk tcell.Color
	bullet  string
	chroma  string // the chroma style code is coloured with
}

func (p *palette) base() tcell.Style { return tcell.StyleDefault.Background(p.bg).Foreground(p.ink) }

// The palette used when no theme file can be found: nocturne's colours, so a
// deck looks the same with and without a templates folder nearby.
func builtin() *palette {
	return &palette{
		name: "nocturne", dark: true,
		bg: hex(0x0e1130), ink: hex(0xd7dcf5), soft: hex(0x8d95c4), head: hex(0xffffff),
		accent: hex(0x7cdcf0), link: hex(0x9ee7f7), rule: hex(0x2c3160),
		codeBg: hex(0x0b0d26), codeInk: hex(0xcfd6f7), bullet: "▸", chroma: "tokyonight-moon",
	}
}

// term is the terminal's own colours, which is also what a transparent
// terminal needs: any colour of ours would paint over its background.
func termPalette() *palette {
	return &palette{
		name: "term", dark: true,
		bg: tcell.ColorDefault, ink: tcell.ColorDefault, soft: tcell.ColorGray, head: tcell.ColorDefault,
		accent: tcell.ColorTeal, link: tcell.ColorTeal, rule: tcell.ColorGray,
		codeBg: tcell.ColorDefault, codeInk: tcell.ColorDefault, bullet: "▸", chroma: "monokai",
	}
}

func hex(v int32) tcell.Color { return tcell.NewHexColor(v) }

var (
	cssVar  = regexp.MustCompile(`--slide-([a-z-]+)\s*:\s*([^;]+);`)
	hljsTag = regexp.MustCompile(`(?i)@hljs:\s*([A-Za-z0-9_./-]+)`)
)

// The highlight.js style a theme names, as the nearest chroma style. A name
// chroma knows itself is used as it stands.
var hljsToChroma = map[string]string{
	"night-owl":                  "tokyonight-moon",
	"github":                     "github",
	"github-dark":                "github-dark",
	"atom-one-dark":              "onedark",
	"atom-one-light":             "github",
	"tokyo-night-dark":           "tokyonight-night",
	"base16/gruvbox-light-soft":  "gruvbox-light",
	"base16/gruvbox-dark-medium": "gruvbox",
	"nord":                       "nord",
	"monokai":                    "monokai",
	"dracula":                    "dracula",
}

// loadPalette reads a theme by name. "" and "builtin" are the built-in palette,
// "term" the terminal's own; anything else is looked for as a slide theme, and
// an unknown name falls back to the built-in one rather than refusing to start
// a talk over a colour scheme.
func loadPalette(name string) (*palette, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "builtin", "default":
		return builtin(), true
	case "term", "terminal", "none":
		return termPalette(), true
	}
	t := slides.LoadTheme(name)
	if !t.Found {
		return builtin(), false
	}
	// The mapping comes first in t.CSS and the theme after it, so reading in
	// order leaves the theme's value wherever both set one.
	vars := map[string]string{}
	for _, m := range cssVar.FindAllStringSubmatch(t.CSS, -1) {
		vars[m[1]] = strings.TrimSpace(m[2])
	}
	p := builtin()
	p.name = name
	bgR, bgG, bgB, ok := cssColour(vars["bg"], [3]float64{14, 17, 48})
	if ok {
		p.bg = rgb(bgR, bgG, bgB)
	}
	ground := [3]float64{bgR, bgG, bgB}
	p.dark = lum(bgR, bgG, bgB) < 0.42
	set := func(dst *tcell.Color, keys ...string) {
		for _, k := range keys {
			if r, g, b, ok := cssColour(vars[k], ground); ok {
				*dst = rgb(r, g, b)
				return
			}
		}
	}
	set(&p.ink, "ink")
	set(&p.soft, "ink-soft", "ink")
	set(&p.head, "head-ink", "ink")
	set(&p.accent, "accent", "link")
	set(&p.link, "link", "accent")
	set(&p.rule, "rule", "ink-soft")
	set(&p.codeBg, "code-bg", "bg-soft", "card-bg")
	set(&p.codeInk, "code-ink", "ink")
	if !p.dark {
		// A light theme with the dark built-in's code colours would be a
		// navy hole in a cream slide when the theme says nothing of its own.
		if _, ok := vars["code-bg"]; !ok {
			p.codeBg = rgb(mix(bgR, 0, .05), mix(bgG, 0, .05), mix(bgB, 0, .05))
		}
		if _, ok := vars["code-ink"]; !ok {
			p.codeInk = p.ink
		}
	}
	if b := cssString(vars["bullet"]); b != "" {
		p.bullet = b
	}
	p.chroma = "github-dark"
	if !p.dark {
		p.chroma = "github"
	}
	if m := hljsTag.FindStringSubmatch(t.CSS); m != nil {
		if c, ok := hljsToChroma[strings.ToLower(m[1])]; ok {
			p.chroma = c
		} else if _, ok := styles.Registry[strings.ToLower(m[1])]; ok {
			p.chroma = strings.ToLower(m[1])
		}
	}
	return p, true
}

// themeNames is every theme this command can show, for `-list-themes` and for
// the key that cycles through them.
func themeNames() []string {
	names := slides.ThemeNames()
	sort.Strings(names)
	return append([]string{"builtin", "term"}, names...)
}

// cssColour reads one css colour, blending any alpha onto the ground.
func cssColour(v string, ground [3]float64) (r, g, b float64, ok bool) {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" || strings.Contains(v, "var(") || strings.Contains(v, "gradient") {
		return 0, 0, 0, false
	}
	a := 1.0
	switch {
	case strings.HasPrefix(v, "#"):
		h := v[1:]
		if len(h) == 3 || len(h) == 4 {
			x := ""
			for _, c := range h {
				x += string(c) + string(c)
			}
			h = x
		}
		if len(h) != 6 && len(h) != 8 {
			return 0, 0, 0, false
		}
		n, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return 0, 0, 0, false
		}
		if len(h) == 8 {
			a = float64(n&0xff) / 255
			n >>= 8
		}
		r, g, b = float64(n>>16&0xff), float64(n>>8&0xff), float64(n&0xff)
	case strings.HasPrefix(v, "rgb"):
		inner := v[strings.Index(v, "(")+1:]
		inner = strings.TrimSuffix(strings.TrimSpace(inner), ")")
		parts := strings.FieldsFunc(inner, func(c rune) bool { return c == ',' || c == ' ' || c == '/' })
		if len(parts) < 3 {
			return 0, 0, 0, false
		}
		f := func(s string) float64 {
			if strings.HasSuffix(s, "%") {
				x, _ := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
				return x / 100
			}
			x, _ := strconv.ParseFloat(s, 64)
			return x
		}
		r, g, b = f(parts[0]), f(parts[1]), f(parts[2])
		if len(parts) > 3 {
			a = f(parts[3])
		}
	default:
		c := tcell.GetColor(v)
		if c == tcell.ColorDefault {
			return 0, 0, 0, false
		}
		ri, gi, bi := c.RGB()
		r, g, b = float64(ri), float64(gi), float64(bi)
	}
	return mix(ground[0], r, a), mix(ground[1], g, a), mix(ground[2], b, a), true
}

// cssString reads a css content string, `"\25B8"` included.
func cssString(v string) string {
	v = strings.Trim(strings.TrimSpace(v), `"'`)
	out := strings.Builder{}
	for i := 0; i < len(v); i++ {
		if v[i] == '\\' {
			j := i + 1
			for j < len(v) && j < i+7 && strings.ContainsRune("0123456789abcdefABCDEF", rune(v[j])) {
				j++
			}
			if n, err := strconv.ParseUint(v[i+1:j], 16, 32); err == nil && j > i+1 {
				out.WriteRune(rune(n))
				i = j - 1
				if i+1 < len(v) && v[i+1] == ' ' {
					i++
				}
				continue
			}
		}
		out.WriteByte(v[i])
	}
	return strings.TrimSpace(out.String())
}

func mix(a, b, t float64) float64 { return a + (b-a)*t }

func rgb(r, g, b float64) tcell.Color {
	c := func(x float64) int32 { return int32(math.Max(0, math.Min(255, math.Round(x)))) }
	return tcell.NewRGBColor(c(r), c(g), c(b))
}

func lum(r, g, b float64) float64 {
	f := func(c float64) float64 {
		c /= 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*f(r) + 0.7152*f(g) + 0.0722*f(b)
}

// lerp blends two terminal colours, which is what a fade is made of. A colour
// with no value of its own (the terminal's default) cannot be blended, so the
// nearer end is taken.
func lerp(a, b tcell.Color, t float64) tcell.Color {
	ar, ag, ab := a.RGB()
	br, bg, bb := b.RGB()
	if ar < 0 || br < 0 {
		if t < 0.5 {
			return a
		}
		return b
	}
	return rgb(mix(float64(ar), float64(br), t), mix(float64(ag), float64(bg), t), mix(float64(ab), float64(bb), t))
}

// templateDirs is where to look for theme files when the config names none.
func templateDirs(configured string) string {
	if configured != "" {
		if st, err := os.Stat(configured); err == nil && st.IsDir() {
			return configured
		}
	}
	if exe, err := os.Executable(); err == nil {
		d := filepath.Join(filepath.Dir(exe), "templates")
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	return configured
}
