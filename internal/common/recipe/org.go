package recipe

import (
	"regexp"
	"strconv"
	"strings"
)

// Options say how a recipe is written as org.
type Options struct {
	Level      int    // the heading's level (1 when 0)
	Record     bool   // add :RECORD: recipe, so the records browser lists it
	Checkboxes bool   // ingredients as - [ ] (org-chef writes plain - items)
	ImageLink  string // [[file:images/recipes/x.jpg]]; "" leaves the page's url out too
	ImageUrl   bool   // with no ImageLink, keep the page's own picture url in :IMAGE:
}

// The property names org-chef writes, in its order.
var chefProps = []string{"source-url", "servings", "prep-time", "cook-time", "ready-in"}

// ToOrg is a recipe as org lines, in org-chef's layout: the heading, its
// properties, an Ingredients child with a plain list and a Directions child
// with a numbered one. A page with steps in sections gets a heading per
// section under Directions.
func ToOrg(r *Recipe, o Options) []string {
	lvl := o.Level
	if lvl < 1 {
		lvl = 1
	}
	stars := strings.Repeat("*", lvl)
	name := strings.TrimSpace(strings.ReplaceAll(r.Name, "\n", " "))
	if name == "" {
		name = "Untitled recipe"
	}
	out := []string{stars + " " + name}

	type kv struct{ k, v string }
	props := []kv{
		{"source-url", r.Url},
		{"servings", servingsText(r)},
		{"prep-time", r.PrepTime},
		{"cook-time", r.CookTime},
		{"ready-in", r.TotalTime},
	}
	if r.Yield != "" && r.Yield != servingsText(r) {
		props = append(props, kv{"yield", r.Yield})
	}
	if r.Author != "" {
		props = append(props, kv{"author", r.Author})
	}
	if r.Category != "" {
		props = append(props, kv{"category", r.Category})
	}
	if r.Cuisine != "" {
		props = append(props, kv{"cuisine", r.Cuisine})
	}
	if o.ImageLink != "" {
		props = append(props, kv{"IMAGE", o.ImageLink})
	} else if o.ImageUrl && r.Image != "" && !strings.HasPrefix(r.Image, "data:") {
		props = append(props, kv{"IMAGE", r.Image})
	}
	if o.Record {
		props = append(props, kv{"RECORD", "recipe"})
	}
	width := 0
	for _, p := range props {
		if p.v != "" || p.k == "source-url" {
			if n := len(p.k) + 2; n > width {
				width = n
			}
		}
	}
	out = append(out, ":PROPERTIES:")
	for _, p := range props {
		if p.v == "" && p.k != "source-url" {
			continue
		}
		k := ":" + p.k + ":"
		line := k + strings.Repeat(" ", width-len(k))
		if p.v != "" {
			line += " " + oneLine(p.v)
		}
		out = append(out, line)
	}
	out = append(out, ":END:")
	if d := strings.TrimSpace(r.Description); d != "" {
		out = append(out, oneLine(d))
	}

	out = append(out, "", stars+"* Ingredients")
	box := ""
	if o.Checkboxes {
		box = "[ ] "
	}
	for _, i := range r.Ingredients {
		if i = oneLine(i); i != "" {
			out = append(out, "- "+box+i)
		}
	}

	out = append(out, "", stars+"* Directions")
	sections := 0
	last := "\x00"
	for _, s := range r.Directions {
		if s.Section != last {
			sections++
			last = s.Section
		}
	}
	last = "\x00"
	n := 0
	for _, s := range r.Directions {
		if sections > 1 && s.Section != last {
			title := s.Section
			if title == "" {
				title = "Steps"
			}
			out = append(out, stars+"** "+oneLine(title))
			n = 0
		}
		last = s.Section
		if t := oneLine(s.Text); t != "" {
			n++
			out = append(out, strconv.Itoa(n)+". "+t)
		}
	}
	return out
}

func servingsText(r *Recipe) string {
	if r.Servings > 0 {
		return strconv.FormatFloat(r.Servings, 'f', -1, 64)
	}
	return r.Yield
}

func oneLine(s string) string {
	return strings.TrimSpace(spaceRe.ReplaceAllString(strings.ReplaceAll(strings.ReplaceAll(s, "\r", ""), "\n", " "), " "))
}

var (
	orgHeadRe     = regexp.MustCompile(`^(\*+)\s+(.*?)\s*$`)
	orgTagsRe     = regexp.MustCompile(`\s+:[\w@#%:]+:\s*$`)
	orgItemRe     = regexp.MustCompile(`^(\s*)(?:[-+]|\s+\*)\s+(?:\[([ xX-])\]\s+)?(.*)$`)
	orgNumItemRe  = regexp.MustCompile(`^(\s*)\d+[.)]\s+(?:\[([ xX-])\]\s+)?(.*)$`)
	orgDrawerRe   = regexp.MustCompile(`^\s*:[A-Za-z_-]+:\s*$`)
	orgEndRe      = regexp.MustCompile(`(?i)^\s*:END:\s*$`)
	orgPlanRe     = regexp.MustCompile(`^\s*(SCHEDULED|DEADLINE|CLOSED):`)
	ingredientsRe = regexp.MustCompile(`(?i)^ingredients?\b`)
	directionsRe  = regexp.MustCompile(`(?i)^(directions?|instructions?|method|steps|preparation|how to make it)\b`)
	imageLinkRe   = regexp.MustCompile(`(?i)\[\[(?:file:)?([^\]]+\.(?:png|jpe?g|gif|webp|avif|svg))\]`)
)

// FromOrg reads a recipe heading back: lines are its subtree, headline
// first; props its own properties (any case). Ingredients are every list item
// under a child called Ingredients, directions every item under Directions
// (or Instructions, Method, Steps), a heading under it naming a section. So a
// file org-chef wrote, or one typed by hand from its manual capture
// template, reads the same as one orgs wrote.
func FromOrg(name string, props map[string]string, lines []string) *Recipe {
	p := map[string]string{}
	for k, v := range props {
		p[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	r := &Recipe{
		Name:      name,
		Url:       p["source-url"],
		PrepTime:  p["prep-time"],
		CookTime:  p["cook-time"],
		TotalTime: p["ready-in"],
		Author:    p["author"],
		Category:  p["category"],
		Cuisine:   p["cuisine"],
		Image:     p["image"],
		Yield:     p["yield"],
	}
	// org-chef writes "15  m"; shown the way orgs writes times.
	for _, t := range []*string{&r.PrepTime, &r.CookTime, &r.TotalTime} {
		if m := ParseDuration(*t); m > 0 {
			*t = FormatMinutes(m)
		}
	}
	if s := p["servings"]; s != "" {
		r.Servings = firstNumber(s)
		if r.Yield == "" {
			r.Yield = s
		}
	}
	if len(lines) == 0 {
		return r
	}
	top := 1
	if m := orgHeadRe.FindStringSubmatch(lines[0]); m != nil {
		top = len(m[1])
	}
	// what: 0 the heading's own body, 1 ingredients, 2 directions, 3 other.
	what, whatLvl, section := 0, 0, ""
	inDrawer := false
	desc := []string{}
	var lastItem *string
	for _, l := range lines[1:] {
		if m := orgHeadRe.FindStringSubmatch(l); m != nil {
			lvl := len(m[1])
			title := strings.TrimSpace(orgTagsRe.ReplaceAllString(m[2], ""))
			lastItem = nil
			switch {
			case lvl <= top:
				return r
			case (what == 1 || what == 2) && lvl > whatLvl:
				if what == 2 {
					section = title
				}
			case ingredientsRe.MatchString(title):
				what, whatLvl = 1, lvl
			case directionsRe.MatchString(title):
				what, whatLvl, section = 2, lvl, ""
			default:
				what, whatLvl = 3, lvl
			}
			continue
		}
		if inDrawer {
			if orgEndRe.MatchString(l) {
				inDrawer = false
			}
			continue
		}
		if orgDrawerRe.MatchString(l) && !orgEndRe.MatchString(l) {
			inDrawer = true
			continue
		}
		if orgPlanRe.MatchString(l) {
			continue
		}
		switch what {
		case 0:
			t := strings.TrimSpace(l)
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			if m := imageLinkRe.FindStringSubmatch(t); m != nil && strings.TrimSpace(imageLinkRe.ReplaceAllString(t, "")) == "" {
				if r.Image == "" {
					r.Image = "[[file:" + strings.TrimPrefix(m[1], "file:") + "]]"
				}
				continue
			}
			desc = append(desc, t)
		case 1, 2:
			if m := orgNumItemRe.FindStringSubmatch(l); m != nil {
				lastItem = add(r, what, section, m[3])
				continue
			}
			if m := orgItemRe.FindStringSubmatch(l); m != nil {
				lastItem = add(r, what, section, m[3])
				continue
			}
			t := strings.TrimSpace(l)
			if t == "" {
				continue
			}
			// A line folded under an item belongs to it.
			if lastItem != nil && (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")) {
				*lastItem = strings.TrimSpace(*lastItem + " " + t)
				continue
			}
			lastItem = nil
		}
	}
	r.Description = strings.Join(desc, " ")
	return r
}

func add(r *Recipe, what int, section, text string) *string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if what == 1 {
		r.Ingredients = append(r.Ingredients, text)
		return &r.Ingredients[len(r.Ingredients)-1]
	}
	r.Directions = append(r.Directions, Step{Section: section, Text: text})
	return &r.Directions[len(r.Directions)-1].Text
}
