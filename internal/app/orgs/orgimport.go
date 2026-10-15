package orgs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

// importEntries splits org text into what comes before the first heading
// (the #+KEYWORD: lines and anything else) and its top level entries, each a
// heading with everything under it.
func importEntries(text string) (string, []string) {
	lines := strings.SplitAfter(text, "\n")
	head := strings.Builder{}
	entries := []string{}
	cur := strings.Builder{}
	in := false
	for _, l := range lines {
		if strings.HasPrefix(l, "* ") || strings.TrimRight(l, "\r\n") == "*" {
			if in {
				entries = append(entries, cur.String())
				cur.Reset()
			}
			in = true
		}
		if in {
			cur.WriteString(l)
		} else {
			head.WriteString(l)
		}
	}
	if in {
		entries = append(entries, cur.String())
	}
	return head.String(), entries
}

// The value of the first :PROP: line in an entry - the heading's own drawer,
// since it comes before any child's.
func entryKey(entry, prop string) string {
	re := regexp.MustCompile(`(?mi)^[ \t]*:` + regexp.QuoteMeta(prop) + `:[ \t]+(\S.*?)[ \t]*$`)
	if m := re.FindStringSubmatch(entry); m != nil {
		return m[1]
	}
	return ""
}

// Every value a file already holds for prop, anywhere in it.
func knownKeys(text, prop string) map[string]bool {
	re := regexp.MustCompile(`(?mi)^[ \t]*:` + regexp.QuoteMeta(prop) + `:[ \t]+(\S.*?)[ \t]*$`)
	out := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		out[m[1]] = true
	}
	return out
}

func orgImportTarget(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("a file has no name")
	}
	if ext := strings.ToLower(filepath.Ext(name)); ext != ".org" {
		return "", fmt.Errorf("%s: an org file has to end in .org", name)
	}
	path, ok := insideOrgDirs(name)
	if !ok {
		return "", fmt.Errorf("%s is not inside one of the org directories", name)
	}
	return path, nil
}

// ImportOrg writes org text a client converted, and the files it links to.
func ImportOrg(req *common.OrgImport) (common.OrgImported, int) {
	fail := func(status int, format string, args ...interface{}) (common.OrgImported, int) {
		return common.OrgImported{Ok: false, Msg: fmt.Sprintf(format, args...)}, status
	}
	if len(req.Files) == 0 {
		return fail(http.StatusBadRequest, "nothing to import")
	}
	prop := strings.TrimSpace(req.KeyProp)
	if prop == "" {
		prop = "ID"
	}
	res := common.OrgImported{Files: []common.ImportedFile{}}

	type write struct {
		path string
		text []byte
		mode int // os.O_TRUNC or os.O_APPEND
	}
	writes := []write{}
	seen := map[string]bool{}
	clash := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}
	for _, a := range req.Assets {
		path, ok := insideOrgDirs(a.Filename)
		if !ok {
			return fail(http.StatusBadRequest, "%s would land outside the org directories", a.Filename)
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		if clash(path) && !req.Overwrite {
			// A picture already there is very likely the same picture, from
			// the last time this was imported: keep it, and say so.
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s is already there and was left alone", path))
			continue
		}
		writes = append(writes, write{path, a.Data, os.O_TRUNC})
		res.Assets = append(res.Assets, path)
	}
	for _, f := range req.Files {
		path, err := orgImportTarget(f.Filename)
		if err != nil {
			return fail(http.StatusBadRequest, "%v", err)
		}
		if seen[path] {
			return fail(http.StatusBadRequest, "%s is named twice", path)
		}
		seen[path] = true
		head, entries := importEntries(f.Org)
		done := common.ImportedFile{Filename: path, Entries: len(entries)}
		text := f.Org
		mode := os.O_TRUNC
		if clash(path) {
			switch {
			case req.Append:
				old, err := os.ReadFile(path)
				if err != nil {
					return fail(http.StatusInternalServerError, "%v", err)
				}
				known := map[string]bool{}
				if req.SkipKnown {
					known = knownKeys(string(old), prop)
				}
				add := strings.Builder{}
				done.Entries = 0
				for _, e := range entries {
					if k := entryKey(e, prop); k != "" && known[k] {
						done.Skipped++
						continue
					}
					done.Entries++
					add.WriteString(e)
				}
				// Text before the first heading is the new file's head; an
				// append keeps only what is not keywords (a note, say).
				for _, l := range strings.SplitAfter(head, "\n") {
					if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "#+") {
						res.Warnings = append(res.Warnings, fmt.Sprintf("%s: text above the first heading was left off the append", path))
						break
					}
				}
				text = add.String()
				if text != "" && len(old) > 0 && old[len(old)-1] != '\n' {
					text = "\n" + text
				}
				done.Appended = true
				mode = os.O_APPEND
			case req.Overwrite:
			default:
				msg := fmt.Sprintf("%s already exists. Pick another name, append to it, or overwrite it", path)
				if !req.DryRun {
					return fail(http.StatusConflict, "%s", msg)
				}
				res.Warnings = append(res.Warnings, msg)
			}
		}
		if req.DryRun {
			done.Org = text
		}
		res.Files = append(res.Files, done)
		if !(mode == os.O_APPEND && text == "") {
			writes = append(writes, write{path, []byte(text), mode})
		}
	}
	if req.DryRun {
		res.Ok = true
		res.Msg = "nothing written (a dry run): " + orgImportSummary(&res)
		return res, http.StatusOK
	}
	// Assets come first in writes, so a file never links to one not yet there.
	for _, w := range writes {
		if err := os.MkdirAll(filepath.Dir(w.path), 0755); err != nil {
			return fail(http.StatusInternalServerError, "%v", err)
		}
		f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|w.mode, 0644)
		if err != nil {
			return fail(http.StatusInternalServerError, "%v", err)
		}
		_, err = f.Write(w.text)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return fail(http.StatusInternalServerError, "%s: %v", w.path, err)
		}
	}
	for _, f := range res.Files {
		GetDb().ReloadFile(f.Filename)
	}
	res.Ok = true
	res.Msg = orgImportSummary(&res)
	return res, http.StatusOK
}

func orgImportSummary(r *common.OrgImported) string {
	entries, skipped := 0, 0
	for _, f := range r.Files {
		entries += f.Entries
		skipped += f.Skipped
	}
	s := fmt.Sprintf("%s, %s", countOf(len(r.Files), "file"), countOf(entries, "entry"))
	s = strings.Replace(s, "entrys", "entries", 1)
	if skipped > 0 {
		s += fmt.Sprintf(", %d already there", skipped)
	}
	if len(r.Assets) > 0 {
		s += ", " + countOf(len(r.Assets), "attachment")
	}
	return s
}

/* SDOC: API
* POST /import/org — Write Org Text Converted Elsewhere

	#+BEGIN_EXAMPLE
	{ "Files": [ { "Filename": "inbox/todo.org", "Org": "* TODO ..." } ],
	  "Assets": [ { "Filename": "vault/pics/a.png", "Data": "<base64>" } ],
	  "Overwrite": false, "Append": false, "SkipKnown": false,
	  "KeyProp": "ID", "DryRun": false }
	#+END_EXAMPLE

	What =orgs import= sends for markdown and Obsidian vaults, todo.txt and
	iCalendar files: those are converted on the client, where the file is,
	and written here. Each =Filename= is relative to the first org directory
	or absolute inside one.

	A file already there is refused unless =Overwrite= replaces it or
	=Append= adds the new entries to its end (leaving off the new text's
	=#+KEYWORD:= lines). =SkipKnown= with =Append= leaves out any top level
	entry whose =KeyProp= property (=ID= unless named; the calendar importer
	uses =ICAL_UID=) the file already has, so importing the same thing twice
	adds only what is new. An asset already there is kept unless
	=Overwrite=. =DryRun= writes nothing and answers each file's text in
	=Org=.

	*Response:* =Files= (=Filename=, =Entries=, =Skipped=, =Appended=),
	=Assets= written and =Warnings=; a refusal is =Ok: false= with =Msg=.
EDOC */
func PostImportOrg(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		answerErr(w, err)
		return
	}
	var req common.OrgImport
	if err := json.Unmarshal(body, &req); err != nil {
		answerErr(w, err)
		return
	}
	res, status := ImportOrg(&req)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(res)
}
