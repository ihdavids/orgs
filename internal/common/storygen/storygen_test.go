package storygen

import (
	"reflect"
	"strings"
	"testing"
)

// Every generator, in every genre, many seeds: something to show, nothing
// left unfilled, no line org would read as a heading once filed.
func TestEveryGeneratorEveryGenre(t *testing.T) {
	for _, info := range Generators() {
		for _, genre := range append(genres, "any") {
			for seed := uint64(1); seed <= 40; seed++ {
				res, err := Generate(Request{Type: info.Id, Seed: seed, Count: 1, Options: map[string]string{"genre": genre}})
				if err != nil {
					t.Fatalf("%s: %v", info.Id, err)
				}
				if len(res.Items) != 1 {
					t.Fatalf("%s: %d items", info.Id, len(res.Items))
				}
				it := res.Items[0]
				where := info.Id + "/" + genre
				if strings.TrimSpace(it.Title) == "" || strings.TrimSpace(it.Prose) == "" || len(it.Facts) == 0 {
					t.Errorf("%s seed %d: empty result %+v", where, seed, it)
				}
				for _, s := range append([]string{it.Title, it.Summary, it.Prose, it.Body}, factValues(it.Facts)...) {
					if strings.ContainsAny(s, "{}") {
						t.Errorf("%s seed %d: unfilled %q", where, seed, s)
					}
					if strings.Contains(s, "  ") || strings.Contains(s, "..") && !strings.Contains(s, "…") {
						t.Errorf("%s seed %d: doubled space or stop in %q", where, seed, s)
					}
				}
				for _, l := range strings.Split(it.Body, "\n") {
					if strings.HasPrefix(l, "*") {
						t.Errorf("%s: body line reads as a heading: %q", where, l)
					}
				}
				if !known(it.Tags[0]) {
					t.Errorf("%s: first tag should be the genre, got %v", where, it.Tags)
				}
			}
		}
	}
}

// Every choice of every option, so a table only one choice reads (a season,
// a role, a kind of conflict) is reached; then no generator asked for a table
// the data does not have.
func TestEveryOptionAndNoMissingTables(t *testing.T) {
	for _, info := range Generators() {
		for _, f := range info.Fields {
			for _, ch := range f.Choices {
				for seed := uint64(1); seed <= 15; seed++ {
					res, err := Generate(Request{Type: info.Id, Seed: seed, Count: 1, Options: map[string]string{f.Id: ch.Value}})
					if err != nil || len(res.Items) != 1 || res.Items[0].Prose == "" {
						t.Fatalf("%s %s=%s: %v %+v", info.Id, f.Id, ch.Value, err, res)
					}
				}
			}
		}
	}
	missingMu.Lock()
	defer missingMu.Unlock()
	for name := range missing {
		t.Errorf("no data table %q", name)
	}
}

// The data files parse, every table has entries, and no table repeats one.
func TestDataTables(t *testing.T) {
	stats, err := TableStats()
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for name, n := range stats {
		if n == 0 {
			t.Errorf("table %s is empty", name)
		}
		total += n
		seen := map[string]bool{}
		for _, e := range tab(name).all() {
			k := strings.ToLower(e)
			if seen[k] {
				t.Errorf("table %s has %q twice", name, e)
			}
			seen[k] = true
			if strings.ContainsAny(e, "{}") {
				t.Errorf("table %s: braces in %q", name, e)
			}
		}
	}
	t.Logf("%d tables, %d entries", len(stats), total)
}

func TestParseTables(t *testing.T) {
	got := map[string]gl{}
	err := parseTables("x.txt", "## a\n# used as: {}\n== all\none\n\n== sf\ntwo\n## b\nthree\n", got)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]gl{"a": {"": {"one"}, "sf": {"two"}}, "b": {"": {"three"}}}) {
		t.Fatalf("got %v", got)
	}
	got = map[string]gl{}
	if err := parseTables("x.txt", "## a\nplain\ncrewmate [sf]\nboth [fantasy, modern]\nodd [notes]\n== winter\nsnow [fantasy]\n", got); err != nil {
		t.Fatal(err)
	}
	want := map[string]gl{"a": {"": {"plain", "odd [notes]"}, "@sf": {"crewmate"}, "@fantasy": {"both"}, "@modern": {"both"}, "winter@fantasy": {"snow"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tags: got %v", got)
	}
	c := &ctx{genre: "sf"}
	if l := c.list(got["a"]); !reflect.DeepEqual(l, []string{"plain", "odd [notes]", "crewmate"}) {
		t.Fatalf("sf list %v", l)
	}
	if l := c.section(got["a"], "winter", ""); len(l) != 0 {
		t.Fatalf("sf winter %v", l)
	}
	if parseTables("x.txt", "stray\n", map[string]gl{}) == nil {
		t.Fatal("an entry before any table should be refused")
	}
	if parseTables("x.txt", "## a\nx\n## a\ny\n", map[string]gl{}) == nil {
		t.Fatal("a table defined twice should be refused")
	}
}

func factValues(fs []Fact) []string {
	out := []string{}
	for _, f := range fs {
		out = append(out, f.Key, f.Value)
	}
	return out
}

func TestSameSeedSameIdeas(t *testing.T) {
	a, _ := Generate(Request{Type: "culture", Seed: 99, Count: 3})
	b, _ := Generate(Request{Type: "culture", Seed: 99, Count: 3})
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed gave different results")
	}
	c, _ := Generate(Request{Type: "culture", Seed: 100, Count: 3})
	if reflect.DeepEqual(a.Items, c.Items) {
		t.Fatal("different seeds gave the same results")
	}
}

func TestUnknownGenerator(t *testing.T) {
	if _, err := Generate(Request{Type: "nope"}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestCountBounded(t *testing.T) {
	res, _ := Generate(Request{Type: "premise", Count: 1000, Seed: 1})
	if len(res.Items) != MaxCount {
		t.Fatalf("got %d", len(res.Items))
	}
}

func TestInventedWordsAreSayable(t *testing.T) {
	c := &ctx{r: newRand(7), genre: "fantasy"}
	for i := 0; i < 300; i++ {
		l := c.newLanguage()
		w := c.word(l)
		if len(w) < 2 || len(w) > 11 {
			t.Errorf("odd length %q", w)
		}
		if strings.ContainsAny(w, " -'") {
			t.Errorf("odd characters %q", w)
		}
	}
}

func TestTidy(t *testing.T) {
	for in, want := range map[string]string{"aaab": "aab", "kstra": "ksa", "aeio": "ae", "lorem": "lorem"} {
		if got := tidy(in); got != want {
			t.Errorf("tidy(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestArticle(t *testing.T) {
	for in, want := range map[string]string{
		"uniform": "a uniform", "one-eyed cat": "a one-eyed cat", "hour": "an hour", "umbrella": "an umbrella",
		"heirloom": "an heirloom", "used car": "a used car", "apple": "an apple", "horse": "a horse", "European": "a European",
	} {
		if got := article(in); got != want {
			t.Errorf("article(%q) = %q, want %q", in, got, want)
		}
	}
}
