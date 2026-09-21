// An end to end test of the things the html sheet grew: the death save marks,
// the concentration band, the attunement counter, the drink buttons and the
// prices on what can be bought or sold. Each of them is rendered through the
// real template from a real org character sheet, because every one of them is a
// join between the engine, the exporter and the page - and a break in any of
// the three shows up here and nowhere else.
package dnd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/templates"
	logging "gopkg.in/op/go-logging.v1"
)

// renderOrg renders one org character sheet through the real exporter.
func renderOrg(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	sheet := filepath.Join(dir, "lyra.org")
	if err := os.WriteFile(sheet, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../../../../../templates")
	if err != nil {
		t.Fatal(err)
	}
	tempo := &templates.TemplateManager{TemplatePath: root}
	tempo.Initialize()
	exp := &SheetExporter{Format: "html"}
	exp.Startup(&common.PluginManager{
		Out:   logging.MustGetLogger("test"),
		Tempo: tempo,
	}, &common.PluginOpts{})
	err, html := exp.ExportToString(stubDb{}, sheet, "", nil)
	if err != nil {
		t.Fatalf("export: %s", err)
	}
	// A pongo2 failure comes back as html rather than as an error, so the one
	// thing every rendered sheet must have is checked here for all of them.
	if !strings.Contains(html, "</html>") {
		t.Fatalf("the template did not render:\n%s", clip(html))
	}
	return html
}

func clip(s string) string {
	if len(s) > 900 {
		return s[:900]
	}
	return s
}

// seed reads one of the json blobs the page is handed.
func seed(t *testing.T, html, id string) map[string]interface{} {
	t.Helper()
	open := `<script type="application/json" id="` + id + `">`
	i := strings.Index(html, open)
	if i < 0 {
		t.Fatalf("no %s in the page", id)
	}
	rest := html[i+len(open):]
	raw := rest[:strings.Index(rest, "</script>")]
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("%s: %s in %.200q", id, err, raw)
	}
	return out
}

// A fighter down on nothing at all, with two death saves already marked.
const dyingSheet = `* Lyra
   :PROPERTIES:
   :DND_NAME:         Lyra
   :DND_RACE:         human
   :DND_CLASSES:      fighter::3
   :DND_LEVEL:        3
   :DND_CON:          14
   :DND_HP_MAX:       28
   :DND_HP_CURRENT:   0
   :DND_DEATH_SAVES:  1/2
   :END:
`

func TestSheetDrawsDeathSaves(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	// The marks are in the page before any script runs, filled to the count
	// the property carries, so a sheet read off disk shows where things stand.
	if !strings.Contains(html, `id="death-pips"`) {
		t.Error("no death save marks in the page")
	}
	if !strings.Contains(html, `data-dying="1"`) {
		t.Error("a character on nothing at all is not marked as dying")
	}
	// One success and two failures: three filled marks in all.
	if n := strings.Count(html, `class="dp win on"`); n != 1 {
		t.Errorf("%d successes filled, want 1", n)
	}
	if n := strings.Count(html, `class="dp lose on"`); n != 2 {
		t.Errorf("%d failures filled, want 2", n)
	}
	if !strings.Contains(html, ">Dying<") {
		t.Error("the line does not say the character is dying")
	}

	// And the state the page redraws from agrees.
	health := seed(t, html, "dnd-health-data")
	hp, _ := health["hp"].(map[string]interface{})
	if hp == nil {
		t.Fatal("no hit points in the seed")
	}
	if hp["deathSuccesses"] != float64(1) || hp["deathFailures"] != float64(2) {
		t.Errorf("seed marks = %v/%v", hp["deathSuccesses"], hp["deathFailures"])
	}
	if hp["dying"] != true || hp["dead"] == true || hp["stable"] == true {
		t.Errorf("seed says dying=%v dead=%v stable=%v",
			hp["dying"], hp["dead"], hp["stable"])
	}
}

// A hale character keeps the line but nothing is filled in and it is not a
// control: the marks only become buttons when they matter.
func TestSheetDeathSavesQuietWhenWell(t *testing.T) {
	html := renderOrg(t, strings.Replace(
		strings.Replace(dyingSheet, ":DND_HP_CURRENT:   0", ":DND_HP_CURRENT:   28", 1),
		":DND_DEATH_SAVES:  1/2", ":DND_DEATH_SAVES:", 1))
	if !strings.Contains(html, `data-dying="0"`) {
		t.Error("a character at full hit points is marked as dying")
	}
	if strings.Contains(html, `class="dp win on"`) ||
		strings.Contains(html, `class="dp lose on"`) {
		t.Error("marks are filled in on a character who has never been down")
	}
}

// A wizard holding a spell. The band is drawn from the character's own
// DND_CONCENTRATION property, so it survives the sheet being reloaded.
func TestSheetCarriesConcentration(t *testing.T) {
	html := renderOrg(t, `* Wizard
   :PROPERTIES:
   :DND_NAME:           Ilya
   :DND_RACE:           elf
   :DND_CLASSES:        wizard::5
   :DND_LEVEL:          5
   :DND_CON:            14
   :DND_INT:            16
   :DND_CONCENTRATION:  haste @3
   :END:
`)
	conc := seed(t, html, "dnd-concentration-data")
	if conc["on"] != true {
		t.Fatalf("the spell is not up: %v", conc)
	}
	if conc["name"] != "Haste" {
		t.Errorf("name = %v, want Haste", conc["name"])
	}
	if conc["label"] != "Haste (3rd level)" {
		t.Errorf("label = %v", conc["label"])
	}
	// Constitution 14 and no proficiency, so a flat +2 to keep it.
	if conc["saveStr"] != "+2" {
		t.Errorf("save = %v, want +2", conc["saveStr"])
	}
	if conc["duration"] == "" || conc["duration"] == nil {
		t.Error("the band cannot say how long the spell lasts")
	}
	if !strings.Contains(html, `id="conc-line"`) {
		t.Error("no concentration band in the page")
	}
	// A spell that asks for concentration tells the cast button so, which is
	// what takes it up when it is cast.
	if !strings.Contains(html, `data-conc="1"`) {
		t.Error("no cast button says it concentrates")
	}
}

func TestSheetHasNoConcentrationBandWhenNothingIsHeld(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	conc := seed(t, html, "dnd-concentration-data")
	if conc["on"] != false {
		t.Errorf("something is being held: %v", conc)
	}
}

// Three magic items that ask for attunement, two of them attuned to.
const attunedSheet = `* Magpie
   :PROPERTIES:
   :DND_NAME:       Magpie
   :DND_RACE:       human
   :DND_CLASSES:    fighter::5
   :DND_LEVEL:      5
   :DND_STR:        14
   :DND_GP:         50
   :END:

** Equipment
| Item               | Qty | Equipped | Attuned | Weight | Container | Notes |
|--------------------+-----+----------+---------+--------+-----------+-------|
| Backpack           |   1 | no       | no      |      5 |           |       |
| Ring of Protection |   1 | yes      | yes     |      0 |           |       |
| Amulet of Health   |   1 | yes      | yes     |      0 |           |       |
| Boots of Levitation|   1 | yes      | no      |      1 |           |       |
| Potion of Healing  |   2 | no       | no      |    0.5 | backpack  |       |
| Longsword          |   1 | yes      | no      |      3 |           |       |
`

func TestSheetCountsAttunement(t *testing.T) {
	html := renderOrg(t, attunedSheet)
	att := seed(t, html, "dnd-attunement-data")
	if att["used"] != float64(2) || att["slots"] != float64(3) {
		t.Errorf("attunement = %v of %v", att["used"], att["slots"])
	}
	if att["over"] == true {
		t.Error("two attunements read as over the limit")
	}
	items, _ := att["items"].([]interface{})
	if len(items) != 2 {
		t.Errorf("items = %v", items)
	}

	// The inventory the panel is drawn from has to say which lines are worth
	// offering the mark on, or the star cannot become a button.
	inv := seed(t, html, "dnd-inventory-data")
	needs := map[string]bool{}
	for _, box := range inv["containers"].([]interface{}) {
		b := box.(map[string]interface{})
		for _, e := range b["entries"].([]interface{}) {
			entry := e.(map[string]interface{})
			needs[entry["name"].(string)] = entry["attunement"] == true
		}
	}
	if !needs["Ring of Protection"] {
		t.Error("the ring does not say it needs attunement")
	}
	if needs["Longsword"] {
		t.Error("a longsword says it needs attunement")
	}
}

// Four attunements is one more than anyone has. The rules engine leaves the
// fourth inert and the counter says so, which is the only way a player can see
// why their boots are doing nothing.
func TestSheetWarnsWhenOverAttuned(t *testing.T) {
	html := renderOrg(t, strings.Replace(attunedSheet,
		"| Boots of Levitation|   1 | yes      | no      |      1 |           |       |",
		"| Boots of Levitation|   1 | yes      | yes     |      1 |           |       |\n"+
			"| Bracers of Archery |   1 | yes      | yes     |      1 |           |       |", 1))
	att := seed(t, html, "dnd-attunement-data")
	if att["used"] != float64(3) {
		t.Errorf("used = %v, want three - the limit", att["used"])
	}
	if att["over"] != true {
		t.Error("four attunements did not read as over the limit")
	}
}

// What a line of the bag can be drunk for and sold for, both worked out by the
// server so the page never has to read an item's text or price it.
func TestSheetCarriesUseAndPrices(t *testing.T) {
	html := renderOrg(t, attunedSheet)
	inv := seed(t, html, "dnd-inventory-data")
	found := map[string]map[string]interface{}{}
	for _, box := range inv["containers"].([]interface{}) {
		b := box.(map[string]interface{})
		for _, e := range b["entries"].([]interface{}) {
			entry := e.(map[string]interface{})
			found[entry["name"].(string)] = entry
		}
	}

	potion := found["Potion of Healing"]
	if potion == nil {
		t.Fatal("no potion in the bag")
	}
	use, _ := potion["use"].(map[string]interface{})
	if use == nil {
		t.Fatalf("the potion does not say what drinking it does: %v", potion)
	}
	if use["verb"] != "Drink" {
		t.Errorf("verb = %v, want Drink", use["verb"])
	}
	// Four strengths, so the page asks which one rather than guessing.
	variants, _ := use["variants"].([]interface{})
	if len(variants) != 4 {
		t.Errorf("variants = %v", variants)
	}

	// A longsword is priced, so it can be sold, and half of 15 gp is 7 gp 5 sp.
	sword := found["Longsword"]
	if sword == nil {
		t.Fatal("no longsword")
	}
	if sword["price"] != "15 gp" {
		t.Errorf("price = %v, want 15 gp", sword["price"])
	}
	if sword["sale"] != "7 gp 5 sp" {
		t.Errorf("sale = %v, want 7 gp 5 sp", sword["sale"])
	}
	// And nothing a sword does can be drunk.
	if sword["use"] != nil {
		t.Errorf("the longsword has an effect: %v", sword["use"])
	}
}

// The halo round the edge of the page carries the band the hit points fall in
// from the moment the sheet loads, so an exported sheet with no server behind
// it still shows the right mood.
func TestSheetHaloCarriesTheMood(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	if !strings.Contains(html, `id="page-halo"`) {
		t.Fatal("no halo on the page")
	}
	if !strings.Contains(html, `data-level="dying"`) {
		t.Error("the halo does not know the character is nearly out")
	}
	well := renderOrg(t, strings.Replace(dyingSheet,
		":DND_HP_CURRENT:   0", ":DND_HP_CURRENT:   28", 1))
	if !strings.Contains(well, `data-level="hale"`) {
		t.Error("a character at full hit points does not read as hale")
	}
}

// A character with temporary hit points, which the bar has to show as hit
// points standing in front of their own rather than as a raised ceiling.
const tempHitPointSheet = `* Lyra
   :PROPERTIES:
   :DND_NAME:         Lyra
   :DND_RACE:         human
   :DND_CLASSES:      fighter::3
   :DND_LEVEL:        3
   :DND_CON:          14
   :DND_HP_MAX:       28
   :DND_HP_CURRENT:   14
   :DND_HP_TEMP:      6
   :END:
`

func TestSheetDrawsTemporaryHitPointsOnTheBar(t *testing.T) {
	html := renderOrg(t, tempHitPointSheet)
	if !strings.Contains(html, `id="hp-temp-fill"`) {
		t.Fatalf("the bar should carry a temporary hit point segment")
	}
	// Half the bar is the character's own hit points, and the temporary ones
	// start where those end rather than overlapping them.
	if !strings.Contains(html, `style="left:50%;width:21%"`) {
		t.Fatalf("the segment should start at the end of the hit points:\n%s",
			clip(html[strings.Index(html, `id="hp-temp-fill"`):]))
	}
	if strings.Contains(html, `id="hp-temp-fill" hidden`) {
		t.Fatalf("six temporary hit points should be drawn, not hidden")
	}
	// Temp is one of the three things done to hit points, so it wears the
	// same button as Hurt and Heal rather than the inventory one.
	if !strings.Contains(html, `class="hpb temp" data-hp="temp"`) {
		t.Fatalf("the Temp button should be an hpb like Hurt and Heal")
	}
	// And the reading itself is the button that gives them up.
	if !strings.Contains(html, `id="hp-temp-val" data-hp="cleartemp"`) {
		t.Fatalf("the temporary hit point reading should clear them when pressed")
	}
}

func TestSheetHidesTheTemporarySegmentWhenThereIsNone(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	if !strings.Contains(html, `id="hp-temp-fill" hidden`) {
		t.Fatalf("with no temporary hit points the segment should be hidden")
	}
	if !strings.Contains(html, `id="hp-temp-val" data-hp="cleartemp"`) {
		t.Fatalf("the reading should still be there, just with nothing to clear")
	}
	if !strings.Contains(html, `disabled`) {
		t.Fatalf("with nothing to give up the reading should not be pressable")
	}
}

func TestSheetHasAnUndoInTheMenu(t *testing.T) {
	html := renderOrg(t, tempHitPointSheet)
	if !strings.Contains(html, `id="sheet-undo"`) {
		t.Fatalf("the dice and session menu should carry an Undo")
	}
	if !strings.Contains(html, `id="sheet-undo-what"`) {
		t.Fatalf("Undo should have room to say what it will take back")
	}
	if !strings.Contains(html, "'/dnd/undo'") {
		t.Fatalf("the page should press /dnd/undo")
	}
}

// A character who is poisoned and exhausted, resistant to fire and vulnerable
// to cold: everything the conditions and the damage types have to reach.
const afflictedSheet = `* Lyra
   :PROPERTIES:
   :DND_NAME:            Lyra
   :DND_RACE:            human
   :DND_CLASSES:         fighter::4
   :DND_LEVEL:           4
   :DND_CON:             14
   :DND_HP_MAX:          36
   :DND_HP_CURRENT:      22
   :DND_CONDITIONS:      poisoned, exhaustion::3
   :DND_RESISTANCES:     fire
   :DND_VULNERABILITIES: cold
   :END:
`

func TestSheetCarriesWhatConditionsDoToARoll(t *testing.T) {
	html := renderOrg(t, afflictedSheet)
	got := seed(t, html, "dnd-defenses-data")
	advice, ok := got["advice"].(map[string]interface{})
	if !ok {
		t.Fatalf("the page should be handed the roll advice")
	}
	attack, _ := advice["attack"].(map[string]interface{})
	if attack["lean"] != "disadvantage" {
		t.Fatalf("poisoned and exhausted is disadvantage on attacks, got %v", attack["lean"])
	}
	why, _ := attack["why"].(string)
	if !strings.Contains(why, "Poisoned") || !strings.Contains(why, "Exhaustion 3") {
		t.Fatalf("the card needs both reasons, got %q", why)
	}
	// And every rollable has to say what kind of roll it is, or the advice
	// has nothing to attach itself to.
	for _, want := range []string{
		`data-roll-as="attack"`,
		`data-roll-as="save" data-ability="`,
		`data-roll-as="check" data-ability="`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("no rollable carries %s", want)
		}
	}
	if !strings.Contains(html, `id="roll-omen"`) && !strings.Contains(html, "roll-omen") {
		t.Fatalf("the page should carry the mark that follows the mouse")
	}
}

func TestSheetOffersDamageTypesOnTheHurtButton(t *testing.T) {
	html := renderOrg(t, afflictedSheet)
	if !strings.Contains(html, `id="hp-type"`) {
		t.Fatalf("the Hurt row should offer a damage type")
	}
	got := seed(t, html, "dnd-defenses-data")
	types, _ := got["damageTypes"].([]interface{})
	if len(types) < 13 {
		t.Fatalf("the page needs the ruleset's damage types, got %d", len(types))
	}
	// The wash the blow leaves, and the colours it is washed in.
	if !strings.Contains(html, "hit-wash") || !strings.Contains(html, "DAMAGE_LOOK") {
		t.Fatalf("a hit should colour the page")
	}
}

// A wizard with one evocation prepared, which is all the school flourish
// needs to get onto a cast button.
const wizardSheet = `* Mira
   :PROPERTIES:
   :DND_NAME:      Mira
   :DND_RACE:      human
   :DND_CLASSES:   wizard::5
   :DND_LEVEL:     5
   :DND_INT:       17
   :DND_HP_MAX:    27
   :END:

** Spellcasting

*** 3rd Level
| Prep | Spell    | Time     | Range    | Comp    | Duration      | Notes |
|------+----------+----------+----------+---------+---------------+-------|
| X    | Fireball | 1 action | 150 feet | V, S, M | Instantaneous |       |
`

func TestSheetGivesEachSpellSchoolItsOwnFlourish(t *testing.T) {
	html := renderOrg(t, wizardSheet)
	if !strings.Contains(html, `data-school="Evocation"`) {
		t.Fatalf("the cast button should carry the spell's school")
	}
	if !strings.Contains(html, "spell-canvas") {
		t.Fatalf("the flourish needs a layer of its own")
	}
	for _, school := range []string{"abjuration", "conjuration", "divination",
		"enchantment", "evocation", "illusion", "necromancy", "transmutation"} {
		if !strings.Contains(html, school+": {") {
			t.Fatalf("no flourish for %s", school)
		}
	}
	// The dice a spell throws carry its school with them, so the flourish
	// happens where they strike the page as well as at the button.
	if !strings.Contains(html, "{ school: spec.school,") {
		t.Fatalf("the cast should hand its school to the dice")
	}
	if !strings.Contains(html, "DiceBoard.prototype.onPage") {
		t.Fatalf("a bouncing die has to be able to say where it is on the page")
	}
}

func TestSheetPortraitReactsToTheHitPoints(t *testing.T) {
	html := renderOrg(t, afflictedSheet)
	if !strings.Contains(html, "portrait-cracks") {
		t.Fatalf("the portrait should be able to crack")
	}
	for _, cls := range []string{".portrait.hp-bloodied", ".portrait.hp-down",
		".portrait.hp-dead", ".portrait.inspired"} {
		if !strings.Contains(html, cls) {
			t.Fatalf("no styling for %s", cls)
		}
	}
	if !strings.Contains(html, "function paintPortrait") {
		t.Fatalf("something has to move it")
	}
}

func TestSheetHasACommandPalette(t *testing.T) {
	html := renderOrg(t, afflictedSheet)
	if !strings.Contains(html, `id="cmdk"`) && !strings.Contains(html, "'cmdk'") {
		t.Fatalf("the sheet should carry a command palette")
	}
	// The matcher is FuzzyScore from fuzzy.go said again in javascript, and
	// the two have to keep agreeing: the same letters must find the same
	// spell here as in the terminal chooser.
	for _, want := range []string{"function fuzzyScore", "function fuzzyAll",
		"function palIndex", "function palRun"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the palette is missing %s", want)
		}
	}
	// It indexes the page's own controls rather than a list kept beside
	// them, which is what stops it going stale.
	for _, want := range []string{`.rollable[data-label]`, `.cast-btn[data-spell]`,
		`.ib.use[data-act="use"]`, `.feature[data-uses]`} {
		if !strings.Contains(html, want) {
			t.Fatalf("the palette does not index %s", want)
		}
	}
	if !strings.Contains(html, "openRest('short')") ||
		!strings.Contains(html, "undoLast()") {
		t.Fatalf("the palette should offer the sheet's own commands")
	}
}

func TestSheetCanThrowANoteAway(t *testing.T) {
	html := renderOrg(t, afflictedSheet)
	// The button sits beside Edit on a note that is already in the file.
	if !strings.Contains(html, `class="note-del"`) {
		t.Fatalf("a note in the file should offer a Del beside its Edit")
	}
	// And it asks before it does it, in the card rather than in a dialog
	// box, so the note it is about to throw away is still readable.
	for _, want := range []string{"function askNoteDelete", "function unaskNoteDelete",
		"function doNoteDelete", "Throw this note away?"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the delete is missing %s", want)
		}
	}
	// The stamp goes with the request: deleting shifts every note after it
	// up by one, so a stale page must not be able to delete the wrong one.
	if !strings.Contains(html, "'?was=' + encodeURIComponent(at.note.time") {
		t.Fatalf("the delete should send the stamp it believes the note carries")
	}
	if !strings.Contains(html, "api('DELETE', '/dnd/play/session/'") {
		t.Fatalf("the delete should go through the session note endpoint")
	}
}

func TestSheetCanThrowARollAway(t *testing.T) {
	html := renderOrg(t, afflictedSheet)
	if !strings.Contains(html, `class="roll-del"`) {
		t.Fatalf("a recorded roll should offer a Del of its own")
	}
	// It asks in the row, so the roll it is about to throw away stays
	// readable while the question is being answered.
	for _, want := range []string{"function askRollDelete", "function unaskRollDelete",
		"function doRollDelete", "function rollRowOf"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the roll delete is missing %s", want)
		}
	}
	// A roll is found by its label the way a note is found by its stamp.
	if !strings.Contains(html, "'?was=' + encodeURIComponent(at.roll.label") {
		t.Fatalf("the delete should send the label it believes the roll carries")
	}
	if !strings.Contains(html, "'/roll/' + at.index") {
		t.Fatalf("the delete should go through the session roll endpoint")
	}
}

func TestSheetCanTakeAnInventoryLineOffTheSheet(t *testing.T) {
	html := renderOrg(t, afflictedSheet)
	if !strings.Contains(html, `data-act="delete"`) {
		t.Fatalf("an inventory line should offer a Del")
	}
	// It has to say what makes it different from Drop, or the two buttons
	// are the same button.
	if !strings.Contains(html, "written to your inventory history - use Drop") {
		t.Fatalf("the delete should say it leaves no history")
	}
	if !strings.Contains(html, "body.action = 'delete'") {
		t.Fatalf("the modal should post the delete action")
	}
	// The hidden attribute has to actually hide: the delete has no quantity
	// and no destination, and display:flex would show both anyway.
	if !strings.Contains(html, ".inv-row[hidden] { display: none; }") {
		t.Fatalf("a hidden row must be hidden")
	}
}

func TestSheetKeepsTheNewlinesANoteWasTypedWith(t *testing.T) {
	html := renderOrg(t, afflictedSheet)
	// Org flows consecutive prose into one paragraph; a note typed as four
	// short lines means four lines.
	if !strings.Contains(html, "}).join('<br>') + '</p>'") {
		t.Fatalf("the note renderer should keep the line breaks")
	}
	if !strings.Contains(html, "var ORG_BREAK") {
		t.Fatalf("a trailing backslash pair is markup, not text, and should be dropped")
	}
}

func TestSheetGivesDamageItsOwnElement(t *testing.T) {
	html := renderOrg(t, wizardSheet)
	// The dice have to know what they are the damage of before they can do
	// anything about it when they land.
	if !strings.Contains(html, `data-damage-type="{{ a.type }}"`) &&
		!strings.Contains(html, `data-damage-type="fire"`) {
		t.Fatalf("a damage roll should say what kind of damage it is")
	}
	for _, want := range []string{"DiceBoard.prototype.elemental",
		"DiceBoard.prototype.drawFaceted", "DiceBoard.prototype.drawFlame",
		"DiceBoard.prototype.drawStrike", "DiceBoard.prototype.drawBeam",
		"DiceBoard.prototype.drawCrest", "DiceBoard.prototype.projectUp"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the elemental layer is missing %s", want)
		}
	}
	// Every damage type the rules have gets something; a roll that is not
	// damage at all gets nothing.
	for _, dt := range []string{"acid", "bludgeoning", "cold", "fire", "force",
		"lightning", "necrotic", "piercing", "poison", "psychic", "radiant",
		"slashing", "thunder"} {
		if !strings.Contains(html, dt+": { kind:") {
			t.Fatalf("no element for %s", dt)
		}
	}
	// The camera looks straight down for dice, which would flatten anything
	// standing on the page - the elemental layer leans it back instead.
	if !strings.Contains(html, "var ELEM_LEAN") {
		t.Fatalf("a crystal projected straight down is a smudge")
	}
}

func TestSheetHasALevelUpTray(t *testing.T) {
	html := renderOrg(t, afflictedSheet)
	for _, want := range []string{"lvlTab.id = 'lvl-tab'", "lvlTray.id = 'lvl-tray'",
		"function buildLevelUi", "function lvlAsk", "function lvlBack"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the level up tray is missing %s", want)
		}
	}
	// It walks the engine's own prompts rather than knowing any rules, and
	// the flow is stateless - every answer goes with every call.
	if !strings.Contains(html, "api('POST', '/dnd/levelup'") {
		t.Fatalf("the tray should walk the levelup endpoint")
	}
	if !strings.Contains(html, "answers: LVL.answers") ||
		!strings.Contains(html, "seed: LVL.seed") {
		t.Fatalf("every answer and the seed have to go with every call")
	}
	// Nothing is written until the last call says so.
	if !strings.Contains(html, "commit: !!commit") {
		t.Fatalf("the tray must not write until it is told to")
	}
	if !strings.Contains(html, "lvlAsk(true)") {
		t.Fatalf("there should be a commit step")
	}
}

func TestSheetElementsAreFacetedAndWatery(t *testing.T) {
	html := renderOrg(t, wizardSheet)
	// Crystals and boulders are built from facets rather than being cones
	// and blobs: a plain taper is the same shape however many you draw.
	// One banded solid does the crystals, the blades and the boulders -
	// they differ only in their band table.
	for _, want := range []string{"function facetsOf", "function solidOf",
		"var BANDS_ICE", "var BANDS_BLADE", "var ROCK_BANDS"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the faceted solids are missing %s", want)
		}
	}
	// And the noise everything organic leans on, so a flame is not a
	// triangle and a shaft of light has grain in it.
	for _, want := range []string{"function noise1", "function fbm1"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the noise is missing %s", want)
		}
	}
	// Water, earth, healing and holding are not damage types the rules
	// have, so they come out of the spell's own name.
	if !strings.Contains(html, "var SPELL_ELEMENTS") ||
		!strings.Contains(html, "function spellElement") {
		t.Fatalf("a spell should be able to be recognised by name")
	}
	for _, want := range []string{"mend: { kind: 'mend'", "bind: { kind: 'bind'",
		"DiceBoard.prototype.drawSigil", "DiceBoard.prototype.drawPlus",
		"DiceBoard.prototype.drawRope"} {
		if !strings.Contains(html, want) {
			t.Fatalf("healing and holding need a flourish: missing %s", want)
		}
	}
	// A spell with nothing else to show for itself still gets a circle of
	// light on the ground with writing round it, and there is more than
	// one of those.
	for _, want := range []string{"circle: { kind: 'circle'",
		"DiceBoard.prototype.drawCircleSigil", "var CIRCLE_STYLES",
		"var RUNE_STROKES"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the magic circle is missing %s", want)
		}
	}
	// Arrows and bangs.
	for _, want := range []string{"missile: { kind: 'missile'", "blast: { kind: 'boom'",
		"DiceBoard.prototype.drawMissile", "DiceBoard.prototype.drawBoom",
		"DiceBoard.prototype.drawLobe", "var BOOM_TINTS"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missiles and explosions are missing %s", want)
		}
	}
	// The same fire twice running should not be the same fire.
	if !strings.Contains(html, "function pickLook") {
		t.Fatalf("an element should be able to carry variants")
	}
	// A rope should come out of the paper rather than hovering over it,
	// and some of them are vines.
	if !strings.Contains(html, "DiceBoard.prototype.drawBurst") {
		t.Fatalf("a rope should tear its way out of the sheet")
	}
	for _, lay := range []string{"'twist'", "'links'", "'leaves'"} {
		if !strings.Contains(html, "lay: "+lay) {
			t.Fatalf("no binding variant for %s", lay)
		}
	}
	// And the dice must not land in the same corner of the sheet all
	// evening, or everything that happens where they land does too.
	if !strings.Contains(html, "best.score * 0.72") {
		t.Fatalf("the landing spot should be picked from any clear patch")
	}
	if !strings.Contains(html, "water: { kind: 'wave'") ||
		!strings.Contains(html, "DiceBoard.prototype.drawCrest") {
		t.Fatalf("there should be a wave to break over the dice")
	}
	// A spell that rolls nothing still happened, so it gets a moment
	// somewhere on the parchment.
	if !strings.Contains(html, "DiceBoard.prototype.elsewhere") {
		t.Fatalf("a spell with no dice should still show something")
	}
	// And a natural 1 is a record scratch rather than two sad beeps.
	if !strings.Contains(html, "function scratchBuffer") {
		t.Fatalf("the fumble should sound like a needle, not an atari")
	}
}

func TestUndoReachesTheChangesThatLeaveNoHistory(t *testing.T) {
	html := renderOrg(t, afflictedSheet)
	// The page side is unchanged - one button, one endpoint - but it now
	// has to be able to say what a journalled change was.
	if !strings.Contains(html, "function undoLast") {
		t.Fatalf("the undo button should still be there")
	}
	if !strings.Contains(html, "'/dnd/undo'") {
		t.Fatalf("undo should go through the one endpoint")
	}
}

// Whether the roll tray pops itself open is a setting with three answers,
// because casting is the one that gets tiresome: a spell rolls to hit and
// for damage at once, and in a fight that is most of what you do.
func TestRollTrayPopIsOptional(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, want := range []string{`<option value="all">`, `<option value="nocast">`,
		`<option value="off">`, "function shouldPop", "orgs.dnd.pop"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the tray's pop-open setting is missing %s", want)
		}
	}
	// And both roll paths must go through it, or the setting only half works.
	if strings.Contains(html, "autoBox.checked") {
		t.Fatalf("a roll path still consults the old checkbox")
	}
	if n := strings.Count(html, "shouldPop(true)"); n != 1 {
		t.Fatalf("the cast path should ask shouldPop(true) once, got %d", n)
	}
	if n := strings.Count(html, "shouldPop(false)"); n != 1 {
		t.Fatalf("the plain roll path should ask shouldPop(false) once, got %d", n)
	}
}

// The level up tray says how much further it is, because that is the one
// number a player actually asks for and the ring round the portrait only
// says where they stand.
func TestLevelTrayShowsExperienceToGo(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, want := range []string{"function xpBar", "var XP_TABLE", "function xpFloor",
		"lvl-xp-bar", `data-xp="`, `data-next-xp="`} {
		if !strings.Contains(html, want) {
			t.Fatalf("the level tray's experience line is missing %s", want)
		}
	}
	if !strings.Contains(html, "to level ") {
		t.Fatalf("the tray never says how far it is to the next level")
	}
}

// The timeline reads the session logs back and works out the shape of the
// evening from them. What it must not do is store any of that: the session
// file is the record, and a second copy of it would go stale.
func TestSessionTimelineTab(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, want := range []string{`data-view="timeline"`, `id="view-timeline"`,
		"function tlEvents", "function tlFight", "function tlKind",
		"function tlNatural", "function tlUnwrap", "function renderTimeline",
		"var TL_GAP", "var TL_LEAST", "var TL_SCENE"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the timeline is missing %s", want)
		}
	}
}

// The combat tracker is the one part of the sheet that is about the table
// rather than about the character, so it is kept in the browser and written
// to no org file at all.
func TestCombatTrackerTab(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, want := range []string{`data-view="combat"`, `id="view-combat"`,
		"function combatStart", "function combatStep", "function combatTick",
		"function combatOrder", "function combatAddClock", "function combatRollMine",
		"orgs.dnd.combat"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the combat tracker is missing %s", want)
		}
	}
	// A fight must never reach the character's file.
	for _, never := range []string{"/dnd/combat", "dnd/tracker"} {
		if strings.Contains(html, never) {
			t.Fatalf("the tracker should talk to no server endpoint, found %s", never)
		}
	}
}

// Lightning is built out of a seeded path plus forks, so two strikes are
// never the same shape, and it has more than one kind of weather.
func TestLightningHasVariants(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, want := range []string{"DiceBoard.prototype.boltPath",
		"DiceBoard.prototype.boltForks", "DiceBoard.prototype.strokePath",
		"ribbon: true", "beads: true"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the strike is missing %s", want)
		}
	}
}

// A vine is a stem with leaves and tendrils on it, not a green arc with
// ellipses stuck to the side.
func TestVinesHaveLeavesAndTendrils(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, want := range []string{"DiceBoard.prototype.drawLeaf", "var GREENS",
		"var SHADES", "leaf: '#"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the vine is missing %s", want)
		}
	}
}

// Every flourish that has a voice must have it wired through one table,
// so a new kind of effect is silent by default rather than borrowing
// somebody else's sound.
func TestElementalVoices(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, want := range []string{"function crackle", "function wash", "function choir",
		"function rumble", "function thunder", "function noiseBed",
		"var ELEM_VOICE", "function elemVoice", "elemVoice(look.kind, look)"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the elemental voices are missing %s", want)
		}
	}
	// Each of the five kinds that has a sound must name it in the table.
	for _, pair := range []string{"blaze: crackle", "wave: wash", "circle: choir",
		"stones: rumble", "bolt: thunder"} {
		if !strings.Contains(html, pair) {
			t.Fatalf("the voice table is missing %s", pair)
		}
	}
	// And they must all go through the sound switch like everything else.
	if !strings.Contains(html, "if (!play || !SOUND.on) { return; }") {
		t.Fatalf("a flourish could make a noise with the sound turned off")
	}
}

// The particles that go with each effect. These are the parts that carry
// on after the big shape has gone, so a missing one shows up as an effect
// that simply stops.
func TestElementalParticles(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, want := range []string{"case 'ember':", "case 'spark':", "case 'mote':",
		"case 'spot':", "case 'flash':"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the particle layer is missing %s", want)
		}
	}
	// A droplet that lands has to leave a mark, or the splash is only half
	// drawn: the bit you go on seeing is the wet paper.
	if !strings.Contains(html, "what: 'spot'") {
		t.Fatalf("a landing droplet leaves nothing behind")
	}
	// Fire throws embers and lightning throws sparks.
	if !strings.Contains(html, "what: 'ember'") {
		t.Fatalf("the fire has no embers")
	}
	if !strings.Contains(html, "what: 'spark'") || !strings.Contains(html, "what: 'mote'") {
		t.Fatalf("the strike has no sparks or motes")
	}
}

// A full-screen change of brightness several times a fight is exactly what
// a reader who asked for less motion asked not to have.
func TestStrikeFlashRespectsReducedMotion(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	if !strings.Contains(html, "var FLASH_OFF") ||
		!strings.Contains(html, "if (FLASH_OFF) { break; }") {
		t.Fatalf("the strike's flash is not gated on prefers-reduced-motion")
	}
	if !strings.Contains(html, "prefers-reduced-motion: reduce") {
		t.Fatalf("nothing asks the browser about reduced motion")
	}
}

// Fire is a crowd of unequal flames, not a row of equal ones, and there
// is more than one kind of fire.
func TestFireHasVariantsAndEmbers(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	if !strings.Contains(html, "ember: '#") {
		t.Fatalf("the fire look carries no ember colour")
	}
	// The flame silhouette multiplies its octaves rather than adding them,
	// which is what lets it pinch into separate tongues.
	if !strings.Contains(html, "lump *= ") {
		t.Fatalf("the flame is back to additive noise and will not pinch")
	}
	if !strings.Contains(html, "embers:") {
		t.Fatalf("no fire variant says how many embers it throws")
	}
}

// Every voice has several of itself. A sound that is identical on every
// cast stops being heard after the third one.
func TestVoicesHaveVariants(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	if !strings.Contains(html, "function pickOne") {
		t.Fatalf("nothing chooses between sound variants")
	}
	for _, table := range []string{"var FIRES", "var WAVES", "var CHOIRS",
		"var STONES", "var SKIES", "var SWINGS", "var STORMS", "var LASHINGS"} {
		if !strings.Contains(html, table) {
			t.Fatalf("no variant table %s", table)
		}
	}
	// The choir is the one that was asked for more layers: a sub octave
	// under the chord, three voices to a note, and a shimmer over it.
	for _, want := range []string{"if (v.sub) { notes.unshift", "v.shimmer",
		"[[-7, 'sine'], [0, 'triangle'], [8, 'sine']]"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the choir is missing a layer: %s", want)
		}
	}
	// Six-note voicings, not four.
	if !strings.Contains(html, "steps: [0, 7, 12, 16, 19, 26]") {
		t.Fatalf("the choir chord is not the wide voicing")
	}
}

// The flourishes that grew a voice of their own.
func TestMoreElementalVoices(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, want := range []string{"function creak", "function howl",
		"function murmur", "function whoosh"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing voice %s", want)
		}
	}
	for _, pair := range []string{"blizzard: howl", "hand: murmur", "slash: whoosh"} {
		if !strings.Contains(html, pair) {
			t.Fatalf("the voice table is missing %s", pair)
		}
	}
	// A binding sounds like what it is made of, so it is handed the lay
	// rather than a loudness.
	if !strings.Contains(html, "if (kind === 'bind') { arg = look && look.lay; }") {
		t.Fatalf("the binding sound is not told what the binding is made of")
	}
	for _, lay := range []string{"twist:", "links:", "leaves:"} {
		if !strings.Contains(html, "    "+lay) {
			t.Fatalf("no lashing sound for %s", lay)
		}
	}
}

// A storm is a storm. Sleet storm does no damage at all, so without a name
// match it falls through to the magic circle, which is the one thing it is
// not.
func TestStormIsNotACircle(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	if !strings.Contains(html, "as: 'blizzard'") {
		t.Fatalf("nothing routes a storm to the blizzard")
	}
	for _, want := range []string{"blizzard: { kind: 'blizzard'", "case 'blizzard':",
		"case 'sleet':", "case 'swirl':"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the blizzard is missing %s", want)
		}
	}
	// The name has to be caught before anything else claims it.
	circle := strings.Index(html, "circle: { kind: 'circle'")
	storm := strings.Index(html, "as: 'blizzard'")
	if storm < 0 || circle < 0 {
		t.Fatalf("could not find both the storm and the circle")
	}
}

// Mage hand: a hand, in a colour, doing something.
func TestGhostHand(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, want := range []string{"var HAND_FINGERS", "var HAND_GESTURES",
		"var HAND_HUES", "DiceBoard.prototype.drawHand", "case 'hand':",
		"hand: { kind: 'hand'"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the ghost hand is missing %s", want)
		}
	}
	// It is a mesh now, built from the same tubes the cat is: a palm,
	// a thenar pad and five fingers of three bones each.
	if !strings.Contains(html, "function handMesh") {
		t.Fatalf("the hand is not built as a mesh")
	}
	// More than one thing to do with itself.
	for _, move := range []string{"'drift'", "'wave'", "'beckon'", "'clench'",
		"'turn'", "'lift'"} {
		if !strings.Contains(html, "move: "+move) {
			t.Fatalf("no hand gesture %s", move)
		}
	}
}

// A sword swing is a cut across the page, not a ring of little blades
// standing up on it.
func TestWeaponSlash(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, want := range []string{"slashing: { kind: 'slash'",
		"DiceBoard.prototype.drawSlash", "DiceBoard.prototype.drawScar",
		"case 'slash':", "case 'scar':"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the slash is missing %s", want)
		}
	}
	// And the cut it leaves has to lie on the paper with the other flat
	// marks, or it draws over the dice.
	if !strings.Contains(html, "it.what === 'scar'") ||
		!strings.Contains(html, "? flat : up).push(it)") {
		t.Fatalf("the cut is not sorted as something lying on the page")
	}
	// A weapon with no damage type written down still swings something.
	if !strings.Contains(html, "as: 'slashing'") {
		t.Fatalf("nothing routes a sword by name")
	}
}

// A hand you would recognise as one. It used to be a flat drawing with
// nails and veins inked onto it; it is a mesh now, and what carries the
// recognition is the geometry - a palm flattened front to back, the
// thenar pad that is most of a hand's width, and five fingers whose
// three bones each bend further than the last.
func TestGhostHandAnatomy(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, want := range []string{"var HAND_TAPER", "var HAND_FINGERS",
		"var HAND_GESTURES", "// The thenar pad"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the hand is missing %s", want)
		}
	}
	// The curl still comes straight out of the gesture table, so a
	// gesture is six numbers rather than six drawings.
	if !strings.Contains(html, "var curl = Math.min(1.25, g.curl[i]) * 0.62;") {
		t.Fatalf("the hand's gestures no longer drive its fingers")
	}
}

// Missiles: seven kinds of volley, and every missile in one still rolls
// its own head and its own trimmings.
func TestMissileVariety(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	if !strings.Contains(html, "var MISSILE_HEADS") {
		t.Fatalf("there is only one kind of missile head")
	}
	for _, want := range []string{"'arrow'", "'orb'", "'lance'", "'shard'", "'star'"} {
		if !strings.Contains(html, "head: "+want) && !strings.Contains(html, want) {
			t.Fatalf("no missile head %s", want)
		}
	}
	// The layers that were added over the plain ribbon.
	for _, want := range []string{"// The smear:", "spirals", "it.chevrons",
		"if (it.burns)", "it.grit", "it.shed"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the missile is missing its %s layer", want)
		}
	}
}

// Some skills have something to show for themselves. Most deliberately
// do not: there is no drawing of an Athletics check that is not silly,
// and the ones with a flourish are worth looking at because the rest
// have none.
func TestSkillFlourishes(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	if !strings.Contains(html, "var SKILL_ELEMENTS") ||
		!strings.Contains(html, "function skillElement") {
		t.Fatalf("nothing maps a skill to a flourish")
	}
	for _, pair := range []string{"religion: 'mend'", "medicine: 'mend'",
		"investigation: 'glass'", "nature: 'bloom'", "insight: 'tome'",
		"history: 'tome'", "performance: 'notes'"} {
		if !strings.Contains(html, pair) {
			t.Fatalf("the skill table is missing %s", pair)
		}
	}
	for _, want := range []string{"DiceBoard.prototype.drawGlass",
		"DiceBoard.prototype.drawBloom", "DiceBoard.prototype.drawTome",
		"DiceBoard.prototype.drawNote", "case 'glass':", "case 'bloom':",
		"case 'tome':", "case 'note':"} {
		if !strings.Contains(html, want) {
			t.Fatalf("a skill flourish is missing %s", want)
		}
	}
	// The glass has to know what the d20 did, because it breaks on a bad
	// roll and sparkles on a good one.
	if !strings.Contains(html, "quality: result.kind === 'check' ? natOf(result) : 0") {
		t.Fatalf("the roll's own number never reaches the flourish")
	}
	if !strings.Contains(html, "this.quality = opts.quality || 0;") {
		t.Fatalf("the board does not carry the roll's quality")
	}
	// A skill with no entry gets the dice and nothing else.
	if strings.Contains(html, "athletics:") || strings.Contains(html, "stealth:") {
		t.Fatalf("a skill that should have no flourish has one")
	}
}

// The named-spell flourishes: a cat, a thicket, a shield, a dome, and a
// figure that assembles itself out of the sheet.
func TestNamedSpellFlourishes(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, want := range []string{
		"DiceBoard.prototype.drawPanther", "DiceBoard.prototype.drawThorn",
		"DiceBoard.prototype.drawTuft", "DiceBoard.prototype.drawShield",
		"DiceBoard.prototype.drawDome", "DiceBoard.prototype.drawCairn",
		"DiceBoard.prototype.drawHexRing",
		"case 'panther':", "case 'thorns':", "case 'shield':", "case 'dome':",
		"case 'cairn':", "case 'hexring':"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %s", want)
		}
	}
	for _, route := range []string{"as: 'panther'", "as: 'thorns'", "as: 'stoneskin'",
		"as: 'barkskin'", "as: 'shield'", "as: 'dome'"} {
		if !strings.Contains(html, route) {
			t.Fatalf("nothing routes %s", route)
		}
	}
	// Spike growth has to be caught before the earth regex claims it for
	// its `spike\w*`, and protection before it falls to the circle.
	spike := strings.Index(html, "as: 'thorns'")
	earth := strings.Index(html, "as: 'stones'")
	if spike < 0 || earth < 0 || spike > earth {
		t.Fatalf("spike growth would be claimed by the earth pattern")
	}
	// The shield stops the swing rather than letting it through.
	if !strings.Contains(html, "stopAt: 0.5") || !strings.Contains(html, "it.stopAt") {
		t.Fatalf("the blade is not stopped on the shield")
	}
	// Eight boards and five patterns, rolled separately.
	for _, shape := range []string{"heater:", "round:", "kite:", "buckler:",
		"tower:", "pavise:", "targe:", "hoplon:"} {
		if !strings.Contains(html, "    "+shape) {
			t.Fatalf("no shield shape %s", shape)
		}
	}
	if !strings.Contains(html, "patterns: ['hex', 'rings', 'scan', 'runes', 'star']") {
		t.Fatalf("the shield has no bank of energy patterns")
	}
	// The dome holds still and only the light on it moves.
	if !strings.Contains(html, "function domePanels") {
		t.Fatalf("the dome is not built from panels")
	}
	// The cairn's pieces come from all over the sheet, not from the dice.
	if !strings.Contains(html, "rnd(this.w * 0.06, this.w * 0.94)") {
		t.Fatalf("the cairn's stones do not come from across the sheet")
	}
}

// A roll that mattered draws from a small bank, and only at the ends of
// the die: a flourish on every good roll is a flourish on nothing.
func TestSignificantRollFlourish(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	if !strings.Contains(html, "var RITE_BANK") ||
		!strings.Contains(html, "function riteElement") {
		t.Fatalf("no bank of flourishes for a roll that mattered")
	}
	if !strings.Contains(html, "if (nat < 18 && nat > 2) { return ''; }") {
		t.Fatalf("the bank fires on ordinary rolls")
	}
	if !strings.Contains(html, "k !== 'initiative' && k.indexOf('attack') < 0") {
		t.Fatalf("the bank is not limited to attacks and initiative")
	}
}

// The upright frame. Anything flat that stands up facing the reader -
// the glass, the book - uses it, because projecting three points at
// three different heights and making a basis out of them puts a shear
// in the frame.
func TestBillboardFrame(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	if !strings.Contains(html, "DiceBoard.prototype.billboard") {
		t.Fatalf("there is no upright frame helper")
	}
	if !strings.Contains(html, "this.billboard(ctx, it.x, it.y, it.z + bob * s, s)") {
		t.Fatalf("the glass does not use the upright frame")
	}
}

// Nothing in the flourish layer may be defined twice: a stale second
// copy of a drawing routine silently wins, and the first sign of it is
// an effect that stopped responding to edits.
func TestNoDuplicateFlourishRoutines(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, name := range []string{
		"DiceBoard.prototype.drawPanther = function",
		"DiceBoard.prototype.drawCairn = function",
		"DiceBoard.prototype.drawShield = function",
		"DiceBoard.prototype.drawDome = function",
		"DiceBoard.prototype.drawHand = function",
		"DiceBoard.prototype.drawTome = function",
		"DiceBoard.prototype.drawGlass = function",
		"var CAIRN_SLOTS", "var SHIELD_SHAPES", "var RUNE_STROKES",
		"var CIRCLE_STYLES", "var HAND_FINGERS"} {
		if n := strings.Count(html, name); n != 1 {
			t.Fatalf("%s is defined %d times, want 1", name, n)
		}
	}
}

// The tome is a solid, not a pair of surfaces. What tells you a book is
// thick is never the top of the page: it is the fore-edge with every
// leaf showing on it, the square of the board standing proud below, and
// the spine between the two halves.
func TestTomeIsASolid(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, want := range []string{"var THICK", "var BOARD", "function tomeStains",
		"// The fore-edge:", "// And the near face,", "// ---- the spine:",
		"// ---- the ribbon marker:"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the tome is missing %s", want)
		}
	}
	// Age: foxing, water stains, and the corner five hundred years of
	// thumbs have gone through.
	for _, want := range []string{"st.fox", "it.thumb", "it.pageLine", "it.gutter"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the tome's paper is missing %s", want)
		}
	}
	// The stains are fixed when the book is conjured, or they crawl
	// about the page while you are reading it.
	if !strings.Contains(html, "stains: tomeStains(") {
		t.Fatalf("the tome's stains are not fixed at spawn")
	}
	// Runes on both pages and a circle on one.
	if !strings.Contains(html, "RUNE_STROKES[(it.seed + i * 5 + k * 3)") {
		t.Fatalf("the tome has no rune writing")
	}
}

// Six domes that are six different objects, not one object in six
// colours: the panelling, the motion and the height change together.
func TestDomeVariety(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, style := range []string{"'quad'", "'hex'", "'tri'", "'scale'",
		"'staves'", "'cage'"} {
		if !strings.Contains(html, "style: "+style) {
			t.Fatalf("no dome style %s", style)
		}
	}
	for _, motion := range []string{"'both'", "'pulse'", "'scan'", "'storm'"} {
		if !strings.Contains(html, "motion: "+motion) {
			t.Fatalf("no dome motion %s", motion)
		}
	}
	// The geodesic has to tessellate. Alternating one triangle or the
	// other per cell leaves a diamond hole between every pair.
	if !strings.Contains(html, "'triA'") || !strings.Contains(html, "'triB'") {
		t.Fatalf("the geodesic dome does not split its cells in two")
	}
	// Seams have to be ink, not light: a near-white edge on cream
	// parchment is an invisible edge.
	if !strings.Contains(html, "ctx.strokeStyle = it.seam;") {
		t.Fatalf("the dome's seams are not drawn in a saturated ink")
	}
}

// The garden: seven flowers, five habits, five greens, all rolled per
// stem so one clump is not all the same plant.
func TestGardenVariety(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	if !strings.Contains(html, "var FLOWER_FORMS") ||
		!strings.Contains(html, "function petalPath") {
		t.Fatalf("the flowers have no forms")
	}
	for _, f := range []string{"daisy:", "poppy:", "star:", "tulip:",
		"foxglove:", "thistle:", "rose:"} {
		if !strings.Contains(html, "    "+f) {
			t.Fatalf("no flower %s", f)
		}
	}
	for _, form := range []string{"'ray'", "'broad'", "'point'", "'cup'",
		"'bell'", "'spike'"} {
		if !strings.Contains(html, "form === "+form) {
			t.Fatalf("petalPath does not draw %s", form)
		}
	}
	for _, st := range []string{"'plain'", "'twine'", "'briar'", "'fern'", "'grassy'"} {
		if !strings.Contains(html, st) {
			t.Fatalf("no stem habit %s", st)
		}
	}
	// Rolled per stem, not per cast.
	if !strings.Contains(html, "look.forms[Math.floor(Math.random()") ||
		!strings.Contains(html, "look.stems[Math.floor(Math.random()") {
		t.Fatalf("the whole patch would be one plant")
	}
}

// The tome's writing is set as prose: words, spaces, ragged right.
func TestTomeWritingIsProse(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, want := range []string{"var para = nhash", "// A word space,",
		"var right = 0.9 - nhash", "var gw = 0.036 + nhash"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the writing is missing %s", want)
		}
	}
	// A fixed pitch is what made the page read as a spreadsheet.
	if strings.Contains(html, "var lu = 0.14 + k * 0.145;") {
		t.Fatalf("the writing is back on a fixed grid")
	}
}

// Five of the flourishes are real meshes now rather than drawings: a
// rock, a cat, a hand, a shield, and the stones of the cairn.
func TestLowPolyMeshes(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	for _, want := range []string{"function meshBuilder", "function rockMesh",
		"function catMesh", "function handMesh", "function shieldMesh",
		"function rotate3", "function pathPoints", "var ICO",
		"DiceBoard.prototype.drawMesh", "var ROCK_LIGHT"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the mesh layer is missing %s", want)
		}
	}
	// Flat shading off each face's own normal is what makes low poly
	// look low poly - no gradient may creep in.
	if !strings.Contains(html, "dot(d.n, ROCK_LIGHT)") {
		t.Fatalf("the faces are not lit from their own normals")
	}
	// Two modes: stone is solid, anything conjured is a wireframe.
	if !strings.Contains(html, "look.mode === 'wire'") {
		t.Fatalf("there is no wireframe mode")
	}
	// A shield is a dished plate, not a closed solid, so it asks for
	// both sides; everything else is culled.
	if !strings.Contains(html, "look.twoSided") {
		t.Fatalf("nothing can ask for two-sided faces")
	}
	// The camera looks down at the page, so a mesh built with its own
	// z as up has to be pitched a quarter turn or it is seen from
	// above and reads as a column.
	if n := strings.Count(html, "Math.PI / 2"); n < 3 {
		t.Fatalf("the meshes are not stood up on the screen (%d)", n)
	}
}

// The dome turns on its own axis and its panels bloom on their own.
func TestDomeTurnsAndPulses(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	if !strings.Contains(html, "var turn = age * it.turn;") {
		t.Fatalf("the dome does not turn")
	}
	// The geometry has to turn, not just the light on it: the seams
	// themselves go round.
	if !strings.Contains(html, "Math.cos(a + turn) * rr") {
		t.Fatalf("only the light turns, not the dome")
	}
	for _, want := range []string{"phase: nhash(", "rate: 1.1 + nhash(",
		"var bloom = Math.pow(beat, 7) * it.bloom;"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the panels do not bloom on their own: %s", want)
		}
	}
}

// Every pane of the session drawer scrolls. The drawer is a fixed
// height, so a night's timeline or a fight with nine combatants in it
// is otherwise unreachable below the fold.
func TestSessionPanesScroll(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	if !strings.Contains(html, ".nd-view {") ||
		!strings.Contains(html, "overflow-y: auto;\n  overscroll-behavior: contain;") {
		t.Fatalf("the session panes do not scroll")
	}
	// The notes pane opts out: it is a split whose halves scroll
	// themselves, and two scrollbars doing different things is worse
	// than one.
	if !strings.Contains(html, "#view-notes { overflow: hidden; }") {
		t.Fatalf("the notes pane should manage its own scrolling")
	}
	// The toolbar stays put while the list moves under it.
	if !strings.Contains(html, ".nd-view > .nd-toolbar") ||
		!strings.Contains(html, "position: sticky") {
		t.Fatalf("the pane toolbars do not stay put")
	}
	// And the play-session popup in the dock.
	if !strings.Contains(html, "#sess-body { max-height: 60vh; overflow-y: auto;") {
		t.Fatalf("the session popup cannot scroll")
	}
}

// Two things about the tube builder that are wrong by default.
func TestTubeFrameAndCaps(t *testing.T) {
	html := renderOrg(t, dyingSheet)
	// A tube running along z has its ring frame guessed, and the guess
	// puts the wide axis on the wrong pair - a flat palm comes out as
	// a vertical slab. Anything not round in cross-section says which
	// way is up.
	if !strings.Contains(html, "var HAND_UP = [0, -1, 0];") ||
		!strings.Contains(html, "tube: function (path, sides, capA, capB, upHint)") {
		t.Fatalf("the tube builder cannot be told which way is up")
	}
	if strings.Count(html, "HAND_UP)") < 3 {
		t.Fatalf("the hand's tubes do not all say which way is up")
	}
	// Caps are one polygon, not a fan: the fan was a ring of slivers
	// and the wireframe's glow pass turned every fingertip into a
	// starburst.
	if !strings.Contains(html, "this.faces.push(rings[0].slice().reverse())") {
		t.Fatalf("the tube caps are fans again")
	}
	// And the shield is dished out of the page, not into it.
	if !strings.Contains(html, "dish * (1 - f * f),") ||
		strings.Contains(html, "-dish * (1 - f * f),") {
		t.Fatalf("the shield is dished the wrong way and faces the paper")
	}
	// A shield lands where the dice land, so it is pulled back on.
	if !strings.Contains(html, "this.w - shr * 1.1") {
		t.Fatalf("the shield is not kept on the page")
	}
}
