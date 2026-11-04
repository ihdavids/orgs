package biblio

import (
	"strings"
	"testing"
)

const bib = `@string{jfp = "Journal of Functional Programming"}
@comment{ignored @article{nope, title={x}} }
@article{doe2020,
  author  = {Doe, Jane and Richard Roe},
  title   = {{Typed} Holes in Practice},
  journal = jfp # " (Special)",
  year    = 2020,
  volume  = {30}, number = {2}, pages = {1--20},
  doi     = {10.1/abc}
}
@book{who2019, author = {{World Health Organization}}, title = "Report", publisher = {WHO}, address = {Geneva}, date = {2019-05-01}}
@inproceedings{gödel, author = {G\"odel, Kurt and van Dalen, Dirk and Ludwig van Beethoven}, title = {On it}, booktitle = {Proc.}, year = {1931}}
`

func TestParseBibTeX(t *testing.T) {
	es, err := ParseBibTeX(bib)
	if err != nil || len(es) != 3 {
		t.Fatalf("%v %d entries", err, len(es))
	}
	d := es[0]
	if d.Key != "doe2020" || d.Title() != "Typed Holes in Practice" || d.Field("journal") != "Journal of Functional Programming (Special)" || d.Year() != "2020" {
		t.Errorf("doe: %+v", d.Fields)
	}
	if len(d.Authors) != 2 || d.Authors[0].Family != "Doe" || d.Authors[1].Family != "Roe" || d.Authors[1].Given != "Richard" {
		t.Errorf("authors %+v", d.Authors)
	}
	if es[1].AuthorShort(false) != "World Health Organization" || es[1].Year() != "2019" {
		t.Errorf("org: %q %q", es[1].AuthorShort(false), es[1].Year())
	}
	g := es[2]
	if g.Authors[0].Family != "Gödel" || g.Authors[1].Family != "van Dalen" || g.Authors[2].Family != "van Beethoven" || g.AuthorShort(false) != "Gödel et al." {
		t.Errorf("names %+v %q", g.Authors, g.AuthorShort(false))
	}
}

func TestParseCSLJSON(t *testing.T) {
	es, err := ParseCSLJSON([]byte(`[{"id":"smith","type":"article-journal","title":"T","container-title":"J","author":[{"family":"Smith","given":"A"}],"issued":{"date-parts":[[2018,3]]}}]`))
	if err != nil || len(es) != 1 || es[0].Year() != "2018" || es[0].AuthorShort(false) != "Smith" || es[0].Field("journal") != "J" {
		t.Fatalf("%v %+v", err, es)
	}
}

func TestCitationsAsOrgBasicWritesThem(t *testing.T) {
	es, _ := ParseBibTeX(bib)
	m := map[string]*Entry{}
	for _, e := range es {
		m[e.Key] = e
	}
	missing := []string{}
	f := func(style, body string, nums map[string]int) string {
		return Format(ParseCitation(style, body), m, nums, func(k string) { missing = append(missing, k) }, nil)
	}
	cases := []struct{ style, body, want string }{
		{"", "@doe2020", "(Doe and Roe, 2020)"},
		{"", "see @doe2020 p. 3; @who2019", "(see Doe and Roe, 2020, p. 3; World Health Organization, 2019)"},
		{"/t", "@doe2020", "Doe and Roe (2020)"},
		{"/a/c", "@gödel", "Gödel et al."},
		{"/a/f", "@gödel", "Gödel, van Dalen and van Beethoven"},
		{"/na", "@who2019", "(2019)"},
		{"/n", "@doe2020", ""},
		{"", "common pre; @doe2020; @who2019; common post", "(common pre Doe and Roe, 2020; World Health Organization, 2019, common post)"},
		{"", "@nobody", "(nobody)"},
		{"//b", "@doe2020", "Doe and Roe, 2020"},
	}
	for _, c := range cases {
		if got := f(c.style, c.body, nil); got != c.want {
			t.Errorf("[cite%s:%s] = %q, want %q", c.style, c.body, got, c.want)
		}
	}
	if len(missing) != 1 || missing[0] != "nobody" {
		t.Errorf("missing %v", missing)
	}
	if got := f("", "@doe2020; @who2019", map[string]int{"doe2020": 1, "who2019": 2}); got != "[1, 2]" {
		t.Errorf("numeric %q", got)
	}
	ref := m["doe2020"].Reference()
	if !strings.HasPrefix(ref, "Doe, Jane, and Richard Roe. 2020. “Typed Holes in Practice.” /Journal of Functional Programming (Special)/ 30 (2): 1–20. [[https://doi.org/10.1/abc]") {
		t.Errorf("reference %q", ref)
	}
	if r := m["who2019"].Reference(); r != "World Health Organization. 2019. /Report/. Geneva: WHO." {
		t.Errorf("book %q", r)
	}
}
