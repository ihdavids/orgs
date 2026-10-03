package orgs

import (
	"fmt"
	"math"
	"math/rand"
	"regexp"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Logic
// ---------------------------------------------------------------------------

// spread reads arguments for functions that take any number of values: a
// range such as @2$1..@>$1 gives every cell, a cell reference the current one.
func spread(args ...interface{}) []interface{} {
	out := []interface{}{}
	for _, a := range args {
		if r, ok := a.(*RangeIter); ok {
			if r.IsRange() {
				out = append(out, argList(r)...)
			} else {
				out = append(out, r.NextVal())
			}
			continue
		}
		walkCells(a, func(_ string, v interface{}) { out = append(out, v) })
	}
	return out
}

// if(cond, then, else). Both branches are evaluated, as in any function call.
func tblIf(args ...interface{}) (interface{}, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, fmt.Errorf("if expects (condition, then [, else]) got %d arguments", len(args))
	}
	if toBool(args[0]) {
		return argVal(args[1]), nil
	}
	if len(args) == 3 {
		return argVal(args[2]), nil
	}
	return "", nil
}

func tblAnd(args ...interface{}) (interface{}, error) {
	for _, v := range spread(args...) {
		if !toBool(v) {
			return false, nil
		}
	}
	return true, nil
}

func tblOr(args ...interface{}) (interface{}, error) {
	for _, v := range spread(args...) {
		if toBool(v) {
			return true, nil
		}
	}
	return false, nil
}

func tblXor(args ...interface{}) (interface{}, error) {
	n := 0
	for _, v := range spread(args...) {
		if toBool(v) {
			n++
		}
	}
	return n%2 == 1, nil
}

func tblNot(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("not expects one argument")
	}
	return !toBool(args[0]), nil
}

// iferror(value, fallback) replaces a result that is not a number (0/0,
// sqrt(-1)), an infinity (1/0) or nothing at all. A function that fails
// outright still stops the formula: arguments are evaluated before the call.
func tblIfError(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("iferror expects (value, fallback)")
	}
	v := argVal(args[0])
	switch x := v.(type) {
	case nil:
		return argVal(args[1]), nil
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return argVal(args[1]), nil
		}
	}
	return v, nil
}

func tblIsBlank(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("isblank expects one argument")
	}
	if r, ok := args[0].(*RangeIter); ok {
		return r.Next() == "", nil
	}
	return toStr(args[0]) == "", nil
}

func tblIsNumber(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("isnumber expects one argument")
	}
	_, ok := numberOnly(argVal(args[0]))
	return ok, nil
}

func tblIsText(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("istext expects one argument")
	}
	s, ok := argVal(args[0]).(string)
	return ok && s != "", nil
}

func tblIsDate(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("isdate expects one argument")
	}
	_, ok := argVal(args[0]).(time.Time)
	return ok, nil
}

// ---------------------------------------------------------------------------
// Text
// ---------------------------------------------------------------------------

func textArg(name string, args []interface{}, n int) (string, error) {
	if len(args) < n {
		return "", fmt.Errorf("%s expects at least %d arguments got %d", name, n, len(args))
	}
	return toStr(args[0]), nil
}

func tblConcat(args ...interface{}) (interface{}, error) {
	var b strings.Builder
	for _, v := range spread(args...) {
		b.WriteString(toStr(v))
	}
	return b.String(), nil
}

// join(sep, values...) skips empty cells, like TEXTJOIN with ignore_empty.
// Unlike concat, a column reference is the whole column: joining a column
// is what join is for, while concat($1, '-', $2) builds one row's text.
func tblJoin(args ...interface{}) (interface{}, error) {
	sep, err := textArg("join", args, 1)
	if err != nil {
		return nil, err
	}
	parts := []string{}
	for _, v := range argList(args[1:]...) {
		if s := toStr(v); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, sep), nil
}

func tblLen(args ...interface{}) (interface{}, error) {
	s, err := textArg("len", args, 1)
	return len([]rune(s)), err
}

func tblUpper(args ...interface{}) (interface{}, error) {
	s, err := textArg("upper", args, 1)
	return strings.ToUpper(s), err
}

func tblLower(args ...interface{}) (interface{}, error) {
	s, err := textArg("lower", args, 1)
	return strings.ToLower(s), err
}

// trim drops spaces at both ends and squeezes runs inside to one, as TRIM does.
func tblTrim(args ...interface{}) (interface{}, error) {
	s, err := textArg("trim", args, 1)
	return strings.Join(strings.Fields(s), " "), err
}

// countArg is the optional character count of left and right, 1 by default.
func countArg(args []interface{}, i int) int {
	if len(args) > i {
		if n, ok := cellInt(args[i]); ok && n >= 0 {
			return n
		}
		return 0
	}
	return 1
}

func tblLeft(args ...interface{}) (interface{}, error) {
	s, err := textArg("left", args, 1)
	r := []rune(s)
	n := min(countArg(args, 1), len(r))
	return string(r[:n]), err
}

func tblRight(args ...interface{}) (interface{}, error) {
	s, err := textArg("right", args, 1)
	r := []rune(s)
	n := min(countArg(args, 1), len(r))
	return string(r[len(r)-n:]), err
}

// mid(text, start, count): start counts from 1.
func tblMid(args ...interface{}) (interface{}, error) {
	s, err := textArg("mid", args, 3)
	if err != nil {
		return nil, err
	}
	r := []rune(s)
	start, ok1 := cellInt(args[1])
	n, ok2 := cellInt(args[2])
	if !ok1 || !ok2 || start < 1 || n < 0 {
		return nil, fmt.Errorf("mid expects (text, start from 1, count)")
	}
	if start > len(r) {
		return "", nil
	}
	end := min(start-1+n, len(r))
	return string(r[start-1 : end]), nil
}

func tblSubstitute(args ...interface{}) (interface{}, error) {
	s, err := textArg("substitute", args, 3)
	if err != nil {
		return nil, err
	}
	return strings.ReplaceAll(s, toStr(args[1]), toStr(args[2])), nil
}

func tblRept(args ...interface{}) (interface{}, error) {
	s, err := textArg("rept", args, 2)
	if err != nil {
		return nil, err
	}
	n, ok := cellInt(args[1])
	if !ok || n < 0 {
		return nil, fmt.Errorf("rept count must be a number of 0 or more")
	}
	return strings.Repeat(s, n), nil
}

// textTest builds contains / startswith / endswith; they ignore case, as a
// spreadsheet's SEARCH does.
func textTest(name string, test func(s, sub string) bool) func(args ...interface{}) (interface{}, error) {
	return func(args ...interface{}) (interface{}, error) {
		s, err := textArg(name, args, 2)
		if err != nil {
			return nil, err
		}
		return test(strings.ToLower(s), strings.ToLower(toStr(args[1]))), nil
	}
}

var (
	tblContains   = textTest("contains", strings.Contains)
	tblStartsWith = textTest("startswith", strings.HasPrefix)
	tblEndsWith   = textTest("endswith", strings.HasSuffix)
)

var firstNumber = regexp.MustCompile(`[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?`)

// value(text) reads the first number out of text ('$1,234.50', '12 kg'),
// thousands separators allowed; 0 when there is none.
func tblValue(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("value expects one argument")
	}
	s := strings.ReplaceAll(toStr(args[0]), ",", "")
	if f, ok := cellFloat(firstNumber.FindString(s)); ok {
		return f, nil
	}
	return 0.0, nil
}

// fmt(pattern, values...) is printf. Cells are numbers as float64, so an
// integer verb (%d %x %o %b %c) is handed an integer.
func tblFmt(args ...interface{}) (interface{}, error) {
	pattern, err := textArg("fmt", args, 1)
	if err != nil {
		return nil, err
	}
	verbs := fspec.FindAllString(pattern, -1)
	vals := []interface{}{}
	for i, a := range args[1:] {
		v := argVal(a)
		if i < len(verbs) && strings.ContainsAny(verbs[i][len(verbs[i])-1:], "dxobc") {
			if f, ok := numberOnly(v); ok {
				v = int64(f)
			}
		}
		if t, ok := v.(time.Time); ok {
			v = orgTimestamp(t)
		}
		// %s of a number would print %!s(float64=3).
		if i < len(verbs) && strings.HasSuffix(verbs[i], "s") {
			v = toStr(v)
		}
		vals = append(vals, v)
	}
	return fmt.Sprintf(pattern, vals...), nil
}

// ---------------------------------------------------------------------------
// Rounding and numbers
// ---------------------------------------------------------------------------

// roundTo rounds x at the given number of decimal places with f.
func roundPlaces(name string, f func(float64) float64, args []interface{}) (interface{}, error) {
	x, ok := cellFloat(args[0])
	if !ok {
		return nil, fmt.Errorf("%s: value is not a number", name)
	}
	d := 0
	if len(args) > 1 {
		if d, ok = cellInt(args[1]); !ok {
			return nil, fmt.Errorf("%s: places is not a number", name)
		}
	}
	p := math.Pow(10, float64(d))
	return f(x*p) / p, nil
}

// round(x [, places]). With one argument it keeps working over lists.
func tblRound(args ...interface{}) (interface{}, error) {
	if len(args) == 2 {
		return roundPlaces("round", math.Round, args)
	}
	return doN(math.Round, args...)
}

// roundup rounds away from zero, rounddown toward it.
func tblRoundUp(args ...interface{}) (interface{}, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, fmt.Errorf("roundup expects (value [, places])")
	}
	return roundPlaces("roundup", func(v float64) float64 {
		if v < 0 {
			return math.Floor(v)
		}
		return math.Ceil(v)
	}, args)
}

func tblRoundDown(args ...interface{}) (interface{}, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, fmt.Errorf("rounddown expects (value [, places])")
	}
	return roundPlaces("rounddown", math.Trunc, args)
}

func tblSign(args ...interface{}) (interface{}, error) {
	return doN(func(v float64) float64 {
		if v > 0 {
			return 1
		}
		if v < 0 {
			return -1
		}
		return 0
	}, args...)
}

func tblClamp(args ...interface{}) (interface{}, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("clamp expects (value, low, high)")
	}
	x, ok1 := cellFloat(args[0])
	lo, ok2 := cellFloat(args[1])
	hi, ok3 := cellFloat(args[2])
	if !ok1 || !ok2 || !ok3 {
		return nil, fmt.Errorf("clamp expects numbers")
	}
	return math.Max(lo, math.Min(hi, x)), nil
}

// randint(low, high) is a whole number from low to high, both included.
func tblRandInt(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("randint expects (low, high)")
	}
	lo, ok1 := cellInt(args[0])
	hi, ok2 := cellInt(args[1])
	if !ok1 || !ok2 || hi < lo {
		return nil, fmt.Errorf("randint expects numbers with low <= high")
	}
	return lo + rand.Intn(hi-lo+1), nil
}
