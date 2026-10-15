// Package mdorg turns markdown into org: a single file, or an Obsidian vault
// with its [[wikilinks]], embeds, callouts, front matter and #tags.
//
// It runs on the client (`orgs import`), where the vault is, and needs
// nothing but the files' text: the vault is converted as a whole so a link is
// resolved the way Obsidian resolves it (by note name, anywhere in the vault)
// and comes out as a relative file: link between the org files, which keep
// the vault's folders. The attachments the notes use are named in the answer
// so the client sends only those.
package mdorg

import (
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
)

// Note is one markdown file; Path is relative to the vault, with slashes.
type Note struct {
	Path string
	Text string
}

// Vault is what is being imported.
type Vault struct {
	Notes []Note
	// Every other file in the vault, relative to it, so ![[pic.png]] can be
	// found wherever Obsidian put it.
	Files []string
}

// Converted is the vault as org.
type Converted struct {
	// Org text by path relative to the vault (notes/a.md → notes/a.org).
	Files map[string]string
	// The vault's files the notes link to or embed, relative to the vault:
	// what has to be copied for the links to work.
	Assets   []string
	Warnings []string
	Stats    Stats
}

// Stats counts what a conversion found.
type Stats struct {
	Notes, Links, Embeds, Unresolved, Tasks, Callouts, Tables, Blocks int
	Tags                                                                []string
}

// OrgPath is where a note's org file goes, relative to the vault.
func OrgPath(md string) string {
	ext := path.Ext(md)
	return strings.TrimSuffix(md, ext) + ".org"
}

func isNote(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".md", ".markdown", ".mdown":
		return true
	}
	return false
}

// index is how links are resolved: by name the way Obsidian does, and by
// path for markdown links.
type index struct {
	byName   map[string][]string // lower-case name without extension → note paths
	byPath   map[string]string   // lower-case path → note path
	files    map[string]string   // lower-case path → file path
	fileName map[string][]string // lower-case base name → file paths
	headings map[string][]string // note path → its heading titles
}

func newIndex(v Vault) *index {
	ix := &index{byName: map[string][]string{}, byPath: map[string]string{}, files: map[string]string{},
		fileName: map[string][]string{}, headings: map[string][]string{}}
	for _, n := range v.Notes {
		base := strings.TrimSuffix(path.Base(n.Path), path.Ext(n.Path))
		ix.byName[strings.ToLower(base)] = append(ix.byName[strings.ToLower(base)], n.Path)
		ix.byPath[strings.ToLower(n.Path)] = n.Path
		ix.byPath[strings.ToLower(strings.TrimSuffix(n.Path, path.Ext(n.Path)))] = n.Path
		ix.headings[n.Path] = headingsOf(n.Text)
	}
	for _, f := range v.Files {
		ix.files[strings.ToLower(f)] = f
		b := strings.ToLower(path.Base(f))
		ix.fileName[b] = append(ix.fileName[b], f)
	}
	for _, m := range []map[string][]string{ix.byName, ix.fileName} {
		for k := range m {
			sort.Slice(m[k], func(i, j int) bool {
				a, b := m[k][i], m[k][j]
				if strings.Count(a, "/") != strings.Count(b, "/") {
					return strings.Count(a, "/") < strings.Count(b, "/")
				}
				return a < b
			})
		}
	}
	return ix
}

// closest picks, of several notes or files with one name, the one in the
// linking note's own folder, else the shallowest - which is Obsidian's
// "shortest path when possible".
func closest(cands []string, from string) string {
	dir := path.Dir(from)
	for _, c := range cands {
		if path.Dir(c) == dir {
			return c
		}
	}
	return cands[0]
}

// note finds the note a link names: a path (folder/Note, Note.md) or a name.
func (ix *index) note(target, from string) (string, bool) {
	t := strings.TrimSpace(target)
	if t == "" {
		return from, true
	}
	if un, err := url.PathUnescape(t); err == nil {
		t = un
	}
	rel := path.Clean(path.Join(path.Dir(from), t))
	for _, p := range []string{rel, t} {
		if n, ok := ix.byPath[strings.ToLower(p)]; ok {
			return n, true
		}
		if n, ok := ix.byPath[strings.ToLower(p+".md")]; ok {
			return n, true
		}
	}
	name := strings.ToLower(strings.TrimSuffix(path.Base(t), path.Ext(t)))
	if !isNote(t) && path.Ext(t) != "" {
		name = strings.ToLower(path.Base(t))
	}
	if c, ok := ix.byName[name]; ok {
		return closest(c, from), true
	}
	return "", false
}

// file finds an attachment: relative to the note, then from the vault root,
// then by name anywhere.
func (ix *index) file(target, from string) (string, bool) {
	t := strings.TrimSpace(target)
	if un, err := url.PathUnescape(t); err == nil {
		t = un
	}
	for _, p := range []string{path.Clean(path.Join(path.Dir(from), t)), path.Clean(strings.TrimPrefix(t, "/"))} {
		if f, ok := ix.files[strings.ToLower(p)]; ok {
			return f, true
		}
	}
	if c, ok := ix.fileName[strings.ToLower(path.Base(t))]; ok {
		return closest(c, from), true
	}
	return "", false
}

// heading finds the title a link's #part names: as written, or a slug of it
// (#my-heading), which is how markdown links to a heading elsewhere.
func (ix *index) heading(note, part string) string {
	part = strings.TrimSpace(part)
	if un, err := url.PathUnescape(part); err == nil {
		part = un
	}
	for _, h := range ix.headings[note] {
		if strings.EqualFold(h, part) || slug(h) == slug(part) {
			return h
		}
	}
	return part
}

func slug(s string) string {
	b := strings.Builder{}
	dash := false
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127 {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// relLink is the path from one vault file to another, as an org link wants
// it: ./b.org beside it, ../x/b.org elsewhere.
func relLink(from, to string) string {
	fromDir := strings.Split(path.Dir(from), "/")
	if path.Dir(from) == "." {
		fromDir = nil
	}
	toParts := strings.Split(to, "/")
	i := 0
	for i < len(fromDir) && i < len(toParts)-1 && fromDir[i] == toParts[i] {
		i++
	}
	up := strings.Repeat("../", len(fromDir)-i)
	rel := up + strings.Join(toParts[i:], "/")
	if up == "" {
		rel = "./" + rel
	}
	return rel
}

// Convert turns every note of a vault into org.
func Convert(v Vault) Converted {
	ix := newIndex(v)
	out := Converted{Files: map[string]string{}}
	assets := map[string]bool{}
	tags := map[string]bool{}
	for _, n := range v.Notes {
		c := &conv{ix: ix, from: n.Path, assets: assets, tags: map[string]bool{}, stats: &out.Stats}
		out.Files[OrgPath(n.Path)] = c.note(n.Text)
		for t := range c.tags {
			tags[t] = true
		}
		out.Warnings = append(out.Warnings, c.warnings...)
		out.Stats.Notes++
	}
	for a := range assets {
		out.Assets = append(out.Assets, a)
	}
	sort.Strings(out.Assets)
	for t := range tags {
		out.Stats.Tags = append(out.Stats.Tags, t)
	}
	sort.Strings(out.Stats.Tags)
	if out.Stats.Unresolved > 0 {
		out.Warnings = append(out.Warnings, fmt.Sprintf("%d links name a note or file that is not in the vault; they point where it would be", out.Stats.Unresolved))
	}
	return out
}

// ConvertText converts one note on its own (name is its file name, for the
// title and for links relative to it).
func ConvertText(name, text string) string {
	return Convert(Vault{Notes: []Note{{Path: name, Text: text}}}).Files[OrgPath(name)]
}
