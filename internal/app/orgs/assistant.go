package orgs

// The writing assistant (worg's writing mode, components/WritingAssistant.tsx):
// a conversation about the text being written, answered by Claude, ChatGPT or
// Gemini. orgs holds the keys (server.assistant in orgs.yaml), builds the
// prompt with the part of the file the writer chose, and streams the answer
// back as newline-delimited JSON, so the browser never sees a key and can
// talk to orgs from anywhere it can reach it.
//
// Claude goes through the official Go SDK; ChatGPT (and any
// OpenAI-compatible server) and Gemini through their REST streaming APIs;
// claudecode through the claude command, on a Claude subscription, with tools
// to read and edit the files the writer allows (assistantcode.go).

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
}

var assistantOrder = []string{"claudecode", "ollama", "claude", "chatgpt", "gemini"}

// The backends that read and edit files themselves, through orgs's tools.
func assistantHasTools(name string) bool { return name == "claudecode" || name == "ollama" }

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
	}
	return nil
}

// Whether worg is offered the backend at all: claudecode and ollama only
// when enabled.
func assistantOffered(name string, b *common.AssistantBackend) bool {
	if b.Disabled {
		return false
	}
	return !assistantHasTools(name) || b.Enabled
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
	if b.Model != "" {
		return b.Model
	}
	return assistantDefaults[name].model
}

// The other models the writer may pick: the yaml's, or for claudecode the
// claude command's own names for the rest of the family.
func assistantModels(name string, b *common.AssistantBackend) []string {
	if name == "ollama" {
		def, names := ollamaModelFor(b)
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
}

func assistantKeySource(name string, b *common.AssistantBackend) string {
	if name == "claudecode" {
		return claudeCodeSource(b)
	}
	if name == "ollama" {
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
	={Ok, Off, Default, Backends: [{Name, Label, Model, Models, Ready, Msg}]}=.
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
	// ollama only: the context window to ask for, in tokens (else
	// assistant.ollama.contextLength).
	NumCtx int
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

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	var sendMu sync.Mutex // claudecode's tools send from their own goroutines
	send := func(v any) {
		sendMu.Lock()
		defer sendMu.Unlock()
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
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()
	var stop string
	switch name {
	case "claude":
		stop, err = assistantClaude(ctx, key, b, model, docs, req.Messages, emit)
	case "chatgpt":
		stop, err = assistantOpenAI(ctx, key, b, model, docs, req.Messages, emit)
	case "gemini":
		stop, err = assistantGemini(ctx, key, b, model, docs, req.Messages, emit)
	case "claudecode":
		stop, err = assistantClaudeCode(ctx, key, b, model, docs, req.Messages, files, emit, send)
	case "ollama":
		stop, err = assistantOllama(ctx, b, model, docs, req.Messages, files, req.NumCtx, emit, send)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ASSISTANT %s: %v\n", name, err)
		msg := err.Error()
		// An API key is billed from API credits, which a Claude (or ChatGPT)
		// subscription does not include.
		if l := strings.ToLower(msg); strings.Contains(l, "credit balance") || strings.Contains(l, "insufficient_quota") || strings.Contains(l, "billing") {
			msg += " - API keys are billed from the service's API credits, which are separate from a Claude Pro/Max (or ChatGPT Plus) subscription. Add credits on the service's billing page, or pick another backend."
		}
		send(map[string]string{"error": msg})
		return
	}
	send(map[string]any{"done": true, "model": model, "stop": stop})
}

// ---------------------------------------------------------------------------
// Claude
// ---------------------------------------------------------------------------

func assistantClaude(ctx context.Context, key string, b *common.AssistantBackend, model, docs string, msgs []assistantMsg, emit func(string)) (string, error) {
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
			{Text: assistantSystem},
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

func assistantOpenAI(ctx context.Context, key string, b *common.AssistantBackend, model, docs string, msgs []assistantMsg, emit func(string)) (string, error) {
	base := strings.TrimRight(b.Url, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	type m struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	all := []m{{"system", assistantSystem + "\n\n" + docs}}
	for _, x := range msgs {
		all = append(all, m{x.Role, x.Text})
	}
	body, _ := json.Marshal(map[string]any{"model": model, "messages": all, "stream": true})
	hreq, _ := http.NewRequestWithContext(ctx, "POST", base+"/chat/completions", bytes.NewReader(body))
	hreq.Header.Set("Content-Type", "application/json")
	if key != "" {
		hreq.Header.Set("Authorization", "Bearer "+key)
	}
	stop := ""
	err := sseLines(hreq, func(data string) (bool, error) {
		if data == "[DONE]" {
			return true, nil
		}
		var ev struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return false, nil
		}
		if ev.Error != nil {
			return true, fmt.Errorf("%s", ev.Error.Message)
		}
		for _, c := range ev.Choices {
			emit(c.Delta.Content)
			if c.FinishReason != "" {
				stop = c.FinishReason
			}
		}
		return false, nil
	})
	return stop, err
}

// ---------------------------------------------------------------------------
// Gemini
// ---------------------------------------------------------------------------

func assistantGemini(ctx context.Context, key string, b *common.AssistantBackend, model, docs string, msgs []assistantMsg, emit func(string)) (string, error) {
	base := strings.TrimRight(b.Url, "/")
	if base == "" {
		base = "https://generativelanguage.googleapis.com/v1beta"
	}
	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Role  string `json:"role,omitempty"`
		Parts []part `json:"parts"`
	}
	all := []content{}
	for _, x := range msgs {
		role := "user"
		if x.Role == "assistant" {
			role = "model"
		}
		all = append(all, content{role, []part{{x.Text}}})
	}
	body, _ := json.Marshal(map[string]any{
		"systemInstruction": content{Parts: []part{{assistantSystem + "\n\n" + docs}}},
		"contents":          all,
	})
	hreq, _ := http.NewRequestWithContext(ctx, "POST", base+"/models/"+model+":streamGenerateContent?alt=sse", bytes.NewReader(body))
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("x-goog-api-key", key)
	stop := ""
	err := sseLines(hreq, func(data string) (bool, error) {
		var ev struct {
			Candidates []struct {
				Content struct {
					Parts []part `json:"parts"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			} `json:"candidates"`
			PromptFeedback *struct {
				BlockReason string `json:"blockReason"`
			} `json:"promptFeedback"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return false, nil
		}
		if ev.Error != nil {
			return true, fmt.Errorf("%s", ev.Error.Message)
		}
		if ev.PromptFeedback != nil && ev.PromptFeedback.BlockReason != "" {
			return true, fmt.Errorf("Gemini declined to answer: %s", ev.PromptFeedback.BlockReason)
		}
		for _, c := range ev.Candidates {
			for _, p := range c.Content.Parts {
				emit(p.Text)
			}
			if c.FinishReason != "" {
				stop = c.FinishReason
			}
		}
		return false, nil
	})
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
