package orgs

// MLX for the local backend (localrt.go), on Apple Silicon only:
// mlx_lm.server from the mlx-lm package, which loads whatever model a
// request names and swaps out the last, so switching is only a different
// name.
//
// - Installed as `python -m venv` plus `pip install mlx-lm` under orgs's
//   cache folder, never into the system's python (assistant.local.mlx.python
//   picks the interpreter, default python3; mlx-lm wants 3.10 or newer).
// - Models are fetched by orgs from Hugging Face into its own folder
//   (owner/repo, safetensors and tokenizer files only; a repository that
//   ships python is refused) and named to the server by path, with
//   HF_HUB_OFFLINE set, so a chat never downloads anything and nothing runs
//   code from a repository.
// - mlx_lm.server stops at 512 tokens unless told: requests carry max_tokens
//   (assistant.local.maxTokens), and the server is started with it too.
// - It takes no context length: it grows as the text does.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

type mlxLM struct{}

func (mlxLM) id() string                         { return "mlx" }
func (mlxLM) label() string                      { return "MLX" }
func (mlxLM) conf() *common.LocalRuntimeSettings { return &localConf().MLX }
func (mlxLM) defaultURL() string                 { return "http://127.0.0.1:8092" }
func (mlxLM) listsFromServer() bool              { return false }
func (mlxLM) canCancel() bool                    { return true }

func (mlxLM) supported() error {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return fmt.Errorf("MLX runs only on a Mac with Apple Silicon")
	}
	return nil
}

func mlxVenv() (string, error) {
	d, err := localDir("mlx")
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "venv"), nil
}

func (m mlxLM) binary() (string, error) {
	if c := m.conf().Command; c != "" {
		p, err := exec.LookPath(expandHome(c))
		if err != nil {
			return "", fmt.Errorf("assistant.local.mlx.command %q is not a command the server can run", c)
		}
		return p, nil
	}
	if v, err := mlxVenv(); err == nil {
		p := filepath.Join(v, "bin", "mlx_lm.server")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	if p, err := exec.LookPath("mlx_lm.server"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("MLX is not installed on the server")
}

// The python to make the environment with: the configured one, else the
// first of the usual names that is 3.10 or newer (macOS's own python3 is
// 3.9, so Homebrew's is looked for too).
var mlxPython struct {
	sync.Mutex
	found string
}

func (m mlxLM) python() string {
	if p := m.conf().Python; p != "" {
		return expandHome(p)
	}
	mlxPython.Lock()
	defer mlxPython.Unlock()
	if mlxPython.found != "" {
		return mlxPython.found
	}
	for _, c := range []string{"python3", "python3.14", "python3.13", "python3.12", "python3.11", "python3.10", "/opt/homebrew/bin/python3", "/usr/local/bin/python3"} {
		p, err := exec.LookPath(c)
		if err != nil {
			continue
		}
		if out, err := exec.Command(p, "-c", "import sys; print(sys.version_info >= (3, 10))").Output(); err == nil && strings.TrimSpace(string(out)) == "True" {
			mlxPython.found = p
			return p
		}
	}
	return "python3"
}

func (m mlxLM) installNote() string {
	return "Makes a python environment of its own in the server's cache folder (with " + m.python() + ") and installs mlx-lm from PyPI into it, about 300 MB."
}

func (m mlxLM) install(ctx context.Context, rt *localRT) error {
	py, err := exec.LookPath(m.python())
	if err != nil {
		return fmt.Errorf("no %s on the server to make MLX's environment with (set assistant.local.mlx.python)", m.python())
	}
	out, err := exec.CommandContext(ctx, py, "-c", "import sys; print(sys.version_info >= (3, 10))").Output()
	if err != nil || strings.TrimSpace(string(out)) != "True" {
		return fmt.Errorf("%s is older than 3.10, which mlx-lm needs (set assistant.local.mlx.python)", py)
	}
	venv, err := mlxVenv()
	if err != nil {
		return err
	}
	// Made where it stays: a venv's scripts name its python by absolute
	// path, so one built elsewhere and moved would not run. A failed install
	// leaves nothing behind for binary() to find.
	os.RemoveAll(venv)
	rt.set("installing", "making MLX's python environment")
	if err := rt.runLogged(ctx, nil, py, "-m", "venv", venv); err != nil {
		os.RemoveAll(venv)
		return err
	}
	rt.set("installing", "installing mlx-lm (pip)")
	if err := rt.runLogged(ctx, nil, filepath.Join(venv, "bin", "python"), "-m", "pip", "install", "--disable-pip-version-check", "--upgrade", "mlx-lm"); err != nil {
		os.RemoveAll(venv)
		return err
	}
	if _, err := m.binary(); err != nil {
		return fmt.Errorf("pip finished but left no mlx_lm.server")
	}
	return nil
}

func (m mlxLM) launch(bin, host, port string) (*exec.Cmd, error) {
	cmd := exec.Command(bin, "--host", host, "--port", port, "--max-tokens", strconv.Itoa(localMaxTokens()))
	cmd.Env = append(os.Environ(), "HF_HUB_OFFLINE=1")
	return cmd, nil
}

func (mlxLM) halt(string) error { return nil }

// owner/repo folders holding a config.json, two levels down.
func (m mlxLM) models(ctx context.Context, rt *localRT) ([]localModel, error) {
	dir, err := rt.modelDir()
	if err != nil {
		return nil, err
	}
	found, _ := filepath.Glob(filepath.Join(dir, "*", "*", "config.json"))
	out := []localModel{}
	for _, cfg := range found {
		md := filepath.Dir(cfg)
		name := filepath.Base(filepath.Dir(md)) + "/" + filepath.Base(md)
		lm := localModel{Name: name, ToolsKnown: true}
		ents, _ := os.ReadDir(md)
		for _, e := range ents {
			if info, err := e.Info(); err == nil && !e.IsDir() {
				lm.Size += info.Size()
			}
		}
		var c struct {
			MaxPos       int `json:"max_position_embeddings"`
			Quantization struct {
				Bits int `json:"bits"`
			} `json:"quantization"`
		}
		if raw, err := os.ReadFile(cfg); err == nil && json.Unmarshal(raw, &c) == nil {
			lm.ContextMax = c.MaxPos
			if c.Quantization.Bits > 0 {
				lm.Quant = fmt.Sprintf("%d-bit", c.Quantization.Bits)
			}
		}
		lm.Params = ggufParams(filepath.Base(md))
		lm.Tools = mlxTemplateHasTools(md)
		out = append(out, lm)
	}
	return out, nil
}

// Whether its chat template knows tools (the template lives in
// tokenizer_config.json or chat_template.jinja).
func mlxTemplateHasTools(dir string) bool {
	if raw, err := os.ReadFile(filepath.Join(dir, "chat_template.jinja")); err == nil && strings.Contains(string(raw), "tools") {
		return true
	}
	var tc struct {
		Template any `json:"chat_template"`
	}
	raw, err := os.ReadFile(filepath.Join(dir, "tokenizer_config.json"))
	if err != nil || json.Unmarshal(raw, &tc) != nil {
		return false
	}
	b, _ := json.Marshal(tc.Template)
	return strings.Contains(string(b), "tools")
}

func (mlxLM) validName(n string) bool { return hfRepoRe.MatchString(n) }

func (m mlxLM) pull(ctx context.Context, rt *localRT, p *localPull) error {
	rt.progress(p, "looking it up", 0, 0)
	files, err := hfTree(ctx, p.Model)
	if err != nil {
		return err
	}
	want := []hfFile{}
	var total int64
	weights := false
	for _, f := range files {
		if strings.HasSuffix(f.Path, ".py") {
			return fmt.Errorf("%s ships python code that would have to run to load it; orgs fetches only models that need none", p.Model)
		}
		if _, ok := hfCleanPath(f.Path); !ok || !localSafeFile(f.Path) || strings.HasSuffix(f.Path, ".gguf") {
			continue
		}
		if strings.HasSuffix(f.Path, ".safetensors") {
			weights = true
		}
		want = append(want, f)
		total += f.Size
	}
	if !weights {
		return fmt.Errorf("%s has no safetensors weights: MLX models are usually under mlx-community/", p.Model)
	}
	dir, err := rt.modelDir()
	if err != nil {
		return err
	}
	dest := filepath.Join(dir, filepath.FromSlash(p.Model))
	tmp := dest + ".part"
	rt.progress(p, "downloading", 0, total)
	for _, f := range want {
		target := filepath.Join(tmp, filepath.FromSlash(f.Path))
		if st, err := os.Stat(target); err == nil && st.Size() == f.Size {
			rt.mu.Lock()
			p.Done += f.Size
			rt.mu.Unlock()
			continue // fetched by an earlier, cancelled pull
		}
		rt.mu.Lock()
		p.Status = "downloading " + path.Base(f.Path)
		rt.mu.Unlock()
		if err := hfDownload(ctx, p.Model, f.Path, target, &p.Done, &rt.mu); err != nil {
			return err
		}
	}
	os.RemoveAll(dest)
	return os.Rename(tmp, dest)
}

func (m mlxLM) remove(ctx context.Context, rt *localRT, name string) error {
	if !hfRepoRe.MatchString(name) {
		return fmt.Errorf("%q is not a model name", name)
	}
	dir, err := rt.modelDir()
	if err != nil {
		return err
	}
	md := filepath.Join(dir, filepath.FromSlash(name))
	if !insideDir(dir, md) {
		return fmt.Errorf("%q is not a model in %s", name, dir)
	}
	if _, err := os.Stat(filepath.Join(md, "config.json")); err != nil {
		return fmt.Errorf("no model %s in %s", name, dir)
	}
	return os.RemoveAll(md)
}

func (m mlxLM) prepare(ctx context.Context, rt *localRT, name string, numCtx int, note func(string)) (string, error) {
	if err := rt.ensure(60 * time.Second); err != nil {
		return "", err
	}
	dir, err := rt.modelDir()
	if err != nil {
		return "", err
	}
	md := filepath.Join(dir, filepath.FromSlash(name))
	if _, err := os.Stat(filepath.Join(md, "config.json")); err != nil || !insideDir(dir, md) {
		return "", fmt.Errorf("MLX has no model %s", name)
	}
	return md, nil
}

func (mlxLM) suggested() []localSuggestion {
	return []localSuggestion{
		{"mlx-community/Qwen3-4B-4bit", "Small and quick; fine for a laptop. Uses tools.", 2_300_000_000, "", false},
		{"mlx-community/Qwen3-8B-4bit", "A good default: careful with tools, decent prose.", 4_623_784_971, "", false},
		{"mlx-community/Qwen3-14B-4bit", "Better writing; wants 16GB of memory or more.", 8_323_870_520, "", false},
		{"mlx-community/gpt-oss-20b-MXFP4-Q8", "OpenAI's open model; strong, wants 16GB or more.", 12_104_218_243, "", false},
		{"mlx-community/Meta-Llama-3.1-8B-Instruct-4bit", "Meta's; plain and reliable.", 4_526_698_444, "", false},
		{"mlx-community/Mistral-Nemo-Instruct-2407-4bit", "Mistral's; good with long text.", 6_905_203_123, "", false},
	}
}
