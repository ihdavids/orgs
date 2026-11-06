package orgs

// Related notes' embeddings engine (related.embeddings in the yaml): a vector
// per heading from a model - Ollama's /api/embed, or an OpenAI-compatible
// /v1/embeddings (OpenAI, Gemini's OpenAI endpoint, a local runtime or
// server) - kept in a gob beside the config by a hash of model and text, so
// a heading is asked for again only when its text changes.
//
// A background loop (startRelated, only while the feature is on and a
// backend is named) embeds what has no vector, a batch at a time, no more
// than perMinute requests a minute, and backs off a minute after an error.
// A saved heading's question vector is the sum of its subtree's stored
// vectors (no request); only unsaved text is embedded on demand.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

func relEmbedConf() *common.RelatedEmbeddingsSettings {
	return &Conf().Server.Related.Embeddings
}

func relBackend() string {
	b := strings.ToLower(strings.TrimSpace(relEmbedConf().Backend))
	switch b {
	case "chatgpt":
		return "openai"
	case "google":
		return "gemini"
	}
	return b
}

func relEmbedConfigured() bool {
	switch relBackend() {
	case "ollama", "openai", "gemini":
		return true
	case "local":
		return relModel() != ""
	}
	return false
}

func relModel() string {
	if m := strings.TrimSpace(relEmbedConf().Model); m != "" {
		return m
	}
	switch relBackend() {
	case "ollama":
		return "nomic-embed-text"
	case "openai":
		return "text-embedding-3-small"
	case "gemini":
		return "gemini-embedding-001"
	}
	return ""
}

func relBlend() float64 {
	if b := relEmbedConf().Blend; b != nil {
		return math.Max(0, math.Min(1, *b))
	}
	return 0.25
}

func relPerMinute() int {
	if n := relEmbedConf().PerMinute; n > 0 {
		return n
	}
	return 60
}

func relBatch() int {
	if n := relEmbedConf().Batch; n > 0 {
		return min(n, 256)
	}
	return 16
}

func relStorePath() string {
	if cfg := Conf().Config; cfg != "" {
		ext := filepath.Ext(cfg)
		return strings.TrimSuffix(cfg, ext) + "_related_vectors.gob"
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "orgs", "related_vectors.gob")
}

// ---------------------------------------------------------------------------
// Asking a model
// ---------------------------------------------------------------------------

var relClient = &http.Client{Timeout: 3 * time.Minute}

func relPostJSON(ctx context.Context, url, key string, body, out any) error {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	res, err := relClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 256<<20))
	if res.StatusCode >= 300 {
		// {"error": "…"}, {"error": {"message": "…"}}, or Gemini's list of those.
		var one struct {
			Error any `json:"error"`
		}
		var list []struct {
			Error any `json:"error"`
		}
		msg := ""
		if json.Unmarshal(data, &one) == nil && one.Error != nil {
			msg = relErrText(one.Error)
		} else if json.Unmarshal(data, &list) == nil && len(list) > 0 {
			msg = relErrText(list[0].Error)
		}
		if msg == "" {
			msg = strings.TrimSpace(string(data))
			if len(msg) > 300 {
				msg = msg[:300]
			}
		}
		return fmt.Errorf("%s: %s", res.Status, msg)
	}
	return json.Unmarshal(data, out)
}

func relErrText(e any) string {
	switch v := e.(type) {
	case string:
		return v
	case map[string]any:
		if m, ok := v["message"].(string); ok {
			return m
		}
	}
	return ""
}

// The key for a cloud backend: related.embeddings.apiKey (literal, env:,
// keyring:), else the assistant's.
func relApiKey(assistantName string) (string, error) {
	k := strings.TrimSpace(relEmbedConf().ApiKey)
	switch {
	case strings.HasPrefix(k, "env:"):
		if v := os.Getenv(strings.TrimPrefix(k, "env:")); v != "" {
			return v, nil
		}
		return "", fmt.Errorf("%s is not set in the server's environment", strings.TrimPrefix(k, "env:"))
	case strings.HasPrefix(k, "keyring:"):
		v, err := common.KeyringGet(strings.TrimPrefix(k, "keyring:"))
		if err != nil || v == "" {
			return "", fmt.Errorf("no %q in the keyring (service orgs)", strings.TrimPrefix(k, "keyring:"))
		}
		return v, nil
	case k != "":
		return k, nil
	}
	return assistantKey(assistantName, assistantBackend(assistantName))
}

func relWithV1(u string) string {
	u = strings.TrimRight(u, "/")
	if !strings.HasSuffix(u, "/v1") && !strings.Contains(u, "/v1beta") {
		u += "/v1"
	}
	return u
}

// relEmbed asks the configured model for one vector per text.
func relEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	s := relEmbedConf()
	model := relModel()
	var base, key string
	switch relBackend() {
	case "ollama":
		u := strings.TrimRight(s.Url, "/")
		if u == "" {
			if Conf().Server.Assistant.Ollama.Enabled {
				if err := ensureOllama(60 * time.Second); err != nil {
					return nil, err
				}
			}
			u = ollamaURL()
		}
		var out struct {
			Embeddings [][]float32 `json:"embeddings"`
		}
		if err := relPostJSON(ctx, u+"/api/embed", "", map[string]any{"model": model, "input": texts}, &out); err != nil {
			return nil, err
		}
		if len(out.Embeddings) != len(texts) {
			return nil, fmt.Errorf("asked for %d vectors, %s answered %d", len(texts), model, len(out.Embeddings))
		}
		return out.Embeddings, nil
	case "openai":
		base = s.Url
		if base == "" {
			base = Conf().Server.Assistant.ChatGPT.Url
		}
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		k, err := relApiKey("chatgpt")
		if err != nil {
			return nil, err
		}
		key = k
	case "gemini":
		base = s.Url
		if base == "" {
			base = Conf().Server.Assistant.Gemini.Url
		}
		if base == "" {
			base = geminiOpenAIBase
		}
		k, err := relApiKey("gemini")
		if err != nil {
			return nil, err
		}
		key = k
	case "local":
		rtName, name, ok := strings.Cut(model, ":")
		switch {
		case s.Url != "":
			base = relWithV1(s.Url)
			if ok && localRuntimeNamed(rtName) != nil || ok && localServerNamed(rtName) != nil {
				model = name
			}
			key = strings.TrimSpace(s.ApiKey)
		case !ok:
			return nil, fmt.Errorf("%q is not a local model (runtime:name)", model)
		case localRuntimeNamed(rtName) != nil:
			base, model = localRuntimeNamed(rtName).url()+"/v1", name
		case localServerNamed(rtName) != nil:
			srv := localServerNamed(rtName)
			base, model, key = relWithV1(srv.conf.Url), name, localServerKey(srv.conf.ApiKey)
		default:
			return nil, fmt.Errorf("no local runtime or server %s", rtName)
		}
	default:
		return nil, fmt.Errorf("no embeddings backend (related.embeddings.backend: ollama, local, openai or gemini)")
	}
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := relPostJSON(ctx, strings.TrimRight(base, "/")+"/embeddings", key, map[string]any{"model": model, "input": texts}, &out); err != nil {
		return nil, err
	}
	vecs := make([][]float32, len(texts))
	for i, d := range out.Data {
		at := d.Index
		if at < 0 || at >= len(texts) {
			at = i
		}
		if at < len(vecs) {
			vecs[at] = d.Embedding
		}
	}
	for i := range vecs {
		if len(vecs[i]) == 0 {
			return nil, fmt.Errorf("asked for %d vectors, %s answered %d", len(texts), model, len(out.Data))
		}
	}
	return vecs, nil
}

func relNormalise(v []float32) []float32 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	if s == 0 {
		return v
	}
	n := float32(1 / math.Sqrt(s))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x * n
	}
	return out
}

func relDot(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var s float32
	for i := range a {
		s += a[i] * b[i]
	}
	return float64(s)
}

// ---------------------------------------------------------------------------
// The store and the loop
// ---------------------------------------------------------------------------

type relEmbedder struct {
	mu        sync.Mutex
	vecs      map[string][]float32
	loaded    bool
	dirty     bool
	state     string
	msg       string
	lastErr   string
	gen       int
	wake      chan struct{}
	keysIx    *relIndex
	keysModel string
	keys      []string // per document of keysIx; "" = nothing to embed
	query     map[string][]float32 // unsaved texts already asked about
}

var relEmb = &relEmbedder{vecs: map[string][]float32{}, state: "off", wake: make(chan struct{}, 1), query: map[string][]float32{}}

func relTextKey(model, text string) string {
	h := sha256.Sum256([]byte(model + "\x00" + text))
	return hex.EncodeToString(h[:12])
}

func relEmbedText(ix *relIndex, doc int) string {
	p, c := ix.chunk(doc)
	if c.row < 0 {
		return strings.TrimSpace(p.title + "\n" + c.text)
	}
	return c.text
}

// The vector keys of an index's documents, worked out once per index.
func (e *relEmbedder) keysFor(ix *relIndex) []string {
	model := relBackend() + ":" + relModel()
	e.mu.Lock()
	if e.keysIx == ix && e.keysModel == model {
		k := e.keys
		e.mu.Unlock()
		return k
	}
	e.mu.Unlock()
	keys := make([]string, len(ix.docs))
	for d := range ix.docs {
		if t := relEmbedText(ix, d); strings.TrimSpace(t) != "" {
			keys[d] = relTextKey(model, t)
		}
	}
	e.mu.Lock()
	e.keysIx, e.keysModel, e.keys = ix, model, keys
	e.mu.Unlock()
	return keys
}

func (e *relEmbedder) load() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.loaded {
		return
	}
	e.loaded = true
	f, err := os.Open(relStorePath())
	if err != nil {
		return
	}
	defer f.Close()
	var m map[string][]float32
	if err := gob.NewDecoder(f).Decode(&m); err == nil && m != nil {
		e.vecs = m
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "related: could not read %s: %v\n", relStorePath(), err)
	}
}

// save writes the store, keeping only vectors some heading still has.
func (e *relEmbedder) save(keep []string) {
	e.mu.Lock()
	if !e.dirty {
		e.mu.Unlock()
		return
	}
	if len(keep) > 0 {
		live := make(map[string]bool, len(keep))
		for _, k := range keep {
			live[k] = true
		}
		for k := range e.vecs {
			if !live[k] {
				delete(e.vecs, k)
			}
		}
	}
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(e.vecs)
	e.dirty = false
	e.mu.Unlock()
	if err != nil {
		return
	}
	p := relStorePath()
	os.MkdirAll(filepath.Dir(p), 0755)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0600); err == nil {
		os.Rename(tmp, p)
	} else {
		fmt.Fprintf(os.Stderr, "related: could not write %s: %v\n", p, err)
	}
}

func (e *relEmbedder) set(state, msg string) {
	e.mu.Lock()
	e.state, e.msg = state, msg
	e.mu.Unlock()
}

func relEmbedWake() {
	select {
	case relEmb.wake <- struct{}{}:
	default:
	}
}

func relEmbedForget() {
	e := relEmb
	e.load()
	e.mu.Lock()
	e.vecs = map[string][]float32{}
	e.query = map[string][]float32{}
	e.dirty = true
	e.mu.Unlock()
	e.save(nil)
}

// startRelated starts (or stops) the embedding loop to match the feature and
// the settings. Called when routes are registered and when the feature flips.
func startRelated() {
	e := relEmb
	e.mu.Lock()
	e.gen++
	gen := e.gen
	e.mu.Unlock()
	if Conf().Server == nil || !featureOn("related") || !relEmbedConfigured() {
		e.set("off", "")
		return
	}
	e.set("idle", "")
	go e.loop(gen)
}

func (e *relEmbedder) current(gen int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.gen == gen
}

func (e *relEmbedder) loop(gen int) {
	// A minute's grace at start, so a server coming up is not slowed by it,
	// unless something asks first.
	select {
	case <-e.wake:
	case <-time.After(20 * time.Second):
	}
	e.load()
	gap := time.Minute / time.Duration(relPerMinute())
	lastSave := time.Now()
	for e.current(gen) {
		if !featureOn("related") {
			e.set("off", "")
			return
		}
		ix := getRelIndex()
		keys := e.keysFor(ix)
		var todo []int
		seen := map[string]bool{}
		e.mu.Lock()
		for d, k := range keys {
			if k == "" || seen[k] {
				continue
			}
			if _, have := e.vecs[k]; !have {
				seen[k] = true
				todo = append(todo, d)
			}
		}
		e.mu.Unlock()
		if len(todo) == 0 {
			e.save(keys)
			e.set("idle", "")
			select {
			case <-e.wake:
			case <-time.After(30 * time.Second):
			}
			continue
		}
		if n := relBatch(); len(todo) > n {
			todo = todo[:n]
		}
		texts := make([]string, len(todo))
		for i, d := range todo {
			texts[i] = relEmbedText(ix, d)
		}
		e.set("embedding", fmt.Sprintf("asking %s for %d vectors", relModel(), len(texts)))
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		vecs, err := relEmbed(ctx, texts)
		cancel()
		if !e.current(gen) {
			return
		}
		if err != nil {
			e.mu.Lock()
			e.state, e.msg, e.lastErr = "error", "trying again in a minute", err.Error()
			e.mu.Unlock()
			fmt.Fprintf(os.Stderr, "related: embeddings: %v\n", err)
			select {
			case <-e.wake:
			case <-time.After(time.Minute):
			}
			continue
		}
		e.mu.Lock()
		for i, d := range todo {
			e.vecs[keys[d]] = relNormalise(vecs[i])
		}
		e.dirty, e.lastErr = true, ""
		e.mu.Unlock()
		if time.Since(lastSave) > 15*time.Second {
			e.save(nil)
			lastSave = time.Now()
		}
		e.set("waiting", "")
		time.Sleep(gap)
	}
}

func relEmbedProgress() common.RelatedEmbedStatus {
	st := common.RelatedEmbedStatus{Configured: relEmbedConfigured(), State: "off"}
	if !st.Configured {
		st.Msg = "no related.embeddings.backend in the server's yaml"
		return st
	}
	st.Backend, st.Model, st.Store, st.PerMinute = relBackend(), relModel(), relStorePath(), relPerMinute()
	e := relEmb
	e.load()
	ix := getRelIndex()
	keys := e.keysFor(ix)
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, k := range keys {
		if k == "" {
			continue
		}
		st.Total++
		if _, ok := e.vecs[k]; ok {
			st.Done++
		}
	}
	st.Pending = st.Total - st.Done
	st.State, st.Msg, st.LastError = e.state, e.msg, e.lastErr
	if st.Pending > 0 && st.State == "idle" {
		st.State = "waiting"
	}
	return st
}

// The vector of each document, nil where there is none yet.
func (e *relEmbedder) vectors(ix *relIndex) [][]float32 {
	keys := e.keysFor(ix)
	out := make([][]float32, len(keys))
	e.mu.Lock()
	for d, k := range keys {
		if k != "" {
			out[d] = e.vecs[k]
		}
	}
	e.mu.Unlock()
	return out
}

// relEmbedScores is every embedded document's cosine with the question.
func relEmbedScores(ix *relIndex, a relAsk) (map[int]float64, error) {
	e := relEmb
	e.load()
	vecs := e.vectors(ix)
	have := 0
	for _, v := range vecs {
		if v != nil {
			have++
		}
	}
	if have == 0 {
		relEmbedWake()
		return nil, fmt.Errorf("no heading has a vector yet")
	}
	var q []float32
	for _, d := range a.docs {
		v := vecs[d]
		if v == nil {
			continue
		}
		_, c := ix.chunk(d)
		w := float32(math.Sqrt(float64(c.n) + 1))
		if q == nil {
			q = make([]float32, len(v))
		}
		for i := range v {
			if i < len(q) {
				q[i] += w * v[i]
			}
		}
	}
	if q == nil {
		text := strings.TrimSpace(a.text)
		if text == "" {
			return nil, fmt.Errorf("nothing to embed")
		}
		if len(text) > 2*relTextCap {
			text = text[:2*relTextCap]
		}
		k := relTextKey(relBackend()+":"+relModel(), text)
		e.mu.Lock()
		q = e.query[k]
		e.mu.Unlock()
		if q == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			got, err := relEmbed(ctx, []string{text})
			cancel()
			if err != nil {
				return nil, err
			}
			q = got[0]
			e.mu.Lock()
			if len(e.query) > 256 {
				e.query = map[string][]float32{}
			}
			e.query[k] = q
			e.mu.Unlock()
		}
	}
	q = relNormalise(q)
	out := make(map[int]float64, have)
	for d, v := range vecs {
		if v != nil {
			out[d] = relDot(q, v)
		}
	}
	return out, nil
}

// relEmbedPair is two documents' cosine, when both have vectors.
func relEmbedPair(vecs [][]float32, a, b int) (float64, bool) {
	if a >= len(vecs) || b >= len(vecs) ||  vecs[a] == nil || vecs[b] == nil {
		return 0, false
	}
	return relDot(vecs[a], vecs[b]), true
}
