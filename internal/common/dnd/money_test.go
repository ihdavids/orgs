package dnd

import (
	"strings"
	"testing"
)

func TestCoinValues(t *testing.T) {
	// The exchange every other sum here rests on.
	m := Money{CP: 100}
	if m.Copper() != 100 || m.Gold() != 1 {
		t.Errorf("100 cp = %d cp, %.2f gp", m.Copper(), m.Gold())
	}
	for _, tc := range []struct {
		m    Money
		want int
	}{
		{Money{CP: 1}, 1},
		{Money{SP: 1}, 10},
		{Money{EP: 1}, 50},
		{Money{GP: 1}, 100},
		{Money{PP: 1}, 1000},
		{Money{CP: 5, SP: 3, EP: 1, GP: 2, PP: 1}, 5 + 30 + 50 + 200 + 1000},
	} {
		if got := tc.m.Copper(); got != tc.want {
			t.Errorf("%s = %d cp, want %d", tc.m, got, tc.want)
		}
	}
}

func TestCoinWeight(t *testing.T) {
	// Fifty coins to the pound, whatever they are made of.
	if w := (Money{GP: 50}).Weight(); w != 1 {
		t.Errorf("50 gp weighs %v lb, want 1", w)
	}
	if w := (Money{CP: 25, GP: 25}).Weight(); w != 1 {
		t.Errorf("50 mixed coins weigh %v lb, want 1", w)
	}
	if w := (Money{GP: 10}).Weight(); w != 0.2 {
		t.Errorf("10 gp weighs %v lb, want 0.2", w)
	}
}

func TestSpendExactCoins(t *testing.T) {
	left, paid, change, err := SpendMoney(Money{GP: 10, SP: 5}, Money{GP: 3})
	if err != nil {
		t.Fatal(err)
	}
	if left != (Money{GP: 7, SP: 5}) {
		t.Errorf("left = %+v", left)
	}
	if paid != (Money{GP: 3}) || !change.IsZero() {
		t.Errorf("paid %+v, change %+v", paid, change)
	}
}

func TestSpendMakesChange(t *testing.T) {
	// The candle problem: two copper out of a purse holding one gold piece.
	left, paid, change, err := SpendMoney(Money{GP: 1}, Money{CP: 2})
	if err != nil {
		t.Fatal(err)
	}
	if paid != (Money{GP: 1}) {
		t.Errorf("paid %+v, want the gold piece", paid)
	}
	if change != (Money{SP: 9, CP: 8}) {
		t.Errorf("change %+v, want 9 sp 8 cp", change)
	}
	if left.Copper() != 98 {
		t.Errorf("left %s = %d cp, want 98", left, left.Copper())
	}
}

func TestSpendUsesSmallCoinFirst(t *testing.T) {
	// Seven copper when the purse holds five copper and a silver: the copper
	// goes first and the silver is broken for what is left.
	left, _, _, err := SpendMoney(Money{CP: 5, SP: 1}, Money{CP: 7})
	if err != nil {
		t.Fatal(err)
	}
	if left.Copper() != 15-7 {
		t.Errorf("left %s = %d cp, want 8", left, left.Copper())
	}
	if left.CP != 8 || left.SP != 0 {
		t.Errorf("left %+v, want 8 cp", left)
	}
}

func TestSpendAcrossDenominations(t *testing.T) {
	// A price no single coin covers, paid out of a mixed purse.
	purse := Money{PP: 1, GP: 3, SP: 4, CP: 9}
	left, _, _, err := SpendMoney(purse, Money{GP: 11, SP: 2})
	if err != nil {
		t.Fatal(err)
	}
	if want := purse.Copper() - 1120; left.Copper() != want {
		t.Errorf("left %s = %d cp, want %d", left, left.Copper(), want)
	}
}

func TestSpendMoreThanTheresCoinFor(t *testing.T) {
	_, _, _, err := SpendMoney(Money{GP: 2}, Money{GP: 3})
	if err == nil {
		t.Fatal("spent more than the purse holds")
	}
	if !strings.Contains(err.Error(), "not enough coin") {
		t.Errorf("error = %v", err)
	}
}

func TestSpendElectrum(t *testing.T) {
	// Electrum is real money and pays for things, it just is not made in change.
	left, _, change, err := SpendMoney(Money{EP: 1}, Money{SP: 1})
	if err != nil {
		t.Fatal(err)
	}
	if change != (Money{GP: 0, SP: 4}) {
		t.Errorf("change %+v, want 4 sp", change)
	}
	if left.EP != 0 || left.Copper() != 40 {
		t.Errorf("left %+v", left)
	}
}

func TestParseMoney(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Money
	}{
		{"15 gp", Money{GP: 15}},
		{"15gp", Money{GP: 15}},
		{"3 gp 4 sp", Money{GP: 3, SP: 4}},
		{"2 platinum", Money{PP: 2}},
		{"5 silver pieces", Money{SP: 5}},
		{"50 gp", Money{GP: 50}},
		{"12", Money{GP: 12}},    // a bare number is gold
		{"0.1 gp", Money{SP: 1}}, // a tenth of a gold piece is ten copper, i.e. a silver
		{"1 gp, 5 cp", Money{GP: 1, CP: 5}},
	} {
		got, err := ParseMoney(tc.in)
		if err != nil {
			t.Errorf("%q: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q = %+v, want %+v", tc.in, got, tc.want)
		}
	}
	if _, err := ParseMoney("a fistful of nothing"); err == nil {
		t.Error("nonsense parsed as an amount")
	}
}

func TestMoneyString(t *testing.T) {
	if s := (Money{GP: 42, CP: 5}).String(); s != "42 gp 5 cp" {
		t.Errorf("string = %q", s)
	}
	if s := (Money{}).String(); s != "0 gp" {
		t.Errorf("empty purse = %q", s)
	}
}

func TestFromCopper(t *testing.T) {
	// The fewest coins that make the value, and never electrum.
	m := FromCopper(1234)
	if m != (Money{PP: 1, GP: 2, SP: 3, CP: 4}) {
		t.Errorf("1234 cp = %+v", m)
	}
	if m.EP != 0 {
		t.Error("change was made in electrum")
	}
	if m.Copper() != 1234 {
		t.Errorf("round trip = %d", m.Copper())
	}
}

func TestConsolidate(t *testing.T) {
	m := ConsolidateMoney(Money{CP: 250, SP: 30}, false)
	if m != (Money{GP: 5, SP: 5}) {
		t.Errorf("consolidated = %+v, want 5 gp 5 sp", m)
	}
	// Electrum is left where it is unless it is asked for.
	kept := ConsolidateMoney(Money{EP: 3, CP: 100}, false)
	if kept.EP != 3 || kept.GP != 1 {
		t.Errorf("consolidated = %+v, want the electrum kept", kept)
	}
	changed := ConsolidateMoney(Money{EP: 3, CP: 100}, true)
	if changed.EP != 0 || changed.Copper() != 250 {
		t.Errorf("consolidated = %+v, want the electrum changed up", changed)
	}
}

func TestExchange(t *testing.T) {
	out, got, err := ExchangeMoney(Money{SP: 30}, "sp", "gp", 30)
	if err != nil {
		t.Fatal(err)
	}
	if got != (Money{GP: 3}) || out != (Money{GP: 3}) {
		t.Errorf("got %+v, purse %+v", got, out)
	}
	// An uneven swap is refused rather than rounded.
	if _, _, err := ExchangeMoney(Money{SP: 3}, "sp", "gp", 3); err == nil {
		t.Error("3 sp changed into a whole number of gold")
	}
	// More than is held.
	if _, _, err := ExchangeMoney(Money{SP: 3}, "sp", "cp", 4); err == nil {
		t.Error("changed coins that were not there")
	}
	if _, _, err := ExchangeMoney(Money{SP: 3}, "sp", "shiny beads", 1); err == nil {
		t.Error("changed silver for something that is not a coin")
	}
}

func TestApplyMoney(t *testing.T) {
	c := &Character{Name: "Lyra", Money: Money{GP: 50}}

	e, err := ApplyMoney(c, MoneyRequest{Action: "spend", Amount: "12 gp", Notes: "a mule"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Money.GP != 38 {
		t.Errorf("after spending: %+v", c.Money)
	}
	if e.Action != CoinSpent || e.Balance != c.Money {
		t.Errorf("event = %+v", e)
	}
	if e.Date == "" || e.Time == "" {
		t.Error("event was not stamped with the time")
	}

	if _, err := ApplyMoney(c, MoneyRequest{Action: "gain", Money: Money{PP: 2}}); err != nil {
		t.Fatal(err)
	}
	if c.Money.PP != 2 || c.Money.GP != 38 {
		t.Errorf("after earning: %+v", c.Money)
	}

	if _, err := ApplyMoney(c, MoneyRequest{Action: "spend", Amount: "500 gp"}); err == nil {
		t.Error("spent more than the purse holds")
	}
	if c.Money.GP != 38 || len(c.MoneyLog) != 2 {
		t.Errorf("a refused purchase changed the purse: %+v %d", c.Money, len(c.MoneyLog))
	}

	if _, err := ApplyMoney(c, MoneyRequest{Action: "juggle"}); err == nil {
		t.Error("an unknown action was accepted")
	}
}

func TestApplyMoneySet(t *testing.T) {
	c := &Character{Money: Money{GP: 5}}
	if _, err := ApplyMoney(c, MoneyRequest{Action: "set", Money: Money{GP: 1, SP: 2}}); err != nil {
		t.Fatal(err)
	}
	if c.Money != (Money{GP: 1, SP: 2}) {
		t.Errorf("set = %+v", c.Money)
	}
	// Setting to nothing is a real thing to want: a purse can be emptied.
	if _, err := ApplyMoney(c, MoneyRequest{Action: "set", Amount: "0 gp"}); err != nil {
		t.Fatal(err)
	}
	if !c.Money.IsZero() {
		t.Errorf("set to nothing = %+v", c.Money)
	}
}

func TestComputeMoney(t *testing.T) {
	v := ComputeMoney(Money{PP: 1, GP: 42, CP: 5})
	if len(v.Coins) != 5 || v.Coins[0].Id != "pp" || v.Coins[4].Id != "cp" {
		t.Errorf("coins listed as %+v", v.Coins)
	}
	if v.Copper != 5205 || v.Total != "52 gp 5 cp" {
		t.Errorf("total = %d cp, %q", v.Copper, v.Total)
	}
	if v.Count != 48 || v.Weight != 0.96 {
		t.Errorf("count %d weight %v", v.Count, v.Weight)
	}
}

func TestItemCost(t *testing.T) {
	if m, ok := ItemCost(&Item{Cost: "50 gp"}); !ok || m.GP != 50 {
		t.Errorf("cost = %+v %v", m, ok)
	}
	if _, ok := ItemCost(&Item{}); ok {
		t.Error("an item with no price has a cost")
	}
	if _, ok := ItemCost(nil); ok {
		t.Error("nothing has a cost")
	}
}

func TestCoinHistoryRoundTrip(t *testing.T) {
	lib := NewLibrary(nil)
	lib.Resolve()
	rs := lib.Get("srd")
	if rs == nil {
		t.Skip("no srd ruleset")
	}
	c := &Character{
		Name: "Lyra", Race: "elf", Background: "sage",
		Classes:   []ClassLevel{{Class: "wizard", Level: 3}},
		Abilities: map[string]int{STR: 10, DEX: 14, CON: 12, INT: 16, WIS: 12, CHA: 10},
		Money:     Money{GP: 38, PP: 2},
	}
	if _, err := ApplyMoney(c, MoneyRequest{
		Action: "spend", Amount: "12 gp", Notes: "a mule",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyMoney(c, MoneyRequest{Action: "spend", Amount: "2 cp"}); err != nil {
		t.Fatal(err)
	}

	org := RenderOrg(c, rs)
	if !strings.Contains(org, CoinHistoryHeading) {
		t.Fatalf("no coin history section in:\n%s", org)
	}
	back, err := ParseOrg(org, rs)
	if err != nil {
		t.Fatal(err)
	}
	if back.Money != c.Money {
		t.Errorf("purse came back as %+v, want %+v", back.Money, c.Money)
	}
	if len(back.MoneyLog) != len(c.MoneyLog) {
		t.Fatalf("history came back with %d lines, want %d", len(back.MoneyLog), len(c.MoneyLog))
	}
	for i, e := range back.MoneyLog {
		want := c.MoneyLog[i]
		if e.Action != want.Action || e.Amount != want.Amount ||
			e.Balance != want.Balance || e.Change != want.Change || e.Notes != want.Notes {
			t.Errorf("line %d came back as %+v, want %+v", i, e, want)
		}
	}
}
