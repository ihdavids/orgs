//lint:file-ignore ST1006 allow the use of self
package dnd

// ----------------------------------------------------------------------------
// Coin
//
// The purse, and the arithmetic of spending out of it. D&D's coins are a fixed
// exchange: a hundred copper, ten silver, two electrum or a tenth of a platinum
// all come to one gold piece, and fifty coins of any kind weigh a pound. That
// table is the whole of the rules here - everything else is making change.
//
// Spending is the interesting part. A purse holding a single gold piece can pay
// for a two copper candle, so paying is not "take two copper" but "hand over
// the smallest thing that covers it and take the change", which is what
// SpendMoney does.
// ----------------------------------------------------------------------------

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Coin is one denomination: what it is called, and what it is worth in copper.
type Coin struct {
	Id    string `json:"id"`
	Name  string `json:"name"`
	Abbr  string `json:"abbr"`
	Value int    `json:"value"` // in copper pieces
}

// Coins are the five denominations, smallest first. The order matters: change
// is made by walking it, and the sheet lists the purse in it.
var Coins = []Coin{
	{Id: "cp", Name: "Copper", Abbr: "cp", Value: 1},
	{Id: "sp", Name: "Silver", Abbr: "sp", Value: 10},
	{Id: "ep", Name: "Electrum", Abbr: "ep", Value: 50},
	{Id: "gp", Name: "Gold", Abbr: "gp", Value: 100},
	{Id: "pp", Name: "Platinum", Abbr: "pp", Value: 1000},
}

// CoinsPerPound is the weight of coin: fifty of them, of any denomination,
// weigh one pound.
const CoinsPerPound = 50

// FindCoin looks a denomination up by id, abbreviation or name, so "gp",
// "gold" and "Gold Pieces" all find the same coin.
func FindCoin(id string) (Coin, bool) {
	key := strings.ToLower(strings.TrimSpace(id))
	key = strings.TrimSuffix(key, "s")
	key = strings.TrimSuffix(key, " piece")
	key = strings.TrimSpace(key)
	for _, c := range Coins {
		if key == c.Id || key == strings.ToLower(c.Name) ||
			key == strings.ToLower(c.Name)+" piece" {
			return c, true
		}
	}
	return Coin{}, false
}

// CoinNames lists the denominations for an error message.
func CoinNames() []string {
	out := make([]string, 0, len(Coins))
	for _, c := range Coins {
		out = append(out, c.Id)
	}
	return out
}

// Get reads one denomination out of a purse.
func (m Money) Get(id string) int {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "cp":
		return m.CP
	case "sp":
		return m.SP
	case "ep":
		return m.EP
	case "gp":
		return m.GP
	case "pp":
		return m.PP
	}
	return 0
}

// Set writes one denomination of a purse. Coin cannot go negative: a purse
// that owes money is not a thing the sheet can draw.
func (m *Money) Set(id string, n int) {
	if n < 0 {
		n = 0
	}
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "cp":
		m.CP = n
	case "sp":
		m.SP = n
	case "ep":
		m.EP = n
	case "gp":
		m.GP = n
	case "pp":
		m.PP = n
	}
}

// Add moves a denomination by n, which may be negative.
func (m *Money) Add(id string, n int) { m.Set(id, m.Get(id)+n) }

// Copper is what the whole purse is worth, in copper pieces. All the coin
// arithmetic goes through this, so nothing has to know the exchange twice.
func (m Money) Copper() int {
	total := 0
	for _, c := range Coins {
		total += m.Get(c.Id) * c.Value
	}
	return total
}

// Gold is the same total said the way a player says it.
func (m Money) Gold() float64 { return float64(m.Copper()) / 100 }

// Count is how many coins there are, all denominations together.
func (m Money) Count() int {
	n := 0
	for _, c := range Coins {
		n += m.Get(c.Id)
	}
	return n
}

// Weight is what the purse weighs, in pounds.
func (m Money) Weight() float64 { return round2(float64(m.Count()) / CoinsPerPound) }

// IsZero reports an empty purse.
func (m Money) IsZero() bool { return m.Count() == 0 }

// Plus and Minus are the two halves of adding purses together. Minus does not
// make change - it is for undoing an addition, not for paying - so use
// SpendMoney to take money out of a purse.
func (m Money) Plus(o Money) Money {
	out := m
	for _, c := range Coins {
		out.Add(c.Id, o.Get(c.Id))
	}
	return out
}

func (m Money) Minus(o Money) Money {
	out := m
	for _, c := range Coins {
		out.Add(c.Id, -o.Get(c.Id))
	}
	return out
}

// String writes a purse the way a character sheet says it: largest coin first,
// empty denominations left out.
func (m Money) String() string {
	parts := []string{}
	for i := len(Coins) - 1; i >= 0; i-- {
		if n := m.Get(Coins[i].Id); n != 0 {
			parts = append(parts, strconv.Itoa(n)+" "+Coins[i].Abbr)
		}
	}
	if len(parts) == 0 {
		return "0 gp"
	}
	return strings.Join(parts, " ")
}

// moneyTerm matches "15gp", "15 gp" or "15 gold" in a cost.
var moneyTerm = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(cp|sp|ep|gp|pp|copper|silver|electrum|gold|platinum)\b`)

// ParseMoney reads an amount of coin out of text: "15 gp", "3gp 4sp", the
// "50 gp" an item's cost field carries, or a bare number, which is gold. It is
// how a purchase gets its price and how the Coin History reads back.
func ParseMoney(s string) (Money, error) {
	var m Money
	s = strings.TrimSpace(s)
	if s == "" {
		return m, nil
	}
	hits := moneyTerm.FindAllStringSubmatch(s, -1)
	if len(hits) == 0 {
		// A bare number is gold, which is how prices are usually quoted, and
		// is kept as gold rather than changed up into platinum.
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			if f == float64(int(f)) {
				return Money{GP: int(f)}, nil
			}
			return FromCopper(int(f * 100)), nil
		}
		return m, fmt.Errorf("cannot read %q as an amount of coin, try \"15 gp\"", s)
	}
	for _, hit := range hits {
		coin, ok := FindCoin(hit[2])
		if !ok {
			continue
		}
		f, err := strconv.ParseFloat(hit[1], 64)
		if err != nil {
			continue
		}
		// A fractional price ("0.1 gp" for a candle) is worth exactly as much
		// copper as it says, so drop to copper rather than rounding it away.
		if f != float64(int(f)) {
			m = m.Plus(FromCopper(int(f * float64(coin.Value))))
			continue
		}
		m.Add(coin.Id, int(f))
	}
	if m.IsZero() && !strings.Contains(s, "0") {
		return m, fmt.Errorf("cannot read %q as an amount of coin, try \"15 gp\"", s)
	}
	return m, nil
}

// ValueString says what an amount is worth in the coin a player quotes prices
// in: gold, silver and copper, with no platinum. It is the total on the coin
// panel, where "52 gp 5 cp" reads better than the "5 pp 2 gp 5 cp" the same
// value makes in the fewest coins.
func ValueString(copper int) string {
	if copper <= 0 {
		return "0 gp"
	}
	parts := []string{}
	for _, c := range []Coin{{Abbr: "gp", Value: 100}, {Abbr: "sp", Value: 10}, {Abbr: "cp", Value: 1}} {
		if n := copper / c.Value; n > 0 {
			parts = append(parts, strconv.Itoa(n)+" "+c.Abbr)
			copper -= n * c.Value
		}
	}
	return strings.Join(parts, " ")
}

// FromCopper turns a value in copper into the fewest coins that make it.
// Electrum is skipped: it is worth having when someone hands it to you, but
// nobody makes change in it.
func FromCopper(total int) Money {
	var m Money
	if total <= 0 {
		return m
	}
	for i := len(Coins) - 1; i >= 0; i-- {
		c := Coins[i]
		if c.Id == "ep" {
			continue
		}
		if n := total / c.Value; n > 0 {
			m.Add(c.Id, n)
			total -= n * c.Value
		}
	}
	return m
}

// ----------------------------------------------------------------------------
// Spending
// ----------------------------------------------------------------------------

// SpendMoney pays a cost out of a purse and gives back the purse that is left,
// along with the coins actually handed over and the change taken back. Coins
// small enough to help are spent first; when what is left owes less than the
// smallest coin still in the purse, one larger coin is broken and the change
// comes back in the fewest coins that make it - exactly what happens when you
// pay for a two copper candle with a gold piece.
func SpendMoney(purse, cost Money) (left, paid, change Money, err error) {
	need := cost.Copper()
	if need <= 0 {
		return purse, Money{}, Money{}, fmt.Errorf("nothing to spend")
	}
	if have := purse.Copper(); need > have {
		return purse, Money{}, Money{}, fmt.Errorf(
			"not enough coin: that costs %s and the purse holds %s",
			ValueString(need), ValueString(have))
	}
	left = purse
	// Largest coin first, but never more than what is still owed, so the
	// purse keeps its small change for the next small purchase.
	for i := len(Coins) - 1; i >= 0 && need > 0; i-- {
		c := Coins[i]
		n := need / c.Value
		if have := left.Get(c.Id); n > have {
			n = have
		}
		if n <= 0 {
			continue
		}
		left.Add(c.Id, -n)
		paid.Add(c.Id, n)
		need -= n * c.Value
	}
	// Whatever is still owed is smaller than any coin left, so one of them
	// has to be broken. The smallest that covers it is the one to hand over.
	if need > 0 {
		for _, c := range Coins {
			if c.Value < need || left.Get(c.Id) <= 0 {
				continue
			}
			left.Add(c.Id, -1)
			paid.Add(c.Id, 1)
			change = FromCopper(c.Value - need)
			left = left.Plus(change)
			need = 0
			break
		}
	}
	if need > 0 {
		// Unreachable while the purse is worth more than the cost, but a
		// silent half payment would be worse than saying so.
		return purse, Money{}, Money{}, fmt.Errorf(
			"cannot make %s out of %s", cost.String(), purse.String())
	}
	return left, paid, change, nil
}

// ConsolidateMoney rolls small change up into the largest coins that hold the
// same value, which is what a character does when their pouch gets heavy.
// Electrum is left alone unless withElectrum is set, since a purse that keeps
// turning gold into electrum is a nuisance rather than a convenience.
func ConsolidateMoney(m Money, withElectrum bool) Money {
	keep := Money{}
	if !withElectrum {
		keep.EP = m.EP
		m.EP = 0
	}
	out := FromCopper(m.Copper())
	return out.Plus(keep)
}

// ExchangeMoney trades qty coins of one denomination for another at the
// standard rate. The trade has to come out even - three silver is not two
// electrum and change - so an uneven swap is refused rather than rounded.
func ExchangeMoney(m Money, from, to string, qty int) (Money, Money, error) {
	src, ok := FindCoin(from)
	if !ok {
		return m, Money{}, fmt.Errorf("no coin called %q, expected one of %s",
			from, strings.Join(CoinNames(), ", "))
	}
	dst, ok := FindCoin(to)
	if !ok {
		return m, Money{}, fmt.Errorf("no coin called %q, expected one of %s",
			to, strings.Join(CoinNames(), ", "))
	}
	if src.Id == dst.Id {
		return m, Money{}, fmt.Errorf("%s is already %s", src.Name, dst.Name)
	}
	if qty <= 0 {
		return m, Money{}, fmt.Errorf("how many %s?", src.Abbr)
	}
	if have := m.Get(src.Id); qty > have {
		return m, Money{}, fmt.Errorf("only %d %s to change, not %d", have, src.Abbr, qty)
	}
	value := qty * src.Value
	if value%dst.Value != 0 {
		return m, Money{}, fmt.Errorf("%d %s is %s, which is not a whole number of %s",
			qty, src.Abbr, FromCopper(value).String(), dst.Abbr)
	}
	got := Money{}
	got.Add(dst.Id, value/dst.Value)
	out := m
	out.Add(src.Id, -qty)
	out = out.Plus(got)
	return out, got, nil
}

// ----------------------------------------------------------------------------
// The purse as the sheet shows it
// ----------------------------------------------------------------------------

// CoinView is one denomination on the coin panel: how many are held and what
// they come to.
type CoinView struct {
	Id    string  `json:"id"`
	Name  string  `json:"name"`
	Abbr  string  `json:"abbr"`
	Qty   int     `json:"qty"`
	Value int     `json:"value"` // one coin, in copper
	Gold  float64 `json:"gold"`  // this pile, in gold
}

// MoneyView is the whole purse worked out: every denomination, the total, and
// what it weighs.
type MoneyView struct {
	Coins  []CoinView `json:"coins"`
	Money  Money      `json:"money"`
	Copper int        `json:"copper"`
	Gold   float64    `json:"gold"`
	Total  string     `json:"total"`
	Count  int        `json:"count"`
	Weight float64    `json:"weight"`
}

// ComputeMoney is the purse as the coin tab draws it: largest coin first, so
// it reads the way a character sheet writes it.
func ComputeMoney(m Money) MoneyView {
	view := MoneyView{
		Coins: []CoinView{}, Money: m, Copper: m.Copper(), Gold: m.Gold(),
		Count: m.Count(), Weight: m.Weight(),
	}
	for i := len(Coins) - 1; i >= 0; i-- {
		c := Coins[i]
		n := m.Get(c.Id)
		view.Coins = append(view.Coins, CoinView{
			Id: c.Id, Name: c.Name, Abbr: c.Abbr, Qty: n, Value: c.Value,
			Gold: round2(float64(n*c.Value) / 100),
		})
	}
	view.Total = ValueString(view.Copper)
	if view.Copper == 0 {
		view.Total = "nothing"
	}
	return view
}

// ----------------------------------------------------------------------------
// The coin history
// ----------------------------------------------------------------------------

// The actions a coin change is recorded under.
const (
	CoinSpent       = "spent"
	CoinGained      = "gained"
	CoinSet         = "set"
	CoinConsolidate = "consolidated"
	CoinExchanged   = "exchanged"
)

// MoneyEvent is one line of the Coin History section: what moved, what the
// purse held afterwards, and what it was for.
type MoneyEvent struct {
	Date    string `json:"date"`
	Time    string `json:"time"`
	Action  string `json:"action"`
	Amount  Money  `json:"amount"`
	Balance Money  `json:"balance"`
	// Change is what came back when a larger coin had to be broken, so the
	// history can say a gold piece went out and nine silver came back.
	Change Money  `json:"change"`
	Notes  string `json:"notes"`
}

// MoneyRequest is one change to a character's purse, posted by the html sheet.
// Amount is the text form ("15 gp 3 sp"), Money the same thing said in fields;
// whichever is filled in is used, and Money wins if both are.
type MoneyRequest struct {
	Filename string `json:"filename"`
	Id       string `json:"id"`
	Action   string `json:"action"`
	Amount   string `json:"amount"`
	Money    Money  `json:"money"`
	From     string `json:"from"`
	To       string `json:"to"`
	Qty      int    `json:"qty"`
	Notes    string `json:"notes"`
	Electrum bool   `json:"electrum"`
}

// MoneyState is the answer to every coin call: the purse as it now stands and
// the history behind it.
type MoneyState struct {
	Id       string       `json:"id"`
	Name     string       `json:"name"`
	Filename string       `json:"filename"`
	Ruleset  string       `json:"ruleset"`
	Purse    MoneyView    `json:"purse"`
	History  []MoneyEvent `json:"history"`
	Msg      string       `json:"msg"`
}

// RequestedMoney is the amount a request is about, however it was said.
func (r MoneyRequest) RequestedMoney() (Money, error) {
	if !r.Money.IsZero() {
		return r.Money, nil
	}
	return ParseMoney(r.Amount)
}

// ApplyMoney runs one coin change against a character and records it. The
// character's purse is only written when the change succeeds, so a purchase
// that cannot be paid for leaves the sheet exactly as it was.
func ApplyMoney(c *Character, req MoneyRequest) (MoneyEvent, error) {
	action := strings.ToLower(strings.TrimSpace(req.Action))
	purse := c.Money
	e := MoneyEvent{Action: action, Notes: strings.TrimSpace(req.Notes)}

	switch action {
	case "spend", "pay", "buy":
		cost, err := req.RequestedMoney()
		if err != nil {
			return e, err
		}
		left, _, change, err := SpendMoney(purse, cost)
		if err != nil {
			return e, err
		}
		e.Action = CoinSpent
		e.Amount = cost
		e.Change = change
		purse = left

	case "gain", "earn", "add", "loot":
		got, err := req.RequestedMoney()
		if err != nil {
			return e, err
		}
		if got.IsZero() {
			return e, fmt.Errorf("nothing to add")
		}
		e.Action = CoinGained
		e.Amount = got
		purse = purse.Plus(got)

	case "set":
		// The purse said outright, for fixing it up by hand.
		want := req.Money
		if want.IsZero() && strings.TrimSpace(req.Amount) != "" {
			parsed, err := ParseMoney(req.Amount)
			if err != nil {
				return e, err
			}
			want = parsed
		}
		e.Action = CoinSet
		e.Amount = want
		purse = want

	case "consolidate", "convert", "stack":
		out := ConsolidateMoney(purse, req.Electrum)
		if out == purse {
			return e, fmt.Errorf("the purse is already in the largest coins it can be")
		}
		e.Action = CoinConsolidate
		e.Amount = out
		purse = out

	case "exchange", "change", "trade":
		out, got, err := ExchangeMoney(purse, req.From, req.To, req.Qty)
		if err != nil {
			return e, err
		}
		e.Action = CoinExchanged
		e.Amount = got
		src, _ := FindCoin(req.From)
		if e.Notes == "" {
			e.Notes = fmt.Sprintf("%d %s for %s", req.Qty, src.Abbr, got.String())
		}
		purse = out

	default:
		return e, fmt.Errorf(
			"unknown action %q, expected spend, gain, set, consolidate or exchange", req.Action)
	}

	c.Money = purse
	e.Balance = purse
	return logMoney(c, e), nil
}

// logMoney stamps a coin event with the time and appends it to the character's
// coin history, which is what gets written into the Coin History section.
func logMoney(c *Character, e MoneyEvent) MoneyEvent {
	now := time.Now()
	e.Date = now.Format("2006-01-02")
	e.Time = now.Format("15:04")
	c.MoneyLog = append(c.MoneyLog, e)
	return e
}

// MoneyEventMsg is the one line the sheet shows after a coin change.
func MoneyEventMsg(e MoneyEvent) string {
	switch e.Action {
	case CoinSpent:
		msg := "spent " + e.Amount.String()
		if !e.Change.IsZero() {
			msg += ", " + e.Change.String() + " back in change"
		}
		return msg
	case CoinGained:
		return "added " + e.Amount.String()
	case CoinSet:
		return "purse set to " + e.Amount.String()
	case CoinConsolidate:
		return "changed up to " + e.Amount.String()
	case CoinExchanged:
		return "changed for " + e.Amount.String()
	}
	return e.Action
}

// ItemCost reads the price out of an item's cost field, so the coin tab can
// offer to pay for what the add item box just found.
func ItemCost(it *Item) (Money, bool) {
	if it == nil || strings.TrimSpace(it.Cost) == "" {
		return Money{}, false
	}
	m, err := ParseMoney(it.Cost)
	if err != nil || m.IsZero() {
		return Money{}, false
	}
	return m, true
}

// moneyCell writes an amount for an org table cell, leaving an empty purse as
// a blank rather than "0 gp", so a table of purchases without change reads
// cleanly.
func moneyCell(m Money) string {
	if m.IsZero() {
		return ""
	}
	return m.String()
}
