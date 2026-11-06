package importcmd

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/notionorg"
)

// ----------------------------------------------------------------------------
// Notion exports
// ----------------------------------------------------------------------------

// isNotionDir says whether a folder is a Notion export: a page or database
// named with Notion's id. It stops at the first one, and after a few
// thousand names without one, so saying no to a large vault stays quick.
func isNotionDir(dir string) bool {
	found, seen := false, 0
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		seen++
		if notionorg.IsExport([]string{d.Name()}) {
			found = true
			return filepath.SkipAll
		}
		if seen > looseFileLimit {
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// readTree is every file of a folder by its slash path, as ReadZip gives a
// zip's.
func readTree(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") && p != root {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		files[filepath.ToSlash(rel)] = b
		return nil
	})
	return files, err
}

var (
	exportName = regexp.MustCompile(`(?i)(^|[_-])export[-_]?[0-9a-f-]{8,}`)
	trailingID = regexp.MustCompile(`\s?[0-9a-fA-F]{32}$`)
)

// notionTarget is the folder an export goes in when -o names none: the
// zip's or folder's name without Notion's ids, or "notion" for Notion's own
// Export-<uuid>.zip.
func notionTarget(name string) string {
	abs, _ := filepath.Abs(name)
	base := filepath.Base(abs)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if exportName.MatchString(base) || base == "" || base == "." || base == "-" {
		return "notion"
	}
	if t := strings.TrimSpace(trailingID.ReplaceAllString(base, "")); t != "" {
		return t
	}
	return "notion"
}

func importNotion(core *commands.Core, name string, data []byte, o *Options) {
	var files map[string][]byte
	var err error
	if data == nil {
		files, err = readTree(name)
	} else {
		files, err = notionorg.ReadZip(data)
	}
	if err != nil {
		commands.Fail("%s: %v", name, err)
	}
	paths := []string{}
	for p := range files {
		paths = append(paths, p)
	}
	if !notionorg.IsExport(paths) {
		fmt.Fprintf(os.Stderr, "note: %s has no Notion page names in it; converting it as a Notion export anyway\n", name)
	}
	conv := notionorg.Convert(files)
	if len(conv.Files) == 0 {
		commands.Fail("%s has no pages or databases in it", name)
	}
	out := o.Out
	if out == "" {
		out = notionTarget(name)
	}
	out = strings.TrimSuffix(filepath.ToSlash(out), ".org")
	req := common.OrgImport{}
	orgs := []string{}
	for p := range conv.Files {
		orgs = append(orgs, p)
	}
	sort.Strings(orgs)
	for _, p := range orgs {
		req.Files = append(req.Files, common.ImportFile{Filename: path.Join(out, p), Org: conv.Files[p]})
	}
	assets := []string{}
	for a := range conv.Assets {
		assets = append(assets, a)
	}
	sort.Strings(assets)
	for _, a := range assets {
		req.Assets = append(req.Assets, common.ImportAsset{Filename: path.Join(out, a), Data: conv.Assets[a]})
	}
	st := conv.Stats
	sum := []string{fmt.Sprintf("%s, %s (%s, %s), %s", plural(st.Pages, "page"), plural(st.Databases, "database"),
		plural(st.Rows, "row"), plural(st.Tasks, "task"), plural(st.Links, "link"))}
	for _, w := range conv.Warnings {
		sum = append(sum, "note: "+w)
	}
	send(core, report{Kind: "Notion", From: name, Summary: sum}, req, o)
}
