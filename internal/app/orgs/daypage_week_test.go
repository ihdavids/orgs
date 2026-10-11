package orgs

import (
	"testing"
	"time"
)

// A week page is named for the most recent start day on or before the date -
// never the next one.
func TestWeekStartOn(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 10, d, 15, 0, 0, 0, time.Local) }
	// 2026-10-05 is a Monday, 2026-10-11 a Sunday.
	cases := []struct {
		at    int
		first string
		want  int
	}{
		{5, "Monday", 5}, {7, "Monday", 5}, {10, "monday", 5},
		{11, "Monday", 5}, // a Sunday stays in the week it ends, not the next
		{11, "", 5},       // empty is Monday
		{11, "Sunday", 11}, {10, "sun", 4},
		{6, "Wednesday", 0}, // Tue Oct 6: the week began Wed Sep 30
	}
	for _, c := range cases {
		got := weekStartOn(day(c.at), c.first)
		want := day(c.want) // day(0) is Sep 30
		if got.Format("2006-01-02") != want.Format("2006-01-02") {
			t.Errorf("weekStartOn(Oct %d, %q) = %s, want %s", c.at, c.first, got.Format("Mon 2006-01-02"), want.Format("Mon 2006-01-02"))
		}
	}
}
