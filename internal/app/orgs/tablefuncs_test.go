package orgs

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"

	"github.com/ihdavids/orgs/internal/common"
)

// evalOrgTable parses an org document holding one table under one heading,
// runs its #+TBLFM and returns the table so cells can be read back.
func evalOrgTable(t *testing.T, text string) *org.Table {
	t.Helper()
	doc := org.New().Parse(strings.NewReader(text), "test.org")
	if doc.Error != nil {
		t.Fatalf("parse: %v", doc.Error)
	}
	if len(doc.Outline.Children) == 0 || len(doc.Outline.Children[0].Headline.Tables) == 0 {
		t.Fatalf("no table found in:\n%s", text)
	}
	sec := doc.Outline.Children[0]
	tbl := sec.Headline.Tables[0]
	ofile := &common.OrgFile{Filename: "test.org", Doc: doc}
	if err := ExecuteFormula(nil, sec, ofile, tbl); err != nil {
		t.Fatalf("execute: %v", err)
	}
	return tbl
}

func cell(tbl *org.Table, row, col int) string {
	return strings.TrimSpace(tbl.GetVal(row, col))
}

func TestHarnessRunsExistingFunctions(t *testing.T) {
	tbl := evalOrgTable(t, `* Heading
| a | b |
|---+---|
| 1 |   |
| 2 |   |
| 3 |   |
#+TBLFM: @2$2=vsum(@2$1..@4$1)
`)
	if got := cell(tbl, 2, 2); got != "6" {
		t.Errorf("vsum: got %q", got)
	}
}

// expectCells checks cells given as "@row$col" against their text.
func expectCells(t *testing.T, tbl *org.Table, want map[string]string) {
	t.Helper()
	for ref, w := range want {
		var r, c int
		if _, err := fmt.Sscanf(ref, "@%d$%d", &r, &c); err != nil {
			t.Fatalf("bad ref %q", ref)
		}
		if got := cell(tbl, r, c); got != w {
			t.Errorf("%s: got %q want %q", ref, got, w)
		}
	}
}

const produce = `* Produce
| name  | qty | price | kind  | out |
|-------+-----+-------+-------+-----|
| apple |   3 |   1.5 | fruit |     |
| pear  |   5 |     2 | fruit |     |
| kale  |   2 |     4 | veg   |     |
| beet  |     |     1 | veg   |     |
`

func TestConditionalAggregates(t *testing.T) {
	cases := map[string]string{
		"vsumif($4,'fruit',$2)":             "8",
		"vsumif($4,'FRUIT',$2)":             "8",
		"vsumif($2,'>2')":                   "8",
		"vcountif($4,'veg')":                "2",
		"vcountif($4,'<>fruit')":            "2",
		"vcountif($1,'p*')":                 "1",
		"vcountif($1,'?ale')":               "1",
		"vcountif($2,'')":                   "1",
		"vmeanif($4,'fruit',$3)":            "1.75",
		"vmaxif($4,'fruit',$2)":             "5",
		"vminif($4,'fruit',$2)":             "3",
		"vsumifs($2,$4,'fruit',$3,'>1.5')":  "5",
		"vcountifs($4,'veg',$3,'<2')":       "1",
		"vmeanifs($3,$4,'veg',$1,'<>beet')": "4",
		"vcounta($2)":                       "3",
		"vcountblank($2)":                   "1",
		"vmaxif($4,'nothing',$2)":           "0",
	}
	for f, want := range cases {
		tbl := evalOrgTable(t, produce+"#+TBLFM: @2$5="+f+"\n")
		if got := cell(tbl, 2, 5); got != want {
			t.Errorf("%s: got %q want %q", f, got, want)
		}
	}
}

const keyed = `* Keyed
| key | val | out |
|-----+-----+-----|
| a   |   1 |     |
| b   |   2 |     |
| a   |   3 |     |
`

func TestLookups(t *testing.T) {
	cases := map[string]string{
		"lookup('a',$1,$2)":               "1",
		"lookupfirst('b',$1,$2)":          "2",
		"lookuplast('a',$1,$2)":           "3",
		"lookup('z',$1,$2)":               "",
		"lookup('b',$1)":                  "b",
		"join(',', lookupall('a',$1,$2))": "1,3",
		"match('b',$1)":                   "2",
		"match('z',$1)":                   "0",
		"index($2,2)":                     "2",
		"index($2,-1)":                    "3",
	}
	for f, want := range cases {
		tbl := evalOrgTable(t, keyed+"#+TBLFM: @2$3="+f+"\n")
		if got := cell(tbl, 2, 3); got != want {
			t.Errorf("%s: got %q want %q", f, got, want)
		}
	}
	// join takes a column whole; concat reads this row.
	tbl0 := evalOrgTable(t, keyed+"#+TBLFM: @2$3=join('+',$1)::@3$3=concat($1,$2)\n")
	expectCells(t, tbl0, map[string]string{"@2$3": "a+b+a", "@3$3": "b2"})
	// The value is read from the current row while the ranges are whole columns.
	tbl := evalOrgTable(t, keyed+"#+TBLFM: $3=lookuplast($1,$1,$2)\n")
	expectCells(t, tbl, map[string]string{"@2$3": "3", "@3$3": "2", "@4$3": "3"})
}

func TestStatistics(t *testing.T) {
	rows := ""
	for _, v := range []string{"2", "4", "4", "4", "5", "5", "7", "9"} {
		rows += "| " + v + " | |\n"
	}
	table := "* Stats\n| x | out |\n|---+-----|\n" + rows
	cases := map[string]string{
		"vpsdev($1)":          "2",
		"vpvar($1)":           "4",
		"vvar($1);%.4f":       "4.5714",
		"vsdev($1);%.4f":      "2.1381",
		"vmode($1)":           "4",
		"vpercentile($1,0.5)": "4.5",
		"vpercentile($1,50)":  "4.5",
		"vquartile($1,4)":     "9",
		"vrank(7,$1)":         "2",
		"vrank(7,$1,true)":    "7",
		"vprod(2,3,4)":        "24",
		"vmode(5,4,4,5)":      "5",
	}
	for f, want := range cases {
		tbl := evalOrgTable(t, table+"#+TBLFM: @2$2="+f+"\n")
		if got := cell(tbl, 2, 2); got != want {
			t.Errorf("%s: got %q want %q", f, got, want)
		}
	}
}

const words = `* Words
| a     | b | out |
|-------+---+-----|
| Hello | 3 |     |
`

func TestLogicTextAndRounding(t *testing.T) {
	cases := map[string]string{
		"if($2>2,'big','small')":  "big",
		"if($2>5,'big','small')":  "small",
		"and($2>1,$2<5)":          "true",
		"or($2>5,$2<1)":           "false",
		"xor(true,false)":         "true",
		"not(isblank($1))":        "true",
		"isnumber($2)":            "true",
		"istext($1)":              "true",
		"iferror(1/0,'none')":     "none",
		"iferror(sqrt(-1),'bad')": "bad",
		"iferror($2,'bad')":       "3",
		"concat($1,'-',$2)":       "Hello-3",
		"join(' ', $1, 'there')":  "Hello there",
		"upper($1)":               "HELLO",
		"lower($1)":               "hello",
		"len($1)":                 "5",
		"left($1,2)":              "He",
		"left($1)":                "H",
		"right($1,3)":             "llo",
		"mid($1,2,3)":             "ell",
		"substitute($1,'l','L')":  "HeLLo",
		"trim('  a   b ')":        "a b",
		"contains($1,'ELL')":      "true",
		"startswith($1,'he')":     "true",
		"endswith($1,'x')":        "false",
		"rept('ab',$2)":           "ababab",
		"value('USD 1,234.5')":    "1234.5",
		"value('12 kg')":          "12",
		"fmt('%05.1f',$2)":        "003.0",
		"fmt('%s: %d',$1,$2)":     "Hello: 3",
		"fmt('%s',$2)":            "3",
		"len($1)+1":               "6",
		"fmt('%d items',$2)":      "3 items",
		"round(2.375,2)":          "2.38",
		"round(2.6)":              "3",
		"roundup(2.301,1)":        "2.4",
		"rounddown(-2.39,1)":      "-2.3",
		"sign(-4)":                "-1",
		"clamp(15,0,10)":          "10",
		"randint($2,$2)":          "3",
	}
	for f, want := range cases {
		tbl := evalOrgTable(t, words+"#+TBLFM: @2$3="+f+"\n")
		if got := cell(tbl, 2, 3); got != want {
			t.Errorf("%s: got %q want %q", f, got, want)
		}
	}
}

const dated = `* Dates
| start            | end        | out |
|------------------+------------+-----|
| <2026-01-31 Sat> | 2026-03-01 |     |
`

func TestDates(t *testing.T) {
	cases := map[string]string{
		"addmonths($1,1)":           "<2026-02-28 Sat>",
		"edate($1,-2)":              "<2025-11-30 Sun>",
		"addyears($1,1)":            "<2027-01-31 Sun>",
		"adddays($1,1)":             "<2026-02-01 Sun>",
		"addtime($1,'2h')":          "<2026-01-31 Sat 02:00>",
		"days($2,$1)":               "29",
		"datedif($1,$2,'d')":        "29",
		"datedif($1,$2,'m')":        "1",
		"datedif($1,$2,'w')":        "4",
		"datedif($2,$1,'d')":        "-29",
		"eomonth($1,1)":             "<2026-02-28 Sat>",
		"eomonth($1)":               "<2026-01-31 Sat>",
		"weeknum($1)":               "5",
		"quarter($2)":               "1",
		"workday($1,1)":             "<2026-02-02 Mon>",
		"workday($1,-1)":            "<2026-01-30 Fri>",
		"networkdays($1,$2)":        "20",
		"datefmt($1,'%Y/%m/%d %a')": "2026/01/31 Sat",
		"datefmt($1,'%j %V %%')":    "031 05 %",
		"date(2026,2,30)":           "<2026-03-02 Mon>",
		"isdate($1)":                "true",
		"monthname('2026-01-05')":   "January",
		"weekdayname('2026-01-04')": "Sunday",
		"weekday('2026-01-04')":     "0",
		"yearday('2026-02-01')":     "32",
		"month($2)":                 "3",
	}
	for f, want := range cases {
		tbl := evalOrgTable(t, dated+"#+TBLFM: @2$3="+f+"\n")
		if got := cell(tbl, 2, 3); got != want {
			t.Errorf("%s: got %q want %q", f, got, want)
		}
	}
}

// Expected values are the examples in the spreadsheet documentation.
func TestFinance(t *testing.T) {
	cases := map[string]string{
		"pmt(0.05/12,360,200000)":                       "-1073.64",
		"pmt(0,10,1000)":                                "-100.00",
		"fv(0.06/12,10,-200,-500,1)":                    "2581.40",
		"pv(0.08/12,12*20,500)":                         "-59777.15",
		"nper(0.12/12,-100,-1000,10000,1)":              "59.67",
		"npv(0.1,-10000,3000,4200,6800)":                "1188.44",
		"irr(-70000,12000,15000,18000,21000,26000)":     "0.09",
		"irr(-70000,12000,15000,18000,21000,26000)*100": "8.66",
	}
	for f, want := range cases {
		tbl := evalOrgTable(t, words+"#+TBLFM: @2$3="+f+";%.2f\n")
		if got := cell(tbl, 2, 3); got != want {
			t.Errorf("%s: got %q want %q", f, got, want)
		}
	}
}

// The list functions used to receive their arguments as one slice and
// return nothing; neg dropped its argument the same way; passed compared
// its first argument with itself; vmax of nothing was -1.7e308.
func TestFixedListFunctions(t *testing.T) {
	table := `* Lists
| x | out |
|---+-----|
| 3 |     |
| 1 |     |
| 3 |     |
| 2 |     |
`
	cases := map[string]string{
		"join(',', sort($1))":  "1,2,3,3",
		"join(',', rsort($1))": "3,3,2,1",
		"join(',', rdup($1))":  "3,1,2",
		"join(',', rev($1))":   "2,3,1,3",
		"neg($1)":              "-3",
		"passed(1,2)":          "FAILED",
		"passed(2,2)":          "PASSED",
		"vmax($2)":             "0",
		"vmin($2)":             "0",
		"vmax($1)":             "3",
		"vmin($1)":             "1",
	}
	for f, want := range cases {
		tbl := evalOrgTable(t, table+"#+TBLFM: @2$2="+f+"\n")
		if got := cell(tbl, 2, 2); got != want {
			t.Errorf("%s: got %q want %q", f, got, want)
		}
	}
}

// remote ranges used to have no calc state, so expanding one (vsum, or any
// range function) read cells through a nil pointer.
func TestRemoteRanges(t *testing.T) {
	saved := odb
	odb = NewOrgDb()
	t.Cleanup(func() { odb = saved })

	fights := `* Fights
| who | dmg |
|-----+-----|
| M1  |   5 |
| M2  |   7 |
| M1  |  12 |
`
	doc := org.New().Parse(strings.NewReader(fights), "fights.org")
	odb.NamedTables["FightHistory"] = []*TableFile{{
		Table: doc.Outline.Children[0].Headline.Tables[0],
		File:  &common.OrgFile{Filename: "fights.org", Doc: doc},
	}}

	tbl := evalOrgTable(t, `* Monsters
| name | total | last |
|------+-------+------|
| M1   |       |      |
| M2   |       |      |
#+TBLFM: $2=vsumif(remote(FightHistory,$1),$1,remote(FightHistory,$2))::$3=lookuplast($1,remote(FightHistory,$1),remote(FightHistory,$2))
`)
	expectCells(t, tbl, map[string]string{"@2$2": "17", "@3$2": "7", "@2$3": "12", "@3$3": "7"})

	tbl = evalOrgTable(t, `* Total
| total |
|-------|
|       |
#+TBLFM: @2$1=vsum(remote(FightHistory,$2))
`)
	expectCells(t, tbl, map[string]string{"@2$1": "24"})
}

// Column formulas on a table with a header used to be evaluated on the header
// row too, and arithmetic with an empty cell failed: either stopped the table.
func TestArithmeticOnHeadersAndEmptyCells(t *testing.T) {
	tbl := evalOrgTable(t, `* Sums
| a | b | product | sum |
|---+---+---------+-----|
| 3 | 2 |         |     |
| 4 |   |         |     |
|   |   |         |     |
#+TBLFM: $3=$1*$2::$4=$1+$2
`)
	expectCells(t, tbl, map[string]string{
		"@1$3": "product", "@2$3": "6", "@3$3": "0", "@4$3": "0",
		"@2$4": "5", "@3$4": "4", "@4$4": "0",
	})
}

// A parameter after an operator: the lexer read "*$" as one operator.
func TestParameterAfterOperator(t *testing.T) {
	tbl := evalOrgTable(t, `* Params
|   | net | tax |
|---+-----+-----|
|   | 100 |     |
| $ | rate=0.2 |     |
#+TBLFM: $3=$2*$rate
`)
	expectCells(t, tbl, map[string]string{"@2$3": "20"})
}

// The format suffix ;%d was handed a float64 and wrote %!d(float64=4.5).
func TestIntegerFormatSuffix(t *testing.T) {
	tbl := evalOrgTable(t, `* Whole
| a | b |
|---+---|
| 3 |   |
| 5 |   |
#+TBLFM: $2=$1*1.5;%d
`)
	expectCells(t, tbl, map[string]string{"@2$2": "4", "@3$2": "7"})
}
