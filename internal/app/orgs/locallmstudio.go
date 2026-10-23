package orgs

// LM Studio for the local backend (localrt.go), through llmster - LM Studio
// without its window - and its lms command, which the desktop app installs
// too. Models, downloads and loading are LM Studio's own REST API
// (/api/v1/models, /models/download, /models/load, /models/unload); the chat
// is its OpenAI-compatible /v1.
//
// - Installing runs LM Studio's own installer (lmstudio.ai/install.sh, or
//   install.ps1 on Windows), saved to a file first, told not to touch the
//   shell profiles. It puts lms in ~/.lmstudio/bin.
// - Starting is `lms daemon up` then `lms server start --port`: both return
//   at once and leave the daemon running, so orgs stops it with `lms server
//   stop` rather than by a child process.
// - Switching models unloads what is loaded and loads the one asked for with
//   the context length, since LM Studio would otherwise keep both in memory.
// - LM Studio's API cannot delete a model or cancel a download: those are
//   done in LM Studio, and worg does not offer them.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

type lmStudio struct{}

func (lmStudio) id() string                         { return "lmstudio" }
func (lmStudio) label() string                      { return "LM Studio" }
func (lmStudio) conf() *common.LocalRuntimeSettings { return &localConf().LMStudio }
func (lmStudio) defaultURL() string                 { return "http://127.0.0.1:1234" }
func (lmStudio) listsFromServer() bool              { return true }
func (lmStudio) canCancel() bool                    { return false }

func (lmStudio) supported() error {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64", "linux/amd64", "linux/arm64", "windows/amd64", "windows/arm64":
		return nil
	}
	return fmt.Errorf("LM Studio does not run on %s/%s", runtime.GOOS, runtime.GOARCH)
}

func (l lmStudio) binary() (string, error) {
	if c := l.conf().Command; c != "" {
		p, err := exec.LookPath(expandHome(c))
		if err != nil {
			return "", fmt.Errorf("assistant.local.lmstudio.command %q is not a command the server can run", c)
		}
		return p, nil
	}
	if p, err := exec.LookPath("lms"); err == nil {
		return p, nil
	}
	exe := "lms"
	if runtime.GOOS == "windows" {
		exe = "lms.exe"
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{filepath.Join(home, ".lmstudio", "bin", exe), filepath.Join(home, ".cache", "lm-studio", "bin", exe)} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("LM Studio is not installed on the server")
}

func (lmStudio) installNote() string {
	return "Runs LM Studio's own installer (from lmstudio.ai), which puts llmster, LM Studio without its window, and the lms command in ~/.lmstudio. Models come from LM Studio's catalogue."
}

func (l lmStudio) install(ctx context.Context, rt *localRT) error {
	dir, err := localDir("lmstudio")
	if err != nil {
		return err
	}
	src, shell := "https://lmstudio.ai/install.sh", []string{"sh"}
	if runtime.GOOS == "windows" {
		src, shell = "https://lmstudio.ai/install.ps1", []string{"powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File"}
	}
	rt.set("installing", "downloading LM Studio's installer")
	got, err := rt.download(ctx, src, dir)
	if err != nil {
		return err
	}
	script := got + filepath.Ext(src)
	if err := os.Rename(got, script); err != nil {
		return err
	}
	defer os.Remove(script)
	rt.set("installing", "installing LM Studio (llmster): this downloads its runtime, a few hundred MB")
	args := append(shell[1:], script)
	if runtime.GOOS != "windows" {
		args = append(args, "--no-modify-path")
	}
	if err := rt.runLogged(ctx, []string{"LMS_NO_MODIFY_PATH=1"}, shell[0], args...); err != nil {
		return err
	}
	if _, err := l.binary(); err != nil {
		return fmt.Errorf("the installer finished but left no lms command in ~/.lmstudio/bin")
	}
	return nil
}

func (l lmStudio) launch(bin, host, port string) (*exec.Cmd, error) {
	rt := localRuntimeNamed("lmstudio")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := rt.runLogged(ctx, nil, bin, "daemon", "up"); err != nil {
		return nil, fmt.Errorf("lms daemon up: %v", err)
	}
	if err := rt.runLogged(ctx, nil, bin, "server", "start", "--port", port); err != nil {
		return nil, fmt.Errorf("lms server start: %v", err)
	}
	return nil, nil
}

func (lmStudio) halt(bin string) error {
	return exec.Command(bin, "server", "stop").Run()
}

type lmsModel struct {
	Type         string `json:"type"`
	Key          string `json:"key"`
	SizeBytes    int64  `json:"size_bytes"`
	ParamsString string `json:"params_string"`
	Quantization struct {
		Name string `json:"name"`
	} `json:"quantization"`
	MaxContext      int `json:"max_context_length"`
	LoadedInstances []struct {
		ID string `json:"id"`
	} `json:"loaded_instances"`
	Capabilities struct {
		Tools bool `json:"trained_for_tool_use"`
	} `json:"capabilities"`
}

func lmsModels(ctx context.Context, rt *localRT) ([]lmsModel, error) {
	var out struct {
		Models []lmsModel `json:"models"`
	}
	if err := localCall(ctx, "GET", rt.url()+"/api/v1/models", nil, &out); err != nil {
		return nil, err
	}
	return out.Models, nil
}

func (l lmStudio) models(ctx context.Context, rt *localRT) ([]localModel, error) {
	ms, err := lmsModels(ctx, rt)
	if err != nil {
		return nil, err
	}
	out := []localModel{}
	for _, m := range ms {
		if m.Type != "llm" {
			continue
		}
		out = append(out, localModel{Name: m.Key, Size: m.SizeBytes, Params: m.ParamsString, Quant: m.Quantization.Name,
			Tools: m.Capabilities.Tools, ToolsKnown: true, ContextMax: m.MaxContext, Loaded: len(m.LoadedInstances) > 0})
	}
	return out, nil
}

var lmsKeyRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/@:-]{0,199}$`)

func (lmStudio) validName(n string) bool { return lmsKeyRe.MatchString(n) }

func (l lmStudio) pull(ctx context.Context, rt *localRT, p *localPull) error {
	var job struct {
		JobID  string `json:"job_id"`
		Status string `json:"status"`
		Total  int64  `json:"total_size_bytes"`
	}
	if err := localCall(ctx, "POST", rt.url()+"/api/v1/models/download", map[string]any{"model": p.Model}, &job); err != nil {
		return err
	}
	for {
		switch job.Status {
		case "completed", "already_downloaded":
			return nil
		case "failed":
			return fmt.Errorf("LM Studio could not download %s", p.Model)
		}
		if job.JobID == "" {
			return fmt.Errorf("LM Studio gave no download to follow (%s)", job.Status)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
		var st struct {
			Status string `json:"status"`
			Total  int64  `json:"total_size_bytes"`
			Done   int64  `json:"downloaded_bytes"`
		}
		if err := localCall(ctx, "GET", rt.url()+"/api/v1/models/download/status/"+job.JobID, nil, &st); err != nil {
			return err
		}
		job.Status = st.Status
		rt.progress(p, st.Status, st.Done, st.Total)
	}
}

func (lmStudio) remove(context.Context, *localRT, string) error {
	return fmt.Errorf("LM Studio's API cannot delete a model: remove it in LM Studio (My Models), or with lms on the server")
}

func (l lmStudio) prepare(ctx context.Context, rt *localRT, name string, numCtx int, note func(string)) (string, error) {
	if err := rt.ensure(60 * time.Second); err != nil {
		return "", err
	}
	ms, err := lmsModels(ctx, rt)
	if err != nil {
		return "", err
	}
	found := false
	for _, m := range ms {
		if m.Key == name {
			found = true
			if len(m.LoadedInstances) > 0 {
				return name, nil
			}
		}
	}
	if !found {
		return "", fmt.Errorf("LM Studio has no model %s", name)
	}
	for _, m := range ms {
		if m.Type != "llm" {
			continue
		}
		for _, in := range m.LoadedInstances {
			localCall(ctx, "POST", rt.url()+"/api/v1/models/unload", map[string]any{"instance_id": in.ID}, nil)
		}
	}
	note("Loading " + name + " into LM Studio")
	n := numCtx
	if n <= 0 {
		n = localContext()
	}
	if err := localCall(ctx, "POST", rt.url()+"/api/v1/models/load", map[string]any{"model": name, "context_length": n}, nil); err != nil {
		return "", fmt.Errorf("loading %s: %v", name, err)
	}
	rt.forget()
	return name, nil
}

func (lmStudio) suggested() []localSuggestion {
	return []localSuggestion{
		{"qwen/qwen3-4b", "Small and quick; fine for a laptop. Uses tools.", 2_500_000_000, "", false},
		{"qwen/qwen3-8b", "A good default: careful with tools, decent prose.", 5_000_000_000, "", false},
		{"qwen/qwen3-14b", "Better writing; wants 16GB of memory or more.", 9_000_000_000, "", false},
		{"openai/gpt-oss-20b", "OpenAI's open model; strong, wants 16GB or more.", 12_100_000_000, "", false},
		{"mistralai/mistral-nemo-instruct-2407", "Mistral's; good with long text.", 7_500_000_000, "", false},
		{"ibm/granite-4-micro", "Tiny and fast; short answers.", 2_300_000_000, "", false},
	}
}
