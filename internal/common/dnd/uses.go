//lint:file-ignore ST1006 allow the use of self
package dnd

// ----------------------------------------------------------------------------
// Uses per rest
//
// A great many features are worded "you can use this feature twice, and you
// regain expended uses when you finish a short rest". The number is in the
// prose and nowhere else - the srd yaml is generated from the rules text, so
// there is no field to read it out of - which is why this file reads it back
// out of the sentence it was written in.
//
// The result is deliberately conservative. A feature whose limit lives in a
// class table rather than in its text - a barbarian's rages, a monk's ki, a
// sorcerer's sorcery points - is left with no limit at all rather than given
// an invented one, and shows on the sheet as the prose it always was. A
// homebrew module that wants a limit the parser cannot see writes it down:
//
//	features:
//	  - name: "Starfall"
//	    uses: "1 + cha"
//	    recharge: "long"
//
// which is always believed over anything found in the text.
// ----------------------------------------------------------------------------

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The two rests a feature can recharge on. A short rest recharge is also
// given back by a long one.
const (
	RechargeShort = "short"
	RechargeLong  = "long"
)

// reUsesVariant is the parenthetical the srd hangs on a feature that gets
// better with level: "Bardic Inspiration (d10)", "Channel Divinity (2/rest)",
// "Action Surge (two uses)". Those are all the same resource under a bigger
// number, so they share one pool. A parenthetical that names something else -
// "Mystic Arcanum (7th level)" is a different spell, not a bigger version of
// the same one - is left alone and keeps its own pool.
var reUsesVariant = regexp.MustCompile(
	`\s*\((?:d\d+|\d+/rest|(?:one|two|three|four|five|six|\d+) uses?)\)$`)

// UsesKey is how a feature's spent uses are keyed on the character. Two
// features with the same name are the same resource, which is what a
// multiclass character with two "Channel Divinity" entries expects.
func UsesKey(t Trait) string {
	return Slugify(reUsesVariant.ReplaceAllString(strings.TrimSpace(t.Name), ""))
}

// numberWords are the counts the rules write out in words.
var numberWords = map[string]int{
	"once": 1, "one": 1, "twice": 2, "two": 2, "thrice": 3, "three": 3,
	"four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
}

var (
	// "a number of times equal to 1 + your Charisma modifier"
	reUsesAbility = regexp.MustCompile(
		`number of times equal to (?:(\d+) \+ )?your (strength|dexterity|constitution|intelligence|wisdom|charisma) modifier`)
	// "a number of times equal to your proficiency bonus"
	reUsesProf = regexp.MustCompile(
		`number of times equal to (?:(\d+) \+ )?your proficiency bonus`)
	// "you can use this feature twice", "you can use it twice before a rest"
	reUsesCount = regexp.MustCompile(
		`you can (?:use|do so) (?:this feature |this trait |it |them )?(once|twice|thrice|three times|four times|five times|six times|seven times|eight times|nine times|ten times|\d+ times)`)
	// "twice between long rests", "three times per day"
	reUsesPer = regexp.MustCompile(
		`(once|twice|thrice|three times|four times|five times|six times|seven times|eight times|nine times|ten times|\d+ times) (?:between|per) `)
	// The single use wordings, all of which mean one.
	reUsesOnce = regexp.MustCompile(
		`once you use this (?:feature|trait)|` +
			`can't (?:use (?:it|this feature|this trait|this ability)|do so) again until|` +
			`before you can (?:use (?:it|this feature|this trait)|do so) again|` +
			`rest to use [a-z' ]+ again|` +
			`regain the use of (?:it|this feature|this trait)|` +
			`once per (?:day|short rest|long rest)`)
	// "starting at 17th level", "when you reach 13th level"
	reUsesLevel = regexp.MustCompile(
		`(?:starting at|beginning at|when you reach) (\d+)(?:st|nd|rd|th) level`)
	// "(a minimum of once)", "(minimum of 1)"
	reUsesFloor = regexp.MustCompile(`minimum of (once|one|\d+)`)

	reRestShortOrLong = regexp.MustCompile(`short (?:rest )?or (?:a )?long rests?`)
	reRestLong        = regexp.MustCompile(`long rests?`)
	reRestShort       = regexp.MustCompile(`short rests?`)

	// A clause only says how a feature recharges if it is talking about
	// getting it back. "You can choose one damage type when you finish a
	// short or long rest" is not a limit, it is when a choice is made.
	reRechargeCue = regexp.MustCompile(
		`regain|replenish|expended uses|use it again|use this feature again|` +
			`use this trait again|before you can use|per day`)

	// Clauses are split on sentence ends and on "and", because the rules put
	// two level-gated limits in one sentence: "twice between long rests
	// starting at 13th level and three times between long rests starting at
	// 17th level".
	reClauseSplit = regexp.MustCompile(`(?:\.\s+|\.$|;\s+|\s+and\s+)`)

	reSpace = regexp.MustCompile(`\s+`)
)

// countWord reads "twice" or "3 times" as a number.
func countWord(word string) int {
	word = strings.TrimSpace(strings.TrimSuffix(word, " times"))
	if n, ok := numberWords[word]; ok {
		return n
	}
	if n, err := strconv.Atoi(word); err == nil {
		return n
	}
	return 0
}

// restKind names the rest a clause talks about. A "short or long rest"
// recharges on the short one, which is the earlier of the two.
func restKind(clause string) string {
	switch {
	case reRestShortOrLong.MatchString(clause):
		return RechargeShort
	case strings.Contains(clause, "per day"):
		return RechargeLong
	case reRestLong.MatchString(clause):
		return RechargeLong
	case reRestShort.MatchString(clause):
		return RechargeShort
	}
	return ""
}

// DetectUses reads a use limit out of a trait.
//
// level is the level the limit scales against - the class level for a class
// feature, the character's total level for a racial trait - because several
// features are written "twice starting at 13th level and three times starting
// at 17th". mods and proficiency are what the formulas are worth for this
// character.
//
// A trait that declares Uses or Recharge itself is taken at its word. A trait
// that neither declares them nor says anything the parser recognises comes
// back with a max of zero, which means "no limit worth a row of slots".
func DetectUses(t Trait, level int, mods map[string]int, proficiency int) (int, string, string) {
	max, recharge, note := 0, strings.ToLower(strings.TrimSpace(t.Recharge)), ""
	if strings.TrimSpace(t.Uses) != "" {
		max = EvalBonus(t.Uses, mods, proficiency)
		if max < 1 {
			max = 1
		}
	}
	// Whatever the module did not say for itself is read out of the prose,
	// so declaring only a recharge - or only a count - is enough.
	if max <= 0 || recharge == "" {
		readMax, readRecharge, readNote := detectUsesFromText(t.Name+". "+t.Text, level, mods, proficiency)
		if max <= 0 {
			max, note = readMax, readNote
		}
		if recharge == "" {
			recharge = readRecharge
		}
	}
	if recharge != RechargeShort && recharge != RechargeLong {
		recharge = RechargeLong
	}
	if max <= 0 {
		return 0, "", ""
	}
	if note == "" {
		note = usesNote(max, recharge)
	}
	return max, recharge, note
}

func usesNote(max int, recharge string) string {
	rest := "long rest"
	if recharge == RechargeShort {
		rest = "short or long rest"
	}
	if max == 1 {
		return fmt.Sprintf("once per %s", rest)
	}
	return fmt.Sprintf("%d per %s", max, rest)
}

// detectUsesFromText is the reader itself: it walks the clauses of the rules
// text, collects every limit it can see along with the level that limit
// starts at, and answers with the largest one this character has reached.
func detectUsesFromText(text string, level int, mods map[string]int, proficiency int) (int, string, string) {
	lower := reSpace.ReplaceAllString(strings.ToLower(text), " ")
	clauses := reClauseSplit.Split(lower, -1)

	// Which rest gives the uses back. Without a clause that actually talks
	// about getting them back there is no limit here to track.
	recharge := ""
	for _, clause := range clauses {
		if !reRechargeCue.MatchString(clause) {
			continue
		}
		if kind := restKind(clause); kind != "" {
			recharge = kind
			break
		}
	}
	if recharge == "" {
		return 0, "", ""
	}

	best, bestNote := 0, ""
	for _, clause := range clauses {
		// A limit that starts at a level this character has not reached is
		// not theirs yet.
		gate := 0
		if m := reUsesLevel.FindStringSubmatch(clause); m != nil {
			gate, _ = strconv.Atoi(m[1])
		}
		if gate > level {
			continue
		}
		count, note := clauseUses(clause, mods, proficiency)
		if count > best {
			best, bestNote = count, note
		}
	}
	if best <= 0 {
		return 0, "", ""
	}
	if bestNote != "" {
		bestNote += ", back on a " + restWords(recharge)
	}
	return best, recharge, bestNote
}

func restWords(recharge string) string {
	if recharge == RechargeShort {
		return "short or long rest"
	}
	return "long rest"
}

// clauseUses is what one clause says the limit is, and how it said it.
func clauseUses(clause string, mods map[string]int, proficiency int) (int, string) {
	floor := 0
	if m := reUsesFloor.FindStringSubmatch(clause); m != nil {
		floor = countWord(m[1])
	}
	scale := func(base int, extra, name string) (int, string) {
		n := base
		if extra != "" {
			add, _ := strconv.Atoi(extra)
			n += add
		}
		if n < floor {
			n = floor
		}
		if n < 1 {
			n = 1
		}
		if extra != "" {
			return n, extra + " + your " + name
		}
		return n, "your " + name
	}
	if m := reUsesAbility.FindStringSubmatch(clause); m != nil {
		return scale(mods[abilityIdFromName(m[2])], m[1], AbilityNames[abilityIdFromName(m[2])]+" modifier")
	}
	if m := reUsesProf.FindStringSubmatch(clause); m != nil {
		return scale(proficiency, m[1], "proficiency bonus")
	}
	if m := reUsesCount.FindStringSubmatch(clause); m != nil {
		return countWord(m[1]), ""
	}
	if m := reUsesPer.FindStringSubmatch(clause); m != nil {
		return countWord(m[1]), ""
	}
	if reUsesOnce.MatchString(clause) {
		return 1, ""
	}
	return 0, ""
}

// abilityIdFromName turns "charisma" back into "cha".
func abilityIdFromName(name string) string {
	for id, full := range AbilityNames {
		if strings.EqualFold(full, name) {
			return id
		}
	}
	return name
}

// AnnotateUses fills in the use limit on every trait in a list, and reads how
// many of them the character has spent. level is what the limits scale
// against; see DetectUses.
func AnnotateUses(c *Character, list []Trait, level int, mods map[string]int, proficiency int) {
	for i := range list {
		max, recharge, note := DetectUses(list[i], level, mods, proficiency)
		list[i].UsesMax = max
		list[i].Recharge = recharge
		list[i].UsesNote = note
		list[i].UsesSpent = 0
		list[i].UsesId = ""
		list[i].UsesPips = nil
		if max <= 0 {
			continue
		}
		list[i].UsesId = UsesKey(list[i])
		for p := 1; p <= max; p++ {
			list[i].UsesPips = append(list[i].UsesPips, p)
		}
		if c == nil || c.UsesSpent == nil {
			continue
		}
		if spent, ok := c.UsesSpent[list[i].UsesId]; ok {
			if spent > max {
				spent = max
			}
			if spent < 0 {
				spent = 0
			}
			list[i].UsesSpent = spent
		}
	}
}

// SpendUse marks one use of a feature as gone, and UnspendUse gives it back.
// Both are told what the maximum is so that the count on the file can never
// drift past what the character actually has.
func SpendUse(c *Character, key string, max int) bool {
	if c == nil || key == "" || max <= 0 {
		return false
	}
	if c.UsesSpent == nil {
		c.UsesSpent = map[string]int{}
	}
	if c.UsesSpent[key] >= max {
		return false
	}
	c.UsesSpent[key]++
	return true
}

func UnspendUse(c *Character, key string) bool {
	if c == nil || key == "" || c.UsesSpent == nil || c.UsesSpent[key] <= 0 {
		return false
	}
	c.UsesSpent[key]--
	if c.UsesSpent[key] == 0 {
		delete(c.UsesSpent, key)
	}
	return true
}

// UsesRequest is one change to a feature's spent uses.
type UsesRequest struct {
	Filename string `json:"filename"`
	Id       string `json:"id"`
	// Action is "spend" or "recover".
	Action string `json:"action"`
	// Feature is the feature's name, or the slug of it.
	Feature string `json:"feature"`
}

// FindLimitedTrait looks a limited feature or trait up on a computed sheet by
// name or by slug. Only a feature with a use limit can be found: spending a
// use of something with no limit is not a thing the sheet can do.
func FindLimitedTrait(s *Sheet, name string) (Trait, bool) {
	want := Slugify(strings.TrimSpace(name))
	if want == "" {
		return Trait{}, false
	}
	for _, t := range limitedTraits(s) {
		if UsesKey(t) == want || Slugify(t.Name) == want {
			return t, true
		}
	}
	return Trait{}, false
}

// ApplyUseChange spends one use of a feature or hands one back, and says what
// it did. The character is left as it was if the change is not allowed.
func ApplyUseChange(c *Character, rs *Ruleset, action, feature string) (string, error) {
	s := Compute(c, rs)
	t, ok := FindLimitedTrait(s, feature)
	if !ok {
		return "", fmt.Errorf("%q is not a feature with uses to spend", feature)
	}
	key := UsesKey(t)
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "spend", "use":
		if !SpendUse(c, key, t.UsesMax) {
			return "", fmt.Errorf("%s has no uses left; take a %s", t.Name,
				restWords(t.Recharge))
		}
		return fmt.Sprintf("%s used, %d of %d left", t.Name,
			t.UsesMax-c.UsesSpent[key], t.UsesMax), nil
	case "recover", "unspend", "restore":
		if !UnspendUse(c, key) {
			return "", fmt.Errorf("%s has nothing spent to give back", t.Name)
		}
		return fmt.Sprintf("%s recovered, %d of %d left", t.Name,
			t.UsesMax-c.UsesSpent[key], t.UsesMax), nil
	}
	return "", fmt.Errorf("a use is either %q or %q, not %q", "spend", "recover", action)
}
