package jcanvas

// How a file node names an org heading.
//
// A JSON Canvas file node names a heading of a note as `"file": "note.md", "subpath":
// "#Heading"` (nested: `#Parent#Child`) and a block as `#^blockid`. For org
// files the same two shapes are read, plus org's own search options:
//
//	"subpath": "#^5f0c…"           the heading with that :ID: (else :CUSTOM_ID:)
//	"subpath": "#Plans#Next year"  the heading by outline path
//	"subpath": "#id:5f0c…"         an org id: link's target
//	"file": "notes.org::*Plans"    an org link's search option: a heading,
//	"file": "notes.org::#plans"    a CUSTOM_ID,
//	"file": "notes.org::42"        a line
//
// Written: always `#^<ID>`. orgs gives the heading an :ID: when it is put on
// a canvas (as org-id would), so the card follows the heading when it is
// renamed or refiled into another file. A heading that already has a
// CUSTOM_ID and no ID is written by its CUSTOM_ID.

import "strings"

type Ref struct {
	Path     string   // the file as written, search option cut off
	ID       string   // an :ID: (or a CUSTOM_ID, for #^)
	CustomID string   // ::#name
	Olp      []string // #A#B, or ::*Heading (one element)
	Line     int      // ::42, from 1
}

// IsHeading is whether the ref names something inside the file.
func (r Ref) IsHeading() bool {
	return r.ID != "" || r.CustomID != "" || len(r.Olp) > 0 || r.Line > 0
}

// ParseRef reads a file node's file and subpath.
func ParseRef(file, subpath string) Ref {
	r := Ref{Path: file}
	if i := strings.Index(file, "::"); i >= 0 {
		r.Path = file[:i]
		search := strings.TrimSpace(file[i+2:])
		switch {
		case strings.HasPrefix(search, "*"):
			if t := strings.TrimSpace(search[1:]); t != "" {
				r.Olp = []string{t}
			}
		case strings.HasPrefix(search, "#"):
			r.CustomID = strings.TrimSpace(search[1:])
		case strings.HasPrefix(search, "id:"):
			r.ID = strings.TrimSpace(search[3:])
		default:
			n := 0
			for _, ch := range search {
				if ch < '0' || ch > '9' {
					n = -1
					break
				}
				n = n*10 + int(ch-'0')
			}
			if n > 0 {
				r.Line = n
			} else if search != "" {
				r.Olp = []string{search}
			}
		}
	}
	sp := strings.TrimSpace(subpath)
	if !strings.HasPrefix(sp, "#") || len(sp) < 2 {
		return r
	}
	sp = sp[1:]
	switch {
	case strings.HasPrefix(sp, "^"):
		r.ID = strings.TrimSpace(sp[1:])
	case strings.HasPrefix(sp, "id:"):
		r.ID = strings.TrimSpace(sp[3:])
	default:
		r.Olp = nil
		for _, p := range strings.Split(sp, "#") {
			if p = strings.TrimSpace(p); p != "" {
				r.Olp = append(r.Olp, p)
			}
		}
	}
	return r
}

// SubpathFor is what orgs writes for a heading with this id.
func SubpathFor(id string) string { return "#^" + id }
