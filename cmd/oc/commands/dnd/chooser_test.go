// Tests for the list chooser: fuzzy filtering, the keys that edit the filter,
// and the ticking of a multiple choice list across filter changes.
package dnd

import (
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common/dnd"
)

var spells = []dnd.Option{
	{Id: "fire-bolt", Name: "Fire Bolt", Summary: "A mote of fire, 1d10 damage."},
	{Id: "fireball", Name: "Fireball", Summary: "A bright streak, 8d6 fire in a 20 ft sphere."},
	{Id: "eldritch-blast", Name: "Eldritch Blast", Summary: "A beam of crackling energy."},
	{Id: "mage-hand", Name: "Mage Hand", Summary: "A spectral hand."},
	{Id: "magic-missile", Name: "Magic Missile", Summary: "Three darts of magical force."},
	{Id: "cure-wounds", Name: "Cure Wounds", Summary: "A creature you touch regains hit points."},
	{Id: "burning-hands", Name: "Burning Hands", Summary: "A thin sheet of flames, fire damage."},
	{Id: "shield", Name: "Shield", Summary: "An invisible barrier of magical force."},
	{Id: "wall-of-fire", Name: "Wall of Fire", Summary: "A wall of fire springs into existence."},
}

func newTestChooser(multi bool) (*chooser, *paneCtx) {
	labels := []string{}
	ctx := &paneCtx{byLabel: map[string]dnd.Option{}}
	for _, o := range spells {
		l := label(o)
		ctx.byLabel[l] = o
		labels = append(labels, l)
	}
	labels = append(labels, entryRandom, entryBack, entryQuit)
	return newChooser("Pick a spell", labels, "help text", 10, "", nil, multi, ctx), ctx
}

func (c *chooser) visible() []string {
	out := []string{}
	for _, m := range c.matches {
		out = append(out, paneName(c.options[m]))
	}
	return out
}

func (c *chooser) type_(s string) {
	for _, r := range s {
		c.refilter(c.filter + string(r))
	}
}

func TestFuzzyNarrowing(t *testing.T) {
	c, _ := newTestChooser(false)
	if len(c.visible()) != len(spells)+3 {
		t.Fatalf("empty filter should show everything, got %v", c.visible())
	}
	c.type_("fir")
	t.Logf("fir -> %v", c.visible())
	if got := c.visible()[0]; got != "Fire Bolt" {
		t.Errorf("fir should lead with Fire Bolt, got %v", c.visible())
	}
	c.type_("eb")
	t.Logf("fireb -> %v", c.visible())
	if c.visible()[0] != "Fireball" || len(c.visible()) > 2 {
		t.Errorf("fireb should lead with Fireball, got %v", c.visible())
	}
	// backspacing widens it again, one character at a time
	c.refilter(dropRune(c.filter))
	t.Logf("fire -> %v", c.visible())
	if len(c.visible()) < 3 {
		t.Errorf("backspace should widen the list, got %v", c.visible())
	}
	c.refilter(dropRune(dropRune(dropRune(c.filter))))
	if c.filter != "f" {
		t.Fatalf("filter should be f, got %q", c.filter)
	}
	t.Logf("f -> %v", c.visible())
	c.refilter("")
	if len(c.visible()) != len(spells)+3 {
		t.Errorf("clearing the filter should restore everything, got %d", len(c.visible()))
	}
}

func TestFuzzySubsequenceAndNamePriority(t *testing.T) {
	c, _ := newTestChooser(false)
	c.type_("eldbl")
	t.Logf("eldbl -> %v", c.visible())
	if len(c.visible()) != 1 || c.visible()[0] != "Eldritch Blast" {
		t.Fatalf("eldbl should find Eldritch Blast, got %v", c.visible())
	}
	c.refilter("")
	// "magic" appears in the summary of shield and magic missile, but only
	// magic missile is called it.
	c.type_("magic")
	t.Logf("magic -> %v", c.visible())
	if c.visible()[0] != "Magic Missile" {
		t.Errorf("magic should lead with Magic Missile, got %v", c.visible())
	}
	c.refilter("")
	c.type_("wall fire")
	t.Logf("wall fire -> %v", c.visible())
	if c.visible()[0] != "Wall of Fire" {
		t.Errorf("multi word filter failed, got %v", c.visible())
	}
	c.refilter("")
	c.type_("zzz")
	if len(c.visible()) != 0 {
		t.Errorf("nonsense should match nothing, got %v", c.visible())
	}
	if c.current() != -1 {
		t.Errorf("no match means no current option")
	}
	c.refilter(dropRune(c.filter))
	if len(c.visible()) == 0 {
		t.Logf("zz still matches nothing, ok")
	}
}

func TestCursorStaysOnTheSameOptionWhileFiltering(t *testing.T) {
	c, _ := newTestChooser(false)
	c.move(4) // Magic Missile
	want := paneName(c.options[c.current()])
	if want != "Magic Missile" {
		t.Fatalf("setup: cursor on %q", want)
	}
	c.type_("ma")
	if got := paneName(c.options[c.current()]); got != want {
		t.Errorf("cursor jumped from %q to %q while filtering", want, got)
	}
	c.refilter("")
	if got := paneName(c.options[c.current()]); got != want {
		t.Errorf("cursor jumped from %q to %q when the filter cleared", want, got)
	}
}

func TestMultiSelectKeepsTicksAcrossFilters(t *testing.T) {
	c, _ := newTestChooser(true)
	c.type_("fireball")
	c.toggle()
	c.refilter("")
	c.type_("shield")
	c.toggle()
	c.refilter("")
	got := c.answer().([]string)
	names := []string{}
	for _, l := range got {
		names = append(names, paneName(l))
	}
	if strings.Join(names, ",") != "Fireball,Shield" {
		t.Errorf("ticks did not survive filtering: %v", names)
	}
}

func TestDropWord(t *testing.T) {
	if got := dropWord("wall of fire"); got != "wall of " {
		t.Errorf("dropWord: %q", got)
	}
	if got := dropWord("fire"); got != "" {
		t.Errorf("dropWord: %q", got)
	}
}

func TestPaging(t *testing.T) {
	matches := []int{}
	for i := 0; i < 30; i++ {
		matches = append(matches, i)
	}
	page, idx := pageOf(matches, 0, 7)
	if len(page) != 7 || page[idx] != 0 {
		t.Errorf("first page wrong: %v %d", page, idx)
	}
	page, idx = pageOf(matches, 29, 7)
	if len(page) != 7 || page[idx] != 29 {
		t.Errorf("last page wrong: %v %d", page, idx)
	}
	page, idx = pageOf(matches, 15, 7)
	if len(page) != 7 || page[idx] != 15 {
		t.Errorf("middle page wrong: %v %d", page, idx)
	}
	page, idx = pageOf([]int{1, 2}, 1, 7)
	if len(page) != 2 || page[idx] != 2 {
		t.Errorf("short page wrong: %v %d", page, idx)
	}
}

// press feeds a literal key sequence through the key handler, the way the
// terminal would.
func (c *chooser) press(keys ...rune) (done bool) {
	for _, k := range keys {
		d, _ := c.onKey(k)
		done = done || d
	}
	return done
}

func typeKeys(c *chooser, s string) {
	for _, r := range s {
		c.press(r)
	}
}

func TestKeysTypeAndBackspace(t *testing.T) {
	const bs = '\x7f' // what a mac backspace actually sends
	c, _ := newTestChooser(false)
	typeKeys(c, "magi")
	if c.filter != "magi" {
		t.Fatalf("typing did not build the filter: %q", c.filter)
	}
	t.Logf("magi -> %v", c.visible())
	if paneName(c.options[c.current()]) != "Magic Missile" {
		t.Errorf("magi should land on Magic Missile, got %v", c.visible())
	}
	// backspace out to "ma", then to nothing, widening as we go
	c.press(bs)
	c.press(bs)
	if c.filter != "ma" {
		t.Fatalf("backspace did not shorten the filter: %q", c.filter)
	}
	t.Logf("ma -> %v", c.visible())
	if len(c.visible()) <= 2 {
		t.Errorf("backspacing should have widened the list: %v", c.visible())
	}
	c.press('\b') // and the other backspace encoding
	if c.filter != "m" {
		t.Errorf("\\b should also delete: %q", c.filter)
	}
	// a different word entirely, found by widening rather than starting over
	typeKeys(c, "age hand")
	t.Logf("%q -> %v", c.filter, c.visible())
	if paneName(c.options[c.current()]) != "Mage Hand" {
		t.Errorf("expected Mage Hand, got %v", c.visible())
	}
	// ctrl+w drops the last word, escape clears the lot
	c.press('\x17')
	if c.filter != "mage " {
		t.Errorf("ctrl+w: %q", c.filter)
	}
	c.press('\x1b')
	if c.filter != "" || len(c.visible()) != len(spells)+3 {
		t.Errorf("escape should clear the filter: %q %v", c.filter, c.visible())
	}
}

func TestKeysEnterAndInterrupt(t *testing.T) {
	c, _ := newTestChooser(false)
	typeKeys(c, "zzz")
	if c.press('\r') {
		t.Errorf("enter with nothing matched should hold the prompt")
	}
	if c.answer().(string) != "" {
		t.Errorf("no match must not answer with an option")
	}
	c.press('\x1b')
	typeKeys(c, "cure")
	if !c.press('\r') {
		t.Errorf("enter should accept a match")
	}
	if paneName(c.answer().(string)) != "Cure Wounds" {
		t.Errorf("wrong answer: %q", c.answer())
	}

	c, _ = newTestChooser(false)
	done, err := c.onKey('\x03')
	if !done || err == nil {
		t.Errorf("ctrl+c should abort: %v %v", done, err)
	}
}

func TestKeysMultiSelectSpaceTicksAndDoesNotFilter(t *testing.T) {
	c, _ := newTestChooser(true)
	typeKeys(c, "cure")
	c.press(' ')
	if c.filter != "cure" {
		t.Errorf("space must not land in the filter of a multi select: %q", c.filter)
	}
	c.press('\x1b')
	typeKeys(c, "shield")
	c.press(' ')
	c.press('\x1b')
	got := []string{}
	for _, l := range c.answer().([]string) {
		got = append(got, paneName(l))
	}
	if strings.Join(got, ",") != "Cure Wounds,Shield" {
		t.Errorf("ticking through the filter failed: %v", got)
	}
}
