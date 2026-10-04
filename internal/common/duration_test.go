package common

import (
	"testing"
	"time"
)

// A login lifetime is usually counted in days, which Go's durations do not
// have; a bad value falls back to an hour rather than to something surprising.
func TestTokenExpiry(t *testing.T) {
	cases := map[string]time.Duration{
		"":      time.Hour,
		"30m":   30 * time.Minute,
		"8h":    8 * time.Hour,
		"7d":    7 * 24 * time.Hour,
		"2w":    14 * 24 * time.Hour,
		"1d12h": 36 * time.Hour,
		"1.5d":  36 * time.Hour,
		"soon":  time.Hour,
		"-5h":   time.Hour,
	}
	for in, want := range cases {
		s := ServerSettings{TokenExpiry: in}
		if got := s.GetTokenExpiry(); got != want {
			t.Errorf("%q: %v, want %v", in, got, want)
		}
	}
}
