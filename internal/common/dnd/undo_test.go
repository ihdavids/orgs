// Tests for undo: taking the last thing back, and refusing to when it is no
// longer the last thing.
package dnd

import (
	"strings"
	"testing"
)

func TestUndoPutsBackADrunkPotion(t *testing.T) {
	rs := srd(t)
	c := testChar()
	if _, err := InventoryAdd(c, rs, "potion-of-healing", "", 2, "", ""); err != nil {
		t.Fatalf("add: %v", err)
	}
	// Take a knock so there is room for the healing to land.
	if _, err := ApplyHealth(c, rs, HealthRequest{Action: Hurt, Amount: 10}); err != nil {
		t.Fatalf("hurt: %v", err)
	}
	hurt := c.HPCurrent
	used, err := UseItem(c, rs, InventoryRequest{
		Item: "potion-of-healing", Effect: UseHeal, Amount: 7,
	})
	if err != nil {
		t.Fatalf("use: %v", err)
	}
	if used.Health == nil {
		t.Fatalf("drinking a potion of healing should have moved the hit points")
	}
	if c.HPCurrent != hurt+7 {
		t.Fatalf("want %d hit points after the potion, got %d", hurt+7, c.HPCurrent)
	}

	plan := LastUndo(c, rs)
	if !plan.Can {
		t.Fatalf("want the potion to be undoable, got %q", plan.Why)
	}
	if !strings.Contains(plan.What, "Potion of Healing") ||
		!strings.Contains(plan.What, "hit points") {
		t.Fatalf("the plan should name the potion and the healing, got %q", plan.What)
	}
	if _, err := ApplyUndo(c, rs); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if c.HPCurrent != hurt {
		t.Fatalf("want the hit points back on %d, got %d", hurt, c.HPCurrent)
	}
	v := ComputeInventory(c.Equipment, rs, 10)
	e := entryFor(v, "", "Potion of Healing")
	if e == nil || e.Qty != 2 {
		t.Fatalf("want both potions back, got %+v", e)
	}
	// The accident should have left no trace at all.
	if len(c.InventoryLog) != 1 || c.InventoryLog[0].Action != InvAdded {
		t.Fatalf("the used line should be gone, got %+v", c.InventoryLog)
	}
	if len(c.HealthLog) != 1 || c.HealthLog[0].Action != "hurt" {
		t.Fatalf("the healing line should be gone, got %+v", c.HealthLog)
	}
}

func TestUndoRefusesOnceSomethingElseHasHappened(t *testing.T) {
	rs := srd(t)
	c := testChar()
	if _, err := InventoryAdd(c, rs, "potion-of-healing", "", 1, "", ""); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := ApplyHealth(c, rs, HealthRequest{Action: Hurt, Amount: 3}); err != nil {
		t.Fatalf("hurt: %v", err)
	}
	// The blow is the last thing now, so that is what undo offers - not the
	// potion behind it.
	plan := LastUndo(c, rs)
	if !plan.Can || plan.Kind != "hurt" {
		t.Fatalf("want the blow offered, got %+v", plan)
	}
	if _, err := ApplyUndo(c, rs); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if c.HPCurrent != Compute(c, rs).HPMax {
		t.Fatalf("want the damage undone, got %d", c.HPCurrent)
	}
	if len(c.HealthLog) != 0 {
		t.Fatalf("want the health log empty, got %+v", c.HealthLog)
	}
}

func TestUndoTakesBackAPurchaseAndThePriceWithIt(t *testing.T) {
	rs := srd(t)
	c := testChar()
	c.Money = Money{GP: 60}
	if _, err := BuyItem(c, rs, "backpack", "", 1, "", ""); err != nil {
		t.Fatalf("buy: %v", err)
	}
	if c.Money.Copper() == 6000 {
		t.Fatalf("the purchase should have cost something")
	}
	plan := LastUndo(c, rs)
	if !plan.Can || plan.Kind != InvBought {
		t.Fatalf("want the purchase offered, got %+v", plan)
	}
	if _, err := ApplyUndo(c, rs); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if c.Money != (Money{GP: 60}) {
		t.Fatalf("want the 60 gp back exactly as it was, got %s", c.Money.String())
	}
	if len(c.Equipment) != 0 {
		t.Fatalf("want the backpack gone again, got %+v", c.Equipment)
	}
	if len(c.MoneyLog) != 0 || len(c.InventoryLog) != 0 {
		t.Fatalf("want both logs empty, got %d coin and %d inventory lines",
			len(c.MoneyLog), len(c.InventoryLog))
	}
}

func TestUndoWillNotGuessAtASheetWithNoBeforeColumn(t *testing.T) {
	rs := srd(t)
	c := testChar()
	// A line as it reads back off a sheet written before the Was column
	// existed: the hit points after are known, the ones before are not.
	c.HPCurrent = 12
	c.HealthLog = []HealthEvent{{
		Date: "2026-09-18", Time: "19:32", Action: "healed", Amount: 7,
		HPAfter: 12, HPMax: 27,
	}}
	plan := LastUndo(c, rs)
	if plan.Can {
		t.Fatalf("a line with no before reading must not be undone")
	}
	if _, err := ApplyUndo(c, rs); err == nil {
		t.Fatalf("want a refusal")
	}
	if c.HPCurrent != 12 {
		t.Fatalf("the refusal must leave the hit points alone, got %d", c.HPCurrent)
	}
}

func TestUndoSurvivesTheRoundTripThroughTheOrgFile(t *testing.T) {
	rs := srd(t)
	c := testChar()
	if _, err := InventoryAdd(c, rs, "potion-of-healing", "", 1, "", ""); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := ApplyHealth(c, rs, HealthRequest{Action: Hurt, Amount: 9}); err != nil {
		t.Fatalf("hurt: %v", err)
	}
	org := RenderOrg(c, rs)
	back, err := ParseOrg(org, rs)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	plan := LastUndo(back, rs)
	if !plan.Can {
		t.Fatalf("want the blow still undoable after a round trip, got %q", plan.Why)
	}
	if _, err := ApplyUndo(back, rs); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if back.HPCurrent != c.HealthLog[0].HPBefore {
		t.Fatalf("want the hit points back on %d, got %d",
			c.HealthLog[0].HPBefore, back.HPCurrent)
	}
}
