package randgen

import (
	"fmt"
	"strings"
)

// ---- names -----------------------------------------------------------------

// A name is built from syllables of a family's sound, so it is new every time
// and still sounds like it comes from somewhere. A race is mapped onto one of
// the families by name; anything unknown sounds human.
type nameSounds struct {
	start, mid, end []string
	// family names, built the same way or taken from a list
	surStart, surEnd []string
}

func familyFor(race string) string {
	r := strings.ToLower(race)
	switch {
	case strings.Contains(r, "half-elf"):
		return c2("human", "elf")
	case strings.Contains(r, "elf") || strings.Contains(r, "eladrin"):
		return "elf"
	case strings.Contains(r, "dwarf") || strings.Contains(r, "duergar"):
		return "dwarf"
	case strings.Contains(r, "halfling"):
		return "halfling"
	case strings.Contains(r, "gnome"):
		return "gnome"
	case strings.Contains(r, "orc") || strings.Contains(r, "goliath"):
		return "orc"
	case strings.Contains(r, "dragonborn"):
		return "dragon"
	case strings.Contains(r, "tiefling"):
		return "infernal"
	}
	return "human"
}

// c2 is a fixed pick between two families, used where a race sits between
// them; the caller's randomness decides later which one is spoken.
func c2(a, b string) string { return a + "|" + b }

func (c *ctx) syllableName(s nameSounds, parts int) string {
	var b strings.Builder
	b.WriteString(c.pick(s.start))
	for i := 2; i < parts; i++ {
		b.WriteString(c.pick(s.mid))
	}
	b.WriteString(c.pick(s.end))
	return capital(b.String())
}

// npcName gives a full name for a race, or for a random family when race is
// empty.
func (c *ctx) npcName(race string) string {
	fam := ""
	if race == "" {
		fam = c.pick([]string{"human", "human", "human", "elf", "dwarf", "halfling", "gnome", "orc", "dragon", "infernal"})
	} else {
		fam = familyFor(race)
	}
	if strings.Contains(fam, "|") {
		fam = c.pick(strings.Split(fam, "|"))
	}
	s := sounds[fam]
	given := c.syllableName(s, c.between(2, 3))
	if fam == "infernal" && c.chance(35) {
		return capital(c.pick(virtueNames))
	}
	if len(s.surStart) == 0 {
		return given
	}
	sur := capital(c.pick(s.surStart) + c.pick(s.surEnd))
	return given + " " + sur
}

func runNames(c *ctx) Item {
	race := c.opt("race")
	if race == "any" {
		race = ""
	}
	n := c.npcName(race)
	return Item{Title: n, Summary: raceOr(race, "any people"), Body: "", Tags: []string{"name"}}
}

func raceOr(race, def string) string {
	if race == "" {
		return def
	}
	return race
}

// ---- NPCs ------------------------------------------------------------------

func (c *ctx) raceNames() []string {
	if c.rs == nil || len(c.rs.Races) == 0 {
		return []string{"Human", "Elf", "Dwarf", "Halfling", "Gnome", "Half-Orc", "Dragonborn", "Tiefling", "Half-Elf"}
	}
	out := make([]string, 0, len(c.rs.Races))
	for _, r := range c.rs.Races {
		out = append(out, r.Name)
	}
	return out
}

func raceChoices(names []string) []Choice {
	out := []Choice{{Value: "any", Label: "Any race"}}
	for _, n := range names {
		out = append(out, Choice{Value: n, Label: n})
	}
	return out
}

func (c *ctx) npc() (name, race string, body string, summary string) {
	race = c.opt("race")
	if race == "" || race == "any" {
		// Humans are most of any crowd; the rest are an even spread.
		races := c.raceNames()
		if c.chance(40) {
			race = "Human"
			found := false
			for _, r := range races {
				if r == "Human" {
					found = true
				}
			}
			if !found {
				race = c.pick(races)
			}
		} else {
			race = c.pick(races)
		}
	}
	name = c.npcName(race)
	gender := c.weighted([]string{"man", "woman", "person"}, []int{45, 45, 10})
	age := c.pick(npcAges)
	job := c.opt("occupation")
	if job == "" || job == "any" {
		job = c.pick(occupations)
	} else {
		job = c.pick(occupationsBy[job])
	}
	align := c.weighted(
		[]string{"lawful good", "neutral good", "chaotic good", "lawful neutral", "neutral",
			"chaotic neutral", "lawful evil", "neutral evil", "chaotic evil"},
		[]int{8, 12, 8, 10, 20, 10, 5, 4, 3})
	summary = fmt.Sprintf("%s %s %s, %s", capital(age), strings.ToLower(race), gender, job)
	body = fields(
		"Race", race,
		"Occupation", job,
		"Age", age,
		"Appearance", c.pick(appearances),
		"Voice", c.pick(voices),
		"Trait", c.pick(traits),
		"Ideal", c.pick(ideals),
		"Bond", c.pick(bonds),
		"Flaw", c.pick(flaws),
		"Wants", c.pick(wants),
		"Knows", c.pick(secrets),
		"Alignment", align,
		"Attitude", c.weighted([]string{"hostile", "wary", "indifferent", "friendly", "helpful"}, []int{1, 3, 5, 4, 2}),
	)
	return
}

func runNpc(c *ctx) Item {
	name, race, body, summary := c.npc()
	return Item{
		Title:   name,
		Summary: summary,
		Body:    body,
		Tags:    []string{"npc"},
		Props:   map[string]string{"RACE": race},
	}
}

// ---- taverns ---------------------------------------------------------------

func (c *ctx) tavernName() string {
	switch c.between(1, 4) {
	case 1:
		return "The " + capital(c.pick(tavernAdjectives)) + " " + capital(c.pick(tavernNouns))
	case 2:
		return "The " + capital(c.pick(tavernNouns)) + " and " + capital(c.pick(tavernNouns))
	case 3:
		return "The " + capital(c.pick(tavernNouns)) + "'s " + capital(c.pick(tavernPlaces))
	}
	return capital(c.pick(tavernAdjectives)) + " " + capital(c.pick(tavernNouns)) + " " + capital(c.pick(tavernPlaces))
}

func runTavern(c *ctx) Item {
	name := c.tavernName()
	keeper := c.npcName("")
	quality := c.weighted([]string{"squalid", "poor", "modest", "comfortable", "wealthy"}, []int{1, 3, 5, 3, 1})
	prices := map[string]string{
		"squalid": "1 cp a cot, 3 cp a meal", "poor": "1 sp a room, 6 cp a meal",
		"modest": "5 sp a room, 3 sp a meal", "comfortable": "8 sp a room, 5 sp a meal",
		"wealthy": "2 gp a room, 8 sp a meal",
	}
	heard := c.pickN(rumours, 2)
	var b strings.Builder
	b.WriteString(fields(
		"Quality", quality+" ("+prices[quality]+")",
		"Keeper", fmt.Sprintf("%s, %s", keeper, c.pick(traits)),
		"Known for", c.pick(tavernSpecials),
		"Drink", c.pick(tavernDrinks),
		"Inside", c.pick(tavernFeatures),
		"Tonight", c.pick(tavernTonight),
	))
	b.WriteString("\nRumours:\n")
	for _, r := range heard {
		b.WriteString("- " + r + "\n")
	}
	return Item{
		Title:   name,
		Summary: fmt.Sprintf("%s inn kept by %s", capital(quality), keeper),
		Body:    b.String(),
		Tags:    []string{"tavern"},
	}
}

// ---- traps -----------------------------------------------------------------

// Save DCs and attack bonuses by danger, and damage by level band and danger:
// the fifth edition trap tables.
var trapDC = map[string][2]int{"setback": {10, 11}, "dangerous": {12, 15}, "deadly": {16, 20}}
var trapAttack = map[string][2]int{"setback": {3, 5}, "dangerous": {6, 8}, "deadly": {9, 12}}
var trapDamage = [4]map[string]string{
	{"setback": "1d10", "dangerous": "2d10", "deadly": "4d10"},
	{"setback": "2d10", "dangerous": "4d10", "deadly": "10d10"},
	{"setback": "4d10", "dangerous": "10d10", "deadly": "18d10"},
	{"setback": "10d10", "dangerous": "18d10", "deadly": "24d10"},
}

func levelBand(level int) int {
	switch {
	case level <= 4:
		return 0
	case level <= 10:
		return 1
	case level <= 16:
		return 2
	}
	return 3
}

// trapSpec is a trap as its parts: what it is called, a one line summary, and
// the label/value lines that describe it.
type trapSpec struct {
	title, summary, danger string
	lines                  []DLine
}

func (c *ctx) trap(level int, danger, kind string) trapSpec {
	if danger == "" || danger == "any" {
		danger = c.weighted([]string{"setback", "dangerous", "deadly"}, []int{5, 4, 1})
	}
	if kind == "" || kind == "any" {
		kind = c.pick([]string{"mechanical", "magical"})
	}
	var effects []trapEffect
	for _, e := range trapEffects {
		if e.kind == kind {
			effects = append(effects, e)
		}
	}
	e := effects[c.r.IntN(len(effects))]
	trigger := c.pick(trapTriggers[kind])
	where := c.pick(trapPlaces)
	dc := c.between(trapDC[danger][0], trapDC[danger][1])
	atk := c.between(trapAttack[danger][0], trapAttack[danger][1])
	dmg := trapDamage[levelBand(level)][danger]
	spot := c.between(10, 10+(dc-8))
	disarm := c.between(dc-2, dc+2)

	resolve := ""
	if e.attack {
		resolve = fmt.Sprintf("+%d to hit, %s %s damage on a hit", atk, dmg, e.damage)
	} else {
		resolve = fmt.Sprintf("DC %d %s save, %s %s damage on a failure, half on a success", dc, e.save, dmg, e.damage)
		if e.noHalf {
			resolve = fmt.Sprintf("DC %d %s save, %s %s damage on a failure", dc, e.save, dmg, e.damage)
		}
	}
	if e.extra != "" {
		resolve += "; " + e.extra
	}
	check := "thieves' tools"
	if kind == "magical" {
		check = "Intelligence (Arcana) or dispel magic"
	}
	return trapSpec{
		title:   capital(e.name),
		summary: fmt.Sprintf("%s %s trap · DC %d · %s", capital(danger), kind, dc, dmg+" "+e.damage),
		danger:  danger,
		lines: []DLine{
			{"Kind", fmt.Sprintf("%s trap, %s for levels %s", kind, danger, levelBandLabel(level))},
			{"Where", where},
			{"Trigger", trigger},
			{"Effect", e.text},
			{"Resolve", resolve},
			{"Spot", fmt.Sprintf("DC %d Wisdom (Perception): %s", spot, c.pick(trapTells[kind]))},
			{"Disarm", fmt.Sprintf("DC %d using %s", disarm, check)},
			{"Bypass", c.pick(trapBypasses)},
		},
	}
}

func runTrap(c *ctx) Item {
	level := c.optInt("level", 1, 1, 20)
	t := c.trap(level, c.opt("danger"), c.opt("kind"))
	return Item{
		Title:   t.title,
		Summary: t.summary,
		Body:    dfields(t.lines),
		Tags:    []string{"trap"},
		Props:   map[string]string{"DANGER": t.danger, "PARTY_LEVEL": fmt.Sprint(level)},
	}
}

func levelBandLabel(level int) string {
	return []string{"1-4", "5-10", "11-16", "17-20"}[levelBand(level)]
}

// ---- small tables ----------------------------------------------------------

func listGen(kind string, list []string, tags ...string) func(c *ctx) Item {
	return func(c *ctx) Item {
		t := c.pick(list)
		return Item{Title: capital(t), Summary: "", Body: "", Tags: append([]string{kind}, tags...)}
	}
}

func runSurge(c *ctx) Item {
	i := c.r.IntN(len(surges))
	return Item{
		Title:   surges[i],
		Summary: fmt.Sprintf("Wild magic surge %d of %d", i+1, len(surges)),
		Tags:    []string{"wildmagic"},
	}
}

func runCarousing(c *ctx) Item {
	e := c.pick(carousing)
	who := c.npcName("")
	e = strings.ReplaceAll(e, "{who}", who)
	cost := c.roll(1, 6) * []int{1, 5, 10}[c.r.IntN(3)]
	return Item{
		Title:   e,
		Summary: fmt.Sprintf("A night out, %d gp spent", cost),
		Body:    fields("Spent", fmt.Sprintf("%d gp", cost), "Check", c.pick(carousingChecks)),
		Tags:    []string{"carousing"},
	}
}

func runHook(c *ctx) Item {
	patron := c.npcName("")
	h := c.pick(hooks)
	h = strings.ReplaceAll(h, "{who}", patron)
	h = strings.ReplaceAll(h, "{place}", c.pick(hookPlaces))
	h = strings.ReplaceAll(h, "{thing}", c.pick(hookThings))
	return Item{
		Title:   h,
		Summary: "Adventure hook",
		Body: fields(
			"Reward", c.pick(hookRewards),
			"Twist", c.pick(hookTwists),
		),
		Tags: []string{"hook"},
	}
}
