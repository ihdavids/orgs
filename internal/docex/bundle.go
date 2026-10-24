package docex

// The part of the documentation `orgs help` and the man pages carry in the
// binary: what is written about each command, and the query language. It is
// picked out of the same tree cmd/docex writes, so the two cannot disagree,
// and kept as json beside the help command (cmd/oc/commands/help/docs.json),
// where a test fails when it no longer matches the sources.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Bundle struct {
	// What is written about a command, by its name: every heading in a guide
	// or SDOC block that names it as =orgs NAME=, and the SDOC Commands::Name
	// sections. Each is org text with its own heading at level one.
	Commands map[string][]Section
	// The query language, as the Querying section explains it, less its
	// list of functions: that list is QueryFuncs, so help can list what the
	// binary really has.
	Querying string
	// A query function's one line, by its name, from the Querying section's
	// reference (- *Name* - what it does).
	QueryFuncs map[string]string
	// The usage lines a command's package opens with, by command:
	//
	//	orgs habits -due                only what wants doing today
	//
	// taken from the comment above its import block, the place every command
	// already shows itself in.
	Examples map[string][]string
}

type Section struct {
	// Where it was written: the section path in the documentation.
	From string
	Text string
}

// BuildBundle reads the sources and guides under root the way cmd/docex does
// (the guides in root/docs), leaving out hidden directories - worktrees and
// the like hold copies of the very same files - and node_modules.
func BuildBundle(root string) (*Bundle, error) {
	t := NewTree()
	examples := map[string][]string{}
	if abs, err := filepath.Abs(root); err == nil {
		t.OutputDir = abs
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if t.IsSource(path) {
			t.ProcessFile(path)
		}
		if strings.Contains(filepath.ToSlash(path), "cmd/oc/commands/") && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			examplesOf(path, examples)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	guides, _ := filepath.Glob(filepath.Join(root, "docs", "*.org"))
	for _, g := range guides {
		t.ProcessGuide(g)
	}
	b := t.Bundle()
	b.Examples = examples
	return b, nil
}

var exampleRe = regexp.MustCompile(`^//\t(orgs ([a-z][a-z0-9-]*)\b.*?)\s*$`)

// examplesOf adds the `//<tab>orgs name ...` lines above a file's imports.
func examplesOf(path string, into map[string][]string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if strings.HasPrefix(l, "import") || strings.HasPrefix(l, "func ") || strings.HasPrefix(l, "type ") {
			return
		}
		m := exampleRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		ex := m[1]
		// A command line continued with a backslash goes on to the lines
		// under it.
		for strings.HasSuffix(ex, `\`) && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "//\t") {
			i++
			ex += "\n" + strings.TrimRight(strings.TrimPrefix(lines[i], "//\t"), " ")
		}
		seen := false
		for _, e := range into[m[2]] {
			seen = seen || e == ex
		}
		if !seen {
			into[m[2]] = append(into[m[2]], ex)
		}
	}
}

// BundleJSON is BuildBundle as the bytes written to docs.json.
func BundleJSON(root string) ([]byte, error) {
	b, err := BuildBundle(root)
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(b, "", " ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

var orgsCmdRe = regexp.MustCompile("[=~]orgs ([a-z][a-z0-9-]*)")
var headingRe = regexp.MustCompile(`^(\*+)\s+(.*)$`)
var queryFuncRe = regexp.MustCompile(`^\s*- \*([A-Za-z][A-Za-z0-9]*)\* - (.*)$`)

func (t *Tree) Bundle() *Bundle {
	b := &Bundle{Commands: map[string][]Section{}, QueryFuncs: map[string]string{}}
	add := func(cmd, from, text string) {
		text = strings.TrimRight(text, "\n") + "\n"
		for _, s := range b.Commands[cmd] {
			if s.Text == text {
				return
			}
		}
		b.Commands[cmd] = append(b.Commands[cmd], Section{From: from, Text: text})
	}
	var walk func(path []string, gr map[string]*DocNode)
	walk = func(path []string, gr map[string]*DocNode) {
		for _, k := range SortedKeys(gr) {
			n := gr[k]
			here := append(append([]string{}, path...), k)
			from := strings.Join(here, " :: ")
			if n.Docs != "" {
				// The Commands section's own blocks are about the command
				// they are named after.
				if len(path) == 1 && path[0] == "Commands" {
					add(strings.ToLower(k), from, relevel(strings.Split(n.Docs, "\n")))
				}
				for _, sub := range headingSubtrees(n.Docs) {
					for _, m := range orgsCmdRe.FindAllStringSubmatch(sub.title, -1) {
						add(m[1], from, sub.text)
					}
				}
			}
			walk(here, n.Children)
		}
	}
	walk(nil, t.Groups)

	if q, ok := t.Groups["Querying"]; ok {
		text := ""
		var gather func(n *DocNode)
		gather = func(n *DocNode) {
			text += n.Docs
			for _, k := range SortedKeys(n.Children) {
				gather(n.Children[k])
			}
		}
		for _, k := range SortedKeys(q.Children) {
			gather(q.Children[k])
		}
		b.Querying, b.QueryFuncs = splitQuerying(text)
	}
	return b
}

type subtree struct{ title, text string }

// Every heading in an org text with what is under it, its heading moved to
// level one. Lines in blocks are never headings.
func headingSubtrees(doc string) []subtree {
	lines := strings.Split(doc, "\n")
	level := make([]int, len(lines))
	inBlock := false
	for i, l := range lines {
		if inBlock {
			if blockEnd.MatchString(l) {
				inBlock = false
			}
			continue
		}
		if blockStart.MatchString(l) {
			inBlock = true
			continue
		}
		if m := headingRe.FindStringSubmatch(l); m != nil {
			level[i] = len(m[1])
		}
	}
	out := []subtree{}
	for i, l := range lines {
		if level[i] == 0 {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if level[j] > 0 && level[j] <= level[i] {
				end = j
				break
			}
		}
		out = append(out, subtree{title: headingRe.FindStringSubmatch(l)[2], text: relevel(lines[i:end])})
	}
	return out
}

// relevel moves the headings of a piece of org text so the shallowest is at
// level one.
func relevel(lines []string) string {
	min := 0
	inBlock := false
	isHeading := make([]bool, len(lines))
	for i, l := range lines {
		if inBlock {
			if blockEnd.MatchString(l) {
				inBlock = false
			}
			continue
		}
		if blockStart.MatchString(l) {
			inBlock = true
			continue
		}
		if m := headingRe.FindStringSubmatch(l); m != nil {
			isHeading[i] = true
			if min == 0 || len(m[1]) < min {
				min = len(m[1])
			}
		}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if isHeading[i] && min > 1 {
			l = l[min-1:]
		}
		out[i] = l
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
}

// splitQuerying takes the reference list (- *Name* - text, continued on the
// lines indented under it) out of the Querying text, and the heading that
// holds nothing but that list with it.
func splitQuerying(text string) (string, map[string]string) {
	funcs := map[string]string{}
	lines := strings.Split(text, "\n")
	keep := []string{}
	last := ""
	itemIndent := 0
	for _, l := range lines {
		if m := queryFuncRe.FindStringSubmatch(l); m != nil {
			last = m[1]
			funcs[last] = strings.TrimSpace(m[2])
			itemIndent = len(l) - len(strings.TrimLeft(l, " \t"))
			continue
		}
		if last != "" {
			ind := len(l) - len(strings.TrimLeft(l, " \t"))
			if t := strings.TrimSpace(l); t != "" && ind > itemIndent && !strings.HasPrefix(t, "- ") && !headingRe.MatchString(l) {
				funcs[last] += " " + t
				continue
			}
			if strings.TrimSpace(l) != "" {
				last = ""
			}
		}
		keep = append(keep, l)
	}
	// A heading left with nothing under it was the list's.
	out := []string{}
	for i := 0; i < len(keep); i++ {
		if m := headingRe.FindStringSubmatch(keep[i]); m != nil {
			j := i + 1
			for j < len(keep) && strings.TrimSpace(keep[j]) == "" {
				j++
			}
			if j == len(keep) || (headingRe.MatchString(keep[j]) && len(headingRe.FindStringSubmatch(keep[j])[1]) <= len(m[1])) {
				i = j - 1
				continue
			}
		}
		out = append(out, keep[i])
	}
	return relevel(trimBlank(out)), funcs
}

func trimBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	return lines
}
