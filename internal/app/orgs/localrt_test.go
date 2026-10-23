package orgs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// llama.cpp sends a tool call's name and then its arguments a few
// characters at a time, by index.
func TestOpenAIStreamJoinsPieces(t *testing.T) {
	var s openAIStream
	for _, ev := range []string{
		`{"choices":[{"delta":{"role":"assistant","content":null}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a1","type":"function","function":{"name":"read_file","arguments":"{"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"file\": "}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"notes.org\"}"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"a2","function":{"name":"find","arguments":"{\"text\":\"x\"}"}}]}}]}`,
		`{"choices":[{"finish_reason":"tool_calls","delta":{}}]}`,
	} {
		if _, err := s.add([]byte(ev)); err != nil {
			t.Fatal(err)
		}
	}
	calls := s.calls()
	if len(calls) != 2 || calls[0].Function.Name != "read_file" || calls[0].Function.Arguments != `{"file": "notes.org"}` || calls[0].ID != "a1" || calls[1].Function.Name != "find" {
		t.Fatalf("calls: %+v", calls)
	}
	if s.stop != "tool_calls" {
		t.Fatalf("stop %q", s.stop)
	}
}

// MLX sends each call whole; some servers number every call 0, told apart by id.
func TestOpenAIStreamWholeCallsAndRepeatedIndex(t *testing.T) {
	var s openAIStream
	s.add([]byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"x","type":"function","function":{"name":"a","arguments":"{}"}}]}}]}`))
	s.add([]byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"y","type":"function","function":{"name":"b","arguments":"{}"}}]}}]}`))
	s.add([]byte(`{"choices":[{"delta":{"tool_calls":[{"function":{"name":"c","arguments":""}}]}}]}`))
	calls := s.calls()
	if len(calls) != 3 || calls[0].Function.Name != "a" || calls[1].Function.Name != "b" || calls[2].Function.Name != "c" || calls[2].ID == "" {
		t.Fatalf("calls: %+v", calls)
	}
}

type fakeTools struct {
	mu    sync.Mutex
	asked []string
}

func (f *fakeTools) toolDefs() []codeTool {
	return []codeTool{{Name: "read_file", Description: "read", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}}}}
}

func (f *fakeTools) call(name string, args json.RawMessage) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, name+" "+string(args))
	return "Chapter one: the lighthouse.", nil
}

// The loop: a tool call, its result sent back as a tool message with the
// call's id, then the answer.
func TestOpenAIChatToolLoop(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b map[string]any
		json.Unmarshal(raw, &b)
		bodies = append(bodies, b)
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("no key sent")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if len(bodies) == 1 {
			fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"Let me look."}}]}`+"\n\n")
			fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"read_file","arguments":"{\"file\":\"a.org\"}"}}]}}]}`+"\n\n")
			fmt.Fprint(w, `data: {"choices":[{"finish_reason":"tool_calls","delta":{}}]}`+"\n\n")
		} else {
			fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"It is about a lighthouse."}}]}`+"\n\n")
			fmt.Fprint(w, `data: {"choices":[{"finish_reason":"stop","delta":{}}]}`+"\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	tools := &fakeTools{}
	var text strings.Builder
	stop, err := openAIChat(context.Background(), openAIConn{base: srv.URL, key: "k", model: "m", extra: map[string]any{"max_tokens": 99}},
		"sys", []assistantMsg{{Role: "user", Text: "what is a.org about?"}}, tools, func(s string) { text.WriteString(s) }, func(any) {})
	if err != nil {
		t.Fatal(err)
	}
	if stop != "stop" || text.String() != "Let me look.\n\nIt is about a lighthouse." {
		t.Fatalf("stop %q text %q", stop, text.String())
	}
	if len(tools.asked) != 1 || tools.asked[0] != `read_file {"file":"a.org"}` {
		t.Fatalf("asked %v", tools.asked)
	}
	if len(bodies) != 2 || bodies[0]["max_tokens"] != float64(99) || bodies[0]["tools"] == nil {
		t.Fatalf("first request %v", bodies[0])
	}
	msgs := bodies[1]["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)
	prev := msgs[len(msgs)-2].(map[string]any)
	if last["role"] != "tool" || last["tool_call_id"] != "c1" || last["content"] != "Chapter one: the lighthouse." {
		t.Fatalf("tool message %v", last)
	}
	if prev["role"] != "assistant" || prev["tool_calls"] == nil {
		t.Fatalf("assistant message %v", prev)
	}
	if _, has := prev["tool_calls"].([]any)[0].(map[string]any)["index"]; has {
		t.Fatalf("index sent back: OpenAI refuses it")
	}
}

// A server that refuses tools for the model is asked again without them.
func TestOpenAIChatRetriesWithoutTools(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		if b["tools"] != nil {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":{"message":"this model does not support tools"}}`)
			return
		}
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"plain"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()
	var warned bool
	_, err := openAIChat(context.Background(), openAIConn{base: srv.URL, model: "m"}, "sys",
		[]assistantMsg{{Role: "user", Text: "hi"}}, &fakeTools{}, func(string) {}, func(v any) { warned = true })
	if err != nil || n != 2 || !warned {
		t.Fatalf("err %v, %d requests, warned %v", err, n, warned)
	}
}

func TestWhereURL(t *testing.T) {
	for in, want := range map[string]string{
		"http://127.0.0.1:8091":       "machine",
		"http://localhost:1234/v1":    "machine",
		"http://[::1]:11434":          "machine",
		"http://192.168.1.20:8000/v1": "network",
		"http://10.0.0.5:8000":        "network",
		"http://gpubox:8000/v1":       "network",
		"http://studio.local:1234":    "network",
		"https://api.openai.com/v1":   "cloud",
		"https://gateway.example.com": "cloud",
		"http://8.8.8.8:8000":         "cloud",
		"":                            "cloud",
	} {
		if got := whereURL(in); got != want {
			t.Errorf("whereURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLlamaAssetSuffix(t *testing.T) {
	for _, c := range []struct{ goos, arch, variant, want string }{
		{"darwin", "arm64", "", "bin-macos-arm64.tar.gz"},
		{"darwin", "amd64", "vulkan", "bin-macos-x64.tar.gz"},
		{"linux", "amd64", "", "bin-ubuntu-x64.tar.gz"},
		{"linux", "amd64", "vulkan", "bin-ubuntu-vulkan-x64.tar.gz"},
		{"linux", "arm64", "cuda-13.4", "bin-ubuntu-cuda-13.4-arm64.tar.gz"},
		{"windows", "amd64", "", "bin-win-cpu-x64.zip"},
		{"windows", "amd64", "cuda-12.4", "bin-win-cuda-12.4-x64.zip"},
	} {
		got, err := llamaAssetSuffix(c.goos, c.arch, c.variant)
		if err != nil || got != c.want {
			t.Errorf("%v: %q %v", c, got, err)
		}
	}
	if _, err := llamaAssetSuffix("freebsd", "amd64", ""); err == nil {
		t.Error("freebsd has no build")
	}
}

func TestPickGGUF(t *testing.T) {
	files := []hfFile{
		{Path: "Qwen3-8B-Q4_K_M.gguf"}, {Path: "Qwen3-8B-Q8_0.gguf"}, {Path: "mmproj-Q4_K_M.gguf"},
		{Path: "big/Big-Q6_K-00001-of-00002.gguf"}, {Path: "big/Big-Q6_K-00002-of-00002.gguf"}, {Path: "README.md"},
	}
	if f, err := pickGGUF(files, "", ""); err != nil || f.Path != "Qwen3-8B-Q4_K_M.gguf" {
		t.Fatalf("default: %v %v", f, err)
	}
	if f, err := pickGGUF(files, "", "q8_0"); err != nil || f.Path != "Qwen3-8B-Q8_0.gguf" {
		t.Fatalf("q8: %v %v", f, err)
	}
	if _, err := pickGGUF(files, "", "Q6_K"); err == nil || !strings.Contains(err.Error(), "split") {
		t.Fatalf("split: %v", err)
	}
	if _, err := pickGGUF(files, "big/Big-Q6_K-00001-of-00002.gguf", ""); err == nil {
		t.Fatal("a named part is still a split model")
	}
	repo, file, quant := parseLlamaRef("Qwen/Qwen3-8B-GGUF:Q8_0")
	if repo != "Qwen/Qwen3-8B-GGUF" || file != "" || quant != "Q8_0" {
		t.Fatal(repo, file, quant)
	}
	repo, file, _ = parseLlamaRef("bartowski/X-GGUF/sub/X-Q4_K_M.gguf")
	if repo != "bartowski/X-GGUF" || file != "sub/X-Q4_K_M.gguf" {
		t.Fatal(repo, file)
	}
	if (llamaCpp{}).validName("../etc:Q4") || !(llamaCpp{}).validName("Qwen/Qwen3-8B-GGUF:Q4_K_M") {
		t.Fatal("validName")
	}
}

// Nothing that runs code, and nothing that climbs out of its folder.
func TestLocalSafeFiles(t *testing.T) {
	for p, ok := range map[string]bool{
		"model.safetensors": true, "tokenizer.json": true, "chat_template.jinja": true, "x.gguf": true,
		"pytorch_model.bin": false, "model.pt": false, "modeling_x.py": false, "x.pkl": false,
	} {
		if localSafeFile(p) != ok {
			t.Errorf("localSafeFile(%q) != %v", p, ok)
		}
	}
	for _, p := range []string{"../x.json", "/etc/x.json", "a/../../x.json", "a\\b.json", ""} {
		if _, ok := hfCleanPath(p); ok {
			t.Errorf("hfCleanPath(%q) allowed", p)
		}
	}
	if !insideDir("/a/models", "/a/models/q/r") || insideDir("/a/models", "/a/models") || insideDir("/a/models", "/a/modelsx/y") {
		t.Error("insideDir")
	}
}

func TestInstallerProgressLines(t *testing.T) {
	for in, want := range map[string]string{
		"##################   34.1%": "34.1",
		"                      0.0%": "0.0",
		"100%":                       "100",
	} {
		m := progressBarRe.FindStringSubmatch(strings.TrimSpace(in))
		if m == nil || m[1] != want {
			t.Errorf("%q: %v", in, m)
		}
	}
	if progressBarRe.MatchString("Verifying checksum...") || progressBarRe.MatchString("Downloading llmster 0.0.25-1 Darwin arm64") {
		t.Error("a message read as a bar")
	}
}

// Gemini puts a thought signature on a tool call and refuses the next turn
// unless it comes back: whatever else is on a call is sent back as it came.
func TestOpenAIToolCallKeepsExtraFields(t *testing.T) {
	var s openAIStream
	s.add([]byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"g1","type":"function","function":{"name":"read_file","arguments":"{}"},"extra_content":{"google":{"thought_signature":"SIG"}}}]}}]}`))
	calls := s.calls()
	raw, _ := json.Marshal(calls[0])
	var back map[string]any
	json.Unmarshal(raw, &back)
	ec, _ := back["extra_content"].(map[string]any)
	g, _ := ec["google"].(map[string]any)
	if g["thought_signature"] != "SIG" || back["id"] != "g1" || back["index"] != nil {
		t.Fatalf("sent back: %s", raw)
	}
}
