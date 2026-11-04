package orgs

// Diary sexp dates (org manual 8.1): <%%(diary-float t 4 2)> and kin in a
// heading's own text land on every day the expression is true for, as org's
// agenda evaluates them. go-org does not parse these stamps, so they are read
// off the file's lines (sexpIndex) and evaluated here, without Emacs:
//
//	(diary-date M D Y)             a day; t for any part, a list for several
//	(diary-float M DAYNAME N [D])  the Nth DAYNAME (0 Sunday) of month M, -1 the last
//	(diary-anniversary M D Y)      every year from Y; %d in the title is the count
//	(diary-block M1 D1 Y1 M2 D2 Y2)  every day between, inclusive
//	(diary-cyclic N M D Y)         every N days from that day
//	(diary-remind SEXP N)          SEXP's days and the N before each
//	(org-anniversary Y M D) (org-block Y1 M1 D1 Y2 M2 D2) (org-cyclic N Y M D)
//	(org-date Y M D)               the same in ISO order, as org writes them
//	(org-class Y1 M1 D1 Y2 M2 D2 DAYNAME [SKIP-WEEK...])  that weekday between
//	(memq (calendar-day-of-week date) '(1 2 3 4 5))  weekdays; (= ... 3) one
//	(and ...) (or ...) (not ...)
//
// diary-* take their dates in agenda.dateStyle order: american (M D Y, Emacs's
// default), european (D M Y) or iso (Y M D). An expression orgs cannot read
// lands on no day and is said once on stderr.

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// ---------------------------------------------------------------------------
// Reading an s-expression
// ---------------------------------------------------------------------------

type sexp struct {
	list  []sexp
	isLst bool
	atom  string
}

func readSexp(s string) (sexp, error) {
	toks := []string{}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n':
			i++
		case c == '(' || c == ')' || c == '\'':
			toks = append(toks, string(c))
			i++
		case c == '"':
			j := strings.IndexByte(s[i+1:], '"')
			if j < 0 {
				return sexp{}, fmt.Errorf("unclosed string")
			}
			toks = append(toks, s[i:i+j+2])
			i += j + 2
		default:
			j := i
			for j < len(s) && !strings.ContainsRune(" \t\n()'", rune(s[j])) {
				j++
			}
			toks = append(toks, s[i:j])
			i = j
		}
	}
	pos := 0
	var read func() (sexp, error)
	read = func() (sexp, error) {
		if pos >= len(toks) {
			return sexp{}, fmt.Errorf("unexpected end")
		}
		t := toks[pos]
		pos++
		switch t {
		case "'":
			// A quoted list reads as the list.
			return read()
		case "(":
			l := sexp{isLst: true}
			for pos < len(toks) && toks[pos] != ")" {
				x, err := read()
				if err != nil {
					return sexp{}, err
				}
				l.list = append(l.list, x)
			}
			if pos >= len(toks) {
				return sexp{}, fmt.Errorf("unclosed (")
			}
			pos++
			return l, nil
		case ")":
			return sexp{}, fmt.Errorf("unexpected )")
		}
		return sexp{atom: t}, nil
	}
	x, err := read()
	if err == nil && pos != len(toks) {
		err = fmt.Errorf("more after the expression")
	}
	return x, err
}

// ---------------------------------------------------------------------------
// Evaluating one for a day
// ---------------------------------------------------------------------------

type sexpDay struct {
	y   int
	m   time.Month
	d   int
	wd  int // 0 Sunday
	num int // agendaDayNum
}

func dayOfNum(n int) sexpDay {
	t := agendaDayTime(n)
	return sexpDay{t.Year(), t.Month(), t.Day(), int(t.Weekday()), n}
}

// matches is whether a date part is t (any), the number, or in a list.
func partMatches(x sexp, v int) (bool, error) {
	if x.isLst {
		for _, e := range x.list {
			if ok, err := partMatches(e, v); err != nil || ok {
				return ok, err
			}
		}
		return false, nil
	}
	if x.atom == "t" {
		return true, nil
	}
	n, err := strconv.Atoi(x.atom)
	if err != nil {
		return false, fmt.Errorf("%q is not a number", x.atom)
	}
	return n == v, nil
}

func sexpInt(x sexp) (int, error) {
	if x.isLst {
		return 0, fmt.Errorf("a number, not a list")
	}
	return strconv.Atoi(x.atom)
}

// ymd orders three date arguments by the style: they come back year, month, day.
func ymd(args []sexp, style string) (y, m, d sexp) {
	switch style {
	case "iso":
		return args[0], args[1], args[2]
	case "european":
		return args[2], args[1], args[0]
	}
	return args[2], args[0], args[1]
}

func dateNum(y, m, d sexp) (int, error) {
	yy, err := sexpInt(y)
	if err != nil {
		return 0, err
	}
	mm, err := sexpInt(m)
	if err != nil {
		return 0, err
	}
	dd, err := sexpInt(d)
	if err != nil {
		return 0, err
	}
	return agendaDayYMD(yy, time.Month(mm), dd), nil
}

// evalSexp is whether the expression holds on day, and the count an
// anniversary fills %d with (0 when none).
func evalSexp(x sexp, day sexpDay, style string) (bool, int, error) {
	if !x.isLst || len(x.list) == 0 {
		switch x.atom {
		case "t":
			return true, 0, nil
		case "nil":
			return false, 0, nil
		}
		return false, 0, fmt.Errorf("%q is not an expression", x.atom)
	}
	fn := x.list[0].atom
	args := x.list[1:]
	need := func(n int) error {
		if len(args) < n {
			return fmt.Errorf("%s needs %d arguments", fn, n)
		}
		return nil
	}
	both := func(y, m, d sexp) (bool, error) {
		ok, err := partMatches(y, day.y)
		if !ok || err != nil {
			return false, err
		}
		if ok, err = partMatches(m, int(day.m)); !ok || err != nil {
			return false, err
		}
		return partMatches(d, day.d)
	}
	anniversary := func(y, m, d sexp) (bool, int, error) {
		yy, err := sexpInt(y)
		if err != nil {
			return false, 0, err
		}
		mm, _ := sexpInt(m)
		dd, _ := sexpInt(d)
		// 29 February lands on the 28th in a common year, as the records'
		// birthdays do.
		if mm == 2 && dd == 29 && agendaDaysIn(day.y, time.February) == 28 {
			dd = 28
		}
		if day.y < yy || int(day.m) != mm || day.d != dd {
			return false, 0, nil
		}
		return true, day.y - yy, nil
	}
	block := func(a, b int) bool { return day.num >= a && day.num <= b }
	switch fn {
	case "diary-date", "org-date":
		if err := need(3); err != nil {
			return false, 0, err
		}
		st := style
		if fn == "org-date" {
			st = "iso"
		}
		y, m, d := ymd(args, st)
		ok, err := both(y, m, d)
		return ok, 0, err
	case "diary-anniversary", "org-anniversary":
		if err := need(3); err != nil {
			return false, 0, err
		}
		st := style
		if fn == "org-anniversary" {
			st = "iso"
		}
		y, m, d := ymd(args, st)
		return anniversary(y, m, d)
	case "diary-block", "org-block":
		if err := need(6); err != nil {
			return false, 0, err
		}
		st := style
		if fn == "org-block" {
			st = "iso"
		}
		y1, m1, d1 := ymd(args[:3], st)
		y2, m2, d2 := ymd(args[3:6], st)
		a, err := dateNum(y1, m1, d1)
		if err != nil {
			return false, 0, err
		}
		b, err := dateNum(y2, m2, d2)
		if err != nil {
			return false, 0, err
		}
		return block(a, b), 0, nil
	case "diary-cyclic", "org-cyclic":
		if err := need(4); err != nil {
			return false, 0, err
		}
		n, err := sexpInt(args[0])
		if err != nil || n <= 0 {
			return false, 0, fmt.Errorf("%s: a cycle of days first", fn)
		}
		st := style
		if fn == "org-cyclic" {
			st = "iso"
		}
		y, m, d := ymd(args[1:4], st)
		start, err := dateNum(y, m, d)
		if err != nil {
			return false, 0, err
		}
		return day.num >= start && (day.num-start)%n == 0, (day.num - start) / n, nil
	case "diary-float":
		if err := need(3); err != nil {
			return false, 0, err
		}
		if ok, err := partMatches(args[0], int(day.m)); !ok || err != nil {
			return false, 0, err
		}
		if ok, err := partMatches(args[1], day.wd); !ok || err != nil {
			return false, 0, err
		}
		n, err := sexpInt(args[2])
		if err != nil {
			return false, 0, err
		}
		if len(args) > 3 {
			// Counted from that day of the month rather than the month's edge.
			from, err := sexpInt(args[3])
			if err != nil {
				return false, 0, err
			}
			if n > 0 {
				return day.d >= from && (day.d-from)/7 == n-1, 0, nil
			}
			return day.d <= from && (from-day.d)/7 == -n-1, 0, nil
		}
		if n > 0 {
			return (day.d-1)/7 == n-1, 0, nil
		}
		last := agendaDaysIn(day.y, day.m)
		return (last-day.d)/7 == -n-1, 0, nil
	case "org-class":
		if err := need(7); err != nil {
			return false, 0, err
		}
		a, err := dateNum(args[0], args[1], args[2])
		if err != nil {
			return false, 0, err
		}
		b, err := dateNum(args[3], args[4], args[5])
		if err != nil {
			return false, 0, err
		}
		wd, err := sexpInt(args[6])
		if err != nil {
			return false, 0, err
		}
		if !block(a, b) || day.wd != wd {
			return false, 0, nil
		}
		// Skipped weeks are ISO week numbers.
		_, week := agendaDayTime(day.num).ISOWeek()
		for _, s := range args[7:] {
			if w, err := sexpInt(s); err == nil && w == week {
				return false, 0, nil
			}
		}
		return true, 0, nil
	case "diary-remind":
		if err := need(2); err != nil {
			return false, 0, err
		}
		leads := []int{}
		if args[1].isLst {
			for _, e := range args[1].list {
				if n, err := sexpInt(e); err == nil {
					leads = append(leads, n)
				}
			}
		} else if n, err := sexpInt(args[1]); err == nil {
			for k := 0; k <= n; k++ {
				leads = append(leads, k)
			}
		}
		if ok, c, err := evalSexp(args[0], day, style); ok || err != nil {
			return ok, c, err
		}
		for _, k := range leads {
			if ok, c, err := evalSexp(args[0], dayOfNum(day.num+k), style); ok || err != nil {
				return ok, c, err
			}
		}
		return false, 0, nil
	case "memq":
		// (memq (calendar-day-of-week date) '(1 2 3 4 5))
		if err := need(2); err != nil {
			return false, 0, err
		}
		if !isDayOfWeek(args[0]) {
			return false, 0, fmt.Errorf("memq: only (calendar-day-of-week date)")
		}
		ok, err := partMatches(args[1], day.wd)
		return ok, 0, err
	case "=":
		if err := need(2); err != nil {
			return false, 0, err
		}
		if !isDayOfWeek(args[0]) {
			return false, 0, fmt.Errorf("=: only (calendar-day-of-week date)")
		}
		n, err := sexpInt(args[1])
		return err == nil && n == day.wd, 0, err
	case "and", "or":
		count := 0
		for _, a := range args {
			ok, c, err := evalSexp(a, day, style)
			if err != nil {
				return false, 0, err
			}
			if c != 0 {
				count = c
			}
			if fn == "and" && !ok {
				return false, 0, nil
			}
			if fn == "or" && ok {
				return true, c, nil
			}
		}
		return fn == "and", count, nil
	case "not":
		if err := need(1); err != nil {
			return false, 0, err
		}
		ok, _, err := evalSexp(args[0], day, style)
		return !ok, 0, err
	}
	return false, 0, fmt.Errorf("orgs does not know (%s ...)", fn)
}

func isDayOfWeek(x sexp) bool {
	return x.isLst && len(x.list) == 2 && x.list[0].atom == "calendar-day-of-week" && x.list[1].atom == "date"
}

// ---------------------------------------------------------------------------
// Where they are in the files
// ---------------------------------------------------------------------------

var sexpStampRe = regexp.MustCompile(`<%%(\(.*?\))>`)

type sexpStamp struct {
	expr sexp
	text string
	err  error
}

type sexpFile struct {
	file string
	// By the row of the heading whose own text the stamp is in.
	byRow map[int][]sexpStamp
}

var sexpParts = NewFileParts[sexpFile]()

// sexpIndex is every file's diary sexp stamps, by file and heading row,
// rebuilt only for files that changed.
func sexpIndex() map[string]map[int][]sexpStamp {
	out := map[string]map[int][]sexpStamp{}
	for _, p := range sexpParts.All(func(f *common.OrgFile) sexpFile {
		sf := sexpFile{file: f.Filename, byRow: map[int][]sexpStamp{}}
		lines := fileLines(f.Filename)
		row := -1
		inBlock := false
		for i, l := range lines {
			if inBlock {
				inBlock = !txBlockEndRe.MatchString(l)
				continue
			}
			if txBlockBeginRe.MatchString(l) {
				inBlock = true
				continue
			}
			if txLevel(l) > 0 {
				row = i
			}
			if !strings.Contains(l, "<%%(") {
				continue
			}
			for _, m := range sexpStampRe.FindAllStringSubmatch(l, -1) {
				x, err := readSexp(m[1])
				sf.byRow[row] = append(sf.byRow[row], sexpStamp{expr: x, text: m[1], err: err})
			}
		}
		return sf
	}) {
		if len(p.byRow) > 0 {
			out[p.file] = p.byRow
		}
	}
	return out
}

var sexpWarned sync.Map

func agendaDateStyle() string {
	if s := Conf().Server; s != nil && s.Agenda.DateStyle != "" {
		return strings.ToLower(s.Agenda.DateStyle)
	}
	return "american"
}

// agendaSexpOccurrences are the days a heading's diary sexp stamps land on,
// with %d in its title filled in where an anniversary or cycle counts.
func agendaSexpOccurrences(sec *org.Section, f *common.OrgFile, idx map[string]map[int][]sexpStamp, r agendaRules) []agendaOcc {
	if sec == nil || sec.Headline == nil || f == nil {
		return nil
	}
	stamps := idx[f.Filename][headlineRow(sec)]
	if len(stamps) == 0 {
		return nil
	}
	style := agendaDateStyle()
	title := strings.TrimSpace(headlineTitle(sec))
	var out []agendaOcc
	for _, st := range stamps {
		if st.err != nil {
			if _, seen := sexpWarned.LoadOrStore(f.Filename+st.text, true); !seen {
				fmt.Fprintf(os.Stderr, "agenda: %s: <%%%%%s>: %v\n", f.Filename, st.text, st.err)
			}
			continue
		}
		for d := r.From; d <= r.To; d++ {
			ok, count, err := evalSexp(st.expr, dayOfNum(d), style)
			if err != nil {
				if _, seen := sexpWarned.LoadOrStore(f.Filename+st.text, true); !seen {
					fmt.Fprintf(os.Stderr, "agenda: %s: <%%%%%s>: %v\n", f.Filename, st.text, err)
				}
				break
			}
			if !ok {
				continue
			}
			e := common.AgendaEntry{Kind: common.AgendaTimestamp, Start: agendaDayStr(d), DueIn: -1}
			if strings.Contains(title, "%d") {
				e.Text = strings.ReplaceAll(sexpStampRe.ReplaceAllString(title, ""), "%d", strconv.Itoa(count))
				e.Text = strings.Join(strings.Fields(e.Text), " ")
			}
			out = append(out, agendaOcc{Day: d, Entry: e})
		}
	}
	return out
}
