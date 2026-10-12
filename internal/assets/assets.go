// Package assets is the templates, web and webfonts folders as they were when
// the binary was built, and the search path that puts them behind the folders
// named in the config.
//
// The embedded trees are unpacked once into the user cache directory, keyed by
// a hash of their contents, rather than read through an fs.FS. Everything that
// reads a template wants a real path: pongo2 resolves `{% include %}` against
// the including file's directory, latex is handed files on disk, the dnd
// library walks directories, and the file servers for /orgimages and /orgfonts
// serve directories. A path that is simply one more folder on the search path
// keeps all of that working unchanged.
//
// Order is always: the configured folder, then ./templates in the working
// directory, then the built in copy. So a file of the same name in the
// configured folder replaces the built in one, and a new file there is added.
package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tplfs "github.com/ihdavids/orgs/templates"
	webfs "github.com/ihdavids/orgs/web"
	fontfs "github.com/ihdavids/orgs/webfonts"
)

// The trees, by the name of the folder they were built from.
var trees = map[string]fs.FS{
	"templates": tplfs.Content,
	"web":       webfs.Content,
	"webfonts":  fontfs.Content,
}

// TemplatePath is the configured templatePath. The server sets it when the
// config is read (and again on a reload), so code with no config to hand -
// the exporters' plugs helpers, the slides themes - still searches it.
var TemplatePath = "./templates"

var (
	unpackOnce sync.Once
	unpackRoot string
)

// skip is what is embedded only because a folder pattern cannot leave it out.
func skip(name string) bool {
	base := filepath.Base(name)
	return base == "embed.go" || base == "CLAUDE.md"
}

// Dir is where the built in copy of tree ("templates", "web" or "webfonts")
// is on disk, or "" if it could not be unpacked.
func Dir(tree string) string {
	unpackOnce.Do(func() {
		root, err := unpack()
		if err != nil {
			fmt.Fprintf(os.Stderr, "assets: could not unpack the built in templates: %v\n", err)
			return
		}
		unpackRoot = root
	})
	if unpackRoot == "" {
		return ""
	}
	return filepath.Join(unpackRoot, tree)
}

// hash is of every embedded name and its contents, so a binary built with an
// edited template unpacks into a folder of its own and never reads a stale one.
func hash() (string, error) {
	h := sha256.New()
	names := make([]string, 0, len(trees))
	for n := range trees {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, tree := range names {
		err := fs.WalkDir(trees[tree], ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || skip(p) {
				return err
			}
			data, err := fs.ReadFile(trees[tree], p)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "%s/%s\x00%d\x00", tree, p, len(data))
			h.Write(data)
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

const marker = ".complete"

func unpack() (string, error) {
	sum, err := hash()
	if err != nil {
		return "", err
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	parent := filepath.Join(base, "orgs")
	root := filepath.Join(parent, "builtin-"+sum)
	if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
		now := time.Now()
		os.Chtimes(filepath.Join(root, marker), now, now)
		return root, nil
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	// Written beside the final name and renamed into place, so two servers
	// starting at once, or one killed half way, never leave a partial tree
	// that looks complete.
	tmp, err := os.MkdirTemp(parent, "unpack-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	for tree, fsys := range trees {
		err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			dst := filepath.Join(tmp, tree, filepath.FromSlash(p))
			if d.IsDir() {
				return os.MkdirAll(dst, 0o755)
			}
			if skip(p) {
				return nil
			}
			data, err := fs.ReadFile(fsys, p)
			if err != nil {
				return err
			}
			return os.WriteFile(dst, data, 0o644)
		})
		if err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, marker), []byte(sum), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, root); err != nil {
		// Somebody else got there first, which is as good.
		if _, serr := os.Stat(filepath.Join(root, marker)); serr != nil {
			return "", err
		}
	}
	prune(parent, root)
	return root, nil
}

// prune removes copies unpacked by other builds that nothing has started
// from in a month. Each start touches its own marker, so a copy a running
// server uses is never that old.
func prune(parent, keep string) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	for _, e := range entries {
		dir := filepath.Join(parent, e.Name())
		if !e.IsDir() || dir == keep || !strings.HasPrefix(e.Name(), "builtin-") {
			continue
		}
		if st, err := os.Stat(filepath.Join(dir, marker)); err == nil && st.ModTime().After(cutoff) {
			continue
		}
		os.RemoveAll(dir)
	}
}

// TemplateDirs is every folder a template is looked for in, in order: each
// configured folder given (TemplatePath when none is), ./templates, and the
// built in copy. Only folders that exist are returned, each once.
func TemplateDirs(configured ...string) []string {
	if len(configured) == 0 {
		configured = []string{TemplatePath}
	}
	return existing(append(append([]string{}, configured...), "./templates", Dir("templates")))
}

// UserTemplateDirs is TemplateDirs without the built in copy: the folders
// somebody put files in.
func UserTemplateDirs(configured ...string) []string {
	if len(configured) == 0 {
		configured = []string{TemplatePath}
	}
	builtin := Dir("templates")
	out := []string{}
	for _, d := range existing(append(append([]string{}, configured...), "./templates")) {
		if d != builtin {
			out = append(out, d)
		}
	}
	return out
}

func existing(dirs []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, d := range dirs {
		if strings.TrimSpace(d) == "" {
			continue
		}
		abs, err := filepath.Abs(d)
		if err != nil || seen[abs] {
			continue
		}
		if st, err := os.Stat(abs); err != nil || !st.IsDir() {
			continue
		}
		seen[abs] = true
		out = append(out, abs)
	}
	return out
}

// FindTemplate is the first file called name (a path relative to a template
// folder, like "html_styles/default_style.css") on the template search path,
// or "" when there is none.
func FindTemplate(name string, configured ...string) string {
	for _, d := range TemplateDirs(configured...) {
		p := filepath.Join(d, filepath.FromSlash(name))
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// GlobTemplates is the names (relative to a template folder, with forward
// slashes) matching pattern in any template folder, each once and sorted - a
// listing of themes or new-file templates that includes both the built in
// ones and any the user added.
func GlobTemplates(pattern string, configured ...string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, d := range TemplateDirs(configured...) {
		matches, _ := filepath.Glob(filepath.Join(d, filepath.FromSlash(pattern)))
		for _, m := range matches {
			if st, err := os.Stat(m); err != nil || st.IsDir() {
				continue
			}
			rel, err := filepath.Rel(d, m)
			if err != nil {
				continue
			}
			rel = filepath.ToSlash(rel)
			if !seen[rel] {
				seen[rel] = true
				out = append(out, rel)
			}
		}
	}
	sort.Strings(out)
	return out
}

// Overlay serves from each directory in turn: the first one holding the
// requested file answers. Missing or empty directories are skipped.
func Overlay(dirs ...string) http.FileSystem {
	o := overlay{}
	for _, d := range existing(dirs) {
		o = append(o, http.Dir(d))
	}
	return o
}

type overlay []http.Dir

func (o overlay) Open(name string) (http.File, error) {
	for _, d := range o {
		if f, err := d.Open(name); err == nil {
			return f, nil
		}
	}
	return nil, fs.ErrNotExist
}
