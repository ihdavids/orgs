package orgs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func TestRelatedStem(t *testing.T) {
	for in, want := range map[string]string{
		"notes": "note", "note": "note", "indexing": "index", "indexed": "index", "index": "index",
		"running": "run", "libraries": "library", "classes": "class", "class": "class",
		"status": "status", "analysis": "analysis", "kubernetes": "kubernete", "planned": "plan",
	} {
		if got := relStem(in); got != want {
			t.Errorf("relStem(%q) = %q, want %q", in, got, want)
		}
	}
	var got []string
	relWords("The node's drain, and 3 nodes: it's 2026; Zürich-based draining.", func(stem, _ string) { got = append(got, stem) })
	if strings.Join(got, " ") != "node drain node zürich-bas drain" {
		t.Errorf("words %q", got)
	}
	if p := relProse("See [[https://k8s.io/docs][the docs]] at https://x.org <2026-11-04 Wed> =code=."); strings.Contains(p, "k8s") || strings.Contains(p, "x.org") || !strings.Contains(p, "the docs") || strings.Contains(p, "code") {
		t.Errorf("prose %q", p)
	}
}

const relatedOps = `#+TITLE: Operations
* Kubernetes
** Upgrading the cluster
We drain each kubernetes node, cordon it, then upgrade the kubelet.
Draining nodes needs the pod disruption budget checked first.
:PROPERTIES:
:NOTE: kubelet kubelet kubelet
:END:
#+begin_src sh
kubectl drain kubelet kubelet
#+end_src
** Child of nothing
Sourdough bread with a long rise.
* Garden
Tomatoes and basil in the raised beds.
`

const relatedOther = `* Node maintenance
Before a kubelet upgrade, drain the node and cordon it so pods move.
The disruption budget must allow it.
* Baking
Sourdough needs a starter, flour and a long rise overnight.
* Linked already
See [[file:ops.org::*Upgrading the cluster][upgrading]]. Drain cordon kubelet node upgrade.
`

func TestRelatedWordsThroughTheApi(t *testing.T) {
	paths := loadOrg(t, map[string]string{"ops.org": relatedOps, "other.org": relatedOther})
	was := Conf().Server.Features.Related
	Conf().Server.Features.Related = true
	defer func() { Conf().Server.Features.Related = was }()

	sec := findHeadingByText(GetDb().FindByFile(paths["ops.org"]), "Upgrading the cluster")
	if sec == nil {
		t.Fatal("no heading")
	}
	get := func(u string) common.RelatedResult {
		t.Helper()
		_, raw := call(t, RequestRelated, "GET", u, nil, nil)
		var res common.RelatedResult
		if err := json.Unmarshal(raw, &res); err != nil || !res.Ok {
			t.Fatalf("%s: %v %s", u, err, raw)
		}
		return res
	}
	res := get("/related?hash=" + url.QueryEscape(sec.Hash) + "&engine=words")
	if res.Engine != "words" || len(res.Items) == 0 {
		t.Fatalf("no answer %+v", res)
	}
	top := res.Items[0]
	if top.Headline != "Node maintenance" || top.Score <= 0 || top.Score > 1 {
		t.Fatalf("top %+v", res.Items)
	}
	if !strings.Contains(strings.Join(top.Terms, " "), "drain") || !strings.Contains(top.Snippet, "drain") {
		t.Errorf("why %+v", top)
	}
	for _, it := range res.Items {
		switch it.Headline {
		case "Upgrading the cluster", "Kubernetes":
			t.Errorf("itself or its ancestor came back: %+v", it)
		case "Linked already":
			t.Errorf("an already linked heading came back: %+v", it)
		}
	}
	if res.Skipped != 1 {
		t.Errorf("skipped %d", res.Skipped)
	}
	// With linked=1 it comes back, marked.
	found := false
	for _, it := range get("/related?hash=" + url.QueryEscape(sec.Hash) + "&engine=words&linked=1").Items {
		if it.Headline == "Linked already" {
			found = it.Linked
		}
	}
	if !found {
		t.Errorf("linked heading not kept and marked")
	}
	// Drawers and blocks are not prose: "kubelet" only counts its body uses.
	ix := getRelIndex()
	for _, p := range ix.parts {
		for _, c := range p.chunks {
			if c.title == "Upgrading the cluster" && c.tf["kubelet"] != 1 {
				t.Errorf("kubelet counted %d times", c.tf["kubelet"])
			}
		}
	}

	// Unsaved text.
	_, raw := call(t, PostRelatedText, "POST", "/related/text", nil, common.RelatedTextRequest{Text: "* Bread\nA sourdough starter and a long rise.", Filename: paths["ops.org"], Line: -1})
	var tr common.RelatedResult
	if err := json.Unmarshal(raw, &tr); err != nil || !tr.Ok || len(tr.Items) == 0 || tr.Items[0].Headline != "Baking" {
		t.Fatalf("text: %v %s", err, raw)
	}
	for _, it := range tr.Items {
		if it.Filename == paths["ops.org"] {
			t.Errorf("the file itself came back: %+v", it)
		}
	}

	// Link it: the target gets an id, the source a Related list.
	_, raw = call(t, PostRelatedLink, "POST", "/related/link", nil, common.RelatedLinkRequest{Hash: sec.Hash, TargetHash: top.Hash})
	var lr common.RelatedLinkResult
	if err := json.Unmarshal(raw, &lr); err != nil || !lr.Ok || !lr.Minted {
		t.Fatalf("link: %v %s", err, raw)
	}
	ops := readOrg(t, paths["ops.org"])
	if !strings.Contains(ops, "#+end_src\n\nRelated:\n- "+lr.Link+"\n** Child of nothing") {
		t.Errorf("ops.org:\n%s", ops)
	}
	if !strings.Contains(readOrg(t, paths["other.org"]), ":ID:") {
		t.Errorf("no id on the target")
	}
	// A second one joins the list; the same one again is not written twice.
	baking := findHeadingByText(GetDb().FindByFile(paths["other.org"]), "Baking")
	sec = findHeadingByText(GetDb().FindByFile(paths["ops.org"]), "Upgrading the cluster")
	call(t, PostRelatedLink, "POST", "/related/link", nil, common.RelatedLinkRequest{Hash: sec.Hash, TargetHash: baking.Hash})
	ops = readOrg(t, paths["ops.org"])
	if strings.Count(ops, "Related:") != 1 || !strings.Contains(ops, "\n- [[id:") || strings.Count(ops, "\n- [[id:") != 2 {
		t.Errorf("second link:\n%s", ops)
	}
	_, raw = call(t, PostRelatedLink, "POST", "/related/link", nil, common.RelatedLinkRequest{Hash: sec.Hash, TargetHash: baking.Hash})
	if !strings.Contains(string(raw), "already linked") || readOrg(t, paths["ops.org"]) != ops {
		t.Errorf("twice: %s", raw)
	}
	// And now it is linked, it is left out.
	for _, it := range get("/related?hash=" + url.QueryEscape(sec.Hash) + "&engine=words").Items {
		if it.Headline == "Node maintenance" {
			t.Errorf("linked result still offered")
		}
	}

	// Pairs: the two baking headings are alike and unlinked.
	_, raw = call(t, RequestRelatedPairs, "GET", "/related/pairs", nil, nil)
	if !strings.Contains(string(raw), "Sourdough") && !strings.Contains(string(raw), "sourdough") {
		t.Errorf("pairs %s", raw)
	}
}

func TestRelatedInsertLink(t *testing.T) {
	cases := []struct{ in, want string }{
		{"* A", "* A\nRelated:\n- [[id:x][X]]"},
		{"* A\n  Some text.\n\n* B", "* A\n  Some text.\n\n  Related:\n  - [[id:x][X]]\n\n* B"},
		{"* A\nText\nRelated:\n- [[id:y][Y]]\n\nMore", "* A\nText\nRelated:\n- [[id:y][Y]]\n- [[id:x][X]]\n\nMore"},
	}
	for _, c := range cases {
		lines := strings.Split(c.in, "\n")
		end := ownLinesEnd(lines, 0, len(lines)-1)
		out, _, err := relInsertLink(lines, 0, end, "[[id:x][X]]", "id:x")
		if err != nil || strings.Join(out, "\n") != c.want {
			t.Errorf("%q:\n got  %q (%v)\n want %q", c.in, strings.Join(out, "\n"), err, c.want)
		}
	}
}

// A fake OpenAI-compatible embeddings server: a word's vector is a hashed
// bag of words, so texts sharing words are alike.
func fakeEmbeddings(t *testing.T) (*httptest.Server, *int) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/embeddings" || r.Header.Get("Authorization") != "Bearer sekrit" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error": {"message": "bad request ` + r.URL.Path + `"}}`))
			return
		}
		var req struct {
			Model string
			Input []string
		}
		json.NewDecoder(r.Body).Decode(&req)
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		out := struct {
			Data []item `json:"data"`
		}{}
		for i, text := range req.Input {
			v := make([]float32, 32)
			relWords(text, func(stem, _ string) {
				h := 0
				for _, c := range stem {
					h = h*31 + int(c)
				}
				v[(h%32+32)%32]++
			})
			out.Data = append(out.Data, item{i, v})
		}
		json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestRelatedEmbeddings(t *testing.T) {
	paths := loadOrg(t, map[string]string{"ops.org": relatedOps, "other.org": relatedOther})
	srv, calls := fakeEmbeddings(t)
	s := Conf().Server
	was, wasRel, wasCfg := s.Features.Related, s.Related, Conf().Config
	s.Features.Related = true
	s.Related = common.RelatedSettings{Embeddings: common.RelatedEmbeddingsSettings{Backend: "openai", Model: "fake", Url: srv.URL + "/v1", ApiKey: "sekrit", Batch: 4}}
	Conf().Config = filepath.Join(t.TempDir(), "orgs.yaml")
	defer func() { s.Features.Related, s.Related, Conf().Config = was, wasRel, wasCfg }()
	relEmb.mu.Lock()
	relEmb.vecs, relEmb.loaded, relEmb.keysIx = map[string][]float32{}, true, nil
	relEmb.mu.Unlock()

	// Embed everything the way the loop does.
	ix := getRelIndex()
	keys := relEmb.keysFor(ix)
	for d, k := range keys {
		if k == "" {
			continue
		}
		vs, err := relEmbed(context.Background(), []string{relEmbedText(ix, d)})
		if err != nil {
			t.Fatal(err)
		}
		relEmb.vecs[k] = relNormalise(vs[0])
	}
	relEmb.dirty = true
	relEmb.save(keys)
	if _, err := os.Stat(relStorePath()); err != nil {
		t.Errorf("no store: %v", err)
	}
	st := relEmbedProgress()
	if st.Total == 0 || st.Done != st.Total {
		t.Errorf("progress %+v", st)
	}
	if e, _ := relEngine(""); e != "embeddings" {
		t.Errorf("auto engine %s", e)
	}

	before := *calls
	sec := findHeadingByText(GetDb().FindByFile(paths["ops.org"]), "Upgrading the cluster")
	res := QueryRelated(GetDb().FindByFile(paths["ops.org"]), sec, 5, "", false)
	if res.Engine != "embeddings" || len(res.Items) == 0 || res.Items[0].Headline != "Node maintenance" || res.Items[0].Embed == 0 {
		t.Fatalf("embeddings answer %+v", res)
	}
	if *calls != before {
		t.Errorf("a saved heading's question asked the model")
	}
	// Unsaved text is embedded on demand, once.
	ix = getRelIndex()
	a := ix.askText("* Bread\nsourdough starter flour rise", "", -1)
	r1 := ix.answer(a, 3, "embeddings", false)
	ix.answer(a, 3, "embeddings", false)
	if r1.Items[0].Headline != "Baking" || *calls != before+1 {
		t.Errorf("text %+v calls %d", r1.Items, *calls-before)
	}
	// A refusal is said, and the words engine answers.
	s.Related.Embeddings.ApiKey = "wrong"
	a = ix.askText("* Something new entirely\nquantum gardening", "", -1)
	r2 := ix.answer(a, 3, "embeddings", false)
	if r2.Engine != "words" || !strings.Contains(r2.Msg, "bad request") {
		t.Errorf("fallback %+v", r2)
	}
}
