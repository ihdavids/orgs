package storygen

import (
	"fmt"
	"strings"
)

// People. The tables are in data/people.txt and data/names.txt.

// ---- characters ------------------------------------------------------------

func ageFor(c *ctx) int {
	return c.pickInt([]int{c.between(15, 24), c.between(25, 39), c.between(25, 39), c.between(40, 59), c.between(40, 59), c.between(60, 90)})
}

// pickInt is pick for numbers.
func (c *ctx) pickInt(xs []int) int { return xs[c.r.IntN(len(xs))] }

func runCharacter(c *ctx) Item {
	name := c.personName(nil, c.opt("gender"))
	age := ageFor(c)
	job := c.t("occupations")
	build := c.t("builds")
	look := c.tN("looks", 2)
	manner := c.t("manners")
	voice := c.t("voices")
	habit := c.t("habits")
	wants := tab("wants")
	want := c.pick(append(append([]string(nil), wants[""]...), wants[c.opt("role")]...))
	fear := c.t("fears")
	good := c.t("strengths")
	flaw := c.t("flaws")
	secret := c.t("secrets")
	return Item{
		Title:   name,
		Summary: fmt.Sprintf("%d, %s", age, job),
		Entry:   "character",
		Facts: facts(
			"Age", fmt.Sprint(age),
			"Occupation", job,
			"Build", capital(build),
			"Look", capital(andList(look)),
			"Manner", capital(strings.TrimPrefix(manner, "is ")),
			"Voice", "Speaks "+voice,
			"Habit", capital(habit),
			"Wants", want,
			"Fears", fear,
			"Strength", good,
			"Flaw", flaw,
			"Secret", capital(secret),
		),
		Prose: prose(
			fmt.Sprintf("%s, %d, %s", name, age, article(job)),
			capital(build)+", with "+andList(look),
			firstName(name)+" "+manner+", and speaks "+voice,
			"Habit: "+habit,
			"Wants "+want+"; afraid of "+fear,
			capital(good)+", but "+flaw,
			"Secret: "+secret,
		),
		Tags: []string{"character"},
	}
}

// ---- names -----------------------------------------------------------------

func runNames(c *ctx) Item {
	// One language for the whole result, so the list reads as one people's.
	l := c.newLanguage()
	names := []string{}
	for range 10 {
		names = append(names, c.personName(l, c.opt("gender")))
	}
	places := []string{}
	for range 5 {
		places = append(places, c.placeName(l))
	}
	firsts := []string{}
	for _, n := range names[:3] {
		firsts = append(firsts, firstName(n))
	}
	return Item{
		Title:   strings.Join(firsts, ", ") + "…",
		Summary: "Names that sound like they belong together",
		Facts: facts(
			"People", strings.Join(names, ", "),
			"Places", strings.Join(places, ", "),
		),
		Prose: prose("Names: "+andList(names), "Places: "+andList(places)),
		Tags:  []string{"names"},
	}
}

// ---- relationships ---------------------------------------------------------

func runRelationship(c *ctx) Item {
	l := c.newLanguage()
	a := c.personName(l, "")
	b := c.personName(l, "")
	bond := c.t("bonds")
	tie := c.t("ties")
	strain := c.t("strains")
	say := c.t("unsaid")
	return Item{
		Title:   firstName(a) + " and " + firstName(b),
		Summary: capital(bond),
		Facts: facts(
			"Between", a+" and "+b,
			"Bond", capital(bond),
			"Tied by", tie,
			"Strain", capital(strain),
			"Unsaid", "“"+capital(say)+".”",
		),
		Prose: prose(
			fmt.Sprintf("%s and %s: %s, tied by %s", a, b, bond, tie),
			"Now "+strain,
			"What neither says: “"+capital(say)+".”",
		),
		Tags: []string{"relationship"},
	}
}
