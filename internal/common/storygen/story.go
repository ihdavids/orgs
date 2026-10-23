package storygen

import (
	"fmt"
	"strings"
)

// Story prompts, objects, treasure and weather. The tables are in
// data/story.txt and data/world.txt.

// ---- premises --------------------------------------------------------------

func runPremise(c *ctx) Item {
	who := c.t("whos")
	goal := c.t("goals")
	when := c.t("deadlines")
	but := c.t("obstacles")
	line := fmt.Sprintf("%s must %s %s, but %s", capital(who), goal, when, but)
	return Item{
		Title:   sentence(line),
		Summary: "A premise",
		Facts:   facts("Who", capital(who), "Must", goal, "When", when, "But", but),
		Prose:   sentence(line),
		Tags:    []string{"premise"},
	}
}

// ---- complications, openings -----------------------------------------------

func runComplication(c *ctx) Item {
	t := sentence(c.t("complications"))
	return Item{Title: t, Summary: "A complication", Prose: t, Facts: facts("Twist", t), Tags: []string{"complication"}}
}

func runOpening(c *ctx) Item {
	t := sentence(c.t("openings"))
	return Item{Title: t, Summary: "A way into a scene", Prose: t, Facts: facts("Opening", t), Tags: []string{"opening"}}
}

// ---- conflicts -------------------------------------------------------------

var conflictKinds = []Choice{
	{"person", "Person against person"}, {"self", "Person against themself"}, {"society", "Person against society"},
	{"nature", "Person against nature"}, {"fate", "Person against fate"}, {"unknown", "Person against the unknown"},
}

func runConflict(c *ctx) Item {
	k := c.opt("kind")
	label := ""
	for _, x := range conflictKinds {
		if x.Value == k {
			label = x.Label
		}
	}
	if label == "" {
		x := conflictKinds[c.r.IntN(len(conflictKinds))]
		k, label = x.Value, x.Label
	}
	line := sentence(c.ts("conflicts", k))
	return Item{Title: line, Summary: label, Prose: line, Facts: facts("Kind", label, "Conflict", line), Tags: []string{"conflict"}}
}

// ---- objects ---------------------------------------------------------------

func runObject(c *ctx) Item {
	thing := c.t("object_things")
	look := c.t("object_looks")
	past := c.t("object_pasts")
	quirk := c.t("object_quirks")
	what := capital(article(thing)) + ", " + look
	return Item{
		Title:   what,
		Summary: "It " + past,
		Entry:   "thing",
		Facts:   facts("What", what, "History", "It "+past, "Odd", "It "+quirk),
		Prose:   prose(what+"; it "+past, "It "+quirk),
		Tags:    []string{"object"},
	}
}

// ---- treasure --------------------------------------------------------------

func runTreasure(c *ctx) Item {
	size := c.opt("size")
	if size == "" || size == "any" {
		size = c.pick([]string{"small", "cache", "cache", "hoard"})
	}
	where := c.t("treasure_places")
	in := c.t("containers")
	n := map[string]int{"small": 1, "cache": c.between(2, 3), "hoard": c.between(4, 6)}[size]
	things := c.tN("valuables", n)
	coins := ""
	if size != "small" || c.chance(40) {
		coins = c.t("coins")
	}
	odd := c.t("curiosities")
	catch := c.t("catches")
	contents := append([]string(nil), things...)
	if coins != "" {
		contents = append([]string{coins}, contents...)
	}
	contents = append(contents, odd)
	return Item{
		Title:   capital(in),
		Summary: "Found " + where,
		Entry:   "thing",
		Facts: facts(
			"Found", capital(where),
			"In", capital(in),
			"Money", capital(coins),
			"Valuables", capital(strings.Join(things, "; ")),
			"Curiosity", capital(odd),
			"Catch", capital(catch),
		),
		Prose: prose(
			"Found "+where+", in "+in+": "+andList(contents),
			"The catch: "+catch,
		),
		Tags: []string{"treasure"},
	}
}

// ---- weather ---------------------------------------------------------------

var seasonsOf = []string{"spring", "summer", "autumn", "winter"}

func runWeather(c *ctx) Item {
	season := c.opt("season")
	if season == "" || season == "any" {
		season = c.pick(seasonsOf)
	}
	light := c.t("lights")
	mood := sentence(c.t("moods"))
	if c.genre == "sf" && c.chance(60) {
		w := c.t("sf_weather")
		return Item{
			Title:   capital(w),
			Summary: "Weather off Earth",
			Facts:   facts("Weather", capital(w), "Light", capital(light), "Mood", mood),
			Prose:   prose(capital(w)+"; "+light, mood),
			Tags:    []string{"weather"},
		}
	}
	sky := c.ts("weather_sky", season)
	wind := c.t("weather_wind")
	temp := c.ts("weather_temp", season)
	event := ""
	if c.chance(30) {
		event = c.ts("weather_events", season)
	}
	return Item{
		Title:   capital(sky),
		Summary: capital(season) + " weather",
		Facts: facts(
			"Season", capital(season),
			"Sky", capital(sky),
			"Wind", capital(wind),
			"Feels", capital(temp),
			"Event", capital(event),
			"Light", capital(light),
			"Mood", mood,
		),
		Prose: prose(capital(sky)+"; "+wind, capital(temp), capital(event), capital(light), mood),
		Tags:  []string{"weather"},
	}
}
