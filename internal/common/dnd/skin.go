//lint:file-ignore ST1006 allow the use of self
package dnd

/* SDOC: DnD
* Sheet skins

  The html character sheet can wear a skin: a whole different look - a hand
  inked page, a druid's grove, a pirate's ledger - over the same sheet, with
  every button and roll working as before. The classic sheet is what you get
  with no skin, and it never changes.

  Pick one from the sheet itself: the round menu button at the bottom right
  has a *Skin* button (or type =/= and "skin"). The gallery that opens puts
  each skin on the sheet as you click it; *Keep* writes it into the
  character's =DND_SHEET_SKIN= property, so the sheet opens in it from then
  on, and *Cancel* (or Esc) puts back what was there.

  A skin is one css file in a =dnd_skins/= folder under any template folder,
  named for its id (=wildwood.css=). Your own go in your =templatePath='s
  =dnd_skins/=, and one with the same name as a built-in skin replaces it. The
  file starts with a comment saying what it is:

  #+BEGIN_SRC css
  /* skin: Wildwood
     about: Bark, vines and a green watercolour wash, for druids and rangers.
     swatch: #2f4a2b #c9b98a #7a5b2e #e9e2c8
     fonts: Kalam:wght@400;700 | Uncial Antiqua
     print: yes
  #+END_SRC

  (and the comment's closing star and slash, which a Go comment cannot quote).

  - =skin= is its name in the gallery, =about= one line under it, =swatch=
    up to five colours drawn on its card.
  - =fonts= are Google Fonts families (the part after =family== in their
    css2 address), loaded only when the skin is worn.
  - =print: yes= keeps the skin on paper; without it a printed sheet is the
    classic one.

  Every rule must start with =html[data-skin="wildwood"]= (keyframes and
  other names start with the id), so a skin can never touch the classic sheet
  or another skin; a file that breaks this is left out with a warning. A
  =wildwood.svg= beside it is put into the page, hidden, for svg filters and
  patterns the css refers to (=url(#wildwood-rough)=).
EDOC */

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Skin is one look the html sheet can wear.
type Skin struct {
	Id     string   `json:"id"`
	Name   string   `json:"name"`
	About  string   `json:"about"`
	Swatch []string `json:"swatch"`
	// FontsUrl is the Google Fonts stylesheet with the skin's fonts, or "".
	FontsUrl string `json:"fontsUrl"`
	// Face is the first of those fonts, which the gallery writes the
	// skin's name in.
	Face  string `json:"face"`
	Print bool   `json:"print"`
	// Css is what goes into the page: the file's rules, wrapped in
	// @media screen unless the skin prints.
	Css string `json:"-"`
	// Svg is the skin's hidden svg definitions, if it has any.
	Svg string `json:"-"`
}

var skinIdRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)

// ValidSkinId is whether id may name a skin: the empty id is the classic
// sheet.
func ValidSkinId(id string) bool {
	return id == "" || skinIdRe.MatchString(id)
}

var (
	skinHeadRe  = regexp.MustCompile(`(?s)^\s*/\*(.*?)\*/`)
	skinFieldRe = regexp.MustCompile(`(?m)^\s*(skin|about|swatch|fonts|print)\s*:\s*(.*?)\s*$`)
	skinColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$`)
)

// ParseSkin reads a skin file. id is the file's name without .css.
func ParseSkin(id, css, svg string) (Skin, error) {
	if !skinIdRe.MatchString(id) {
		return Skin{}, fmt.Errorf("%q is not a skin name: lower case letters, digits and dashes", id)
	}
	s := Skin{Id: id, Name: id, Svg: strings.TrimSpace(svg)}
	if m := skinHeadRe.FindStringSubmatch(css); m != nil {
		for _, f := range skinFieldRe.FindAllStringSubmatch(m[1], -1) {
			switch f[1] {
			case "skin":
				s.Name = f[2]
			case "about":
				s.About = f[2]
			case "swatch":
				for _, c := range strings.Fields(f[2]) {
					if skinColorRe.MatchString(c) && len(s.Swatch) < 5 {
						s.Swatch = append(s.Swatch, c)
					}
				}
			case "fonts":
				s.FontsUrl = skinFontsUrl(f[2])
				first, _, _ := strings.Cut(f[2], "|")
				first, _, _ = strings.Cut(first, ":")
				s.Face = strings.TrimSpace(first)
			case "print":
				s.Print = isYes(f[2])
			}
		}
	}
	if err := CheckSkinScope(id, css); err != nil {
		return Skin{}, err
	}
	// A </style> in the file would end the page's style element early.
	if strings.Contains(strings.ToLower(css), "</style") {
		return Skin{}, fmt.Errorf("a skin may not contain </style>")
	}
	if strings.Contains(strings.ToLower(s.Svg), "<script") {
		return Skin{}, fmt.Errorf("a skin's svg may not contain a script")
	}
	s.Css = css
	if !s.Print {
		s.Css = "@media screen {\n" + css + "\n}"
	}
	return s, nil
}

// skinFontsUrl is the Google Fonts css2 address for families separated by
// "|", each written as its css2 family parameter ("Kalam:wght@400;700").
func skinFontsUrl(list string) string {
	q := []string{}
	for _, f := range strings.Split(list, "|") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		name, axes, _ := strings.Cut(f, ":")
		v := strings.ReplaceAll(url.QueryEscape(strings.TrimSpace(name)), "%20", "+")
		if axes != "" {
			v += ":" + strings.ReplaceAll(url.PathEscape(strings.TrimSpace(axes)), "%3B", ";")
		}
		q = append(q, "family="+v)
	}
	if len(q) == 0 {
		return ""
	}
	return "https://fonts.googleapis.com/css2?" + strings.Join(q, "&") + "&display=swap"
}

// CheckSkinScope makes sure every rule in a skin's css is for that skin
// alone: each selector starts with html[data-skin="id"], and each
// @keyframes, @counter-style or @property name starts with the id. It is a
// brace walker, not a css parser: comments and strings are skipped, at-rules
// that hold rules (@media, @supports, @container, @layer) are walked into.
func CheckSkinScope(id, css string) error {
	scope := `html[data-skin="` + id + `"]`
	src := stripCssComments(css)
	var walk func(s string, depth int) error
	walk = func(s string, depth int) error {
		i := 0
		for i < len(s) {
			open := strings.IndexByte(s[i:], '{')
			if open < 0 {
				if strings.TrimSpace(s[i:]) != "" && !strings.HasPrefix(strings.TrimSpace(s[i:]), "@") {
					return fmt.Errorf("stray text in the css: %.40q", strings.TrimSpace(s[i:]))
				}
				return nil
			}
			prelude := strings.TrimSpace(s[i : i+open])
			// Statements before the block (@charset, @import) end with ";".
			if k := strings.LastIndexByte(prelude, ';'); k >= 0 {
				head := strings.TrimSpace(prelude[:k])
				if strings.HasPrefix(head, "@import") || strings.Contains(head, "@import") {
					return fmt.Errorf("a skin may not @import (list its fonts in the header)")
				}
				prelude = strings.TrimSpace(prelude[k+1:])
			}
			close := matchBrace(s, i+open)
			if close < 0 {
				return fmt.Errorf("unbalanced braces after %.40q", prelude)
			}
			body := s[i+open+1 : close]
			switch {
			case strings.HasPrefix(prelude, "@media"), strings.HasPrefix(prelude, "@supports"),
				strings.HasPrefix(prelude, "@container"), strings.HasPrefix(prelude, "@layer"):
				if err := walk(body, depth+1); err != nil {
					return err
				}
			case strings.HasPrefix(prelude, "@keyframes"), strings.HasPrefix(prelude, "@-webkit-keyframes"),
				strings.HasPrefix(prelude, "@counter-style"), strings.HasPrefix(prelude, "@property"):
				name := strings.TrimSpace(prelude[strings.IndexByte(prelude, ' ')+1:])
				name = strings.TrimPrefix(name, "--")
				if !strings.HasPrefix(name, id) {
					return fmt.Errorf("%s: the name must start with %q", prelude, id)
				}
			case strings.HasPrefix(prelude, "@font-face"):
				// a font of its own is fine: it only exists by its family name
			case strings.HasPrefix(prelude, "@"):
				return fmt.Errorf("%s is not allowed in a skin", strings.Fields(prelude)[0])
			default:
				for _, sel := range splitSelectors(prelude) {
					if !strings.HasPrefix(sel, scope) {
						return fmt.Errorf("selector %q does not start with %s", sel, scope)
					}
				}
			}
			i = close + 1
		}
		return nil
	}
	return walk(src, 0)
}

func stripCssComments(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '/' && i+1 < len(s) && s[i+1] == '*' {
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				break
			}
			b.WriteByte(' ')
			i += end + 3
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// matchBrace finds the } that closes the { at open, skipping strings.
func matchBrace(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '"', '\'':
			q := s[i]
			for i++; i < len(s) && s[i] != q; i++ {
				if s[i] == '\\' {
					i++
				}
			}
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitSelectors splits a selector list on its top level commas (not those
// inside :is(), :not() or an attribute's quotes).
func splitSelectors(list string) []string {
	out := []string{}
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(list); i++ {
		c := list[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			depth--
		case c == ',' && depth == 0:
			out = append(out, strings.TrimSpace(list[start:i]))
			start = i + 1
		}
	}
	return append(out, strings.TrimSpace(list[start:]))
}

// SkinRequest changes which skin a character's sheet wears.
type SkinRequest struct {
	Filename string `json:"filename"`
	Id       string `json:"id"`
	// Skin is the skin's id, or "" for the classic sheet.
	Skin string `json:"skin"`
}
