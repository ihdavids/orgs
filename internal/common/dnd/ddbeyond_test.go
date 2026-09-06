// Tests for the D&D Beyond import: that a character-service payload lands on
// the right ruleset ids, that the numbers survive the trip, and that content
// the ruleset does not carry is kept by name rather than dropped.
package dnd

import (
	"strings"
	"testing"
)

// ddbPayload is a character-service response cut down to the fields the
// import reads: a level 4 high elf wizard with a magic item, a container and
// a spell list.
const ddbPayload = `{
 "success": true,
 "data": {
  "id": 12345678,
  "name": "Lyra Moonwhisper",
  "username": "someplayer",
  "avatarUrl": "https://www.dndbeyond.com/avatars/lyra.png",
  "gender": "Female",
  "age": 124,
  "hair": "Silver",
  "eyes": "Green",
  "skin": "Pale",
  "height": "5'7\"",
  "weight": 130,
  "inspiration": true,
  "baseHitPoints": 18,
  "removedHitPoints": 7,
  "temporaryHitPoints": 3,
  "currentXp": 2700,
  "alignmentId": 6,
  "stats": [{"id":1,"value":8},{"id":2,"value":14},{"id":3,"value":13},
            {"id":4,"value":15},{"id":5,"value":12},{"id":6,"value":10}],
  "bonusStats": [{"id":1,"value":null},{"id":2,"value":null},{"id":3,"value":null},
                 {"id":4,"value":null},{"id":5,"value":null},{"id":6,"value":null}],
  "overrideStats": [{"id":1,"value":null},{"id":2,"value":null},{"id":3,"value":null},
                    {"id":4,"value":null},{"id":5,"value":null},{"id":6,"value":null}],
  "race": {"isSubRace": true, "baseRaceName": "Elf", "fullName": "High Elf",
           "subRaceShortName": "High"},
  "classes": [{"id": 900, "level": 4, "isStartingClass": true,
               "definition": {"name": "Wizard", "hitDice": 6},
               "subclassDefinition": {"name": "School of Evocation"}}],
  "background": {"hasCustomBackground": false, "definition": {"name": "Sage"}},
  "feats": [{"definition": {"name": "Alert"}}],
  "traits": {"personalityTraits": "I speak in riddles.", "ideals": "Knowledge.",
             "bonds": "My old master.", "flaws": "I am a terrible liar.",
             "appearance": "Tall and severe."},
  "notes": {"backstory": "Raised in the towers.", "allies": "The Circle",
            "otherNotes": "Afraid of boats."},
  "currencies": {"cp": 5, "sp": 3, "ep": 0, "gp": 42, "pp": 1},
  "deathSaves": {"successCount": 2, "failCount": 1},
  "spellSlots": [{"level":1,"used":2,"available":4},{"level":2,"used":1,"available":3}],
  "inventory": [
    {"id": 111, "quantity": 1, "equipped": true, "isAttuned": false,
     "containerEntityId": 12345678,
     "definition": {"name": "Quarterstaff", "weight": 4, "filterType": "Weapon"}},
    {"id": 222, "quantity": 1, "equipped": true, "isAttuned": false,
     "containerEntityId": 12345678,
     "definition": {"name": "Backpack", "weight": 5, "isContainer": true}},
    {"id": 333, "quantity": 2, "equipped": false, "isAttuned": false,
     "containerEntityId": 222,
     "definition": {"name": "Potion of Healing", "weight": 0.5}},
    {"id": 444, "quantity": 1, "equipped": false, "isAttuned": true,
     "containerEntityId": 12345678,
     "definition": {"name": "Wand of Wonder Weaving", "weight": 1, "isHomebrew": true}}
  ],
  "classSpells": [{"characterClassId": 900, "spells": [
    {"definition": {"name": "Fire Bolt", "level": 0}, "prepared": false},
    {"definition": {"name": "Magic Missile", "level": 1}, "prepared": true},
    {"definition": {"name": "Misty Step", "level": 2}, "prepared": false}
  ]}],
  "spells": {"race": [{"definition": {"name": "Prestidigitation", "level": 0}}],
             "class": [], "item": [], "feat": [], "background": []},
  "modifiers": {
    "race": [{"type":"bonus","subType":"dexterity-score","value":2},
             {"type":"bonus","subType":"intelligence-score","value":1},
             {"type":"proficiency","subType":"perception","friendlySubtypeName":"Perception"},
             {"type":"language","subType":"elvish","friendlySubtypeName":"Elvish"},
             {"type":"language","subType":"common","friendlySubtypeName":"Common"}],
    "class": [{"type":"proficiency","subType":"arcana","friendlySubtypeName":"Arcana"},
              {"type":"proficiency","subType":"investigation","friendlySubtypeName":"Investigation"}],
    "background": [{"type":"proficiency","subType":"history","friendlySubtypeName":"History"},
                   {"type":"proficiency","subType":"thieves-tools","friendlySubtypeName":"Thieves' Tools"},
                   {"type":"expertise","subType":"arcana","friendlySubtypeName":"Arcana"}],
    "feat": [],
    "item": [{"type":"bonus","subType":"strength-score","value":9}],
    "condition": []
  }
 }
}`

func importTest(t *testing.T) (*Character, []string) {
	t.Helper()
	rs := srd(t)
	c, warnings, err := ImportDDB([]byte(ddbPayload), rs)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	return c, warnings
}

func TestImportIdentityAndBuild(t *testing.T) {
	c, _ := importTest(t)
	if c.Name != "Lyra Moonwhisper" {
		t.Errorf("name = %q", c.Name)
	}
	if c.Player != "someplayer" {
		t.Errorf("player = %q", c.Player)
	}
	if c.Race != "elf" || c.Subrace != "high-elf" {
		t.Errorf("race/subrace = %q/%q, want elf/high-elf", c.Race, c.Subrace)
	}
	if len(c.Classes) != 1 || c.Classes[0].Class != "wizard" || c.Classes[0].Level != 4 {
		t.Fatalf("classes = %+v", c.Classes)
	}
	if c.Classes[0].Subclass == "" {
		t.Errorf("subclass did not match, got empty for School of Evocation")
	}
	if c.Background != "sage" {
		t.Errorf("background = %q", c.Background)
	}
	if !containsStr(c.Feats, "alert") {
		t.Errorf("feats = %v, want alert", c.Feats)
	}
	if c.Alignment != "Chaotic Neutral" {
		t.Errorf("alignment = %q", c.Alignment)
	}
	if c.XP != 2700 {
		t.Errorf("xp = %d", c.XP)
	}
}

// Racial increases are part of the score on our sheet, and an item lending a
// score is not - the rules engine applies magic items itself.
func TestImportAbilitiesIncludeRacialButNotItems(t *testing.T) {
	c, _ := importTest(t)
	want := map[string]int{STR: 8, DEX: 16, CON: 13, INT: 16, WIS: 12, CHA: 10}
	for ab, v := range want {
		if c.Abilities[ab] != v {
			t.Errorf("%s = %d, want %d", ab, c.Abilities[ab], v)
		}
	}
}

func TestImportHitPointsAndState(t *testing.T) {
	c, _ := importTest(t)
	// 18 base + con mod 1 x 4 levels
	if c.HPMax != 22 {
		t.Errorf("hpMax = %d, want 22", c.HPMax)
	}
	if c.HPCurrent != 15 {
		t.Errorf("hpCurrent = %d, want 15", c.HPCurrent)
	}
	if c.HPTemp != 3 {
		t.Errorf("hpTemp = %d, want 3", c.HPTemp)
	}
	if !c.Inspiration {
		t.Errorf("inspiration was lost")
	}
	if c.DeathSaves != "2/1" {
		t.Errorf("deathSaves = %q, want 2/1", c.DeathSaves)
	}
	if len(c.SlotsUsed) != 2 || c.SlotsUsed[0] != 2 || c.SlotsUsed[1] != 1 {
		t.Errorf("slotsUsed = %v, want [2 1]", c.SlotsUsed)
	}
	if c.Money.GP != 42 || c.Money.PP != 1 || c.Money.CP != 5 {
		t.Errorf("money = %+v", c.Money)
	}
}

func TestImportProficiencies(t *testing.T) {
	c, _ := importTest(t)
	for _, want := range []string{"perception", "arcana", "investigation", "history"} {
		if !containsStr(c.Skills, want) {
			t.Errorf("skills = %v, missing %s", c.Skills, want)
		}
	}
	if !containsStr(c.Expertise, "arcana") {
		t.Errorf("expertise = %v, want arcana", c.Expertise)
	}
	if !containsStr(c.Languages, "Elvish") || !containsStr(c.Languages, "Common") {
		t.Errorf("languages = %v", c.Languages)
	}
	if !containsStr(c.Tools, "Thieves' Tools") {
		t.Errorf("tools = %v, want the thieves' tools", c.Tools)
	}
	// A skill proficiency is not a tool, and a tool is not a skill.
	if containsStr(c.Skills, "thieves-tools") {
		t.Errorf("a tool ended up in the skill list: %v", c.Skills)
	}
}

func TestImportInventoryKeepsContainersAndUnknowns(t *testing.T) {
	c, warnings := importTest(t)
	byName := map[string]Gear{}
	for _, g := range c.Equipment {
		byName[g.Name] = g
	}
	staff, ok := byName["Quarterstaff"]
	if !ok || staff.Id != "quarterstaff" || !staff.Equipped {
		t.Errorf("quarterstaff = %+v", staff)
	}
	potion, ok := byName["Potion of Healing"]
	if !ok || potion.Qty != 2 || potion.Container != "backpack" {
		t.Errorf("potion = %+v, want 2 in the backpack", potion)
	}
	wand, ok := byName["Wand of Wonder Weaving"]
	if !ok {
		t.Fatalf("the homebrew item was dropped: %v", c.Equipment)
	}
	if wand.Id != "" {
		t.Errorf("homebrew item matched %q, it is not in the srd", wand.Id)
	}
	if !wand.Attuned {
		t.Errorf("attunement was lost on %+v", wand)
	}
	if !warned(warnings, "Wand of Wonder Weaving") {
		t.Errorf("no warning about the homebrew item: %v", warnings)
	}
}

func TestImportSpells(t *testing.T) {
	c, _ := importTest(t)
	byId := map[string]KnownSpell{}
	for _, s := range c.Spells {
		byId[s.Id] = s
	}
	if len(byId) != 4 {
		t.Fatalf("spells = %+v, want 4", c.Spells)
	}
	if bolt, ok := byId["fire-bolt"]; !ok || bolt.Level != 0 || !bolt.Prepared {
		t.Errorf("fire bolt = %+v, a cantrip is always prepared", bolt)
	}
	if mm, ok := byId["magic-missile"]; !ok || mm.Level != 1 || !mm.Prepared {
		t.Errorf("magic missile = %+v", mm)
	}
	if step, ok := byId["misty-step"]; !ok || step.Prepared {
		t.Errorf("misty step = %+v, it was not prepared", step)
	}
	if p, ok := byId["prestidigitation"]; !ok || p.Source != "race" {
		t.Errorf("prestidigitation = %+v, want it sourced from the race", p)
	}
	// Spells arrive grouped by level so the sheet reads in order.
	for i := 1; i < len(c.Spells); i++ {
		if c.Spells[i-1].Level > c.Spells[i].Level {
			t.Fatalf("spells are out of level order: %+v", c.Spells)
		}
	}
}

func TestImportDetails(t *testing.T) {
	c, _ := importTest(t)
	if c.Age != "124" || c.Hair != "Silver" || c.Eyes != "Green" {
		t.Errorf("appearance = %q %q %q", c.Age, c.Hair, c.Eyes)
	}
	if !strings.Contains(c.Weight, "130") {
		t.Errorf("weight = %q", c.Weight)
	}
	if c.Image == "" {
		t.Errorf("the portrait url was dropped")
	}
	if c.Personality == "" || c.Ideals == "" || c.Bonds == "" || c.Flaws == "" {
		t.Errorf("roleplaying traits were dropped: %+v", c)
	}
	if !strings.Contains(c.Allies, "The Circle") {
		t.Errorf("allies = %q", c.Allies)
	}
	if !strings.Contains(c.Notes, "boats") {
		t.Errorf("notes = %q", c.Notes)
	}
}

// The whole point is a sheet, so the imported character has to compute and
// round trip through the org file like any other.
func TestImportedCharacterRoundTrips(t *testing.T) {
	rs := srd(t)
	c, _ := importTest(t)
	sheet := Compute(c, rs)
	if sheet == nil || sheet.Level != 4 {
		t.Fatalf("sheet = %+v", sheet)
	}
	org := RenderOrg(c, rs)
	back, err := ParseOrg(org, rs)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if back.Name != c.Name || back.HPMax != c.HPMax || len(back.Equipment) != len(c.Equipment) {
		t.Errorf("round trip lost something: %q %d %d",
			back.Name, back.HPMax, len(back.Equipment))
	}
	if len(back.Spells) != len(c.Spells) {
		t.Errorf("round trip lost spells: %d vs %d", len(back.Spells), len(c.Spells))
	}
}

func TestImportRejectsRubbish(t *testing.T) {
	rs := srd(t)
	if _, _, err := ImportDDB([]byte(`{"success": false, "message": "not found"}`), rs); err == nil {
		t.Errorf("a failed response was accepted")
	}
	if _, _, err := ImportDDB([]byte(`{"data": {"id": 1}}`), rs); err == nil {
		t.Errorf("a character with no name was accepted")
	}
	if _, _, err := ImportDDB([]byte(`not json`), rs); err == nil {
		t.Errorf("garbage was accepted")
	}
}

func warned(warnings []string, needle string) bool {
	for _, w := range warnings {
		if strings.Contains(w, needle) {
			return true
		}
	}
	return false
}

// D&D Beyond is loose about how it sends numbers and it carries fields this
// import does not read. A payload full of both has to convert anyway: a
// character that will not import over a field the sheet never shows is the
// worst possible failure.
const ddbAwkwardPayload = `{
 "success": true,
 "data": {
  "id": 87654321,
  "name": "Brannor",
  "username": null,
  "age": "38",
  "weight": "215",
  "height": null,
  "baseHitPoints": "44",
  "removedHitPoints": null,
  "temporaryHitPoints": null,
  "bonusHitPoints": null,
  "overrideHitPoints": null,
  "currentXp": "6500",
  "alignmentId": "1",
  "inspiration": false,
  "stats": [{"id":1,"value":"16"},{"id":2,"value":12},{"id":3,"value":"15"},
            {"id":4,"value":10},{"id":5,"value":13},{"id":6,"value":8}],
  "bonusStats": [],
  "overrideStats": [],
  "race": {"isSubRace": false, "baseRaceName": "Human", "fullName": "Human"},
  "classes": [{"id": "700", "level": "5", "isStartingClass": true,
               "definition": {"name": "Fighter", "hitDice": 10, "canCastSpells": false},
               "subclassDefinition": {"name": "Champion"},
               "classFeatures": [{"definition": {"name": "Second Wind"}}]}],
  "background": {"hasCustomBackground": false, "definition": {"name": "Soldier"}},
  "deathSaves": {"successCount": null, "failCount": null},
  "spellSlots": [],
  "currencies": {"cp": 0, "sp": 0, "ep": 0, "gp": "150", "pp": 0},
  "inventory": [
    {"id": "555", "quantity": "1", "equipped": true, "isAttuned": null,
     "containerEntityId": 87654321,
     "definition": {"name": "Longsword", "weight": "3", "canAttune": false,
                    "filterType": "Weapon", "subType": null, "rarity": "Common",
                    "damage": {"diceString": "1d8"}, "grantedModifiers": []}}
  ],
  "classSpells": [{"characterClassId": "700", "spells": [
    {"definition": {"name": "Shield", "level": "1", "school": "Abjuration",
                    "components": [1, 3], "duration": {"durationType": "Instantaneous"}},
     "prepared": true,
     "restriction": "1/Long Rest",
     "countsAsKnownSpell": null,
     "castAtLevel": null,
     "spellCastingAbilityId": 4,
     "additionalDescription": "",
     "activation": {"activationTime": 1, "activationType": 3}}
  ]}],
  "spells": {"race": [], "class": [], "item": [], "feat": [], "background": []},
  "modifiers": {
    "race": [{"type":"bonus","subType":"strength-score","value":"1"},
             {"type":"bonus","subType":"constitution-score","value":1}],
    "class": [{"type":"proficiency","subType":"athletics","friendlySubtypeName":"Athletics"}],
    "background": [], "feat": [], "item": [], "condition": []
  }
 }
}`

func TestImportSurvivesLooseNumbersAndUnknownFields(t *testing.T) {
	rs := srd(t)
	c, _, err := ImportDDB([]byte(ddbAwkwardPayload), rs)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if c.Name != "Brannor" {
		t.Fatalf("name = %q", c.Name)
	}
	// Quoted numbers are read as numbers, and the racial bonus is applied
	// whether it came quoted or bare.
	if c.Abilities[STR] != 17 || c.Abilities[CON] != 16 || c.Abilities[DEX] != 12 {
		t.Errorf("abilities = %v", c.Abilities)
	}
	if len(c.Classes) != 1 || c.Classes[0].Class != "fighter" || c.Classes[0].Level != 5 {
		t.Errorf("classes = %+v", c.Classes)
	}
	if c.Classes[0].Subclass != "champion" {
		t.Errorf("subclass = %q", c.Classes[0].Subclass)
	}
	if c.XP != 6500 {
		t.Errorf("xp = %d", c.XP)
	}
	if c.Age != "38" || !strings.Contains(c.Weight, "215") {
		t.Errorf("age/weight = %q %q", c.Age, c.Weight)
	}
	if c.Alignment != "Lawful Good" {
		t.Errorf("alignment = %q", c.Alignment)
	}
	if c.Money.GP != 150 {
		t.Errorf("gp = %d", c.Money.GP)
	}
	// 44 base + con mod 3 x 5 levels
	if c.HPMax != 59 || c.HPCurrent != 59 {
		t.Errorf("hp = %d/%d, want 59/59", c.HPCurrent, c.HPMax)
	}
	// A null death save count is nothing, not a failure.
	if c.DeathSaves != "0/0" {
		t.Errorf("deathSaves = %q", c.DeathSaves)
	}
	if len(c.Equipment) != 1 || c.Equipment[0].Id != "longsword" ||
		c.Equipment[0].Qty != 1 || c.Equipment[0].Weight != 3 {
		t.Errorf("equipment = %+v", c.Equipment)
	}
	// This is the spell whose "restriction": "1/Long Rest" used to fail the
	// whole import.
	if len(c.Spells) != 1 || c.Spells[0].Id != "shield" || c.Spells[0].Level != 1 {
		t.Errorf("spells = %+v", c.Spells)
	}
	if !containsStr(c.Skills, "athletics") {
		t.Errorf("skills = %v", c.Skills)
	}
	if _, err := ParseOrg(RenderOrg(c, rs), rs); err != nil {
		t.Errorf("round trip: %v", err)
	}
}

// A number that is not a number at all is treated as absent rather than
// failing the character.
func TestImportIgnoresUnreadableNumbers(t *testing.T) {
	rs := srd(t)
	c, warnings, err := ImportDDB([]byte(`{"data": {"name": "Odd",
	  "currentXp": "lots", "age": "middle aged",
	  "stats": [{"id":1,"value":"strong"},{"id":2,"value":14}],
	  "classes": [{"level": "many", "definition": {"name": "Rogue"}}]}}`), rs)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if c.XP != 0 || c.Age != "" {
		t.Errorf("unreadable numbers were not ignored: xp=%d age=%q", c.XP, c.Age)
	}
	if c.Abilities[STR] != 10 {
		t.Errorf("str = %d, an unreadable score should fall back to 10", c.Abilities[STR])
	}
	if !warned(warnings, "Strength") {
		t.Errorf("no warning about the missing score: %v", warnings)
	}
	if c.Abilities[DEX] != 14 {
		t.Errorf("dex = %d, the readable score beside it was lost", c.Abilities[DEX])
	}
}

// A field that arrives in a shape no amount of guessing covers must not cost
// the character. It is reported and the rest of the sheet still lands.
func TestImportWarnsRatherThanFailsOnAWrongShape(t *testing.T) {
	rs := srd(t)
	c, warnings, err := ImportDDB([]byte(`{"data": {
	  "name": "Shaped Wrong",
	  "hair": {"colour": "red"},
	  "stats": [{"id":1,"value":15},{"id":2,"value":12},{"id":3,"value":14},
	            {"id":4,"value":10},{"id":5,"value":11},{"id":6,"value":13}],
	  "classes": [{"level": 2, "definition": {"name": "Cleric"}}],
	  "currencies": {"gp": 10}}}`), rs)
	if err != nil {
		t.Fatalf("a wrong shaped field failed the whole import: %v", err)
	}
	if c.Name != "Shaped Wrong" || len(c.Classes) != 1 || c.Classes[0].Class != "cleric" {
		t.Errorf("the rest of the character did not survive: %+v", c)
	}
	if c.Abilities[STR] != 15 || c.Money.GP != 10 {
		t.Errorf("fields after the bad one were lost: %v %+v", c.Abilities, c.Money)
	}
	if !warned(warnings, "hair") {
		t.Errorf("the skipped field was not reported: %v", warnings)
	}
}

// The name matcher must not reach. Scattered letters are a match a person
// typing at a list would accept and a silent import must not: "String" shares
// its letters, in order, with "Signet ring".
func TestImportDoesNotInventItemMatches(t *testing.T) {
	rs := srd(t)
	c, warnings, err := ImportDDB([]byte(`{"data": {"name": "Packrat",
	  "inventory": [
	    {"id": 1, "quantity": 10, "definition": {"name": "String", "weight": 0}},
	    {"id": 2, "quantity": 1, "definition": {"name": "Costume", "weight": 4}},
	    {"id": 3, "quantity": 1, "definition": {"name": "Mirror", "weight": 0.5}}
	  ]}}`), rs)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	byName := map[string]Gear{}
	for _, g := range c.Equipment {
		byName[g.Name] = g
	}
	if g, ok := byName["String"]; !ok || g.Id != "" {
		t.Errorf("String matched %q, it is not in the srd", g.Id)
	}
	if _, wrong := byName["Signet ring"]; wrong {
		t.Errorf("String was turned into a signet ring: %+v", c.Equipment)
	}
	if !warned(warnings, "String") {
		t.Errorf("the unmatched item was not reported: %v", warnings)
	}
	// A name the books qualify and D&D Beyond does not still has to land.
	if g, ok := byName["Clothes, costume"]; !ok || g.Id != "clothes-costume" {
		t.Errorf("Costume did not match Clothes, costume: %+v", c.Equipment)
	}
	if g, ok := byName["Mirror, steel"]; !ok || g.Id != "mirror-steel" {
		t.Errorf("Mirror did not match Mirror, steel: %+v", c.Equipment)
	}
}

// D&D Beyond counts single arrows; the ruleset carries a bundle of twenty.
func TestImportCountsBundlesNotUnits(t *testing.T) {
	rs := srd(t)
	c, _, err := ImportDDB([]byte(`{"data": {"name": "Archer",
	  "inventory": [
	    {"id": 1, "quantity": 20, "definition": {"name": "Arrows", "bundleSize": 20, "weight": 1}},
	    {"id": 2, "quantity": 60, "definition": {"name": "Arrows", "bundleSize": 20, "weight": 1}},
	    {"id": 3, "quantity": 1000, "definition": {"name": "Ball Bearings (bag of 1,000)",
	                                               "bundleSize": 1000, "weight": 2}},
	    {"id": 4, "quantity": 5, "definition": {"name": "Candle", "bundleSize": 1, "weight": 0}}
	  ]}}`), rs)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	qty := map[string]int{}
	for _, g := range c.Equipment {
		qty[g.Name] += g.Qty
	}
	// 20 arrows is one bundle, 60 is three.
	if qty["Arrows (20)"] != 4 {
		t.Errorf("arrows = %d bundles, want 4 (one of 20 plus three of 60)", qty["Arrows (20)"])
	}
	if qty["Ball bearings (bag of 1,000)"] != 1 {
		t.Errorf("ball bearings = %d bags, want 1", qty["Ball bearings (bag of 1,000)"])
	}
	// Something that is not bundled keeps its own count.
	if qty["Candle"] != 5 {
		t.Errorf("candles = %d, want 5", qty["Candle"])
	}
	// And the weight follows the count, which is what the mistake was
	// actually noticed as: a bag of a thousand ball bearings weighs two
	// pounds, not two thousand, and four bundles of arrows weigh four.
	lb := map[string]float64{}
	for _, g := range c.Equipment {
		lb[g.Name] += float64(g.Qty) * g.Weight
	}
	if lb["Ball bearings (bag of 1,000)"] != 2 {
		t.Errorf("ball bearings weigh %v lb, want 2", lb["Ball bearings (bag of 1,000)"])
	}
	if lb["Arrows (20)"] != 4 {
		t.Errorf("arrows weigh %v lb, want 4", lb["Arrows (20)"])
	}
}

// An ability score improvement granted by a source D&D Beyond does not have a
// feat for is still filed under feats. The scores come across as modifiers, so
// the entry itself is bookkeeping and does not belong on the sheet.
func TestImportDropsAbilityIncreasesFromFeats(t *testing.T) {
	rs := srd(t)
	c, _, err := ImportDDB([]byte(`{"data": {"name": "Improved",
	  "stats": [{"id":1,"value":15}],
	  "feats": [{"definition": {"name": "Grappler"}},
	            {"definition": {"name": "Wandering Troupe Ability Score Increase"}}],
	  "modifiers": {"feat": [{"type":"bonus","subType":"strength-score","value":2}]}}}`), rs)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(c.Feats) != 1 || c.Feats[0] != "grappler" {
		t.Errorf("feats = %v, want just the real one", c.Feats)
	}
	// The increase itself is not lost, it is already in the score.
	if c.Abilities[STR] != 17 {
		t.Errorf("str = %d, want 17 - the increase was dropped with the entry", c.Abilities[STR])
	}
}

// Expertise in a tool is kept on its own list. It must not land in Expertise,
// which is indexed by skill id, and it must not be silently flattened into
// plain proficiency either.
func TestImportKeepsToolExpertise(t *testing.T) {
	rs := srd(t)
	c, warnings, err := ImportDDB([]byte(`{"data": {"name": "Sneak",
	  "modifiers": {"class": [
	    {"type":"expertise","subType":"thieves-tools","friendlySubtypeName":"Thieves' Tools"},
	    {"type":"expertise","subType":"stealth","friendlySubtypeName":"Stealth"}]}}}`), rs)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !containsStr(c.Expertise, "stealth") {
		t.Errorf("skill expertise = %v, want stealth", c.Expertise)
	}
	if containsStr(c.Expertise, "thieves-tools") {
		t.Errorf("a tool ended up in the skill expertise list: %v", c.Expertise)
	}
	if !containsStr(c.ToolExpertise, "Thieves' Tools") {
		t.Errorf("tool expertise = %v, want Thieves' Tools", c.ToolExpertise)
	}
	// Expertise implies proficiency, so the tool is listed either way.
	if !containsStr(c.Tools, "Thieves' Tools") {
		t.Errorf("tools = %v, want Thieves' Tools", c.Tools)
	}
	if warned(warnings, "Thieves' Tools") {
		t.Errorf("tool expertise still warns now that it has somewhere to go: %v", warnings)
	}
}

// The doubling has to reach the sheet, otherwise recording it changed nothing.
func TestSheetMarksToolExpertise(t *testing.T) {
	rs := srd(t)
	c := &Character{
		Name:          "Sneak",
		Classes:       []ClassLevel{{Class: "rogue", Level: 5}},
		Tools:         []string{"Disguise Kit", "Thieves' Tools"},
		ToolExpertise: []string{"Thieves' Tools"},
	}
	s := Compute(c, rs)
	if !containsStr(s.ToolProficiencies, "Thieves' Tools (expertise)") {
		t.Errorf("tool proficiencies = %v, want Thieves' Tools marked", s.ToolProficiencies)
	}
	if !containsStr(s.ToolProficiencies, "Disguise Kit") {
		t.Errorf("tool proficiencies = %v, want Disguise Kit left alone", s.ToolProficiencies)
	}
}

// A tool only the expertise list mentions is still a proficiency.
func TestToolExpertiseImpliesProficiency(t *testing.T) {
	rs := srd(t)
	s := Compute(&Character{
		Name:          "Sneak",
		Classes:       []ClassLevel{{Class: "rogue", Level: 5}},
		ToolExpertise: []string{"Thieves' Tools"},
	}, rs)
	if !containsStr(s.ToolProficiencies, "Thieves' Tools (expertise)") {
		t.Errorf("tool proficiencies = %v, want Thieves' Tools listed", s.ToolProficiencies)
	}
}

// The SRD strips the wizard a spell is named after, so a payload written
// against the books has to find the trimmed entry rather than warn about it.
func TestImportMatchesNamedWizardSpells(t *testing.T) {
	rs := srd(t)
	idx := newNameIndex(rs)
	for name, want := range map[string]string{
		"Tasha's Hideous Laughter":       "hideous-laughter",
		"Evard's Black Tentacles":        "black-tentacles",
		"Melf's Acid Arrow":              "acid-arrow",
		"Leomund's Tiny Hut":             "tiny-hut",
		"Otiluke's Resilient Sphere":     "resilient-sphere",
		"Rary's Telepathic Bond":         "telepathic-bond",
		"Otto's Irresistible Dance":      "irresistible-dance",
		"Mordenkainen's Faithful Hound":  "faithful-hound",
		"Drawmij's Instant Summons":      "instant-summons",
		"Leomund's Secret Chest":         "secret-chest",
		"Mordenkainen's Private Sanctum": "private-sanctum",
		// The reverse direction: the SRD kept a possessive of its own here,
		// and the books' name is the one being looked up.
		"Nystul's Magic Aura": "arcanists-magic-aura",
	} {
		if got := idx.spells.find(name); got != want {
			t.Errorf("%s matched %q, want %q", name, got, want)
		}
	}
}

// An alias must never outrank a name that matched outright, and a possessive
// that is really a category must not become one at all.
func TestAliasesDoNotSwallowRealNames(t *testing.T) {
	rs := srd(t)
	idx := newNameIndex(rs)
	// "Burglar's Pack" would alias to "pack" if one word were allowed.
	if got := idx.items.find("Burglar's Pack"); got != "burglars-pack" {
		t.Errorf("burglar's pack matched %q", got)
	}
	if len(ddbAliasKeys("Thieves' Tools")) != 0 {
		t.Errorf("a one word remainder became an alias: %v", ddbAliasKeys("Thieves' Tools"))
	}
	// Mage Hand is a spell in its own right, so Bigby's Hand must not take it.
	if got := idx.spells.find("Bigby's Hand"); got == "mage-hand" {
		t.Errorf("bigby's hand was quietly turned into mage hand")
	}
}
