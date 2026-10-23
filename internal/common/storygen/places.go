package storygen

import "fmt"

// Places. The tables are in data/places.txt.

// ---- settlements -----------------------------------------------------------

// popOf is a head count for a settlement's size tag.
func popOf(c *ctx, size string) string {
	switch size {
	case "tiny":
		return fmt.Sprintf("about %d", c.between(2, 9)*10)
	case "small":
		return fmt.Sprintf("about %d", c.between(2, 9)*100)
	case "large":
		return fmt.Sprintf("about %d,000", c.between(20, 90))
	case "huge":
		return fmt.Sprintf("about %d,000", c.between(100, 900))
	}
	return fmt.Sprintf("about %d,000", c.between(2, 15))
}

func runSettlement(c *ctx) Item {
	l := c.newLanguage()
	name := c.placeName(l)
	kind, size := split2(c.t("settlement_kinds"))
	known := c.t("known_for")
	ruler := c.t("run_by")
	trade := c.t("industries")
	mark := c.t("landmarks")
	trouble := c.t("troubles")
	senses := c.t("senses")
	custom := c.t("customs")
	rumour := c.t("rumours")
	return Item{
		Title:   name,
		Summary: capital(article(kind)) + " known for " + known,
		Entry:   "place",
		Facts: facts(
			"Kind", capital(kind),
			"People", popOf(c, size),
			"Known for", capital(known),
			"Lives on", capital(trade),
			"Run by", capital(ruler),
			"Landmark", capital(mark),
			"Smells and sounds", capital(senses),
			"Custom", capital(custom),
			"Trouble", capital(trouble),
			"Rumour", capital(rumour),
		),
		Prose: prose(
			fmt.Sprintf("%s, %s known for %s, run by %s", name, article(kind), known, ruler),
			"It lives on "+trade+"; it smells and sounds of "+senses+", and "+mark+" stands over it",
			"Here "+custom,
			"Lately "+trouble+", and people say "+rumour,
		),
		Tags: []string{"settlement"},
	}
}

// ---- a place for a scene ---------------------------------------------------

func runPlace(c *ctx) Item {
	spot := c.t("scene_spots")
	see := c.t("sights")
	hear := c.t("sounds")
	smell := c.t("smells")
	feel := c.t("touches")
	odd := c.t("odd_details")
	wrong := c.t("go_wrong")
	return Item{
		Title:   capital(spot),
		Summary: "A place for a scene, and what's in the air",
		Facts: facts(
			"Where", capital(spot),
			"See", capital(see),
			"Hear", capital(hear),
			"Smell", capital(smell),
			"Feel", capital(feel),
			"Odd", capital(odd),
			"Could go wrong", capital(wrong),
		),
		Prose: prose(
			capital(spot)+": "+see+"; "+hear,
			"It smells of "+smell+", "+feel,
			"Something is off: "+odd,
		),
		Tags: []string{"place"},
	}
}

// ---- taverns ---------------------------------------------------------------

// tavernName is a sign's name, or for a bar of now or the future sometimes a
// name of its own.
func (c *ctx) tavernName() string {
	if (c.genre == "sf" && c.chance(70)) || (c.genre == "modern" && c.chance(35)) {
		return c.t("bar_names")
	}
	adj := func() string { return c.pick(tab("tavern_adjectives").all()) }
	noun := func() string { return c.pick(tab("tavern_nouns").all()) }
	switch c.r.IntN(4) {
	case 0:
		return "The " + noun() + " and " + noun()
	case 1:
		return "The " + noun() + "'s " + c.pick(tab("tavern_rests").all())
	}
	return "The " + adj() + " " + noun()
}

func runTavern(c *ctx) Item {
	name := c.tavernName()
	kind, quality := split2(c.t("tavern_kinds"))
	keeper := c.personName(nil, "")
	manner := c.t("manners")
	drink := c.t("drinks")
	dish := c.t("dishes")
	inside := c.t("tavern_features")
	who := c.t("patrons")
	tonight := c.t("tonight")
	rule := c.t("house_rules")
	heard := c.tN("rumours", 2)
	return Item{
		Title:   name,
		Summary: fmt.Sprintf("%s %s kept by %s", capital(quality), kind, keeper),
		Entry:   "place",
		Facts: facts(
			"Kind", capital(kind),
			"Quality", capital(quality),
			"Keeper", keeper+", who "+manner,
			"Pours", capital(drink),
			"Known for", capital(dish),
			"Inside", capital(inside),
			"Drinking here", capital(who),
			"Tonight", capital(tonight),
			"House rule", rule,
			"Rumours", capital(heard[0])+"; "+heard[len(heard)-1],
		),
		Prose: prose(
			fmt.Sprintf("%s, %s %s kept by %s, who %s", name, article(quality), kind, keeper, manner),
			"They pour "+drink+" and are known for "+dish,
			"Inside: "+inside+"; drinking here, "+who,
			"Tonight "+tonight,
			"The talk is that "+heard[0],
		),
		Tags: []string{"tavern"},
	}
}
