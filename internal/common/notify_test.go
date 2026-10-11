package common

import (
	"testing"
	"time"
)

func TestQuietHours(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 11, h, m, 0, 0, time.Local) }
	night, err := ParseQuiet("22:00-07:00")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		h, m int
		in   bool
	}{{23, 0, true}, {2, 30, true}, {6, 59, true}, {7, 0, false}, {12, 0, false}, {21, 59, false}, {22, 0, true}} {
		if got := night.In(at(c.h, c.m)); got != c.in {
			t.Errorf("22:00-07:00 at %02d:%02d = %v, want %v", c.h, c.m, got, c.in)
		}
	}
	lunch, _ := ParseQuiet("12–13")
	if !lunch.In(at(12, 30)) || lunch.In(at(13, 0)) {
		t.Error("12-13 is noon up to one")
	}
	if none, _ := ParseQuiet(""); none.In(at(3, 0)) {
		t.Error("no quiet hours is never quiet")
	}
	for _, bad := range []string{"22:00", "25:00-07:00", "ten-eleven", "22:61-07:00"} {
		if _, err := ParseQuiet(bad); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
}

func TestHeadingLink(t *testing.T) {
	got := HeadingLink("http://localhost:8010/", "/n/work notes.org", 12, "Standup & sync")
	want := "http://localhost:8010/?app=Files&heading=Standup+%26+sync&line=12&open=%2Fn%2Fwork+notes.org"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if HeadingLink("", "/a.org", 1, "") != "" {
		t.Fatal("no base, no link")
	}
}
