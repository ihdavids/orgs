package orgs

// Model files from Hugging Face for the local runtimes (localrt.go): a
// repository's file list, and one file fetched with its progress counted.
// Gated repositories need HF_TOKEN in the server's environment.
//
// Only files that hold no code are fetched (localSafeFile): weights as
// .gguf or .safetensors, and the tokenizer's and template's text. A .bin,
// .pt or .pkl (pickles, which run code when loaded) or a .py (a model that
// needs trust_remote_code) is never written, whatever the repository holds.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var hfRepoRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,95}/[A-Za-z0-9][A-Za-z0-9._-]{0,95}$`)

type hfFile struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

func hfRequest(ctx context.Context, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	if t := os.Getenv("HF_TOKEN"); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	return http.DefaultClient.Do(req)
}

// The files of a repository's main branch, folders walked.
func hfTree(ctx context.Context, repo string) ([]hfFile, error) {
	if !hfRepoRe.MatchString(repo) {
		return nil, fmt.Errorf("%q is not a Hugging Face repository (owner/name)", repo)
	}
	res, err := hfRequest(ctx, "https://huggingface.co/api/models/"+repo+"/tree/main?recursive=1")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == 401 || res.StatusCode == 403:
		return nil, fmt.Errorf("%s is gated or private: accept its terms on huggingface.co and set HF_TOKEN in the server's environment", repo)
	case res.StatusCode == 404:
		return nil, fmt.Errorf("there is no repository %s on Hugging Face", repo)
	case res.StatusCode != 200:
		return nil, fmt.Errorf("Hugging Face said %s for %s", res.Status, repo)
	}
	var files []hfFile
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&files); err != nil {
		return nil, err
	}
	out := []hfFile{}
	for _, f := range files {
		if f.Type == "file" || f.Type == "" {
			out = append(out, f)
		}
	}
	return out, nil
}

// A path inside a repository that is safe to write under a folder of ours.
func hfCleanPath(p string) (string, bool) {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return "", false
	}
	c := path.Clean(p)
	if c != p || c == "." || strings.HasPrefix(c, "../") || c == ".." {
		return "", false
	}
	return c, true
}

// Weights and tokenizer files: nothing a runtime would execute.
func localSafeFile(p string) bool {
	base := strings.ToLower(path.Base(p))
	for _, ext := range []string{".gguf", ".safetensors", ".json", ".txt", ".model", ".jinja", ".tiktoken"} {
		if strings.HasSuffix(base, ext) {
			return true
		}
	}
	return false
}

// Fetches repo's file to dest (via dest.part, so a half download is never a
// model), adding what arrives to *done under mu.
func hfDownload(ctx context.Context, repo, file, dest string, done *int64, mu *sync.Mutex) error {
	if _, ok := hfCleanPath(file); !ok || !localSafeFile(file) {
		return fmt.Errorf("%s is not a file orgs fetches", file)
	}
	esc := []string{}
	for _, seg := range strings.Split(file, "/") {
		esc = append(esc, url.PathEscape(seg))
	}
	res, err := hfRequest(ctx, "https://huggingface.co/"+repo+"/resolve/main/"+strings.Join(esc, "/"))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("Hugging Face said %s for %s/%s", res.Status, repo, file)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	part := dest + ".part"
	f, err := os.Create(part)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, &countingReader{r: res.Body, n: done, mu: mu})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(part)
		return err
	}
	return os.Rename(part, dest)
}

// Is dir (or a file) inside root, after cleaning: what remove may touch.
func insideDir(root, p string) bool {
	root, p = filepath.Clean(root), filepath.Clean(p)
	return p != root && strings.HasPrefix(p, root+string(os.PathSeparator))
}
