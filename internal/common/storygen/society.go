package storygen

import (
	"fmt"
	"strings"
)

// Cultures, faiths and organisations. The tables are in data/society.txt.

// ---- cultures --------------------------------------------------------------

func runCulture(c *ctx) Item {
	l := c.newLanguage()
	people := c.name(l)
	home := c.t("culture_homes")
	vs := c.tN("values", 2)
	taboo := c.t("taboos")
	greet := c.t("greetings")
	food := c.t("foods")
	clothes := c.t("dress")
	age := c.t("coming_of_age")
	marry := c.t("marriages")
	death := c.t("death_ways")
	art := c.t("arts")
	feast := c.t("festivals")
	wrong := c.t("misread")
	saying := c.t("sayings")
	names := []string{}
	for range 6 {
		names = append(names, c.name(l))
	}
	if c.genre == "modern" {
		// A contemporary community is named for where it is, not invented.
		people = "The people of " + c.placeName(nil)
		names = nil
	}
	return Item{
		Title:   people,
		Summary: "People of " + article(home) + ", who prize " + andList(vs),
		Entry:   "group",
		Facts: facts(
			"Home", capital(home),
			"Prize", capital(andList(vs)),
			"Taboo", capital(taboo),
			"Greeting", capital(greet),
			"Food", capital(food),
			"Dress", capital(clothes),
			"Coming of age", capital(age),
			"Marriage", "They marry "+marry,
			"The dead", capital(death),
			"Art", capital(art),
			"Festival", capital(feast),
			"Outsiders mistake", capital(wrong),
			"Saying", saying,
			"Names", strings.Join(names, ", "),
		),
		Prose: prose(
			fmt.Sprintf("%s, people of %s, prize %s", people, article(home), andList(vs)),
			"They greet each other as they "+greet+" and never permit "+taboo,
			"They grow up through "+age+", marry "+marry+", and their dead are "+death,
			"Their great festival is "+feast,
			"Outsiders mistake "+wrong,
			"As they say: "+saying,
		),
		Tags: []string{"culture"},
	}
}

// ---- religions -------------------------------------------------------------

func runReligion(c *ctx) Item {
	l := c.newLanguage()
	shape := c.t("divine_shapes")
	thing := c.t("holy")
	god := c.name(l)
	faith := "The " + title(c.pick([]string{"way", "faith", "church", "covenant", "path", "assembly", "order", "witness", "communion", "lantern"})) + " of " + god
	if c.chance(35) {
		faith = c.name(l) + "ism"
	}
	practice := c.t("practices")
	priests := c.t("clergy")
	sym := c.t("symbols")
	law := c.t("commandments")
	after := c.t("afterlives")
	place := c.t("holy_places")
	sin := c.t("sins")
	split := c.t("schisms")
	return Item{
		Title:   faith,
		Summary: "Worship of " + shape + "; " + thing + " is holy",
		Entry:   "group",
		Facts: facts(
			"Worship", capital(shape),
			"Holy", capital(thing),
			"Practice", capital(practice),
			"Clergy", capital(priests),
			"Symbol", capital(sym),
			"Commandment", law,
			"Worst sin", capital(sin),
			"The dead", "They believe the dead "+after,
			"Holiest place", capital(place),
			"Split over", capital(split),
		),
		Prose: prose(
			fmt.Sprintf("%s worships %s, and holds %s holy", faith, shape, thing),
			"The faithful keep "+practice+"; their symbol is "+sym+", their clergy "+priests,
			"First law: "+law,
			"The worst sin is "+sin+", and they believe the dead "+after,
			"Their holiest place is "+place+"; they are divided over "+split,
		),
		Tags: []string{"religion"},
	}
}

// ---- organisations ---------------------------------------------------------

func runOrganisation(c *ctx) Item {
	kind := c.t("org_kinds")
	name := "The " + c.t("org_name_parts") + " " + c.t("org_name_nouns")
	if c.genre == "modern" && c.chance(35) {
		name = c.pick(tab("surnames").all()) + " " + c.t("org_name_nouns")
	}
	leader := c.personName(nil, "")
	trait := c.t("manners")
	public := c.t("public_aims")
	real := c.t("real_aims")
	how := c.t("methods")
	sym := c.t("symbols")
	join := c.t("recruits")
	rift := c.t("rifts")
	asset := c.t("org_assets")
	return Item{
		Title:   name,
		Summary: capital(article(kind)) + " that says it exists " + public,
		Entry:   "group",
		Facts: facts(
			"Kind", capital(kind),
			"Says it exists", capital(public),
			"Really wants", capital(real),
			"Works by", capital(how),
			"Led by", leader+", who "+trait,
			"Symbol", capital(sym),
			"Recruits", capital(join),
			"Has", capital(asset),
			"Rift", capital(rift),
		),
		Prose: prose(
			fmt.Sprintf("%s, %s that says it exists %s; really, %s", name, article(kind), public, real),
			"They "+how,
			"Led by "+leader+", who "+trait+"; they recruit "+join+" and have "+asset,
			"Inside: "+rift,
		),
		Tags: []string{"organisation"},
	}
}
