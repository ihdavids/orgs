//lint:file-ignore ST1006 allow the use of self
package dnd

/* SDOC: DnD
* Buying And Selling

  The inventory knows what a character carries and the purse knows what they
  can afford, and buying something is the one move that needs both at once. Done
  as two separate changes it is easy to get half right - the item added and the
  coin never paid - so a purchase is one call that either happens or does not.

  The price is the item's own =cost= from the ruleset, times how many are being
  bought, and it is paid out of the purse the way any other payment is: a two
  copper candle bought with a gold piece comes back with nine silver and eight
  copper in change (see [[money.go]]).

  Selling is the same move backwards, at half the price. That is the customary
  rate rather than a rule in the SRD, so it lives here as =SellNumerator= over
  =SellDenominator= where it can be read and argued with, and the line it
  writes into the Coin History says what was sold.

  Something the rules put no price on cannot be bought or sold for coin: the
  sheet says so rather than guessing at a value. Loot with no entry in the
  rulebook is still added and dropped the ordinary way, and the coin for it is
  gained and spent on the coin tab by hand.
EDOC */

import (
	"fmt"
	"strings"
)

// SellFraction is what a shop gives for used goods, as a numerator and
// denominator so the arithmetic stays in copper and nothing is lost to
// floating point. Half is the customary rate.
const (
	SellNumerator   = 1
	SellDenominator = 2
)

// The actions a shop change is recorded under in the two histories.
const (
	InvBought = "bought"
	InvSold   = "sold"
)

// Deal is what one purchase or sale came to: the two events it wrote, and the
// price it settled at.
type Deal struct {
	Item  InventoryEvent `json:"item"`
	Coin  MoneyEvent     `json:"coin"`
	Price Money          `json:"price"`
	// Each is the price of one of them, which is what the sheet quotes before
	// the deal is done.
	Each Money  `json:"each"`
	Qty  int    `json:"qty"`
	Msg  string `json:"msg"`
}

// ItemPrice is what a number of something costs, and whether the rules put a
// price on it at all.
//
// The price keeps the coins the rules quoted it in: fifteen longswords cost
// 225 gp, not 2 pp 25 gp. Consolidating a price into the largest coins that
// hold it would be arithmetically the same and unreadable on a shelf label.
func ItemPrice(it *Item, qty int) (Money, bool) {
	each, ok := ItemCost(it)
	if !ok {
		return Money{}, false
	}
	if qty < 1 {
		qty = 1
	}
	return Money{
		CP: each.CP * qty, SP: each.SP * qty, EP: each.EP * qty,
		GP: each.GP * qty, PP: each.PP * qty,
	}, true
}

// SellPrice is what a shop gives for a number of something. Half of 15 gp is
// not a whole number of gold, so unlike a price this does go through copper and
// comes back in the fewest coins that make it. It rounds down, so selling one
// 1 cp candle back is worth nothing at all - which is the honest answer, and
// the caller says so rather than paying out a coin that does not exist.
func SellPrice(it *Item, qty int) (Money, bool) {
	full, ok := ItemPrice(it, qty)
	if !ok {
		return Money{}, false
	}
	return FromCopper(full.Copper() * SellNumerator / SellDenominator), true
}

// BuyItem pays for something and puts it in the bag, or does neither. The
// character is moved on in place only once the price is known to be payable, so
// a purchase that cannot be afforded leaves both the purse and the bag exactly
// as they were.
//
// Only what the ruleset prices can be bought: something it has never heard of
// has no price to pay, and inventing one would be worse than saying so.
func BuyItem(c *Character, rs *Ruleset, item, name string, qty int,
	container, notes string) (Deal, error) {
	d := Deal{Qty: maxInt(qty, 1)}
	if c == nil {
		return d, fmt.Errorf("no character")
	}
	it := shopItem(rs, item, name)
	if it == nil {
		return d, fmt.Errorf("the rules know no %s, so there is no price to pay - "+
			"add it and pay for it on the coin tab", shopName(item, name))
	}
	each, ok := ItemCost(it)
	if !ok {
		return d, fmt.Errorf("the rules put no price on %s - add it and pay for it "+
			"on the coin tab", it.Name)
	}
	price, _ := ItemPrice(it, d.Qty)
	d.Each, d.Price = each, price

	// Where the bag would refuse it is worth finding out before any coin
	// changes hands, so the usual reason a purchase fails never has to be
	// undone.
	if err := checkContainer(c, rs, ContainerKey(container)); err != nil {
		return d, err
	}

	// Pay first: a purchase that cannot be afforded must not add the item.
	// SpendMoney leaves the purse alone when it refuses.
	purse := c.Money
	coin, err := ApplyMoney(c, MoneyRequest{
		Action: "spend", Money: price,
		Notes: shopNote(InvBought, it.Name, d.Qty, notes),
	})
	if err != nil {
		return d, err
	}
	event, err := InventoryAdd(c, rs, it.Id, it.Name, d.Qty, container, notes)
	if err != nil {
		// The bag refused it after all, so the payment is put back exactly as
		// it stood - the coins themselves, not their value, since paying may
		// have broken a gold piece into silver - and the line the payment
		// wrote in the coin history comes off with it.
		c.Money = purse
		c.MoneyLog = trimLastMoney(c.MoneyLog)
		return d, err
	}
	// InventoryAdd writes the line as "added", which is true but not the
	// whole truth when it was paid for.
	event.Action = InvBought
	c.InventoryLog = setLastAction(c.InventoryLog, InvBought)
	d.Item, d.Coin = event, coin
	d.Msg = fmt.Sprintf("bought %s for %s", shopQty(it.Name, d.Qty), price.String())
	if !coin.Change.IsZero() {
		d.Msg += ", " + coin.Change.String() + " back"
	}
	return d, nil
}

// SellItem hands something over and takes the coin for it. As with buying,
// nothing moves unless all of it can: a sale of something not carried leaves
// the purse alone.
func SellItem(c *Character, rs *Ruleset, item, name string, qty int,
	container, notes string) (Deal, error) {
	d := Deal{Qty: maxInt(qty, 1)}
	if c == nil {
		return d, fmt.Errorf("no character")
	}
	it := shopItem(rs, item, name)
	if it == nil {
		return d, fmt.Errorf("the rules know no %s, so there is no price for it - "+
			"drop it and take the coin on the coin tab", shopName(item, name))
	}
	each, ok := ItemCost(it)
	if !ok {
		return d, fmt.Errorf("the rules put no price on %s - drop it and take the "+
			"coin on the coin tab", it.Name)
	}
	price, _ := SellPrice(it, d.Qty)
	d.Each, d.Price = FromCopper(each.Copper()*SellNumerator/SellDenominator), price
	if price.IsZero() {
		return d, fmt.Errorf("%s is not worth a single copper second hand",
			shopQty(it.Name, d.Qty))
	}

	// The item goes first here: unlike a purchase, the thing being given up is
	// the part that can fail.
	event, err := InventoryRemove(c, rs, it.Id, d.Qty, container, InvSold, notes)
	if err != nil {
		return d, err
	}
	// Taking coin in cannot fail once there is a non zero price for it, which
	// was settled above, so there is no half done sale to undo here.
	coin, err := ApplyMoney(c, MoneyRequest{
		Action: "gain", Money: price,
		Notes: shopNote(InvSold, it.Name, d.Qty, notes),
	})
	if err != nil {
		return d, err
	}
	d.Item, d.Coin = event, coin
	d.Msg = fmt.Sprintf("sold %s for %s", shopQty(it.Name, d.Qty), price.String())
	return d, nil
}

// shopItem resolves what is being traded to an entry in the ruleset, which is
// the only thing that carries a price.
func shopItem(rs *Ruleset, item, name string) *Item {
	if rs == nil {
		return nil
	}
	for _, which := range []string{item, name} {
		if strings.TrimSpace(which) == "" {
			continue
		}
		if it := rs.Item(which); it != nil {
			return it
		}
	}
	return nil
}

func shopName(item, name string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	if strings.TrimSpace(item) != "" {
		return item
	}
	return "that"
}

// shopQty says a count of something the way the message wants it read.
func shopQty(name string, qty int) string {
	if qty <= 1 {
		return name
	}
	return fmt.Sprintf("%d %s", qty, name)
}

// shopNote is the line the coin history carries, so the purse's own record says
// what the money went on without having to be read against the inventory's.
func shopNote(action, name string, qty int, notes string) string {
	said := action + " " + shopQty(name, qty)
	if strings.TrimSpace(notes) != "" {
		said += " - " + strings.TrimSpace(notes)
	}
	return said
}

// setLastAction renames the action on the line just written, so a purchase
// reads as bought rather than as added.
func setLastAction(log []InventoryEvent, action string) []InventoryEvent {
	if len(log) == 0 {
		return log
	}
	log[len(log)-1].Action = action
	return log
}

// trimLastMoney takes back the last line of the coin history, for the one case
// that has to undo a payment it has already made.
func trimLastMoney(log []MoneyEvent) []MoneyEvent {
	if len(log) == 0 {
		return log
	}
	return log[:len(log)-1]
}
