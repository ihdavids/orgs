package orgs

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// Argument conventions shared by the functions in the tablefuncs_*.go files.
//
// A range argument (the v* functions, lookups, criteria ranges) is expanded in
// full: a column reference like $2 means the whole column, as it does for vsum.
// A value argument reads one cell: $2 means the cell in the current row, as it
// does for sqrt.
//
// govaluate splats a lone list argument into the argument list and appends the
// arguments after a list onto it, so a function returning a list (sort, rdup,
// lookupall) can only be the last argument of another function.

// defaultCalc reads cells of a range that came without a calc state (remote).
var defaultCalc = &CalcState{SkipHeader: true}

func rangeMode(r *RangeIter) *CalcState {
	if r.Mode != nil {
		return r.Mode
	}
	return defaultCalc
}

// argVal reads a value argument: a cell reference yields the current cell.
func argVal(a interface{}) interface{} {
	if r, ok := a.(*RangeIter); ok {
		return r.NextVal()
	}
	return a
}

// walkCells visits every cell of a range argument, empty ones included, so
// two ranges walked side by side stay aligned. Only a header row is skipped.
// Anything that is not a range is visited as itself, lists element by element.
func walkCells(a interface{}, fn func(raw string, v interface{})) {
	switch v := a.(type) {
	case *RangeIter:
		mode := rangeMode(v)
		v.Reset()
		for ref := v.It(); ref != nil; ref = v.It() {
			if mode.SkipHeader && TableHasHeader(v.Tbl) && ref.Row == 1 {
				continue
			}
			// A column runs one row past the end of a table with separators,
			// and a name or parameter row is not data.
			if !isDataRow(v.Tbl, ref) {
				continue
			}
			raw := strings.TrimSpace(v.Tbl.GetValRef(ref))
			fn(raw, mode.ProcessCellVal(raw))
		}
	case []interface{}:
		for _, x := range v {
			walkCells(x, fn)
		}
	case []float64:
		for _, x := range v {
			fn(toStr(x), x)
		}
	case []int:
		for _, x := range v {
			fn(toStr(x), x)
		}
	case []string:
		for _, x := range v {
			fn(x, x)
		}
	case []bool:
		for _, x := range v {
			fn(toStr(x), x)
		}
	case nil:
	default:
		fn(toStr(v), v)
	}
}

func isDataRow(tbl *org.Table, ref *org.RowColRef) bool {
	row := ref.Row
	if ref.RelativeRow {
		row += tbl.Cur.Row
	}
	real, _ := tbl.GetRealRowCol(row, 1)
	return real >= 0 && real < len(tbl.Rows) && !ShouldSkipAdvancedRow(tbl.Rows[real].IsAdvanced)
}

// argCells expands range arguments keeping empty cells, for aligned walks.
func argCells(args ...interface{}) []interface{} {
	out := []interface{}{}
	for _, a := range args {
		walkCells(a, func(_ string, v interface{}) { out = append(out, v) })
	}
	return out
}

// argList expands arguments the way the existing v* functions do: empty cells
// are dropped unless the formula asks for them with E.
func argList(args ...interface{}) []interface{} {
	out := []interface{}{}
	for _, a := range args {
		if r, ok := a.(*RangeIter); ok {
			mode := rangeMode(r)
			r.Reset()
			l, _ := makelistfromrange(r.Tbl, mode, r.It)
			out = append(out, l.([]interface{})...)
			continue
		}
		walkCells(a, func(_ string, v interface{}) { out = append(out, v) })
	}
	return out
}

// numbers is every numeric value in the arguments; text, dates and blanks are
// ignored, as a spreadsheet ignores them in a range.
func numbers(args ...interface{}) []float64 {
	out := []float64{}
	for _, v := range argList(args...) {
		switch x := v.(type) {
		case float64:
			out = append(out, x)
		case int:
			out = append(out, float64(x))
		case int64:
			out = append(out, float64(x))
		}
	}
	return out
}

func cellFloat(v interface{}) (float64, bool) {
	switch x := argVal(v).(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(x), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func cellInt(v interface{}) (int, bool) {
	f, ok := cellFloat(v)
	return int(f), ok
}

func toBool(v interface{}) bool {
	switch x := argVal(v).(type) {
	case bool:
		return x
	case float64:
		return x != 0
	case int:
		return x != 0
	case string:
		return isTrue(x)
	}
	return false
}

// orgTimestamp writes a date the way org does: no time when it is midnight.
func orgTimestamp(t time.Time) string {
	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
		return t.Format("<2006-01-02 Mon>")
	}
	return t.Format("<2006-01-02 Mon 15:04>")
}

func toStr(v interface{}) string {
	switch x := argVal(v).(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case bool:
		return strconv.FormatBool(x)
	case time.Time:
		return orgTimestamp(x)
	case common.OrgDuration:
		return x.ToString()
	}
	return fmt.Sprint(v)
}

func toTime(v interface{}) (time.Time, bool) {
	switch x := argVal(v).(type) {
	case time.Time:
		return x, true
	case string:
		if t, err := common.ParseDateString(strings.TrimSpace(x)); err == nil {
			return t, true
		}
	case int:
		if x > 0 {
			return time.Unix(int64(x), 0), true
		}
	case float64:
		// govaluate turns a date literal ('2026-03-01') into Unix seconds.
		if x > 0 {
			return time.Unix(int64(x), 0), true
		}
	}
	return time.Time{}, false
}

// compareVals orders two cell values: numbers by value, dates by time,
// durations by length, booleans only for equality, anything else as text
// without regard to case. ok is false when the two cannot be ordered.
func compareVals(a, b interface{}) (int, bool) {
	if fa, ok := numberOnly(a); ok {
		if fb, ok := numberOnly(b); ok {
			return cmpFloat(fa, fb), true
		}
		return 0, false
	}
	switch x := a.(type) {
	case time.Time:
		if y, ok := b.(time.Time); ok {
			return x.Compare(y), true
		}
		return 0, false
	case common.OrgDuration:
		if y, ok := b.(common.OrgDuration); ok {
			return cmpFloat(x.Mins, y.Mins), true
		}
		return 0, false
	case bool:
		if y, ok := b.(bool); ok && x == y {
			return 0, true
		}
		return 1, false
	}
	if _, ok := numberOnly(b); ok {
		return 0, false
	}
	return strings.Compare(strings.ToLower(toStr(a)), strings.ToLower(toStr(b))), true
}

func numberOnly(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

func cmpFloat(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func valsEqual(a, b interface{}) bool {
	c, ok := compareVals(a, b)
	return ok && c == 0
}

var criterionOp = regexp.MustCompile(`^\s*(<>|!=|>=|<=|=|>|<)?(.*)$`)

// parseCriterion turns a criteria argument into a test on one cell.
// A string may start with an operator: '>10', '<=2026-01-01', '<>done', '=';
// without one it means equal to. * and ? are wildcards in a text comparison.
// An empty criterion matches empty cells. Anything else must be equal.
func parseCriterion(c interface{}) func(v interface{}) bool {
	c = argVal(c)
	s, isStr := c.(string)
	if !isStr {
		return func(v interface{}) bool { return valsEqual(v, c) }
	}
	m := criterionOp.FindStringSubmatch(s)
	op, text := m[1], strings.TrimSpace(m[2])
	if op == "!=" {
		op = "<>"
	}
	if text == "" {
		blank := func(v interface{}) bool { return toStr(v) == "" }
		if op == "<>" {
			return func(v interface{}) bool { return !blank(v) }
		}
		return blank
	}
	want := (&CalcState{}).ProcessCellVal(text)
	if ws, ok := want.(string); ok && (op == "" || op == "=" || op == "<>") && strings.ContainsAny(ws, "*?") {
		re := globRegexp(ws)
		if op == "<>" {
			return func(v interface{}) bool { return !re.MatchString(toStr(v)) }
		}
		return func(v interface{}) bool { return re.MatchString(toStr(v)) }
	}
	return func(v interface{}) bool {
		c, ok := compareVals(v, want)
		switch op {
		case "", "=":
			return ok && c == 0
		case "<>":
			return !ok || c != 0
		case ">":
			return ok && c > 0
		case ">=":
			return ok && c >= 0
		case "<":
			return ok && c < 0
		case "<=":
			return ok && c <= 0
		}
		return false
	}
}

func globRegexp(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("(?is)^")
	for _, r := range glob {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// ---------------------------------------------------------------------------
// Conditional aggregates
// ---------------------------------------------------------------------------

// ifValues is the values of rng (or of valrng beside it) whose cell in rng
// meets the criterion: the shape of sumif(range, criteria [, sumrange]).
func ifValues(name string, args []interface{}) ([]interface{}, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, fmt.Errorf("%s expects (range, criteria [, values]) got %d arguments", name, len(args))
	}
	keys := argCells(args[0])
	vals := keys
	if len(args) == 3 {
		vals = argCells(args[2])
	}
	test := parseCriterion(args[1])
	out := []interface{}{}
	for i, k := range keys {
		if i < len(vals) && test(k) {
			out = append(out, vals[i])
		}
	}
	return out, nil
}

// ifsValues is the shape of sumifs(values, range1, criteria1, range2, criteria2, ...):
// the values whose row meets every criterion.
func ifsValues(name string, vals []interface{}, pairs []interface{}) ([]interface{}, error) {
	if len(pairs) == 0 || len(pairs)%2 != 0 {
		return nil, fmt.Errorf("%s expects range, criteria pairs", name)
	}
	keep := make([]bool, len(vals))
	for i := range keep {
		keep[i] = true
	}
	for p := 0; p < len(pairs); p += 2 {
		keys := argCells(pairs[p])
		test := parseCriterion(pairs[p+1])
		for i := range keep {
			keep[i] = keep[i] && i < len(keys) && test(keys[i])
		}
	}
	out := []interface{}{}
	for i, v := range vals {
		if keep[i] {
			out = append(out, v)
		}
	}
	return out, nil
}

func sumOf(vals []interface{}) float64 {
	acc := 0.0
	for _, v := range vals {
		if f, ok := numberOnly(v); ok {
			acc += f
		}
	}
	return acc
}

func meanOf(vals []interface{}) float64 {
	acc, cnt := 0.0, 0
	for _, v := range vals {
		if f, ok := numberOnly(v); ok {
			acc += f
			cnt++
		}
	}
	if cnt == 0 {
		return 0
	}
	return acc / float64(cnt)
}

// extremeOf is the largest (sign 1) or smallest (sign -1) number, 0 when none.
func extremeOf(vals []interface{}, sign float64) float64 {
	have := false
	m := 0.0
	for _, v := range vals {
		if f, ok := numberOnly(v); ok && (!have || f*sign > m*sign) {
			m, have = f, true
		}
	}
	return m
}

func vsumif(args ...interface{}) (interface{}, error) {
	vals, err := ifValues("vsumif", args)
	return sumOf(vals), err
}

func vcountif(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("vcountif expects (range, criteria) got %d arguments", len(args))
	}
	vals, err := ifValues("vcountif", args)
	return len(vals), err
}

func vmeanif(args ...interface{}) (interface{}, error) {
	vals, err := ifValues("vmeanif", args)
	return meanOf(vals), err
}

func vmaxif(args ...interface{}) (interface{}, error) {
	vals, err := ifValues("vmaxif", args)
	return extremeOf(vals, 1), err
}

func vminif(args ...interface{}) (interface{}, error) {
	vals, err := ifValues("vminif", args)
	return extremeOf(vals, -1), err
}

func vsumifs(args ...interface{}) (interface{}, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("vsumifs expects (values, range, criteria, ...)")
	}
	vals, err := ifsValues("vsumifs", argCells(args[0]), args[1:])
	return sumOf(vals), err
}

func vmeanifs(args ...interface{}) (interface{}, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("vmeanifs expects (values, range, criteria, ...)")
	}
	vals, err := ifsValues("vmeanifs", argCells(args[0]), args[1:])
	return meanOf(vals), err
}

func vcountifs(args ...interface{}) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("vcountifs expects (range, criteria, ...)")
	}
	vals, err := ifsValues("vcountifs", argCells(args[0]), args)
	return len(vals), err
}

func vcounta(args ...interface{}) (interface{}, error) {
	cnt := 0
	for _, a := range args {
		walkCells(a, func(raw string, _ interface{}) {
			if raw != "" {
				cnt++
			}
		})
	}
	return cnt, nil
}

func vcountblank(args ...interface{}) (interface{}, error) {
	cnt := 0
	for _, a := range args {
		walkCells(a, func(raw string, _ interface{}) {
			if raw == "" {
				cnt++
			}
		})
	}
	return cnt, nil
}

// ---------------------------------------------------------------------------
// Lookups
// ---------------------------------------------------------------------------

// lookupArgs reads (value, keys [, results]); results default to the keys,
// as org-lookup-first does with no R-LIST.
func lookupArgs(name string, args []interface{}) (interface{}, []interface{}, []interface{}, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, nil, nil, fmt.Errorf("%s expects (value, range [, results]) got %d arguments", name, len(args))
	}
	want := argVal(args[0])
	keys := argCells(args[1])
	res := keys
	if len(args) == 3 {
		res = argCells(args[2])
	}
	return want, keys, res, nil
}

func tblLookupFirst(args ...interface{}) (interface{}, error) {
	want, keys, res, err := lookupArgs("lookupfirst", args)
	if err != nil {
		return nil, err
	}
	for i, k := range keys {
		if i < len(res) && valsEqual(k, want) {
			return res[i], nil
		}
	}
	return "", nil
}

func tblLookupLast(args ...interface{}) (interface{}, error) {
	want, keys, res, err := lookupArgs("lookuplast", args)
	if err != nil {
		return nil, err
	}
	for i := len(keys) - 1; i >= 0; i-- {
		if i < len(res) && valsEqual(keys[i], want) {
			return res[i], nil
		}
	}
	return "", nil
}

func tblLookupAll(args ...interface{}) (interface{}, error) {
	want, keys, res, err := lookupArgs("lookupall", args)
	if err != nil {
		return nil, err
	}
	out := []interface{}{}
	for i, k := range keys {
		if i < len(res) && valsEqual(k, want) {
			out = append(out, res[i])
		}
	}
	return out, nil
}

// index(range, n) is the nth cell of a range, counting from 1; a negative n
// counts back from the end.
func tblIndex(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("index expects (range, n) got %d arguments", len(args))
	}
	cells := argCells(args[0])
	n, ok := cellInt(args[1])
	if !ok {
		return nil, fmt.Errorf("index position is not a number")
	}
	if n < 0 {
		n = len(cells) + n + 1
	}
	if n < 1 || n > len(cells) {
		return nil, fmt.Errorf("index %d is outside a range of %d cells", n, len(cells))
	}
	return cells[n-1], nil
}

// match(value, range) is the position of the first equal cell, from 1, or 0.
func tblMatch(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("match expects (value, range) got %d arguments", len(args))
	}
	want := argVal(args[0])
	for i, k := range argCells(args[1]) {
		if valsEqual(k, want) {
			return i + 1, nil
		}
	}
	return 0, nil
}

// ---------------------------------------------------------------------------
// Statistics
// ---------------------------------------------------------------------------

func variance(xs []float64, sample bool) float64 {
	n := float64(len(xs))
	if n == 0 || (sample && n < 2) {
		return 0
	}
	mean := 0.0
	for _, x := range xs {
		mean += x
	}
	mean /= n
	ss := 0.0
	for _, x := range xs {
		ss += (x - mean) * (x - mean)
	}
	if sample {
		return ss / (n - 1)
	}
	return ss / n
}

func vvar(args ...interface{}) (interface{}, error) {
	return variance(numbers(args...), true), nil
}

func vpvar(args ...interface{}) (interface{}, error) {
	return variance(numbers(args...), false), nil
}

func vsdev(args ...interface{}) (interface{}, error) {
	return math.Sqrt(variance(numbers(args...), true)), nil
}

func vpsdev(args ...interface{}) (interface{}, error) {
	return math.Sqrt(variance(numbers(args...), false)), nil
}

func vprod(args ...interface{}) (interface{}, error) {
	xs := numbers(args...)
	if len(xs) == 0 {
		return 0.0, nil
	}
	p := 1.0
	for _, x := range xs {
		p *= x
	}
	return p, nil
}

// vmode is the most frequent number; on a tie the one that appears first,
// as MODE does (5 for 5,4,4,5).
func vmode(args ...interface{}) (interface{}, error) {
	xs := numbers(args...)
	counts := map[float64]int{}
	for _, x := range xs {
		counts[x]++
	}
	best, bestN := 0.0, 0
	for _, x := range xs {
		if counts[x] > bestN {
			best, bestN = x, counts[x]
		}
	}
	return best, nil
}

// percentile interpolates between ranks like PERCENTILE.INC; p is a fraction,
// or a percentage when it is over 1.
func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	if p > 1 {
		p /= 100
	}
	p = math.Max(0, math.Min(1, p))
	sort.Float64s(xs)
	rank := p * float64(len(xs)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	return xs[lo] + (rank-float64(lo))*(xs[hi]-xs[lo])
}

// vpercentile(range..., p): the last argument is the percentile.
func vpercentile(args ...interface{}) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("vpercentile expects (range, p)")
	}
	p, ok := cellFloat(args[len(args)-1])
	if !ok {
		return nil, fmt.Errorf("vpercentile: percentile is not a number")
	}
	return percentile(numbers(args[:len(args)-1]...), p), nil
}

// vquartile(range..., q): q is 0 to 4.
func vquartile(args ...interface{}) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("vquartile expects (range, q)")
	}
	q, ok := cellFloat(args[len(args)-1])
	if !ok {
		return nil, fmt.Errorf("vquartile: quartile is not a number")
	}
	return percentile(numbers(args[:len(args)-1]...), q/4), nil
}

// vrank(value, range [, ascending]) is 1 for the largest number, or for the
// smallest when ascending is true.
func vrank(args ...interface{}) (interface{}, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, fmt.Errorf("vrank expects (value, range [, ascending])")
	}
	v, ok := cellFloat(args[0])
	if !ok {
		return nil, fmt.Errorf("vrank: value is not a number")
	}
	asc := len(args) == 3 && toBool(args[2])
	rank := 1
	for _, x := range numbers(args[1]) {
		if (!asc && x > v) || (asc && x < v) {
			rank++
		}
	}
	return rank, nil
}
