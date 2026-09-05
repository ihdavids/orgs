package dnd

/* SDOC: DnD
* Appearance

  The appearance step is the one place where the rules have nothing to say, so
  the builder carries its own tables. Every race has a set of eye, skin and hair
  colours plus the height, weight and age ranges its people fall into, and each
  class adds a handful of suggestions on top - a druid is offered moss green
  eyes, a warlock unnaturally violet ones.

  A ruleset module can supply its own table by adding an =appearance:= block to
  a race, which replaces the built in one. Subraces extend their parent rather
  than replacing it.

  The values are also validated: age, height and weight have to be numbers (in
  any of the usual notations - =5'6"=, =168 cm=, =12 stone= is not accepted but
  =70 kg= is) and have to land in a plausible band for the race, and the three
  colour fields have to be words rather than numbers. =44= is not an eye colour.
EDOC */

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Appearance is what the people of a race - or the members of a class - tend
// to look like. Every field is optional: a module can supply colours without
// supplying ranges, and the built in table fills in the rest.
type Appearance struct {
	Eyes []string `yaml:"eyes" json:"eyes"`
	Skin []string `yaml:"skin" json:"skin"`
	Hair []string `yaml:"hair" json:"hair"`
	// AgeAdult is when this race is considered grown, AgeMax how long they live.
	AgeAdult int `yaml:"ageAdult" json:"ageAdult"`
	AgeMax   int `yaml:"ageMax" json:"ageMax"`
	// HeightMin and HeightMax are in inches, WeightMin and WeightMax in pounds.
	HeightMin int `yaml:"heightMin" json:"heightMin"`
	HeightMax int `yaml:"heightMax" json:"heightMax"`
	WeightMin int `yaml:"weightMin" json:"weightMin"`
	WeightMax int `yaml:"weightMax" json:"weightMax"`
	// Note is appended to the help text of the appearance prompt.
	Note string `yaml:"note" json:"note"`
}

// appearance field ids, they double as the Field.Kind of the prompt.
const (
	FieldAge    = "age"
	FieldHeight = "height"
	FieldWeight = "weight"
	FieldEyes   = "eyes"
	FieldSkin   = "skin"
	FieldHair   = "hair"
)

// genericAppearance is the fallback for a race we have no table for, and the
// tail of every suggestion list so that the ordinary colours are always there.
var genericAppearance = Appearance{
	Eyes:      []string{"brown", "blue", "green", "grey", "hazel", "amber", "dark brown", "pale blue"},
	Hair:      []string{"black", "brown", "auburn", "blond", "red", "chestnut", "grey", "white"},
	Skin:      []string{"pale", "fair", "olive", "tan", "bronze", "brown", "dark brown", "ebony"},
	AgeAdult:  18,
	AgeMax:    100,
	HeightMin: 48, HeightMax: 84,
	WeightMin: 80, WeightMax: 320,
}

// raceAppearance is the built in table, keyed by race (or subrace) id. A
// subrace entry adds to its parent rather than replacing it.
var raceAppearance = map[string]Appearance{
	"dragonborn": {
		Eyes: []string{"red", "gold", "silver", "green", "bronze", "black"},
		Hair: []string{"none, a crest of horns", "none, a fringe of scales", "none"},
		Skin: []string{"brass scales", "bronze scales", "copper scales", "gold scales",
			"silver scales", "black scales", "blue scales", "green scales",
			"red scales", "white scales"},
		AgeAdult: 15, AgeMax: 80,
		HeightMin: 68, HeightMax: 80,
		WeightMin: 200, WeightMax: 320,
		Note: "Dragonborn are scaled rather than skinned, and have no hair.",
	},
	"dwarf": {
		Eyes:     []string{"coal black", "dark brown", "hazel", "steel grey"},
		Hair:     []string{"black", "iron grey", "brown", "fiery red", "white, braided"},
		Skin:     []string{"ruddy", "deep tan", "earthy brown", "weathered bronze"},
		AgeAdult: 50, AgeMax: 350,
		HeightMin: 46, HeightMax: 56,
		WeightMin: 130, WeightMax: 200,
	},
	"hill-dwarf": {
		Eyes: []string{"warm hazel"},
		Hair: []string{"sandy brown"},
		Skin: []string{"ruddy tan"},
	},
	"elf": {
		Eyes:     []string{"green", "blue", "violet", "gold", "silver", "amber"},
		Hair:     []string{"silver white", "golden blond", "copper red", "raven black"},
		Skin:     []string{"pale", "alabaster", "coppery", "bronze", "almost blue white"},
		AgeAdult: 100, AgeMax: 750,
		HeightMin: 56, HeightMax: 74,
		WeightMin: 90, WeightMax: 150,
	},
	"high-elf": {
		Eyes: []string{"pale blue"},
		Hair: []string{"pale gold"},
	},
	"gnome": {
		Eyes:     []string{"glittering black", "brilliant blue", "bright brown"},
		Hair:     []string{"fair", "sandy", "snow white", "ash grey"},
		Skin:     []string{"ruddy tan", "woody brown"},
		AgeAdult: 40, AgeMax: 500,
		HeightMin: 37, HeightMax: 43,
		WeightMin: 35, WeightMax: 45,
	},
	"rock-gnome": {
		Hair: []string{"soot streaked grey"},
		Skin: []string{"stone grey tan"},
	},
	"half-elf": {
		Eyes:     []string{"blue", "green", "gold flecked hazel", "grey green"},
		Hair:     []string{"dark brown", "copper", "ash blond", "black"},
		Skin:     []string{"fair", "tan", "coppery", "olive"},
		AgeAdult: 20, AgeMax: 180,
		HeightMin: 59, HeightMax: 73,
		WeightMin: 110, WeightMax: 200,
	},
	"half-orc": {
		Eyes:     []string{"reddish", "dark brown", "amber", "yellow"},
		Hair:     []string{"coarse black", "dark grey", "black, braided"},
		Skin:     []string{"greyish", "grey green", "dusky olive", "ashen grey"},
		AgeAdult: 14, AgeMax: 75,
		HeightMin: 60, HeightMax: 78,
		WeightMin: 140, WeightMax: 250,
	},
	"halfling": {
		Eyes:     []string{"brown", "hazel", "green"},
		Hair:     []string{"curly brown", "sandy", "straw blond", "black"},
		Skin:     []string{"ruddy tan", "fair", "warm brown"},
		AgeAdult: 20, AgeMax: 150,
		HeightMin: 33, HeightMax: 39,
		WeightMin: 35, WeightMax: 45,
	},
	"lightfoot": {
		Hair: []string{"mousy brown"},
	},
	"human": {
		AgeAdult: 18, AgeMax: 90,
		HeightMin: 58, HeightMax: 76,
		WeightMin: 110, WeightMax: 270,
	},
	"tiefling": {
		Eyes:     []string{"solid black", "solid red", "solid white", "solid silver", "solid gold"},
		Hair:     []string{"jet black", "dark blue", "deep purple", "crimson"},
		Skin:     []string{"brick red", "dusky red", "deep russet", "ruddy human tones"},
		AgeAdult: 18, AgeMax: 95,
		HeightMin: 59, HeightMax: 73,
		WeightMin: 110, WeightMax: 220,
		Note: "Tiefling eyes are solid orbs of colour with no visible pupil.",
	},
}

// classAppearance is the flavour each class adds to the colour lists. These
// are suggestions, nothing here is a rule.
var classAppearance = map[string]Appearance{
	"barbarian": {
		Hair: []string{"sun bleached, matted with clay"},
		Skin: []string{"wind burned", "painted with woad"},
	},
	"bard":    {Hair: []string{"elaborately braided", "dyed a bold colour"}},
	"cleric":  {Hair: []string{"shorn in a tonsure"}, Skin: []string{"sun touched"}},
	"druid":   {Eyes: []string{"moss green"}, Hair: []string{"leaf tangled brown"}, Skin: []string{"bark brown, weathered"}},
	"fighter": {Hair: []string{"close cropped"}, Skin: []string{"scarred, weather beaten"}},
	"monk":    {Hair: []string{"shaven", "black topknot"}, Skin: []string{"callused, sun darkened"}},
	"paladin": {Eyes: []string{"clear steady blue"}, Hair: []string{"close cropped"}},
	"ranger":  {Eyes: []string{"keen grey"}, Hair: []string{"wind tangled"}, Skin: []string{"weathered brown"}},
	"rogue":   {Eyes: []string{"watchful dark brown"}, Hair: []string{"cropped short"}},
	"sorcerer": {
		Eyes: []string{"gold flecked", "faintly luminous"},
		Hair: []string{"crackling white", "ember red"},
		Skin: []string{"faintly scaled"},
	},
	"warlock": {
		Eyes: []string{"unnaturally violet", "faintly glowing"},
		Hair: []string{"shadow black"},
		Skin: []string{"unnaturally pale"},
	},
	"wizard": {Hair: []string{"ink stained grey"}, Skin: []string{"lamp pale"}},
}

// appearanceSource is one contributor to the suggestion list, labelled so the
// player can see where a colour came from.
type appearanceSource struct {
	note string // "typical of dwarves", "a druid touch", "" for the generic list
	app  Appearance
}

// appearanceSources gathers the tables that apply to this character, most
// specific first: race, subrace, classes, then the generic colours.
func appearanceSources(rs *Ruleset, c *Character) []appearanceSource {
	out := []appearanceSource{}
	add := func(note string, app Appearance) {
		if len(app.Eyes)+len(app.Skin)+len(app.Hair) == 0 &&
			app.AgeMax == 0 && app.HeightMax == 0 && app.WeightMax == 0 {
			return
		}
		out = append(out, appearanceSource{note: note, app: app})
	}
	raceApp := func(id string) (Appearance, bool) {
		if rs != nil {
			if r := rs.Race(id); r != nil && r.Appearance != nil {
				return *r.Appearance, true
			}
		}
		a, ok := raceAppearance[id]
		return a, ok
	}
	raceName := func(id string) string {
		if rs != nil {
			if r := rs.Race(id); r != nil && r.Name != "" {
				return r.Name
			}
		}
		return Titleize(id)
	}
	if c != nil {
		// The subrace goes first so its colours head the list.
		if c.Subrace != "" && c.Subrace != c.Race {
			if a, ok := raceApp(c.Subrace); ok {
				add("typical of "+raceName(c.Subrace), a)
			}
		}
		if c.Race != "" {
			if a, ok := raceApp(c.Race); ok {
				add("typical of "+raceName(c.Race), a)
			}
		}
		for _, cl := range c.Classes {
			if cl.Class == "" {
				continue
			}
			if a, ok := classAppearance[cl.Class]; ok {
				name := Titleize(cl.Class)
				if rs != nil {
					if def := rs.Class(cl.Class); def != nil && def.Name != "" {
						name = def.Name
					}
				}
				add("a "+strings.ToLower(name)+" touch", a)
			}
		}
	}
	add("", genericAppearance)
	return out
}

// AppearanceFor merges everything that applies to this character into one
// table. Colour lists are concatenated, ranges come from the most specific
// source that has them.
func AppearanceFor(rs *Ruleset, c *Character) Appearance {
	out := Appearance{}
	for _, src := range appearanceSources(rs, c) {
		out.Eyes = addUnique(out.Eyes, src.app.Eyes...)
		out.Skin = addUnique(out.Skin, src.app.Skin...)
		out.Hair = addUnique(out.Hair, src.app.Hair...)
		if out.AgeAdult == 0 {
			out.AgeAdult = src.app.AgeAdult
		}
		if out.AgeMax == 0 {
			out.AgeMax = src.app.AgeMax
		}
		if out.HeightMax == 0 {
			out.HeightMin, out.HeightMax = src.app.HeightMin, src.app.HeightMax
		}
		if out.WeightMax == 0 {
			out.WeightMin, out.WeightMax = src.app.WeightMin, src.app.WeightMax
		}
		if out.Note == "" {
			out.Note = src.app.Note
		}
	}
	return out
}

// colourSuggestions builds the option list for one of the three colour fields.
func colourSuggestions(rs *Ruleset, c *Character, field string) []Option {
	opts := []Option{}
	seen := map[string]bool{}
	for _, src := range appearanceSources(rs, c) {
		var list []string
		switch field {
		case FieldEyes:
			list = src.app.Eyes
		case FieldSkin:
			list = src.app.Skin
		case FieldHair:
			list = src.app.Hair
		}
		for _, v := range list {
			key := strings.ToLower(strings.TrimSpace(v))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			detail := "An ordinary colour, nothing about this character says otherwise."
			if src.note != "" {
				detail = Titleize(field) + ": " + src.note + "."
			}
			opts = append(opts, Option{Id: v, Name: v, Summary: src.note, Detail: detail})
		}
	}
	return opts
}

// ----------------------------------------------------------------------------
// numeric suggestions
// ----------------------------------------------------------------------------

// spread returns n values evenly spaced across [lo, hi].
func spread(lo, hi, n int) []int {
	if n < 2 || hi <= lo {
		return []int{lo}
	}
	out := []int{}
	for i := 0; i < n; i++ {
		out = append(out, lo+(hi-lo)*i/(n-1))
	}
	return out
}

var sizeWords = []string{"small for the kind", "on the small side", "average", "large", "the biggest of their kind"}

func heightSuggestions(app Appearance) []Option {
	opts := []Option{}
	for i, in := range spread(app.HeightMin, app.HeightMax, 5) {
		v := FormatHeight(in)
		cm := fmt.Sprintf("%d cm", int(float64(in)*2.54+0.5))
		opts = append(opts, Option{Id: v, Name: v, Summary: sizeWords[i],
			Detail: fmt.Sprintf("%s, or %s. Type your own if you want something else.", v, cm),
			Meta:   map[string]string{"metric": cm}})
	}
	return opts
}

var buildWords = []string{"slight", "lean", "average", "heavy set", "hulking"}

func weightSuggestions(app Appearance) []Option {
	opts := []Option{}
	for i, lb := range spread(app.WeightMin, app.WeightMax, 5) {
		v := fmt.Sprintf("%d lb.", lb)
		kg := fmt.Sprintf("%d kg", int(float64(lb)/2.2046+0.5))
		opts = append(opts, Option{Id: v, Name: v, Summary: buildWords[i],
			Detail: fmt.Sprintf("%s, or %s. Type your own if you want something else.", v, kg),
			Meta:   map[string]string{"metric": kg}})
	}
	return opts
}

// ageSuggestions walks a life from just grown to the end of it.
func ageSuggestions(app Appearance) []Option {
	adult, max := app.AgeAdult, app.AgeMax
	if adult <= 0 {
		adult = 18
	}
	if max <= adult {
		max = adult * 4
	}
	stages := []struct {
		at   float64
		what string
	}{
		{0.0, "newly grown"},
		{0.15, "young"},
		{0.35, "in their prime"},
		{0.55, "middle aged"},
		{0.8, "old"},
	}
	opts := []Option{}
	span := float64(max - adult)
	for _, st := range stages {
		years := adult + int(span*st.at)
		v := strconv.Itoa(years)
		opts = append(opts, Option{Id: v, Name: v, Summary: st.what,
			Detail: fmt.Sprintf("%s at %d, for a people grown at %d and living to about %d.",
				Titleize(st.what), years, adult, max)})
	}
	return opts
}

// ----------------------------------------------------------------------------
// the prompt fields
// ----------------------------------------------------------------------------

// AppearanceFields builds the six inputs of the appearance prompt, each with
// suggestions and with the bounds its answer is checked against.
func AppearanceFields(rs *Ruleset, c *Character) []Field {
	app := AppearanceFor(rs, c)
	pad := func(lo, hi int, out, in float64) (int, int) {
		return int(float64(lo) * out), int(float64(hi) * in)
	}
	hMin, hMax := pad(app.HeightMin, app.HeightMax, 0.8, 1.2)
	wMin, wMax := pad(app.WeightMin, app.WeightMax, 0.5, 2.0)
	ageMax := app.AgeMax
	if ageMax <= 0 {
		ageMax = 100
	}
	return []Field{
		{Id: FieldAge, Name: "Age", Kind: FieldAge, Options: ageSuggestions(app),
			Min: 1, Max: ageMax, AllowCustom: true,
			Hint: fmt.Sprintf("in years, grown at %d and living to about %d", app.AgeAdult, ageMax)},
		{Id: FieldHeight, Name: "Height", Kind: FieldHeight, Options: heightSuggestions(app),
			Min: hMin, Max: hMax, AllowCustom: true,
			Hint: fmt.Sprintf("%s to %s, or say it in cm", FormatHeight(app.HeightMin), FormatHeight(app.HeightMax))},
		{Id: FieldWeight, Name: "Weight", Kind: FieldWeight, Options: weightSuggestions(app),
			Min: wMin, Max: wMax, AllowCustom: true,
			Hint: fmt.Sprintf("%d to %d lb., or say it in kg", app.WeightMin, app.WeightMax)},
		{Id: FieldEyes, Name: "Eyes", Kind: FieldEyes, Options: colourSuggestions(rs, c, FieldEyes),
			AllowCustom: true, Hint: "a colour, not a number"},
		{Id: FieldSkin, Name: "Skin", Kind: FieldSkin, Options: colourSuggestions(rs, c, FieldSkin),
			AllowCustom: true, Hint: "a colour, not a number"},
		{Id: FieldHair, Name: "Hair", Kind: FieldHair, Options: colourSuggestions(rs, c, FieldHair),
			AllowCustom: true, Hint: "a colour, not a number"},
	}
}

// ----------------------------------------------------------------------------
// parsing and validation
// ----------------------------------------------------------------------------

// FormatHeight renders inches the way a character sheet does: 5'6".
func FormatHeight(inches int) string {
	if inches <= 0 {
		return ""
	}
	return fmt.Sprintf("%d'%d\"", inches/12, inches%12)
}

// number pulls the leading number off a string, returning the rest of it.
func number(s string) (float64, string, bool) {
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	if i == 0 {
		return 0, s, false
	}
	v, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, s, false
	}
	return v, strings.TrimSpace(s[i:]), true
}

// unitOf strips a unit word off the front of s, reporting whether one of the
// given names matched.
func unitOf(s string, names ...string) (string, bool) {
	for _, n := range names {
		if strings.HasPrefix(s, n) {
			return strings.TrimSpace(strings.TrimPrefix(s, n)), true
		}
	}
	return s, false
}

// isUnit reports whether what is left after a number is one of the accepted
// unit words, an empty tail counting as the default unit.
func isUnit(s string, names ...string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	for _, n := range names {
		if s == n {
			return true
		}
	}
	return false
}

// ParseHeight reads a height in any of the notations people actually type and
// returns it in inches, plus whether it was written in metric.
func ParseHeight(s string) (inches float64, metric, ok bool) {
	t := strings.ToLower(strings.TrimSpace(s))
	t = strings.ReplaceAll(t, "’", "'") // curly quotes off a web form
	t = strings.ReplaceAll(t, "′", "'")
	t = strings.ReplaceAll(t, "”", "\"")
	t = strings.ReplaceAll(t, "″", "\"")
	v, rest, got := number(t)
	if !got {
		return 0, false, false
	}
	switch {
	case strings.HasPrefix(rest, "cm"):
		return v / 2.54, true, true
	case strings.HasPrefix(rest, "mm"):
		return v / 25.4, true, true
	case strings.HasPrefix(rest, "m"):
		return v * 39.3701, true, true
	}
	if r, isFeet := unitOf(rest, "'", "feet", "foot", "ft.", "ft"); isFeet {
		feet := v
		if r == "" {
			return feet * 12, false, true
		}
		in, tail, got2 := number(r)
		if !got2 || !isUnit(tail, "\"", "in", "in.", "inch", "inches") {
			return 0, false, false
		}
		return feet*12 + in, false, true
	}
	if isUnit(rest, "\"", "in", "in.", "inch", "inches") {
		return v, false, true
	}
	return 0, false, false
}

// ParseWeight reads a weight and returns it in pounds, plus whether it was
// written in metric.
func ParseWeight(s string) (pounds float64, metric, ok bool) {
	t := strings.ToLower(strings.TrimSpace(s))
	v, rest, got := number(t)
	if !got {
		return 0, false, false
	}
	switch {
	case strings.HasPrefix(rest, "kg"), strings.HasPrefix(rest, "kilo"):
		return v * 2.20462, true, true
	case strings.HasPrefix(rest, "g") && !strings.HasPrefix(rest, "gr"):
		return v * 0.00220462, true, true
	}
	if isUnit(rest, "lb", "lb.", "lbs", "lbs.", "pound", "pounds", "#") {
		return v, false, true
	}
	return 0, false, false
}

// hasLetter reports whether s contains anything that could be part of a word.
func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// ValidateField checks one answer to an appearance field and returns it in the
// form the sheet stores. An empty value is always allowed, the whole step is
// optional. The client runs this before it posts and the builder runs it again
// on the way in, so the rules are the same wherever the answer came from.
func ValidateField(f Field, value string) (string, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", nil
	}
	switch f.Kind {
	case FieldAge:
		years, rest, ok := number(strings.ToLower(v))
		if !ok || !isUnit(rest, "y", "yr", "yrs", "year", "years", "years old") {
			return "", fmt.Errorf("age should be a number of years, not %q", value)
		}
		n := int(years)
		if f.Max > 0 && (n < f.Min || n > f.Max) {
			return "", fmt.Errorf("%d is outside the %d to %d years this race lives", n, f.Min, f.Max)
		}
		return strconv.Itoa(n), nil
	case FieldHeight:
		in, metric, ok := ParseHeight(v)
		if !ok {
			return "", fmt.Errorf("%q is not a height, try 5'6\" or 168 cm", value)
		}
		if f.Max > 0 && (int(in) < f.Min || int(in) > f.Max) {
			return "", fmt.Errorf("%s is outside the %s to %s this race stands",
				FormatHeight(int(in+0.5)), FormatHeight(f.Min), FormatHeight(f.Max))
		}
		if metric {
			return fmt.Sprintf("%d cm", int(in*2.54+0.5)), nil
		}
		return FormatHeight(int(in + 0.5)), nil
	case FieldWeight:
		lb, metric, ok := ParseWeight(v)
		if !ok {
			return "", fmt.Errorf("%q is not a weight, try 160 lb. or 73 kg", value)
		}
		if f.Max > 0 && (int(lb) < f.Min || int(lb) > f.Max) {
			return "", fmt.Errorf("%d lb. is outside the %d to %d lb. this race weighs",
				int(lb+0.5), f.Min, f.Max)
		}
		if metric {
			return fmt.Sprintf("%d kg", int(lb/2.20462+0.5)), nil
		}
		return fmt.Sprintf("%d lb.", int(lb+0.5)), nil
	case FieldEyes, FieldSkin, FieldHair:
		if !hasLetter(v) {
			return "", fmt.Errorf("%q is not %s colour, describe it in words like %q",
				value, colourNoun(f.Kind), exampleColour(f))
		}
		if len([]rune(v)) > 60 {
			return "", fmt.Errorf("keep the %s to a few words, that is a backstory",
				strings.ToLower(f.Name))
		}
		return v, nil
	}
	return v, nil
}

// colourNoun names one of the colour fields the way a sentence wants it.
func colourNoun(kind string) string {
	switch kind {
	case FieldEyes:
		return "an eye"
	case FieldHair:
		return "a hair"
	case FieldSkin:
		return "a skin"
	}
	return "a"
}

// exampleColour is the first suggestion of a field, used in its error message.
func exampleColour(f Field) string {
	if len(f.Options) > 0 {
		return f.Options[0].Name
	}
	return "hazel"
}

// RandomAppearanceValue picks a value for one field, used by "choose for me"
// and by the fully random character generator.
func RandomAppearanceValue(f Field) string {
	if len(f.Options) > 0 {
		return f.Options[randIndex(len(f.Options))].Name
	}
	if f.Max > f.Min && f.Min > 0 {
		n := f.Min + randIndex(f.Max-f.Min+1)
		switch f.Kind {
		case FieldHeight:
			return FormatHeight(n)
		case FieldWeight:
			return fmt.Sprintf("%d lb.", n)
		default:
			return strconv.Itoa(n)
		}
	}
	return ""
}
