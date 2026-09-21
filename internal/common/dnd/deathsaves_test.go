package dnd

import "testing"

func dyingChar() (*Character, *Ruleset) {
	rs := NewLibrary(nil).Get("srd")
	c := &Character{
		Name:      "Downed",
		Ruleset:   "srd",
		Abilities: map[string]int{"str": 10, "dex": 10, "con": 14, "int": 10, "wis": 10, "cha": 10},
		Classes:   []ClassLevel{{Class: "fighter", Level: 3}},
		HPMax:     28,
		HPCurrent: 0,
	}
	return c, rs
}

func TestDeathSaveMarks(t *testing.T) {
	if s, f := ParseDeathSaves("2/1"); s != 2 || f != 1 {
		t.Errorf("2/1 = %d/%d", s, f)
	}
	if s, f := ParseDeathSaves(""); s != 0 || f != 0 {
		t.Errorf("empty = %d/%d", s, f)
	}
	// Free text a player wrote counts as no marks rather than failing to load.
	if s, f := ParseDeathSaves("s1 f1"); s != 0 || f != 0 {
		t.Errorf("free text = %d/%d", s, f)
	}
	// Nothing runs past three either way.
	if s, f := ParseDeathSaves("9/9"); s != 3 || f != 3 {
		t.Errorf("9/9 = %d/%d", s, f)
	}
	if got := FormatDeathSaves(0, 0); got != "" {
		t.Errorf("no marks = %q, want empty", got)
	}
	if got := FormatDeathSaves(1, 2); got != "1/2" {
		t.Errorf("1/2 = %q", got)
	}
}

// The d20 reads itself: 10 or better is a success, a natural 20 puts you back
// up, and a natural 1 costs two failures.
func TestDeathSaveReadsTheDie(t *testing.T) {
	cases := []struct {
		roll int
		succ int
		fail int
		hp   int
	}{
		{15, 1, 0, 0},
		{10, 1, 0, 0},
		{9, 0, 1, 0},
		{2, 0, 1, 0},
		{1, 0, 2, 0},
		{20, 0, 0, 1},
	}
	for _, tc := range cases {
		c, rs := dyingChar()
		e, err := ApplyHealth(c, rs, HealthRequest{Action: DeathSave, Roll: tc.roll})
		if err != nil {
			t.Fatalf("roll %d: %s", tc.roll, err)
		}
		s, f := ParseDeathSaves(c.DeathSaves)
		if s != tc.succ || f != tc.fail || c.HPCurrent != tc.hp {
			t.Errorf("roll %d gave %d/%d on %d hp, want %d/%d on %d hp",
				tc.roll, s, f, c.HPCurrent, tc.succ, tc.fail, tc.hp)
		}
		// The die is worth keeping: the marks cannot say what it landed on.
		if e.Notes == "" {
			t.Errorf("roll %d left no note", tc.roll)
		}
	}
}

func TestDeathSaveInWords(t *testing.T) {
	c, rs := dyingChar()
	for i := 0; i < 3; i++ {
		if _, err := ApplyHealth(c, rs, HealthRequest{
			Action: DeathSave, Result: SaveFailure,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if c.DeathSaves != "0/3" {
		t.Errorf("death saves = %q, want 0/3", c.DeathSaves)
	}
	hp := ComputeHealth(Compute(c, rs))
	if !hp.Dead || hp.Dying || hp.Stable {
		t.Errorf("three failures: dead=%v dying=%v stable=%v", hp.Dead, hp.Dying, hp.Stable)
	}
	// And once it is settled there is nothing left to roll.
	if _, err := ApplyHealth(c, rs, HealthRequest{Action: DeathSave, Roll: 20}); err == nil {
		t.Error("rolled a death save after dying")
	}
}

func TestThreeSuccessesAreStable(t *testing.T) {
	c, rs := dyingChar()
	for i := 0; i < 3; i++ {
		if _, err := ApplyHealth(c, rs, HealthRequest{
			Action: DeathSave, Result: SaveSuccess,
		}); err != nil {
			t.Fatal(err)
		}
	}
	hp := ComputeHealth(Compute(c, rs))
	if !hp.Stable || hp.Dying || hp.Dead {
		t.Errorf("stable=%v dying=%v dead=%v", hp.Stable, hp.Dying, hp.Dead)
	}
	// Stable is still down: nobody is walking about on nought hit points.
	if !hp.Down || hp.Current != 0 {
		t.Errorf("stable on %d hit points, down=%v", hp.Current, hp.Down)
	}
}

func TestStabilizeAndClear(t *testing.T) {
	c, rs := dyingChar()
	if _, err := ApplyHealth(c, rs, HealthRequest{Action: Stabilize}); err != nil {
		t.Fatal(err)
	}
	if c.DeathSaves != "3/0" {
		t.Errorf("stabilised = %q, want 3/0", c.DeathSaves)
	}
	if _, err := ApplyHealth(c, rs, HealthRequest{Action: Stabilize}); err == nil {
		t.Error("stabilised twice")
	}
	if _, err := ApplyHealth(c, rs, HealthRequest{Action: ClearDeath}); err != nil {
		t.Fatal(err)
	}
	if c.DeathSaves != "" {
		t.Errorf("cleared = %q", c.DeathSaves)
	}
}

// A death save is only a question while you are down, and being healed off
// nought wipes the marks - which is the rule the existing healing already had.
func TestDeathSavesOnlyWhileDown(t *testing.T) {
	c, rs := dyingChar()
	c.HPCurrent = 5
	if _, err := ApplyHealth(c, rs, HealthRequest{Action: DeathSave, Roll: 15}); err == nil {
		t.Error("rolled a death save on 5 hit points")
	}
	if _, err := ApplyHealth(c, rs, HealthRequest{Action: Stabilize}); err == nil {
		t.Error("stabilised someone who was not dying")
	}

	c.HPCurrent = 0
	c.DeathSaves = "1/2"
	if _, err := ApplyHealth(c, rs, HealthRequest{Action: Heal, Amount: 4}); err != nil {
		t.Fatal(err)
	}
	if c.DeathSaves != "" {
		t.Errorf("healing left the marks at %q", c.DeathSaves)
	}
}

// Every event carries the marks as they stood, so a line of the Health History
// about a death saving throw can say what it came to.
func TestDeathSaveEventCarriesMarks(t *testing.T) {
	c, rs := dyingChar()
	e, err := ApplyHealth(c, rs, HealthRequest{Action: DeathSave, Result: SaveSuccess})
	if err != nil {
		t.Fatal(err)
	}
	if e.DeathSaves != "1/0" {
		t.Errorf("event marks = %q", e.DeathSaves)
	}
	if msg := HealthEventMsg(e); msg == "" || msg == e.Action {
		t.Errorf("message = %q", msg)
	}
	if line := HealthEventLine(e); line == "" {
		t.Error("no org line for a death save")
	}
}
