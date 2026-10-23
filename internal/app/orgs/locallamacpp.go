package orgs

// llama.cpp for the local backend (localrt.go): llama-server in router mode
// (--models-dir), which serves every .gguf in orgs's model folder by its file
// name and loads the one a request names, swapping out the last
// (--models-max 1), so switching models is only a different name.
//
// - Installed from the newest numbered build on GitHub (bNNNNN; the
//   "latest" release is a library version without binaries), unpacked whole,
//   since llama-server needs the libraries beside it. On Linux and Windows
//   assistant.local.llamacpp.variant picks the build (cpu by default; vulkan,
//   cuda-12.8...); a Windows CUDA build also gets its cudart archive.
// - The router reads the folder only when it starts, so a model added or
//   removed marks it stale and it is restarted before it is next used.
// - Models are fetched from Hugging Face by orgs (localhf.go), one .gguf
//   each: owner/repo:QUANT (Q4_K_M by default) or owner/repo/file.gguf. A
//   model split over several files is refused.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

type llamaCpp struct{}

func (llamaCpp) id() string                         { return "llamacpp" }
func (llamaCpp) label() string                      { return "llama.cpp" }
func (llamaCpp) conf() *common.LocalRuntimeSettings { return &localConf().Llamacpp }
func (llamaCpp) defaultURL() string                 { return "http://127.0.0.1:8091" }
func (llamaCpp) listsFromServer() bool              { return false }
func (llamaCpp) canCancel() bool                    { return true }

func (llamaCpp) supported() error {
	if _, err := llamaAssetSuffix(runtime.GOOS, runtime.GOARCH, ""); err != nil {
		return err
	}
	return nil
}

func llamaExe() string {
	if runtime.GOOS == "windows" {
		return "llama-server.exe"
	}
	return "llama-server"
}

func (l llamaCpp) binary() (string, error) {
	if c := l.conf().Command; c != "" {
		p, err := exec.LookPath(expandHome(c))
		if err != nil {
			return "", fmt.Errorf("assistant.local.llamacpp.command %q is not a command the server can run", c)
		}
		return p, nil
	}
	if p, err := exec.LookPath("llama-server"); err == nil {
		return p, nil
	}
	if p := llamaInstalled(); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("llama.cpp is not installed on the server")
}

// The newest build orgs installed: <cache>/orgs/llamacpp/bNNNN/.../llama-server.
func llamaInstalled() string {
	dir, err := localDir("llamacpp")
	if err != nil {
		return ""
	}
	builds, _ := os.ReadDir(dir)
	sort.Slice(builds, func(i, j int) bool { return llamaBuildNum(builds[i].Name()) > llamaBuildNum(builds[j].Name()) })
	for _, b := range builds {
		if !b.IsDir() || llamaBuildNum(b.Name()) == 0 {
			continue
		}
		for _, pat := range []string{"*", "*/*", "*/*/*"} {
			m, _ := filepath.Glob(filepath.Join(dir, b.Name(), pat, llamaExe()))
			if len(m) == 0 && pat == "*" {
				m, _ = filepath.Glob(filepath.Join(dir, b.Name(), llamaExe()))
			}
			if len(m) > 0 {
				return m[0]
			}
		}
	}
	return ""
}

func llamaBuildNum(tag string) int {
	if !strings.HasPrefix(tag, "b") {
		return 0
	}
	n, _ := strconv.Atoi(tag[1:])
	return n
}

// The end of a release file's name for this machine and variant:
// "bin-macos-arm64.tar.gz", "bin-ubuntu-vulkan-x64.tar.gz", "bin-win-cpu-x64.zip".
func llamaAssetSuffix(goos, goarch, variant string) (string, error) {
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[goarch]
	if arch == "" {
		return "", fmt.Errorf("llama.cpp publishes no build for %s/%s", goos, goarch)
	}
	v := strings.ToLower(strings.TrimSpace(variant))
	switch goos {
	case "darwin":
		return "bin-macos-" + arch + ".tar.gz", nil
	case "linux":
		if v == "" || v == "cpu" {
			return "bin-ubuntu-" + arch + ".tar.gz", nil
		}
		return "bin-ubuntu-" + v + "-" + arch + ".tar.gz", nil
	case "windows":
		if v == "" {
			v = "cpu"
		}
		return "bin-win-" + v + "-" + arch + ".zip", nil
	}
	return "", fmt.Errorf("llama.cpp publishes no build for %s/%s", goos, goarch)
}

func (l llamaCpp) installNote() string {
	s, _ := llamaAssetSuffix(runtime.GOOS, runtime.GOARCH, l.conf().Variant)
	return "Downloads the newest llama.cpp build from GitHub (llama-…-" + s + ", about 15-60 MB) into the server's cache folder."
}

type ghRelease struct {
	Tag    string `json:"tag_name"`
	Draft  bool   `json:"draft"`
	Assets []struct {
		Name string `json:"name"`
		Url  string `json:"browser_download_url"`
	} `json:"assets"`
}

// The newest numbered build that has the file for this machine.
func llamaRelease(ctx context.Context, suffix string) (tag, file, src string, rel *ghRelease, err error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/ggml-org/llama.cpp/releases?per_page=15", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", "", nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", "", "", nil, fmt.Errorf("GitHub said %s for llama.cpp's releases", res.Status)
	}
	var rels []ghRelease
	if err := json.NewDecoder(io.LimitReader(res.Body, 32<<20)).Decode(&rels); err != nil {
		return "", "", "", nil, err
	}
	for i := range rels {
		r := &rels[i]
		if r.Draft || llamaBuildNum(r.Tag) == 0 {
			continue
		}
		want := "llama-" + r.Tag + "-" + suffix
		for _, a := range r.Assets {
			if a.Name == want {
				return r.Tag, a.Name, a.Url, r, nil
			}
		}
	}
	return "", "", "", nil, fmt.Errorf("no recent llama.cpp build has a file ending %s (check assistant.local.llamacpp.variant)", suffix)
}

func (l llamaCpp) install(ctx context.Context, rt *localRT) error {
	suffix, err := llamaAssetSuffix(runtime.GOOS, runtime.GOARCH, l.conf().Variant)
	if err != nil {
		return err
	}
	dir, err := localDir("llamacpp")
	if err != nil {
		return err
	}
	tag, file, src, rel, err := llamaRelease(ctx, suffix)
	if err != nil {
		return err
	}
	srcs := [][2]string{{file, src}}
	if runtime.GOOS == "windows" && strings.HasPrefix(strings.ToLower(l.conf().Variant), "cuda") {
		// The CUDA runtime comes in an archive of its own.
		want := "cudart-llama-bin-" + strings.TrimPrefix(strings.TrimSuffix(suffix, ".zip"), "bin-") + ".zip"
		for _, a := range rel.Assets {
			if a.Name == want {
				srcs = append(srcs, [2]string{a.Name, a.Url})
			}
		}
	}
	dest := filepath.Join(dir, tag)
	tmp := dest + ".part"
	os.RemoveAll(tmp)
	for _, s := range srcs {
		rt.set("installing", "downloading "+s[0])
		fmt.Fprintf(os.Stderr, "LOCAL: downloading %s\n", s[1])
		got, err := rt.download(ctx, s[1], dir)
		if err != nil {
			os.RemoveAll(tmp)
			return err
		}
		rt.set("installing", "unpacking "+s[0])
		err = unpackOllama(got, s[0], tmp)
		os.Remove(got)
		if err != nil {
			os.RemoveAll(tmp)
			return err
		}
	}
	os.RemoveAll(dest)
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	if llamaInstalled() == "" {
		return fmt.Errorf("no %s in %s", llamaExe(), file)
	}
	fmt.Fprintf(os.Stderr, "LOCAL: installed llama.cpp %s in %s\n", tag, dest)
	return nil
}

func (l llamaCpp) launch(bin, host, port string) (*exec.Cmd, error) {
	dir, err := localRuntimeNamed("llamacpp").modelDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, "--models-dir", dir, "--models-max", "1", "--host", host, "--port", port,
		"--no-webui", "-c", strconv.Itoa(localContext()))
	// Its libraries are beside it.
	cmd.Dir = filepath.Dir(bin)
	return cmd, nil
}

func (llamaCpp) halt(string) error { return nil }

// The .gguf files in the folder; context and loaded state from the router
// when it is up.
func (l llamaCpp) models(ctx context.Context, rt *localRT) ([]localModel, error) {
	dir, err := rt.modelDir()
	if err != nil {
		return nil, err
	}
	ents, _ := os.ReadDir(dir)
	out := []localModel{}
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(n), ".gguf") || strings.HasPrefix(strings.ToLower(n), "mmproj") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		m := localModel{Name: strings.TrimSuffix(n, filepath.Ext(n)), Size: info.Size()}
		m.Quant = ggufQuant(m.Name)
		m.Params = ggufParams(m.Name)
		out = append(out, m)
	}
	var live struct {
		Data []struct {
			ID            string `json:"id"`
			ContextLength int    `json:"context_length"`
			Status        struct {
				Value string `json:"value"`
			} `json:"status"`
		} `json:"data"`
	}
	if rt.alive(time.Second) && localCall(ctx, "GET", rt.url()+"/v1/models", nil, &live) == nil {
		for i := range out {
			for _, d := range live.Data {
				if d.ID == out[i].Name {
					out[i].ContextMax = d.ContextLength
					out[i].Loaded = d.Status.Value == "loaded"
				}
			}
		}
	}
	return out, nil
}

var ggufQuantRe = regexp.MustCompile(`(?i)(IQ\d_[A-Z]+|Q\d_K_[SML]|Q\d_K|Q\d_\d|BF16|F16|F32|MXFP4)`)
var ggufParamsRe = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?[BM])(?:-|_|$)`)

func ggufQuant(name string) string { return strings.ToUpper(ggufQuantRe.FindString(name)) }
func ggufParams(name string) string {
	if m := ggufParamsRe.FindStringSubmatch(name); m != nil {
		return strings.ToUpper(m[1])
	}
	return ""
}

var llamaRefRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*(:[A-Za-z0-9_]+|/[A-Za-z0-9._/-]+\.gguf)?$`)

func (llamaCpp) validName(n string) bool { return llamaRefRe.MatchString(n) && len(n) < 300 }

// owner/repo:QUANT, owner/repo/file.gguf, or owner/repo (Q4_K_M).
func parseLlamaRef(ref string) (repo, file, quant string) {
	parts := strings.SplitN(ref, "/", 3)
	if len(parts) == 3 {
		return parts[0] + "/" + parts[1], parts[2], ""
	}
	repo, quant, _ = strings.Cut(ref, ":")
	return repo, "", quant
}

var ggufSplitRe = regexp.MustCompile(`(?i)-\d{5}-of-\d{5}\.gguf$`)

// The repository's file the reference names: the one asked for, or the
// single (unsplit, not mmproj) .gguf with the quantisation in its name.
func pickGGUF(files []hfFile, file, quant string) (hfFile, error) {
	if file != "" {
		for _, f := range files {
			if f.Path == file {
				if ggufSplitRe.MatchString(f.Path) {
					return f, fmt.Errorf("%s is one part of a model split over several files, which orgs does not fetch; pick a single-file quantisation", f.Path)
				}
				return f, nil
			}
		}
		return hfFile{}, fmt.Errorf("the repository has no %s", file)
	}
	if quant == "" {
		quant = "Q4_K_M"
	}
	var split bool
	cands := []hfFile{}
	for _, f := range files {
		low := strings.ToLower(path.Base(f.Path))
		if !strings.HasSuffix(low, ".gguf") || strings.HasPrefix(low, "mmproj") {
			continue
		}
		if !strings.Contains(strings.ToUpper(low), strings.ToUpper(quant)) {
			continue
		}
		if ggufSplitRe.MatchString(low) {
			split = true
			continue
		}
		cands = append(cands, f)
	}
	switch {
	case len(cands) == 1:
		return cands[0], nil
	case len(cands) > 1:
		names := []string{}
		for _, c := range cands {
			names = append(names, c.Path)
		}
		return hfFile{}, fmt.Errorf("more than one file is %s: name one (owner/repo/file.gguf): %s", quant, strings.Join(names, ", "))
	case split:
		return hfFile{}, fmt.Errorf("the %s of this model is split over several files, which orgs does not fetch; pick a smaller quantisation", quant)
	}
	return hfFile{}, fmt.Errorf("the repository has no single .gguf file for %s", quant)
}

func (l llamaCpp) pull(ctx context.Context, rt *localRT, p *localPull) error {
	repo, file, quant := parseLlamaRef(p.Model)
	rt.progress(p, "looking it up", 0, 0)
	files, err := hfTree(ctx, repo)
	if err != nil {
		return err
	}
	f, err := pickGGUF(files, file, quant)
	if err != nil {
		return err
	}
	dir, err := rt.modelDir()
	if err != nil {
		return err
	}
	dest := filepath.Join(dir, path.Base(f.Path))
	if st, err := os.Stat(dest); err == nil && st.Size() == f.Size {
		return nil
	}
	rt.progress(p, "downloading "+path.Base(f.Path), 0, f.Size)
	if err := hfDownload(ctx, repo, f.Path, dest, &p.Done, &rt.mu); err != nil {
		return err
	}
	rt.markStale()
	return nil
}

func (l llamaCpp) remove(ctx context.Context, rt *localRT, name string) error {
	dir, err := rt.modelDir()
	if err != nil {
		return err
	}
	f := filepath.Join(dir, name+".gguf")
	if !insideDir(dir, f) || filepath.Dir(f) != filepath.Clean(dir) {
		return fmt.Errorf("%q is not a model in %s", name, dir)
	}
	if _, err := os.Stat(f); err != nil {
		return fmt.Errorf("no model %s in %s", name, dir)
	}
	if err := os.Remove(f); err != nil {
		return err
	}
	rt.markStale()
	return nil
}

func (l llamaCpp) prepare(ctx context.Context, rt *localRT, name string, numCtx int, note func(string)) (string, error) {
	rt.mu.Lock()
	stale := rt.stale
	rt.mu.Unlock()
	if stale {
		note("Restarting llama.cpp to pick up the models added or removed")
	}
	if err := rt.ensure(60 * time.Second); err != nil {
		return "", err
	}
	rt.mu.Lock()
	adopted := rt.state == "adopted"
	rt.mu.Unlock()
	if adopted {
		// Not orgs's to restart, so it may not have seen a model added since
		// it started.
		var live struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if localCall(ctx, "GET", rt.url()+"/v1/models", nil, &live) == nil {
			for _, d := range live.Data {
				if d.ID == name {
					return name, nil
				}
			}
			return "", fmt.Errorf("the llama.cpp at %s was not started by orgs and does not have %s: restart it so it reads its models folder again", rt.url(), name)
		}
	}
	return name, nil
}

func (llamaCpp) suggested() []localSuggestion {
	return []localSuggestion{
		{"Qwen/Qwen3-4B-GGUF:Q4_K_M", "Small and quick; fine for a laptop. Uses tools.", 2_497_280_256, "Qwen3-4B-Q4_K_M", false},
		{"Qwen/Qwen3-8B-GGUF:Q4_K_M", "A good default: careful with tools, decent prose.", 5_027_783_488, "Qwen3-8B-Q4_K_M", false},
		{"Qwen/Qwen3-14B-GGUF:Q4_K_M", "Better writing; wants 16GB of memory or more.", 9_001_752_960, "Qwen3-14B-Q4_K_M", false},
		{"ggml-org/gpt-oss-20b-GGUF:MXFP4", "OpenAI's open model; strong, wants 16GB or more.", 12_109_566_624, "gpt-oss-20b-MXFP4", false},
		{"bartowski/Meta-Llama-3.1-8B-Instruct-GGUF:Q4_K_M", "Meta's; plain and reliable.", 4_920_739_232, "Meta-Llama-3.1-8B-Instruct-Q4_K_M", false},
		{"bartowski/Mistral-Nemo-Instruct-2407-GGUF:Q4_K_M", "Mistral's; good with long text.", 7_477_208_192, "Mistral-Nemo-Instruct-2407-Q4_K_M", false},
	}
}
