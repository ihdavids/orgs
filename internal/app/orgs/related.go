package orgs

// Related notes (features.related): the headings most like the one being read.
//
// Two engines. **Words** needs nothing: every heading's own prose (title and
// body up to its first child, read the way mentions.go reads prose - no
// drawers, planning, src/example blocks, link targets, urls, timestamps) is
// a bag of lightly stemmed words without stop words, weighted BM25-style
// (saturated term frequency over idf, length normalised) and compared by
// cosine, so a score is 0-1 and can be blended. **Embeddings**
// (relatedembed.go) asks a model for a vector per heading, kept on disk by a
// hash of the text.
//
// A heading asks with its subtree's words (or vectors), a file with all of
// its own. Left out of the answer: the heading itself, its ancestors and
// descendants (a file: the whole file), and what a link already joins to it
// either way (kept with linked=1, marked Linked).
//
// Per file through FileParts (one save rebuilds one file), the corpus-wide
// numbers (idf, postings, norms) once per OrgDb.ReloadIndex.

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/go-org/org"
)

func registerRelatedRoutes(api *mux.Router) {
	api.HandleFunc("/related", RequestRelated).Methods("GET")
	api.HandleFunc("/related/text", PostRelatedText).Methods("POST", "GET")
	api.HandleFunc("/related/status", RequestRelatedStatus).Methods("GET")
	api.HandleFunc("/related/reindex", PostRelatedReindex).Methods("POST")
	api.HandleFunc("/related/link", PostRelatedLink).Methods("POST")
	api.HandleFunc("/related/pairs", RequestRelatedPairs).Methods("GET")
	startRelated()
}

// ---------------------------------------------------------------------------
// Words
// ---------------------------------------------------------------------------

// The words that say nothing about what a note is about.
var relStopWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a about above after again against all also am an and any are aren't as at be
	because been before being below between both but by can can't cannot could couldn't did didn't do does doesn't
	doing don't down during each else even ever every few for from further get gets got had hadn't has hasn't have
	haven't having he her here hers herself him himself his how however i if in into is isn't it it's its itself
	just let let's like made make many may me might more most much must my myself need no nor not now of off on once
	one only or other ought our ours ourselves out over own per quite rather really same say says shall she should
	shouldn't since so some still such than that that's the their theirs them themselves then there there's these they
	this those though through thus to too under until up upon us use used using very via was wasn't way we well were
	weren't what what's when where which while who whom whose why will with within without won't would wouldn't yes yet
	you your yours yourself yourselves etc eg ie todo done next wait waiting someday`) {
		relStopWords[w] = true
	}
}

// relStem is a light stemmer: plurals and the commonest endings, so "notes"
// meets "note" and "indexing" meets "index", and nothing cleverer. Both
// sides of every comparison go through it, so being consistent matters more
// than being right.
func relStem(w string) string {
	n := len(w)
	switch {
	case n > 4 && strings.HasSuffix(w, "ies"):
		return w[:n-3] + "y"
	case n > 4 && strings.HasSuffix(w, "sses"):
		return w[:n-2]
	case n > 5 && strings.HasSuffix(w, "ing"):
		w = w[:n-3]
	case n > 4 && strings.HasSuffix(w, "ed") && !strings.HasSuffix(w, "eed"):
		w = w[:n-2]
	case n > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && !strings.HasSuffix(w, "us") && !strings.HasSuffix(w, "is"):
		return w[:n-1]
	default:
		return w
	}
	// After -ing / -ed: "running" -> "runn" -> "run", "indexed" -> "index".
	if m := len(w); m > 2 && w[m-1] == w[m-2] && !strings.ContainsRune("lsz", rune(w[m-1])) {
		w = w[:m-1]
	}
	return w
}

// relWords calls add for each word of s worth counting: lower case, stemmed,
// no stop words, no numbers, at least two letters.
func relWords(s string, add func(stem, surface string)) {
	start := -1
	flush := func(end int) {
		if start < 0 {
			return
		}
		w := s[start:end]
		start = -1
		w = strings.Trim(w, "'’-_")
		lw := strings.ToLower(w)
		lw = strings.TrimSuffix(strings.TrimSuffix(lw, "'s"), "’s")
		letters := 0
		for _, r := range lw {
			if unicode.IsLetter(r) {
				letters++
			}
		}
		if letters < 2 || relStopWords[lw] || len(lw) > 40 {
			return
		}
		st := relStem(lw)
		if relStopWords[st] {
			return
		}
		add(st, lw)
	}
	for i, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || ((r == '\'' || r == '’' || r == '-' || r == '_') && start >= 0) {
			if start < 0 {
				start = i
			}
			continue
		}
		flush(i)
	}
	flush(len(s))
}

// What is not prose inside a prose line, after links have become their
// description: mentions.go's list less the link itself.
func relProse(text string) string {
	text = linkText(text)
	for i, re := range mentionSkipRes {
		if i == 0 {
			continue
		}
		text = re.ReplaceAllString(text, " ")
	}
	for _, re := range mentionVerbatimRes {
		text = re.ReplaceAllString(text, " ")
	}
	return text
}

// ---------------------------------------------------------------------------
// One file's headings
// ---------------------------------------------------------------------------

// One heading's own text (its title and body up to its first child), or a
// file's preamble (row -1).
type relChunk struct {
	row    int // headline row, -1 for the preamble
	lvl    int
	end    int // last row of its own text
	subEnd int // last row of its subtree
	hash   string
	olp    []string // parents, outermost first
	title  string
	id     string
	tf     map[string]int32
	surf   map[string]string // stem -> the word as first written
	n      int               // words counted
	text   string            // its prose, for snippets and embedding
}

type relPart struct {
	file   string
	title  string // the file's #+TITLE, else its name
	chunks []relChunk
}

var relParts = NewFileParts[relPart]()

const relTextCap = 4000

func buildRelPart(f *common.OrgFile) relPart {
	p := relPart{file: f.Filename}
	lines, err := readOrgLines(f.Filename)
	if err != nil {
		return p
	}
	kw, props := filePreamble(lines)
	p.title = kw["TITLE"]
	if p.title == "" {
		base := strings.TrimSuffix(filepath.Base(f.Filename), filepath.Ext(f.Filename))
		p.title = strings.NewReplacer("-", " ", "_", " ").Replace(base)
	}
	scan := scanMentionLines(lines, mentionKeywords(lines))
	secs := map[int]*org.Section{}
	for _, s := range flattenSections(f) {
		if s.Headline != nil {
			secs[s.Headline.Pos.Row] = s
		}
	}
	last := len(lines) - 1
	pre := relChunk{row: -1, end: last, subEnd: last, title: p.title, id: props["ID"]}
	if len(scan.headRows) > 0 {
		pre.end = scan.headRows[0] - 1
	}
	p.chunks = append(p.chunks, pre)
	at := map[int]int{-1: 0}
	for i, r := range scan.headRows {
		c := relChunk{row: r, lvl: scan.headLvls[i], end: last}
		if i+1 < len(scan.headRows) {
			c.end = scan.headRows[i+1] - 1
		}
		_, c.subEnd = scan.subtree(r)
		if c.subEnd > last {
			c.subEnd = last
		}
		if s := secs[r]; s != nil {
			c.hash = s.Hash
			c.title = common.GetHeadlineTitle(s.Headline)
			if olp := outlinePath(s); len(olp) > 0 {
				c.olp = olp[:len(olp)-1]
			}
		}
		c.id = headingPropsIn(lines, r, c.end)["ID"]
		at[r] = len(p.chunks)
		p.chunks = append(p.chunks, c)
	}
	var texts = make([]strings.Builder, len(p.chunks))
	for _, l := range scan.lines {
		i, ok := at[l.head]
		if !ok {
			continue
		}
		c := &p.chunks[i]
		if c.tf == nil {
			c.tf, c.surf = map[string]int32{}, map[string]string{}
		}
		text := relProse(l.text[l.from:l.to])
		weight := int32(1)
		if l.isHead {
			weight = 2 // a title says what the heading is about
		}
		relWords(text, func(stem, surface string) {
			c.tf[stem] += weight
			c.n += int(weight)
			if _, have := c.surf[stem]; !have {
				c.surf[stem] = surface
			}
		})
		b := &texts[i]
		if b.Len() < relTextCap {
			t := strings.TrimSpace(strings.Join(strings.Fields(text), " "))
			t = strings.TrimLeft(t, "-+ ")
			if t != "" {
				if b.Len() > 0 {
					if l.isHead {
						b.WriteString("\n")
					} else {
						b.WriteString("\n")
					}
				}
				b.WriteString(t)
			}
		}
	}
	for i := range p.chunks {
		s := texts[i].String()
		if len(s) > relTextCap {
			s = s[:relTextCap]
		}
		p.chunks[i].text = s
	}
	return p
}

// ---------------------------------------------------------------------------
// The whole corpus
// ---------------------------------------------------------------------------

type relDoc struct{ part, chunk int }

type relPost struct {
	doc int32
	w   float32
}

type relIndex struct {
	db     *OrgDb
	at     uint64
	parts  []relPart
	byFile map[string]int
	docs   []relDoc
	docOf  map[[2]int]int // part, chunk -> doc
	idf    map[string]float64
	avg    float64
	post   map[string][]relPost
	norm   []float64
}

var (
	relIdxMu sync.Mutex
	relIdx   *relIndex
)

const relK1, relB = 1.2, 0.75

func getRelIndex() *relIndex {
	db := GetDb()
	relIdxMu.Lock()
	defer relIdxMu.Unlock()
	if relIdx != nil && relIdx.db == db && relIdx.at == db.ReloadIndex {
		return relIdx
	}
	at := db.ReloadIndex
	ix := &relIndex{db: db, at: at, byFile: map[string]int{}, docOf: map[[2]int]int{},
		idf: map[string]float64{}, post: map[string][]relPost{}}
	ix.parts = relParts.All(buildRelPart)
	df := map[string]int{}
	total := 0
	for pi, p := range ix.parts {
		ix.byFile[p.file] = pi
		for ci, c := range p.chunks {
			if c.n == 0 {
				continue
			}
			ix.docOf[[2]int{pi, ci}] = len(ix.docs)
			ix.docs = append(ix.docs, relDoc{pi, ci})
			total += c.n
			for t := range c.tf {
				df[t]++
			}
		}
	}
	N := float64(len(ix.docs))
	if len(ix.docs) > 0 {
		ix.avg = float64(total) / N
	}
	for t, d := range df {
		ix.idf[t] = math.Log(1 + (N-float64(d)+0.5)/(float64(d)+0.5))
	}
	ix.norm = make([]float64, len(ix.docs))
	for di, d := range ix.docs {
		c := &ix.parts[d.part].chunks[d.chunk]
		var sum float64
		for t, tf := range c.tf {
			w := ix.weight(t, float64(tf), float64(c.n))
			sum += w * w
			ix.post[t] = append(ix.post[t], relPost{int32(di), float32(w)})
		}
		ix.norm[di] = math.Sqrt(sum)
	}
	relIdx = ix
	return ix
}

func (ix *relIndex) weight(t string, tf, n float64) float64 {
	idf := ix.idf[t]
	if idf == 0 {
		return 0
	}
	avg := ix.avg
	if avg == 0 {
		avg = 1
	}
	return idf * tf * (relK1 + 1) / (tf + relK1*(1-relB+relB*n/avg))
}

func (ix *relIndex) chunk(doc int) (*relPart, *relChunk) {
	d := ix.docs[doc]
	p := &ix.parts[d.part]
	return p, &p.chunks[d.chunk]
}

// A question in words: its weighted terms, the strongest first, cut short.
type relQuery struct {
	terms []string
	w     []float64
	norm  float64
	words int
}

const relQueryTerms = 64

func (ix *relIndex) query(tf map[string]int32, n int) relQuery {
	q := relQuery{words: n}
	type tw struct {
		t string
		w float64
	}
	var all []tw
	for t, c := range tf {
		if w := ix.weight(t, float64(c), float64(n)); w > 0 {
			all = append(all, tw{t, w})
		}
	}
	sort.Slice(all, func(a, b int) bool {
		if all[a].w != all[b].w {
			return all[a].w > all[b].w
		}
		return all[a].t < all[b].t
	})
	if len(all) > relQueryTerms {
		all = all[:relQueryTerms]
	}
	var sum float64
	for _, x := range all {
		q.terms = append(q.terms, x.t)
		q.w = append(q.w, x.w)
		sum += x.w * x.w
	}
	q.norm = math.Sqrt(sum)
	return q
}

// Cosine of the question against every document sharing a term with it.
func (ix *relIndex) scores(q relQuery) map[int]float64 {
	acc := map[int]float64{}
	if q.norm == 0 {
		return acc
	}
	for i, t := range q.terms {
		for _, p := range ix.post[t] {
			acc[int(p.doc)] += q.w[i] * float64(p.w)
		}
	}
	for d, v := range acc {
		if ix.norm[d] > 0 {
			acc[d] = v / (q.norm * ix.norm[d])
		} else {
			delete(acc, d)
		}
	}
	return acc
}

// The words a question and a result share most, as the result writes them.
func (ix *relIndex) shared(q relQuery, doc int, max int) []string {
	_, c := ix.chunk(doc)
	type tw struct {
		s string
		w float64
	}
	var hits []tw
	for i, t := range q.terms {
		if tf, ok := c.tf[t]; ok {
			hits = append(hits, tw{c.surf[t], q.w[i] * ix.weight(t, float64(tf), float64(c.n))})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].w > hits[b].w })
	out := []string{}
	for _, h := range hits {
		if len(out) == max {
			break
		}
		out = append(out, h.s)
	}
	return out
}

var relSentenceRe = regexp.MustCompile(`[^.!?\n]+[.!?]*`)

// The sentence of a result that holds most of the question's weight: why it
// came up, in its own words.
func (ix *relIndex) snippet(q relQuery, doc int) string {
	_, c := ix.chunk(doc)
	want := map[string]float64{}
	for i, t := range q.terms {
		want[t] = q.w[i]
	}
	best, bestW := "", 0.0
	for _, s := range relSentenceRe.FindAllString(c.text, -1) {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		seen := map[string]bool{}
		w := 0.0
		relWords(s, func(stem, _ string) {
			if !seen[stem] {
				seen[stem] = true
				w += want[stem]
			}
		})
		if w > bestW {
			best, bestW = s, w
		}
	}
	if best == "" {
		best = strings.SplitN(c.text, "\n", 2)[0]
	}
	if r := []rune(best); len(r) > 200 {
		best = string(r[:199]) + "…"
	}
	return best
}

// ---------------------------------------------------------------------------
// Asking
// ---------------------------------------------------------------------------

// What a question is about, worked out from the cached parts.
type relAsk struct {
	target  common.RelatedTarget
	tf      map[string]int32
	n       int
	text    string // the text, for an embedding of a question with no stored vectors
	docs    []int  // the target's own documents (a subtree, a file)
	exclude map[int]bool
	linked  func(it *common.RelatedItem) bool
}

func relKey(file, hash string) string { return file + "\x00" + hash }

// The ends of every link touching the target's region, either way.
func relLinkedKeys(file string, lo, hi int, hashes map[string]bool, wholeFile bool) map[string]bool {
	out := map[string]bool{}
	idx := getLinkIndex()
	for _, l := range idx.links {
		if l.From.Filename == file && l.From.Line >= lo && l.From.Line <= hi && l.To.Filename != "" {
			out[relKey(l.To.Filename, l.To.Hash)] = true
		}
		if l.To.Filename == file && (wholeFile || (l.To.Hash != "" && hashes[l.To.Hash])) {
			out[relKey(l.From.Filename, l.From.Hash)] = true
		}
	}
	return out
}

// askFor builds the question for a saved heading (sec) or file (sec nil).
func (ix *relIndex) askFor(f *common.OrgFile, sec *org.Section) (relAsk, error) {
	a := relAsk{tf: map[string]int32{}, exclude: map[int]bool{}}
	pi, ok := ix.byFile[f.Filename]
	if !ok {
		return a, fmt.Errorf("%s is not indexed", filepath.Base(f.Filename))
	}
	p := &ix.parts[pi]
	a.target = common.RelatedTarget{Kind: "file", Filename: f.Filename, Line: -1, Headline: p.title}
	lo, hi := 0, int(^uint(0)>>1)
	if sec != nil && sec.Headline != nil {
		row := sec.Headline.Pos.Row
		lo, hi = row, row
		for _, c := range p.chunks {
			if c.row == row {
				hi = c.subEnd
			}
		}
		a.target = common.RelatedTarget{Kind: "heading", Filename: f.Filename, Hash: sec.Hash, Line: row,
			Headline: common.GetHeadlineTitle(sec.Headline)}
		if olp := outlinePath(sec); len(olp) > 0 {
			a.target.Olp = olp[:len(olp)-1]
		}
	}
	hashes := map[string]bool{}
	var text strings.Builder
	for ci := range p.chunks {
		c := &p.chunks[ci]
		in := c.row >= lo && c.row <= hi
		if sec == nil {
			in = true
		}
		doc, isDoc := ix.docOf[[2]int{pi, ci}]
		if in {
			if c.hash != "" {
				hashes[c.hash] = true
			}
			for t, n := range c.tf {
				a.tf[t] += n
			}
			a.n += c.n
			if isDoc {
				a.docs = append(a.docs, doc)
			}
			if text.Len() < relTextCap && c.text != "" {
				text.WriteString(c.text + "\n")
			}
		}
		// Itself, its subtree, and every heading above it.
		ancestor := sec != nil && c.row >= 0 && c.row < lo && c.subEnd >= lo
		if isDoc && (in || ancestor) {
			a.exclude[doc] = true
		}
	}
	a.text = text.String()
	a.target.Words = a.n
	keys := relLinkedKeys(f.Filename, lo, hi, hashes, sec == nil)
	a.linked = func(it *common.RelatedItem) bool { return keys[relKey(it.Filename, it.Hash)] }
	return a, nil
}

var relTextLinkRe = regexp.MustCompile(`\[\[([^\]]+)\](?:\[[^\]]*\])?\]`)

// askText builds the question for a client's unsaved text.
func (ix *relIndex) askText(text, file string, line int) relAsk {
	a := relAsk{tf: map[string]int32{}, exclude: map[int]bool{}, text: text}
	a.target = common.RelatedTarget{Kind: "text", Filename: file, Line: -1}
	scan := scanMentionLines(strings.Split(text, "\n"), mentionKeywords(nil))
	for _, l := range scan.lines {
		w := int32(1)
		if l.isHead {
			w = 2
			if a.target.Headline == "" {
				a.target.Headline = strings.TrimSpace(cleanMentionTitle(l.text[l.from:l.to]))
			}
		}
		relWords(relProse(l.text[l.from:l.to]), func(stem, _ string) {
			a.tf[stem] += w
			a.n += int(w)
		})
	}
	a.target.Words = a.n
	// The links written in the text itself.
	ids, titles, files := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, m := range relTextLinkRe.FindAllStringSubmatch(text, -1) {
		t := m[1]
		switch {
		case strings.HasPrefix(t, "id:"):
			ids[strings.TrimPrefix(t, "id:")] = true
		case strings.HasPrefix(t, "*"):
			titles[strings.ToLower(strings.TrimSpace(t[1:]))] = true
		case strings.HasPrefix(t, "file:"):
			path, search := splitSearch(strings.TrimPrefix(t, "file:"))
			if strings.HasPrefix(search, "*") {
				titles[strings.ToLower(strings.TrimSpace(search[1:]))] = true
			} else {
				files[filepath.Base(path)] = true
			}
		}
	}
	keys := map[string]bool{}
	// Its own heading, if the file is saved: left out, and its links back.
	if f := GetDb().FindByFile(file); f != nil && file != "" {
		if line >= 0 {
			if sec := findHeadingByLine(f, line); sec != nil {
				if own, err := ix.askFor(f, sec); err == nil {
					a.exclude = own.exclude
					a.target.Hash, a.target.Line, a.target.Olp = own.target.Hash, own.target.Line, own.target.Olp
					if a.target.Headline == "" {
						a.target.Headline = own.target.Headline
					}
					for k := range relLinkedKeys(file, own.target.Line, own.target.Line-1, map[string]bool{own.target.Hash: true}, false) {
						keys[k] = true
					}
				}
			}
		} else if pi, ok := ix.byFile[f.Filename]; ok {
			for ci := range ix.parts[pi].chunks {
				if d, ok := ix.docOf[[2]int{pi, ci}]; ok {
					a.exclude[d] = true
				}
			}
		}
	}
	a.linked = func(it *common.RelatedItem) bool {
		if keys[relKey(it.Filename, it.Hash)] || (it.Id != "" && ids[it.Id]) {
			return true
		}
		if it.Kind == "file" {
			return files[filepath.Base(it.Filename)]
		}
		return titles[strings.ToLower(it.Headline)]
	}
	return a
}

func (ix *relIndex) item(doc int) common.RelatedItem {
	p, c := ix.chunk(doc)
	it := common.RelatedItem{Kind: "heading", Filename: p.file, Hash: c.hash, Headline: c.title, Olp: c.olp, Line: c.row, Id: c.id}
	if c.row < 0 {
		it.Kind, it.Hash, it.Headline = "file", "", p.title
	}
	return it
}

// relEngine is the engine a question gets: the one asked for, else the
// setting, else embeddings once most headings have vectors.
func relEngine(asked string) (string, string) {
	e := strings.ToLower(strings.TrimSpace(asked))
	if e == "" {
		e = strings.ToLower(strings.TrimSpace(Conf().Server.Related.Engine))
	}
	switch e {
	case "words":
		return "words", ""
	case "embeddings", "embedding", "embed":
		if !relEmbedConfigured() {
			return "words", "no embeddings model is configured (related.embeddings): answered by words"
		}
		return "embeddings", ""
	}
	if !relEmbedConfigured() {
		return "words", ""
	}
	st := relEmbedProgress()
	if st.Total > 0 && st.Done*2 >= st.Total {
		return "embeddings", ""
	}
	if st.Total > 0 {
		return "words", fmt.Sprintf("embeddings are %d%% done: answered by words", st.Done*100/st.Total)
	}
	return "words", ""
}

// answer ranks every document against the question.
func (ix *relIndex) answer(a relAsk, n int, engine string, keepLinked bool) common.RelatedResult {
	res := common.RelatedResult{Ok: true, Target: a.target, Items: []common.RelatedItem{}}
	engine, note := relEngine(engine)
	res.Engine, res.Msg = engine, note
	q := ix.query(a.tf, a.n)
	words := ix.scores(q)
	score := words
	var emb map[int]float64
	if engine == "embeddings" {
		var err error
		emb, err = relEmbedScores(ix, a)
		if err != nil {
			res.Engine, res.Msg = "words", "embeddings: "+err.Error()+": answered by words"
		} else {
			blend := relBlend()
			score = map[int]float64{}
			for d, v := range emb {
				score[d] = (1-blend)*v + blend*words[d]
			}
		}
	}
	if len(score) == 0 && a.n == 0 && res.Engine == "words" {
		res.Msg = "no words to go on"
		return res
	}
	type ds struct {
		d int
		s float64
	}
	ranked := make([]ds, 0, len(score))
	for d, s := range score {
		if a.exclude[d] || s <= 0.01 {
			continue
		}
		ranked = append(ranked, ds{d, s})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].s != ranked[j].s {
			return ranked[i].s > ranked[j].s
		}
		return ranked[i].d < ranked[j].d
	})
	for _, r := range ranked {
		if len(res.Items) >= n {
			break
		}
		it := ix.item(r.d)
		it.Linked = a.linked != nil && a.linked(&it)
		if it.Linked && !keepLinked {
			res.Skipped++
			continue
		}
		it.Score = round3(r.s)
		it.Words = round3(words[r.d])
		if emb != nil {
			it.Embed = round3(emb[r.d])
		}
		it.Terms = ix.shared(q, r.d, 5)
		it.Snippet = ix.snippet(q, r.d)
		res.Items = append(res.Items, it)
	}
	return res
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

func relN(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n <= 0 {
		return 10
	}
	if n > 100 {
		return 100
	}
	return n
}

// QueryRelated answers for a saved heading or file.
func QueryRelated(f *common.OrgFile, sec *org.Section, n int, engine string, keepLinked bool) common.RelatedResult {
	ix := getRelIndex()
	a, err := ix.askFor(f, sec)
	if err != nil {
		return common.RelatedResult{Msg: err.Error(), Items: []common.RelatedItem{}}
	}
	return ix.answer(a, n, engine, keepLinked)
}

func relTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "t", "true", "yes", "y", "on":
		return true
	}
	return false
}

/* SDOC: API
* GET /related — Notes Like This One

	The headings most like a heading (its whole subtree) or a file
	(=features.related=). Left out: the heading itself, its ancestors and
	descendants (a file: everything in it), and anything a link already joins
	to it either way.

	| Parameter  | Type   | Required | Description                                                  |
	|------------+--------+----------+--------------------------------------------------------------|
	| =hash=     | string | or file  | The heading.                                                 |
	| =file=     | string | or hash  | A file (=filename= also works); with =line= the heading that zero-based line is in. |
	| =n=        | int    | no       | How many (default 10, at most 100).                          |
	| =engine=   | string | no       | =words= or =embeddings= (default =related.engine=, auto).   |
	| =linked=   | bool   | no       | Keep results already linked, marked =Linked=.                |

	*Response:* =RelatedResult=: =Engine= (the one that answered), =Target=,
	=Items= (=Hash=, =Filename=, =Olp= - its parents -, =Headline=, =Line=,
	=Score= 0-1, =Words=/=Embed= the two engines' parts, =Terms= the words
	they share most, =Snippet= the sentence most like the target, =Linked=),
	=Skipped= (already linked and left out).
EDOC */
func RequestRelated(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "related") {
		return
	}
	q := r.URL.Query()
	if q.Get("filename") == "" && q.Get("file") != "" {
		q.Set("filename", q.Get("file"))
		r.URL.RawQuery = q.Encode()
	}
	f, sec, err := requestedTarget(r)
	if err != nil {
		dynJson(w, common.RelatedResult{Msg: err.Error(), Items: []common.RelatedItem{}})
		return
	}
	dynJson(w, QueryRelated(f, sec, relN(q.Get("n")), q.Get("engine"), relTruthy(q.Get("linked"))))
}

/* SDOC: API
* POST /related/text — Notes Like Some Text

	As =GET /related=, for text that need not be saved: an editor's buffer.

	#+BEGIN_EXAMPLE
	POST /related/text  {"Text": "** Kubernetes upgrades\nWe drain each node...",
	                     "Filename": "/notes/ops.org", "Line": 12, "N": 10}
	#+END_EXAMPLE

	=Filename= and =Line= (the zero-based row of the heading in the saved
	file) leave that heading, its ancestors and descendants out; =Line= -1 with
	a =Filename= leaves the whole file out. Links written in the text count as
	already linked. =Engine= and =Linked= as for =GET /related=. A GET with
	=text== works too.
EDOC */
func PostRelatedText(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "related") {
		return
	}
	req := common.RelatedTextRequest{Line: -1}
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		req.Text, req.Filename, req.Engine = q.Get("text"), q.Get("filename"), q.Get("engine")
		if req.Filename == "" {
			req.Filename = q.Get("file")
		}
		if l, err := strconv.Atoi(q.Get("line")); err == nil {
			req.Line = l
		}
		req.N, req.Linked = relN(q.Get("n")), relTruthy(q.Get("linked"))
	} else {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 8<<20))
		if err := json.Unmarshal(body, &req); err != nil {
			dynJson(w, common.RelatedResult{Msg: err.Error(), Items: []common.RelatedItem{}})
			return
		}
	}
	if req.N <= 0 {
		req.N = 10
	}
	if req.N > 100 {
		req.N = 100
	}
	if req.Filename != "" {
		if fn, err := resolveRequestedFile(req.Filename); err == nil {
			req.Filename = fn
		}
	}
	ix := getRelIndex()
	dynJson(w, ix.answer(ix.askText(req.Text, req.Filename, req.Line), req.N, req.Engine, req.Linked))
}

/* SDOC: API
* GET /related/status — How Related Notes Stands

	How many headings the words engine holds, the engine a question gets by
	default, and the embeddings engine's progress: =Configured=, =Backend=,
	=Model=, =State= (off, idle, embedding, waiting, error), =Total=, =Done=,
	=Pending=, =LastError=, =Store= (the vectors file).
EDOC */
func RequestRelatedStatus(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "related") {
		return
	}
	ix := getRelIndex()
	st := common.RelatedStatus{Ok: true, Files: len(ix.parts), Chunks: len(ix.docs), Terms: len(ix.idf)}
	st.Engine, st.Msg = relEngine("")
	st.Embeddings = relEmbedProgress()
	dynJson(w, st)
}

/* SDOC: API
* POST /related/reindex — Read Everything Again

	Throws the words index away and builds it again, and has the embeddings
	engine look for headings without vectors now. With ={"Embeddings": true}=
	every vector is asked for again (the stored ones are forgotten).
EDOC */
func PostRelatedReindex(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "related") {
		return
	}
	var req struct{ Embeddings bool }
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	json.Unmarshal(body, &req)
	relParts.Forget()
	relIdxMu.Lock()
	relIdx = nil
	relIdxMu.Unlock()
	if req.Embeddings {
		relEmbedForget()
	}
	ix := getRelIndex()
	relEmbedWake()
	st := common.RelatedStatus{Ok: true, Files: len(ix.parts), Chunks: len(ix.docs), Terms: len(ix.idf),
		Msg: fmt.Sprintf("read %d headings in %d files", len(ix.docs), len(ix.parts))}
	st.Engine, _ = relEngine("")
	st.Embeddings = relEmbedProgress()
	dynJson(w, st)
}

// ---------------------------------------------------------------------------
// Pairs nobody has linked
// ---------------------------------------------------------------------------

var (
	relPairsMu sync.Mutex
	relPairsAt uint64
	relPairsDb *OrgDb
	relPairsOf []common.RelatedPair
	relPairsEn string
)

// The most alike pairs of headings with no link between them. Found by
// words - each heading against the rest through its rarest words - and, when
// embeddings are ready, those candidates re-scored by them (a whole-database
// comparison of vectors is n² and not worth it).
func relatedPairs() ([]common.RelatedPair, string) {
	ix := getRelIndex()
	engine, _ := relEngine("")
	relPairsMu.Lock()
	if relPairsDb == ix.db && relPairsAt == ix.at && relPairsEn == engine && relPairsOf != nil {
		out := relPairsOf
		relPairsMu.Unlock()
		return out, engine
	}
	relPairsMu.Unlock()

	linked := map[string]bool{}
	for _, l := range getLinkIndex().links {
		if l.To.Filename == "" {
			continue
		}
		a, b := relKey(l.From.Filename, l.From.Hash), relKey(l.To.Filename, l.To.Hash)
		linked[a+"\x01"+b], linked[b+"\x01"+a] = true, true
	}
	N := len(ix.docs)
	rare := float64(N) / 10
	acc := make([]float64, N)
	touched := []int{}
	type cand struct {
		a, b int
		s    float64
	}
	best := map[[2]int]float64{}
	for d := 0; d < N; d++ {
		p, c := ix.chunk(d)
		if c.n < 8 || c.row < 0 {
			continue
		}
		q := ix.query(c.tf, c.n)
		if len(q.terms) > 20 {
			q.terms, q.w = q.terms[:20], q.w[:20]
		}
		var sum float64
		for _, w := range q.w {
			sum += w * w
		}
		q.norm = math.Sqrt(sum)
		for i, t := range q.terms {
			ps := ix.post[t]
			if float64(len(ps)) > rare && N > 50 {
				continue
			}
			for _, po := range ps {
				if acc[po.doc] == 0 {
					touched = append(touched, int(po.doc))
				}
				acc[po.doc] += q.w[i] * float64(po.w)
			}
		}
		bestD, bestS := -1, 0.0
		for _, o := range touched {
			s := acc[o]
			acc[o] = 0
			if o == d || ix.norm[o] == 0 || q.norm == 0 {
				continue
			}
			op, oc := ix.chunk(o)
			if oc.n < 8 || oc.row < 0 {
				continue
			}
			if op.file == p.file && ((oc.row >= c.row && oc.row <= c.subEnd) || (c.row >= oc.row && c.row <= oc.subEnd)) {
				continue
			}
			if linked[relKey(p.file, c.hash)+"\x01"+relKey(op.file, oc.hash)] {
				continue
			}
			s = s / (q.norm * ix.norm[o])
			if s > bestS {
				bestD, bestS = o, s
			}
		}
		touched = touched[:0]
		if bestD >= 0 && bestS > 0.15 {
			k := [2]int{d, bestD}
			if bestD < d {
				k = [2]int{bestD, d}
			}
			if bestS > best[k] {
				best[k] = bestS
			}
		}
	}
	var cands []cand
	for k, s := range best {
		cands = append(cands, cand{k[0], k[1], s})
	}
	if engine == "embeddings" {
		blend := relBlend()
		vecs := relEmb.vectors(ix)
		for i := range cands {
			if e, ok := relEmbedPair(vecs, cands[i].a, cands[i].b); ok {
				cands[i].s = (1-blend)*e + blend*cands[i].s
			}
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].s != cands[j].s {
			return cands[i].s > cands[j].s
		}
		return cands[i].a < cands[j].a
	})
	if len(cands) > 200 {
		cands = cands[:200]
	}
	out := make([]common.RelatedPair, 0, len(cands))
	for _, c := range cands {
		a, b := ix.item(c.a), ix.item(c.b)
		_, ca := ix.chunk(c.a)
		q := ix.query(ca.tf, ca.n)
		b.Terms = ix.shared(q, c.b, 5)
		b.Snippet = ix.snippet(q, c.b)
		out = append(out, common.RelatedPair{A: a, B: b, Score: round3(c.s)})
	}
	relPairsMu.Lock()
	relPairsDb, relPairsAt, relPairsEn, relPairsOf = ix.db, ix.at, engine, out
	relPairsMu.Unlock()
	return out, engine
}

/* SDOC: API
* GET /related/pairs — Alike, And Not Linked

	Pairs of headings across the database most like each other with no link
	between them either way, best first: =Pairs= of ={A, B, Score}= (=B= has
	the shared =Terms= and a =Snippet=). =n= (default 30, at most 200).
	Found by words, re-scored by embeddings when they are ready.
EDOC */
func RequestRelatedPairs(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "related") {
		return
	}
	n := 30
	if v, err := strconv.Atoi(r.URL.Query().Get("n")); err == nil && v > 0 {
		n = v
	}
	if n > 200 {
		n = 200
	}
	pairs, engine := relatedPairs()
	if len(pairs) > n {
		pairs = pairs[:n]
	}
	dynJson(w, common.RelatedPairs{Ok: true, Engine: engine, Pairs: pairs})
}
