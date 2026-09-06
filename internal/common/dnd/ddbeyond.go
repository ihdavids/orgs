package dnd

// D&D Beyond import.
//
// D&D Beyond has no supported public API, so this reads the same json the
// character sheet page reads for itself - character-service's character
// endpoint - and maps it onto a Character. The fetch lives in the cli (it is
// the side that holds the user's cookie); everything here is pure conversion,
// so it works just as well on a json file saved out of a browser.
//
// The mapping is by name. D&D Beyond names its content the way the books do
// and this ruleset ids its content as the slug of that same name, so almost
// everything lands with a straight slug compare; the fuzzy matcher picks up
// the rest ("Crossbow, Light" against "light crossbow"). Anything that still
// does not match is not dropped - it is carried onto the sheet under its own
// name with an empty id and reported as a warning, because a character who
// half arrives is worse than one that arrives with a list of what to check.

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// ----------------------------------------------------------------------------
// The wire format of the import
// ----------------------------------------------------------------------------

// DDBImportRequest hands the server a character-service payload to convert.
// Payload is the raw json exactly as it came back from D&D Beyond; the client
// does the fetching so that no D&D Beyond credential ever reaches the server.
type DDBImportRequest struct {
	Ruleset   string          `json:"ruleset"`
	Payload   json.RawMessage `json:"payload"`
	Filename  string          `json:"filename"`
	Overwrite bool            `json:"overwrite"`
	// Preview converts and returns the character without writing anything.
	Preview bool `json:"preview"`
	// Player overrides the player name, which D&D Beyond only knows as a
	// site username.
	Player string `json:"player"`
}

// DDBImportResponse is the converted character, plus everything the mapping
// was not sure about.
type DDBImportResponse struct {
	Ok        bool       `json:"ok"`
	Filename  string     `json:"filename"`
	Msg       string     `json:"msg"`
	Character *Character `json:"character"`
	Sheet     *Sheet     `json:"sheet"`
	Org       string     `json:"org"`
	Warnings  []string   `json:"warnings"`
}

// ----------------------------------------------------------------------------
// The subset of the character-service payload we read
// ----------------------------------------------------------------------------

// ddbNum is a number out of a D&D Beyond payload. The site is loose about how
// it sends them - a number, a quoted number, or null for "no answer" - and
// which one you get varies by field and by how the character was built. A
// plain int would fail the whole import over a single field the sheet may not
// even use, so this reads all three, and treats anything it cannot read as
// simply absent.
type ddbNum struct {
	set   bool
	value float64
}

func (n *ddbNum) UnmarshalJSON(b []byte) error {
	text := strings.TrimSpace(string(b))
	if text == "" || text == "null" {
		return nil
	}
	text = strings.Trim(text, `"`)
	if text == "" {
		return nil
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		// Not a number at all. Leave it unset rather than refusing the
		// character over one field.
		return nil
	}
	n.set, n.value = true, f
	return nil
}

// Set reports whether a usable number arrived, which is how "0" is told apart
// from "nothing was sent".
func (n ddbNum) Set() bool { return n.set }

// Int is the value rounded towards zero, and 0 when nothing arrived.
func (n ddbNum) Int() int { return int(n.value) }

// Float is the value as it came, for the weights that carry a fraction.
func (n ddbNum) Float() float64 { return n.value }

// ddbEnvelope is the { success, message, data } wrapper the endpoint returns.
// A json file saved from the browser may be either the envelope or the data
// on its own, so ParseDDB accepts both.
type ddbEnvelope struct {
	Success *bool           `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type ddbCharacter struct {
	Name     string `json:"name"`
	Username string `json:"username"`

	AvatarUrl   string          `json:"avatarUrl"`
	Decorations *ddbDecorations `json:"decorations"`

	Faith  string `json:"faith"`
	Age    ddbNum `json:"age"`
	Hair   string `json:"hair"`
	Eyes   string `json:"eyes"`
	Skin   string `json:"skin"`
	Height string `json:"height"`
	Weight ddbNum `json:"weight"`

	Inspiration        bool   `json:"inspiration"`
	BaseHitPoints      ddbNum `json:"baseHitPoints"`
	BonusHitPoints     ddbNum `json:"bonusHitPoints"`
	OverrideHitPoints  ddbNum `json:"overrideHitPoints"`
	RemovedHitPoints   ddbNum `json:"removedHitPoints"`
	TemporaryHitPoints ddbNum `json:"temporaryHitPoints"`

	CurrentXp   ddbNum `json:"currentXp"`
	AlignmentId ddbNum `json:"alignmentId"`

	Stats         []ddbStat `json:"stats"`
	BonusStats    []ddbStat `json:"bonusStats"`
	OverrideStats []ddbStat `json:"overrideStats"`

	Race       *ddbRace       `json:"race"`
	Classes    []ddbClass     `json:"classes"`
	Background *ddbBackground `json:"background"`
	Feats      []ddbFeat      `json:"feats"`

	Traits      *ddbTraits    `json:"traits"`
	Notes       *ddbNotes     `json:"notes"`
	Currencies  *ddbCurrency  `json:"currencies"`
	DeathSaves  *ddbDeath     `json:"deathSaves"`
	SpellSlots  []ddbSlot     `json:"spellSlots"`
	PactMagic   []ddbSlot     `json:"pactMagic"`
	Inventory   []ddbItem     `json:"inventory"`
	Spells      *ddbSpellSets `json:"spells"`
	ClassSpells []ddbClassSpells

	Modifiers *ddbModifiers `json:"modifiers"`
}

type ddbDecorations struct {
	AvatarUrl string `json:"avatarUrl"`
}

type ddbStat struct {
	Id    ddbNum `json:"id"`
	Value ddbNum `json:"value"`
}

type ddbRace struct {
	IsSubRace        bool   `json:"isSubRace"`
	BaseRaceName     string `json:"baseRaceName"`
	FullName         string `json:"fullName"`
	SubRaceShortName string `json:"subRaceShortName"`
}

// ddbNamed is the definition block that hangs off a class, a feat, an item or
// a spell. Only the fields the conversion actually reads are declared: every
// field named here is a type this has to guess right, and guessing wrong on
// one the sheet does not even use would fail the whole import.
type ddbNamed struct {
	Name  string `json:"name"`
	Level ddbNum `json:"level"`
	// Weight is the weight of one bundle, not of one unit: a bundle of 20
	// arrows weighs 1 lb.
	Weight ddbNum `json:"weight"`
	// BundleSize is how many of a thing D&D Beyond counts as one entry in the
	// books - 20 arrows, a bag of 1,000 ball bearings. Its quantity is in
	// single units, so a quiver holding one bundle of arrows arrives as
	// quantity 20, bundleSize 20.
	BundleSize   ddbNum `json:"bundleSize"`
	IsContainer  bool   `json:"isContainer"`
	BaseItemName string `json:"baseItemName"`
}

type ddbClass struct {
	Id                 ddbNum    `json:"id"`
	Level              ddbNum    `json:"level"`
	IsStartingClass    bool      `json:"isStartingClass"`
	Definition         *ddbNamed `json:"definition"`
	SubclassDefinition *ddbNamed `json:"subclassDefinition"`
}

type ddbBackground struct {
	HasCustomBackground bool      `json:"hasCustomBackground"`
	Definition          *ddbNamed `json:"definition"`
	CustomBackground    *ddbNamed `json:"customBackground"`
}

type ddbFeat struct {
	Definition *ddbNamed `json:"definition"`
}

type ddbTraits struct {
	PersonalityTraits *string `json:"personalityTraits"`
	Ideals            *string `json:"ideals"`
	Bonds             *string `json:"bonds"`
	Flaws             *string `json:"flaws"`
	Appearance        *string `json:"appearance"`
}

type ddbNotes struct {
	Allies              *string `json:"allies"`
	PersonalPossessions *string `json:"personalPossessions"`
	OtherHoldings       *string `json:"otherHoldings"`
	Organizations       *string `json:"organizations"`
	Enemies             *string `json:"enemies"`
	Backstory           *string `json:"backstory"`
	OtherNotes          *string `json:"otherNotes"`
}

type ddbCurrency struct {
	CP ddbNum `json:"cp"`
	SP ddbNum `json:"sp"`
	GP ddbNum `json:"gp"`
	EP ddbNum `json:"ep"`
	PP ddbNum `json:"pp"`
}

type ddbDeath struct {
	FailCount    ddbNum `json:"failCount"`
	SuccessCount ddbNum `json:"successCount"`
}

type ddbSlot struct {
	Level ddbNum `json:"level"`
	Used  ddbNum `json:"used"`
}

type ddbItem struct {
	Id                ddbNum    `json:"id"`
	Definition        *ddbNamed `json:"definition"`
	Quantity          ddbNum    `json:"quantity"`
	Equipped          bool      `json:"equipped"`
	IsAttuned         bool      `json:"isAttuned"`
	ContainerEntityId ddbNum    `json:"containerEntityId"`
}

type ddbSpell struct {
	Definition     *ddbNamed `json:"definition"`
	Prepared       bool      `json:"prepared"`
	AlwaysPrepared bool      `json:"alwaysPrepared"`
}

type ddbSpellSets struct {
	Race       []ddbSpell `json:"race"`
	Class      []ddbSpell `json:"class"`
	Item       []ddbSpell `json:"item"`
	Feat       []ddbSpell `json:"feat"`
	Background []ddbSpell `json:"background"`
}

type ddbClassSpells struct {
	CharacterClassId ddbNum     `json:"characterClassId"`
	Spells           []ddbSpell `json:"spells"`
}

type ddbModifier struct {
	Type                string `json:"type"`
	SubType             string `json:"subType"`
	FriendlySubtypeName string `json:"friendlySubtypeName"`
	Value               ddbNum `json:"value"`
}

type ddbModifiers struct {
	Race       []ddbModifier `json:"race"`
	Class      []ddbModifier `json:"class"`
	Background []ddbModifier `json:"background"`
	Item       []ddbModifier `json:"item"`
	Feat       []ddbModifier `json:"feat"`
	Condition  []ddbModifier `json:"condition"`
}

// ddbAlignments is the site's alignment table, which is sent as an id.
var ddbAlignments = map[int]string{
	1: "Lawful Good", 2: "Neutral Good", 3: "Chaotic Good",
	4: "Lawful Neutral", 5: "Neutral", 6: "Chaotic Neutral",
	7: "Lawful Evil", 8: "Neutral Evil", 9: "Chaotic Evil",
}

// ddbStatOrder maps the site's stat ids onto our ability ids.
var ddbStatOrder = map[int]string{1: STR, 2: DEX, 3: CON, 4: INT, 5: WIS, 6: CHA}

// ----------------------------------------------------------------------------
// Conversion
// ----------------------------------------------------------------------------

// parseDDB pulls the character out of a character-service response. It takes
// either the whole { success, data } envelope or a bare character object, so a
// json file saved straight out of the browser works too.
//
// The warnings it returns are fields that arrived in a shape this did not
// expect. They are warnings rather than errors on purpose: encoding/json
// carries on decoding after a type mismatch, so the rest of the character is
// already in hand, and losing one field is a far better outcome than refusing
// a character over it.
func parseDDB(data []byte) (*ddbCharacter, []string, error) {
	var env ddbEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, nil, fmt.Errorf("this is not a D&D Beyond character: %s", err)
	}
	if env.Success != nil && !*env.Success {
		msg := env.Message
		if msg == "" {
			msg = "the character service refused the request"
		}
		return nil, nil, fmt.Errorf("%s", msg)
	}
	body := data
	if len(env.Data) > 0 {
		body = env.Data
	}
	warnings := []string{}
	var c ddbCharacter
	if err := json.Unmarshal(body, &c); err != nil {
		if w, ok := ddbSoftError(err); ok {
			warnings = append(warnings, w)
		} else {
			return nil, nil, fmt.Errorf("could not read the character: %s", err)
		}
	}
	// classSpells sits beside the other spell lists rather than inside them.
	var extra struct {
		ClassSpells []ddbClassSpells `json:"classSpells"`
	}
	if err := json.Unmarshal(body, &extra); err == nil || isTypeError(err) {
		c.ClassSpells = extra.ClassSpells
	}
	if strings.TrimSpace(c.Name) == "" {
		return nil, nil, fmt.Errorf("no character in this json - is it a campaign or a monster export?")
	}
	return &c, warnings, nil
}

// ddbSoftError turns a field that came in the wrong shape into something to
// report rather than something to stop for. Anything else - json that is not
// json at all - is a real error.
func ddbSoftError(err error) (string, bool) {
	var typeErr *json.UnmarshalTypeError
	if !errors.As(err, &typeErr) {
		return "", false
	}
	field := typeErr.Field
	if field == "" {
		field = "a field"
	}
	return fmt.Sprintf("D&D Beyond sent %s for %q where a %s was expected, so that "+
		"field was skipped - check it on the sheet", typeErr.Value, field, typeErr.Type), true
}

func isTypeError(err error) bool {
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &typeErr)
}

// ImportDDB converts a character-service payload into a Character against the
// given ruleset. The returned warnings name everything that did not map onto
// ruleset content, in the order it was met.
func ImportDDB(payload []byte, rs *Ruleset) (*Character, []string, error) {
	ddb, warnings, err := parseDDB(payload)
	if err != nil {
		return nil, nil, err
	}
	im := &ddbImport{rs: rs, idx: newNameIndex(rs), warnings: warnings}
	return im.convert(ddb)
}

type ddbImport struct {
	rs       *Ruleset
	idx      *nameIndex
	warnings []string
}

func (im *ddbImport) warn(format string, args ...interface{}) {
	im.warnings = append(im.warnings, fmt.Sprintf(format, args...))
}

func (im *ddbImport) convert(d *ddbCharacter) (*Character, []string, error) {
	c := &Character{
		Name:      strings.TrimSpace(d.Name),
		Player:    strings.TrimSpace(d.Username),
		Ruleset:   im.rs.Id,
		XP:        d.CurrentXp.Int(),
		Abilities: map[string]int{},
		Choices:   map[string][]string{},
	}
	im.abilities(d, c)
	im.race(d, c)
	im.classes(d, c)
	im.background(d, c)
	im.feats(d, c)
	im.proficiencies(d, c)
	im.hitPoints(d, c)
	im.money(d, c)
	im.slots(d, c)
	im.inventory(d, c)
	im.spells(d, c)
	im.details(d, c)
	return c, im.warnings, nil
}

// abilities sums the site's base scores with every bonus that is baked into a
// score on our sheet: racial increases, ability score improvements and feats.
// Item and condition modifiers are left out - the rules engine applies magic
// items itself, and a condition is not part of the character.
func (im *ddbImport) abilities(d *ddbCharacter, c *Character) {
	base := map[string]int{}
	for _, s := range d.Stats {
		if ab, ok := ddbStatOrder[s.Id.Int()]; ok && s.Value.Set() {
			base[ab] = s.Value.Int()
		}
	}
	for _, s := range d.BonusStats {
		if ab, ok := ddbStatOrder[s.Id.Int()]; ok && s.Value.Set() {
			base[ab] += s.Value.Int()
		}
	}
	for _, mods := range im.characterModifiers(d) {
		for _, m := range mods {
			ab, ok := ddbAbilityFromSubType(m.SubType)
			if !ok || !m.Value.Set() {
				continue
			}
			switch strings.ToLower(m.Type) {
			case "bonus":
				base[ab] += m.Value.Int()
			case "set":
				if m.Value.Int() > base[ab] {
					base[ab] = m.Value.Int()
				}
			}
		}
	}
	for _, s := range d.OverrideStats {
		if ab, ok := ddbStatOrder[s.Id.Int()]; ok && s.Value.Set() && s.Value.Int() > 0 {
			base[ab] = s.Value.Int()
		}
	}
	for _, ab := range AbilityOrder {
		if v, ok := base[ab]; ok && v > 0 {
			c.Abilities[ab] = v
		} else {
			c.Abilities[ab] = 10
			im.warn("no %s score came across, defaulted to 10", AbilityNames[ab])
		}
	}
}

// characterModifiers are the modifier groups that belong to the character
// permanently, as opposed to the ones an item or a condition is lending it.
func (im *ddbImport) characterModifiers(d *ddbCharacter) [][]ddbModifier {
	if d.Modifiers == nil {
		return nil
	}
	return [][]ddbModifier{d.Modifiers.Race, d.Modifiers.Class,
		d.Modifiers.Background, d.Modifiers.Feat}
}

func ddbAbilityFromSubType(sub string) (string, bool) {
	if !strings.HasSuffix(sub, "-score") {
		return "", false
	}
	switch strings.TrimSuffix(sub, "-score") {
	case "strength":
		return STR, true
	case "dexterity":
		return DEX, true
	case "constitution":
		return CON, true
	case "intelligence":
		return INT, true
	case "wisdom":
		return WIS, true
	case "charisma":
		return CHA, true
	}
	return "", false
}

func (im *ddbImport) race(d *ddbCharacter, c *Character) {
	if d.Race == nil {
		im.warn("no race on the D&D Beyond sheet")
		return
	}
	base := d.Race.BaseRaceName
	if base == "" {
		base = d.Race.FullName
	}
	race := im.idx.race(base)
	if race == nil {
		// A subrace name on its own ("Hill Dwarf") still finds the parent.
		race = im.idx.raceByAnyName(d.Race.FullName)
	}
	if race == nil {
		c.Race = Slugify(base)
		im.warn("race %q is not in the %s ruleset, kept as %q", base, im.rs.Id, c.Race)
		return
	}
	c.Race = race.Id
	full := d.Race.FullName
	if full == "" && d.Race.SubRaceShortName != "" {
		full = d.Race.SubRaceShortName + " " + base
	}
	if !d.Race.IsSubRace && d.Race.SubRaceShortName == "" && len(race.Subraces) == 0 {
		return
	}
	if sub := im.idx.subrace(race, full, d.Race.SubRaceShortName); sub != nil {
		c.Subrace = sub.Id
	} else if len(race.Subraces) > 0 && full != "" && !strings.EqualFold(full, race.Name) {
		im.warn("subrace %q is not in the %s ruleset, left blank", full, im.rs.Id)
	}
}

func (im *ddbImport) classes(d *ddbCharacter, c *Character) {
	// Starting class first: the rest of the module treats classes[0] as the
	// primary one, which is what decides hit dice and the sheet's headline.
	ordered := append([]ddbClass{}, d.Classes...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].IsStartingClass && !ordered[j].IsStartingClass
	})
	for _, cl := range ordered {
		if cl.Definition == nil {
			continue
		}
		name := cl.Definition.Name
		entry := ClassLevel{Level: cl.Level.Int()}
		if match := im.idx.class(name); match != nil {
			entry.Class = match.Id
			if cl.SubclassDefinition != nil && cl.SubclassDefinition.Name != "" {
				if sub := im.idx.subclass(match, cl.SubclassDefinition.Name); sub != nil {
					entry.Subclass = sub.Id
				} else {
					im.warn("subclass %q of %s is not in the %s ruleset, left blank",
						cl.SubclassDefinition.Name, match.Name, im.rs.Id)
				}
			}
		} else {
			entry.Class = Slugify(name)
			im.warn("class %q is not in the %s ruleset, kept as %q - its features and "+
				"spell progression will be missing", name, im.rs.Id, entry.Class)
		}
		c.Classes = append(c.Classes, entry)
	}
	if len(c.Classes) == 0 {
		im.warn("no classes on the D&D Beyond sheet")
	}
}

func (im *ddbImport) background(d *ddbCharacter, c *Character) {
	if d.Background == nil {
		return
	}
	def := d.Background.Definition
	if d.Background.HasCustomBackground && d.Background.CustomBackground != nil &&
		d.Background.CustomBackground.Name != "" {
		def = d.Background.CustomBackground
	}
	if def == nil || def.Name == "" {
		return
	}
	if bg := im.idx.background(def.Name); bg != nil {
		c.Background = bg.Id
		return
	}
	c.Background = Slugify(def.Name)
	im.warn("background %q is not in the %s ruleset, kept as %q",
		def.Name, im.rs.Id, c.Background)
}

func (im *ddbImport) feats(d *ddbCharacter, c *Character) {
	missing := []string{}
	for _, f := range d.Feats {
		if f.Definition == nil || f.Definition.Name == "" {
			continue
		}
		name := f.Definition.Name
		// D&D Beyond files an ability score improvement as a feat when a
		// homebrew source grants one. The scores themselves arrive as
		// modifiers and are already counted, so the entry is bookkeeping
		// rather than a feat and does not belong on the sheet.
		if ddbIsAbilityIncrease(name) {
			continue
		}
		if match := im.idx.feat(name); match != nil {
			c.Feats = addUnique(c.Feats, match.Id)
			continue
		}
		c.Feats = addUnique(c.Feats, Slugify(name))
		missing = append(missing, name)
	}
	if len(missing) > 0 {
		// Say what actually happens to them: the id is written into the
		// property drawer, but the rules engine can only put a feat on the
		// sheet if the ruleset carries its text.
		im.warn("%d %s in the %s ruleset, so %s recorded in DND_FEATS but will not "+
			"show in the features list until a ruleset module carries them: %s",
			len(missing), ddbPlural(len(missing), "feat is not", "feats are not"),
			im.rs.Id, ddbPlural(len(missing), "it is", "they are"),
			strings.Join(missing, ", "))
	}
}

// ddbIsAbilityIncrease spots the "<source> Ability Score Increase" entries
// D&D Beyond keeps in the feat list.
func ddbIsAbilityIncrease(name string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(name)),
		"ability score increase")
}

// proficiencies reads skills, expertise, languages and tools back out of the
// modifier lists, which is where D&D Beyond records all four regardless of
// whether they came from the race, the class, the background or a feat.
func (im *ddbImport) proficiencies(d *ddbCharacter, c *Character) {
	oddExpertise := []string{}
	for _, mods := range im.characterModifiers(d) {
		for _, m := range mods {
			sub := strings.ToLower(strings.TrimSpace(m.SubType))
			if sub == "" {
				continue
			}
			switch strings.ToLower(m.Type) {
			case "proficiency":
				if sk := im.rs.Skill(sub); sk != nil {
					c.Skills = addUnique(c.Skills, sk.Id)
				} else if name := ddbFriendly(m); name != "" && ddbIsTool(sub) {
					c.Tools = addUnique(c.Tools, name)
				}
			case "expertise":
				if sk := im.rs.Skill(sub); sk != nil {
					c.Expertise = addUnique(c.Expertise, sk.Id)
					c.Skills = addUnique(c.Skills, sk.Id)
				} else if name := ddbFriendly(m); name != "" && ddbIsTool(sub) {
					c.ToolExpertise = addUnique(c.ToolExpertise, name)
					c.Tools = addUnique(c.Tools, name)
				} else if name != "" {
					// Expertise in something that is neither a skill nor a
					// tool. There is nowhere on the sheet to double it, and
					// guessing what it meant would be worse than saying so.
					oddExpertise = addUnique(oddExpertise, name)
				}
			case "language":
				if name := ddbFriendly(m); name != "" {
					c.Languages = addUnique(c.Languages, name)
				}
			}
		}
	}
	sort.Strings(c.Skills)
	sort.Strings(c.Expertise)
	sort.Strings(c.Languages)
	sort.Strings(c.Tools)
	sort.Strings(c.ToolExpertise)
	if len(oddExpertise) > 0 {
		im.warn("expertise in %s is neither a skill nor a tool, so it has nowhere to "+
			"go on the sheet - apply it by hand", strings.Join(oddExpertise, " and "))
	}
}

func ddbFriendly(m ddbModifier) string {
	if s := strings.TrimSpace(m.FriendlySubtypeName); s != "" {
		return s
	}
	return Titleize(m.SubType)
}

// ddbIsTool keeps armor, weapon and saving throw proficiencies out of the
// tools list - only the named kits and instruments belong there.
func ddbIsTool(sub string) bool {
	for _, suffix := range []string{"-tools", "-supplies", "-kit", "-set",
		"-tool", "-instrument", "-utensils", "-implements"} {
		if strings.HasSuffix(sub, suffix) {
			return true
		}
	}
	switch sub {
	case "thieves-tools", "disguise-kit", "forgery-kit", "herbalism-kit",
		"navigators-tools", "poisoners-kit", "vehicles-land", "vehicles-water",
		"playing-card-set", "dice-set":
		return true
	}
	return false
}

func (im *ddbImport) hitPoints(d *ddbCharacter, c *Character) {
	level := 0
	for _, cl := range c.Classes {
		level += cl.Level
	}
	if level < 1 {
		level = 1
	}
	conMod := AbilityMod(c.Abilities[CON])
	max := d.BaseHitPoints.Int() + conMod*level
	max += d.BonusHitPoints.Int()
	// "hit points per level" is how the site records Tough and a hill dwarf's
	// Dwarven Toughness, both of which are already part of a character's max.
	for _, mods := range im.characterModifiers(d) {
		for _, m := range mods {
			if strings.EqualFold(m.SubType, "hit-points-per-level") && m.Value.Set() {
				max += m.Value.Int() * level
			}
		}
	}
	if d.OverrideHitPoints.Int() > 0 {
		max = d.OverrideHitPoints.Int()
	}
	if max < 1 {
		max = 1
		im.warn("hit points did not come across, set to 1 - run orgs dnd refresh to recompute")
	}
	c.HPMax = max
	c.HPCurrent = max - d.RemovedHitPoints.Int()
	if c.HPCurrent < 0 {
		c.HPCurrent = 0
	}
	c.HPTemp = d.TemporaryHitPoints.Int()
	c.Inspiration = d.Inspiration
	success, fail := 0, 0
	if d.DeathSaves != nil {
		success = d.DeathSaves.SuccessCount.Int()
		fail = d.DeathSaves.FailCount.Int()
	}
	c.DeathSaves = fmt.Sprintf("%d/%d", success, fail)
}

func (im *ddbImport) money(d *ddbCharacter, c *Character) {
	if d.Currencies == nil {
		return
	}
	c.Money = Money{CP: d.Currencies.CP.Int(), SP: d.Currencies.SP.Int(),
		EP: d.Currencies.EP.Int(), GP: d.Currencies.GP.Int(), PP: d.Currencies.PP.Int()}
}

// slots records how many spell slots are already spent. Pact magic slots are
// a separate track on D&D Beyond but the same level here, so a warlock's
// spent slots are folded into the level they sit at.
func (im *ddbImport) slots(d *ddbCharacter, c *Character) {
	used := map[int]int{}
	top := 0
	for _, s := range append(append([]ddbSlot{}, d.SpellSlots...), d.PactMagic...) {
		lvl := s.Level.Int()
		if lvl < 1 || lvl > 9 {
			continue
		}
		used[lvl] += s.Used.Int()
		if s.Used.Int() > 0 && lvl > top {
			top = lvl
		}
	}
	if top == 0 {
		return
	}
	c.SlotsUsed = make([]int, top)
	for lvl := 1; lvl <= top; lvl++ {
		c.SlotsUsed[lvl-1] = used[lvl]
	}
}

func (im *ddbImport) inventory(d *ddbCharacter, c *Character) {
	// Anything stored inside a container points at that container's entity id.
	containers := map[int]string{}
	for _, it := range d.Inventory {
		if it.Definition != nil && it.Definition.IsContainer {
			containers[it.Id.Int()] = Slugify(it.Definition.Name)
		}
	}
	unmatched := []string{}
	for _, it := range d.Inventory {
		if it.Definition == nil || it.Definition.Name == "" {
			continue
		}
		g := Gear{
			Name:      it.Definition.Name,
			Qty:       ddbBundles(it.Quantity.Int(), it.Definition.BundleSize.Int()),
			Equipped:  it.Equipped,
			Attuned:   it.IsAttuned,
			Container: containers[it.ContainerEntityId.Int()],
		}
		g.Weight = it.Definition.Weight.Float()
		if match := im.idx.item(it.Definition.Name, it.Definition.BaseItemName); match != nil {
			g.Id = match.Id
			g.Name = match.Name
			if g.Weight == 0 {
				g.Weight = match.Weight
			}
		} else {
			unmatched = append(unmatched, it.Definition.Name)
		}
		c.Equipment = append(c.Equipment, g)
	}
	if len(unmatched) > 0 {
		im.warn("%d %s not in the %s ruleset and carry no rules on the sheet: %s",
			len(unmatched), ddbPlural(len(unmatched), "item is", "items are"),
			im.rs.Id, strings.Join(unmatched, ", "))
	}
}

func (im *ddbImport) spells(d *ddbCharacter, c *Character) {
	classById := map[int]string{}
	for _, cl := range d.Classes {
		if cl.Definition != nil {
			classById[cl.Id.Int()] = cl.Definition.Name
		}
	}
	unmatched := []string{}
	add := func(s ddbSpell, source string) {
		if s.Definition == nil || s.Definition.Name == "" {
			return
		}
		ks := KnownSpell{
			Name:  s.Definition.Name,
			Level: s.Definition.Level.Int(),
			// A cantrip is always available, it is never "prepared".
			Prepared: s.Prepared || s.AlwaysPrepared || s.Definition.Level.Int() == 0,
			Source:   source,
		}
		if match := im.idx.spell(s.Definition.Name); match != nil {
			ks.Id = match.Id
			ks.Name = match.Name
			ks.Level = match.Level
		} else {
			ks.Id = Slugify(s.Definition.Name)
			unmatched = append(unmatched, s.Definition.Name)
		}
		if s.AlwaysPrepared {
			ks.Notes = "always prepared"
		}
		for _, have := range c.Spells {
			if have.Id == ks.Id {
				return
			}
		}
		c.Spells = append(c.Spells, ks)
	}
	for _, cs := range d.ClassSpells {
		source := classById[cs.CharacterClassId.Int()]
		if source == "" {
			source = "class"
		}
		for _, s := range cs.Spells {
			add(s, strings.ToLower(source))
		}
	}
	if d.Spells != nil {
		for _, s := range d.Spells.Class {
			add(s, "class")
		}
		for _, s := range d.Spells.Race {
			add(s, "race")
		}
		for _, s := range d.Spells.Feat {
			add(s, "feat")
		}
		for _, s := range d.Spells.Background {
			add(s, "background")
		}
		for _, s := range d.Spells.Item {
			add(s, "item")
		}
	}
	sort.SliceStable(c.Spells, func(i, j int) bool {
		if c.Spells[i].Level != c.Spells[j].Level {
			return c.Spells[i].Level < c.Spells[j].Level
		}
		return c.Spells[i].Name < c.Spells[j].Name
	})
	if len(unmatched) > 0 {
		im.warn("%d %s not in the %s ruleset and carry no rules text: %s",
			len(unmatched), ddbPlural(len(unmatched), "spell is", "spells are"),
			im.rs.Id, strings.Join(unmatched, ", "))
	}
}

// details carries across everything that is prose or appearance rather than
// rules: the roleplaying traits, the notes, the portrait and the alignment.
func (im *ddbImport) details(d *ddbCharacter, c *Character) {
	if d.Traits != nil {
		c.Personality = ddbStr(d.Traits.PersonalityTraits)
		c.Ideals = ddbStr(d.Traits.Ideals)
		c.Bonds = ddbStr(d.Traits.Bonds)
		c.Flaws = ddbStr(d.Traits.Flaws)
		c.Appearance = ddbStr(d.Traits.Appearance)
	}
	if d.Notes != nil {
		c.Backstory = ddbStr(d.Notes.Backstory)
		c.Allies = ddbJoin(
			labelled("Allies", ddbStr(d.Notes.Allies)),
			labelled("Organizations", ddbStr(d.Notes.Organizations)),
			labelled("Enemies", ddbStr(d.Notes.Enemies)))
		c.Treasure = ddbJoin(
			labelled("Possessions", ddbStr(d.Notes.PersonalPossessions)),
			labelled("Holdings", ddbStr(d.Notes.OtherHoldings)))
		c.Notes = ddbStr(d.Notes.OtherNotes)
	}
	if d.Age.Int() > 0 {
		c.Age = fmt.Sprintf("%d", d.Age.Int())
	}
	c.Height = strings.TrimSpace(d.Height)
	if w := d.Weight.Float(); w > 0 {
		c.Weight = fmt.Sprintf("%s lb.", trimFloat(w))
	}
	c.Eyes = strings.TrimSpace(d.Eyes)
	c.Skin = strings.TrimSpace(d.Skin)
	c.Hair = strings.TrimSpace(d.Hair)
	if d.Decorations != nil && d.Decorations.AvatarUrl != "" {
		c.Image = d.Decorations.AvatarUrl
	} else if d.AvatarUrl != "" {
		c.Image = d.AvatarUrl
	}
	if name, ok := ddbAlignments[d.AlignmentId.Int()]; ok {
		c.Alignment = im.idx.alignment(name)
	}
	if c.Player == "" {
		c.Player = "D&D Beyond"
	}
	if strings.TrimSpace(d.Faith) != "" {
		c.Notes = ddbJoin(labelled("Faith", strings.TrimSpace(d.Faith)), c.Notes)
	}
}

// ddbBundles turns D&D Beyond's count of single things into the count of the
// bundles the equipment table lists, because the ruleset carries the bundle -
// "Arrows (20)" - and not the arrow. 20 arrows is one line of arrows, not
// twenty of them, and getting this wrong multiplies both the count and the
// weight by twenty.
func ddbBundles(quantity, bundle int) int {
	if quantity < 1 {
		return 1
	}
	if bundle < 2 {
		return quantity
	}
	n := quantity / bundle
	if quantity%bundle != 0 {
		// A part bundle still takes up a line.
		n++
	}
	if n < 1 {
		n = 1
	}
	return n
}

// ddbPlural picks the wording for a count, so a warning reads as a sentence
// rather than as a log line.
func ddbPlural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func ddbStr(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func ddbJoin(parts ...string) string {
	kept := []string{}
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, strings.TrimSpace(p))
		}
	}
	return strings.Join(kept, "\n\n")
}

// ----------------------------------------------------------------------------
// Matching D&D Beyond names onto ruleset ids
// ----------------------------------------------------------------------------

// nameIndex looks ruleset content up by name. The slug of a name is tried
// first because it is exact and almost always right; failing that the fuzzy
// matcher used by the chooser picks the closest name, which is what catches
// the places the site words a name differently from the books ("Crossbow,
// Light", "Half-Elf (Variant)").
type nameIndex struct {
	rs     *Ruleset
	items  *matchTable
	spells *matchTable
	feats  *matchTable
	bgs    *matchTable
	races  *matchTable
}

type matchTable struct {
	ids   []string
	names []string
	// bySlug holds the readings of a name that are certainly that name.
	bySlug map[string]int
	// byAlias holds the readings that are usually right but not certainly, so
	// they are only consulted once every entry has failed to match outright.
	// Keeping them in a second map is what makes that ordering hold: bySlug
	// is complete before the first lookup, so an alias can never beat a name
	// that some later entry owns for real.
	byAlias map[string]int
}

func newMatchTable() *matchTable {
	return &matchTable{bySlug: map[string]int{}, byAlias: map[string]int{}}
}

func (t *matchTable) add(id, name string) {
	idx := len(t.ids)
	t.ids = append(t.ids, id)
	t.names = append(t.names, name)
	for _, key := range ddbKeys(name) {
		if _, seen := t.bySlug[key]; !seen {
			t.bySlug[key] = idx
		}
	}
	if _, seen := t.bySlug[id]; !seen {
		t.bySlug[id] = idx
	}
	for _, key := range ddbAliasKeys(name) {
		if _, seen := t.byAlias[key]; !seen {
			t.byAlias[key] = idx
		}
	}
}

// find returns the id of the best match, or "" when nothing is close enough.
func (t *matchTable) find(names ...string) string {
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		for _, key := range ddbKeys(name) {
			if idx, ok := t.bySlug[key]; ok {
				return t.ids[idx]
			}
		}
	}
	for _, name := range names {
		for _, key := range ddbAliasKeys(name) {
			if idx, ok := t.bySlug[key]; ok {
				return t.ids[idx]
			}
			if idx, ok := t.byAlias[key]; ok {
				return t.ids[idx]
			}
		}
	}
	for _, name := range names {
		if idx := ddbClosest(name, t.names); idx >= 0 {
			return t.ids[idx]
		}
	}
	return ""
}

// ddbAliasKeys are the second choice readings of a name - right often enough
// to beat carrying an entry across unmatched, not certain enough to outrank a
// name that matched outright.
//
// The one that matters is the wizard a spell is named after. The SRD drops
// those, so the books' Tasha's Hideous Laughter is published as Hideous
// Laughter, Evard's Black Tentacles as Black Tentacles and Melf's Acid Arrow
// as Acid Arrow, and a payload written against the books misses every one of
// them. Aliasing both sides also covers the reverse: the SRD kept a possessive
// of its own on Arcanist's Magic Aura, which is filed here under "magic aura"
// as well, and that is what the books' Nystul's Magic Aura comes looking for.
//
// The remainder has to be two words or more. At one word this stops being a
// rename and becomes a category: burglar's pack would answer to "pack" and
// thieves' tools to "tools", and either would swallow whichever unrelated
// entry asked for it first.
//
// It does not stretch to the handful the SRD renamed outright rather than
// trimmed - Bigby's Hand is Arcane Hand, Mordenkainen's Sword is Arcane Sword.
// Those share no words to match on, and a guess there would be a different
// spell wearing the right name, which is worse than a warning.
func ddbAliasKeys(name string) []string {
	name = strings.TrimSpace(name)
	first := strings.IndexAny(name, " 	")
	if first <= 0 {
		return nil
	}
	if !ddbPossessive(name[:first]) {
		return nil
	}
	rest := strings.TrimSpace(name[first:])
	if len(strings.Fields(rest)) < 2 {
		return nil
	}
	out := []string{}
	for _, k := range ddbKeys(rest) {
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}

// ddbPossessive reports a word in the possessive - "Tasha's", "Thieves'" - in
// either the straight or the typographic apostrophe the site sends.
func ddbPossessive(word string) bool {
	for _, suffix := range []string{"'s", "\u2019s", "s'", "s\u2019"} {
		if len(word) > len(suffix) && strings.HasSuffix(word, suffix) {
			return true
		}
	}
	return false
}

// ddbKeys are the slugs a D&D Beyond name might be filed under here. The site
// writes some names inverted ("Crossbow, Light") and qualifies others in
// brackets ("Mage Hand (Legerdemain)"), so both readings are indexed.
func ddbKeys(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	keys := []string{Slugify(name)}
	if i := strings.Index(name, "("); i > 0 {
		keys = append(keys, Slugify(name[:i]))
	}
	if parts := strings.SplitN(name, ",", 2); len(parts) == 2 {
		keys = append(keys, Slugify(strings.TrimSpace(parts[1])+" "+strings.TrimSpace(parts[0])),
			Slugify(strings.TrimSpace(parts[0])))
	}
	out := []string{}
	for _, k := range keys {
		if k != "" && !containsStr(out, k) {
			out = append(out, k)
		}
	}
	return out
}

// ddbClosest is the last resort when no slug matched. It takes a candidate
// that contains the whole of the name being looked for, which catches the
// places D&D Beyond drops a qualifier the books keep: "Costume" for the srd's
// "Clothes, costume", "Arrows" for "Arrows (20)".
//
// It deliberately will not match letters found scattered through a name, the
// way the chooser's fuzzy matcher does. That is right for a person typing at a
// list and watching what comes back, and wrong here, where nobody is watching:
// it quietly turns "String" into "Signet ring", and an item silently renamed
// into a different item is far worse than one carried across unmatched with a
// warning beside it.
func ddbClosest(name string, names []string) int {
	want := ddbWords(name)
	if len(want) < 4 {
		return -1
	}
	best, bestLen := -1, 0
	for i, candidate := range names {
		have := ddbWords(candidate)
		if have == "" || !strings.Contains(have, want) {
			continue
		}
		// A name much longer than what it contains is a different thing that
		// happens to mention this one.
		if len(have) > 2*len(want)+8 {
			continue
		}
		// The shortest name that contains it is the most specific match.
		if best < 0 || len(have) < bestLen {
			best, bestLen = i, len(have)
		}
	}
	return best
}

// ddbWords reduces a name to its lowercase words, so that punctuation and
// spacing do not decide whether two names are the same.
func ddbWords(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}

func newNameIndex(rs *Ruleset) *nameIndex {
	idx := &nameIndex{rs: rs, items: newMatchTable(), spells: newMatchTable(),
		feats: newMatchTable(), bgs: newMatchTable(), races: newMatchTable()}
	for i := range rs.Items {
		idx.items.add(rs.Items[i].Id, rs.Items[i].Name)
	}
	for i := range rs.Spells {
		idx.spells.add(rs.Spells[i].Id, rs.Spells[i].Name)
	}
	for i := range rs.Feats {
		idx.feats.add(rs.Feats[i].Id, rs.Feats[i].Name)
	}
	for i := range rs.Backgrounds {
		idx.bgs.add(rs.Backgrounds[i].Id, rs.Backgrounds[i].Name)
	}
	for i := range rs.Races {
		idx.races.add(rs.Races[i].Id, rs.Races[i].Name)
	}
	return idx
}

func (n *nameIndex) race(name string) *Race {
	if id := n.races.find(name); id != "" {
		return n.rs.Race(id)
	}
	return nil
}

// raceByAnyName finds the parent race of a subrace named on its own, so that
// "Hill Dwarf" with no base race still lands on the dwarf.
func (n *nameIndex) raceByAnyName(name string) *Race {
	if r := n.race(name); r != nil {
		return r
	}
	slug := Slugify(name)
	for i := range n.rs.Races {
		for j := range n.rs.Races[i].Subraces {
			if Slugify(n.rs.Races[i].Subraces[j].Name) == slug ||
				n.rs.Races[i].Subraces[j].Id == slug {
				return &n.rs.Races[i]
			}
		}
	}
	// "Elf (High)" and the like: fall back on the first word.
	if fields := strings.Fields(name); len(fields) > 1 {
		return n.race(fields[len(fields)-1])
	}
	return nil
}

func (n *nameIndex) subrace(race *Race, full, short string) *Race {
	if race == nil || len(race.Subraces) == 0 {
		return nil
	}
	table := newMatchTable()
	for i := range race.Subraces {
		table.add(race.Subraces[i].Id, race.Subraces[i].Name)
	}
	candidates := []string{full}
	if short != "" {
		candidates = append(candidates, short+" "+race.Name, short)
	}
	id := table.find(candidates...)
	if id == "" {
		return nil
	}
	for i := range race.Subraces {
		if race.Subraces[i].Id == id {
			return &race.Subraces[i]
		}
	}
	return nil
}

func (n *nameIndex) class(name string) *Class {
	table := newMatchTable()
	for i := range n.rs.Classes {
		table.add(n.rs.Classes[i].Id, n.rs.Classes[i].Name)
	}
	if id := table.find(name); id != "" {
		return n.rs.Class(id)
	}
	return nil
}

func (n *nameIndex) subclass(class *Class, name string) *Subclass {
	if class == nil {
		return nil
	}
	table := newMatchTable()
	for i := range class.Subclasses {
		table.add(class.Subclasses[i].Id, class.Subclasses[i].Name)
	}
	// A subclass is often named for its theme alone on one side and with its
	// full title on the other: "Evocation" against "School of Evocation".
	extra := []string{name}
	for _, prefix := range []string{"School of ", "Path of the ", "College of ",
		"Circle of the ", "Way of the ", "Oath of ", "The "} {
		extra = append(extra, prefix+name)
	}
	extra = append(extra, ddbStripSubclassPrefix(name))
	if id := table.find(extra...); id != "" {
		for i := range class.Subclasses {
			if class.Subclasses[i].Id == id {
				return &class.Subclasses[i]
			}
		}
	}
	return nil
}

func ddbStripSubclassPrefix(name string) string {
	lower := strings.ToLower(name)
	for _, prefix := range []string{"school of ", "path of the ", "path of ",
		"college of ", "circle of the ", "circle of ", "way of the ", "way of ",
		"oath of the ", "oath of ", "the "} {
		if strings.HasPrefix(lower, prefix) {
			return name[len(prefix):]
		}
	}
	return name
}

func (n *nameIndex) background(name string) *Background {
	if id := n.bgs.find(name); id != "" {
		return n.rs.Background(id)
	}
	return nil
}

func (n *nameIndex) feat(name string) *Feat {
	if id := n.feats.find(name); id != "" {
		return n.rs.Feat(id)
	}
	return nil
}

func (n *nameIndex) item(names ...string) *Item {
	if id := n.items.find(names...); id != "" {
		return n.rs.Item(id)
	}
	return nil
}

func (n *nameIndex) spell(name string) *Spell {
	if id := n.spells.find(name); id != "" {
		return n.rs.Spell(id)
	}
	return nil
}

// alignment matches the site's alignment name against the ruleset's list so a
// module that words them differently still gets its own spelling.
func (n *nameIndex) alignment(name string) string {
	for _, a := range n.rs.AllAlignments() {
		if strings.EqualFold(a, name) {
			return a
		}
	}
	return name
}
