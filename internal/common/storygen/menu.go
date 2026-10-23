package storygen

// The menu, in the order it is shown.
func init() {
	gender := Field{Id: "gender", Label: "Names", Kind: "select", Default: "any",
		Choices: []Choice{{"any", "Any names"}, {"feminine", "Feminine names"}, {"masculine", "Masculine names"}, {"neutral", "Either"}}}

	register(Info{Id: "character", Name: "Character", Group: "People",
		Summary: "Someone to write: looks, manner, wants, fears and a secret.",
		Fields: []Field{genreField, gender,
			{Id: "role", Label: "Role", Kind: "select", Default: "any",
				Choices: []Choice{{"any", "Any role"}, {"lead", "Lead"}, {"antagonist", "Antagonist"}, {"supporting", "Supporting"}}}},
	}, runCharacter)
	register(Info{Id: "names", Name: "Names", Group: "People", Count: 2,
		Summary: "People and places whose names sound as if they belong together.",
		Fields:  []Field{genreField, gender},
	}, runNames)
	register(Info{Id: "relationship", Name: "Relationship", Group: "People",
		Summary: "Two people, what binds them, what strains it, and what neither says.",
		Fields:  []Field{genreField},
	}, runRelationship)

	register(Info{Id: "settlement", Name: "Settlement", Group: "Places", Count: 2,
		Summary: "A village, town, colony or station: who runs it, its custom, its trouble.",
		Fields:  []Field{genreField},
	}, runSettlement)
	register(Info{Id: "place", Name: "Scene setting", Group: "Places",
		Summary: "Somewhere for a scene, with what you see, hear, smell and feel.",
		Fields:  []Field{genreField},
	}, runPlace)
	register(Info{Id: "tavern", Name: "Tavern", Group: "Places", Count: 2,
		Summary: "An inn, bar or cantina: its keeper, drink, regulars, tonight and the talk.",
		Fields:  []Field{genreField},
	}, runTavern)

	register(Info{Id: "culture", Name: "Culture", Group: "Societies", Count: 2,
		Summary: "A people: what they prize and forbid, how they greet, grow up and bury.",
		Fields:  []Field{genreField},
	}, runCulture)
	register(Info{Id: "religion", Name: "Religion", Group: "Societies", Count: 2,
		Summary: "A faith: what it worships, keeps and commands, and what splits it.",
		Fields:  []Field{genreField},
	}, runReligion)
	register(Info{Id: "organisation", Name: "Organisation", Group: "Societies", Count: 2,
		Summary: "A guild, company or gang: what it says it's for, and what it wants.",
		Fields:  []Field{genreField},
	}, runOrganisation)

	register(Info{Id: "premise", Name: "Premise", Group: "Story", Count: 5,
		Summary: "Who must do what, by when, and what's in the way.",
		Fields:  []Field{genreField},
	}, runPremise)
	register(Info{Id: "complication", Name: "Complication", Group: "Story", Count: 5,
		Summary: "A twist to throw at a plot that's going too smoothly.",
		Fields:  []Field{genreField},
	}, runComplication)
	register(Info{Id: "conflict", Name: "Conflict", Group: "Story", Count: 5,
		Summary: "A struggle, by kind.",
		Fields: []Field{{Id: "kind", Label: "Kind", Kind: "select", Default: "any",
			Choices: append([]Choice{{"any", "Any kind"}}, conflictKinds...)}},
	}, runConflict)
	register(Info{Id: "opening", Name: "Opening", Group: "Story", Count: 5,
		Summary: "A way into a scene or a chapter.",
		Fields:  []Field{genreField},
	}, runOpening)
	register(Info{Id: "object", Name: "Object", Group: "Story", Count: 4,
		Summary: "A thing with a past, for someone to carry, lose or want.",
		Fields:  []Field{genreField},
	}, runObject)
	register(Info{Id: "treasure", Name: "Treasure", Group: "Story", Count: 3,
		Summary: "A find: where, in what, what's inside, and the catch.",
		Fields: []Field{genreField,
			{Id: "size", Label: "Size", Kind: "select", Default: "any",
				Choices: []Choice{{"any", "Any size"}, {"small", "A small find"}, {"cache", "A cache"}, {"hoard", "A hoard"}}}},
	}, runTreasure)
	register(Info{Id: "weather", Name: "Weather", Group: "Story", Count: 4,
		Summary: "Weather, light and the mood it puts people in.",
		Fields: []Field{genreField,
			{Id: "season", Label: "Season", Kind: "select", Default: "any",
				Choices: []Choice{{"any", "Any season"}, {"spring", "Spring"}, {"summer", "Summer"}, {"autumn", "Autumn"}, {"winter", "Winter"}}}},
	}, runWeather)
}
