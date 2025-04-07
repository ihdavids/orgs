package orgs

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/govaluate"
	"github.com/ihdavids/orgs/internal/common"
)

/* SDOC: Tables
* Table Functions
  
  TODO: Fill in information on table functions
EDOC */

// Additional functions
// head - Get first cell
// tail - Get all but first cell
// eq - Equals?
// lt - Less than?
// find - Find first field which is equal
// / neg - Negate all field values
// mul - Vector multipication
// add - Vector addition
var functions map[string]govaluate.ExpressionFunction = map[string]govaluate.ExpressionFunction{
	"vmean":   vmean,
	"vmedian": vmedian,
	"vsum":    vsum,
	"vmin":    vmin,
	"vmax":    vmax,
	// Math methods
	"floor":     tblFloor,
	"ceil":      tblCeil,
	"round":     tblRound,
	"trunc":     tblTrunc,
	"abs":       tblAbs,
	"degrees":   tblDegrees,
	"radians":   tblRadians,
	"int":       tblInt,
	"float":     tblFloat,
	"bool":      tblBool,
	"sqrt":      tblSqrt,
	"cos":       tblCos,
	"acos":      tblACos,
	"cosh":      tblCosh,
	"acosh":     tblACosh,
	"tan":       tblTan,
	"atan":      tblATan,
	"atanh":     tblATanh,
	"tanh":      tblTanh,
	"sin":       tblSin,
	"asin":      tblASin,
	"asinh":     tblASinh,
	"sinh":      tblSinh,
	"log":       tblLog,
	"log10":     tblLog10,
	"log2":      tblLog2,
	"exp":       tblExp,
	"exp2":      tblExp2,
	"rem":       tblRemainder,
	"pow":       tblPow,
	"mod":       tblMod,
	"pow10":     tblPow10,
	"neg":       tblNeg,
	"rdup":      tblRDup,
	"sort":      tblSort,
	"rsort":     tblRSort,
	"rev":       tblRev,
	"vlen":      vcount,
	"vcount":    vcount,
	"rand":      tblRand,
	"passed":    tblPassed,
	"highlight": tblHighlight, // Only exists for compatibilty with sublime text
	// Date methods
	"now":         tblNow,
	"date":        tblDate,
	"minute":      tblMinute,
	"hour":        tblHour,
	"day":         tblDay,
	"year":        tblYear,
	"month":       tblMonth,
	"monthname":   tblMonthName,
	"weekday":     tblWeekday,
	"weekdayname": tblWeekdayName,
	"yearday":     tblYearday,
	"duration":    tblDuration,
}

// Table cells are expanded from a range iterator in entirety for V* functions
func fullyexpandrange(a interface{}) (interface{}, error) {
	var err error
	if r, ok := a.(*RangeIter); ok {
		r.Reset()
		if a, err = makelistfromrange(r.Tbl, r.Mode, r.It); err != nil {
			return a, err
		}
	}
	return a, nil
}

func vsum(args ...interface{}) (interface{}, error) {
	acc := 0.0
	for _, a := range args {
		var err error
		if a, err = fullyexpandrange(a); err != nil {
			return nil, err
		}
		switch v := a.(type) {
		case int:
			acc += float64(v)
		case float64:
			acc += v
		case []float64:
			for _, val := range v {
				acc += val
			}
		case []interface{}:
			for _, val := range v {
				switch x := val.(type) {
				case int:
					acc += float64(x)
				case float64:
					acc += x
				}
			}

		}
	}
	return acc, nil
}

func vmedian(args ...interface{}) (interface{}, error) {
	acc := []float64{}
	for _, a := range args {
		var err error
		if a, err = fullyexpandrange(a); err != nil {
			return nil, err
		}
		switch v := a.(type) {
		case int:
			acc = append(acc, float64(v))
		case float64:
			acc = append(acc, float64(v))
		case []float64:
			for _, val := range v {
				acc = append(acc, float64(val))
			}
		case []interface{}:
			for _, val := range v {
				switch x := val.(type) {
				case int:
					acc = append(acc, float64(x))
				case float64:
					acc = append(acc, x)
				}
			}
		}
	}
	l := len(acc)
	if l <= 0 {
		return 0, nil
	} else if l == 1 {
		return acc[0], nil
	} else {
		sort.Float64s(acc)
		l2 := int(math.Floor(float64(l) / 2.0))
		if l2*2 != l {
			// 0 index makes this the actual value 3/2=1 which is actually index 2
			return acc[l2], nil
		} else {
			// 0 index makes this 1 & 2 for 4 which is actually 2 + 3
			return (acc[l2] + acc[l2-1]) / 2.0, nil
		}
	}
}

func vcount(args ...interface{}) (interface{}, error) {
	cnt := 0
	for _, a := range args {
		var err error
		if a, err = fullyexpandrange(a); err != nil {
			return nil, err
		}
		switch v := a.(type) {
		case int:
			cnt += 1
		case float64:
			cnt += 1
		case []float64:
			cnt += len(v)
		case []int:
			cnt += len(v)
		case []string:
			cnt += len(v)
		case []interface{}:
			cnt += len(v)
		}
	}
	return cnt, nil
}

func vmean(args ...interface{}) (interface{}, error) {
	acc := 0.0
	cnt := 0
	for _, a := range args {
		var err error
		if a, err = fullyexpandrange(a); err != nil {
			return nil, err
		}
		switch v := a.(type) {
		case int:
			acc += float64(v)
			cnt += 1
		case float64:
			acc += v
			cnt += 1
		case []float64:
			for _, val := range v {
				acc += val
				cnt += 1
			}
		case []interface{}:
			for _, val := range v {
				switch x := val.(type) {
				case int:
					acc += float64(x)
					cnt += 1
				case float64:
					acc += x
					cnt += 1
				}
			}
		}
	}
	if cnt != 0 {
		res := (acc / float64(cnt))
		return res, nil
	}
	return 0, nil
}

func vmax(args ...interface{}) (interface{}, error) {
	m := -1.7e+308
	for _, a := range args {
		var err error
		if a, err = fullyexpandrange(a); err != nil {
			return nil, err
		}
		switch v := a.(type) {
		case int:
			m = max(m, float64(v))
		case float64:
			m = max(m, v)
		case []float64:
			for _, val := range v {
				m = max(m, val)
			}
		case []interface{}:
			for _, val := range v {
				switch x := val.(type) {
				case int:
					m = max(m, float64(x))
				case float64:
					m = max(m, x)
				}
			}
		}
	}
	return m, nil
}

func vmin(args ...interface{}) (interface{}, error) {
	m := 1.7e+308
	for _, a := range args {
		var err error
		if a, err = fullyexpandrange(a); err != nil {
			return nil, err
		}
		switch v := a.(type) {
		case int:
			m = min(m, float64(v))
		case float64:
			m = min(m, v)
		case []float64:
			for _, val := range v {
				m = min(m, val)
			}
		case []interface{}:
			for _, val := range v {
				switch x := val.(type) {
				case int:
					m = min(m, float64(x))
				case float64:
					m = min(m, x)
				}
			}
		}
	}
	return m, nil
}

type Op func(v float64) float64
type Number interface {
	int | int64 | float64
}
type OpN[T Number, R Number] func(v T) R

func doN[T Number, R Number](f OpN[T, R], args ...interface{}) (interface{}, error) {
	var m R = 0
	for _, a := range args {

		// Table cells are expanded from a range iterator one at a time for doN functions
		if r, ok := a.(*RangeIter); ok {
			a = r.NextVal()
		}

		switch v := a.(type) {
		case int:
			m = f(T(v))
		case float64:
			m = f(T(v))
		case []float64:
			ret := []R{}
			for _, val := range v {
				ret = append(ret, f(T(val)))
			}
			return ret, nil
		case []int:
			ret := []R{}
			for _, val := range v {
				ret = append(ret, f(T(val)))
			}
			return ret, nil
		}
	}
	return m, nil
}

type Op2N[T Number, R Number] func(a T, b T) R

func do2N[T Number, R Number](f Op2N[T, R], args ...interface{}) (interface{}, error) {
	params := []T{}
	for _, a := range args {

		// Table cells are expanded from a range iterator one at a time for doN functions
		if r, ok := a.(*RangeIter); ok {
			a = r.NextVal()
		}

		switch v := a.(type) {
		case int:
			params = append(params, T(v))
		case float64:
			params = append(params, T(v))
		case []float64:
			for _, val := range v {
				params = append(params, T(val))
			}
		case []int:
			for _, val := range v {
				params = append(params, T(val))
			}
		}
	}
	if len(params) == 2 {
		m := f(params[0], params[1])
		return m, nil
	}
	return 0, fmt.Errorf("Wrong number of parameters for function [%s] expected [2] got [%d]", common.GetFunctionName(f), len(params))
}

func tblSqrt(args ...interface{}) (interface{}, error) {
	return doN(math.Sqrt, args...)
}

func tblFloor(args ...interface{}) (interface{}, error) {
	return doN(math.Floor, args...)
}

func tblCeil(args ...interface{}) (interface{}, error) {
	return doN(math.Ceil, args...)
}

func tblAbs(args ...interface{}) (interface{}, error) {
	return doN(math.Abs, args...)
}

func tblDegrees(args ...interface{}) (interface{}, error) {
	pi := 3.1415926535897932384626433832795
	return doN(func(radians float64) float64 { return radians * (180.0 / pi) }, args...)
}

func tblRadians(args ...interface{}) (interface{}, error) {
	pi := 3.1415926535897932384626433832795
	return doN(func(degrees float64) float64 { return degrees * (pi / 180.0) }, args...)
}

func tblSin(args ...interface{}) (interface{}, error) {
	return doN(math.Sin, args...)
}

func tblASin(args ...interface{}) (interface{}, error) {
	return doN(math.Asin, args...)
}

func tblASinh(args ...interface{}) (interface{}, error) {
	return doN(math.Asinh, args...)
}

func tblSinh(args ...interface{}) (interface{}, error) {
	return doN(math.Sinh, args...)
}

func tblCos(args ...interface{}) (interface{}, error) {
	return doN(math.Cos, args...)
}

func tblACos(args ...interface{}) (interface{}, error) {
	return doN(math.Acos, args...)
}

func tblACosh(args ...interface{}) (interface{}, error) {
	return doN(math.Acosh, args...)
}

func tblCosh(args ...interface{}) (interface{}, error) {
	return doN(math.Cosh, args...)
}

func tblTan(args ...interface{}) (interface{}, error) {
	return doN(math.Tan, args...)
}

func tblATan(args ...interface{}) (interface{}, error) {
	return doN(math.Atan, args...)
}

func tblATanh(args ...interface{}) (interface{}, error) {
	return doN(math.Atanh, args...)
}

func tblTanh(args ...interface{}) (interface{}, error) {
	return doN(math.Tanh, args...)
}

func tblLog(args ...interface{}) (interface{}, error) {
	return doN(math.Log, args...)
}

func tblLog10(args ...interface{}) (interface{}, error) {
	return doN(math.Log10, args...)
}

func tblLog2(args ...interface{}) (interface{}, error) {
	return doN(math.Log2, args...)
}

func tblExp(args ...interface{}) (interface{}, error) {
	return doN(math.Exp, args...)
}

func tblExp2(args ...interface{}) (interface{}, error) {
	return doN(math.Exp2, args...)
}

/*
func tblModf(args ...interface{}) (interface{}, error) {
	return do2F(math.Modf, args)
}
*/

func tblMod(args ...interface{}) (interface{}, error) {
	return do2N(math.Mod, args...)
}

func tblPow(args ...interface{}) (interface{}, error) {
	return do2N(math.Pow, args...)
}

func tblPow10(args ...interface{}) (interface{}, error) {
	return doN(math.Pow10, args...)
}

func tblRemainder(args ...interface{}) (interface{}, error) {
	return do2N(math.Remainder, args...)
}

func tblRound(args ...interface{}) (interface{}, error) {
	return doN(math.Round, args...)
}

func tblTrunc(args ...interface{}) (interface{}, error) {
	return doN(math.Trunc, args...)
}

func tblInt(args ...interface{}) (interface{}, error) {
	for _, a := range args {
		// Table cells are expanded from a range iterator
		if r, ok := a.(*RangeIter); ok {
			a = r.NextVal()
		}
		switch v := a.(type) {
		case int:
			return v, nil
		case float64:
			return int(v), nil
		case []float64:
			ret := []int{}
			for _, val := range v {
				ret = append(ret, int(val))
			}
			return ret, nil
		case []int:
			return v, nil
		}
	}
	return 0, nil
}

func tblFloat(args ...interface{}) (interface{}, error) {
	for _, a := range args {
		// Table cells are expanded from a range iterator
		if r, ok := a.(*RangeIter); ok {
			a = r.NextVal()
		}
		switch v := a.(type) {
		case int:
			return float64(v), nil
		case float64:
			return v, nil
		case []int:
			ret := []float64{}
			for _, val := range v {
				ret = append(ret, float64(val))
			}
			return ret, nil
		case []float64:
			return v, nil
		}
	}
	return 0.0, nil
}

func isTrue(n string) bool {
	return n == "T" || n == "True" || n == "true" || n == "t" || n == "TRUE"
}

func isFalse(n string) bool {
	return n == "F" || n == "False" || n == "false" || n == "f" || n == "FALSE"
}

func tblBool(args ...interface{}) (interface{}, error) {
	for _, a := range args {
		// Table cells are expanded from a range iterator
		if r, ok := a.(*RangeIter); ok {
			a = r.NextVal()
		}
		switch v := a.(type) {
		case int:
			return v > 0, nil
		case float64:
			return v > 0, nil
		case bool:
			return v, nil
		case string:
			return isTrue(v), nil
		case []string:
			ok := true
			for _, val := range v {
				ok = ok && isTrue(val)
			}
			return ok, nil
		case []int:
			ok := true
			for _, val := range v {
				ok = ok && val > 0
			}
			return ok, nil
		case []bool:
			ok := true
			for _, val := range v {
				ok = ok && val
			}
			return ok, nil
		case []float64:
			ok := true
			for _, val := range v {
				ok = ok && val > 0
			}
			return ok, nil
		}
	}
	return 0.0, nil
}

type SliceType interface {
	~string | ~int | ~float64 // add more *comparable* types as needed
}

/*
	func removeDuplicates[T SliceType](s []T) []T {
		if len(s) < 1 {
			return s
		}
		// stable sort keeps things in order for us
		sort.SliceStable(s, func(i, j int) bool {
			return s[i] < s[j]
		})
		prev := 1
		for curr := 1; curr < len(s); curr++ {
			if s[curr-1] != s[curr] {
				s[prev] = s[curr]
				prev++
			}
		}
		return s[:prev]
	}
*/
/*
func removeDuplicate[T SliceType](sliceList []T) []T {
	allKeys := make(map[T]bool)
	list := []T{}
	for _, item := range sliceList {
		if _, value := allKeys[item]; !value {
			allKeys[item] = true
			list = append(list, item)
		}
	}
	return list
}
*/
func removeDuplicate(sliceList []interface{}) []interface{} {
	allKeys := make(map[interface{}]bool)
	list := []interface{}{}
	for _, item := range sliceList {
		if _, value := allKeys[item]; !value {
			allKeys[item] = true
			list = append(list, item)
		}
	}
	return list
}

func tblBuildList(args ...interface{}) []interface{} {
	params := []interface{}{}
	for _, a := range args {
		switch v := a.(type) {
		case int:
			params = append(params, v)
		case float64:
			params = append(params, v)
		case string:
			params = append(params, v)
		case []float64:
			for _, val := range v {
				params = append(params, val)
			}
		case []int:
			for _, val := range v {
				params = append(params, val)
			}
		case []string:
			for _, val := range v {
				params = append(params, val)
			}
		}
	}
	return params
}

func tblRDup(args ...interface{}) (interface{}, error) {
	params := tblBuildList(args)
	if len(params) > 0 {
		m := removeDuplicate(params)
		return m, nil
	}
	return params, nil
}

func tblSort(args ...interface{}) (interface{}, error) {
	params := tblBuildList(args)
	if len(params) < 1 {
		return params, nil
	}
	sort.SliceStable(params, func(i, j int) bool {
		switch a := params[i].(type) {
		case float64:
			switch b := params[j].(type) {
			case float64:
				return a < b
			case int:
				return a < float64(b)
			case string:
				return false
			}
			return false
		case int:
			switch b := params[j].(type) {
			case float64:
				return float64(a) < b
			case int:
				return a < b
			case string:
				return false
			}
			return false
		case string:
			switch b := params[j].(type) {
			case float64:
				return true
			case int:
				return true
			case string:
				return a < b
			}
			return true
		}
		return false
	})
	return params, nil
}

func tblRSort(args ...interface{}) (interface{}, error) {
	params := tblBuildList(args)
	if len(params) < 1 {
		return params, nil
	}
	sort.SliceStable(params, func(i, j int) bool {
		switch a := params[i].(type) {
		case float64:
			switch b := params[j].(type) {
			case float64:
				return a > b
			case int:
				return a > float64(b)
			case string:
				return true
			}
			return true
		case int:
			switch b := params[j].(type) {
			case float64:
				return float64(a) > b
			case int:
				return a > b
			case string:
				return true
			}
			return true
		case string:
			switch b := params[j].(type) {
			case float64:
				return false
			case int:
				return false
			case string:
				return a > b
			}
			return false
		}
		return false
	})
	return params, nil
}

func ReverseSlice[T any](s []T) {
	sort.SliceStable(s, func(i, j int) bool {
		return i > j
	})
}

func tblRev(args ...interface{}) (interface{}, error) {
	params := tblBuildList(args)
	if len(params) < 1 {
		return params, nil
	}
	ReverseSlice(params)
	return params, nil
}

func tblNeg(args ...interface{}) (interface{}, error) {
	return doN(func(v float64) float64 {
		return -v
	}, args)
}

func tblRemote(fname string, args ...interface{}) (interface{}, error) {
	if len(args) != 5 {
		return nil, fmt.Errorf("remote only accepts 5 arguments %d given", len(args))
	}
	name := args[0].(string)
	tbl := GetDb().GetNamedTable(name, fname)
	if tbl != nil {
		// We return a range iterator for remote
		rng := org.MakeFormulaTarget(args[1].(string), args[2].(string), args[3].(string), args[4].(string), tbl.Table)
		it := &RangeIter{}
		it.Form = *rng
		it.Tbl = tbl.Table
		it.Reset()
		return it, nil
	} else {
		return nil, fmt.Errorf("cannot find table named: [%s]", name)
	}
}

func tblPassed(args ...interface{}) (interface{}, error) {
	if len(args) == 1 {
		p := args[0]
		// fmt.Printf("PASSED: 1 Argument: [%v]\n", p)
		if t, ok := p.(*RangeIter); ok {
			p = t.NextVal()
		}
		switch n := p.(type) {
		case float64:
			if n > 0 {
				return "PASSED", nil
			}
		case bool:
			if n {
				return "PASSED", nil
			}
		case int:
			if n > 0 {
				return "PASSED", nil
			}
		case string:
			if isTrue(n) {
				return "PASSED", nil
			}
		}
	}
	if len(args) == 2 {
		p1 := args[0]
		p2 := args[0]
		//fmt.Printf("PASSED: 2 Argument: [%v] vs [%v]\n", p1, p2)
		if t, ok := p1.(*RangeIter); ok {
			p1 = t.NextVal()
		}
		if t, ok := p2.(*RangeIter); ok {
			p2 = t.NextVal()
		}
		switch n1 := p1.(type) {
		case float64:
			switch n2 := p2.(type) {
			case float64:
				if n1 == n2 {
					return "PASSED", nil
				}
			case int:
				if n1 == float64(n2) {
					return "PASSED", nil
				}
			}
		case bool:
			if n2, ok := p2.(bool); ok {
				if n1 == n2 {
					return "PASSED", nil
				}
			}
		case int:
			switch n2 := p2.(type) {
			case float64:
				if float64(n1) == n2 {
					return "PASSED", nil
				}
			case int:
				if n1 == n2 {
					return "PASSED", nil
				}
			}
		case string:
			if n2, ok := p2.(string); ok {
				if n1 == n2 {
					return "PASSED", nil
				}
			}
		case common.OrgDuration:
			if n2, ok := p2.(common.OrgDuration); ok {
				if n1.Mins == n2.Mins {
					return "PASSED", nil
				}
			}
		case time.Time:
			if n2, ok := p2.(time.Time); ok {
				if n1 == n2 {
					return "PASSED", nil
				}
			}
		case time.Month:
			if n2, ok := p2.(time.Month); ok {
				if n1 == n2 {
					return "PASSED", nil
				}
			}
		case time.Weekday:
			if n2, ok := p2.(time.Weekday); ok {
				if n1 == n2 {
					return "PASSED", nil
				}
			}
		}

	}
	return "FAILED", nil
}

func tblRand(args ...interface{}) (interface{}, error) {
	return rand.Float64(), nil
}

// This does nothing at the moment!
// It exists for compatibility with sublime text at the moment.
// Some day we might have it export ascii character codes for console highlight
// or something.
func tblHighlight(args ...interface{}) (interface{}, error) {
	if len(args) >= 3 {
		return args[2], nil
	}
	return "", nil
}

func tblNow(args ...interface{}) (interface{}, error) {
	return time.Now(), nil
}

func tblDate(args ...interface{}) (interface{}, error) {
	if len(args) == 1 {
		p := args[0]
		if t, ok := p.(*RangeIter); ok {
			p = t.NextVal()
		}
		switch n := p.(type) {
		case int:
			if n > 0 {
				tm := time.Unix(int64(n), 0)
				return tm, nil
			}
		case string:
			return common.ParseDateString(n)
		case time.Time:
			return n, nil
		// Should this be now + duration?
		case common.OrgDuration:
			return n, nil
		}
	}
	return nil, fmt.Errorf("failed to parse date from cell")
}

type TimeFun func(time.Time) (interface{}, error)

func timeOp(name string, fun TimeFun, args ...interface{}) (interface{}, error) {
	if len(args) == 1 {
		p := args[0]
		if t, ok := p.(*RangeIter); ok {
			p = t.NextVal()
		}
		switch n := p.(type) {
		case int:
			if n > 0 {
				return fun(time.Unix(int64(n), 0))
			}
		case string:
			if tm, err := common.ParseDateString(n); err == nil {
				return fun(tm)
			}
		case time.Time:
			return fun(n)
		}
	}
	return nil, fmt.Errorf("failed to parse %s from cell", name)
}

func tblHour(args ...interface{}) (interface{}, error) {
	return timeOp("hour", func(tm time.Time) (interface{}, error) {
		return tm.Hour(), nil
	}, args...)
}

func tblMinute(args ...interface{}) (interface{}, error) {
	return timeOp("minute", func(tm time.Time) (interface{}, error) {
		return tm.Minute(), nil
	}, args...)
}

func tblDay(args ...interface{}) (interface{}, error) {
	return timeOp("day", func(tm time.Time) (interface{}, error) {
		return tm.Day(), nil
	}, args...)
}

func tblYear(args ...interface{}) (interface{}, error) {
	return timeOp("year", func(tm time.Time) (interface{}, error) {
		return tm.Year(), nil
	}, args...)
}

func tblMonth(args ...interface{}) (interface{}, error) {
	return timeOp("month", func(tm time.Time) (interface{}, error) {
		return int(tm.Month()), nil
	}, args...)
}

func tblMonthName(args ...interface{}) (interface{}, error) {
	return timeOp("monthname", func(tm time.Time) (interface{}, error) {
		return tm.Month(), nil
	}, args...)
}

func tblWeekday(args ...interface{}) (interface{}, error) {
	return timeOp("weekday", func(tm time.Time) (interface{}, error) {
		return int(tm.Weekday()), nil
	}, args...)
}

func tblWeekdayName(args ...interface{}) (interface{}, error) {
	return timeOp("weekdayname", func(tm time.Time) (interface{}, error) {
		return tm.Weekday(), nil
	}, args...)
}

func tblYearday(args ...interface{}) (interface{}, error) {
	return timeOp("yearday", func(tm time.Time) (interface{}, error) {
		return tm.YearDay(), nil
	}, args...)
}

func tblDuration(args ...interface{}) (interface{}, error) {
	if len(args) == 1 {
		p := args[0]
		if t, ok := p.(*RangeIter); ok {
			p = t.NextVal()
		}
		switch n := p.(type) {
		case float64:
			if n > 0 {
				return common.NewDuration(n), nil
			}
		case string:
			d := common.ParseDuration(n)
			if d != nil {
				return *d, nil
			}
		case common.OrgDuration:
			return n, nil
		}
	}
	return nil, fmt.Errorf("failed to parse duration from cell")
}
