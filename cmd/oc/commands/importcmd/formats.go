package importcmd

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/ical"
	"github.com/ihdavids/orgs/internal/common/mdorg"
	"github.com/ihdavids/orgs/internal/common/todotxt"
)

// What one import says about itself beside the server's answer.
type report struct {
	Kind    string
	From    string
	Summary []string
	common.OrgImported
}

// target is where a converted file goes when -o names none: its own name,
// with .org, in the first org directory.
func target(o *Options, name, fallback string) string {
	if o.Out != "" {
		out := o.Out
		if filepath.Ext(out) == "" {
			out += ".org"
		}
		return out
	}
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	if base == "" || base == "." || base == "-" {
		base = fallback
	}
	return base + ".org"
}

// send writes converted org through POST /import/org and says what happened.
func send(core *commands.Core, rep report, req common.OrgImport, o *Options) {
	req.Overwrite = o.Overwrite
	req.Append = o.Append
	// A dry run is the server saying what it would write, which is the useful
	// part (what an append would skip); the request itself is mostly org.
	dry := o.Print || commands.DryRun
	req.DryRun = dry
	res, err := common.RestPost[common.OrgImported](&core.Rest, "import/org", &req)
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		if res.Msg == "" {
			res.Msg = "the server did not import it (is it new enough to have /import/org?)"
		}
		commands.Fail("%s", res.Msg)
	}
	rep.OrgImported = res
	if o.Print && !commands.Machine() {
		for i, f := range res.Files {
			if len(res.Files) > 1 {
				if i > 0 {
					fmt.Println()
				}
				fmt.Printf("# ---- %s ----\n", f.Filename)
			}
			fmt.Print(f.Org)
		}
		return
	}
	if !dry && o.Open && len(res.Files) == 1 {
		core.LaunchEditor(res.Files[0].Filename, 1)
	}
	if commands.RenderOne(rep, nil) {
		return
	}
	g, d, r := commands.C(commands.AnsiGreen), commands.C(commands.AnsiDim), commands.C(commands.AnsiReset)
	verb := "Imported"
	if dry {
		verb = "Would import"
	}
	from := filepath.Base(rep.From)
	if len(res.Files) == 1 {
		f := res.Files[0]
		how := "→"
		if f.Appended {
			how = "→ the end of"
		}
		if f.Appended && f.Entries == 0 && f.Skipped > 0 {
			fmt.Printf("%sNothing new%s in %s: %s already in %s\n", g, r, from, plural(f.Skipped, "entry"), f.Filename)
			for _, w := range res.Warnings {
				fmt.Printf("  %snote:%s %s\n", commands.C(commands.AnsiGold), r, w)
			}
			return
		}
		fmt.Printf("%s%s%s %s %s %s\n", g, verb, r, from, how, f.Filename)
	} else {
		dir := commonDir(res.Files)
		fmt.Printf("%s%s%s %s → %d files in %s\n", g, verb, r, from, len(res.Files), dir)
	}
	for _, s := range rep.Summary {
		fmt.Printf("  %s\n", s)
	}
	if entries, skipped := totals(res.Files); skipped > 0 {
		fmt.Printf("  %s added, %d already there %s(by %s)%s\n", plural(entries, "entry"), skipped, d, keyName(req.KeyProp), r)
	}
	if len(res.Assets) > 0 {
		fmt.Printf("  %s copied %s(%s)%s\n", plural(len(res.Assets), "attachment"), d, commonDir2(res.Assets), r)
	}
	for _, w := range res.Warnings {
		fmt.Printf("  %snote:%s %s\n", commands.C(commands.AnsiGold), r, w)
	}
}

func keyName(k string) string {
	if k == "" {
		return "ID"
	}
	return k
}

func totals(fs []common.ImportedFile) (int, int) {
	e, s := 0, 0
	for _, f := range fs {
		e += f.Entries
		s += f.Skipped
	}
	return e, s
}

func commonDir(fs []common.ImportedFile) string {
	paths := []string{}
	for _, f := range fs {
		paths = append(paths, f.Filename)
	}
	return commonDir2(paths)
}

func commonDir2(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	dir := filepath.Dir(paths[0])
	for _, p := range paths[1:] {
		for !strings.HasPrefix(p, dir+string(filepath.Separator)) && dir != filepath.Dir(dir) {
			dir = filepath.Dir(dir)
		}
	}
	return dir
}

// ----------------------------------------------------------------------------
// Markdown and Obsidian vaults
// ----------------------------------------------------------------------------

// Folders a vault keeps that are not notes.
var skipDirs = map[string]bool{".obsidian": true, ".trash": true, ".git": true, "node_modules": true, ".stfolder": true}

// Reading a folder for the files a lone note might link to stops here, so
// importing ~/Downloads/note.md does not walk all of Downloads.
const looseFileLimit = 5000

// walk lists a folder's notes and other files, relative to it.
func walk(root string, notesToo bool, limit int) ([]mdorg.Note, []string, error) {
	notes := []mdorg.Note{}
	files := []string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (skipDirs[name] || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		switch strings.ToLower(filepath.Ext(name)) {
		case ".md", ".markdown", ".mdown":
			if notesToo {
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				notes = append(notes, mdorg.Note{Path: rel, Text: string(b)})
			}
			return nil
		case ".canvas":
			return nil
		}
		files = append(files, rel)
		if limit > 0 && len(files) >= limit {
			return filepath.SkipAll
		}
		return nil
	})
	return notes, files, err
}

func importMarkdown(core *commands.Core, name string, data []byte, o *Options) {
	var v mdorg.Vault
	root := ""
	single := data != nil
	if single {
		// One note: its folder is where the pictures it names are looked for.
		root = filepath.Dir(name)
		base := filepath.Base(name)
		if name == "" {
			base, root = "note.md", "."
		}
		_, files, _ := walk(root, false, looseFileLimit)
		v = mdorg.Vault{Notes: []mdorg.Note{{Path: filepath.ToSlash(base), Text: string(data)}}, Files: files}
	} else {
		root = name
		notes, files, err := walk(root, true, 0)
		if err != nil {
			commands.Fail("could not read %s: %v", root, err)
		}
		if len(notes) == 0 {
			commands.Fail("%s has no markdown notes in it", root)
		}
		v = mdorg.Vault{Notes: notes, Files: files}
	}
	conv := mdorg.Convert(v)

	req := common.OrgImport{}
	out := ""
	if single {
		out = target(o, name, "note")
		for _, org := range conv.Files {
			req.Files = append(req.Files, common.ImportFile{Filename: out, Org: org})
		}
	} else {
		out = o.Out
		if out == "" {
			abs, _ := filepath.Abs(root)
			out = filepath.Base(abs)
		}
		paths := []string{}
		for p := range conv.Files {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			req.Files = append(req.Files, common.ImportFile{Filename: path.Join(filepath.ToSlash(out), p), Org: conv.Files[p]})
		}
	}
	// The pictures go where the links say, relative to the org files, which
	// keep the vault's own folders.
	dest := filepath.ToSlash(out)
	if single {
		dest = path.Dir(dest)
	}
	for _, a := range conv.Assets {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(a)))
		if err != nil {
			conv.Warnings = append(conv.Warnings, fmt.Sprintf("could not read %s: %v", a, err))
			continue
		}
		req.Assets = append(req.Assets, common.ImportAsset{Filename: path.Join(dest, a), Data: b})
	}
	st := conv.Stats
	sum := []string{fmt.Sprintf("%s: %s, %s, %s", plural(st.Notes, "note"), plural(st.Links, "link"),
		plural(st.Embeds, "embed"), plural(st.Tasks, "task"))}
	if len(st.Tags) > 0 {
		tags := st.Tags
		more := ""
		if len(tags) > 12 {
			tags, more = tags[:12], fmt.Sprintf(" and %d more", len(st.Tags)-12)
		}
		sum = append(sum, "tags: "+strings.Join(tags, " ")+more)
	}
	for _, w := range conv.Warnings {
		sum = append(sum, "note: "+w)
	}
	send(core, report{Kind: "markdown", From: name, Summary: sum}, req, o)
}

// ----------------------------------------------------------------------------
// todo.txt
// ----------------------------------------------------------------------------

func importTodoTxt(core *commands.Core, name string, data []byte, o *Options) {
	tasks := todotxt.Parse(string(data))
	if len(tasks) == 0 {
		commands.Fail("%s has no tasks in it", name)
	}
	out := target(o, name, "todo")
	title := strings.TrimSuffix(filepath.Base(out), filepath.Ext(out))
	org := todotxt.ToOrg(tasks, todotxt.OrgOptions{Title: strings.ToUpper(title[:1]) + title[1:], Flat: o.Flat})
	open, done, projects := 0, 0, map[string]bool{}
	for _, t := range tasks {
		if t.Done {
			done++
		} else {
			open++
		}
		for _, p := range t.Projects {
			projects[p] = true
		}
	}
	sum := []string{fmt.Sprintf("%s open, %d done, %s", plural(open, "task"), done, plural(len(projects), "project"))}
	send(core, report{Kind: "todo.txt", From: name, Summary: sum},
		common.OrgImport{Files: []common.ImportFile{{Filename: out, Org: org}}}, o)
}

// ----------------------------------------------------------------------------
// iCalendar
// ----------------------------------------------------------------------------

func importCalendar(core *commands.Core, name string, data []byte, o *Options) {
	cal, err := ical.Parse(string(data))
	if err != nil {
		commands.Fail("%s: %v", name, err)
	}
	opts := ical.OrgOptions{Tag: o.Tag}
	if o.Since != "" {
		d, _, err := commands.ParseDate(o.Since, time.Now())
		if err != nil {
			commands.Fail("-since %s: %v", o.Since, err)
		}
		opts.Since = d.Day
	}
	fallback := cal.Name
	if fallback == "" {
		fallback = "calendar"
	}
	out := target(o, name, fallback)
	org, st := ical.ToOrg(cal, opts)
	if st.Events+st.Todos == 0 {
		commands.Fail("nothing to import from %s (%d left out: cancelled, or before -since)", name, st.Skipped)
	}
	sum := []string{fmt.Sprintf("%s, %s", plural(st.Events, "event"), plural(st.Todos, "task"))}
	if st.Skipped > 0 {
		sum = append(sum, fmt.Sprintf("%d left out (cancelled, undated, or before -since)", st.Skipped))
	}
	if st.PartialRules > 0 {
		sum = append(sum, fmt.Sprintf("%d repeats say more than org can (an end date, several weekdays); the rule is kept in :RRULE:", st.PartialRules))
	}
	// Each entry carries its calendar uid, so appending the same calendar
	// again adds only what is new.
	send(core, report{Kind: "iCalendar", From: name, Summary: sum},
		common.OrgImport{Files: []common.ImportFile{{Filename: out, Org: org}}, SkipKnown: true, KeyProp: "ICAL_UID"}, o)
}
