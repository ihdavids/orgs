package dnd

/* SDOC: DnD
* Fallback Portraits

  A character with no =DND_IMAGE= of their own still gets a face. The module
  ships one drawn portrait per race, and the html character sheet falls back
  to the one matching the character's race - or their subrace, if there is a
  portrait for it - so the medallion is never empty.

  They are flat, stylised busts in the sheet's own parchment and ink, drawn
  rather than photographed on purpose: a fallback should look like a stand in
  for a portrait, not like somebody's actual character art. They live in
  =internal/common/dnd/data/portraits/*.svg= and are *generated* - regenerate
  them with =python3 tools/dndportraits/gen_portraits.py= rather than editing
  the svg by hand.

  Each one is drawn to what its race's own entry says about it rather than
  being a recoloured human: a tabaxi has a cat's ears, muzzle, whiskers and
  slit pupils, a dragonborn a scaled snout and swept horns, a warforged
  plates and a lit eye slit, a triton fins and gills. Every one has brows,
  eyes and a mouth, because a bare oval reads as somebody the artist did not
  mean to draw.

  Portraits are carried for the races in the basic rules and for the ones the
  common supplement rulesets add - aasimar, firbolg, goliath, tabaxi, triton,
  warforged and cairnborn - so a ruleset beyond the SRD still gets a face.
  A race none of them has heard of falls back to a hooded adventurer, so a
  portrait is always found.

  Nothing is written into the org file: the fallback is chosen every time the
  sheet is rendered. Setting =DND_IMAGE= at any point takes over from it, and
  clearing it hands the character back to their race's portrait.
EDOC */

import (
	"embed"
	"encoding/base64"
	"path"
	"strings"
	"sync"
)

//go:embed data/portraits/*.svg
var portraitFS embed.FS

// GenericPortrait is the portrait a race with no drawing of its own gets.
const GenericPortrait = "adventurer"

var (
	portraitOnce sync.Once
	portraits    map[string]string
)

func loadPortraits() {
	portraits = map[string]string{}
	entries, err := portraitFS.ReadDir("data/portraits")
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".svg") {
			continue
		}
		data, err := portraitFS.ReadFile(path.Join("data/portraits", name))
		if err != nil {
			continue
		}
		portraits[strings.TrimSuffix(name, ".svg")] =
			"data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(data)
	}
}

// PortraitIds is every fallback portrait the module carries, for anything
// that wants to offer them as a choice.
func PortraitIds() []string {
	portraitOnce.Do(loadPortraits)
	out := []string{}
	for id := range portraits {
		out = append(out, id)
	}
	return out
}

// DefaultPortrait is the drawn portrait for a race, as a data uri ready to
// drop into an <img src>. It always finds something: a race with no portrait
// of its own, and a subrace whose parent has none either, both come back with
// the hooded adventurer.
//
// A subrace is tried first and then narrowed onto its parent, so a high elf
// gets the elf portrait without every subrace needing a drawing: "high-elf"
// falls back to "elf" because the id ends with it. Drow is the one subrace
// whose id says nothing about its parent, so it is spelled out.
func DefaultPortrait(race, subrace string) string {
	portraitOnce.Do(loadPortraits)
	for _, key := range []string{subrace, subraceParent(subrace), race} {
		if src := findPortrait(key); src != "" {
			return src
		}
	}
	return portraits[GenericPortrait]
}

// subraceParent is the race a subrace id names, when it names one at all.
func subraceParent(subrace string) string {
	id := Slugify(subrace)
	if id == "" {
		return ""
	}
	// The one subrace in the basic rules whose id does not carry its parent.
	if id == "drow" {
		return "elf"
	}
	if i := strings.LastIndex(id, "-"); i > 0 {
		return id[i+1:]
	}
	return ""
}

func findPortrait(key string) string {
	id := Slugify(key)
	if id == "" {
		return ""
	}
	return portraits[id]
}

// PortraitSrc is the portrait to draw for a character: their own if they have
// one, and their race's otherwise. The error is only ever about the
// character's own portrait - a broken DND_IMAGE is worth telling them about -
// and the fallback is returned anyway so the sheet still has a face.
func PortraitSrc(c *Character, baseDir string) (string, error) {
	if c == nil {
		return "", nil
	}
	src, err := ImageSrc(c.Image, baseDir)
	if src != "" {
		return src, err
	}
	return DefaultPortrait(c.Race, c.Subrace), err
}
