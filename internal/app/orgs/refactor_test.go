package orgs

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

// The refactors are tested through their handlers: a preview must write
// nothing, an apply with the preview's token must write exactly the preview's
// After text, and an apply after a file changed must refuse and write nothing.

func refactorCall[T any](t *testing.T, h func(w http.ResponseWriter, r *http.Request), path string, req T) common.RefactorPlan {
	t.Helper()
	_, raw := call(t, h, "POST", path, nil, req)
	var res common.RefactorPlan
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("%s: %v %s", path, err, raw)
	}
	return res
}

// orgDirsFor points the org directories at a test's folder, for the
// refactors that make files and must stay inside them.
func orgDirsFor(t *testing.T, paths map[string]string) string {
	t.Helper()
	var dir string
	for _, p := range paths {
		dir = filepath.Dir(p)
	}
	old := Conf().Server.OrgDirs
	Conf().Server.OrgDirs = []string{dir}
	t.Cleanup(func() { Conf().Server.OrgDirs = old })
	return dir
}

func fileOf(res common.RefactorPlan, path string) *common.RefactorFile {
	for i := range res.Files {
		if res.Files[i].Filename == path {
			return &res.Files[i]
		}
	}
	return nil
}

// Every file a preview names, written, is the preview's After - and no file it
// does not name has changed.
func wantApplied(t *testing.T, preview common.RefactorPlan, before map[string]string, paths map[string]string) {
	t.Helper()
	for _, f := range preview.Files {
		got, err := os.ReadFile(f.Filename)
		if err != nil {
			t.Fatalf("%s: %v", f.Filename, err)
		}
		if string(got) != f.After {
			t.Errorf("%s written:\n%s\nwant\n%s", f.Filename, got, f.After)
		}
	}
	for name, p := range paths {
		if fileOf(preview, p) != nil {
			continue
		}
		if got := readOrg(t, p); got != strings.TrimRight(before[name], "\n") {
			t.Errorf("%s changed though the preview did not name it:\n%s", name, got)
		}
	}
}

func readAll(t *testing.T, paths map[string]string) map[string]string {
	out := map[string]string{}
	for name, p := range paths {
		b, _ := os.ReadFile(p)
		out[name] = string(b)
	}
	return out
}

const tagIndented = `#+TITLE: Work
#+FILETAGS: :work:office:
#+TAGS: { @office(o) @home(h) } work(w) laptop

* TODO Plan the launch                                         :work:urgent:
  :PROPERTIES:
  :ID:       aaa
  :END:
  Some text mentioning :work: that is not a tag line.
** Sub task                                                    :urgent:job:work:
#+begin_src org
,* Example :work:
#+TAGS: work
#+end_src
* Done thing :job:
`

const tagColumnZero = `#+FILETAGS: work
* Home heading :work:
:PROPERTIES:
:ID: bbb
:END:
* Untouched :other:
`

func TestRefactorTagRename(t *testing.T) {
	paths := loadOrg(t, map[string]string{"work.org": tagIndented, "home.org": tagColumnZero})
	before := readAll(t, paths)
	req := common.TagRenameRequest{Old: "work", New: "job"}
	res := refactorCall(t, PostTagRenamePreview, "/refactor/tag/preview", req)
	if !res.Ok || len(res.Files) != 2 || res.Token == "" {
		t.Fatalf("preview %+v", res)
	}
	if got := readAll(t, paths); got["work.org"] != before["work.org"] || got["home.org"] != before["home.org"] {
		t.Fatal("a preview wrote")
	}
	work := fileOf(res, paths["work.org"])
	want := strings.NewReplacer(
		"#+FILETAGS: :work:office:", "#+FILETAGS: :job:office:",
		"work(w) laptop", "job(w) laptop",
		"* TODO Plan the launch                                         :work:urgent:",
		"* TODO Plan the launch                                          :job:urgent:",
		// Already had :job:, so :work: goes rather than being written twice;
		// the tags still end at the column they ended at.
		"** Sub task                                                    :urgent:job:work:",
		"** Sub task                                                         :urgent:job:",
	).Replace(tagIndented)
	if work.After != want {
		t.Errorf("work.org:\n%s\nwant\n%s", work.After, want)
	}
	if !strings.Contains(work.Patch, "\n-#+FILETAGS: :work:office:\n") || !strings.Contains(work.Patch, "\n+#+FILETAGS: :job:office:\n") || work.Added != 4 || work.Removed != 4 {
		t.Errorf("patch %d+ %d-:\n%s", work.Added, work.Removed, work.Patch)
	}
	home := fileOf(res, paths["home.org"])
	wantHome := strings.Replace(strings.Replace(tagColumnZero, "#+FILETAGS: work", "#+FILETAGS: job", 1),
		"* Home heading :work:", "* Home heading :job:", 1)
	if home.After != wantHome {
		t.Errorf("home.org:\n%s", home.After)
	}
	if len(res.Warnings) == 0 || !strings.Contains(res.Warnings[0], "already had :job:") {
		t.Errorf("warnings %v", res.Warnings)
	}

	// An apply without the token is refused.
	if bad := refactorCall(t, PostTagRenameApply, "/refactor/tag/apply", req); bad.Ok {
		t.Errorf("apply without a token: %+v", bad)
	}
	req.Token = res.Token
	done := refactorCall(t, PostTagRenameApply, "/refactor/tag/apply", req)
	if !done.Ok || !done.Applied {
		t.Fatalf("apply %+v", done)
	}
	wantApplied(t, res, before, paths)

	// The database reads the new tag straight away.
	sec := findHeadingByText(GetDb().FindByFile(paths["home.org"]), "Home heading")
	if sec == nil || strings.Join(sec.Headline.Tags, ",") != "job" {
		t.Errorf("reloaded tags %+v", sec)
	}
}

func TestRefactorTagApplyRefusesAStalePreview(t *testing.T) {
	paths := loadOrg(t, map[string]string{"home.org": tagColumnZero})
	req := common.TagRenameRequest{Old: "work", New: "job"}
	res := refactorCall(t, PostTagRenamePreview, "/refactor/tag/preview", req)
	if !res.Ok || len(res.Files) != 1 {
		t.Fatalf("preview %+v", res)
	}
	// Somebody edits the file between the preview and the apply.
	edited := tagColumnZero + "* Added by hand :work:\n"
	os.WriteFile(paths["home.org"], []byte(edited), 0644)
	req.Token = res.Token
	req.Expect = map[string]string{res.Files[0].Filename: res.Files[0].Hash}
	stale := refactorCall(t, PostTagRenameApply, "/refactor/tag/apply", req)
	if stale.Ok || !stale.Stale || !strings.Contains(stale.Msg, "home.org") {
		t.Errorf("stale apply %+v", stale)
	}
	if got := readOrg(t, paths["home.org"]); got != strings.TrimRight(edited, "\n") {
		t.Errorf("a stale apply wrote:\n%s", got)
	}
}

func TestRefactorTagDeleteAndQueryScope(t *testing.T) {
	paths := loadOrg(t, map[string]string{"work.org": tagIndented})
	// Delete: off every headline, out of FILETAGS and #+TAGS.
	res := refactorCall(t, PostTagRenamePreview, "/refactor/tag/preview", common.TagRenameRequest{Old: "urgent", Delete: true})
	f := fileOf(res, paths["work.org"])
	if f == nil || strings.Contains(strings.Split(f.After, "#+begin_src")[0], "urgent") {
		t.Fatalf("delete %+v", res)
	}
	if !strings.Contains(f.After, "* TODO Plan the launch                                                :work:") {
		t.Errorf("right-aligned after delete:\n%s", f.After)
	}

	// A FILETAGS line left with nothing goes.
	res = refactorCall(t, PostTagRenamePreview, "/refactor/tag/preview", common.TagRenameRequest{Old: "office", Delete: true})
	if f = fileOf(res, paths["work.org"]); f == nil || !strings.Contains(f.After, "#+FILETAGS: :work:\n") {
		t.Errorf("filetags:\n%v", res)
	}

	// A query: only the headlines it matched, never the file's keyword lines.
	res = refactorCall(t, PostTagRenamePreview, "/refactor/tag/preview",
		common.TagRenameRequest{Old: "work", New: "career", Query: `MatchHeadline("Sub task")`})
	f = fileOf(res, paths["work.org"])
	if f == nil || f.Added != 1 || !strings.Contains(f.After, ":urgent:job:career:") || !strings.Contains(f.After, "#+FILETAGS: :work:office:") {
		t.Errorf("query scope %+v", res)
	}
}

func TestRenameTagsLine(t *testing.T) {
	got, hit, _ := renameTagsLine("#+TAGS: { @work(w) @home(h) } laptop", "@home", "@house")
	if !hit || got != "#+TAGS: { @work(w) @house(h) } laptop" {
		t.Errorf("rename %q", got)
	}
	got, _, merged := renameTagsLine("#+TAGS: a b(x)  c", "a", "c")
	if !merged || got != "#+TAGS: b(x)  c" {
		t.Errorf("merge %q", got)
	}
}

const splitSource = `#+TITLE: Projects
#+TODO: TODO NEXT | DONE
#+FILETAGS: :proj:

* Area                                                                  :area:
** NEXT Apollo :space:
:PROPERTIES:
:OWNER: me
:END:
Text of Apollo, see [[file:./img/moon.png]] and [[*Gemini]].
*** DONE Launch
CLOSED: [2026-01-01 Thu]
Launched.
*** Landing
** Gemini

* Other
`

func TestRefactorSplitHeading(t *testing.T) {
	paths := loadOrg(t, map[string]string{
		"projects.org": splitSource,
		"refs.org":     "* Refs\nSee [[file:projects.org::*Apollo][Apollo]].\n",
	})
	dir := orgDirsFor(t, paths)
	before := readAll(t, paths)
	sec := findHeadingByText(GetDb().FindByFile(paths["projects.org"]), "Apollo")
	if sec == nil {
		t.Fatal("no Apollo")
	}
	req := common.SplitRequest{Hash: sec.Hash, To: "space/"}
	os.MkdirAll(filepath.Join(dir, "space"), 0755)
	res := refactorCall(t, PostSplitPreview, "/refactor/split/preview", req)
	dest := filepath.Join(dir, "space", "apollo.org")
	if !res.Ok || res.NewFile != dest || res.NewId == "" || len(res.Files) != 2 || !res.Files[0].Created {
		t.Fatalf("preview %+v", res)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("a preview made the file")
	}
	id := res.NewId
	wantNew := `#+TITLE: Apollo
#+TODO: TODO NEXT | DONE
#+FILETAGS: :proj:

* NEXT Apollo :space:
:PROPERTIES:
:OWNER: me
:ID:    ` + id + `
:END:
Text of Apollo, see [[file:../img/moon.png]] and [[*Gemini]].
** DONE Launch
CLOSED: [2026-01-01 Thu]
Launched.
** Landing
`
	if res.Files[0].After != wantNew {
		t.Errorf("new file:\n%s\nwant\n%s", res.Files[0].After, wantNew)
	}
	wantSrc := strings.Replace(splitSource, `** NEXT Apollo :space:
:PROPERTIES:
:OWNER: me
:END:
Text of Apollo, see [[file:./img/moon.png]] and [[*Gemini]].
*** DONE Launch
CLOSED: [2026-01-01 Thu]
Launched.
*** Landing
`, "** [[id:"+id+"][Apollo]]\n", 1)
	if f := fileOf(res, paths["projects.org"]); f == nil || f.After != wantSrc {
		t.Errorf("source:\n%v", f)
	}
	joined := strings.Join(res.Warnings, "\n")
	if !strings.Contains(joined, ":area:") || !strings.Contains(joined, "*Gemini") {
		t.Errorf("warnings %v", res.Warnings)
	}

	// The apply must carry the id the preview made, or it is a different
	// change - and is refused as one.
	req.Token = res.Token
	if again := refactorCall(t, PostSplitApply, "/refactor/split/apply", req); again.Ok || !again.Stale {
		t.Errorf("apply without NewId %+v", again)
	}
	req.NewId = id
	done := refactorCall(t, PostSplitApply, "/refactor/split/apply", req)
	if !done.Ok || !done.Applied {
		t.Fatalf("apply %+v", done)
	}
	wantApplied(t, res, before, paths)

	// The id resolves to the heading in its new file.
	moved := GetDb().FindByAnyId(id)
	if moved == nil || GetDb().FileFromSection(moved) == nil || GetDb().FileFromSection(moved).Filename != dest {
		t.Errorf("id %s does not land in %s", id, dest)
	}

	// Doing it again is refused: the file is there now.
	sec2 := findHeadingByText(GetDb().FindByFile(paths["projects.org"]), "Gemini")
	if r := refactorCall(t, PostSplitPreview, "/refactor/split/preview", common.SplitRequest{Hash: sec2.Hash, To: "space/apollo.org"}); r.Ok || !strings.Contains(r.Msg, "already exists") {
		t.Errorf("overwrite %+v", r)
	}
	if r := refactorCall(t, PostSplitPreview, "/refactor/split/preview", common.SplitRequest{Hash: sec2.Hash, To: "/tmp/elsewhere.org"}); r.Ok || !strings.Contains(r.Msg, "not inside") {
		t.Errorf("outside %+v", r)
	}
}

const splitIndented = `* Top
** Keep
   :PROPERTIES:
   :ID:       keep-0001
   :END:
   Body of keep.
   #+begin_src python
   def f():
       return 1
   #+end_src
*** Child
    Child text.
** After
`

// An indented file: the text moves left with its heading, by the same amount
// everywhere, and an existing id is kept rather than replaced.
func TestRefactorSplitIndentedKeepsItsId(t *testing.T) {
	paths := loadOrg(t, map[string]string{"top.org": splitIndented,
		"links.org": "* L\n[[id:keep-0001][the keep]]\n"})
	orgDirsFor(t, paths)
	sec := findHeadingByText(GetDb().FindByFile(paths["top.org"]), "Keep")
	res := refactorCall(t, PostSplitPreview, "/refactor/split/preview", common.SplitRequest{Hash: sec.Hash})
	if !res.Ok || res.NewId != "" {
		t.Fatalf("preview %+v", res)
	}
	want := `#+TITLE: Keep

* Keep
  :PROPERTIES:
  :ID:       keep-0001
  :END:
  Body of keep.
  #+begin_src python
  def f():
      return 1
  #+end_src
** Child
   Child text.
`
	if res.Files[0].After != want {
		t.Errorf("new file:\n%s", res.Files[0].After)
	}
	if f := fileOf(res, paths["top.org"]); f == nil || f.After != "* Top\n** [[id:keep-0001][Keep]]\n** After\n" {
		t.Errorf("source %v", f)
	}
	done := refactorCall(t, PostSplitApply, "/refactor/split/apply", common.SplitRequest{Hash: sec.Hash,
		RefactorApply: common.RefactorApply{Token: res.Token}})
	if !done.Ok {
		t.Fatalf("apply %+v", done)
	}
	moved := GetDb().FindByAnyId("keep-0001")
	if moved == nil || !strings.HasSuffix(GetDb().FileFromSection(moved).Filename, "keep.org") {
		t.Error("the existing id does not land in keep.org")
	}
}

const mergeIndented = `* Projects
** Rocket                                                   :space:
   :PROPERTIES:
   :ID:       rocket-id
   :OWNER:    ann
   :END:
   Rocket body.
*** Fuel
** Other
*** Booster                                                  :engine:
    SCHEDULED: <2026-02-01 Sun>
    :PROPERTIES:
    :ID:       booster-id
    :OWNER:    bob
    :COST:     10
    :END:
    :LOGBOOK:
    - Note taken on [2026-01-01 Thu 10:00]
    :END:
    Booster body.
**** Nozzle
     Nozzle text.

* Tail
`

func TestRefactorMergeHeadings(t *testing.T) {
	paths := loadOrg(t, map[string]string{
		"m.org":     mergeIndented,
		"links.org": "* Links\n[[id:booster-id][the booster]] and [[id:rocket-id]].\n",
	})
	before := readAll(t, paths)
	f := GetDb().FindByFile(paths["m.org"])
	a, b := findHeadingByText(f, "Rocket"), findHeadingByText(f, "Booster")
	req := common.MergeRequest{Into: a.Hash, From: b.Hash}
	res := refactorCall(t, PostMergePreview, "/refactor/merge/preview", req)
	if !res.Ok || len(res.Files) != 2 {
		t.Fatalf("preview %+v", res)
	}
	want := `* Projects
** Rocket                                            :space:engine:
   SCHEDULED: <2026-02-01 Sun>
   :PROPERTIES:
   :ID:       rocket-id
   :OWNER:    ann
   :COST:     10
   :MERGED_IDS: booster-id
   :END:
   :LOGBOOK:
   - Note taken on [2026-01-01 Thu 10:00]
   :END:
   Rocket body.

   Booster body.
*** Fuel
*** Nozzle
    Nozzle text.
** Other

* Tail
`
	if got := fileOf(res, paths["m.org"]).After; got != want {
		t.Errorf("m.org:\n%s\nwant\n%s", got, want)
	}
	if got := fileOf(res, paths["links.org"]).After; got != "* Links\n[[id:rocket-id][the booster]] and [[id:rocket-id]].\n" {
		t.Errorf("links.org:\n%s", got)
	}
	if j := strings.Join(res.Warnings, "\n"); !strings.Contains(j, `OWNER (kept "ann", dropped "bob")`) {
		t.Errorf("warnings %v", res.Warnings)
	}
	req.Token = res.Token
	done := refactorCall(t, PostMergeApply, "/refactor/merge/apply", req)
	if !done.Ok {
		t.Fatalf("apply %+v", done)
	}
	wantApplied(t, res, before, paths)
}

// Column zero, Into with no id: it takes From's, and no link has to change.
func TestRefactorMergeTakesTheId(t *testing.T) {
	text := `* Inbox
:PROPERTIES:
:CATEGORY: in
:END:
First.
* Later :x:
:PROPERTIES:
:ID: later-id
:END:
Second.
* End
`
	paths := loadOrg(t, map[string]string{"c.org": text, "l.org": "* L\n[[id:later-id]]\n"})
	f := GetDb().FindByFile(paths["c.org"])
	a, b := findHeadingByText(f, "Inbox"), findHeadingByText(f, "Later")
	res := refactorCall(t, PostMergePreview, "/refactor/merge/preview", common.MergeRequest{Into: a.Hash, From: b.Hash})
	if !res.Ok || len(res.Files) != 1 {
		t.Fatalf("preview %+v", res)
	}
	want := `* Inbox  :x:
:PROPERTIES:
:CATEGORY: in
:ID:       later-id
:END:
First.

Second.
* End
`
	if res.Files[0].After != want {
		t.Errorf("c.org:\n%s", res.Files[0].After)
	}
	// Merging a heading into one inside it is refused.
	child := `* Parent
** Kid
`
	paths = loadOrg(t, map[string]string{"p.org": child})
	f = GetDb().FindByFile(paths["p.org"])
	r := refactorCall(t, PostMergePreview, "/refactor/merge/preview",
		common.MergeRequest{Into: findHeadingByText(f, "Kid").Hash, From: findHeadingByText(f, "Parent").Hash})
	if r.Ok || !strings.Contains(r.Msg, "inside") {
		t.Errorf("into own child %+v", r)
	}
}

const listSource = `#+TODO: TASK | FIXED
* Shopping
Things to get:
- [ ] Milk
  semi-skimmed
- [X] Bread
  - [ ] Rye
- Eggs :: a dozen

After the list.
** Existing child
* Next
`

func TestRefactorListToHeadings(t *testing.T) {
	paths := loadOrg(t, map[string]string{"s.org": listSource})
	before := readAll(t, paths)
	sec := findHeadingByText(GetDb().FindByFile(paths["s.org"]), "Shopping")

	_, raw := call(t, RequestRefactorLists, "GET", "/refactor/lists?hash="+url.QueryEscape(sec.Hash), nil, nil)
	var lists common.RefactorLists
	json.Unmarshal(raw, &lists)
	if !lists.Ok || len(lists.Lists) != 1 || lists.Lists[0].Items != 4 || lists.Lists[0].Checkboxes != 3 || lists.Lists[0].Line != 3 {
		t.Fatalf("lists %s", raw)
	}

	req := common.ListToHeadingsRequest{Hash: sec.Hash, Line: lists.Lists[0].Line + 2}
	res := refactorCall(t, PostListToHeadingsPreview, "/refactor/list/preview", req)
	want := `#+TODO: TASK | FIXED
* Shopping
Things to get:

After the list.
** TASK Milk
semi-skimmed
** FIXED Bread
*** TASK Rye
** Eggs
a dozen
** Existing child
* Next
`
	if !res.Ok || res.Files[0].After != want {
		t.Fatalf("list:\n%s\n%+v", res.Files[0].After, res)
	}
	if len(res.Warnings) == 0 {
		t.Error("no warning about the text after the list")
	}
	req.Token = res.Token
	if done := refactorCall(t, PostListToHeadingsApply, "/refactor/list/apply", req); !done.Ok {
		t.Fatalf("apply %+v", done)
	}
	wantApplied(t, res, before, paths)
}

func TestRefactorListToHeadingsIndented(t *testing.T) {
	text := `* Plan
  - [ ] one
    more about one
  - [x] two
** Child
`
	paths := loadOrg(t, map[string]string{"i.org": text})
	sec := findHeadingByText(GetDb().FindByFile(paths["i.org"]), "Plan")
	res := refactorCall(t, PostListToHeadingsPreview, "/refactor/list/preview", common.ListToHeadingsRequest{Hash: sec.Hash})
	want := `* Plan
** TODO one
   more about one
** DONE two
** Child
`
	if !res.Ok || res.Files[0].After != want {
		t.Errorf("indented:\n%s\n%+v", res.Files[0].After, res)
	}
	// Nothing followed the list, so there is nothing to warn about.
	if len(res.Warnings) != 0 {
		t.Errorf("warnings %v", res.Warnings)
	}
}

func TestUnifiedPatch(t *testing.T) {
	a := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	b := []string{"a", "B", "c", "d", "e", "f", "g", "h", "i", "j", "k"}
	p, add, del := unifiedPatch("x.org", a, b, false)
	want := "--- a/x.org\n+++ b/x.org\n@@ -1,5 +1,5 @@\n a\n-b\n+B\n c\n d\n e\n@@ -8,3 +8,4 @@\n h\n i\n j\n+k\n"
	if p != want || add != 2 || del != 1 {
		t.Errorf("patch %d %d:\n%s", add, del, p)
	}
	if ops := lineDiff([]string{"x", "y"}, []string{"y", "x"}); len(ops) != 3 {
		t.Errorf("swap %+v", ops)
	}
}

// The edit script rebuilds both sides, for any two texts: a patch that drops
// or repeats a line would show somebody a change that is not the one written.
func TestLineDiffRebuildsBothSides(t *testing.T) {
	seed := uint32(7)
	rnd := func(n int) int {
		seed = seed*1664525 + 1013904223
		return int(seed>>16) % n
	}
	for round := 0; round < 400; round++ {
		gen := func() []string {
			out := []string{}
			for i := rnd(14); i > 0; i-- {
				out = append(out, string(rune('a'+rnd(4))))
			}
			return out
		}
		a, b := gen(), gen()
		var ga, gb []string
		for _, op := range lineDiff(a, b) {
			switch op.kind {
			case ' ':
				if a[op.a] != b[op.b] {
					t.Fatalf("%v %v: kept %q against %q", a, b, a[op.a], b[op.b])
				}
				ga, gb = append(ga, a[op.a]), append(gb, b[op.b])
			case '-':
				ga = append(ga, a[op.a])
			case '+':
				gb = append(gb, b[op.b])
			}
		}
		if strings.Join(ga, "") != strings.Join(a, "") || strings.Join(gb, "") != strings.Join(b, "") {
			t.Fatalf("%v -> %v rebuilt as %v -> %v", a, b, ga, gb)
		}
	}
}
