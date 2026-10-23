package orgs

// The writing assistant (worg's writing mode, components/WritingAssistant.tsx):
// a conversation about the text being written, answered by Claude, ChatGPT or
// Gemini. orgs holds the keys (server.assistant in orgs.yaml), builds the
// prompt with the part of the file the writer chose, and streams the answer
// back as newline-delimited JSON, so the browser never sees a key and can
// talk to orgs from anywhere it can reach it.
//
// Claude goes through the official Go SDK; ChatGPT (and any
// OpenAI-compatible server) and Gemini (Google's OpenAI-compatible endpoint)
// through openAIChat, the OpenAI tool loop, with orgs's tools unless their
// tools setting is off;
// claudecode through the claude command, on a Claude subscription, with tools
// to read and edit the files the writer allows (assistantcode.go). local is
// the OpenAI-compatible runtimes orgs installs and runs (localrt.go), through
// the same OpenAI tool loop (openAIChat) chatgpt takes with tools on.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/ihdavids/orgs/internal/common"
)

var assistantDefaults = map[string]struct{ label, model, env string }{
	"claude":  {"Claude", "claude-opus-5-5", "ANTHROPIC_API_KEY"},
	"chatgpt": {"ChatGPT", "gpt-5", "OPENAI_API_KEY"},
	"gemini":  {"Gemini", "gemini-flash-latest", "GEMINI_API_KEY"},
	// Its "key" is optional: a `claude setup-token` token, for a server whose
	// user has not run `claude login`.
	"claudecode": {"Claude Code (subscription)", "opus", "CLAUDE_CODE_OAUTH_TOKEN"},
	// No key: a model on this machine (ollama.go, assistantollama.go).
	"ollama": {"Ollama (local)", "", ""},
	// No key: llama.cpp, LM Studio or MLX on this machine (localrt.go).
	"local": {"Local runtimes", "", ""},
}

var assistantOrder = []string{"claudecode", "ollama", "local", "claude", "chatgpt", "gemini"}

// The backends that read and edit files themselves, through orgs's tools.
func assistantHasTools(name string) bool {
	switch name {
	case "claudecode", "ollama":
		return true
	case "chatgpt":
		t := Conf().Server.Assistant.ChatGPT.Tools
		return t == nil || *t
	case "gemini":
		t := Conf().Server.Assistant.Gemini.Tools
		return t == nil || *t
	case "local":
		t := Conf().Server.Assistant.Local.Tools
		return t == nil || *t
	}
	return false
}

// The backends offered only when turned on.
func assistantOptIn(name string) bool {
	return name == "claudecode" || name == "ollama" || name == "local"
}

func assistantBackend(name string) *common.AssistantBackend {
	a := &Conf().Server.Assistant
	switch name {
	case "claude":
		return &a.Claude
	case "chatgpt":
		return &a.ChatGPT
	case "gemini":
		return &a.Gemini
	case "claudecode":
		return &a.ClaudeCode
	case "ollama":
		return &a.Ollama.AssistantBackend
	case "local":
		return &a.Local.AssistantBackend
	}
	return nil
}

// Whether worg is offered the backend at all: claudecode, ollama and local
// only when enabled.
func assistantOffered(name string, b *common.AssistantBackend) bool {
	if b.Disabled {
		return false
	}
	return !assistantOptIn(name) || b.Enabled
}

// The key for a backend: one set from worg (assistantkeys.go), else written
// in the yaml, read from the environment or the keyring, else the service's
// usual variable.
func assistantKey(name string, b *common.AssistantBackend) (string, error) {
	if name == "claudecode" {
		return claudeCodeKey(b)
	}
	if name == "ollama" {
		return "", ollamaReady(b)
	}
	if name == "local" {
		return "", localReady(b)
	}
	if k := assistantStoredKey(name); k != "" {
		return k, nil
	}
	k := strings.TrimSpace(b.ApiKey)
	switch {
	case strings.HasPrefix(k, "env:"):
		v := os.Getenv(strings.TrimPrefix(k, "env:"))
		if v == "" {
			return "", fmt.Errorf("%s is not set in the server's environment", strings.TrimPrefix(k, "env:"))
		}
		return v, nil
	case strings.HasPrefix(k, "keyring:"):
		v, err := common.KeyringGet(strings.TrimPrefix(k, "keyring:"))
		if err != nil || v == "" {
			return "", fmt.Errorf("no %q in the keyring (service orgs)", strings.TrimPrefix(k, "keyring:"))
		}
		return v, nil
	case k != "":
		return k, nil
	}
	if v := os.Getenv(assistantDefaults[name].env); v != "" {
		return v, nil
	}
	if name == "gemini" {
		if v := os.Getenv("GOOGLE_API_KEY"); v != "" {
			return v, nil
		}
	}
	if name == "claude" {
		// The SDK also finds ANTHROPIC_AUTH_TOKEN and an `ant auth login`
		// profile; an empty key lets it look.
		if os.Getenv("ANTHROPIC_AUTH_TOKEN") != "" || anthropicProfile() {
			return "", nil
		}
	}
	if name == "chatgpt" && b.Url != "" {
		return "", nil // a local OpenAI-compatible server may want none
	}
	return "", fmt.Errorf("no key: set apiKey under assistant.%s, or %s", name, assistantDefaults[name].env)
}

func anthropicProfile() bool {
	if os.Getenv("ANTHROPIC_PROFILE") != "" {
		return true
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return false
	}
	for _, d := range []string{dir, os.Getenv("HOME") + "/.config"} {
		if st, err := os.Stat(d + "/anthropic"); err == nil && st.IsDir() {
			return true
		}
	}
	return false
}

func assistantModel(name string, b *common.AssistantBackend) string {
	if name == "ollama" {
		def, _ := ollamaModelFor(b)
		return def
	}
	if name == "local" {
		def, _ := localModelFor(b)
		return def
	}
	if b.Model != "" {
		return b.Model
	}
	return assistantDefaults[name].model
}

// The other models the writer may pick: the yaml's, or for claudecode the
// claude command's own names for the rest of the family.
func assistantModels(name string, b *common.AssistantBackend) []string {
	if name == "ollama" || name == "local" {
		def, names := ollamaModelFor(b)
		if name == "local" {
			def, names = localModelFor(b)
		}
		out := []string{}
		for _, n := range names {
			if n != def {
				out = append(out, n)
			}
		}
		return out
	}
	if len(b.Models) == 0 && name == "claudecode" {
		out := []string{}
		for _, m := range []string{"opus", "sonnet", "haiku"} {
			if m != assistantModel(name, b) {
				out = append(out, m)
			}
		}
		return out
	}
	return b.Models
}

type assistantBackendStatus struct {
	Name   string
	Label  string
	Model  string
	Models []string
	Ready  bool
	Msg    string
	// Where its key comes from: worg (set in the key helper), orgs.yaml,
	// environment, profile (`ant auth login`), or "" for none. For
	// claudecode, subscription (`claude login`), token or api.
	Source string
	// It can read and edit the files the writer allows (claudecode).
	Tools bool
	// Where the writer's text goes: machine (this one), network (another
	// machine named in the yaml) or cloud (the service's own); for local,
	// each model's own in ModelWhere.
	Where      string
	ModelWhere map[string]string `json:",omitempty"`
	// It may be used in worg's views (column view, gantt, kanban, search),
	// and there it can look headings up and change them (assistantview.go).
	Views     bool
	ViewTools bool
}

func assistantKeySource(name string, b *common.AssistantBackend) string {
	if name == "claudecode" {
		return claudeCodeSource(b)
	}
	if name == "ollama" || name == "local" {
		return "local"
	}
	switch {
	case assistantStoredKey(name) != "":
		return "worg"
	case strings.HasPrefix(b.ApiKey, "env:"):
		return "environment"
	case strings.TrimSpace(b.ApiKey) != "":
		return "orgs.yaml"
	case os.Getenv(assistantDefaults[name].env) != "" || (name == "gemini" && os.Getenv("GOOGLE_API_KEY") != ""):
		return "environment"
	case name == "claude" && (os.Getenv("ANTHROPIC_AUTH_TOKEN") != "" || anthropicProfile()):
		return "profile"
	}
	return ""
}

/* SDOC: API
* GET /assistant/status — The Writing Assistant's Backends
	Which AI backends the server offers worg's writing assistant, whether each
	has a key, its models, and the default:
	={Ok, Off, Default, Backends: [{Name, Label, Model, Models, Ready, Msg,
	Tools, Where, ModelWhere, Views, ViewTools}]}=. =Views= says it may be
	used in worg's views and =ViewTools= that it can look up and change
	headings there. =Where= is where the writer's text is sent:
	=machine= (this one), =network= (another machine, by a private address
	or local name) or =cloud=; =ModelWhere= gives it per model for =local=,
	whose models may be on servers elsewhere.
	=Off= is true when =assistant.backend= is =off=. Keys are never sent;
	=Source= says where each one comes from (=worg=, =orgs.yaml=,
	=environment=, =profile=), and =CanSetKeys= whether this user may set
	them (=POST /assistant/key=).
EDOC */
func RequestAssistantStatus(w http.ResponseWriter, r *http.Request) {
	a := Conf().Server.Assistant
	out := map[string]any{"Ok": true}
	if strings.EqualFold(a.Backend, "off") {
		out["Off"] = true
		out["Backends"] = []assistantBackendStatus{}
		writingJson(w, out)
		return
	}
	list := []assistantBackendStatus{}
	def := ""
	for _, n := range assistantOrder {
		b := assistantBackend(n)
		if !assistantOffered(n, b) {
			continue
		}
		// The key first: for ollama it is what starts the server the model
		// list comes from.
		_, keyErr := assistantKey(n, b)
		s := assistantBackendStatus{Name: n, Label: assistantDefaults[n].label, Model: assistantModel(n, b), Tools: assistantHasTools(n)}
		s.Models = append([]string{s.Model}, assistantModels(n, b)...)
		s.Where, s.ModelWhere = assistantWhere(n, b, s.Models)
		s.Views, s.ViewTools = assistantViewOffered(n), assistantViewTools(n)
		if err := keyErr; err != nil {
			s.Msg = err.Error()
		} else {
			s.Ready = true
			s.Source = assistantKeySource(n, b)
			if def == "" {
				def = n
			}
		}
		list = append(list, s)
	}
	if a.Backend != "" {
		def = strings.ToLower(a.Backend)
	}
	out["Default"] = def
	out["Backends"] = list
	out["CanSetKeys"] = mayChangeKeys(r) == nil
	writingJson(w, out)
}

type assistantMsg struct {
	Role string // user or assistant
	Text string
}

type assistantReq struct {
	Backend  string
	Model    string
	Messages []assistantMsg
	// What the writer is working on, sent with every turn so the answer can
	// lean on it: the file's name and title, and the text in scope (the
	// whole file, the section the cursor is in, the selection, or nothing).
	File      string
	Title     string
	Scope     string
	Context   string
	Section   string // the title path of the section in scope
	Selection string // always sent when there is one, whatever the scope
	// Other org files the writer has let the assistant read for this file.
	// Nothing else is ever read: the assistant has no way to ask for a file,
	// and each of these must be one the OrgDb knows.
	Others []string
	// For a backend with tools (claudecode) only: the editor's text of File
	// as it is now, which it reads and edits in place of the disk's, and the
	// files (File and any of Others) the writer has let it change.
	Doc  string
	Edit []string
	// ollama and local only: the context window to ask for, in tokens (else
	// assistant.ollama.contextLength or assistant.local.contextLength).
	NumCtx int
	// Set when the conversation is about one of worg's views (the column
	// view, a gantt chart, a kanban board, search results) rather than a
	// file in writing mode: assistantview.go. File, Scope, Context, Others,
	// Doc and Edit are then unused.
	View *assistantView
}

// The other files' text, each named, in the order asked for. Refuses a file
// the server does not serve as an org file rather than read anything at all
// the server can reach.
func assistantOthers(req *assistantReq, budget int) (string, error) {
	var b strings.Builder
	for _, name := range req.Others {
		f := GetDb().FindByFile(name)
		if f == nil {
			return "", fmt.Errorf("%s is not an org file this server knows", name)
		}
		raw, err := os.ReadFile(f.Filename)
		if err != nil {
			return "", err
		}
		text := string(raw)
		if len(text) > budget {
			text = text[:max(0, budget)]
		}
		budget -= len(text)
		fmt.Fprintf(&b, "\nAnother of the writer's files, %q, which they have let you read:\n<document>\n%s\n</document>\n", f.Filename, text)
		if budget <= 0 {
			b.WriteString("(The files above were cut short: there was more than can be sent.)\n")
			break
		}
	}
	return b.String(), nil
}

const assistantSystem = `You are a writing assistant inside an Org mode editor. The writer is working on a piece of prose (fiction or non-fiction) kept in an Org file, and talks to you in a pane beside the text.

Help the way a good editor would: answer what was asked, keep the writer's voice, tense, point of view and spelling (British or American as the text has it), and do not pad. When asked to rewrite, continue, shorten or otherwise produce text meant to go into the manuscript, give only that text - no preamble, no quotation marks around it, no commentary after it - so it can be inserted as it is. Write it as Org: *bold*, /italic/, headings as * lines, never Markdown. When asked for feedback, ideas or names, answer plainly and briefly, in short paragraphs or a short list.

Org files may carry a property drawer under a heading (:PROPERTIES: ... :END:) holding notes such as SYNOPSIS, STATUS, POV, CHARACTERS, TENSION and WORD_TARGET: they are the writer's notes about the scene, not part of the prose.`

func assistantContextText(req *assistantReq) (string, error) {
	var b strings.Builder
	name := req.Title
	if name == "" {
		name = req.File
	}
	max := Conf().Server.Assistant.MaxContext
	if max <= 0 {
		max = 400000
	}
	ctx := req.Context
	cut := false
	if len(ctx) > max {
		ctx, cut = ctx[:max], true
	}
	switch req.Scope {
	case "file":
		fmt.Fprintf(&b, "The writer's file, %q:\n<document>\n%s\n</document>\n", name, ctx)
	case "section":
		fmt.Fprintf(&b, "The section of %q the writer is in (%s):\n<section>\n%s\n</section>\n", name, req.Section, ctx)
	default:
		fmt.Fprintf(&b, "The writer is working in %q.\n", name)
	}
	if cut {
		fmt.Fprintf(&b, "(The text above was cut at %d characters.)\n", max)
	}
	if len(req.Others) > 0 {
		more, err := assistantOthers(req, max-len(ctx))
		if err != nil {
			return "", err
		}
		b.WriteString(more)
	}
	if req.Selection != "" {
		fmt.Fprintf(&b, "\nThe passage the writer has selected:\n<selection>\n%s\n</selection>\n", req.Selection)
	}
	return b.String(), nil
}

/* SDOC: API
* POST /assistant/chat — Talk to the Writing Assistant
	Body ={Backend, Model, Messages: [{Role, Text}], File, Title, Scope,
	Context, Section, Selection}=. =Backend= is =claude=, =chatgpt= or
	=gemini= (else the default), =Model= one the status lists (else the
	backend's own). =Scope= says what =Context= holds: =file=, =section=, or
	=none=; =Selection= is the passage selected, if any. =Others= are other
	org files the writer has allowed it to read (each must be one the server
	knows, or the request is refused); nothing else is read.

	For =claudecode= (Claude Code on a subscription, which has tools): =Doc=
	is the editor's text of =File=, which it reads and edits instead of the
	disk's, and =Edit= the files (=File= and any of =Others=) it may change.
	=Others= are read by it when needed rather than sent in the prompt.

	*Response:* newline-delimited JSON, as it arrives: ={"t": "text"}= pieces,
	then ={"done": true, "model": ..., "stop": ...}= or ={"error": "..."}=.
	=claudecode= also sends ={"tool": "Read notes.org"}= for each look and
	write (with ="wrote": file= when a file on disk changed) and
	={"edit": {"file", "old", "changes": [{from, to, insert}]}}= for a change to
	=File=, positions in UTF-16 units of the text as it last saw it, for the
	editor to make.

	With =View= ={Kind, Title, Query, File, Columns, About, Edit, Selected}=
	the conversation is about one of worg's views instead (=Kind= is
	=columns= with a =File=, or =gantt=, =kanban= or =search= with a
	=Query=): see assistantview.go. =claude=, =chatgpt=, =gemini= and
	=local= may be used (the status says =Views=); the last three get tools
	to look headings up and, with =Edit=, change them (=ViewTools=). It
	sends ={"tool", "wrote": file}= for each change, ={"query", "why",
	"count"}= for a query offered to the view, and ends with ="undo": token=
	when it changed anything (=POST /assistant/undo=).
EDOC */
func PostAssistantChat(w http.ResponseWriter, r *http.Request) {
	var req assistantReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	a := Conf().Server.Assistant
	if strings.EqualFold(a.Backend, "off") {
		writingJson(w, common.ResultMsg{Ok: false, Msg: "the writing assistant is turned off on this server"})
		return
	}
	name := strings.ToLower(req.Backend)
	if name == "" {
		name = strings.ToLower(a.Backend)
	}
	if name == "" {
		name = "claude"
	}
	b := assistantBackend(name)
	if b == nil || !assistantOffered(name, b) {
		writingJson(w, common.ResultMsg{Ok: false, Msg: "no backend " + name})
		return
	}
	key, err := assistantKey(name, b)
	if err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	if len(req.Messages) == 0 || req.Messages[len(req.Messages)-1].Role != "user" {
		writingJson(w, common.ResultMsg{Ok: false, Msg: "the last message must be the writer's"})
		return
	}
	if req.View != nil && !assistantViewOffered(name) {
		writingJson(w, common.ResultMsg{Ok: false, Msg: assistantDefaults[name].label + " is for writing mode only; pick another backend"})
		return
	}
	model := assistantModel(name, b)
	if req.Model != "" {
		ok := req.Model == model
		for _, m := range assistantModels(name, b) {
			ok = ok || m == req.Model
		}
		if !ok {
			writingJson(w, common.ResultMsg{Ok: false, Msg: "model " + req.Model + " is not one the server offers"})
			return
		}
		model = req.Model
	}
	if req.View != nil {
		assistantViewTurn(w, r, name, b, key, model, &req)
		return
	}

	// A backend with tools reads the other files when it needs them, rather
	// than having them all in every turn's prompt.
	inPrompt := req
	if assistantHasTools(name) {
		inPrompt.Others = nil
	}
	docs, err := assistantContextText(&inPrompt)
	if err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	var files *codeFiles
	if assistantHasTools(name) {
		if files, err = newCodeFiles(&req); err != nil {
			writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
			return
		}
	}

	send, emit := ndjsonSender(w)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()
	var stop string
	switch name {
	case "claude":
		stop, err = assistantClaude(ctx, key, b, model, assistantSystem, docs, req.Messages, emit)
	case "chatgpt":
		stop, err = assistantOpenAI(ctx, key, b, model, docs, req.Messages, files, emit, send)
	case "gemini":
		stop, err = assistantGemini(ctx, key, b, model, docs, req.Messages, files, emit, send)
	case "claudecode":
		stop, err = assistantClaudeCode(ctx, key, b, model, docs, req.Messages, files, emit, send)
	case "ollama":
		stop, err = assistantOllama(ctx, b, model, docs, req.Messages, files, req.NumCtx, emit, send)
	case "local":
		stop, err = assistantLocal(ctx, model, docs, req.Messages, files, req.NumCtx, emit, send)
	}
	if err != nil {
		send(map[string]string{"error": assistantErrMsg(name, err)})
		return
	}
	send(map[string]any{"done": true, "model": model, "stop": stop})
}

func assistantErrMsg(name string, err error) string {
	fmt.Fprintf(os.Stderr, "ASSISTANT %s: %v\n", name, err)
	msg := err.Error()
	// An API key is billed from API credits, which a Claude (or ChatGPT)
	// subscription does not include.
	if l := strings.ToLower(msg); strings.Contains(l, "credit balance") || strings.Contains(l, "insufficient_quota") || strings.Contains(l, "billing") {
		msg += " - API keys are billed from the service's API credits, which are separate from a Claude Pro/Max (or ChatGPT Plus) subscription. Add credits on the service's billing page, or pick another backend."
	}
	return msg
}

// The answer as newline-delimited JSON, flushed as it goes: send for any
// event, emit for a piece of text.
func ndjsonSender(w http.ResponseWriter) (func(any), func(string)) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	var mu sync.Mutex // claudecode's tools send from their own goroutines
	send := func(v any) {
		mu.Lock()
		defer mu.Unlock()
		enc.Encode(v)
		if flusher != nil {
			flusher.Flush()
		}
	}
	emit := func(t string) {
		if t != "" {
			send(map[string]string{"t": t})
		}
	}
	return send, emit
}

// ---------------------------------------------------------------------------
// Claude
// ---------------------------------------------------------------------------

func assistantClaude(ctx context.Context, key string, b *common.AssistantBackend, model, system, docs string, msgs []assistantMsg, emit func(string)) (string, error) {
	opts := []option.RequestOption{}
	if key != "" {
		opts = append(opts, option.WithAPIKey(key))
	}
	if b.Url != "" {
		opts = append(opts, option.WithBaseURL(b.Url))
	}
	client := anthropic.NewClient(opts...)
	params := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 64000,
		// The instructions and then the writer's text, cached together: a
		// conversation about one file sends the same prefix every turn.
		System: []anthropic.BetaTextBlockParam{
			{Text: system},
			{Text: docs, CacheControl: anthropic.NewBetaCacheControlEphemeralParam()},
		},
		// A policy decline is retried on the model's default fallback
		// inside the same call, rather than leaving the writer with nothing.
		Fallbacks: anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()},
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
	}
	if b.Effort != "" {
		params.OutputConfig = anthropic.BetaOutputConfigParam{Effort: anthropic.BetaOutputConfigEffort(b.Effort)}
	}
	for _, m := range msgs {
		block := anthropic.NewBetaTextBlock(m.Text)
		if m.Role == "assistant" {
			params.Messages = append(params.Messages, anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{block}})
		} else {
			params.Messages = append(params.Messages, anthropic.NewBetaUserMessage(block))
		}
	}
	stream := client.Beta.Messages.NewStreaming(ctx, params)
	stop := ""
	for stream.Next() {
		switch ev := stream.Current().AsAny().(type) {
		case anthropic.BetaRawContentBlockDeltaEvent:
			switch d := ev.Delta.AsAny().(type) {
			case anthropic.BetaTextDelta:
				emit(d.Text)
			}
		case anthropic.BetaRawMessageDeltaEvent:
			stop = string(ev.Delta.StopReason)
			if ev.Delta.StopReason == anthropic.BetaStopReasonRefusal {
				why := string(ev.Delta.StopDetails.Category)
				if ev.Delta.StopDetails.Explanation != "" {
					why = ev.Delta.StopDetails.Explanation
				}
				return stop, fmt.Errorf("Claude declined to answer%s", ifNotEmpty(": ", why))
			}
		}
	}
	if err := stream.Err(); err != nil {
		return stop, err
	}
	return stop, nil
}

func ifNotEmpty(pre, s string) string {
	if s == "" {
		return ""
	}
	return pre + s
}

// ---------------------------------------------------------------------------
// ChatGPT, and OpenAI-compatible servers
// ---------------------------------------------------------------------------

// chatgpt: the hosted service (or the one at url), with orgs's tools when
// assistant.chatgpt.tools is on (files is nil otherwise).
func assistantOpenAI(ctx context.Context, key string, b *common.AssistantBackend, model, docs string, msgs []assistantMsg, files *codeFiles, emit func(string), send func(any)) (string, error) {
	base := strings.TrimRight(b.Url, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	sys := assistantSystem + "\n\n"
	var ts assistantToolset
	if files != nil {
		files.send = send
		ts = files
		sys += codeToolsPrompt(files) + "\n"
	}
	return openAIChat(ctx, openAIConn{base: base, key: key, model: model}, sys+docs, msgs, ts, emit, send)
}

// assistantToolset is what a tool loop may offer the model and call: the
// writer's files (codeFiles) today; anything else that can name its tools
// and run them by name (a query, a board) the same way.
type assistantToolset interface {
	toolDefs() []codeTool
	call(name string, args json.RawMessage) (string, error)
}

// One OpenAI-compatible server and model: the hosted service, a local
// runtime (localrt.go), or a server named under assistant.local.servers.
type openAIConn struct {
	base  string // up to and including /v1
	key   string
	model string
	extra map[string]any // more request fields (max_tokens)
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
	// Anything else the server put on the call, sent back as it came:
	// Gemini refuses the next turn without its thought signature
	// (extra_content), and other servers have their own.
	Extra map[string]json.RawMessage `json:"-"`
}

func (tc openAIToolCall) MarshalJSON() ([]byte, error) {
	out := map[string]any{"id": tc.ID, "type": tc.Type, "function": tc.Function}
	for k, v := range tc.Extra {
		if _, taken := out[k]; !taken {
			out[k] = v
		}
	}
	return json.Marshal(out)
}

type openAIMsg struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

const openAIRounds = 24

// openAIChat is the tool loop for OpenAI's chat completions, as
// assistantOllama is for Ollama's /api/chat: ask (streaming, text to emit as
// it comes), run the tool calls asked for, send their results back as role
// "tool" messages, and ask again until an answer calls no tools. With ts nil
// it is one plain request.
func openAIChat(ctx context.Context, c openAIConn, system string, msgs []assistantMsg, ts assistantToolset, emit func(string), send func(any)) (string, error) {
	all := []openAIMsg{{Role: "system", Content: system}}
	for _, m := range msgs {
		all = append(all, openAIMsg{Role: m.Role, Content: m.Text})
	}
	var tools []map[string]any
	if ts != nil {
		for _, t := range ts.toolDefs() {
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
				"name": t.Name, "description": t.Description, "parameters": t.InputSchema,
			}})
		}
	}
	wrote, stop := false, ""
	for round := 0; round < openAIRounds; round++ {
		body := map[string]any{"model": c.model, "messages": all, "stream": true}
		for k, v := range c.extra {
			body[k] = v
		}
		if tools != nil {
			body["tools"] = tools
		}
		first := true
		text, calls, why, err := openAIOnce(ctx, c, body, func(t string) {
			if first {
				// Thinking models leave the blank lines around their (empty,
				// or separately sent) thoughts at the start.
				t = strings.TrimLeft(t, " \t\r\n")
			}
			if t == "" {
				return
			}
			if first && wrote {
				// The first words after a tool call start a new paragraph.
				emit("\n\n")
			}
			first, wrote = false, true
			emit(t)
		})
		if err != nil && tools != nil && openAINoTools(err) {
			send(map[string]any{"tool": c.model + " cannot use tools here: it sees only what was sent, and cannot read or change files", "warn": true})
			tools = nil
			round--
			continue
		}
		if err != nil {
			return stop, err
		}
		stop = why
		if len(calls) == 0 || ts == nil {
			return stop, nil
		}
		all = append(all, openAIMsg{Role: "assistant", Content: text, ToolCalls: calls})
		for _, tc := range calls {
			args := strings.TrimSpace(tc.Function.Arguments)
			if args == "" {
				args = "{}"
			}
			var out string
			if !json.Valid([]byte(args)) {
				out = "Error: the arguments were not JSON: " + args
			} else if res, err := ts.call(tc.Function.Name, json.RawMessage(args)); err != nil {
				out = "Error: " + err.Error()
			} else {
				out = res
			}
			all = append(all, openAIMsg{Role: "tool", ToolCallID: tc.ID, Content: out})
		}
	}
	return stop, fmt.Errorf("stopped after %d rounds of tool calls without an answer", openAIRounds)
}

// A server refusing tools for this model, rather than failing outright.
func openAINoTools(err error) bool {
	l := strings.ToLower(err.Error())
	return strings.Contains(l, "tool") && (strings.Contains(l, "not support") || strings.Contains(l, "jinja") || strings.Contains(l, "unsupported"))
}

// One streamed request: the text, the tool calls asked for and why it
// stopped.
func openAIOnce(ctx context.Context, c openAIConn, body map[string]any, emit func(string)) (string, []openAIToolCall, string, error) {
	raw, _ := json.Marshal(body)
	hreq, _ := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.base, "/")+"/chat/completions", bytes.NewReader(raw))
	hreq.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		hreq.Header.Set("Authorization", "Bearer "+c.key)
	}
	var acc openAIStream
	err := sseLines(hreq, func(data string) (bool, error) {
		if data == "[DONE]" {
			return true, nil
		}
		t, err := acc.add([]byte(data))
		emit(t)
		return false, err
	})
	return acc.text.String(), acc.calls(), acc.stop, err
}

// openAIStream gathers one streamed answer. Tool calls arrive in pieces
// (llama.cpp sends a call's name and then its arguments a few characters at a
// time, by index) or whole (MLX, OpenAI for short ones), so pieces are joined
// by index; a piece with an id that is not the one at its index starts a new
// call, for servers that number every call 0.
type openAIStream struct {
	text  strings.Builder
	stop  string
	order []*openAIToolCall
	at    map[int]*openAIToolCall
}

func (s *openAIStream) add(data []byte) (string, error) {
	var ev struct {
		Choices []struct {
			Delta struct {
				Content   string `json:"content"`
				ToolCalls []openAIDelta `json:"tool_calls"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		return "", nil
	}
	if ev.Error != nil {
		return "", fmt.Errorf("%s", ev.Error.Message)
	}
	if s.at == nil {
		s.at = map[int]*openAIToolCall{}
	}
	text := ""
	for _, c := range ev.Choices {
		text += c.Delta.Content
		for _, d := range c.Delta.ToolCalls {
			i := len(s.order)
			if d.Index != nil {
				i = *d.Index
			}
			tc := s.at[i]
			if tc == nil || (d.ID != "" && tc.ID != "" && d.ID != tc.ID) {
				tc = &openAIToolCall{Type: "function"}
				s.at[i] = tc
				s.order = append(s.order, tc)
			}
			if d.ID != "" {
				tc.ID = d.ID
			}
			tc.Function.Name += d.Function.Name
			tc.Function.Arguments += d.Function.Arguments
			for k, v := range d.rest {
				if tc.Extra == nil {
					tc.Extra = map[string]json.RawMessage{}
				}
				tc.Extra[k] = v
			}
		}
		if c.FinishReason != "" {
			s.stop = c.FinishReason
		}
	}
	s.text.WriteString(text)
	return text, nil
}

// One piece of a streamed tool call, keeping the fields it does not know.
type openAIDelta struct {
	Index    *int
	ID       string
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	rest map[string]json.RawMessage
}

func (d *openAIDelta) UnmarshalJSON(b []byte) error {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return err
	}
	for k, v := range all {
		var err error
		switch k {
		case "index":
			err = json.Unmarshal(v, &d.Index)
		case "id":
			err = json.Unmarshal(v, &d.ID)
		case "function":
			err = json.Unmarshal(v, &d.Function)
		case "type":
		default:
			if d.rest == nil {
				d.rest = map[string]json.RawMessage{}
			}
			d.rest[k] = v
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *openAIStream) calls() []openAIToolCall {
	out := []openAIToolCall{}
	for i, tc := range s.order {
		if tc.Function.Name == "" {
			continue
		}
		c := *tc
		if c.ID == "" {
			c.ID = fmt.Sprintf("call_%d", i)
		}
		out = append(out, c)
	}
	return out
}

// ---------------------------------------------------------------------------
// Gemini
// ---------------------------------------------------------------------------

const geminiOpenAIBase = "https://generativelanguage.googleapis.com/v1beta/openai"

// gemini: Google's OpenAI-compatible endpoint, through the same tool loop as
// chatgpt and local (files is nil when assistant.gemini.tools is off).
func assistantGemini(ctx context.Context, key string, b *common.AssistantBackend, model, docs string, msgs []assistantMsg, files *codeFiles, emit func(string), send func(any)) (string, error) {
	base := strings.TrimRight(b.Url, "/")
	if base == "" {
		base = geminiOpenAIBase
	}
	sys := assistantSystem + "\n\n"
	var ts assistantToolset
	if files != nil {
		files.send = send
		ts = files
		sys += codeToolsPrompt(files) + "\n"
	}
	stop, err := openAIChat(ctx, openAIConn{base: base, key: key, model: model}, sys+docs, msgs, ts, emit, send)
	if err == nil && stop == "content_filter" {
		return stop, fmt.Errorf("Gemini declined to answer")
	}
	return stop, err
}

// sseLines sends req and hands each server-sent event's data to each, until
// it says it is done or the stream ends. An error status is read for its
// message.
func sseLines(req *http.Request, each func(data string) (bool, error)) error {
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
			return fmt.Errorf("%s (%d)", e.Error.Message, res.StatusCode)
		}
		// Gemini's OpenAI endpoint wraps its error in a list.
		var list []struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &list) == nil && len(list) > 0 && list[0].Error.Message != "" {
			return fmt.Errorf("%s (%d)", list[0].Error.Message, res.StatusCode)
		}
		return fmt.Errorf("%s: %s", res.Status, strings.TrimSpace(string(raw)))
	}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		done, err := each(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	return sc.Err()
}
