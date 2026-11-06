package common

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// The same cases as parseHistDate's in worg/src/orgtimeline.test.ts and
// vscode-org-everywhere/test/timeline-model.test.ts: input, label, start,
// end, precision ("" for no reading).
var timelineDateCases = [][5]string{
	{"1066", "1066", "1066", "", "year"},
	{"1066-10", "Oct 1066", "1066-10", "", "month"},
	{"1066-10-14", "14 Oct 1066", "1066-10-14", "", "day"},
	{"1066-10-14 09:30", "14 Oct 1066 09:30", "1066-10-14T09:30", "", "minute"},
	{"14 October 1066", "14 Oct 1066", "1066-10-14", "", "day"},
	{"Oct 14, 1066", "14 Oct 1066", "1066-10-14", "", "day"},
	{"October 1066", "Oct 1066", "1066-10", "", "month"},
	{"-500", "500 BC", "-0499", "", "year"},
	{"500 BC", "500 BC", "-0499", "", "year"},
	{"500 BCE", "500 BC", "-0499", "", "year"},
	{"-44-03-15", "15 Mar 44 BC", "-0043-03-15", "", "day"},
	{"AD 800", "800", "0800", "", "year"},
	{"800 CE", "800", "0800", "", "year"},
	{"c. 1200", "c. 1200", "1200", "", "year"},
	{"ca. 1200", "c. 1200", "1200", "", "year"},
	{"circa 1200", "c. 1200", "1200", "", "year"},
	{"~1200", "c. 1200", "1200", "", "year"},
	{"1200?", "c. 1200", "1200", "", "year"},
	{"1950s", "1950s", "1950", "1960", "decade"},
	{"12th century", "12th century", "1101", "1201", "century"},
	{"5th century BC", "5th century BC", "-0499", "-0399", "century"},
	{"1939-1945", "1939 – 1945", "1939", "1946", "year"},
	{"1939--1945", "1939 – 1945", "1939", "1946", "year"},
	{"1939 – 1945", "1939 – 1945", "1939", "1946", "year"},
	{"1914 to 1918", "1914 – 1918", "1914", "1919", "year"},
	{"500 - 400 BC", "500 BC – 400 BC", "-0499", "-0398", "year"},
	{"c. 1450 .. 1500", "c. 1450 – 1500", "1450", "1501", "year"},
	{"<2026-11-04 Wed>", "4 Nov 2026", "2026-11-04", "", "day"},
	{"<2026-11-04 Wed 10:00>--<2026-11-06 Fri>", "4 Nov 2026 10:00 – 6 Nov 2026", "2026-11-04T10:00", "2026-11-07", "day"},
	{"1 BC", "1 BC", "0000", "", "year"},
	{"yesterday", "", "", "", ""},
	{"1066-13", "", "", "", ""},
	{"31 Feb 2020", "", "", "", ""},
	{"1945-1939", "", "", "", ""},
}

func TestParseTimelineDate(t *testing.T) {
	for _, c := range timelineDateCases {
		d, ok := ParseTimelineDate(c[0])
		if c[4] == "" {
			if ok {
				t.Errorf("%q read as %+v, want nothing", c[0], d)
			}
			continue
		}
		if !ok {
			t.Errorf("%q not read", c[0])
			continue
		}
		got := [5]string{c[0], d.Label, d.Start, d.End, d.Precision}
		if got != c {
			t.Errorf("%q: got %q, want %q", c[0], got, c)
		}
	}
}

// Positions are wall-clock fields read as UTC, whatever the year.
func TestTimelineDatePositions(t *testing.T) {
	d, _ := ParseTimelineDate("1066-10-14")
	if d.S != -28502755200000 {
		t.Errorf("1066-10-14 at %d", d.S)
	}
	d, _ = ParseTimelineDate("44 BC")
	if d.S != -63524131200000 {
		t.Errorf("44 BC at %d", d.S)
	}
	if got := TimelineWall(time.Date(2026, 11, 4, 9, 0, 0, 0, time.FixedZone("x", -7*3600))); got != 1793782800000 {
		t.Errorf("wall clock: %d", got)
	}
}

func tickString(ts []TimelineTick) string {
	parts := []string{}
	for _, t := range ts {
		m := ""
		if t.Major {
			m = "*"
		}
		parts = append(parts, t.Label+m)
	}
	return strings.Join(parts, " ")
}

// The same cases as timelineTicks's in orgtimeline.test.ts: from, to (ISO
// fields or a free date), width, the labels (* = major).
var timelineTickCases = [][4]string{
	{"2026-11-04T06:00", "2026-11-04T18:00", "800", "06:00 09:00 12:00 15:00 18:00"},
	{"2026-11-03T20:00", "2026-11-04T04:00", "800", "20:00 21:00 22:00 23:00 Nov 4* 01:00 02:00 03:00 04:00"},
	{"2026-11-01", "2026-11-08", "800", "Nov 1* Nov 2 Nov 3 Nov 4 Nov 5 Nov 6 Nov 7 Nov 8"},
	{"2026-10-20", "2026-11-20", "800", "Oct 26 Nov 2 Nov 9 Nov 16"},
	{"2026-01-01", "2026-12-31", "800", "2026* Apr Jul Oct"},
	{"2020", "2030", "800", "2020* 2022 2024 2026 2028 2030*"},
	{"1900", "2000", "800", "1900* 1910 1920 1930 1940 1950 1960 1970 1980 1990 2000*"},
	{"300 BC", "300", "800", "300 BC 200 BC 100 BC 1* 100 200 300"},
	{"3000 BC", "2000", "800", "3000 BC* 2500 BC 2000 BC* 1500 BC 1000 BC* 500 BC 1* 500 1000* 1500 2000*"},
}

func tickPos(s string) int64 {
	if t, err := time.Parse("2006-01-02T15:04", s); err == nil {
		return t.UnixMilli()
	}
	d, _ := ParseTimelineDate(s)
	return d.S
}

func TestTimelineTicks(t *testing.T) {
	for _, c := range timelineTickCases {
		var w float64
		fmt.Sscan(c[2], &w)
		got := tickString(TimelineTicks(tickPos(c[0]), tickPos(c[1]), w, 80))
		if got != c[3] {
			t.Errorf("%s .. %s: got %q, want %q", c[0], c[1], got, c[3])
		}
	}
}
