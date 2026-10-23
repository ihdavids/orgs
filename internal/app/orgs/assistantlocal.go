package orgs

// The writing assistant's local backend (assistant.local): a model on one of
// the runtimes in localrt.go, or on a server named under
// assistant.local.servers, asked through the OpenAI tool loop (openAIChat)
// with the same tools as claudecode and ollama.
//
// Its models are named runtime:name (llamacpp:Qwen3-8B-Q4_K_M,
// mlx:mlx-community/Qwen3-8B-4bit, lmstudio:qwen/qwen3-8b, vllm:...), so one
// backend spans every runtime and choosing a model chooses its runtime too.
// The runtime is started (and the model loaded) when a model is used.
//
// assistantWhere says where each backend sends the writer's text - this
// machine, another one on the network, or a cloud service - from the url it
// actually uses, for worg to show beside the backend and each model.

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// The models of every runtime and server, as runtime:name, the configured
// one (else the configured runtime's first, else the first) by default.
func localModelFor(b *common.AssistantBackend) (string, []string) {
	names := []string{}
	first := map[string]string{}
	for _, rt := range localOffered() {
		ms, _ := rt.listModels(false)
		for _, m := range ms {
			n := rt.impl.id() + ":" + m.Name
			names = append(names, n)
			if first[rt.impl.id()] == "" {
				first[rt.impl.id()] = n
			}
		}
	}
	for _, s := range localServers() {
		for _, m := range s.models {
			n := s.conf.Name + ":" + m
			names = append(names, n)
			if first[s.conf.Name] == "" {
				first[s.conf.Name] = n
			}
		}
	}
	def := ""
	for _, n := range names {
		if n == b.Model {
			def = n
		}
	}
	if def == "" {
		def = first[localConf().Runtime]
	}
	if def == "" && len(names) > 0 {
		def = names[0]
	}
	return def, names
}

func localReady(b *common.AssistantBackend) error {
	if def, _ := localModelFor(b); def == "" {
		return fmt.Errorf("no models yet: add one in Local runtimes")
	}
	return nil
}

func assistantLocal(ctx context.Context, model, docs string, msgs []assistantMsg, files *codeFiles, numCtx int, emit func(string), send func(any)) (string, error) {
	conn, info, err := localConn(ctx, model, numCtx, send)
	if err != nil {
		return "", err
	}
	sys := assistantSystem + "\n\n"
	var ts assistantToolset
	if files != nil {
		files.send = send
		sys += codeToolsPrompt(files) + "\n"
		ts = files
	}
	sys += docs
	ts = localToolsFor(info, conn, ts, send)
	localContextWarn(info, model, numCtx, sys, msgs, send)
	return openAIChat(ctx, conn, sys, msgs, ts, emit, send)
}

// The connection for a local model (runtime:name), its runtime started and
// the model loaded; info is what the runtime knows of it (nil for a server
// orgs only talks to).
func localConn(ctx context.Context, model string, numCtx int, send func(any)) (openAIConn, *localModel, error) {
	rtName, name, ok := strings.Cut(model, ":")
	if !ok {
		return openAIConn{}, nil, fmt.Errorf("%q is not a local model (runtime:name)", model)
	}
	note := func(s string) { send(map[string]any{"tool": s}) }
	conn := openAIConn{model: name, extra: map[string]any{"max_tokens": localMaxTokens()}}
	if rt := localRuntimeNamed(rtName); rt != nil {
		reqName, err := rt.impl.prepare(ctx, rt, name, numCtx, note)
		if err != nil {
			return conn, nil, err
		}
		conn.base, conn.model = rt.url()+"/v1", reqName
		ms, _ := rt.listModels(false)
		for i := range ms {
			if ms[i].Name == name {
				return conn, &ms[i], nil
			}
		}
		return conn, nil, nil
	}
	if s := localServerNamed(rtName); s != nil {
		conn.base = strings.TrimRight(s.conf.Url, "/")
		conn.key = localServerKey(s.conf.ApiKey)
		return conn, nil, nil
	}
	return conn, nil, fmt.Errorf("no local runtime or server %s", rtName)
}

// The tools, unless the runtime knows the model cannot take them.
func localToolsFor(info *localModel, conn openAIConn, ts assistantToolset, send func(any)) assistantToolset {
	if ts != nil && info != nil && info.ToolsKnown && !info.Tools {
		send(map[string]any{"tool": info.Name + " cannot use tools: it sees only what was sent, and cannot read or change anything", "warn": true})
		return nil
	}
	return ts
}

// Says so when the text sent will not fit the model's context.
func localContextWarn(info *localModel, model string, numCtx int, sys string, msgs []assistantMsg, send func(any)) {
	if info == nil {
		return
	}
	rtName, _, _ := strings.Cut(model, ":")
	n := info.ContextMax
	if n == 0 || (rtName == "llamacpp" && localContext() < n) {
		n = localContext()
	}
	if rtName == "lmstudio" && numCtx > 0 {
		n = numCtx
	}
	if est := (len(sys) + len(codePrompt(msgs))) / 4; rtName != "mlx" && n > 0 && est > n*9/10 {
		send(map[string]any{"tool": fmt.Sprintf("The text sent is about %d tokens but the context is %d: it may not all fit. Raise the context length, or send less.", est, n), "warn": true})
	}
}

// ---------------------------------------------------------------------------
// Servers orgs only talks to
// ---------------------------------------------------------------------------

type localServer struct {
	conf   common.LocalServer
	models []string
	err    error
}

var localServerCache struct {
	sync.Mutex
	at   time.Time
	list []localServer
}

// The configured servers with their models (the yaml's, else /v1/models),
// asked at most every half minute.
func localServers() []localServer {
	c := &localServerCache
	c.Lock()
	defer c.Unlock()
	if time.Since(c.at) < 30*time.Second && len(c.list) == len(localConf().Servers) {
		return c.list
	}
	list := []localServer{}
	for _, s := range localConf().Servers {
		if s.Name == "" || s.Url == "" {
			continue
		}
		ls := localServer{conf: s, models: s.Models}
		if len(ls.models) == 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			var out struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			ls.err = openAIModels(ctx, s.Url, localServerKey(s.ApiKey), &out)
			cancel()
			for _, d := range out.Data {
				ls.models = append(ls.models, d.ID)
			}
		}
		list = append(list, ls)
	}
	c.list, c.at = list, time.Now()
	return list
}

func openAIModels(ctx context.Context, base, key string, out any) error {
	return localCallAuth(ctx, "GET", strings.TrimRight(base, "/")+"/models", key, nil, out)
}

func localServerNamed(name string) *localServer {
	for _, s := range localServers() {
		if s.conf.Name == name {
			return &s
		}
	}
	return nil
}

func localServerKey(k string) string {
	k = strings.TrimSpace(k)
	if strings.HasPrefix(k, "env:") {
		return os.Getenv(strings.TrimPrefix(k, "env:"))
	}
	if strings.HasPrefix(k, "keyring:") {
		v, _ := common.KeyringGet(strings.TrimPrefix(k, "keyring:"))
		return v
	}
	return k
}

func localServerStatus() []map[string]any {
	out := []map[string]any{}
	for _, s := range localServers() {
		m := map[string]any{"Name": s.conf.Name, "Url": s.conf.Url, "Ready": s.err == nil && len(s.models) > 0,
			"Models": append([]string{}, s.models...), "Where": whereURL(s.conf.Url)}
		if s.err != nil {
			m["Msg"] = s.err.Error()
		}
		out = append(out, m)
	}
	return out
}

// ---------------------------------------------------------------------------
// Where the text goes
// ---------------------------------------------------------------------------

// "machine" for this one, "network" for another machine reached by a
// private address or a bare/local host name (a GPU box down the hall),
// "cloud" for anything else.
func whereURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "cloud"
	}
	h := strings.ToLower(u.Hostname())
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return "machine"
	}
	if ip := net.ParseIP(h); ip != nil {
		switch {
		case ip.IsLoopback():
			return "machine"
		case ip.IsPrivate() || ip.IsLinkLocalUnicast():
			return "network"
		}
		return "cloud"
	}
	if !strings.Contains(h, ".") {
		return "network"
	}
	for _, suf := range []string{".local", ".lan", ".home", ".internal", ".home.arpa"} {
		if strings.HasSuffix(h, suf) {
			return "network"
		}
	}
	return "cloud"
}

// The furthest of two places.
func whereFurthest(a, b string) string {
	rank := map[string]int{"": 0, "machine": 1, "network": 2, "cloud": 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

// Where a backend sends the text, and for local each model's own place.
func assistantWhere(name string, b *common.AssistantBackend, models []string) (string, map[string]string) {
	switch name {
	case "claudecode":
		return "cloud", nil // the claude command talks to Anthropic
	case "ollama":
		return whereURL(ollamaURL()), nil
	case "local":
		per := map[string]string{}
		far := ""
		for _, m := range models {
			if m == "" {
				continue
			}
			rtName, _, _ := strings.Cut(m, ":")
			w := "cloud"
			if rt := localRuntimeNamed(rtName); rt != nil {
				w = whereURL(rt.url())
			} else if s := localServerNamed(rtName); s != nil {
				w = whereURL(s.conf.Url)
			}
			per[m] = w
			far = whereFurthest(far, w)
		}
		if far == "" {
			far = "machine"
		}
		return far, per
	}
	if b.Url != "" {
		return whereURL(b.Url), nil
	}
	return "cloud", nil
}
