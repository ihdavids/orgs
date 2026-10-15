package site

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"sort"
	"strings"

	chtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/ihdavids/orgs/internal/assets"
	"gopkg.in/yaml.v3"
)

// A theme is a folder under site/ in any template folder: theme.yaml says
// what it is and how its pages are laid out, style.css dresses them, and any
// of base's templates may be replaced by a file of the same name. The base
// templates and stylesheet are in site/base/ (not _base: go:embed leaves out
// names starting with _ or .).
//
// Each file is looked up on its own through the template search path, so a
// user's folder can add a theme (site/<name>/theme.yaml and style.css) or
// replace one file of a built-in one, with no Go.

// Layout is how a theme arranges a page. The templates read it; the switches
// are what keeps seven themes on one set of templates.
type Layout struct {
	// "tree": a navigation tree of every page down the left (docs).
	Sidebar string `yaml:"sidebar"`
	// Where a page's contents go: "aside" (beside the text), "inline" (above
	// it) or "none".
	TOC string `yaml:"toc"`
	// Where "Linked from" goes: "bottom" or "aside".
	Backlinks string `yaml:"backlinks"`
	// How the home page lists posts: "list", "cards" or "magazine".
	Home string `yaml:"home"`
	// Dark until the reader picks light, whatever the system says (ink).
	DarkDefault bool `yaml:"darkDefault"`
	// Footnotes become margin notes where there is room (tufte).
	Sidenotes bool `yaml:"sidenotes"`
}

// Theme is one theme's theme.yaml.
type Theme struct {
	Name        string   `yaml:"-"`
	Title       string   `yaml:"name"`
	Description string   `yaml:"description"`
	Fonts       []string `yaml:"fonts"`
	Highlight   struct {
		Light string `yaml:"light"`
		Dark  string `yaml:"dark"`
	} `yaml:"highlight"`
	Layout Layout `yaml:"layout"`
}

// ThemeInfo is a theme as a listing shows it.
type ThemeInfo struct {
	Name        string
	Description string
}

// Themes lists every theme on the template search path.
func Themes() []ThemeInfo {
	out := []ThemeInfo{}
	for _, f := range assets.GlobTemplates("site/*/theme.yaml") {
		name := strings.TrimSuffix(strings.TrimPrefix(f, "site/"), "/theme.yaml")
		if name == "base" {
			continue
		}
		info := ThemeInfo{Name: name}
		if th, err := loadTheme(name); err == nil {
			info.Description = th.Description
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func readTemplateFile(rel string) ([]byte, bool) {
	p := assets.FindTemplate(rel)
	if p == "" {
		return nil, false
	}
	data, err := os.ReadFile(p)
	return data, err == nil
}

func loadTheme(name string) (*Theme, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" || name == "base" || strings.ContainsAny(name, `/\.`) {
		return nil, fmt.Errorf("no site theme called %q", name)
	}
	data, ok := readTemplateFile("site/" + name + "/theme.yaml")
	if !ok {
		names := []string{}
		for _, t := range Themes() {
			names = append(names, t.Name)
		}
		return nil, fmt.Errorf("no site theme called %q (there are: %s)", name, strings.Join(names, ", "))
	}
	th := &Theme{}
	if err := yaml.Unmarshal(data, th); err != nil {
		return nil, fmt.Errorf("site/%s/theme.yaml: %v", name, err)
	}
	th.Name = name
	if th.Title == "" {
		th.Title = humanize(name)
	}
	if th.Layout.TOC == "" {
		th.Layout.TOC = "aside"
	}
	if th.Layout.Backlinks == "" {
		th.Layout.Backlinks = "bottom"
	}
	if th.Layout.Home == "" {
		th.Layout.Home = "list"
	}
	if th.Highlight.Light == "" {
		th.Highlight.Light = "github"
	}
	if th.Highlight.Dark == "" {
		th.Highlight.Dark = "github-dark"
	}
	return th, nil
}

// file is a theme's own copy of a file, else base's.
func (th *Theme) file(name string) ([]byte, bool) {
	if data, ok := readTemplateFile("site/" + th.Name + "/" + name); ok {
		return data, true
	}
	return readTemplateFile("site/base/" + name)
}

// ownFile is only the theme's copy, for files that add to base's.
func (th *Theme) ownFile(name string) ([]byte, bool) {
	return readTemplateFile("site/" + th.Name + "/" + name)
}

var funcs = template.FuncMap{
	"dict": func(kv ...interface{}) map[string]interface{} {
		m := map[string]interface{}{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[fmt.Sprint(kv[i])] = kv[i+1]
		}
		return m
	},
	"plural": func(n int, one, many string) string {
		if n == 1 {
			return fmt.Sprintf("%d %s", n, one)
		}
		return fmt.Sprintf("%d %s", n, many)
	},
	"lower": strings.ToLower,
	"upper": strings.ToUpper,
	"first": func(s string) string {
		for _, r := range s {
			return strings.ToUpper(string(r))
		}
		return ""
	},
}

// templates parses what a kind of page needs: base's layout and partials,
// the theme's replacements for them, then the kind's own template - base's
// unless the theme has one. A later definition of a {{define}} replaces an
// earlier one, which is how a theme changes a part without copying the rest.
func (th *Theme) templates(kind string) (*template.Template, error) {
	t := template.New("site").Funcs(funcs)
	parse := func(rel string) error {
		data, ok := readTemplateFile(rel)
		if !ok {
			return nil
		}
		_, err := t.New(rel).Parse(string(data))
		if err != nil {
			return fmt.Errorf("%s: %v", rel, err)
		}
		return nil
	}
	for _, rel := range []string{
		"site/base/layout.html", "site/base/partials.html",
		"site/" + th.Name + "/layout.html", "site/" + th.Name + "/partials.html",
		"site/base/" + kind + ".html", "site/" + th.Name + "/" + kind + ".html",
	} {
		if err := parse(rel); err != nil {
			return nil, err
		}
	}
	if t.Lookup("layout") == nil {
		return nil, fmt.Errorf("site theme %s: no layout template (site/base/layout.html is missing)", th.Name)
	}
	return t, nil
}

var cssFormatter = chtml.New(chtml.WithClasses(true), chtml.WithAllClasses(true))

// highlightCSS is the code colouring for both light and dark, the dark half
// scoped the way the themes scope their own colours: the reader's choice
// first, the system's when they have made none.
func (th *Theme) highlightCSS() string {
	light := styles.Get(th.Highlight.Light)
	dark := styles.Get(th.Highlight.Dark)
	lb, db := bytes.Buffer{}, bytes.Buffer{}
	cssFormatter.WriteCSS(&lb, light)
	cssFormatter.WriteCSS(&db, dark)
	scope := func(css, prefix string) string {
		css = strings.ReplaceAll(css, ".chroma", prefix+".chroma")
		css = strings.ReplaceAll(css, " .bg ", " "+prefix+".bg ")
		return css
	}
	out := strings.Builder{}
	out.WriteString("/* code: " + th.Highlight.Light + " / " + th.Highlight.Dark + " */\n")
	out.WriteString(lb.String())
	// A dark style says nothing about the tokens it leaves plain, so the light
	// style's colours for those (tango's near-black names) showed through on a
	// dark ground. Each token is put back to plain first; the dark style's own
	// rules (two classes) still win over this reset (a class and an element).
	reset := func(prefix string) string {
		return prefix + ".chroma span { color: inherit; background-color: transparent; font-weight: inherit; font-style: inherit; text-decoration: inherit; }\n"
	}
	out.WriteString("@media (prefers-color-scheme: dark) {\n")
	out.WriteString(reset(`:root:not([data-theme="light"]) `))
	out.WriteString(scope(db.String(), `:root:not([data-theme="light"]) `))
	out.WriteString("}\n")
	out.WriteString(reset(`:root[data-theme="dark"] `))
	out.WriteString(scope(db.String(), `:root[data-theme="dark"] `))
	return out.String()
}
