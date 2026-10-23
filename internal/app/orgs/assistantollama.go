package orgs

// The ollama backend of the writing assistant: a local model through
// Ollama's own /api/chat, with orgs running the tool loop itself. The tools
// and the files they may touch are the claudecode backend's (codeFiles,
// codeTools in assistantcode.go), called directly rather than over MCP:
//
//   ask (streaming) -> text goes to worg as it comes; tool calls are
//   collected -> each is run against the allowed files -> the results go
//   back as role "tool" messages -> ask again, until an answer has no tool
//   calls or ollamaRounds is reached.
//
// - A model without tool support (Ollama answers 400 "does not support
//   tools") is asked again without them, and worg is told it sees only
//   what was sent.
// - The context window is num_ctx, per request: worg's choice, else
//   assistant.ollama.contextLength, never past the model's own maximum. A
//   prompt that will not fit is said, since Ollama cuts the start off
//   silently.
// - Thinking (a model's message.thinking) is not shown; assistant.ollama.think
//   turns it on or off for models that can.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

const ollamaRounds = 24

type ollamaToolCall struct {
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type ollamaMsg struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
	ToolName  string           `json:"tool_name,omitempty"`
}

// The model a request may use: one installed, the configured one by
// default, else the first.
func ollamaModelFor(b *common.AssistantBackend) (string, []string) {
	models, _ := ollamaModels(false)
	names := []string{}
	for _, m := range models {
		names = append(names, m.Name)
	}
	def := ""
	for _, n := range names {
		if n == b.Model || n == b.Model+":latest" {
			def = n
		}
	}
	if def == "" && len(names) > 0 {
		def = names[0]
	}
	return def, names
}

// Ready: running, with a model to talk to.
func ollamaReady(b *common.AssistantBackend) error {
	if err := ensureOllama(5 * time.Second); err != nil {
		return err
	}
	if def, _ := ollamaModelFor(b); def == "" {
		return fmt.Errorf("no models yet: add one in the Ollama manager")
	}
	return nil
}

// The context window for a request: asked for, else configured, clamped to
// what the model takes.
func ollamaNumCtx(asked int, model string) int {
	n := asked
	if n <= 0 {
		n = ollamaContextDefault()
	}
	if n < 2048 {
		n = 2048
	}
	models, _ := ollamaModels(false)
	for _, m := range models {
		if m.Name == model && m.ContextMax > 0 && n > m.ContextMax {
			n = m.ContextMax
		}
	}
	return n
}

func assistantOllama(ctx context.Context, b *common.AssistantBackend, model, docs string, msgs []assistantMsg, files *codeFiles, numCtx int, emit func(string), send func(any)) (string, error) {
	files.send = send
	c := ollamaConf()
	sys := assistantSystem + "\n\n" + codeToolsPrompt(files) + "\n" + docs
	all := []ollamaMsg{{Role: "system", Content: sys}}
	for _, m := range msgs {
		all = append(all, ollamaMsg{Role: m.Role, Content: m.Text})
	}
	n := ollamaNumCtx(numCtx, model)
	if est := (len(sys) + len(codePrompt(msgs))) / 4; est > n*9/10 {
		send(map[string]any{"tool": fmt.Sprintf("The text sent is about %d tokens but the context is %d: the start of it will be lost. Raise the context length, or send less (a section instead of the whole file).", est, n), "warn": true})
	}
	tools := []map[string]any{}
	for _, t := range codeTools(files) {
		tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
			"name": t.Name, "description": t.Description, "parameters": t.InputSchema,
		}})
	}
	// A model known not to take tools is not offered them (it would refuse
	// the whole request); one not known is tried, and asked again without.
	models, _ := ollamaModels(false)
	for _, m := range models {
		if m.Name == model && !m.Tools && m.ContextMax > 0 {
			send(map[string]any{"tool": model + " cannot use tools: it sees only what was sent, and cannot read or change files", "warn": true})
			tools = nil
		}
	}
	opts := map[string]any{"num_ctx": n}
	wrote, stop := false, ""
	for round := 0; round < ollamaRounds; round++ {
		body := map[string]any{"model": model, "messages": all, "stream": true, "options": opts}
		if tools != nil {
			body["tools"] = tools
		}
		if c.KeepAlive != "" {
			body["keep_alive"] = c.KeepAlive
		}
		if c.Think != nil {
			body["think"] = *c.Think
		}
		first := true
		text, calls, why, err := ollamaChatOnce(ctx, body, func(t string) {
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
		if err != nil && tools != nil && strings.Contains(strings.ToLower(err.Error()), "does not support tools") {
			send(map[string]any{"tool": model + " cannot use tools: it sees only what was sent, and cannot read or change files", "warn": true})
			tools = nil
			round--
			continue
		}
		if err != nil {
			return stop, err
		}
		stop = why
		if len(calls) == 0 {
			return stop, nil
		}
		all = append(all, ollamaMsg{Role: "assistant", Content: text, ToolCalls: calls})
		for _, tc := range calls {
			args := tc.Function.Arguments
			// Some models send the arguments as a JSON string.
			var s string
			if json.Unmarshal(args, &s) == nil {
				args = json.RawMessage(s)
			}
			out, err := files.call(tc.Function.Name, args)
			if err != nil {
				out = "Error: " + err.Error()
			}
			all = append(all, ollamaMsg{Role: "tool", ToolName: tc.Function.Name, Content: out})
		}
	}
	return stop, fmt.Errorf("stopped after %d rounds of tool calls without an answer", ollamaRounds)
}

// One streamed /api/chat: the text (handed to emit as it comes), the tool
// calls asked for, and why it stopped.
func ollamaChatOnce(ctx context.Context, body map[string]any, emit func(string)) (string, []ollamaToolCall, string, error) {
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", ollamaURL()+"/api/chat", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
		return "", nil, "", ollamaError(res.Status, b)
	}
	var text strings.Builder
	var calls []ollamaToolCall
	why := ""
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		var ev struct {
			Message struct {
				Content   string           `json:"content"`
				ToolCalls []ollamaToolCall `json:"tool_calls"`
			} `json:"message"`
			Done       bool   `json:"done"`
			DoneReason string `json:"done_reason"`
			Error      string `json:"error"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if ev.Error != "" {
			return text.String(), calls, why, fmt.Errorf("%s", ev.Error)
		}
		if ev.Message.Content != "" {
			text.WriteString(ev.Message.Content)
			emit(ev.Message.Content)
		}
		calls = append(calls, ev.Message.ToolCalls...)
		if ev.Done {
			why = ev.DoneReason
		}
	}
	return text.String(), calls, why, sc.Err()
}
