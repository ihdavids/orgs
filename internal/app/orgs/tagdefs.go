package orgs

// A file's #+TAGS: lines (org manual 6.2, 6.3), read for what they say about
// how tags relate:
//
//	#+TAGS: [ Work : ProjA ProjB {P@.+} ]   a group tag: searching Work finds
//	                                        ProjA, ProjB and tags matching P@.+
//	#+TAGS: { @home(h) @work(w) }           mutually exclusive: setting one
//	                                        takes the others off
//	#+TAGS: { Context : @home @work }       both at once
//
// Fast keys - (h) - are dropped. Groups nest (a member may be a group tag of
// its own), and a cycle is not followed twice. The server's tagGroups in the
// yaml are a separate thing, asked for by InTagGroup.

import (
	"regexp"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

type tagDefs struct {
	// Group tag (lower case) to its members as written; {re} members are
	// regular expressions.
	groups map[string][]string
	// Sets of which a heading may carry one.
	exclusive [][]string
}

var fastKeyRe = regexp.MustCompile(`\([^)]*\)$`)

// parseTagDefs reads #+TAGS: lines.
func parseTagDefs(spec string) tagDefs {
	d := tagDefs{groups: map[string][]string{}}
	words := strings.Fields(strings.ReplaceAll(spec, "\n", " "))
	for i := 0; i < len(words); i++ {
		w := words[i]
		if w != "[" && w != "{" {
			continue
		}
		closer := "]"
		if w == "{" {
			closer = "}"
		}
		members := []string{}
		group := ""
		j := i + 1
		for ; j < len(words) && words[j] != closer; j++ {
			m := words[j]
			if m == ":" && len(members) == 1 && group == "" {
				group, members = members[0], []string{}
				continue
			}
			if !(strings.HasPrefix(m, "{") && strings.HasSuffix(m, "}")) {
				m = fastKeyRe.ReplaceAllString(m, "")
			}
			if m != "" {
				members = append(members, m)
			}
		}
		i = j
		if group != "" {
			key := strings.ToLower(group)
			d.groups[key] = append(d.groups[key], members...)
		}
		if w == "{" {
			set := []string{}
			for _, m := range members {
				if !strings.HasPrefix(m, "{") {
					set = append(set, m)
				}
			}
			if len(set) > 1 {
				d.exclusive = append(d.exclusive, set)
			}
		}
	}
	return d
}

func fileTagDefs(f *common.OrgFile) tagDefs {
	if f == nil || f.Doc == nil {
		return tagDefs{groups: map[string][]string{}}
	}
	return parseTagDefs(f.Doc.BufferSettings["TAGS"])
}

// expand is a tag with every member of its group, nested groups included:
// names, and the patterns of {re} members.
func (d tagDefs) expand(tag string) (names, patterns []string) {
	seen := map[string]bool{}
	var walk func(t string)
	walk = func(t string) {
		if seen[strings.ToLower(t)] {
			return
		}
		seen[strings.ToLower(t)] = true
		names = append(names, t)
		for _, m := range d.groups[strings.ToLower(t)] {
			if strings.HasPrefix(m, "{") && strings.HasSuffix(m, "}") {
				patterns = append(patterns, m[1:len(m)-1])
				continue
			}
			walk(m)
		}
	}
	walk(tag)
	return names, patterns
}

// hasTagOrGroup is HasTag with the file's group tags expanded: searching a
// group tag finds a heading with any of its members.
func hasTagOrGroup(tag string, sec *org.Section, f *common.OrgFile) bool {
	var doc *org.Document
	if f != nil {
		doc = f.Doc
	}
	d := fileTagDefs(f)
	if len(d.groups) == 0 {
		return HasTag(tag, sec, doc)
	}
	names, patterns := d.expand(tag)
	for _, n := range names {
		if HasTag(n, sec, doc) {
			return true
		}
	}
	for _, p := range patterns {
		if HasTagRegex(p, sec, doc) {
			return true
		}
	}
	return false
}

// exclusiveTags is want with the file's mutually exclusive sets kept to one
// tag each: of two in a set, the one the heading did not have before (the one
// just added) stays, or the last one written when both or neither are new.
func exclusiveTags(d tagDefs, before, want []string) []string {
	had := map[string]bool{}
	for _, t := range before {
		had[t] = true
	}
	drop := map[string]bool{}
	for _, set := range d.exclusive {
		in := map[string]bool{}
		for _, m := range set {
			in[m] = true
		}
		present := []string{}
		for _, t := range want {
			if in[t] {
				present = append(present, t)
			}
		}
		if len(present) < 2 {
			continue
		}
		keep := present[len(present)-1]
		for i := len(present) - 1; i >= 0; i-- {
			if !had[present[i]] {
				keep = present[i]
				break
			}
		}
		for _, t := range present {
			if t != keep {
				drop[t] = true
			}
		}
	}
	out := []string{}
	for _, t := range want {
		if !drop[t] {
			out = append(out, t)
		}
	}
	return out
}
