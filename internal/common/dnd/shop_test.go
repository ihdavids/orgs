package dnd

import "testing"

// shopChar is a character with a purse and a backpack, which is as much as
// buying and selling needs.
func shopChar() (*Character, *Ruleset) {
	rs := NewLibrary(nil).Get("srd")
	c := &Character{
		Name:      "Shopper",
		Ruleset:   "srd",
		Abilities: map[string]int{"str": 10, "dex": 10, "con": 10, "int": 10, "wis": 10, "cha": 10},
		Classes:   []ClassLevel{{Class: "fighter", Level: 1}},
		Money:     Money{GP: 20},
		Equipment: []Gear{{Id: "backpack", Name: "Backpack", Qty: 1}},
	}
	return c, rs
}

func TestBuyPaysAndAdds(t *testing.T) {
	c, rs := shopChar()
	// A longsword is 15 gp in the SRD.
	deal, err := BuyItem(c, rs, "longsword", "", 1, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if deal.Price != (Money{GP: 15}) {
		t.Errorf("price = %s, want 15 gp", deal.Price)
	}
	if c.Money.Copper() != (Money{GP: 5}).Copper() {
		t.Errorf("purse = %s, want 5 gp", c.Money)
	}
	if n := countGear(c, "longsword"); n != 1 {
		t.Errorf("longswords carried = %d, want 1", n)
	}
	// Both histories carry the deal, and the inventory line says bought
	// rather than added: the sheet should not have to read the coin log to
	// know the difference.
	if len(c.InventoryLog) != 1 || c.InventoryLog[0].Action != InvBought {
		t.Errorf("inventory log = %+v", c.InventoryLog)
	}
	if len(c.MoneyLog) != 1 || c.MoneyLog[0].Action != CoinSpent {
		t.Errorf("coin log = %+v", c.MoneyLog)
	}
}

func TestBuyMultipliesThePrice(t *testing.T) {
	c, rs := shopChar()
	// Ten torches at 1 cp each, into the backpack.
	deal, err := BuyItem(c, rs, "torch", "", 10, "backpack", "")
	if err != nil {
		t.Fatal(err)
	}
	if deal.Price.Copper() != 10 {
		t.Errorf("ten torches cost %s, want 10 cp", deal.Price)
	}
	if deal.Each.Copper() != 1 {
		t.Errorf("each = %s, want 1 cp", deal.Each)
	}
	// Paid out of a gold piece, so the change comes back.
	if c.Money.Copper() != (Money{GP: 20}).Copper()-10 {
		t.Errorf("purse = %s", c.Money)
	}
}

func TestBuyRefusedLeavesEverythingAlone(t *testing.T) {
	c, rs := shopChar()
	c.Money = Money{GP: 2}
	before := c.Money
	if _, err := BuyItem(c, rs, "plate", "", 1, "", ""); err == nil {
		t.Fatal("bought 1500 gp of plate armour out of 2 gp")
	}
	if c.Money != before {
		t.Errorf("purse moved to %s", c.Money)
	}
	if len(c.Equipment) != 1 || len(c.MoneyLog) != 0 || len(c.InventoryLog) != 0 {
		t.Errorf("a refused purchase left a mark: equip %d, coin %d, inv %d",
			len(c.Equipment), len(c.MoneyLog), len(c.InventoryLog))
	}
}

// A container the character does not own is the other way a purchase fails,
// and it has to fail before any coin changes hands.
func TestBuyIntoMissingContainerPaysNothing(t *testing.T) {
	c, rs := shopChar()
	before := c.Money
	if _, err := BuyItem(c, rs, "torch", "", 1, "chest", ""); err == nil {
		t.Fatal("bought something into a chest that is not carried")
	}
	if c.Money != before {
		t.Errorf("purse = %s, want it untouched at %s", c.Money, before)
	}
	if len(c.MoneyLog) != 0 {
		t.Errorf("coin log = %+v, want nothing", c.MoneyLog)
	}
}

// Something the rules put no price on is not bought or sold for coin, and the
// sheet is told why rather than being given a made up value.
func TestUnpricedCannotBeTraded(t *testing.T) {
	c, rs := shopChar()
	c.Equipment = append(c.Equipment, Gear{Name: "Grandfather's Locket", Qty: 1})
	if _, err := BuyItem(c, rs, "", "Grandfather's Locket", 1, "", ""); err == nil {
		t.Error("bought something the rules never heard of")
	}
	if _, err := SellItem(c, rs, "", "Grandfather's Locket", 1, "", ""); err == nil {
		t.Error("sold something the rules never heard of")
	}
	if len(c.MoneyLog) != 0 {
		t.Errorf("coin log = %+v, want nothing", c.MoneyLog)
	}
}

func TestSellGivesHalf(t *testing.T) {
	c, rs := shopChar()
	c.Equipment = append(c.Equipment, Gear{Id: "longsword", Name: "Longsword", Qty: 1})
	deal, err := SellItem(c, rs, "longsword", "", 1, "", "")
	if err != nil {
		t.Fatal(err)
	}
	// 15 gp new, so 7 gp 5 sp second hand.
	if deal.Price.Copper() != 750 {
		t.Errorf("sale = %s, want 7 gp 5 sp", deal.Price)
	}
	if c.Money.Copper() != (Money{GP: 20}).Copper()+750 {
		t.Errorf("purse = %s", c.Money)
	}
	if n := countGear(c, "longsword"); n != 0 {
		t.Errorf("longswords left = %d, want 0", n)
	}
	if len(c.InventoryLog) != 1 || c.InventoryLog[0].Action != InvSold {
		t.Errorf("inventory log = %+v", c.InventoryLog)
	}
}

// Half of one copper is nothing, and paying out a coin that does not exist
// would be worse than saying so.
func TestSellWorthlessIsRefused(t *testing.T) {
	c, rs := shopChar()
	c.Equipment = append(c.Equipment, Gear{Id: "torch", Name: "Torch", Qty: 1})
	if _, err := SellItem(c, rs, "torch", "", 1, "", ""); err == nil {
		t.Fatal("sold a 1 cp torch for half a copper")
	}
	if n := countGear(c, "torch"); n != 1 {
		t.Errorf("the torch went anyway: %d left", n)
	}
	if c.Money.Copper() != (Money{GP: 20}).Copper() {
		t.Errorf("purse = %s", c.Money)
	}
}

// Selling something not carried must not pay out either.
func TestSellWhatYouDoNotHave(t *testing.T) {
	c, rs := shopChar()
	if _, err := SellItem(c, rs, "longsword", "", 1, "", ""); err == nil {
		t.Fatal("sold a longsword nobody had")
	}
	if c.Money.Copper() != (Money{GP: 20}).Copper() {
		t.Errorf("purse = %s", c.Money)
	}
}

func countGear(c *Character, id string) int {
	n := 0
	for _, g := range c.Equipment {
		if g.Id == id {
			n += maxInt(g.Qty, 1)
		}
	}
	return n
}
