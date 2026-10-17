package randgen

import (
	"embed"
	"fmt"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed data/srd-monsters.yaml
var monsterData embed.FS

// Monster is the part of a stat block an encounter is built from.
type Monster struct {
	Id           string   `yaml:"id" json:"id"`
	Name         string   `yaml:"name" json:"name"`
	Size         string   `yaml:"size" json:"size"`
	Type         string   `yaml:"type" json:"type"`
	Subtype      string   `yaml:"subtype" json:"subtype"`
	Alignment    string   `yaml:"alignment" json:"alignment"`
	AC           int      `yaml:"ac" json:"ac"`
	HP           int      `yaml:"hp" json:"hp"`
	HitDice      string   `yaml:"hitDice" json:"hitDice"`
	CR           string   `yaml:"cr" json:"cr"`
	XP           int      `yaml:"xp" json:"xp"`
	Environments []string `yaml:"environments" json:"environments"`

	// The stat block, for the initiative tracker.
	AcNote         string         `yaml:"acNote" json:"acNote,omitempty"`
	Speed          string         `yaml:"speed" json:"speed,omitempty"`
	Abilities      []int          `yaml:"abilities" json:"abilities,omitempty"` // STR DEX CON INT WIS CHA
	Details        [][]string     `yaml:"details" json:"details,omitempty"`     // [label, text]
	Traits         []MonsterEntry `yaml:"traits" json:"traits,omitempty"`
	Actions        []MonsterEntry `yaml:"actions" json:"actions,omitempty"`
	Reactions      []MonsterEntry `yaml:"reactions" json:"reactions,omitempty"`
	Legendary      []MonsterEntry `yaml:"legendary" json:"legendary,omitempty"`
	LegendaryIntro string         `yaml:"legendaryIntro" json:"legendaryIntro,omitempty"`
}

type MonsterEntry struct {
	Name string `yaml:"name" json:"name"`
	Text string `yaml:"text" json:"text"`
}

// Init is the monster's initiative bonus: its Dexterity modifier.
func (m *Monster) Init() int {
	if len(m.Abilities) < 2 {
		return 0
	}
	d := m.Abilities[1] - 10
	if d < 0 {
		d--
	}
	return d / 2
}

// FindMonster looks a monster up the way it is written in a room or a
// wandering monster table: by name, ignoring case and a plural "s", and
// ignoring a "(...)" the SRD hangs on some names.
func FindMonster(name string) *Monster {
	norm := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		if i := strings.Index(s, " ("); i > 0 {
			s = s[:i]
		}
		return s
	}
	want := norm(name)
	all := Monsters()
	for _, try := range []string{want, strings.TrimSuffix(want, "s"), strings.TrimSuffix(want, "es")} {
		for i := range all {
			if norm(all[i].Name) == try {
				return &all[i]
			}
		}
	}
	return nil
}

// SearchMonsters is the monsters whose names hold every word of q, the ones
// whose names start with it first.
func SearchMonsters(q string, limit int) []Monster {
	words := strings.Fields(strings.ToLower(q))
	type hit struct {
		m     Monster
		start bool
	}
	var hits []hit
	for _, m := range Monsters() {
		n := strings.ToLower(m.Name)
		ok := true
		for _, w := range words {
			if !strings.Contains(n, w) {
				ok = false
				break
			}
		}
		if ok {
			hits = append(hits, hit{m, len(words) > 0 && strings.HasPrefix(n, words[0])})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool {
		if hits[a].start != hits[b].start {
			return hits[a].start
		}
		return hits[a].m.Name < hits[b].m.Name
	})
	out := []Monster{}
	for i := 0; i < len(hits) && (limit <= 0 || i < limit); i++ {
		out = append(out, hits[i].m)
	}
	return out
}

var (
	monstersOnce sync.Once
	monsters     []Monster
)

// Monsters is the SRD monster list, read once.
func Monsters() []Monster {
	monstersOnce.Do(func() {
		b, err := monsterData.ReadFile("data/srd-monsters.yaml")
		if err != nil {
			return
		}
		var doc struct {
			Monsters []Monster `yaml:"monsters"`
		}
		if yaml.Unmarshal(b, &doc) == nil {
			monsters = doc.Monsters
		}
	})
	return monsters
}

func (m *Monster) livesIn(env string) bool {
	if env == "" || env == "any" {
		return true
	}
	for _, e := range m.Environments {
		if e == env {
			return true
		}
	}
	return false
}

// The XP a character of each level can take at each difficulty: easy,
// medium, hard, deadly. These are the fifth edition encounter thresholds.
var thresholds = [21][4]int{
	{},
	{25, 50, 75, 100},
	{50, 100, 150, 200},
	{75, 150, 225, 400},
	{125, 250, 375, 500},
	{250, 500, 750, 1100},
	{300, 600, 900, 1400},
	{350, 750, 1100, 1700},
	{450, 900, 1400, 2100},
	{550, 1100, 1600, 2400},
	{600, 1200, 1900, 2800},
	{800, 1600, 2400, 3600},
	{1000, 2000, 3000, 4500},
	{1100, 2200, 3400, 5100},
	{1250, 2500, 3800, 5700},
	{1400, 2800, 4300, 6400},
	{1600, 3200, 4800, 7200},
	{2000, 3900, 5900, 8800},
	{2100, 4200, 6300, 9500},
	{2400, 4900, 7300, 10900},
	{2800, 5700, 8500, 12700},
}

var difficulties = []string{"easy", "medium", "hard", "deadly"}

// multiplier is how much harder a crowd is than the sum of its XP. A small
// party finds every crowd one step worse, a large one one step easier.
func multiplier(monsters, party int) float64 {
	steps := []float64{0.5, 1, 1.5, 2, 2.5, 3, 4, 5}
	i := 1
	switch {
	case monsters <= 1:
		i = 1
	case monsters == 2:
		i = 2
	case monsters <= 6:
		i = 3
	case monsters <= 10:
		i = 4
	case monsters <= 14:
		i = 5
	default:
		i = 6
	}
	if party < 3 {
		i++
	} else if party > 5 {
		i--
	}
	if i < 0 {
		i = 0
	}
	if i >= len(steps) {
		i = len(steps) - 1
	}
	return steps[i]
}

type group struct {
	m *Monster
	n int
}

func adjusted(gs []group, party int) (raw, adj int) {
	count := 0
	for _, g := range gs {
		raw += g.m.XP * g.n
		count += g.n
	}
	return raw, int(float64(raw) * multiplier(count, party))
}

// rate says which difficulty an adjusted XP comes to for a party.
func rate(adj, party, level int) string {
	t := thresholds[level]
	out := "trivial"
	for i, d := range difficulties {
		if adj >= t[i]*party {
			out = d
		}
	}
	return out
}

// encounter builds a fight for a party out of the monsters keep allows, at a
// difficulty ("any" picks one). It answers nil when nothing fits.
func (c *ctx) encounter(party, level int, diff string, keep func(*Monster) bool) (best []group, raw, adj int, actual string) {
	if diff == "" || diff == "any" {
		diff = c.weighted(difficulties, []int{2, 4, 3, 1})
	}
	di := 1
	for i, d := range difficulties {
		if d == diff {
			di = i
		}
	}
	lo := thresholds[level][di] * party
	hi := lo * 3 / 2
	if di < 3 {
		hi = thresholds[level][di+1] * party
	}

	pool := []*Monster{}
	all := Monsters()
	for i := range all {
		m := &all[i]
		if m.XP > 0 && m.XP <= hi && keep(m) {
			pool = append(pool, m)
		}
	}
	if len(pool) == 0 {
		return nil, 0, 0, ""
	}
	bestGap := -1
	consider := func(gs []group) bool {
		_, adj := adjusted(gs, party)
		gap := 0
		if adj < lo {
			gap = lo - adj
		} else if adj >= hi {
			gap = adj - hi + 1
		}
		if bestGap < 0 || gap < bestGap {
			best, bestGap = append([]group(nil), gs...), gap
		}
		return gap == 0
	}

	// Three shapes, tried at random until one lands in the band: one big
	// thing, a pack of one kind, or a leader with followers. The pack is the
	// commonest at a table, so it is tried most.
	for try := 0; try < 400; try++ {
		lead := pool[c.r.IntN(len(pool))]
		shape := c.weighted([]string{"solo", "pack", "leader"}, []int{2, 5, 3})
		var gs []group
		switch shape {
		case "solo":
			gs = []group{{lead, 1}}
		case "pack":
			n := 1
			for n < 12 {
				if _, adj := adjusted([]group{{lead, n + 1}}, party); adj >= hi {
					break
				}
				n++
				if _, adj := adjusted([]group{{lead, n}}, party); adj >= lo && c.chance(40) {
					break
				}
			}
			gs = []group{{lead, n}}
		case "leader":
			// Followers of a lesser kind that shares the leader's home.
			var mins []*Monster
			for _, m := range pool {
				if m.XP < lead.XP && m.XP*4 >= lead.XP/4 && sharesHome(m, lead) {
					mins = append(mins, m)
				}
			}
			if len(mins) == 0 {
				continue
			}
			f := mins[c.r.IntN(len(mins))]
			n := 1
			for n < 10 {
				if _, adj := adjusted([]group{{lead, 1}, {f, n + 1}}, party); adj >= hi {
					break
				}
				n++
				if _, adj := adjusted([]group{{lead, 1}, {f, n}}, party); adj >= lo && c.chance(40) {
					break
				}
			}
			gs = []group{{lead, 1}, {f, n}}
		}
		if consider(gs) {
			break
		}
	}

	raw, adj = adjusted(best, party)
	return best, raw, adj, rate(adj, party, level)
}

func runEncounter(c *ctx) Item {
	party := c.optInt("party", 4, 1, 12)
	level := c.optInt("level", 1, 1, 20)
	env := c.opt("environment")
	best, raw, adj, actual := c.encounter(party, level, c.opt("difficulty"), func(m *Monster) bool { return m.livesIn(env) })
	if best == nil {
		return Item{Title: "No encounter", Summary: "Nothing in that environment fits a party that size and level.",
			Body: "Nothing in the monster list lives there at that level. Try another environment.\n"}
	}
	rows := [][]string{}
	names := []string{}
	for _, g := range best {
		rows = append(rows, []string{fmt.Sprint(g.n), g.m.Name, g.m.CR, commas(g.m.XP * g.n),
			fmt.Sprint(g.m.AC), fmt.Sprint(g.m.HP)})
		if g.n > 1 {
			names = append(names, fmt.Sprintf("%d × %s", g.n, g.m.Name))
		} else {
			names = append(names, g.m.Name)
		}
	}
	where := env
	if where == "" || where == "any" {
		where = c.pick(best[0].m.Environments)
	}
	activity := c.pick(encounterActivities)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s encounter for %s of level %d, %s.\n\n",
		capital(actual), plural(party, "character", "characters"), level, envLabel(where)))
	b.WriteString(orgTable([]string{"#", "Monster", "CR", "XP", "AC", "HP"}, rows))
	b.WriteString("\n")
	b.WriteString(fields(
		"XP", fmt.Sprintf("%s (%s adjusted for the crowd)", commas(raw), commas(adj)),
		"Each character earns", commas(raw/party)+" XP",
		"Doing", activity,
		"Distance", fmt.Sprintf("%d ft.", 10*c.between(3, 12)),
	))
	return Item{
		Title:   strings.Join(names, " and "),
		Summary: fmt.Sprintf("%s · %s XP · %s", capital(actual), commas(adj), activity),
		Body:    b.String(),
		Tags:    []string{"encounter"},
		Props: map[string]string{
			"DIFFICULTY":  actual,
			"PARTY_LEVEL": fmt.Sprint(level),
			"PARTY_SIZE":  fmt.Sprint(party),
			"XP":          fmt.Sprint(raw),
			"ENVIRONMENT": where,
		},
	}
}

func sharesHome(a, b *Monster) bool {
	for _, x := range a.Environments {
		for _, y := range b.Environments {
			if x == y {
				return true
			}
		}
	}
	return false
}

var envLabels = map[string]string{
	"arctic": "in the arctic", "coastal": "on the coast", "desert": "in the desert",
	"forest": "in a forest", "grassland": "on the grassland", "hill": "in the hills",
	"mountain": "in the mountains", "swamp": "in a swamp", "underdark": "in the underdark",
	"underwater": "underwater", "urban": "in a town", "planar": "on another plane",
}

func envLabel(e string) string {
	if l, ok := envLabels[e]; ok {
		return l
	}
	return "anywhere"
}

func environmentChoices() []Choice {
	out := []Choice{{Value: "any", Label: "Any environment"}}
	keys := make([]string, 0, len(envLabels))
	for k := range envLabels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, Choice{Value: k, Label: title(k)})
	}
	return out
}
