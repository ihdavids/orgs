package dnd

import "testing"

func TestApplyXP(t *testing.T) {
	c := &Character{Name: "Lyra", Classes: []ClassLevel{{Class: "wizard", Level: 2}}, XP: 300}
	steps := []struct {
		action string
		amount int
		want   int
		ready  bool
	}{
		{"add", 500, 800, false},
		{"", 100, 900, true}, // 900 buys level 3
		{"remove", 50, 850, false},
		{"remove", 5000, 0, false}, // never below nought
		{"set", 2700, 2700, true},
	}
	for _, s := range steps {
		if _, err := ApplyXP(c, s.action, s.amount); err != nil {
			t.Fatalf("%s %d: %v", s.action, s.amount, err)
		}
		st := NewXPState(c)
		if c.XP != s.want || st.Ready != s.ready {
			t.Errorf("%s %d: xp %d ready %v, want %d ready %v", s.action, s.amount, c.XP, st.Ready, s.want, s.ready)
		}
	}
	if _, err := ApplyXP(c, "add", -5); err == nil {
		t.Error("a negative amount was taken")
	}
	if _, err := ApplyXP(c, "double", 5); err == nil {
		t.Error("an unknown action was taken")
	}
}

func TestXPStateAtLevel20(t *testing.T) {
	c := &Character{Classes: []ClassLevel{{Class: "fighter", Level: 20}}, XP: 400000}
	if s := NewXPState(c); s.NextLevelXP != 0 || s.Ready {
		t.Errorf("level 20 still climbing: %+v", s)
	}
}
