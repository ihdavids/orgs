package orgs

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

// The mentions found on a file's text, as "row:text" in order.
func mentionsOn(text string, terms ...string) []string {
	lines := strings.Split(text, "\n")
	scan := scanMentionLines(lines, map[string]bool{"TODO": true, "DONE": true})
	ts := compileMentionTerms(terms)
	var out []string
	for i := range scan.lines {
		l := &scan.lines[i]
		for _, m := range matchMentionLine(l, ts) {
			out = append(out, fmt.Sprintf("%02d:%s", l.row, l.text[m.s:m.e]))
		}
	}
	return out
}

func wantMentions(t *testing.T, label string, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%s:\n got  %q\n want %q", label, got, want)
	}
}

func TestMentionWordBoundaries(t *testing.T) {
	text := "Apollo went up. Apollos did not. preApollo neither.\n" +
		"apollo in lower case, APOLLO in upper, (Apollo) in brackets.\n" +
		"Project  Apollo spans two spaces; Project\tApollo a tab."
	wantMentions(t, "boundaries", mentionsOn(text, "Apollo"),
		"00:Apollo", "01:apollo", "01:APOLLO", "01:Apollo", "02:Apollo", "02:Apollo")
	// The longer name claims its words before the shorter one can.
	wantMentions(t, "longest first", mentionsOn(text, "Apollo", "Project Apollo"),
		"00:Apollo", "01:apollo", "01:APOLLO", "01:Apollo", "02:Project  Apollo", "02:Project\tApollo")
	// A symbol at the end of a name is not held to a word boundary.
	wantMentions(t, "symbol", mentionsOn("we use C++ and C++11 here", "C++"), "00:C++", "00:C++")
	// Unicode letters are letters.
	wantMentions(t, "unicode", mentionsOn("Zürich und Zürichsee", "Zürich"), "00:Zürich")
}

func TestMentionSkipsWhatIsNotProse(t *testing.T) {
	text := strings.Join([]string{
		"#+TITLE: Apollo notes",                       // 0 keyword line
		"* TODO Apollo launch  :space:",               // 1 headline: the title counts
		":PROPERTIES:",                                // 2
		":NAME: Apollo",                               // 3 property drawer
		":END:",                                       // 4
		"Already [[id:x][Apollo]] linked, and [[Apollo]] too.", // 5
		"Code =Apollo= and ~Apollo~ and https://x.org/Apollo/y.", // 6
		"#+begin_src go",                              // 7
		"// Apollo",                                   // 8 src block
		"#+end_src",                                   // 9
		"#+BEGIN_QUOTE",                               // 10
		"Apollo in a quote is prose.",                 // 11
		"#+END_QUOTE",                                 // 12
		"# Apollo in a comment",                       // 13
		":LOGBOOK:",                                   // 14
		"- Note Apollo",                               // 15
		":END:",                                       // 16
		"#+begin_example",                             // 17
		"Apollo",                                      // 18
		"#+end_example",                               // 19
		": Apollo fixed width",                        // 20
		"* Apollo  :apollo:",                          // 21 the tag is not prose
		"Plain Apollo, and after the link [[x][y]] Apollo again.", // 22
	}, "\n")
	wantMentions(t, "prose only", mentionsOn(text, "Apollo"),
		"01:Apollo", "11:Apollo", "21:Apollo", "22:Apollo", "22:Apollo")
}

func TestMentionHeadlineKeyword(t *testing.T) {
	// The keyword and priority are not the title; a word in the title that
	// looks like a keyword is.
	wantMentions(t, "keyword", mentionsOn("** TODO [#A] Review TODO lists", "TODO"), "00:TODO")
}

func TestMentionTermsGuard(t *testing.T) {
	terms, skipped := mentionTerms("Notes", []string{"API", "x"}, false)
	if strings.Join(terms, ",") != "API" {
		t.Errorf("terms %q", terms)
	}
	if len(skipped) != 2 || !strings.Contains(skipped[0], "too common") || !strings.Contains(skipped[1], "too short") {
		t.Errorf("skipped %q", skipped)
	}
	// all= takes the title at its word.
	if terms, _ := mentionTerms("Notes", nil, true); strings.Join(terms, ",") != "Notes" {
		t.Errorf("forced %q", terms)
	}
	// Markup, cookies and links come off a title before it is looked for.
	if got := cleanMentionTitle("*Project* [[id:1][Apollo]] [2/3] <2026-10-10 Sat>"); got != "Project Apollo" {
		t.Errorf("clean %q", got)
	}
	if got := splitAliases(`"Moon shot" Apollo  "Saturn V"`); strings.Join(got, "|") != "Moon shot|Apollo|Saturn V" {
		t.Errorf("roam aliases %q", got)
	}
	if got := splitAliases(`Moon shot, Apollo`); strings.Join(got, "|") != "Moon shot|Apollo" {
		t.Errorf("comma aliases %q", got)
	}
}

func TestLinkMentionLines(t *testing.T) {
	lines := []string{"Apollo and Apollo.", "nothing", "then Apollo"}
	refs := []common.MentionRef{
		{Line: 0, Col: 0, Text: "Apollo"},
		{Line: 0, Col: 11, Text: "Apollo"},
		{Line: 2, Col: 5, Text: "Apollo"},
		{Line: 1, Col: 0, Text: "Apollo"}, // stale
	}
	out, res := linkMentionLines(lines, refs, func(t string) string { return "[[id:X][" + t + "]]" })
	want := "[[id:X][Apollo]] and [[id:X][Apollo]].|nothing|then [[id:X][Apollo]]"
	if strings.Join(out, "|") != want {
		t.Errorf("lines %q", strings.Join(out, "|"))
	}
	if !res[0].Ok || !res[1].Ok || !res[2].Ok || res[3].Ok || !strings.Contains(res[3].Msg, "no longer") {
		t.Errorf("outcomes %+v", res)
	}
	// Linking the same mention twice is refused the second time: it is in a
	// link now.
	_, again := linkMentionLines(out, []common.MentionRef{{Line: 2, Col: 5, Text: "[[id:X][Apollo]]"}}, func(t string) string { return t })
	if again[0].Ok {
		t.Errorf("linked a link: %+v", again[0])
	}
}

const mentionsFixture = `#+TITLE: Space
* Apollo
:PROPERTIES:
:ROAM_ALIASES: "Moon shot"
:END:
Apollo itself is not a mention of itself.
** Saturn
Apollo inside its own subtree is not one either.
* Plans
We will fly Apollo, the moon shot, soon.
#+begin_src sh
echo Apollo
#+end_src
* Done
Already [[*Apollo][Apollo]] linked.
`

func TestMentionsAndLinkingThroughTheApi(t *testing.T) {
	paths := loadOrg(t, map[string]string{
		"space.org": mentionsFixture,
		"other.org": "* Elsewhere\nThe Apollo programme.\n",
	})
	sec := findHeadingByText(GetDb().FindByFile(paths["space.org"]), "Apollo")
	if sec == nil {
		t.Fatal("no Apollo heading")
	}
	hash := sec.Hash
	get := func(u string) common.MentionsResult {
		t.Helper()
		_, raw := call(t, RequestMentions, "GET", u, nil, nil)
		var res common.MentionsResult
		if err := json.Unmarshal(raw, &res); err != nil || !res.Ok {
			t.Fatalf("%s: %v %s", u, err, raw)
		}
		return res
	}
	res := get("/mentions?hash=" + url.QueryEscape(hash))
	var got []string
	for _, m := range res.Mentions {
		got = append(got, m.Text+"@"+m.Headline)
	}
	wantMentions(t, "found", got, "Apollo@Elsewhere", "Apollo@Plans", "moon shot@Plans") // other.org sorts before space.org
	if res.Target.Id != "" || strings.Join(res.Target.Terms, ",") != "Apollo,Moon shot" {
		t.Errorf("target %+v", res.Target)
	}
	// The same heading, named by file and a line inside it.
	if again := get("/mentions?filename=" + url.QueryEscape(paths["space.org"]) + "&line=5"); again.Target.Hash != res.Target.Hash {
		t.Errorf("line 5 is Apollo's own text: %+v", again.Target)
	}
	if again := get("/mentions?filename=" + url.QueryEscape(paths["space.org"]) + "&line=7"); again.Target.Headline != "Saturn" {
		t.Errorf("line 7 is under Saturn: %+v", again.Target)
	}

	// Link them all: the heading gets an id, every line holding a mention
	// changes, and nothing else does.
	beforeSpace := readOrg(t, paths["space.org"])
	req := common.MentionLinkRequest{Hash: hash}
	for _, m := range res.Mentions {
		req.Mentions = append(req.Mentions, common.MentionRef{Filename: m.Filename, Line: m.Line, Col: m.Col, Text: m.Text})
	}
	_, raw := call(t, PostLinkMentions, "POST", "/mentions/link", nil, req)
	var lr common.MentionLinkResult
	if err := json.Unmarshal(raw, &lr); err != nil || !lr.Ok || lr.Linked != 3 || !lr.NewId {
		t.Fatalf("link: %v %s", err, raw)
	}
	if !regexp.MustCompile(`^[0-9a-f-]{36}$`).MatchString(lr.Id) {
		t.Errorf("id %q", lr.Id)
	}
	space := readOrg(t, paths["space.org"])
	wantSpace := strings.Replace(beforeSpace,
		":ROAM_ALIASES: \"Moon shot\"", ":ROAM_ALIASES: \"Moon shot\"\n:ID:           "+lr.Id, 1)
	wantSpace = strings.Replace(wantSpace, "We will fly Apollo, the moon shot, soon.",
		"We will fly [[id:"+lr.Id+"][Apollo]], the [[id:"+lr.Id+"][moon shot]], soon.", 1)
	wantOrg(t, "space.org", space, wantSpace)
	wantOrg(t, "other.org", readOrg(t, paths["other.org"]), "* Elsewhere\nThe [[id:"+lr.Id+"][Apollo]] programme.")

	// Asked again, they are links now: nothing left to find, and the heading
	// has backlinks from both files.
	if left := get("/mentions?hash=" + url.QueryEscape(hash)); len(left.Mentions) != 0 || left.Target.Id != lr.Id {
		t.Errorf("left %+v", left)
	}
	_, raw = call(t, RequestLinksAround, "GET", "/links/heading?hash="+url.QueryEscape(hash), nil, nil)
	var around common.LinksAround
	if err := json.Unmarshal(raw, &around); err != nil || !around.Ok {
		t.Fatalf("links: %v %s", err, raw)
	}
	// Three id links and the fuzzy heading link in "Done".
	if len(around.In) != 4 {
		t.Fatalf("in %s", raw)
	}
	for _, l := range around.In {
		if l.Context == "" || strings.Contains(l.Context, "[[") {
			t.Errorf("context %q", l.Context)
		}
	}

	// Stale: the same request again refuses every mention and writes nothing.
	_, raw = call(t, PostLinkMentions, "POST", "/mentions/link", nil, req)
	if err := json.Unmarshal(raw, &lr); err != nil || lr.Ok || lr.Linked != 0 || !strings.Contains(lr.Msg, "no longer") {
		t.Errorf("stale: %s", raw)
	}
	wantOrg(t, "unchanged", readOrg(t, paths["space.org"]), space)
}

func TestMentionsOfAFileLinkByPath(t *testing.T) {
	paths := loadOrg(t, map[string]string{
		"kubernetes.org": "* Pods\n",
		"ops.org":        "* Cluster\nWe run Kubernetes everywhere.\n",
	})
	_, raw := call(t, RequestMentions, "GET", "/mentions?filename="+url.QueryEscape(paths["kubernetes.org"]), nil, nil)
	var res common.MentionsResult
	if err := json.Unmarshal(raw, &res); err != nil || len(res.Mentions) != 1 {
		t.Fatalf("%v %s", err, raw)
	}
	m := res.Mentions[0]
	req := common.MentionLinkRequest{Filename: paths["kubernetes.org"],
		Mentions: []common.MentionRef{{Filename: m.Filename, Line: m.Line, Col: m.Col, Text: m.Text}}}
	_, raw = call(t, PostLinkMentions, "POST", "/mentions/link", nil, req)
	var lr common.MentionLinkResult
	if err := json.Unmarshal(raw, &lr); err != nil || lr.Linked != 1 {
		t.Fatalf("%v %s", err, raw)
	}
	wantOrg(t, "ops.org", readOrg(t, paths["ops.org"]), "* Cluster\nWe run [[file:./kubernetes.org][Kubernetes]] everywhere.")
	wantOrg(t, "kubernetes.org", readOrg(t, paths["kubernetes.org"]), "* Pods")
}
