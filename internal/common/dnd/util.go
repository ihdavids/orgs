package dnd

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
)

// Slugify turns a display name into a stable id: "Path of the Berserker" -> "path-of-the-berserker"
func Slugify(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == ' ', r == '-', r == '_', r == '/':
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// Titleize makes a display name out of an id: "high-elf" -> "High Elf"
func Titleize(id string) string {
	parts := strings.FieldsFunc(id, func(r rune) bool { return r == '-' || r == '_' || r == ' ' })
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// Ordinal renders 1 -> 1st, 2 -> 2nd ...
func Ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
		suffix = "th"
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

// Signed renders a modifier the way a character sheet does: +3 / -1
func Signed(n int) string {
	if n >= 0 {
		return fmt.Sprintf("+%d", n)
	}
	return fmt.Sprintf("%d", n)
}

// AbilityMod is the classic (score - 10) / 2 rounded down.
func AbilityMod(score int) int {
	d := score - 10
	if d < 0 {
		// go division truncates toward zero, we need floor
		return -((-d + 1) / 2)
	}
	return d / 2
}

// ProficiencyBonus for a total character level.
func ProficiencyBonus(level int) int {
	if level < 1 {
		level = 1
	}
	return 2 + (level-1)/4
}

// EvalBonus works out what a bonus formula is worth for a given character.
//
// A formula is a list of terms joined with "+". A term is a whole number, an
// ability id whose modifier is added, or the proficiency bonus written as
// "prof", "prof/2" (rounded down) or "prof/2up" (rounded up). Whitespace and
// case do not matter, and a term that means nothing is skipped rather than
// failing the whole sheet - a typo in a homebrew module should cost you one
// bonus, not the character.
//
//	EvalBonus("5", mods, 3)          // 5,  Alert
//	EvalBonus("prof/2", mods, 5)     // 2,  Jack of All Trades
//	EvalBonus("prof/2up", mods, 5)   // 3,  Remarkable Athlete
//	EvalBonus("cha", mods, 3)        // the charisma modifier
func EvalBonus(formula string, mods map[string]int, proficiency int) int {
	total := 0
	for _, part := range strings.Split(formula, "+") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		switch part {
		case "prof", "proficiency":
			total += proficiency
			continue
		case "prof/2", "proficiency/2":
			total += proficiency / 2
			continue
		case "prof/2up", "proficiency/2up":
			total += (proficiency + 1) / 2
			continue
		}
		if m, ok := mods[part]; ok {
			total += m
			continue
		}
		if n, err := strconv.Atoi(part); err == nil {
			total += n
		}
	}
	return total
}

func containsStr(list []string, v string) bool {
	for _, s := range list {
		if strings.EqualFold(s, v) {
			return true
		}
	}
	return false
}

func sprintf(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

func containsInt(list []int, v int) bool {
	for _, n := range list {
		if n == v {
			return true
		}
	}
	return false
}

func addUnique(list []string, vals ...string) []string {
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" || containsStr(list, v) {
			continue
		}
		list = append(list, v)
	}
	return list
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := []string{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func joinList(l []string) string { return strings.Join(l, ", ") }

func atoi(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Roll rolls dice, "4d6k3" style is not supported, use RollAbility for that.
func Roll(count, sides int) int {
	total := 0
	for i := 0; i < count; i++ {
		total += rand.Intn(sides) + 1
	}
	return total
}

// RollAbility rolls 4d6 and drops the lowest die, the classic ability roll.
func RollAbility() int {
	dice := []int{}
	for i := 0; i < 4; i++ {
		dice = append(dice, rand.Intn(6)+1)
	}
	sort.Ints(dice)
	return dice[1] + dice[2] + dice[3]
}

// AverageRoll is the average result of an N sided die rounded up, used for
// the "take the average" hit point rule (d8 -> 5).
func AverageRoll(sides int) int { return sides/2 + 1 }

// randIndex returns a random index into a list of size n.
func randIndex(n int) int {
	if n <= 0 {
		return 0
	}
	return rand.Intn(n)
}

// randPerm returns a shuffled sequence of the numbers 0..n-1.
func randPerm(n int) []int {
	if n <= 0 {
		return []int{}
	}
	return rand.Perm(n)
}

// PickRandom returns a random element of a list ("" when empty).
func PickRandom(list []string) string {
	if len(list) == 0 {
		return ""
	}
	return list[rand.Intn(len(list))]
}

// XPThresholds is the standard experience point table, index is level.
var XPThresholds = []int{0, 0, 300, 900, 2700, 6500, 14000, 23000, 34000, 48000,
	64000, 85000, 100000, 120000, 140000, 165000, 195000, 225000, 265000, 305000, 355000}

// LevelForXP returns the character level for an xp total.
func LevelForXP(xp int) int {
	lvl := 1
	for l := 1; l < len(XPThresholds); l++ {
		if xp >= XPThresholds[l] {
			lvl = l
		}
	}
	return lvl
}

// XPForLevel returns the xp needed to reach a level.
func XPForLevel(level int) int {
	if level < 1 {
		level = 1
	}
	if level >= len(XPThresholds) {
		return XPThresholds[len(XPThresholds)-1]
	}
	return XPThresholds[level]
}

// StandardArray is the default point spread offered during creation.
var StandardArray = []int{15, 14, 13, 12, 10, 8}

// PointBuyCost is the cost of each score in the point buy system.
var PointBuyCost = map[int]int{8: 0, 9: 1, 10: 2, 11: 3, 12: 4, 13: 5, 14: 7, 15: 9}

// PointBuyBudget is the standard number of points to spend.
const PointBuyBudget = 27
