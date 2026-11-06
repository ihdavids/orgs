package recipe

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	isoDurRe   = regexp.MustCompile(`(?i)^P(?:(\d+(?:\.\d+)?)Y)?(?:(\d+(?:\.\d+)?)M)?(?:(\d+(?:\.\d+)?)W)?(?:(\d+(?:\.\d+)?)D)?(?:T(?:(\d+(?:\.\d+)?)H)?(?:(\d+(?:\.\d+)?)M)?(?:(\d+(?:\.\d+)?)S)?)?$`)
	clockDurRe = regexp.MustCompile(`^(\d+):(\d{1,2})$`)
	wordDurRe  = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(d|days?|h|hrs?|hours?|m|mins?|minutes?|s|secs?|seconds?)\b`)
)

// ParseDuration is a time in minutes, read from what a page or a person
// writes: ISO 8601 (PT1H20M, P0DT0H15M, PT90M), org-chef's "15  m", "1 h 20
// min", "1 hour 20 minutes", "1:30", a bare "45". 0 when it is none of them.
func ParseDuration(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if m := isoDurRe.FindStringSubmatch(s); m != nil && len(s) > 1 {
		f := func(i int) float64 { v, _ := strconv.ParseFloat(m[i], 64); return v }
		mins := f(1)*525600 + f(2)*43200 + f(3)*10080 + f(4)*1440 + f(5)*60 + f(6) + f(7)/60
		return int(mins + 0.5)
	}
	if m := clockDurRe.FindStringSubmatch(s); m != nil {
		h, _ := strconv.Atoi(m[1])
		mi, _ := strconv.Atoi(m[2])
		return h*60 + mi
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	total, any := 0.0, false
	for _, m := range wordDurRe.FindAllStringSubmatch(s, -1) {
		v, _ := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
		switch strings.ToLower(m[2])[0] {
		case 'd':
			total += v * 1440
		case 'h':
			total += v * 60
		case 'm':
			total += v
		case 's':
			total += v / 60
		}
		any = true
	}
	if !any {
		return 0
	}
	return int(total + 0.5)
}

// FormatMinutes writes a time the way a heading shows it: "15 min", "1 h",
// "1 h 20 min", "1 d 2 h". "" for none.
func FormatMinutes(m int) string {
	if m <= 0 {
		return ""
	}
	d, h, mi := m/1440, (m%1440)/60, m%60
	parts := []string{}
	if d > 0 {
		parts = append(parts, fmt.Sprintf("%d d", d))
	}
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%d h", h))
	}
	if mi > 0 {
		parts = append(parts, fmt.Sprintf("%d min", mi))
	}
	return strings.Join(parts, " ")
}
