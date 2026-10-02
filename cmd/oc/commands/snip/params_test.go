package snip

import (
	"reflect"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func names(ps []Param) []string {
	out := []string{}
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

func TestParseHoles(t *testing.T) {
	ps := Parse(`ssh <user=root>@<host> -p <port=22> <user> \<not> a < b > c cat <<EOF`, nil)
	if got := names(ps); !reflect.DeepEqual(got, []string{"user", "host", "port"}) {
		t.Fatalf("names %v", got)
	}
	if ps[0].Default != "root" || ps[2].Default != "22" {
		t.Errorf("defaults %+v", ps)
	}
	// An = in a default is part of it (pet #119).
	if p := Parse(`curl -H <h=X-Key=a=b>`, nil); p[0].Default != "X-Key=a=b" {
		t.Errorf("= in default: %q", p[0].Default)
	}
}

func TestChoicesAndOptional(t *testing.T) {
	ps := Parse(`kubectl -n <ns=|_dev_||_staging_||_prod env_|> get pods <extra?>`, nil)
	if !reflect.DeepEqual(ps[0].Choices, []string{"dev", "staging", "prod env"}) || ps[0].Default != "dev" {
		t.Errorf("choices %+v", ps[0])
	}
	if !ps[1].Optional {
		t.Error("optional not seen")
	}
	if got := Fill(`ls <flags?> /tmp`, Parse(`ls <flags?> /tmp`, nil), nil); got != "ls /tmp" {
		t.Errorf("optional left empty: %q", got)
	}
	if got := Fill(`ls /tmp <flags?>`, Parse(`ls /tmp <flags?>`, nil), nil); got != "ls /tmp" {
		t.Errorf("optional at the end: %q", got)
	}
}

func TestFillQuotesForWhereItLands(t *testing.T) {
	cmd := `grep <pat> "<file>" '<x>' <glob=*.log>`
	got := Fill(cmd, Parse(cmd, nil), map[string]string{
		"pat": "two words", "file": `my "notes".txt`, "x": "it's",
	})
	want := `grep 'two words' "my \"notes\".txt" 'it'\''s' *.log`
	if got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}
	// A plain value is left alone.
	if got := Fill(`echo <a>`, Parse(`echo <a>`, nil), map[string]string{"a": "hello"}); got != "echo hello" {
		t.Errorf("%q", got)
	}
	// The escape leaves a literal bracket.
	if got := Fill(`echo \<b>`, nil, nil); got != `echo <b>` {
		t.Errorf("escape: %q", got)
	}
}

func TestBabelVars(t *testing.T) {
	cmd := "psql -h $host -p ${port} '$host' $hostname"
	vars := []common.CodeVar{
		{Name: "host", Value: `"db.internal"`},
		{Name: "port", Value: "5432"},
		{Name: "unused", Value: "1"},
		{Name: "env", Value: `'("dev" "prod")`},
	}
	ps := Parse(cmd+" $env", vars)
	if got := names(ps); !reflect.DeepEqual(got, []string{"host", "port", "env"}) {
		t.Fatalf("names %v", got)
	}
	if ps[0].Default != "db.internal" || !reflect.DeepEqual(ps[2].Choices, []string{"dev", "prod"}) {
		t.Errorf("%+v", ps)
	}
	got := Fill(cmd, ps, map[string]string{"host": "a b"})
	// Single quotes keep their $host; $hostname is not $host.
	want := "psql -h 'a b' -p 5432 '$host' $hostname"
	if got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}
	// A :var naming a table is data for babel, not a parameter.
	if ps := Parse("echo $t", []common.CodeVar{{Name: "t", Value: "tbl", Ref: "tbl", RefKind: "table"}}); len(ps) != 0 {
		t.Errorf("table var became a parameter: %+v", ps)
	}
}

func TestVarGivesInlineHoleItsDefault(t *testing.T) {
	ps := Parse("ping <host>", []common.CodeVar{{Name: "host", Value: `"8.8.8.8"`}})
	if len(ps) != 1 || ps[0].Default != "8.8.8.8" {
		t.Errorf("%+v", ps)
	}
}
