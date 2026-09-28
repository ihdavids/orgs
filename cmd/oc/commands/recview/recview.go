// Package recview draws one record, and the line that stands for it in a list.
//
// It is its own package because two commands draw records and neither owns the
// other. `orgs rec` is the general one, over every collection; `orgs contact`
// is the same engine with the address book's manners on. If the drawing lived
// in one of them, the other would grow its own copy and the two would say
// different things about the same record within a month - which is exactly what
// the records engine itself avoids by working out a field's kind from its name
// rather than having a view per collection.
//
// Nothing here knows what a contact is, or a guitar pedal. A record is a name,
// some fields whose kinds the server worked out, some notes and a history, and
// that is all this draws.
package recview

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// The order fields are drawn in: the ones somebody reaches for first, then
// everything else in the order the record itself keeps them.
//
// A kind with no entry sorts into the middle rather than last, which is right
// for the odds and ends a collection of guitar pedals grows: they are the whole
// point of that record and would be hidden under its urls otherwise.
var kindOrder = map[string]int{
	"tel": 1, "email": 2, "social": 3, "postal": 4, "date": 5, "url": 7, "image": 9,
}

func kindRank(k string) int {
	if r, ok := kindOrder[k]; ok {
		return r
	}
	return 6
}

// What to call a field on screen. "phone (mobile)" reads better than
// "PHONE_MOBILE" and is the same thing.
func FieldLabel(f common.RecordField) string {
	if f.Label == "" {
		return strings.ToLower(f.Name)
	}
	return strings.ToLower(f.Name) + " (" + strings.ToLower(f.Label) + ")"
}

// The two or three fields worth putting beside a name in a list.
//
// Chosen by *kind* rather than by property name, which is what lets one line
// format serve a contact and a laptop: whatever the collection happens to keep,
// the most contactable-looking of it shows.
func Blurb(r common.Record) string {
	parts := []string{}
	seen := map[string]bool{}
	for _, kind := range []string{"email", "tel", "url", "date", "postal", "text"} {
		for _, f := range r.Fields {
			if f.Kind != kind || seen[f.Key] || f.Value == "" {
				continue
			}
			seen[f.Key] = true
			parts = append(parts, commands.Ellipsis(f.Value, 34))
			break
		}
		if len(parts) >= 3 {
			break
		}
	}
	return strings.Join(parts, "  ·  ")
}

// One line of the picker.
//
// `withType` is for the general command, where a list can hold a person and a
// wine and the collection is the difference between them. The address book
// leaves it off: every row is a contact and a column saying so is a column of
// the same word.
func PickLine(r common.Record, withType bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s%s", commands.C(commands.AnsiBold), r.Name, commands.C(commands.AnsiReset))
	if withType && r.Type != "" {
		fmt.Fprintf(&b, "  %s[%s]%s", commands.C(commands.AnsiCyan), r.Type, commands.C(commands.AnsiReset))
	}
	if len(r.Tags) > 0 {
		fmt.Fprintf(&b, "  %s:%s:%s", commands.C(commands.AnsiPurple),
			strings.Join(r.Tags, ":"), commands.C(commands.AnsiReset))
	}
	if blurb := Blurb(r); blurb != "" {
		fmt.Fprintf(&b, "  %s%s%s", commands.C(commands.AnsiDim), blurb, commands.C(commands.AnsiReset))
	}
	// The file is in the display as well as in the address, because it is
	// searchable there and "the one in the work file" is a thing people type.
	fmt.Fprintf(&b, "  %s%s:%d%s", commands.C(commands.AnsiDim),
		r.Filename, r.LineNum+1, commands.C(commands.AnsiReset))
	return b.String()
}

// RenderPane draws one record in full: who or what it is, its fields, whatever
// was written about it, and what has happened to it.
//
// `all` shows every field and the whole history rather than the useful part of
// each - the same flag `orgs rec -all` has, because a pane and a `show` are the
// same question asked from two places.
func RenderPane(r common.Record, width int, all bool) {
	// ---- the name, the collection, the tags
	fmt.Printf("%s%s%s", commands.C(commands.AnsiBold), r.Name, commands.C(commands.AnsiReset))
	if r.Type != "" {
		fmt.Printf("  %s%s%s", commands.C(commands.AnsiCyan), r.Type, commands.C(commands.AnsiReset))
	}
	if len(r.Tags) > 0 {
		fmt.Printf("  %s:%s:%s", commands.C(commands.AnsiPurple),
			strings.Join(r.Tags, ":"), commands.C(commands.AnsiReset))
	}
	fmt.Println()
	fmt.Printf("%s%s:%d%s\n\n", commands.C(commands.AnsiDim),
		commands.BaseName(r.Filename), r.LineNum+1, commands.C(commands.AnsiReset))

	// ---- the fields
	fields := r.Fields
	if !all {
		// A picture is a path, and a path is not something to read.
		kept := fields[:0:0]
		for _, f := range fields {
			if f.Kind == "image" {
				continue
			}
			kept = append(kept, f)
		}
		fields = kept
	}
	if len(fields) > 0 {
		sorted := append([]common.RecordField{}, fields...)
		sort.SliceStable(sorted, func(a, b int) bool {
			return kindRank(sorted[a].Kind) < kindRank(sorted[b].Kind)
		})
		w := 0
		for _, f := range sorted {
			if n := commands.RuneLen(FieldLabel(f)); n > w {
				w = n
			}
		}
		commands.OpenBox("fields", width)
		for _, f := range sorted {
			commands.BoxLine(fmt.Sprintf("%s%s%s  %s",
				commands.C(commands.AnsiDim), pad(FieldLabel(f), w), commands.C(commands.AnsiReset),
				f.Value))
		}
		commands.CloseBox(width)
	} else {
		fmt.Printf("%s(no fields yet)%s\n", commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	}

	// ---- what was written about it
	if strings.TrimSpace(r.Notes) != "" {
		fmt.Println()
		commands.OpenBox("notes", width)
		commands.BoxText(r.Notes)
		commands.CloseBox(width)
	}

	// ---- what has happened to it
	//
	// The history usually opens with the line saying it was added, and saying
	// that twice reads as two things having happened.
	shown := r.History
	if !all && len(shown) > 4 {
		shown = shown[:4]
	}
	if len(shown) > 0 || (r.Added != "" && !hasAdded(r.History)) {
		fmt.Println()
		commands.OpenBox("history", width)
		for _, h := range shown {
			what := h.What
			if what != "" {
				what = "  " + what
			}
			commands.BoxLine(fmt.Sprintf("%s%-8s %s%s%s",
				commands.C(commands.AnsiDim), h.Kind, h.When, what, commands.C(commands.AnsiReset)))
		}
		if r.Added != "" && !hasAdded(r.History) {
			commands.BoxLine(fmt.Sprintf("%sadded    %s%s",
				commands.C(commands.AnsiDim), r.Added, commands.C(commands.AnsiReset)))
		}
		if len(shown) < len(r.History) {
			commands.BoxLine(fmt.Sprintf("%s… %d more%s",
				commands.C(commands.AnsiDim), len(r.History)-len(shown), commands.C(commands.AnsiReset)))
		}
		commands.CloseBox(width)
	}
}

func hasAdded(h []common.RecordChange) bool {
	for _, c := range h {
		if c.Kind == "added" {
			return true
		}
	}
	return false
}

func pad(s string, n int) string {
	for commands.RuneLen(s) < n {
		s += " "
	}
	return s
}

// ---------------------------------------------------------------------------
// The picker
// ---------------------------------------------------------------------------
//
// Here rather than in either command for the same reason the drawing is: `orgs
// rec` and `orgs contact` are one engine with different manners, and a picker
// written twice would drift twice.

type PickOptions struct {
	// The subcommand name this binary answers to - "rec" or "contact". The
	// pane is a second run of this binary, so it has to be told which of the
	// two names to call back into; getting it wrong gives a pane that works
	// for one command and is blank for the other.
	Verb string
	// What to put in front of the box.
	Prompt string
	// Whether to say which collection each row is in. The general command
	// does; the address book does not, because every row is a contact and a
	// column saying so is a column of the same word.
	WithType bool
	// Draw every field and the whole history in the pane.
	All bool
}

// Choose puts the list up and hands back what was picked.
func Choose(core *commands.Core, recs []common.Record, o PickOptions) []common.Record {
	self2, err := commands.SelfCommand(core)
	if err != nil {
		fmt.Fprintf(os.Stderr, "orgs %s: no preview (%v)\n", o.Verb, err)
	}

	// A record has a hash of its own, so unlike a link it is addressed
	// directly rather than by its place in the list - which means the pane
	// still finds it after the list has been filtered under it.
	lines := make([]string, 0, len(recs))
	for _, r := range recs {
		lines = append(lines, commands.PickLine([]string{r.Hash}, PickLine(r, o.WithType)))
	}

	opts := commands.PickOpts{
		Lines:         lines,
		AddressFields: 1,
		Prompt:        o.Prompt,
		Header:        "enter: read it · ctrl-o: edit · ctrl-/: hide pane",
	}
	if self2 != "" {
		opts.Preview = self2 + " " + o.Verb + " preview -hash {1}"
		if o.All {
			opts.Preview += " -all"
		}
		opts.Extra = []string{
			"--bind", "ctrl-o:execute-silent(" + self2 + " " + o.Verb + " open -hash {1})",
		}
	}

	byHash := map[string]common.Record{}
	for _, r := range recs {
		byHash[r.Hash] = r
	}
	out := []common.Record{}
	for _, chosen := range commands.Pick(opts) {
		addr, ok := commands.Address(chosen, 1)
		if !ok {
			continue
		}
		if r, found := byHash[addr[0]]; found {
			out = append(out, r)
		}
	}
	return out
}

// ByHash is one record, fetched. The hash goes into the path encoded a second
// time (`commands.HashPath`): it arrives as base64 already, and written into a
// url as it stands its "+" becomes a space and its "/" a path separator.
func ByHash(core *commands.Core, hash string) (common.Record, bool) {
	if strings.TrimSpace(hash) == "" {
		return common.Record{}, false
	}
	r, err := commands.SendReceiveGetErr[common.Record](core, "record/"+commands.HashPath(hash), nil)
	// The record endpoint answers a refusal as a ResultMsg, which decodes into
	// a Record with nothing in it - so an empty name is the failure, not err.
	if err != nil || r.Name == "" {
		return common.Record{}, false
	}
	return r, true
}

// Preview draws the pane fzf asked for, and says so plainly when the record has
// gone rather than leaving a blank half of the screen.
func Preview(core *commands.Core, hash string, all bool) {
	r, ok := ByHash(core, hash)
	if !ok {
		fmt.Printf("%sthat record is not there any more%s\n",
			commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
		return
	}
	RenderPane(r, commands.PaneWidth(), all)
}
