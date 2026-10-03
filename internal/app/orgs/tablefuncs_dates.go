package orgs

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// civil is the calendar date of t, so day counts ignore clock and zone.
func civil(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func dateArg(name string, args []interface{}, i int) (time.Time, error) {
	if i >= len(args) {
		return time.Time{}, fmt.Errorf("%s: missing date argument", name)
	}
	t, ok := toTime(args[i])
	if !ok {
		return time.Time{}, fmt.Errorf("%s: [%v] is not a date", name, toStr(args[i]))
	}
	return t, nil
}

func intArg(name string, args []interface{}, i int) (int, error) {
	if i >= len(args) {
		return 0, fmt.Errorf("%s: missing number argument", name)
	}
	n, ok := cellInt(args[i])
	if !ok {
		return 0, fmt.Errorf("%s: [%v] is not a number", name, toStr(args[i]))
	}
	return n, nil
}

func tblToday(args ...interface{}) (interface{}, error) {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), nil
}

// days(end, start) is the number of calendar days from start to end.
func tblDays(args ...interface{}) (interface{}, error) {
	end, err := dateArg("days", args, 0)
	if err != nil {
		return nil, err
	}
	start, err := dateArg("days", args, 1)
	if err != nil {
		return nil, err
	}
	return int(math.Round(civil(end).Sub(civil(start)).Hours() / 24)), nil
}

// wholeMonths is the number of complete months from a to b (a <= b).
func wholeMonths(a, b time.Time) int {
	m := (b.Year()-a.Year())*12 + int(b.Month()) - int(a.Month())
	if b.Day() < a.Day() {
		m--
	}
	return m
}

// datedif(start, end, unit): unit is 'd' days, 'w' weeks, 'm' complete
// months or 'y' complete years.
func tblDateDif(args ...interface{}) (interface{}, error) {
	start, err := dateArg("datedif", args, 0)
	if err != nil {
		return nil, err
	}
	end, err := dateArg("datedif", args, 1)
	if err != nil {
		return nil, err
	}
	unit := "d"
	if len(args) > 2 {
		unit = strings.ToLower(toStr(args[2]))
	}
	sign := 1
	a, b := civil(start), civil(end)
	if b.Before(a) {
		a, b, sign = b, a, -1
	}
	days := int(math.Round(b.Sub(a).Hours() / 24))
	switch unit {
	case "d":
		return sign * days, nil
	case "w":
		return sign * (days / 7), nil
	case "m":
		return sign * wholeMonths(a, b), nil
	case "y":
		return sign * (wholeMonths(a, b) / 12), nil
	}
	return nil, fmt.Errorf("datedif unit must be d, w, m or y, not [%s]", unit)
}

func tblAddDays(args ...interface{}) (interface{}, error) {
	t, err := dateArg("adddays", args, 0)
	if err != nil {
		return nil, err
	}
	n, err := intArg("adddays", args, 1)
	if err != nil {
		return nil, err
	}
	return t.AddDate(0, 0, n), nil
}

// addMonths keeps the day of the month, or the month's last day when the
// target month is shorter: Jan 31 + 1 month is Feb 28, not Mar 3.
func addMonths(t time.Time, n int) time.Time {
	first := time.Date(t.Year(), t.Month()+time.Month(n), 1, t.Hour(), t.Minute(), t.Second(), 0, t.Location())
	last := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(t.Day(), last)-1)
}

func tblAddMonths(args ...interface{}) (interface{}, error) {
	t, err := dateArg("addmonths", args, 0)
	if err != nil {
		return nil, err
	}
	n, err := intArg("addmonths", args, 1)
	if err != nil {
		return nil, err
	}
	return addMonths(t, n), nil
}

func tblAddYears(args ...interface{}) (interface{}, error) {
	t, err := dateArg("addyears", args, 0)
	if err != nil {
		return nil, err
	}
	n, err := intArg("addyears", args, 1)
	if err != nil {
		return nil, err
	}
	return addMonths(t, 12*n), nil
}

// addtime(date, duration) adds a duration ('1d 2h', '1:30', or a duration
// value) to a date.
func tblAddTime(args ...interface{}) (interface{}, error) {
	t, err := dateArg("addtime", args, 0)
	if err != nil {
		return nil, err
	}
	if len(args) != 2 {
		return nil, fmt.Errorf("addtime expects (date, duration)")
	}
	switch d := argVal(args[1]).(type) {
	case common.OrgDuration:
		return t.Add(d.Duration()), nil
	case string:
		if pd := common.ParseDuration(d); pd != nil {
			return t.Add(pd.Duration()), nil
		}
	case float64:
		return t.Add(time.Duration(d * float64(time.Minute))), nil
	}
	return nil, fmt.Errorf("addtime: [%v] is not a duration", toStr(args[1]))
}

// eomonth(date [, months]) is the last day of the month, months later.
func tblEOMonth(args ...interface{}) (interface{}, error) {
	t, err := dateArg("eomonth", args, 0)
	if err != nil {
		return nil, err
	}
	n := 0
	if len(args) > 1 {
		if n, err = intArg("eomonth", args, 1); err != nil {
			return nil, err
		}
	}
	first := time.Date(t.Year(), t.Month()+time.Month(n), 1, 0, 0, 0, 0, t.Location())
	return first.AddDate(0, 1, -1), nil
}

// weeknum is the ISO 8601 week number.
func tblWeekNum(args ...interface{}) (interface{}, error) {
	return timeOp("weeknum", func(tm time.Time) (interface{}, error) {
		_, w := tm.ISOWeek()
		return w, nil
	}, args...)
}

func tblQuarter(args ...interface{}) (interface{}, error) {
	return timeOp("quarter", func(tm time.Time) (interface{}, error) {
		return (int(tm.Month())-1)/3 + 1, nil
	}, args...)
}

func isWeekend(t time.Time) bool {
	return t.Weekday() == time.Saturday || t.Weekday() == time.Sunday
}

// workday(date, n) is n working days (Monday to Friday) after date, or
// before it when n is negative.
func tblWorkday(args ...interface{}) (interface{}, error) {
	t, err := dateArg("workday", args, 0)
	if err != nil {
		return nil, err
	}
	n, err := intArg("workday", args, 1)
	if err != nil {
		return nil, err
	}
	step := 1
	if n < 0 {
		step, n = -1, -n
	}
	for n > 0 {
		t = t.AddDate(0, 0, step)
		if !isWeekend(t) {
			n--
		}
	}
	return t, nil
}

// networkdays(start, end) counts working days from start to end, both
// included; negative when end is before start.
func tblNetworkDays(args ...interface{}) (interface{}, error) {
	start, err := dateArg("networkdays", args, 0)
	if err != nil {
		return nil, err
	}
	end, err := dateArg("networkdays", args, 1)
	if err != nil {
		return nil, err
	}
	a, b := civil(start), civil(end)
	sign := 1
	if b.Before(a) {
		a, b, sign = b, a, -1
	}
	n := 0
	for d := a; !d.After(b); d = d.AddDate(0, 0, 1) {
		if !isWeekend(d) {
			n++
		}
	}
	return sign * n, nil
}

var strftimeVerbs = map[byte]string{
	'Y': "2006", 'y': "06", 'm': "01", 'd': "02", 'e': "_2",
	'H': "15", 'I': "03", 'M': "04", 'S': "05", 'p': "PM",
	'a': "Mon", 'A': "Monday", 'b': "Jan", 'B': "January",
	'Z': "MST", 'z': "-0700",
}

// datefmt(date, pattern) formats with strftime verbs, as org's
// format-time-string does: %Y-%m-%d %a %H:%M. %j is the day of the year,
// %V the ISO week; %% is a percent sign.
func tblDateFmt(args ...interface{}) (interface{}, error) {
	t, err := dateArg("datefmt", args, 0)
	if err != nil {
		return nil, err
	}
	if len(args) != 2 {
		return nil, fmt.Errorf("datefmt expects (date, pattern)")
	}
	pattern := toStr(args[1])
	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '%' || i+1 == len(pattern) {
			b.WriteByte(pattern[i])
			continue
		}
		i++
		c := pattern[i]
		switch {
		case c == '%':
			b.WriteByte('%')
		case c == 'j':
			fmt.Fprintf(&b, "%03d", t.YearDay())
		case c == 'V':
			_, w := t.ISOWeek()
			fmt.Fprintf(&b, "%02d", w)
		case strftimeVerbs[c] != "":
			b.WriteString(t.Format(strftimeVerbs[c]))
		default:
			b.WriteByte('%')
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}
