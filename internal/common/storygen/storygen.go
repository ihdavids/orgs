// Package storygen makes ideas for a writer: people, places, cultures,
// faiths, organisations, premises, complications, objects and weather, in a
// fantasy, science fiction or contemporary key. worg's writing mode offers
// them beside the draft, to be put in at the cursor, left as a revision note
// or filed in the manuscript's compendium.
//
// The tables are our own. Invented names come from a small made-up language
// built for each result (lang.go), so a culture's people, towns and gods sound
// as though they come from the same place.
//
// It is shaped like the table's random generator (dnd/randgen) - a list of
// generators each describing its form, a seeded run giving a handful of
// results - so worg draws both forms the same way. Each result carries:
//
//   - Title and Summary, for the list;
//   - Facts, the result as labelled lines (what a compendium entry keeps);
//   - Body, the facts as an org description list (never a heading);
//   - Prose, the same idea as a short paragraph to drop into a draft.
//
// Generation is deterministic for a seed. A request with no seed gets one and
// the response says which.
package storygen

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
)

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
// A "genre" field is shared by the client across every generator that has
// one, so it is listed here but usually shown once.
type Info struct {
	Id      string  `json:"id"`
	Name    string  `json:"name"`
	Group   string  `json:"group"`
	Summary string  `json:"summary"`
	Fields  []Field `json:"fields"`
	Count   int     `json:"count"`
}

// Fact is one labelled line of a result.
type Fact struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Item is one generated idea. Entry is the compendium kind it would be filed
// as (character, place, group, thing) or "" when it is not a thing in the
// story's world but a prompt about it.
type Item struct {
	Kind    string   `json:"kind"`
	Title   string   `json:"title"`
	Summary string   `json:"summary"`
	Facts   []Fact   `json:"facts"`
	Body    string   `json:"body"`
	Prose   string   `json:"prose"`
	Entry   string   `json:"entry,omitempty"`
	Tags    []string `json:"tags"`
}

type Request struct {
	Type    string            `json:"type"`
	Options map[string]string `json:"options"`
	Count   int               `json:"count"`
	Seed    uint64            `json:"seed"`
}

type Response struct {
	Ok    bool   `json:"ok"`
	Msg   string `json:"msg,omitempty"`
	Type  string `json:"type"`
	Seed  uint64 `json:"seed"`
	Items []Item `json:"items"`
}

// MaxCount bounds a run.
const MaxCount = 25

// Genres, as a field's choices. "any" picks one for each result.
var genreField = Field{Id: "genre", Label: "Genre", Kind: "select", Default: "fantasy",
	Choices: []Choice{{"fantasy", "Fantasy"}, {"sf", "Science fiction"}, {"modern", "Contemporary"}, {"any", "Any genre"}}}

var genres = []string{"fantasy", "sf", "modern"}

type ctx struct {
	r     *rand.Rand
	opts  map[string]string
	genre string // for the result being made: never "any"
}

type generator struct {
	info Info
	run  func(c *ctx) Item
}

var registry []generator

func register(info Info, run func(c *ctx) Item) {
	if info.Count == 0 {
		info.Count = 3
	}
	registry = append(registry, generator{info: info, run: run})
}

// Generators lists every generator in menu order.
func Generators() []Info {
	out := make([]Info, 0, len(registry))
	for _, g := range registry {
		info := g.info
		info.Fields = append([]Field(nil), g.info.Fields...)
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

// Generate runs a request.
func Generate(req Request) (Response, error) {
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
	c := &ctx{r: newRand(seed), opts: opts}
	res := Response{Ok: true, Type: g.info.Id, Seed: seed}
	for i := 0; i < n; i++ {
		c.genre = opts["genre"]
		if !known(c.genre) {
			c.genre = c.pick(genres)
		}
		it := g.run(c)
		if it.Kind == "" {
			it.Kind = g.info.Id
		}
		if it.Body == "" {
			it.Body = factList(it.Facts)
		}
		it.Tags = append([]string{c.genre}, it.Tags...)
		res.Items = append(res.Items, it)
	}
	return res, nil
}

func newRand(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
}

func known(genre string) bool {
	for _, g := range genres {
		if g == genre {
			return true
		}
	}
	return false
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

func (c *ctx) between(lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + c.r.IntN(hi-lo+1)
}

func (c *ctx) chance(pct int) bool { return c.r.IntN(100) < pct }

func (c *ctx) opt(id string) string { return c.opts[id] }

// gl is a table by genre; the "" list belongs to every genre.
type gl map[string][]string

// list is what a table offers the result's genre: its "all" entries, those
// tagged for the genre, and the genre's own section.
func (c *ctx) list(t gl) []string {
	return c.section(t, "", c.genre)
}

// section is one section's entries for the genre, tagged ones included.
func (c *ctx) section(t gl, sec, more string) []string {
	out := append(append([]string(nil), t[sec]...), t[sec+"@"+c.genre]...)
	if more != "" {
		out = append(out, t[more]...)
	}
	return out
}

func (c *ctx) g(t gl) string { return c.pick(c.list(t)) }

func (c *ctx) gN(t gl, n int) []string { return c.pickN(c.list(t), n) }

// t picks from a data table by name, in the result's genre.
func (c *ctx) t(name string) string { return c.g(tab(name)) }

// tN is n different entries of a data table.
func (c *ctx) tN(name string, n int) []string { return c.gN(tab(name), n) }

// ts picks from one section of a data table (a season, a kind).
func (c *ctx) ts(name, section string) string { return c.pick(c.section(tab(name), section, "")) }

// fill replaces each {name} with vars[name]. A name it has no value for is
// left as it is, which the tests catch.
func fill(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

// ---- formatting helpers ----------------------------------------------------

func capital(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

func title(s string) string {
	words := strings.Fields(s)
	small := map[string]bool{"of": true, "the": true, "and": true, "in": true, "on": true, "a": true, "at": true}
	for i, w := range words {
		if i > 0 && small[w] {
			continue
		}
		words[i] = capital(w)
	}
	return strings.Join(words, " ")
}

// article puts "a" or "an" in front, by sound where the spelling misleads:
// a uniform, a one-eyed cat, a European; an hour, an honest man.
func article(s string) string {
	if s == "" {
		return s
	}
	l := strings.ToLower(s)
	for _, p := range []string{"hour", "honest", "honour", "heir"} {
		if strings.HasPrefix(l, p) {
			return "an " + s
		}
	}
	for _, p := range []string{"uni", "use", "usu", "uti", "ura", "ure", "eu", "ewe", "one", "once", "ubiq"} {
		if strings.HasPrefix(l, p) {
			return "a " + s
		}
	}
	switch l[:1] {
	case "a", "e", "i", "o", "u":
		return "an " + s
	}
	return "a " + s
}

// sentence makes s a sentence: a capital, and a full stop unless it ends in
// one already.
func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = capital(s)
	switch s[len(s)-1] {
	case '.', '!', '?', '"':
		return s
	}
	if strings.HasSuffix(s, "”") || strings.HasSuffix(s, "…") {
		return s
	}
	return s + "."
}

// prose joins sentences, skipping empty ones.
func prose(parts ...string) string {
	out := []string{}
	for _, p := range parts {
		if p = sentence(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}

func facts(kv ...string) []Fact {
	out := []Fact{}
	for i := 0; i+1 < len(kv); i += 2 {
		if strings.TrimSpace(kv[i+1]) != "" {
			out = append(out, Fact{Key: kv[i], Value: kv[i+1]})
		}
	}
	return out
}

// factList writes "- Key :: value" lines, the org description list.
func factList(fs []Fact) string {
	var b strings.Builder
	for _, f := range fs {
		b.WriteString("- " + f.Key + " :: " + f.Value + "\n")
	}
	return b.String()
}

// andList is "a", "a and b", "a, b and c".
func andList(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
}
