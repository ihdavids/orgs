package dnd

/* SDOC: DnD
* Inventory, Containers and Encumbrance

  The equipment table on a character sheet is a flat list of lines, and the
  inventory is that same list read as a bag: identical things stacked into one
  line with a count, split up by the container each is stored in, weighed, and
  measured against what the character can carry.

  A line says where it lives in its =Container= column, which holds the slug
  of a container item the character actually owns:

  #+BEGIN_SRC org
  ,** Equipment
  | Item              | Qty | Equipped | Attuned | Weight | Container | Notes |
  |-------------------+-----+----------+---------+--------+-----------+-------|
  | Backpack          |   1 | no       | no      |      5 |           |       |
  | Potion of Healing |   3 | no       | no      |    0.5 | backpack  |       |
  #+END_SRC

  An empty container means carried on the person. Storage is deliberately
  flat: a pouch inside a backpack is just a line stored in the backpack, its
  own contents stay listed under the pouch.

  Containers come from the ruleset - any item with =container: true= is one,
  and =capacity= is what it holds in pounds. Rulesets that say nothing fall
  back to the standard list below, so the generated SRD needs no annotation.
  An =extradimensional= container (a bag of holding) carries its contents
  somewhere else, so what is inside it does not count against what the
  character is carrying.

  Encumbrance follows the variant rule: over five times Strength you are
  encumbered, over ten times heavily encumbered, and over fifteen times you
  are past your carrying capacity altogether.

  The =Equipped= column says what is being worn or wielded, and it is what the
  AC and the attack list are computed from: armour you are not wearing does
  nothing for you. On the html sheet the Worn column of the inventory is a
  toggle, so a ring goes on with a click. Only what can be worn or wielded
  offers one - armour, shields, weapons, rings, wands, staffs, rods and
  wondrous items, plus homebrew the ruleset has never heard of - and only on
  the person: something in a backpack has to come out first, the same rule
  that takes gear off when it is packed away.
EDOC */

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ----------------------------------------------------------------------------
// Containers
// ----------------------------------------------------------------------------

// ContainerSpec is what a container holds.
type ContainerSpec struct {
	// Capacity in pounds, 0 when the rules count something else (a quiver
	// holds twenty arrows, not a weight).
	Capacity float64
	// Extradimensional containers hold their contents outside the world, so
	// the weight inside them is not carried.
	Extradimensional bool
}

// StandardContainers is the fallback used for rulesets that do not mark their
// containers, which includes the generated SRD. Keyed by item id.
var StandardContainers = map[string]ContainerSpec{
	"backpack":            {Capacity: 30},
	"barrel":              {Capacity: 300},
	"basket":              {Capacity: 40},
	"bucket":              {Capacity: 20},
	"case-crossbow-bolt":  {},
	"case-map-or-scroll":  {},
	"chest":               {Capacity: 300},
	"component-pouch":     {},
	"pouch":               {Capacity: 6},
	"quiver":              {},
	"sack":                {Capacity: 30},
	"bag-of-holding":      {Capacity: 500, Extradimensional: true},
	"handy-haversack":     {Capacity: 120, Extradimensional: true},
	"portable-hole":       {Extradimensional: true},
	"efficient-quiver":    {Extradimensional: true},
	"bag-of-devouring":    {Extradimensional: true},
	"heward-handy-hversk": {Capacity: 120, Extradimensional: true},
}

// ItemContainer reports whether an item can be stored in and what it holds.
// The ruleset has the first word; anything it does not mark falls back to the
// standard list.
func ItemContainer(it *Item) (ContainerSpec, bool) {
	if it == nil {
		return ContainerSpec{}, false
	}
	if it.Container {
		return ContainerSpec{Capacity: it.Capacity, Extradimensional: it.Extradimensional}, true
	}
	if spec, ok := StandardContainers[it.Id]; ok {
		return spec, true
	}
	if spec, ok := StandardContainers[Slugify(it.Name)]; ok {
		return spec, true
	}
	return ContainerSpec{}, false
}

// ContainerKey is the stable name a container is referred to by, which is the
// slug of the item. "Bag of Holding", "bag-of-holding" and "BAG OF HOLDING"
// all name the same bag.
func ContainerKey(name string) string { return Slugify(strings.TrimSpace(name)) }

// ----------------------------------------------------------------------------
// Wearing and wielding
// ----------------------------------------------------------------------------

// EquippableKinds are the kinds of item a character wears or wields rather
// than merely carries: armour and shields go on, weapons and the magic
// implements are held, and rings and wondrous items - cloaks, boots, belts,
// amulets - are worn. Rations, rope and thieves' tools are carried and nothing
// more, so their Worn column has nothing to say.
var EquippableKinds = map[string]bool{
	"armor":    true,
	"shield":   true,
	"weapon":   true,
	"ring":     true,
	"wondrous": true,
	"wand":     true,
	"staff":    true,
	"rod":      true,
}

// CanEquip reports whether an item is something that can be worn or wielded.
//
// A container is never equippable however its kind reads: a bag of holding is
// typed as a wondrous item but it is carried, not worn, and the inventory
// already unequips anything packed into one.
//
// An item the ruleset does not know is treated as equippable. Homebrew written
// straight onto the equipment table says nothing about what it is, and
// refusing to let someone wear their own armour is a worse answer than
// offering to let them wear their own rope.
func CanEquip(it *Item) bool {
	if it == nil {
		return true
	}
	if _, isBox := ItemContainer(it); isBox {
		return false
	}
	return EquippableKinds[strings.ToLower(strings.TrimSpace(it.Kind))]
}

// CarriedLabel is what the character carries on their person rather than in
// anything, shown as the first tab of the inventory.
const CarriedLabel = "On Person"

// ----------------------------------------------------------------------------
// Inventory views
// ----------------------------------------------------------------------------

// InventoryEntry is one stack of identical things in one container.
type InventoryEntry struct {
	// Key identifies the stack for the api: the item id (or the slug of its
	// name for homebrew) together with the container it sits in.
	Key      string  `json:"key"`
	Id       string  `json:"id"`
	Name     string  `json:"name"`
	Qty      int     `json:"qty"`
	Weight   float64 `json:"weight"`
	Total    float64 `json:"total"`
	Equipped bool    `json:"equipped"`
	Attuned  bool    `json:"attuned"`
	// Wearable marks a stack that can be worn or wielded, so a sheet knows
	// which Worn boxes are worth offering as a toggle. Something stowed in a
	// container can be wearable without being wearable *there* - it has to
	// come out first, which is the same rule that unequips anything packed
	// away.
	Wearable  bool   `json:"wearable"`
	Notes     string `json:"notes"`
	Kind      string `json:"kind"`
	Container string `json:"container"`
	// IsContainer marks a stack that is itself a container, so the ui can
	// point at the tab holding what is inside it.
	IsContainer bool    `json:"isContainer"`
	Capacity    float64 `json:"capacity"`
	Magic       bool    `json:"magic"`
	// Rarity is the display rarity of a magic item and "" for ordinary gear,
	// so a sheet can colour a line by how rare what is on it is.
	Rarity string `json:"rarity"`
}

// InventoryContainer is everything stored in one place.
type InventoryContainer struct {
	// Key is "" for what is carried on the person, otherwise the container
	// item's slug.
	Key      string           `json:"key"`
	Name     string           `json:"name"`
	Entries  []InventoryEntry `json:"entries"`
	Weight   float64          `json:"weight"`
	Capacity float64          `json:"capacity"`
	Items    int              `json:"items"`
	// Extradimensional containers do not add their contents to what is
	// carried, Over is set when the contents weigh more than the capacity.
	Extradimensional bool `json:"extradimensional"`
	Over             bool `json:"over"`
	// Missing is set when lines claim to be in a container the character does
	// not own any more, so the things inside it are still reachable.
	Missing bool `json:"missing"`
}

// InventoryView is the whole bag: every container, the weight carried and
// where that lands on the encumbrance scale.
type InventoryView struct {
	Containers []InventoryContainer `json:"containers"`
	Weight     float64              `json:"weight"`
	// Stored is the weight held in extradimensional containers, which is
	// carried in the fiction but not on the character's back.
	Stored              float64 `json:"stored"`
	CarryCapacity       int     `json:"carryCapacity"`
	PushDragLift        int     `json:"pushDragLift"`
	EncumberedAt        int     `json:"encumberedAt"`
	HeavilyEncumberedAt int     `json:"heavilyEncumberedAt"`
	// Level is "", "encumbered", "heavy" or "over", Label and Effect say the
	// same thing in words.
	Level   string `json:"level"`
	Label   string `json:"label"`
	Effect  string `json:"effect"`
	Percent int    `json:"percent"`
	Items   int    `json:"items"`
}

// InventoryEvent is one line of the Inventory History section: something
// gained, used, dropped or moved between containers.
type InventoryEvent struct {
	Date   string `json:"date"`
	Time   string `json:"time"`
	Action string `json:"action"`
	Item   string `json:"item"`
	Qty    int    `json:"qty"`
	From   string `json:"from"`
	To     string `json:"to"`
	Notes  string `json:"notes"`
}

// The actions an inventory change is recorded under.
const (
	InvAdded   = "added"
	InvUsed    = "used"
	InvDropped = "dropped"
	InvMoved   = "moved"
	// InvWorn and InvRemoved are what wearing and taking something off are
	// called. They are never written to the Inventory History - see
	// InventoryEquip - they only name the change for the message a sheet
	// shows after it.
	InvWorn    = "worn"
	InvRemoved = "removed"
)

// InventoryRequest is a change to a character's inventory, posted by the html
// sheet. Item is an item id or name, Container the key of the container the
// change applies to, and To the container an item is being moved into.
type InventoryRequest struct {
	Filename  string `json:"filename"`
	Id        string `json:"id"`
	Action    string `json:"action"`
	Item      string `json:"item"`
	Name      string `json:"name"`
	Qty       int    `json:"qty"`
	Container string `json:"container"`
	To        string `json:"to"`
	// Equipped is the state the equip action puts the stack into. It is the
	// state wanted rather than a toggle, so two clicks racing each other end
	// up where the second one asked rather than wherever the order happened
	// to leave them.
	Equipped bool   `json:"equipped"`
	Notes    string `json:"notes"`
}

// InventoryState is the answer to every inventory call: the bag as it now
// stands, plus the history behind it.
type InventoryState struct {
	Id        string           `json:"id"`
	Name      string           `json:"name"`
	Filename  string           `json:"filename"`
	Ruleset   string           `json:"ruleset"`
	Inventory InventoryView    `json:"inventory"`
	History   []InventoryEvent `json:"history"`
	Money     Money            `json:"money"`
	Msg       string           `json:"msg"`
}

// ItemMatch is one hit from the item search behind the add item box.
type ItemMatch struct {
	Id          string  `json:"id"`
	Name        string  `json:"name"`
	Kind        string  `json:"kind"`
	Category    string  `json:"category"`
	Cost        string  `json:"cost"`
	Weight      float64 `json:"weight"`
	Damage      string  `json:"damage"`
	Rarity      string  `json:"rarity"`
	Text        string  `json:"text"`
	IsContainer bool    `json:"isContainer"`
	Capacity    float64 `json:"capacity"`
	Owned       int     `json:"owned"`
}

// ----------------------------------------------------------------------------
// Computing the inventory
// ----------------------------------------------------------------------------

// ComputeInventory stacks an equipment list into containers and works out the
// weight carried and the encumbrance that follows from it. The gear is
// expected to have been through the rules engine already, so names and
// weights are resolved.
func ComputeInventory(equip []Gear, rs *Ruleset, str int) InventoryView {
	view := InventoryView{
		Containers:          []InventoryContainer{},
		CarryCapacity:       str * 15,
		PushDragLift:        str * 30,
		EncumberedAt:        str * 5,
		HeavilyEncumberedAt: str * 10,
	}

	// Every container the character actually owns, and what it holds.
	specs := map[string]ContainerSpec{}
	names := map[string]string{}
	order := []string{""}
	for _, g := range equip {
		it := ruleItem(rs, g)
		spec, ok := ItemContainer(it)
		if !ok {
			continue
		}
		key := gearContainerKey(g, it)
		if _, seen := specs[key]; seen {
			continue
		}
		specs[key] = spec
		names[key] = gearName(g, it)
		order = append(order, key)
	}

	// Stack the lines, keyed by what they are and where they are kept.
	groups := map[string][]InventoryEntry{}
	index := map[string]int{}
	for _, g := range equip {
		it := ruleItem(rs, g)
		where := ContainerKey(g.Container)
		if where != "" {
			if _, ok := specs[where]; !ok {
				// A container the sheet no longer owns still has to show what
				// is inside it, or the things in it would simply vanish.
				specs[where] = ContainerSpec{}
				names[where] = Titleize(where)
				order = append(order, where)
			}
		}
		key := stackKey(g, it)
		spec, isBox := ItemContainer(it)
		entry := InventoryEntry{
			Key:         key,
			Id:          g.Id,
			Name:        gearName(g, it),
			Qty:         maxInt(g.Qty, 1),
			Weight:      g.Weight,
			Equipped:    g.Equipped,
			Attuned:     g.Attuned,
			Notes:       g.Notes,
			Container:   where,
			IsContainer: isBox,
			Capacity:    spec.Capacity,
			Wearable:    CanEquip(it),
		}
		if it != nil {
			entry.Kind = it.Kind
			entry.Magic = it.IsMagic()
			entry.Rarity = it.RarityName()
			if entry.Weight == 0 {
				entry.Weight = it.Weight
			}
		}
		at, seen := index[where+"\x00"+key]
		if seen {
			list := groups[where]
			list[at].Qty += entry.Qty
			// An equipped or attuned line in the stack marks the whole stack.
			list[at].Equipped = list[at].Equipped || entry.Equipped
			list[at].Attuned = list[at].Attuned || entry.Attuned
			if list[at].Notes == "" {
				list[at].Notes = entry.Notes
			}
			groups[where] = list
			continue
		}
		index[where+"\x00"+key] = len(groups[where])
		groups[where] = append(groups[where], entry)
	}

	for _, key := range order {
		spec := specs[key]
		entries := groups[key]
		if entries == nil {
			// An empty container is still a container: it answers with an
			// empty list rather than a null so a client can just walk it.
			entries = []InventoryEntry{}
		}
		box := InventoryContainer{
			Key:              key,
			Name:             CarriedLabel,
			Entries:          entries,
			Capacity:         spec.Capacity,
			Extradimensional: spec.Extradimensional,
		}
		if key != "" {
			box.Name = orDefault(names[key], Titleize(key))
			if _, owned := ownedContainer(equip, rs, key); !owned {
				box.Missing = true
			}
		}
		for i := range box.Entries {
			e := &box.Entries[i]
			e.Total = round2(e.Weight * float64(e.Qty))
			box.Weight += e.Total
			box.Items += e.Qty
		}
		box.Weight = round2(box.Weight)
		box.Over = box.Capacity > 0 && box.Weight > box.Capacity
		if box.Extradimensional {
			view.Stored += box.Weight
		} else {
			view.Weight += box.Weight
		}
		view.Items += box.Items
		view.Containers = append(view.Containers, box)
	}
	view.Weight = round2(view.Weight)
	view.Stored = round2(view.Stored)
	view.setEncumbrance()
	return view
}

// setEncumbrance places the weight carried on the variant encumbrance scale.
func (v *InventoryView) setEncumbrance() {
	if v.CarryCapacity > 0 {
		v.Percent = int(v.Weight / float64(v.CarryCapacity) * 100)
		if v.Percent > 100 {
			v.Percent = 100
		}
	}
	switch {
	case v.CarryCapacity > 0 && v.Weight > float64(v.CarryCapacity):
		v.Level, v.Label = "over", "Over capacity"
		v.Effect = "You cannot carry this much, drop something."
	case v.HeavilyEncumberedAt > 0 && v.Weight > float64(v.HeavilyEncumberedAt):
		v.Level, v.Label = "heavy", "Heavily encumbered"
		v.Effect = "Speed -20 ft., disadvantage on Str, Dex and Con checks, attacks and saves."
	case v.EncumberedAt > 0 && v.Weight > float64(v.EncumberedAt):
		v.Level, v.Label = "encumbered", "Encumbered"
		v.Effect = "Speed -10 ft."
	default:
		v.Level, v.Label = "", "Unencumbered"
		v.Effect = ""
	}
}

// Container returns one container of the view by key.
func (v *InventoryView) Container(key string) *InventoryContainer {
	key = ContainerKey(key)
	for i := range v.Containers {
		if v.Containers[i].Key == key {
			return &v.Containers[i]
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// Changing the inventory
// ----------------------------------------------------------------------------

// InventoryAdd puts a quantity of an item into a container, stacking it onto
// the line already there when there is one. Item is an id or a name; anything
// the ruleset does not know is kept as written, so homebrew still works.
func InventoryAdd(c *Character, rs *Ruleset, item, name string, qty int, container, notes string) (InventoryEvent, error) {
	if qty <= 0 {
		qty = 1
	}
	id, label, weight := resolveItem(rs, item, name)
	if label == "" {
		return InventoryEvent{}, fmt.Errorf("what item? pass an item id or a name")
	}
	where := ContainerKey(container)
	if err := checkContainer(c, rs, where); err != nil {
		return InventoryEvent{}, err
	}
	found := false
	for i := range c.Equipment {
		g := &c.Equipment[i]
		if ContainerKey(g.Container) != where {
			continue
		}
		if !sameItem(*g, id, label) {
			continue
		}
		g.Qty = maxInt(g.Qty, 1) + qty
		found = true
		break
	}
	if !found {
		c.Equipment = append(c.Equipment, Gear{
			Id: id, Name: label, Qty: qty, Weight: weight,
			Container: where, Notes: notes,
		})
	}
	return logEvent(c, InventoryEvent{
		Action: InvAdded, Item: label, Qty: qty,
		To: containerLabel(c, rs, where), Notes: notes,
	}), nil
}

// InventoryRemove takes a quantity off a stack. The action is what it is
// recorded as - used or dropped - and removing the last of something takes
// the line out of the equipment table altogether.
//
// Emptying out a container hands its contents back to the character rather
// than losing them with the bag.
func InventoryRemove(c *Character, rs *Ruleset, item string, qty int, container, action, notes string) (InventoryEvent, error) {
	if qty <= 0 {
		qty = 1
	}
	if action != InvUsed && action != InvDropped {
		action = InvDropped
	}
	where := ContainerKey(container)
	id, label, _ := resolveItem(rs, item, "")
	idx := findGear(c, id, label, where)
	if idx < 0 {
		return InventoryEvent{}, fmt.Errorf("no %s in %s", orDefault(label, item),
			strings.ToLower(containerLabel(c, rs, where)))
	}
	g := &c.Equipment[idx]
	have := maxInt(g.Qty, 1)
	if qty > have {
		qty = have
	}
	name := g.Name
	emptied := ""
	if qty >= have {
		// If this was a container, whatever was inside it comes out first.
		if it := ruleItem(rs, *g); it != nil {
			if _, isBox := ItemContainer(it); isBox {
				key := gearContainerKey(*g, it)
				if still := countIn(c, key); still > 0 && !anotherContainer(c, rs, key, idx) {
					emptied = spillContainer(c, key)
				}
			}
		}
		c.Equipment = append(c.Equipment[:idx], c.Equipment[idx+1:]...)
	} else {
		g.Qty = have - qty
	}
	if emptied != "" {
		notes = strings.TrimSpace(notes + " " + emptied)
	}
	return logEvent(c, InventoryEvent{
		Action: action, Item: name, Qty: qty,
		From: containerLabel(c, rs, where), Notes: notes,
	}), nil
}

// InventoryMove shifts a quantity of a stack from one container to another.
func InventoryMove(c *Character, rs *Ruleset, item string, qty int, from, to string) (InventoryEvent, error) {
	if qty <= 0 {
		qty = 1
	}
	src, dst := ContainerKey(from), ContainerKey(to)
	if src == dst {
		return InventoryEvent{}, fmt.Errorf("that is already in %s",
			strings.ToLower(containerLabel(c, rs, dst)))
	}
	if err := checkContainer(c, rs, dst); err != nil {
		return InventoryEvent{}, err
	}
	id, label, _ := resolveItem(rs, item, "")
	idx := findGear(c, id, label, src)
	if idx < 0 {
		return InventoryEvent{}, fmt.Errorf("no %s in %s", orDefault(label, item),
			strings.ToLower(containerLabel(c, rs, src)))
	}
	// A container cannot be put inside itself.
	if it := ruleItem(rs, c.Equipment[idx]); it != nil {
		if _, isBox := ItemContainer(it); isBox && gearContainerKey(c.Equipment[idx], it) == dst {
			return InventoryEvent{}, fmt.Errorf("%s cannot be put inside itself", c.Equipment[idx].Name)
		}
	}
	moved := c.Equipment[idx]
	have := maxInt(moved.Qty, 1)
	if qty > have {
		qty = have
	}
	if qty >= have {
		c.Equipment = append(c.Equipment[:idx], c.Equipment[idx+1:]...)
	} else {
		c.Equipment[idx].Qty = have - qty
	}
	landed := false
	for i := range c.Equipment {
		g := &c.Equipment[i]
		if ContainerKey(g.Container) != dst || !sameItem(*g, moved.Id, moved.Name) {
			continue
		}
		g.Qty = maxInt(g.Qty, 1) + qty
		landed = true
		break
	}
	if !landed {
		moved.Qty = qty
		moved.Container = dst
		// Something packed away is not being worn.
		if dst != "" {
			moved.Equipped = false
		}
		c.Equipment = append(c.Equipment, moved)
	}
	return logEvent(c, InventoryEvent{
		Action: InvMoved, Item: moved.Name, Qty: qty,
		From: containerLabel(c, rs, src), To: containerLabel(c, rs, dst),
	}), nil
}

// InventoryEquip wears or puts away a stack: it sets the Equipped flag on
// every line of the equipment table that stack was built from, so a pair of
// daggers held as one line of two goes on together the way it is shown.
//
// Only what can be worn or wielded can be equipped, and only on the person -
// something in a backpack has to come out first, which is the same rule
// InventoryMove applies when it packs something away.
//
// Nothing is written to the Inventory History. The history is the record of
// what a character has and has lost; drawing a sword and sheathing it again
// is neither, and a log of it would bury the lines that matter.
func InventoryEquip(c *Character, rs *Ruleset, item, container string, on bool) (InventoryEvent, error) {
	where := ContainerKey(container)
	id, label, _ := resolveItem(rs, item, "")
	idx := findGear(c, id, label, where)
	if idx < 0 {
		return InventoryEvent{}, fmt.Errorf("no %s in %s", orDefault(label, item),
			strings.ToLower(containerLabel(c, rs, where)))
	}
	name := gearName(c.Equipment[idx], ruleItem(rs, c.Equipment[idx]))
	if on {
		if where != "" {
			return InventoryEvent{}, fmt.Errorf("%s is in your %s, take it out first",
				name, strings.ToLower(containerLabel(c, rs, where)))
		}
		if !CanEquip(ruleItem(rs, c.Equipment[idx])) {
			return InventoryEvent{}, fmt.Errorf("%s is not something you wear or wield", name)
		}
	}
	// The whole stack, not just the first line of it: the inventory shows one
	// line for identical things kept in the same place, so equipping has to
	// mean the same thing the sheet is showing.
	qty := 0
	for i := range c.Equipment {
		g := &c.Equipment[i]
		if ContainerKey(g.Container) != where || !sameItem(*g, id, label) {
			continue
		}
		g.Equipped = on
		qty += maxInt(g.Qty, 1)
	}
	action := InvRemoved
	if on {
		action = InvWorn
	}
	return InventoryEvent{Action: action, Item: name, Qty: qty,
		From: containerLabel(c, rs, where)}, nil
}

// ----------------------------------------------------------------------------
// Item filters
// ----------------------------------------------------------------------------

// ItemFilter is one of the groups the add item box offers beside its search
// box: a name for the group and what falls into it. Filtering never changes
// how a search is ranked, it only drops what does not belong to the group, so
// "dagger" under Weapons is the same search with the rest taken away.
type ItemFilter struct {
	Id    string `json:"id"`
	Label string `json:"label"`
	// Match is nil for the group that lets everything through.
	Match func(*Item) bool `json:"-"`
}

// ItemFilters are the groups, in the order a sheet should offer them. The
// ids are what the =filter= parameter of the item search goes by.
var ItemFilters = []ItemFilter{
	{Id: "all", Label: "All"},
	{Id: "weapon", Label: "Weapons", Match: func(it *Item) bool { return it.Kind == "weapon" }},
	{Id: "armor", Label: "Armor", Match: func(it *Item) bool {
		return it.Kind == "armor" || it.Kind == "shield"
	}},
	{Id: "potion", Label: "Potions", Match: func(it *Item) bool { return it.Kind == "potion" }},
	{Id: "focus", Label: "Components", Match: isSpellFocus},
	{Id: "gear", Label: "Gear", Match: func(it *Item) bool { return it.Kind == "gear" }},
	{Id: "tool", Label: "Tools", Match: func(it *Item) bool { return it.Kind == "tool" }},
	{Id: "pack", Label: "Packs", Match: func(it *Item) bool { return it.Kind == "pack" }},
	{Id: "container", Label: "Containers", Match: func(it *Item) bool {
		_, ok := ItemContainer(it)
		return ok
	}},
	{Id: "magic", Label: "Magic", Match: (*Item).IsMagic},
}

// spellFoci are the things a caster channels a spell through. The SRD files
// each implement under plain gear - a crystal is a crystal - so the ones its
// three focus entries name are listed here by id instead.
var spellFoci = map[string]bool{
	"arcane-focus": true, "druidic-focus": true, "holy-symbol": true,
	"component-pouch": true,
	// an arcane focus
	"crystal": true, "orb": true, "rod": true, "staff": true, "wand": true,
	// a druidic focus
	"sprig-of-mistletoe": true, "totem": true, "wooden-staff": true, "yew-wand": true,
	// a holy symbol
	"amulet": true, "emblem": true, "reliquary": true,
}

func isSpellFocus(it *Item) bool {
	if it == nil {
		return false
	}
	if it.Category == "focus" || spellFoci[it.Id] {
		return true
	}
	return spellFoci[Slugify(it.Name)]
}

// FindItemFilter returns the filter of that name. An empty name is the group
// that lets everything through; ok is false for a name no filter goes by, so
// a typo is an error rather than a silent "everything".
func FindItemFilter(id string) (ItemFilter, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" || id == "all" {
		return ItemFilters[0], true
	}
	for _, f := range ItemFilters {
		if f.Id == id {
			return f, true
		}
	}
	return ItemFilter{}, false
}

// ItemFilterNames is every filter id, for the error a bad one gets.
func ItemFilterNames() []string {
	out := []string{}
	for _, f := range ItemFilters {
		out = append(out, f.Id)
	}
	return out
}

// SearchItems is the fuzzy item search behind the add item box: "helth pot"
// finds a Potion of Healing. An empty query returns the front of the list so
// the box has something in it before anything is typed. The filter narrows
// the pool the search runs over, not the answer it comes back with.
func SearchItems(rs *Ruleset, query string, limit int, filter ItemFilter, containersOnly bool, owned map[string]int) []ItemMatch {
	if rs == nil {
		return []ItemMatch{}
	}
	if limit <= 0 {
		limit = 40
	}
	items := []*Item{}
	labels, names := []string{}, []string{}
	for i := range rs.Items {
		it := &rs.Items[i]
		if _, isBox := ItemContainer(it); containersOnly && !isBox {
			continue
		}
		if filter.Match != nil && !filter.Match(it) {
			continue
		}
		items = append(items, it)
		names = append(names, it.Name)
		labels = append(labels, it.Name+" "+it.Category+" "+it.Kind+" "+it.Rarity)
	}
	if strings.TrimSpace(query) == "" {
		sort.SliceStable(items, func(a, b int) bool { return items[a].Name < items[b].Name })
		out := []ItemMatch{}
		for _, it := range items {
			if len(out) >= limit {
				break
			}
			out = append(out, itemMatch(it, owned))
		}
		return out
	}
	out := []ItemMatch{}
	for _, idx := range FuzzyMatches(query, labels, names) {
		if len(out) >= limit {
			break
		}
		out = append(out, itemMatch(items[idx], owned))
	}
	return out
}

// OwnedCounts is how many of each item id the character has, used to show
// "you have 2" beside a search hit.
func OwnedCounts(c *Character) map[string]int {
	out := map[string]int{}
	if c == nil {
		return out
	}
	for _, g := range c.Equipment {
		key := g.Id
		if key == "" {
			key = Slugify(g.Name)
		}
		out[key] += maxInt(g.Qty, 1)
	}
	return out
}

func itemMatch(it *Item, owned map[string]int) ItemMatch {
	spec, isBox := ItemContainer(it)
	m := ItemMatch{
		Id: it.Id, Name: it.Name, Kind: it.Kind, Category: it.Category,
		Cost: it.Cost, Weight: it.Weight, Damage: it.Damage, Rarity: it.RarityName(),
		Text: it.Text, IsContainer: isBox, Capacity: spec.Capacity,
	}
	if owned != nil {
		m.Owned = owned[it.Id]
	}
	return m
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

// logEvent stamps an event with the time and appends it to the character's
// history, which is what gets written into the Inventory History section.
func logEvent(c *Character, e InventoryEvent) InventoryEvent {
	now := time.Now()
	e.Date = now.Format("2006-01-02")
	e.Time = now.Format("15:04")
	if e.Qty <= 0 {
		e.Qty = 1
	}
	c.InventoryLog = append(c.InventoryLog, e)
	return e
}

// resolveItem turns an id or a name into the id, display name and weight to
// store. Anything the ruleset does not know keeps the name it was given.
func resolveItem(rs *Ruleset, item, name string) (string, string, float64) {
	item = strings.TrimSpace(item)
	name = strings.TrimSpace(name)
	if rs != nil {
		for _, key := range []string{item, name} {
			if key == "" {
				continue
			}
			if it := rs.Item(key); it != nil {
				return it.Id, it.Name, it.Weight
			}
		}
	}
	if name == "" {
		name = Titleize(item)
	}
	return "", name, 0
}

// sameItem is the stacking test: two lines stack when they are the same item,
// by id when both have one and by name otherwise.
func sameItem(g Gear, id, name string) bool {
	if id != "" && g.Id != "" {
		return g.Id == id
	}
	return strings.EqualFold(strings.TrimSpace(g.Name), strings.TrimSpace(name))
}

func findGear(c *Character, id, name, container string) int {
	for i := range c.Equipment {
		g := c.Equipment[i]
		if ContainerKey(g.Container) != container {
			continue
		}
		if sameItem(g, id, name) {
			return i
		}
	}
	return -1
}

// checkContainer refuses to store something in a container the character does
// not own, which is what keeps the tabs honest.
func checkContainer(c *Character, rs *Ruleset, key string) error {
	if key == "" {
		return nil
	}
	if _, ok := ownedContainer(c.Equipment, rs, key); ok {
		return nil
	}
	return fmt.Errorf("no %s in your inventory to store that in", Titleize(key))
}

// ownedContainer finds the gear line that is the container with this key.
func ownedContainer(equip []Gear, rs *Ruleset, key string) (Gear, bool) {
	key = ContainerKey(key)
	for _, g := range equip {
		it := ruleItem(rs, g)
		if _, isBox := ItemContainer(it); !isBox {
			continue
		}
		if gearContainerKey(g, it) == key {
			return g, true
		}
	}
	return Gear{}, false
}

// anotherContainer reports whether some line other than the one at skip is
// also this container, so that dropping one of two backpacks keeps the tab.
func anotherContainer(c *Character, rs *Ruleset, key string, skip int) bool {
	for i, g := range c.Equipment {
		if i == skip {
			continue
		}
		it := ruleItem(rs, g)
		if _, isBox := ItemContainer(it); !isBox {
			continue
		}
		if gearContainerKey(g, it) == key {
			return true
		}
	}
	return false
}

func countIn(c *Character, key string) int {
	n := 0
	for _, g := range c.Equipment {
		if ContainerKey(g.Container) == key {
			n += maxInt(g.Qty, 1)
		}
	}
	return n
}

// spillContainer tips everything in a container out onto the character.
func spillContainer(c *Character, key string) string {
	n := 0
	for i := range c.Equipment {
		if ContainerKey(c.Equipment[i].Container) != key {
			continue
		}
		c.Equipment[i].Container = ""
		n++
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("(%d %s moved onto your person)", n, plural(n, "line", "lines"))
}

// gearContainerKey is the key the tab for this container item uses.
func gearContainerKey(g Gear, it *Item) string {
	if it != nil && it.Id != "" {
		return it.Id
	}
	if g.Id != "" {
		return ContainerKey(g.Id)
	}
	return ContainerKey(g.Name)
}

// stackKey identifies what a line holds, ignoring where it is kept.
func stackKey(g Gear, it *Item) string {
	if it != nil && it.Id != "" {
		return it.Id
	}
	if g.Id != "" {
		return g.Id
	}
	return Slugify(g.Name)
}

// containerLabel is a container's name for the history, so the log reads
// "moved 2 Rations into Backpack" rather than naming a slug.
func containerLabel(c *Character, rs *Ruleset, key string) string {
	key = ContainerKey(key)
	if key == "" {
		return CarriedLabel
	}
	if g, ok := ownedContainer(c.Equipment, rs, key); ok {
		return gearName(g, ruleItem(rs, g))
	}
	return Titleize(key)
}

func ruleItem(rs *Ruleset, g Gear) *Item {
	if rs == nil {
		return nil
	}
	return rs.Item(gearKey(g))
}

func gearName(g Gear, it *Item) string {
	if g.Name != "" {
		return g.Name
	}
	if it != nil {
		return it.Name
	}
	return Titleize(g.Id)
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
