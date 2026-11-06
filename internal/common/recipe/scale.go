package recipe

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Scaling and shopping. Only the leading quantity of an ingredient is a
// quantity: "1 (16 ounce) can black beans" doubles to "2 (16 ounce) cans",
// not to a 32 ounce can. Done on the server only, so there is no twin.

const vulgar = "½⅓⅔¼¾⅛⅜⅝⅞⅕⅖⅗⅘⅙⅚"

var vulgarValue = map[rune]float64{
	'½': 1.0 / 2, '⅓': 1.0 / 3, '⅔': 2.0 / 3, '¼': 1.0 / 4, '¾': 3.0 / 4,
	'⅛': 1.0 / 8, '⅜': 3.0 / 8, '⅝': 5.0 / 8, '⅞': 7.0 / 8,
	'⅕': 1.0 / 5, '⅖': 2.0 / 5, '⅗': 3.0 / 5, '⅘': 4.0 / 5, '⅙': 1.0 / 6, '⅚': 5.0 / 6,
}

const numPat = `(?:\d+\s+\d+\s*/\s*\d+|\d+\s*[` + vulgar + `]|\d+\s*/\s*\d+|\d+(?:[.,]\d+)?|[` + vulgar + `])`

var (
	qtyRe   = regexp.MustCompile(`^(\s*(?:about\s+|approx\.?\s+|~\s*)?)(` + numPat + `)(?:(\s*(?:-|–|—|to|or)\s*)(` + numPat + `))?`)
	mixedRe = regexp.MustCompile(`^(\d+)\s+(\d+)\s*/\s*(\d+)$`)
	fracRe  = regexp.MustCompile(`^(\d+)\s*/\s*(\d+)$`)
)

// Quantity is the leading amount of an ingredient line.
type Quantity struct {
	Lo, Hi float64 // Hi is 0 unless it is a range
	style  string  // "decimal", "fraction" or "unicode": how it is written back
}

func parseNumber(s string) (float64, string, bool) {
	s = strings.TrimSpace(s)
	if m := mixedRe.FindStringSubmatch(s); m != nil {
		w, _ := strconv.Atoi(m[1])
		n, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		if d == 0 {
			return 0, "", false
		}
		return float64(w) + float64(n)/float64(d), "fraction", true
	}
	if m := fracRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		d, _ := strconv.Atoi(m[2])
		if d == 0 {
			return 0, "", false
		}
		return float64(n) / float64(d), "fraction", true
	}
	rs := []rune(s)
	if len(rs) > 0 {
		if v, ok := vulgarValue[rs[len(rs)-1]]; ok {
			whole := strings.TrimSpace(string(rs[:len(rs)-1]))
			w := 0.0
			if whole != "" {
				w, _ = strconv.ParseFloat(whole, 64)
			}
			return w + v, "unicode", true
		}
	}
	if strings.ContainsAny(s, ".,") {
		v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
		return v, "decimal", err == nil
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, "whole", err == nil
}

// splitQuantity cuts a line into its lead ("about "), quantity and the rest.
func splitQuantity(line string) (lead string, q Quantity, rest string, ok bool) {
	m := qtyRe.FindStringSubmatchIndex(line)
	if m == nil {
		return "", Quantity{}, line, false
	}
	lead = line[m[2]:m[3]]
	lo, style, ok1 := parseNumber(line[m[4]:m[5]])
	if !ok1 {
		return "", Quantity{}, line, false
	}
	q = Quantity{Lo: lo, style: style}
	end := m[5]
	if m[8] >= 0 {
		if hi, st2, ok2 := parseNumber(line[m[8]:m[9]]); ok2 && hi > lo {
			q.Hi = hi
			if style == "whole" {
				q.style = st2
			}
			end = m[9]
		}
	}
	// "1.5x" or "2-3" glued to a letter is still a quantity ("400g"); a
	// number glued to more digits is not reached here.
	return lead, q, line[end:], true
}

var fractions = []struct {
	v       float64
	ascii   string
	unicode string
}{
	{0, "", ""}, {1.0 / 8, "1/8", "⅛"}, {1.0 / 4, "1/4", "¼"}, {1.0 / 3, "1/3", "⅓"},
	{3.0 / 8, "3/8", "⅜"}, {1.0 / 2, "1/2", "½"}, {5.0 / 8, "5/8", "⅝"}, {2.0 / 3, "2/3", "⅔"},
	{3.0 / 4, "3/4", "¾"}, {7.0 / 8, "7/8", "⅞"}, {1, "", ""},
}

// formatNumber writes a scaled amount in the style it was written in. Big
// amounts are rounded the way a cook would: 133 g, not 133 1/3 g.
func formatNumber(v float64, style string) string {
	if v <= 0 {
		return "0"
	}
	if v >= 20 {
		return strconv.Itoa(int(math.Round(v)))
	}
	if v >= 10 {
		v = math.Round(v*2) / 2
	}
	if style == "decimal" {
		s := strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
		return s
	}
	whole := math.Floor(v)
	frac := v - whole
	best := 0
	for i, f := range fractions {
		if math.Abs(f.v-frac) < math.Abs(fractions[best].v-frac) {
			best = i
		}
	}
	if fractions[best].v == 1 {
		whole++
		best = 0
	}
	f := fractions[best]
	switch {
	case whole == 0 && f.v == 0:
		// Too small for an eighth: say it in decimals rather than as 0.
		return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
	case f.v == 0:
		return strconv.Itoa(int(whole))
	case style == "unicode":
		if whole == 0 {
			return f.unicode
		}
		return strconv.Itoa(int(whole)) + f.unicode
	case whole == 0:
		return f.ascii
	}
	return strconv.Itoa(int(whole)) + " " + f.ascii
}

// top is the larger end of the amount, what decides "cup" or "cups".
func (q Quantity) top() float64 {
	if q.Hi > q.Lo {
		return q.Hi
	}
	return q.Lo
}

func (q Quantity) format(factor float64) string {
	s := formatNumber(q.Lo*factor, q.style)
	if q.Hi > 0 {
		s += "-" + formatNumber(q.Hi*factor, q.style)
	}
	return s
}

// countable words that change with the amount: "1 cup", "2 cups".
var plurals = map[string]string{
	"cup": "cups", "tablespoon": "tablespoons", "teaspoon": "teaspoons", "clove": "cloves",
	"can": "cans", "pound": "pounds", "ounce": "ounces", "slice": "slices", "pinch": "pinches",
	"stick": "sticks", "sprig": "sprigs", "bunch": "bunches", "head": "heads", "piece": "pieces",
	"package": "packages", "packet": "packets", "jar": "jars", "bottle": "bottles", "egg": "eggs",
	"onion": "onions", "carrot": "carrots", "potato": "potatoes", "tomato": "tomatoes",
	"lemon": "lemons", "lime": "limes", "dash": "dashes", "handful": "handfuls", "leaf": "leaves",
	"stalk": "stalks", "fillet": "fillets", "breast": "breasts", "thigh": "thighs", "sheet": "sheets",
	"litre": "litres", "liter": "liters", "gram": "grams", "kilogram": "kilograms", "tin": "tins",
	"bag": "bags", "box": "boxes", "loaf": "loaves", "pepper": "peppers", "apple": "apples",
	"banana": "bananas", "shallot": "shallots", "cucumber": "cucumbers", "zucchini": "zucchinis",
}
var singulars = func() map[string]string {
	m := map[string]string{}
	for k, v := range plurals {
		m[v] = k
	}
	return m
}()

var firstWordRe = regexp.MustCompile(`^(\s*(?:\([^)]*\)\s*)?)([A-Za-z]+)`)

// agree makes the word after an amount agree with it.
func agree(rest string, one bool) string {
	m := firstWordRe.FindStringSubmatchIndex(rest)
	if m == nil {
		return rest
	}
	word := rest[m[4]:m[5]]
	low := strings.ToLower(word)
	var to string
	if one {
		to = singulars[low]
	} else {
		to = plurals[low]
	}
	if to == "" {
		return rest
	}
	if word[0] >= 'A' && word[0] <= 'Z' {
		to = strings.ToUpper(to[:1]) + to[1:]
	}
	return rest[:m[4]] + to + rest[m[5]:]
}

// ScaleIngredient multiplies an ingredient's leading amount, keeping its
// spelling (1/2, ½, 0.5, 1-2) and its units. A line with no amount ("salt to
// taste") comes back as it was.
func ScaleIngredient(line string, factor float64) string {
	if factor <= 0 || math.Abs(factor-1) < 1e-9 {
		return line
	}
	lead, q, rest, ok := splitQuantity(line)
	if !ok {
		return line
	}
	out := q.format(factor)
	if was, is := q.top() > 1, q.top()*factor > 1; was != is {
		rest = agree(rest, !is)
	}
	return lead + out + rest
}

// Scale is a copy of a recipe made for servings people instead of its own.
// A recipe that does not say how many it serves cannot be scaled to a
// number of people; factor then scales it by itself (2 = double).
func Scale(r *Recipe, servings, factor float64) (*Recipe, float64) {
	out := *r
	if servings > 0 && r.Servings > 0 {
		factor = servings / r.Servings
	}
	if factor <= 0 {
		factor = 1
	}
	out.Ingredients = make([]string, len(r.Ingredients))
	for i, l := range r.Ingredients {
		out.Ingredients[i] = ScaleIngredient(l, factor)
	}
	if r.Servings > 0 {
		out.Servings = r.Servings * factor
		if math.Abs(factor-1) > 1e-9 {
			out.Yield = formatNumber(out.Servings, "fraction")
		}
	}
	return &out, factor
}

// ---------------------------------------------------------------------------
// Shopping
// ---------------------------------------------------------------------------

var units = map[string]string{
	"tbsp": "tbsp", "tbs": "tbsp", "tbl": "tbsp", "tablespoon": "tbsp", "tablespoons": "tbsp", "T": "tbsp",
	"tsp": "tsp", "teaspoon": "tsp", "teaspoons": "tsp", "t": "tsp",
	"cup": "cup", "cups": "cup", "c": "cup",
	"g": "g", "gram": "g", "grams": "g", "gr": "g",
	"kg": "kg", "kilogram": "kg", "kilograms": "kg",
	"ml": "ml", "millilitre": "ml", "milliliter": "ml", "millilitres": "ml", "milliliters": "ml",
	"l": "l", "litre": "l", "liter": "l", "litres": "l", "liters": "l",
	"oz": "oz", "ounce": "oz", "ounces": "oz",
	"lb": "lb", "lbs": "lb", "pound": "lb", "pounds": "lb",
	"clove": "clove", "cloves": "clove", "can": "can", "cans": "can", "tin": "can", "tins": "can",
	"pinch": "pinch", "pinches": "pinch", "slice": "slice", "slices": "slice",
	"stick": "stick", "sticks": "stick", "bunch": "bunch", "bunches": "bunch",
	"sprig": "sprig", "sprigs": "sprig", "handful": "handful", "handfuls": "handful",
	"package": "package", "packages": "package", "packet": "package", "packets": "package",
}

var unitWordRe = regexp.MustCompile(`^\s*([A-Za-z]+)\.?\b`)

// Ingredient is a line taken apart for the shopping list.
type Ingredient struct {
	Line   string
	Qty    Quantity
	Has    bool   // it has an amount
	Unit   string // the canonical unit, "" for a count ("3 eggs")
	Rest   string // what follows the amount, as written ("cups flour, sifted")
	Key    string // what two lines must share to be added up
	Recipe string
}

var (
	noteCutRe = regexp.MustCompile(`\s*(?:,|\(|;| - | – ).*$`)
	tasteRe   = regexp.MustCompile(`(?i)\s+(?:to taste|as needed|if needed|optional|for serving|to serve)\s*$`)
	ofRe      = regexp.MustCompile(`(?i)^(?:of\s+)`)
	nonWordRe = regexp.MustCompile(`[^a-z0-9 ]+`)
)

func singular(w string) string {
	if s, ok := singulars[w]; ok {
		return s
	}
	switch {
	case strings.HasSuffix(w, "ss") || len(w) < 4:
		return w
	case strings.HasSuffix(w, "ies"):
		return w[:len(w)-3] + "y"
	case strings.HasSuffix(w, "oes") || strings.HasSuffix(w, "ches") || strings.HasSuffix(w, "shes"):
		return w[:len(w)-2]
	case strings.HasSuffix(w, "s"):
		return w[:len(w)-1]
	}
	return w
}

// nameKey is what an ingredient is, for adding up: lower case, the note after
// a comma or bracket dropped, plurals made singular.
func nameKey(s string) string {
	s = noteCutRe.ReplaceAllString(s, "")
	s = tasteRe.ReplaceAllString(s, "")
	s = ofRe.ReplaceAllString(strings.TrimSpace(s), "")
	s = nonWordRe.ReplaceAllString(strings.ToLower(s), " ")
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = singular(w)
	}
	return strings.Join(words, " ")
}

// ParseIngredient takes an ingredient line apart.
func ParseIngredient(line string) Ingredient {
	in := Ingredient{Line: strings.TrimSpace(line)}
	_, q, rest, ok := splitQuantity(in.Line)
	if !ok {
		in.Rest = in.Line
		in.Key = nameKey(in.Line)
		return in
	}
	in.Qty, in.Has, in.Rest = q, true, rest
	name := rest
	if m := unitWordRe.FindStringSubmatchIndex(rest); m != nil {
		w := rest[m[2]:m[3]]
		u, found := units[w]
		if !found {
			u, found = units[strings.ToLower(w)]
			if found && (w == "T" || w == "t") {
				found = false
			}
		}
		if found {
			in.Unit = u
			name = rest[m[1]:]
		}
	}
	in.Key = in.Unit + "|" + nameKey(name)
	return in
}

// ShopItem is one line of a shopping list, and the recipes it is for.
type ShopItem struct {
	Text string
	From []string `json:",omitempty"`
}

// Shop adds up the ingredients of several recipes: the same thing in the
// same unit once, its amounts summed. Lines that cannot be added (different
// units, no amount on one) stay apart, in the order first met.
func Shop(lists map[string][]string, order []string) []ShopItem {
	type group struct {
		first Ingredient
		sum   Quantity
		count int
		lines []string
		from  []string
	}
	groups := []*group{}
	byKey := map[string]*group{}
	for _, name := range order {
		for _, l := range lists[name] {
			in := ParseIngredient(l)
			if strings.TrimSpace(in.Line) == "" {
				continue
			}
			key := in.Key
			if !in.Has {
				key = "?|" + in.Key
			}
			g := byKey[key]
			lo, hi := in.Qty.Lo, in.Qty.Hi
			if hi == 0 {
				hi = lo
			}
			if g == nil {
				g = &group{first: in, sum: Quantity{Lo: lo, Hi: hi, style: in.Qty.style}}
				byKey[key] = g
				groups = append(groups, g)
			} else if in.Has {
				g.sum.Lo += lo
				g.sum.Hi += hi
				if g.sum.style == "whole" {
					g.sum.style = in.Qty.style
				}
			}
			g.count++
			g.lines = append(g.lines, in.Line)
			if !contains(g.from, name) {
				g.from = append(g.from, name)
			}
		}
	}
	out := []ShopItem{}
	for _, g := range groups {
		text := g.first.Line
		if g.count > 1 && g.first.Has {
			sum := g.sum
			if math.Abs(sum.Hi-sum.Lo) < 1e-9 {
				sum.Hi = 0
			}
			amount := sum.format(1)
			rest := agree(g.first.Rest, sum.top() <= 1)
			text = amount + rest
		}
		out = append(out, ShopItem{Text: text, From: g.from})
	}
	return out
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
