package slides

// Themes.
//
// A theme here is **one file for all four frameworks**, which is the only way
// this was worth doing. reveal.js ships twelve themes, deck.js three and
// WebSlides and impress.js none, and all of them are written against their own
// library's class names - so "make my deck look like this" was four different
// jobs and three of the four answers did not exist.
//
// A theme is therefore two things loaded together:
//
//   - `slides_theme_base.css`, the mapping, which says what a heading and a
//     table and a block of code are *in each of the four frameworks* and sets
//     them from a dozen custom properties; and
//   - `slides_theme_<name>.css`, the theme, which is those properties, plus
//     whatever flourish it wants.
//
// So a new theme is one file of colours and fonts and it works everywhere, and
// a framework that grows a new surface is fixed once in the mapping.
//
// The mapping is loaded *only when a theme is in use*. It has to be: its rules
// are written to win over the library's own, and leaving them on top of
// reveal's `league` would be a third thing fighting for the same headings.
//
// A name that is not one of ours is passed straight through to the framework,
// so `#+REVEAL_THEME: dracula` still means reveal's dracula.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
)

// Theme is a local theme, loaded.
type Theme struct {
	// Name is what the file asked for.
	Name string
	// CSS is the mapping and the theme, in that order, ready to inline.
	CSS string
	// Hljs is the highlight.js stylesheet this theme asks for. A theme that
	// has chosen its own greys cannot have somebody else's monokai dropped
	// into the middle of it, so a theme names its code colours and the file
	// can still override that.
	Hljs string
	// Mermaid is the diagram theme, for the same reason: a mermaid graph draws
	// itself on a white card, which on a dark deck is a hole in the slide.
	Mermaid string
	// Backdrop is the animated scene this theme asks for behind the deck
	// (`@backdrop: cubes`). It is named rather than written here for the same
	// reason the highlight.js style is: a theme is one file of colours, and a
	// field of turning cubes is not something a palette can say.
	Backdrop string
	// Found says whether there was a theme of that name at all. When there was
	// not, the name belongs to the framework and is passed through untouched.
	Found bool
}

const themePrefix = "slides_theme_"

var hljsHint = regexp.MustCompile(`(?i)@hljs:\s*([A-Za-z0-9_./-]+)`)
var mermaidHint = regexp.MustCompile(`(?i)@mermaid:\s*([A-Za-z0-9_-]+)`)
var backdropHint = regexp.MustCompile(`(?i)@backdrop:\s*([A-Za-z0-9_-]+)`)

// sceneName is what may be written into the page as a scene name. The name
// reaches the browser inside a javascript string literal, so this is the thing
// standing between a theme file and a script injection - and a name that is not
// of this shape is not one of ours whatever else it is.
var sceneName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// SearchPath is where themes are looked for, over and above the working
// directory's own ./templates. The server sets it from `templatePath:` at
// startup, because the one thing a theme must not depend on is which directory
// the server happened to be started in - and `orgs serve` is normally started
// somewhere else entirely.
var SearchPath = ""

// LoadTheme reads a theme by name. An empty name, or one this server has no
// file for, comes back with Found false and nothing else - the caller then
// leaves the framework's own theme alone.
func LoadTheme(name string) Theme {
	t := Theme{Name: strings.TrimSpace(name)}
	if t.Name == "" || strings.ContainsAny(t.Name, `/\.`) {
		// A name with a path in it is not a theme name. Refusing it here is
		// what keeps `#+REVEAL_THEME: ../../etc/passwd` from being a file read.
		return t
	}
	data, err := readTheme(themePrefix + t.Name + ".css")
	if err != nil {
		return t
	}
	base, _ := readTheme(themePrefix + "base.css")
	t.Found = true
	t.CSS = string(base) + "\n\n" + string(data)
	if m := hljsHint.FindStringSubmatch(string(data)); m != nil {
		t.Hljs = m[1]
	}
	if m := mermaidHint.FindStringSubmatch(string(data)); m != nil {
		t.Mermaid = m[1]
	}
	if m := backdropHint.FindStringSubmatch(string(data)); m != nil {
		t.Backdrop = strings.ToLower(m[1])
	}
	return t
}

// ThemeNames is every theme this server has, for a client offering a list.
//
// The list is read off the disk rather than kept in a table, so a theme
// somebody drops into their templates folder turns up in worg's dropdown
// without anything being rebuilt - which is the same promise the html themes
// make.
func ThemeNames() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, dir := range themeDirs() {
		matches, err := filepath.Glob(filepath.Join(dir, themePrefix+"*.css"))
		if err != nil {
			continue
		}
		for _, m := range matches {
			name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), themePrefix), ".css")
			if name == "base" || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// readTheme looks in the configured template path first and the working
// directory's ./templates after, which is the order the template manager
// itself resolves in.
func readTheme(name string) ([]byte, error) {
	for _, dir := range themeDirs() {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			return data, nil
		}
	}
	return os.ReadFile(plugs.PlugExpandTemplatePath(name))
}

func themeDirs() []string {
	dirs := []string{}
	if SearchPath != "" {
		dirs = append(dirs, SearchPath)
		if abs, err := filepath.Abs(SearchPath); err == nil && abs != SearchPath {
			dirs = append(dirs, abs)
		}
	}
	dirs = append(dirs, "./templates")
	if abs, err := filepath.Abs("./templates"); err == nil {
		dirs = append(dirs, abs)
	}
	return dirs
}

// ── The animated backdrop ───────────────────────────────────────────────────

// Backdrop is the scene behind a deck, resolved: which one, how it was tuned,
// and the two files that draw it.
//
// A zero Backdrop is "none", which is what a deck without a theme, a theme that
// did not ask for one, and `#+SLIDE_BACKDROP: nil` all come to - the template
// asks `{%if backdrop%}` and the page is byte for byte what it was before any
// of this existed.
type Backdrop struct {
	// Scene is the name the javascript looks up. Empty means no backdrop.
	Scene string
	// Config is the javascript object literal the page assigns to
	// window.ORG_BACKDROP: the scene, plus whatever the document overrode.
	Config string
	// CSS and JS are the two shared files, inlined like the autofit pair and
	// for the same reason - a deck shown from a memory stick is still a deck.
	CSS string
	JS  string
}

// ReadBackdrop decides which scene a deck gets.
//
// The theme asks for one and the document has the last word, because a theme is
// a thing somebody chose once and a deck is the talk they are giving on
// Thursday: `#+SLIDE_BACKDROP: globe` tries another, and
// `#+SLIDE_BACKDROP: nil` turns it off - which is the setting that matters,
// since a backdrop is the first thing to go when a projector cannot cope or the
// room cannot read it.
//
// What a scene is called is deliberately *not* checked against a list here. The
// scenes live in the javascript, and a list of their names in Go would be the
// second copy that goes quietly out of step - the same mistake as a capture
// template's placeholder names or a health band said twice. An unknown name is
// refused in the browser, out loud, where the scenes actually are.
func ReadBackdrop(c Conf, th Theme) Backdrop {
	b := Backdrop{}
	name := strings.ToLower(strings.TrimSpace(c.DocStr("BACKDROP", th.Backdrop)))
	if name == "" || !Truthy(name) || !sceneName.MatchString(name) {
		return b
	}
	b.Scene = name
	cfg := []string{`scene:` + quote(name)}
	// Speed and density are the two every scene has, because they are the two
	// things somebody asks for after seeing one: slower, and less of it. They
	// are also properties on the theme, so this only says what the *document*
	// said - a scene with nothing overridden reads the theme's own numbers.
	if v, ok := docNum(c, "BACKDROP_SPEED"); ok {
		cfg = append(cfg, `speed:`+strconv.FormatFloat(v, 'f', -1, 64))
	}
	if v, ok := docNum(c, "BACKDROP_DENSITY"); ok {
		cfg = append(cfg, `density:`+strconv.FormatFloat(v, 'f', -1, 64))
	}
	// `prefers-reduced-motion` is honoured by default, and a deck can overrule
	// it. That setting is a system-wide one, and somebody who turned it on for
	// their desktop has not thereby decided anything about the deck they are
	// about to present - so `#+SLIDE_BACKDROP_MOTION: always` says so, and
	// `never` is the other way round for a deck that wants the still frame.
	switch strings.ToLower(strings.TrimSpace(c.DocStr("BACKDROP_MOTION", ""))) {
	case "always":
		cfg = append(cfg, `motion:'always'`)
	case "never", "nil", "no", "off", "false":
		cfg = append(cfg, `motion:'never'`)
	}
	b.Config = "{" + strings.Join(cfg, ",") + "}"
	b.CSS, _ = readThemeStr("slides_backdrop.css")
	b.JS, _ = readThemeStr("slides_backdrop.js")
	// Two files that are not there is a server missing its templates, not a
	// deck asking for something odd - and half a backdrop (the css without the
	// script) is a page with an empty canvas over it.
	if b.JS == "" || b.CSS == "" {
		return Backdrop{}
	}
	return b
}

func docNum(c Conf, key string) (float64, bool) {
	v := strings.TrimSpace(c.DocStr(key, ""))
	if v == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 {
		return 0, false
	}
	return f, true
}

func readThemeStr(name string) (string, error) {
	data, err := readTheme(name)
	return string(data), err
}

// AutoFitAssets is the stylesheet and the script that shrink an overfull slide
// to fit, read from the templates folder like everything else here.
//
// They are inlined into the page rather than linked, for the same reason the
// theme is: a deck is often saved to disk and shown from there, and a file that
// stops working the moment it leaves the server is not an export.
func AutoFitAssets() (string, string) {
	css, _ := readTheme("slides_autofit.css")
	js, _ := readTheme("slides_autofit.js")
	return string(css), string(js)
}
