package dnd

// The SRD publishes seventeen Player's Handbook spells under a different name
// from the one everybody uses at the table. The wizards those spells are named
// after - Bigby, Tasha, Mordenkainen and the rest - are product identity, so
// the SRD strips the name off the front: Tasha's Hideous Laughter is published
// as Hideous Laughter, and a few are renamed outright rather than trimmed,
// Bigby's Hand becoming Arcane Hand and Mordenkainen's Sword Arcane Sword.
//
// That matters because nothing else calls them by the SRD's names. A D&D
// Beyond character arrives with the book names on it, and so does a sheet
// somebody typed out by hand, and every one of them used to come through
// unmatched: no casting time, no range, nothing to roll. So each SRD spell
// carries the book's name for it as an alias.
//
// This lives in Go rather than in a yaml module because it is a fact about
// SRD 5.1 rather than anybody's campaign content, and because the SRD data
// itself is generated - there is nowhere in it to write this by hand. A
// ruleset that has never heard of these ids is simply not affected: the
// aliases are only indexed for spells that are actually there.
var srdRenamedSpells = map[string][]string{
	"acid-arrow":           {"Melf's Acid Arrow"},
	"arcane-hand":          {"Bigby's Hand"},
	"arcane-sword":         {"Mordenkainen's Sword"},
	"arcanists-magic-aura": {"Nystul's Magic Aura"},
	"black-tentacles":      {"Evard's Black Tentacles"},
	"faithful-hound":       {"Mordenkainen's Faithful Hound"},
	"floating-disk":        {"Tenser's Floating Disk"},
	"freezing-sphere":      {"Otiluke's Freezing Sphere"},
	"hideous-laughter":     {"Tasha's Hideous Laughter"},
	"instant-summons":      {"Drawmij's Instant Summons"},
	"irresistible-dance":   {"Otto's Irresistible Dance"},
	"magnificent-mansion":  {"Mordenkainen's Magnificent Mansion"},
	"private-sanctum":      {"Mordenkainen's Private Sanctum"},
	"resilient-sphere":     {"Otiluke's Resilient Sphere"},
	"secret-chest":         {"Leomund's Secret Chest"},
	"telepathic-bond":      {"Rary's Telepathic Bond"},
	"tiny-hut":             {"Leomund's Tiny Hut"},
}

// spellAliases is every other name a spell answers to: the ones its own entry
// declares, plus the book name when the SRD renamed it.
func spellAliases(s *Spell) []string {
	out := append([]string{}, s.Aliases...)
	return append(out, srdRenamedSpells[s.Id]...)
}
