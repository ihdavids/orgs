// Package recipe reads a recipe out of a web page (schema.org Recipe, as
// JSON-LD or microdata), writes it as an org heading in org-chef's layout,
// reads such a heading back, scales its ingredients and merges several into
// a shopping list. Pure: bytes in, values out, so every rule is pinned by a
// test on a saved page rather than on the network.
//
// org-chef's layout, which Emacs users already have files of:
//
//   - Homemade Black Bean Veggie Burgers
//     :PROPERTIES:
//     :source-url: https://www.allrecipes.com/recipe/85452/
//     :servings:   4
//     :prep-time:  15 min
//     :cook-time:  20 min
//     :ready-in:   35 min
//     :END:
//     ** Ingredients
//   - 1 (16 ounce) can black beans, drained and rinsed
//     ** Directions
//     1. If grilling, preheat an outdoor grill for high heat.
package recipe

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Recipe is one recipe, from a page or from an org heading.
type Recipe struct {
	Name        string
	Url         string   `json:",omitempty"`
	Description string   `json:",omitempty"`
	Image       string   `json:",omitempty"` // a url on the page; a link once saved
	Yield       string   `json:",omitempty"` // as the page says it: "4 servings", "1 loaf"
	Servings    float64  `json:",omitempty"` // the number in Yield, 0 when there is none
	PrepTime    string   `json:",omitempty"` // "15 min", "1 h 20 min"
	CookTime    string   `json:",omitempty"`
	TotalTime   string   `json:",omitempty"` // org-chef's ready-in
	Author      string   `json:",omitempty"`
	Category    string   `json:",omitempty"`
	Cuisine     string   `json:",omitempty"`
	Keywords    []string `json:",omitempty"`
	Ingredients []string
	Directions  []Step
}

// Step is one instruction. Section is the HowToSection it sits in ("For the
// sauce"), "" when the page has none.
type Step struct {
	Section string `json:",omitempty"`
	Text    string
}

// ErrNoRecipe is a page with no recipe on it that could be read.
var ErrNoRecipe = fmt.Errorf("no recipe on that page (it has no schema.org Recipe data)")

var (
	ldScriptRe = regexp.MustCompile(`(?is)<script[^>]*type\s*=\s*["']?application/ld\+json["']?[^>]*>(.*?)</script>`)
	tagRe      = regexp.MustCompile(`(?s)<[^>]*>`)
	blockTagRe = regexp.MustCompile(`(?i)<\s*(br|/p|/li|/div|/h[1-6])\s*/?>`)
	spaceRe    = regexp.MustCompile(`[ \t\f\v\x{00a0}]+`)
	numRe      = regexp.MustCompile(`\d+(?:[.,]\d+)?`)
)

// Parse reads the recipe out of a page. pageUrl is where it came from: kept
// as Url when the page does not say, and used to make relative image urls
// absolute.
func Parse(page []byte, pageUrl string) (*Recipe, error) {
	text := string(page)
	var found *Recipe
	for _, m := range ldScriptRe.FindAllStringSubmatch(text, -1) {
		body := strings.TrimSpace(m[1])
		body = strings.TrimPrefix(body, "<!--")
		body = strings.TrimSuffix(body, "-->")
		body = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(body, "]]>"), "//<![CDATA["))
		var v interface{}
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			// A page with a stray control character in its json is common
			// enough to try once more without them.
			if err := json.Unmarshal([]byte(strings.Map(dropControl, body)), &v); err != nil {
				continue
			}
		}
		if obj := findRecipe(v, 0); obj != nil {
			found = fromLD(obj)
			break
		}
	}
	if found == nil {
		found = parseMicrodata(text)
	}
	if found == nil || (found.Name == "" && len(found.Ingredients) == 0) {
		return nil, ErrNoRecipe
	}
	if found.Url == "" {
		found.Url = pageUrl
	}
	if found.Image != "" {
		found.Image = absURL(pageUrl, found.Image)
	}
	if found.Name == "" {
		found.Name = "Untitled recipe"
	}
	return found, nil
}

func dropControl(r rune) rune {
	if r < 0x20 && r != '\n' && r != '\t' && r != '\r' {
		return -1
	}
	if r == '\n' || r == '\r' || r == '\t' {
		return ' '
	}
	return r
}

// findRecipe is the first object typed Recipe, wherever it is: the top, an
// array, an @graph, a mainEntity.
func findRecipe(v interface{}, depth int) map[string]interface{} {
	if depth > 8 {
		return nil
	}
	switch x := v.(type) {
	case []interface{}:
		for _, e := range x {
			if r := findRecipe(e, depth+1); r != nil {
				return r
			}
		}
	case map[string]interface{}:
		if isType(x["@type"], "Recipe") {
			return x
		}
		for _, k := range []string{"@graph", "mainEntity", "mainEntityOfPage", "itemListElement", "item"} {
			if e, ok := x[k]; ok {
				if r := findRecipe(e, depth+1); r != nil {
					return r
				}
			}
		}
	}
	return nil
}

func isType(v interface{}, want string) bool {
	switch t := v.(type) {
	case string:
		t = strings.TrimPrefix(strings.TrimPrefix(t, "http://schema.org/"), "https://schema.org/")
		return strings.EqualFold(t, want)
	case []interface{}:
		for _, e := range t {
			if isType(e, want) {
				return true
			}
		}
	}
	return false
}

func fromLD(o map[string]interface{}) *Recipe {
	r := &Recipe{
		Name:        clean(str(o["name"])),
		Url:         strings.TrimSpace(str(o["url"])),
		Description: clean(str(o["description"])),
		Image:       imageOf(o["image"]),
		Author:      personOf(o["author"]),
		Category:    strings.Join(strs(o["recipeCategory"]), ", "),
		Cuisine:     strings.Join(strs(o["recipeCuisine"]), ", "),
	}
	r.Yield, r.Servings = yieldOf(o["recipeYield"])
	if r.Yield == "" {
		r.Yield, r.Servings = yieldOf(o["yield"])
	}
	r.PrepTime = FormatMinutes(ParseDuration(str(o["prepTime"])))
	r.CookTime = FormatMinutes(ParseDuration(str(o["cookTime"])))
	r.TotalTime = FormatMinutes(ParseDuration(str(o["totalTime"])))
	if r.TotalTime == "" {
		if p, c := ParseDuration(str(o["prepTime"])), ParseDuration(str(o["cookTime"])); p+c > 0 && p > 0 && c > 0 {
			r.TotalTime = FormatMinutes(p + c)
		}
	}
	for _, k := range strs(o["keywords"]) {
		for _, w := range strings.Split(k, ",") {
			if w = clean(w); w != "" {
				r.Keywords = append(r.Keywords, w)
			}
		}
	}
	ings := o["recipeIngredient"]
	if ings == nil {
		ings = o["ingredients"]
	}
	for _, s := range strs(ings) {
		if s = clean(s); s != "" {
			r.Ingredients = append(r.Ingredients, s)
		}
	}
	r.Directions = stepsOf(o["recipeInstructions"], "", 0)
	return r
}

// stepsOf reads recipeInstructions in every shape it comes in: one string
// (lines or html), a list of strings, HowToStep, HowToSection, ItemList.
func stepsOf(v interface{}, section string, depth int) []Step {
	if depth > 6 {
		return nil
	}
	out := []Step{}
	switch x := v.(type) {
	case string:
		for _, l := range splitSteps(x) {
			out = append(out, Step{Section: section, Text: l})
		}
	case []interface{}:
		for _, e := range x {
			out = append(out, stepsOf(e, section, depth+1)...)
		}
	case map[string]interface{}:
		if isType(x["@type"], "HowToSection") || (x["itemListElement"] != nil && !isType(x["@type"], "HowToStep")) {
			name := clean(str(x["name"]))
			if name == "" {
				name = section
			}
			return stepsOf(x["itemListElement"], name, depth+1)
		}
		t := clean(str(x["text"]))
		if t == "" {
			t = clean(str(x["name"]))
		}
		if t == "" {
			t = clean(str(x["description"]))
		}
		if t != "" {
			out = append(out, Step{Section: section, Text: t})
		}
	}
	return out
}

var stepNumRe = regexp.MustCompile(`^(?:step\s*)?\d{1,2}[.):]\s+`)

// splitSteps cuts one block of instructions into steps: at paragraphs and
// list items when it is html, else at line breaks.
func splitSteps(s string) []string {
	s = blockTagRe.ReplaceAllString(s, "\n")
	s = html.UnescapeString(tagRe.ReplaceAllString(s, ""))
	out := []string{}
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		l = strings.TrimSpace(spaceRe.ReplaceAllString(l, " "))
		l = stepNumRe.ReplaceAllString(l, "")
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func str(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case []interface{}:
		if len(x) > 0 {
			return str(x[0])
		}
	case map[string]interface{}:
		if s, ok := x["@value"]; ok {
			return str(s)
		}
	}
	return ""
}

func strs(v interface{}) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case float64:
		return []string{str(x)}
	case []interface{}:
		out := []string{}
		for _, e := range x {
			if s := str(e); s != "" {
				out = append(out, s)
			} else if m, ok := e.(map[string]interface{}); ok {
				if s := str(m["name"]); s != "" {
					out = append(out, s)
				} else if s := str(m["text"]); s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	case map[string]interface{}:
		if s := str(x["name"]); s != "" {
			return []string{s}
		}
	}
	return nil
}

// imageOf is the first picture: a url, a list of them, an ImageObject.
func imageOf(v interface{}) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case []interface{}:
		for _, e := range x {
			if s := imageOf(e); s != "" {
				return s
			}
		}
	case map[string]interface{}:
		for _, k := range []string{"url", "contentUrl", "@id"} {
			if s := str(x[k]); s != "" && (k != "@id" || strings.HasPrefix(s, "http")) {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func personOf(v interface{}) string {
	switch x := v.(type) {
	case string:
		return clean(x)
	case []interface{}:
		names := []string{}
		for _, e := range x {
			if s := personOf(e); s != "" {
				names = append(names, s)
			}
		}
		return strings.Join(names, ", ")
	case map[string]interface{}:
		return clean(str(x["name"]))
	}
	return ""
}

// yieldOf reads recipeYield: "4", 4, "4 servings", ["4", "4 servings"],
// "Makes 12 cookies". The wordiest is kept as text, the first number as
// servings.
func yieldOf(v interface{}) (string, float64) {
	all := strs(v)
	text := ""
	for _, s := range all {
		s = clean(s)
		if len(s) > len(text) {
			text = s
		}
	}
	return text, firstNumber(text)
}

func firstNumber(s string) float64 {
	m := numRe.FindString(s)
	if m == "" {
		return 0
	}
	f, _ := strconv.ParseFloat(strings.ReplaceAll(m, ",", "."), 64)
	return f
}

// clean is text out of html: tags dropped, entities read (twice, as pages
// double-encode), space squeezed.
func clean(s string) string {
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(html.UnescapeString(s))
	s = tagRe.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " ")
	s = strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
	return punctSpaceRe.ReplaceAllString(s, "$1")
}

// "in <b>40 minutes</b>." loses its tags to spaces; the one left before the
// full stop is not the writer's.
var punctSpaceRe = regexp.MustCompile(`\s+([.,;:!?])(\s|$)`)

func absURL(base, ref string) string {
	ref = strings.TrimSpace(ref)
	switch {
	case ref == "" || strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "data:"):
		return ref
	case strings.HasPrefix(ref, "//"):
		if strings.HasPrefix(base, "http://") {
			return "http:" + ref
		}
		return "https:" + ref
	}
	i := strings.Index(base, "://")
	if i < 0 {
		return ref
	}
	rest := base[i+3:]
	host := rest
	if j := strings.IndexByte(rest, '/'); j >= 0 {
		host = rest[:j]
	}
	root := base[:i+3] + host
	if strings.HasPrefix(ref, "/") {
		return root + ref
	}
	dir := base
	if j := strings.LastIndexByte(base, '/'); j > i+2 {
		dir = base[:j]
	}
	return dir + "/" + ref
}

// ---------------------------------------------------------------------------
// Microdata: itemprop attributes inside an itemtype schema.org/Recipe. Older
// WordPress plugins and some big sites still only have this.
// ---------------------------------------------------------------------------

var (
	mdScopeRe = regexp.MustCompile(`(?i)itemtype\s*=\s*["']https?://schema\.org/Recipe["']`)
	mdPropRe  = regexp.MustCompile(`(?is)<([a-z0-9]+)([^>]*?\bitemprop\s*=\s*["']([^"']+)["'][^>]*)>`)
	attrRe    = func(name string) *regexp.Regexp {
		return regexp.MustCompile(`(?is)\b` + name + `\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	}
	contentAttr  = attrRe("content")
	srcAttr      = attrRe("src")
	hrefAttr     = attrRe("href")
	datetimeAttr = attrRe("datetime")
)

func parseMicrodata(text string) *Recipe {
	loc := mdScopeRe.FindStringIndex(text)
	if loc == nil {
		return nil
	}
	scope := text[loc[0]:]
	r := &Recipe{}
	section := ""
	for _, m := range mdPropRe.FindAllStringSubmatchIndex(scope, -1) {
		tag := strings.ToLower(scope[m[2]:m[3]])
		attrs := scope[m[4]:m[5]]
		props := strings.Fields(scope[m[6]:m[7]])
		value := func() string {
			for _, re := range []*regexp.Regexp{contentAttr, datetimeAttr} {
				if a := re.FindStringSubmatch(attrs); a != nil {
					return a[1] + a[2]
				}
			}
			if tag == "img" || tag == "meta" || tag == "link" {
				for _, re := range []*regexp.Regexp{srcAttr, hrefAttr} {
					if a := re.FindStringSubmatch(attrs); a != nil {
						return a[1] + a[2]
					}
				}
				return ""
			}
			return innerHTML(scope[m[1]:], tag)
		}
		for _, p := range props {
			switch p {
			case "name":
				if r.Name == "" {
					r.Name = clean(value())
				} else if strings.Contains(attrs, "HowToSection") {
					section = clean(value())
				}
			case "description":
				if r.Description == "" {
					r.Description = clean(value())
				}
			case "image":
				if r.Image == "" {
					r.Image = strings.TrimSpace(value())
				}
			case "recipeYield":
				if r.Yield == "" {
					r.Yield = clean(value())
					r.Servings = firstNumber(r.Yield)
				}
			case "prepTime":
				r.PrepTime = FormatMinutes(ParseDuration(value()))
			case "cookTime":
				r.CookTime = FormatMinutes(ParseDuration(value()))
			case "totalTime":
				r.TotalTime = FormatMinutes(ParseDuration(value()))
			case "recipeIngredient", "ingredients":
				if s := clean(value()); s != "" {
					r.Ingredients = append(r.Ingredients, s)
				}
			case "recipeInstructions":
				raw := innerHTML(scope[m[1]:], tag)
				for _, l := range splitSteps(raw) {
					r.Directions = append(r.Directions, Step{Section: section, Text: l})
				}
			case "text":
				if s := clean(value()); s != "" {
					r.Directions = append(r.Directions, Step{Section: section, Text: s})
				}
			}
		}
	}
	return r
}

// innerHTML is what is inside an element up to its closing tag, counting
// elements of the same name opened inside it.
func innerHTML(rest, tag string) string {
	open := regexp.MustCompile(`(?i)<` + tag + `\b`)
	closeRe := regexp.MustCompile(`(?i)</` + tag + `\s*>`)
	depth, pos := 1, 0
	for depth > 0 {
		c := closeRe.FindStringIndex(rest[pos:])
		if c == nil {
			if len(rest) > 4000 {
				return rest[:4000]
			}
			return rest
		}
		o := open.FindStringIndex(rest[pos:])
		if o != nil && o[0] < c[0] {
			depth++
			pos += o[1]
			continue
		}
		depth--
		if depth == 0 {
			return rest[:pos+c[0]]
		}
		pos += c[1]
	}
	return ""
}
