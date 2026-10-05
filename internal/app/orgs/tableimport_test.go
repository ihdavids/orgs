package orgs

import (
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func TestTheSeparatorIsGuessedFromTheFirstLine(t *testing.T) {
	cases := map[string]rune{
		"a,b,c\n1,2,3":       ',',
		"a\tb\tc\n1\t2\t3":   '\t',
		"a;b;c\n1,5;2,5;3":   ';',
		"name|qty\nfoo|2":    '|',
		"just one column\nx": ',',
	}
	for text, want := range cases {
		if got := guessSeparator(text); got != want {
			t.Errorf("%q: got %q, want %q", text, got, want)
		}
	}
}

func TestACsvBecomesATableAtTheEndOfAFile(t *testing.T) {
	paths := loadOrg(t, map[string]string{"t.org": "* Sales\nSome words.\n\n"})
	res, err := ImportTable(&common.TableImport{
		Filename: paths["t.org"], AfterLine: -1, Header: true, Name: "sales",
		Text: "\uFEFFmonth,\"amount, gross\",note\nJan,120,\"a | b\"\nFeb,180\n",
	})
	if err != nil || !res.Ok {
		t.Fatalf("import: %v %+v", err, res)
	}
	wantOrg(t, "import", readOrg(t, paths["t.org"]), `* Sales
Some words.

#+NAME: sales
| month | amount, gross | note        |
|-------+---------------+-------------|
| Jan   |           120 | a \vert{} b |
| Feb   |           180 |             |`)
}

func TestACsvGoesUnderItsHeadingBeforeTheNext(t *testing.T) {
	paths := loadOrg(t, map[string]string{"h.org": "* One\nText.\n\n* Two\n"})
	registerAllSections()
	hash := ""
	for _, s := range flattenSections(GetDb().FindByFile(paths["h.org"])) {
		if common.GetSectionTitle(s) == "One" {
			hash = s.Hash
		}
	}
	if _, err := ImportTable(&common.TableImport{Hash: hash, Text: "x\ty\n1\t2"}); err != nil {
		t.Fatal(err)
	}
	got := readOrg(t, paths["h.org"])
	if !strings.HasPrefix(got, "* One\nText.\n\n| x | y |\n| 1 | 2 |\n\n* Two") {
		t.Errorf("under heading:\n%s", got)
	}
}
