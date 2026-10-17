// Package randgen is the table's random generator: encounters, traps,
// treasure, magic items, spellbooks, NPCs, taverns and the small oddities a
// game master reaches for mid-session.
//
// It works the way donjon's 5e random generator does - pick a kind, set a few
// options, get a handful of results - but the tables are our own. What comes
// from the rules (monsters, magic items, spells, races) is read from the SRD
// 5.1 (CC-BY-4.0) and from whatever ruleset the server has loaded, so a
// homebrew module's items and spells turn up in the treasure too.
//
// Every result is an Item whose Body is org text, so what is shown is exactly
// what gets saved: a result is filed as one heading with that body under it.
// The body is indented under the heading when it is written, so it must never
// hold a heading of its own - tables and lists only.
//
// Generation is deterministic for a seed. A request with no seed gets one and
// the response says which, so a result that someone liked can be had again.
package randgen

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common/dnd"
)

// Choice is one value of a select field.
type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Field is one option a generator takes. Kind is "select" or "number".
type Field struct {
	Id      string   `json:"id"`
	Label   string   `json:"label"`
	Kind    string   `json:"kind"`
	Choices []Choice `json:"choices,omitempty"`
	Default string   `json:"default"`
	Min     int      `json:"min,omitempty"`
	Max     int      `json:"max,omitempty"`
}

// Info describes a generator to a client, which draws its form from Fields.
type Info struct {
	Id      string  `json:"id"`
	Name    string  `json:"name"`
	Group   string  `json:"group"`
	Summary string  `json:"summary"`
	Fields  []Field `json:"fields"`
	// Count is how many results a run gives unless asked for another number.
	Count int `json:"count"`
}

// Item is one generated thing.
type Item struct {
	Kind    string            `json:"kind"`
	Title   string            `json:"title"`
	Summary string            `json:"summary"`
	Body    string            `json:"body"`
	Tags    []string          `json:"tags"`
	Props   map[string]string `json:"props,omitempty"`
}

// Request asks for Count results of generator Type.
type Request struct {
	Type    string            `json:"type"`
	Options map[string]string `json:"options"`
	Count   int               `json:"count"`
	Seed    uint64            `json:"seed"`
	Ruleset string            `json:"ruleset"`
}

type Response struct {
	Ok    bool   `json:"ok"`
	Msg   string `json:"msg,omitempty"`
	Type  string `json:"type"`
	Seed  uint64 `json:"seed"`
	Items []Item `json:"items"`
}

// MaxCount bounds a run: enough for a page of trinkets, few enough that a
// typo in a count field does not build ten thousand encounters.
const MaxCount = 25

type ctx struct {
	r    *rand.Rand
	rs   *dnd.Ruleset
	opts map[string]string
}

type generator struct {
	info Info
	run  func(c *ctx) Item
}

var registry []generator

func register(info Info, run func(c *ctx) Item) {
	if info.Count == 0 {
		info.Count = 5
	}
	registry = append(registry, generator{info: info, run: run})
}

// Generators lists every generator in the order the menu shows them. A race
// field's choices are the ruleset's races, so a homebrew race can be asked
// for by name.
func Generators(rs *dnd.Ruleset) []Info {
	c := &ctx{rs: rs}
	out := make([]Info, 0, len(registry))
	for _, g := range registry {
		info := g.info
		info.Fields = append([]Field(nil), g.info.Fields...)
		for i := range info.Fields {
			if info.Fields[i].Id == "race" {
				info.Fields[i].Choices = raceChoices(c.raceNames())
			}
		}
		out = append(out, info)
	}
	return out
}

func find(id string) *generator {
	for i := range registry {
		if strings.EqualFold(registry[i].info.Id, id) {
			return &registry[i]
		}
	}
	return nil
}

// Generate runs a request against a ruleset. The ruleset may be nil; the
// generators that need one (magic items, spellbooks, races) then say so in
// their result rather than failing the run.
func Generate(rs *dnd.Ruleset, req Request) (Response, error) {
	g := find(req.Type)
	if g == nil {
		return Response{}, fmt.Errorf("no generator %q", req.Type)
	}
	n := req.Count
	if n <= 0 {
		n = g.info.Count
	}
	if n > MaxCount {
		n = MaxCount
	}
	seed := req.Seed
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	opts := map[string]string{}
	for _, f := range g.info.Fields {
		opts[f.Id] = f.Default
	}
	for k, v := range req.Options {
		if strings.TrimSpace(v) != "" {
			opts[k] = v
		}
	}
	c := &ctx{r: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)), rs: rs, opts: opts}
	res := Response{Ok: true, Type: g.info.Id, Seed: seed}
	for i := 0; i < n; i++ {
		it := g.run(c)
		if it.Kind == "" {
			it.Kind = g.info.Id
		}
		it.Tags = withTag(it.Tags, "dnd")
		res.Items = append(res.Items, it)
	}
	return res, nil
}

func withTag(tags []string, t string) []string {
	for _, x := range tags {
		if x == t {
			return tags
		}
	}
	return append([]string{t}, tags...)
}

// ---- random helpers --------------------------------------------------------

func (c *ctx) pick(list []string) string {
	if len(list) == 0 {
		return ""
	}
	return list[c.r.IntN(len(list))]
}

// pickN is n distinct entries, or all of them when there are fewer.
func (c *ctx) pickN(list []string, n int) []string {
	idx := c.r.Perm(len(list))
	if n > len(idx) {
		n = len(idx)
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = list[idx[i]]
	}
	return out
}

// roll throws n dice of d sides.
func (c *ctx) roll(n, d int) int {
	t := 0
	for i := 0; i < n; i++ {
		t += 1 + c.r.IntN(d)
	}
	return t
}

func (c *ctx) between(lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + c.r.IntN(hi-lo+1)
}

func (c *ctx) chance(pct int) bool { return c.r.IntN(100) < pct }

func (c *ctx) opt(id string) string { return c.opts[id] }

func (c *ctx) optInt(id string, def, lo, hi int) int {
	n := def
	fmt.Sscanf(c.opts[id], "%d", &n)
	if n < lo {
		n = lo
	}
	if n > hi {
		n = hi
	}
	return n
}

// weighted picks a key with probability proportional to its weight.
func (c *ctx) weighted(keys []string, weights []int) string {
	total := 0
	for _, w := range weights {
		total += w
	}
	if total <= 0 {
		return c.pick(keys)
	}
	x := c.r.IntN(total)
	for i, w := range weights {
		if x < w {
			return keys[i]
		}
		x -= w
	}
	return keys[len(keys)-1]
}

// ---- formatting helpers ----------------------------------------------------

func choices(values ...string) []Choice {
	out := make([]Choice, 0, len(values))
	for _, v := range values {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) == 2 {
			out = append(out, Choice{Value: parts[0], Label: parts[1]})
		} else {
			out = append(out, Choice{Value: v, Label: title(v)})
		}
	}
	return out
}

func levelChoices() []Choice {
	out := []Choice{}
	for i := 1; i <= 20; i++ {
		out = append(out, Choice{Value: fmt.Sprint(i), Label: fmt.Sprint(i)})
	}
	return out
}

func title(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func capital(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// article puts "a" or "an" in front.
func article(s string) string {
	if s == "" {
		return s
	}
	switch strings.ToLower(s[:1]) {
	case "a", "e", "i", "o", "u":
		return "an " + s
	}
	return "a " + s
}

// orgTable lays rows out as an org table with a rule under the header.
func orgTable(header []string, rows [][]string) string {
	widths := make([]int, len(header))
	all := append([][]string{header}, rows...)
	for _, r := range all {
		for i, c := range r {
			if i < len(widths) && len([]rune(c)) > widths[i] {
				widths[i] = len([]rune(c))
			}
		}
	}
	line := func(r []string) string {
		var b strings.Builder
		b.WriteString("|")
		for i := range widths {
			c := ""
			if i < len(r) {
				c = r[i]
			}
			b.WriteString(" " + c + strings.Repeat(" ", widths[i]-len([]rune(c))) + " |")
		}
		return b.String()
	}
	var b strings.Builder
	b.WriteString(line(header) + "\n")
	b.WriteString("|")
	for i, w := range widths {
		b.WriteString(strings.Repeat("-", w+2))
		if i < len(widths)-1 {
			b.WriteString("+")
		}
	}
	b.WriteString("|\n")
	for _, r := range rows {
		b.WriteString(line(r) + "\n")
	}
	return b.String()
}

// fields writes "- Name :: value" lines, the org description list, skipping
// the empty ones.
func fields(kv ...string) string {
	var b strings.Builder
	for i := 0; i+1 < len(kv); i += 2 {
		if strings.TrimSpace(kv[i+1]) == "" {
			continue
		}
		b.WriteString("- " + kv[i] + " :: " + kv[i+1] + "\n")
	}
	return b.String()
}

// DLine is one "label :: text" line of a description.
type DLine struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

func dfields(lines []DLine) string {
	kv := []string{}
	for _, l := range lines {
		kv = append(kv, l.Label, l.Text)
	}
	return fields(kv...)
}

// noHeadings keeps text from the ruleset from starting a line with stars,
// which org would read as a heading once the body is filed.
func noHeadings(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "*") {
			t := strings.TrimLeft(l, "*")
			if strings.HasPrefix(t, " ") {
				lines[i] = "-" + t
			}
		}
	}
	return strings.Join(lines, "\n")
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func commas(n int) string {
	s := fmt.Sprint(n)
	if n < 0 {
		return "-" + commas(-n)
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
