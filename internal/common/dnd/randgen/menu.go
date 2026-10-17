package randgen

// The menu, in the order it is shown. One init rather than one per file so
// that the order is written down somewhere instead of following file names.
func init() {
	party := Field{Id: "party", Label: "Party size", Kind: "number", Default: "4", Min: 1, Max: 12}
	level := Field{Id: "level", Label: "Party level", Kind: "select", Choices: levelChoices(), Default: "1"}
	race := Field{Id: "race", Label: "Race", Kind: "select", Default: "any"}

	register(Info{
		Id: "encounter", Name: "Encounters", Group: "Combat", Count: 3,
		Summary: "Monsters balanced against the party's XP budget.",
		Fields: []Field{party, level,
			{Id: "difficulty", Label: "Difficulty", Kind: "select", Default: "any",
				Choices: choices("any=Any difficulty", "easy", "medium", "hard", "deadly")},
			{Id: "environment", Label: "Environment", Kind: "select", Default: "any", Choices: environmentChoices()},
		},
	}, runEncounter)
	register(Info{
		Id: "trap", Name: "Traps", Group: "Combat", Count: 3,
		Summary: "A trigger, an effect, and the numbers to run it.",
		Fields: []Field{level,
			{Id: "danger", Label: "Danger", Kind: "select", Default: "any",
				Choices: choices("any=Any danger", "setback", "dangerous", "deadly")},
			{Id: "kind", Label: "Kind", Kind: "select", Default: "any",
				Choices: choices("any=Mechanical or magical", "mechanical", "magical")},
		},
	}, runTrap)

	cr := Field{Id: "cr", Label: "Challenge", Kind: "select", Default: "0-4",
		Choices: choices("0-4=Challenge 0-4", "5-10=Challenge 5-10", "11-16=Challenge 11-16", "17+=Challenge 17+")}
	register(Info{
		Id: "treasure", Name: "Treasure", Group: "Treasure", Count: 3,
		Summary: "A purse or a hoard, by the challenge of whoever owned it.",
		Fields: []Field{cr,
			{Id: "kind", Label: "Kind", Kind: "select", Default: "individual",
				Choices: choices("individual=Individual treasure", "hoard=Treasure hoard")},
		},
	}, runTreasure)
	register(Info{
		Id: "magicitem", Name: "Magic Items", Group: "Treasure",
		Summary: "From the loaded ruleset, homebrew included.",
		Fields: []Field{
			{Id: "rarity", Label: "Rarity", Kind: "select", Default: "any",
				Choices: choices("any=Any rarity", "common", "uncommon", "rare", "very rare", "legendary")},
			{Id: "kind", Label: "Kind", Kind: "select", Default: "any",
				Choices: choices("any=Any kind", "armor", "weapon", "potion", "ring", "rod", "scroll", "staff", "wand", "wondrous")},
		},
	}, runMagicItem)
	register(Info{
		Id: "gem", Name: "Gems", Group: "Treasure",
		Fields: []Field{{Id: "value", Label: "Value", Kind: "select", Default: "0",
			Choices: choices("0=Any value", "10=10 gp", "50=50 gp", "100=100 gp", "500=500 gp", "1000=1,000 gp", "5000=5,000 gp")}},
	}, runGems)
	register(Info{
		Id: "art", Name: "Art Objects", Group: "Treasure",
		Fields: []Field{{Id: "value", Label: "Value", Kind: "select", Default: "0",
			Choices: choices("0=Any value", "25=25 gp", "250=250 gp", "750=750 gp", "2500=2,500 gp", "7500=7,500 gp")}},
	}, runArt)
	register(Info{
		Id: "spellbook", Name: "Spellbook", Group: "Treasure", Count: 1,
		Summary: "A caster's book, from the ruleset's spell lists.",
		Fields: []Field{
			{Id: "level", Label: "Caster level", Kind: "select", Choices: levelChoices(), Default: "5"},
			{Id: "class", Label: "Class", Kind: "select", Default: "wizard",
				Choices: choices("wizard", "sorcerer", "warlock", "bard", "cleric", "druid", "paladin", "ranger")},
		},
	}, runSpellbook)

	register(Info{
		Id: "npc", Name: "NPCs", Group: "People", Count: 3,
		Summary: "Someone to meet: looks, voice, wants and secrets.",
		Fields: []Field{race,
			{Id: "occupation", Label: "Occupation", Kind: "select", Default: "any",
				Choices: choices("any=Any occupation", "commoner", "craft=Crafter", "merchant", "martial=Fighter", "learned=Scholar", "religious", "criminal", "noble", "wanderer")},
		},
	}, runNpc)
	register(Info{
		Id: "name", Name: "Names", Group: "People", Count: 10,
		Fields: []Field{race},
	}, runNames)
	register(Info{
		Id: "tavern", Name: "Taverns", Group: "People", Count: 2,
		Summary: "Somewhere to drink, with a keeper and two rumours.",
	}, runTavern)

	register(Info{Id: "hook", Name: "Adventure Hooks", Group: "Story", Count: 3}, runHook)
	register(Info{Id: "trinket", Name: "Trinkets", Group: "Oddities", Count: 6}, listGen("trinket", trinkets))
	register(Info{Id: "wildsurge", Name: "Wild Magic Surges", Group: "Oddities", Count: 3}, runSurge)
	register(Info{Id: "carousing", Name: "Carousing Events", Group: "Downtime", Count: 3}, runCarousing)
}
