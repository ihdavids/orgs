package randgen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/internal/common/dnd"
)

// ---- gems and art objects --------------------------------------------------

var gemValues = []int{10, 50, 100, 500, 1000, 5000}

func (c *ctx) gem(value int) string {
	list := gems[value]
	return c.pick(list)
}

// artObject builds an object out of a material, a thing and a detail, priced
// by tier. Richer tiers draw from richer materials.
func (c *ctx) artObject(value int) string {
	tier := 0
	for i, v := range artValues {
		if v == value {
			tier = i
		}
	}
	mat := c.pick(artMaterials[tier])
	thing := c.pick(artThings)
	detail := c.pick(artDetails[tier])
	return fmt.Sprintf("%s %s %s", capital(article(mat)), thing, detail)
}

var artValues = []int{25, 250, 750, 2500, 7500}

func runGems(c *ctx) Item {
	value := c.optInt("value", 0, 0, 5000)
	if value == 0 {
		value = c.weightedNum(intStrings(gemValues), []int{6, 6, 5, 3, 2, 1}).toInt()
	}
	g := c.gem(value)
	return Item{
		Title:   capital(g),
		Summary: commas(value) + " gp gemstone",
		Body:    fields("Value", commas(value)+" gp", "Look", c.pick(gemLooks)),
		Tags:    []string{"treasure", "gem"},
		Props:   map[string]string{"VALUE": fmt.Sprintf("%d gp", value)},
	}
}

func runArt(c *ctx) Item {
	value := c.optInt("value", 0, 0, 7500)
	if value == 0 {
		value = c.weightedNum(intStrings(artValues), []int{6, 5, 3, 2, 1}).toInt()
	}
	a := c.artObject(value)
	return Item{
		Title:   a,
		Summary: commas(value) + " gp art object",
		Body:    fields("Value", commas(value)+" gp", "Condition", c.pick(artConditions)),
		Tags:    []string{"treasure", "art"},
		Props:   map[string]string{"VALUE": fmt.Sprintf("%d gp", value)},
	}
}

type numStr string

func (s numStr) toInt() int {
	n := 0
	fmt.Sscanf(string(s), "%d", &n)
	return n
}

func intStrings(xs []int) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = fmt.Sprint(x)
	}
	return out
}

func (c *ctx) weightedNum(keys []string, weights []int) numStr {
	return numStr(c.weighted(keys, weights))
}

// ---- magic items -----------------------------------------------------------

var rarities = []string{"common", "uncommon", "rare", "very rare", "legendary"}

func magicItems(rs *dnd.Ruleset, rarity, kind string) []*dnd.Item {
	if rs == nil {
		return nil
	}
	var out []*dnd.Item
	for i := range rs.Items {
		it := &rs.Items[i]
		if !it.IsMagic() || strings.EqualFold(it.Rarity, "artifact") {
			continue
		}
		if rarity != "" && rarity != "any" && !strings.EqualFold(it.Rarity, rarity) {
			continue
		}
		if kind != "" && kind != "any" && !itemIsKind(it, kind) {
			continue
		}
		out = append(out, it)
	}
	return out
}

func itemIsKind(it *dnd.Item, kind string) bool {
	k := strings.ToLower(it.Kind + " " + it.Category + " " + it.Name)
	switch kind {
	case "armor":
		return it.Kind == "armor" || it.Kind == "shield"
	case "weapon":
		return it.Kind == "weapon"
	case "potion":
		return strings.Contains(k, "potion") || strings.Contains(k, "oil") || strings.Contains(k, "elixir")
	case "scroll":
		return strings.Contains(k, "scroll")
	case "wondrous":
		return strings.Contains(k, "wondrous")
	default:
		return strings.Contains(k, kind)
	}
}

func (c *ctx) magicItem(rarity, kind string) *dnd.Item {
	list := magicItems(c.rs, rarity, kind)
	if len(list) == 0 {
		return nil
	}
	return list[c.r.IntN(len(list))]
}

func magicItemBody(it *dnd.Item) string {
	att := "no"
	if it.Attunement {
		att = "yes"
		if it.AttunementNote != "" {
			att = "yes, " + it.AttunementNote
		}
	}
	var b strings.Builder
	b.WriteString(fields(
		"Rarity", it.RarityName(),
		"Type", title(strings.ReplaceAll(it.Category, "-", " ")),
		"Attunement", att,
	))
	if t := strings.TrimSpace(it.Text); t != "" {
		b.WriteString("\n" + noHeadings(t) + "\n")
	}
	return b.String()
}

func runMagicItem(c *ctx) Item {
	rarity := c.opt("rarity")
	if rarity == "" || rarity == "any" {
		rarity = c.weighted(rarities, []int{5, 8, 5, 2, 1})
		if len(magicItems(c.rs, rarity, c.opt("kind"))) == 0 {
			rarity = "any"
		}
	}
	it := c.magicItem(rarity, c.opt("kind"))
	if it == nil {
		return Item{Title: "No magic item", Summary: "Nothing in the ruleset matches that.",
			Body: "The loaded ruleset has no magic item of that rarity and kind.\n"}
	}
	return Item{
		Title:   it.Name,
		Summary: it.RarityName() + " " + strings.ReplaceAll(it.Category, "-", " "),
		Body:    magicItemBody(it),
		Tags:    []string{"treasure", "magicitem"},
		Props:   map[string]string{"RARITY": it.Rarity, "DND_ITEM": it.Id},
	}
}

// ---- treasure --------------------------------------------------------------

// The four bands treasure is rolled in, by the challenge of whoever owned it.
func crBand(cr string) int {
	switch cr {
	case "5-10":
		return 1
	case "11-16":
		return 2
	case "17+":
		return 3
	}
	return 0
}

type coins map[string]int

func (cs coins) String() string {
	order := []string{"pp", "gp", "ep", "sp", "cp"}
	parts := []string{}
	for _, k := range order {
		if cs[k] > 0 {
			parts = append(parts, commas(cs[k])+" "+k)
		}
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, ", ")
}

func (cs coins) gp() int {
	return cs["pp"]*10 + cs["gp"] + cs["ep"]/2 + cs["sp"]/10 + cs["cp"]/100
}

// A loose purse: what a monster or a bandit carries. One coin kind leads,
// with small change in another, and both grow by band.
func (c *ctx) purse(band int) coins {
	cs := coins{}
	switch band {
	case 0:
		switch c.weighted([]string{"cp", "sp", "gp"}, []int{3, 4, 3}) {
		case "cp":
			cs["cp"] = c.roll(5, 6) * 10
		case "sp":
			cs["sp"] = c.roll(4, 6) * 10
			cs["cp"] = c.roll(3, 6) * 5
		default:
			cs["gp"] = c.roll(3, 6) * 3
			cs["sp"] = c.roll(2, 6) * 5
		}
	case 1:
		switch c.weighted([]string{"sp", "gp", "pp"}, []int{3, 5, 2}) {
		case "sp":
			cs["sp"] = c.roll(4, 6) * 100
			cs["gp"] = c.roll(1, 6) * 10
		case "gp":
			cs["gp"] = c.roll(4, 6) * 15
			cs["ep"] = c.roll(2, 6) * 10
		default:
			cs["pp"] = c.roll(3, 6) * 3
			cs["gp"] = c.roll(2, 6) * 10
		}
	case 2:
		switch c.weighted([]string{"gp", "pp"}, []int{6, 4}) {
		case "gp":
			cs["gp"] = c.roll(4, 6) * 120
			cs["ep"] = c.roll(1, 6) * 50
		default:
			cs["pp"] = c.roll(4, 6) * 15
			cs["gp"] = c.roll(2, 6) * 100
		}
	default:
		cs["pp"] = c.roll(5, 6) * 80
		cs["gp"] = c.roll(2, 6) * 1000
	}
	return cs
}

// A hoard: a lair's worth. Coins, then a chance of gems or art of one value,
// then magic items whose rarity climbs with the band.
func (c *ctx) hoard(band int) (coins, []string, []*dnd.Item) {
	cs := coins{}
	switch band {
	case 0:
		cs["cp"] = c.roll(6, 6) * 100
		cs["sp"] = c.roll(3, 6) * 100
		cs["gp"] = c.roll(2, 6) * 10
	case 1:
		cs["cp"] = c.roll(2, 6) * 100
		cs["sp"] = c.roll(2, 6) * 1000
		cs["gp"] = c.roll(6, 6) * 100
		cs["pp"] = c.roll(3, 6) * 10
	case 2:
		cs["gp"] = c.roll(4, 6) * 1000
		cs["pp"] = c.roll(5, 6) * 100
	default:
		cs["gp"] = c.roll(12, 6) * 1000
		cs["pp"] = c.roll(8, 6) * 1000
	}

	valuables := []string{}
	if c.chance(75) {
		n := c.roll(2, 4)
		if c.chance(50) {
			v := []int{10, 50, 100, 1000}[band]
			if c.chance(40) {
				v = []int{50, 100, 500, 5000}[band]
			}
			for i := 0; i < n; i++ {
				valuables = append(valuables, fmt.Sprintf("%s (%s gp gem)", c.gem(v), commas(v)))
			}
		} else {
			v := []int{25, 250, 750, 2500}[band]
			if c.chance(40) {
				v = []int{25, 250, 2500, 7500}[band]
			}
			for i := 0; i < n && i < 6; i++ {
				valuables = append(valuables, fmt.Sprintf("%s (%s gp art)", c.artObject(v), commas(v)))
			}
		}
	}

	weights := [][]int{
		{6, 4, 1, 0, 0},
		{3, 6, 4, 1, 0},
		{0, 3, 6, 4, 1},
		{0, 0, 3, 6, 4},
	}[band]
	items := []*dnd.Item{}
	if c.chance([]int{35, 50, 65, 80}[band]) {
		n := c.between(1, []int{3, 4, 4, 5}[band])
		for i := 0; i < n; i++ {
			if it := c.magicItem(c.weighted(rarities, weights), "any"); it != nil {
				items = append(items, it)
			}
		}
	}
	return cs, valuables, items
}

func runTreasure(c *ctx) Item {
	band := crBand(c.opt("cr"))
	kind := c.opt("kind")
	if kind == "hoard" {
		cs, vals, items := c.hoard(band)
		total := cs.gp()
		var b strings.Builder
		b.WriteString(fields("Coins", cs.String(), "Found", c.pick(hoardPlaces)))
		if len(vals) > 0 {
			b.WriteString("\nValuables:\n")
			for _, v := range vals {
				b.WriteString("- " + v + "\n")
			}
		}
		if len(items) > 0 {
			b.WriteString("\nMagic items:\n")
			for _, it := range items {
				att := ""
				if it.Attunement {
					att = ", attunement"
				}
				b.WriteString(fmt.Sprintf("- %s (%s%s)\n", it.Name, strings.ToLower(it.RarityName()), att))
			}
		}
		for _, v := range vals {
			var n int
			if i := strings.LastIndex(v, "("); i >= 0 {
				fmt.Sscanf(strings.ReplaceAll(v[i+1:], ",", ""), "%d", &n)
			}
			total += n
		}
		name := "Hoard"
		if len(items) > 0 {
			name = "Hoard with " + items[0].Name
			if len(items) > 1 {
				name += fmt.Sprintf(" and %d more", len(items)-1)
			}
		}
		return Item{
			Title:   name,
			Summary: fmt.Sprintf("about %s gp before magic · %s", commas(total), magicCount(len(items))),
			Body:    b.String(),
			Tags:    []string{"treasure", "hoard"},
			Props:   map[string]string{"CR": c.opt("cr"), "VALUE": fmt.Sprintf("%d gp", total)},
		}
	}
	cs := c.purse(band)
	extra := ""
	if c.chance(20) {
		extra = c.pick(pocketThings)
	}
	return Item{
		Title:   cs.String(),
		Summary: fmt.Sprintf("worth %s gp", commas(cs.gp())),
		Body:    fields("Coins", cs.String(), "Carried in", c.pick(purseKinds), "Also", extra),
		Tags:    []string{"treasure"},
		Props:   map[string]string{"CR": c.opt("cr"), "VALUE": fmt.Sprintf("%d gp", cs.gp())},
	}
}

// ---- spellbooks ------------------------------------------------------------

func runSpellbook(c *ctx) Item {
	level := c.optInt("level", 1, 1, 20)
	class := c.opt("class")
	if class == "" {
		class = "wizard"
	}
	if c.rs == nil {
		return Item{Title: "No spellbook", Body: "No ruleset is loaded.\n"}
	}
	top := (level + 1) / 2
	if top > 9 {
		top = 9
	}
	byLevel := map[int][]*dnd.Spell{}
	for i := range c.rs.Spells {
		sp := &c.rs.Spells[i]
		if sp.Level < 0 || sp.Level > top || !hasClass(sp, class) {
			continue
		}
		byLevel[sp.Level] = append(byLevel[sp.Level], sp)
	}
	// A wizard starts with six spells of first level and copies two of any
	// level they can cast at every level after, plus a few found on the road.
	// Cantrips are known, not written, but a book that names them is handy.
	want := map[int]int{}
	want[1] = 6
	for l := 2; l <= level; l++ {
		// One of the two is nearly always the newest level they can cast:
		// that is what a level-up is for.
		want[(l+1)/2]++
		want[c.between(1, (l+1)/2)]++
	}
	found := c.between(0, level/3)
	for k := 0; k < found; k++ {
		want[c.between(1, top)]++
	}
	cantrips := 3
	if level >= 4 {
		cantrips++
	}
	if level >= 10 {
		cantrips++
	}
	want[0] = cantrips

	owner := c.npcName("")
	rows := [][]string{}
	total := 0
	for l := 0; l <= top; l++ {
		pool := byLevel[l]
		sort.Slice(pool, func(i, j int) bool { return pool[i].Name < pool[j].Name })
		names := make([]string, len(pool))
		for i, sp := range pool {
			names[i] = sp.Name
		}
		got := c.pickN(names, want[l])
		sort.Strings(got)
		if len(got) == 0 {
			continue
		}
		total += len(got)
		lname := "Cantrips"
		if l > 0 {
			lname = ordinal(l)
		}
		rows = append(rows, []string{lname, strings.Join(got, ", ")})
	}
	var b strings.Builder
	b.WriteString(fields(
		"Owner", fmt.Sprintf("%s, a level %d %s", owner, level, class),
		"Binding", c.pick(bookBindings),
		"Quirk", c.pick(bookQuirks),
	))
	b.WriteString("\n")
	b.WriteString(orgTable([]string{"Level", "Spells"}, rows))
	return Item{
		Title:   fmt.Sprintf("Spellbook of %s", owner),
		Summary: fmt.Sprintf("%s level %d · %s", title(class), level, plural(total, "spell", "spells")),
		Body:    b.String(),
		Tags:    []string{"spellbook"},
		Props:   map[string]string{"CASTER_LEVEL": fmt.Sprint(level), "CLASS": class},
	}
}

func hasClass(sp *dnd.Spell, class string) bool {
	for _, c := range sp.Classes {
		if strings.EqualFold(c, class) {
			return true
		}
	}
	return false
}

func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

func magicCount(n int) string {
	if n == 0 {
		return "no magic items"
	}
	return plural(n, "magic item", "magic items")
}
