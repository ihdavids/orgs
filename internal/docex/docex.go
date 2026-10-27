// Package docex reads the documentation out of the sources: the comment
// blocks fenced with SDOC / EDOC, and the hand written guides in docs/*.org.
//
// cmd/docex writes it out as one org document; Bundle picks out what
// `orgs help` and the man pages need, which the binary carries embedded.
package docex

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type DocNode struct {
	Name     string
	Docs     string
	Children map[string]*DocNode
	// A guide (an org file from the guides directory) is written ahead of
	// the reference sections beside it.
	Guide bool
	// #+DOC_ORDER in a guide: guides with one come first, lowest first;
	// the rest follow alphabetically.
	Order int
}

// Tree is everything read so far, by section.
type Tree struct {
	Groups map[string]*DocNode
	// What the sources are written in. Orgs is Go; the vscode plugin, which
	// builds its docs with this too, is TypeScript.
	Extensions map[string]bool
	// The directory the merged document is written to. A guide's file: links
	// are written relative to the guide (so they work when the guide is read
	// on its own) and have to be rebased to here, or every image in docs/
	// breaks once the guide is merged into a document one directory up.
	OutputDir string
}

func NewTree() *Tree {
	return &Tree{Groups: map[string]*DocNode{}, Extensions: map[string]bool{".go": true}, OutputDir: "."}
}

func (t *Tree) IsSource(filename string) bool {
	return t.Extensions[filepath.Ext(filename)]
}

func reMatch(regEx, txt string) map[string]string {
	var compRegEx = regexp.MustCompile(regEx)
	match := compRegEx.FindStringSubmatch(txt)

	// Return nil if we did not match anything!
	if match == nil || len(match) <= 0 {
		return nil
	}

	empty := true
	for _, m := range match {
		if m != "" {
			empty = false
			break
		}
	}
	if empty {
		return nil
	}
	// Otherwise build a param map out of the match
	paramsMap := make(map[string]string)
	for i, name := range compRegEx.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}
	return paramsMap
}

// ProcessFile reads the SDOC blocks of one source file into the tree.
func (t *Tree) ProcessFile(filename string) {
	if r, err := os.Open(filename); err == nil {
		defer r.Close()
		scanner := bufio.NewScanner(r)
		inDoc := false
		inBlock := false
		docName := ""
		heading := ""
		out := ""
		for scanner.Scan() {
			line := scanner.Text()
			if inDoc {
				if m := reMatch(`^\s*(?P<EMatch>EDOC)\s*[*]/`, line); m != nil {
					inDoc = false
					docName = strings.TrimSpace(docName)
					if docName == "" {
						docName = "General"
					}
					docNames := strings.Split(docName, "::")
					if out != "" {
						gr := t.Groups
						for _, cur := range docNames {
							cur = strings.TrimSpace(cur)
							var d *DocNode = nil
							ok := false
							if d, ok = gr[cur]; ok {
								gr = d.Children
							} else {
								d = &DocNode{Name: cur, Docs: "", Children: map[string]*DocNode{}}
								gr[cur] = d
								gr = d.Children
							}
						}

						if dn, ok2 := gr[heading]; ok2 {
							dn.Docs = out
						} else {
							dn := &DocNode{Name: heading, Docs: out, Children: map[string]*DocNode{}}
							gr[heading] = dn
						}
					}
					out = ""
					docName = ""
					heading = ""
				} else if inBlock {
					// An example inside a block is written as it is: a "* Mon"
					// there is text, and pulled to column zero it would be a
					// heading that ends the block.
					if blockEnd.MatchString(line) {
						inBlock = false
					}
					out += line + "\n"
				} else if blockStart.MatchString(line) {
					inBlock = true
					out += line + "\n"
				} else {
					// Reindent for grouping
					if m := reMatch(`^\s*(?P<stars>[*]+)\s+(?P<heading>.+)`, line); m != nil {
						line = m["stars"] + "* " + m["heading"]
						if heading == "" && len(m["stars"]) > 0 {
							heading = m["heading"]
						}
					}
					out += line + "\n"
				}

			} else {
				if m := reMatch(`^\s*/[*]\s+SDOC(([:]\s+(?P<group>[:a-zA-Z0-9 ]+)\s*)|(?P<nogroup>\s*))`, line); m != nil {
					inDoc = true
					inBlock = false
					ok := false
					if docName, ok = m["group"]; !ok {
						docName = "General"
					}
				}
			}
		}
	}
}

// SectionChildren walks (and creates) the section path a::b::c, returning its
// children.
func (t *Tree) SectionChildren(section string) map[string]*DocNode {
	gr := t.Groups
	if strings.TrimSpace(section) == "" {
		return gr
	}
	for _, cur := range strings.Split(section, "::") {
		cur = strings.TrimSpace(cur)
		d, ok := gr[cur]
		if !ok {
			d = &DocNode{Name: cur, Docs: "", Children: map[string]*DocNode{}}
			gr[cur] = d
		}
		gr = d.Children
	}
	return gr
}

var guideKeyword = regexp.MustCompile(`^#\+(?P<key>[A-Za-z_]+):\s*(?P<val>.*)$`)
var guideHeading = regexp.MustCompile(`^(?P<stars>[*]+)\s`)
var blockStart = regexp.MustCompile(`(?i)^\s*#\+begin_`)
var blockEnd = regexp.MustCompile(`(?i)^\s*#\+end_`)

// ProcessGuide reads one hand written org guide into the tree. The keyword
// lines at the top of the file are dropped: #+TITLE becomes the guide's
// heading and #+DOC_SECTION (a::b, as with SDOC) says which section it sits
// in; without one the guide is a top level section of its own. Headings are
// pushed down to sit under the guide's heading. Lines inside blocks are left
// alone.
func (t *Tree) ProcessGuide(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docex: %v\n", err)
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	title := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	section := ""
	order := 0
	i := 0
	for ; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		m := guideKeyword.FindStringSubmatch(lines[i])
		if m == nil {
			break
		}
		switch strings.ToUpper(m[1]) {
		case "TITLE":
			title = strings.TrimSpace(m[2])
		case "DOC_SECTION":
			section = strings.TrimSpace(m[2])
		case "DOC_ORDER":
			fmt.Sscanf(strings.TrimSpace(m[2]), "%d", &order)
		}
	}
	depth := 0
	if section != "" {
		depth = len(strings.Split(section, "::"))
	}
	out := strings.Repeat("*", depth+1) + " " + title + "\n"
	inBlock := false
	for _, line := range lines[i:] {
		if inBlock {
			if blockEnd.MatchString(line) {
				inBlock = false
			}
		} else if blockStart.MatchString(line) {
			inBlock = true
		} else {
			if m := guideHeading.FindStringSubmatch(line); m != nil {
				line = strings.Repeat("*", depth+1) + line
			}
			line = t.RebaseLinks(line, filepath.Dir(filename))
		}
		out += line + "\n"
	}
	// A guide can also be a section other guides (or SDOC blocks) sit in:
	// what was filed under it before it was read stays under it.
	parent := t.SectionChildren(section)
	node := &DocNode{Name: title, Docs: out, Children: map[string]*DocNode{}, Guide: true, Order: order}
	if old, ok := parent[title]; ok {
		node.Children = old.Children
	}
	parent[title] = node
}

var fileLink = regexp.MustCompile(`\[\[file:([^\]:]+)`)

func (t *Tree) RebaseLinks(line, fromDir string) string {
	return fileLink.ReplaceAllStringFunc(line, func(m string) string {
		target := fileLink.FindStringSubmatch(m)[1]
		if filepath.IsAbs(target) || strings.HasPrefix(target, "~") {
			return m
		}
		abs, err := filepath.Abs(filepath.Join(fromDir, target))
		// Only a file that is there: a guide also writes links as examples
		// of org syntax (=[[file:art/lyra.png]]=), and those stay as written.
		if err != nil {
			return m
		}
		if _, err := os.Stat(abs); err != nil {
			return m
		}
		rel, err := filepath.Rel(t.OutputDir, abs)
		if err != nil {
			return m
		}
		return "[[file:" + filepath.ToSlash(rel)
	})
}

// Sections written after everything else: the API reference is long and
// read by looking things up, so it closes the document.
var lastSections = map[string]bool{"API": true}

// SortedKeys is the order sections are written in: guides first, then by
// #+DOC_ORDER, then alphabetically, with lastSections at the end.
func SortedKeys(gr map[string]*DocNode) []string {
	keys := []string{}
	for k := range gr {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if lastSections[keys[i]] != lastSections[keys[j]] {
			return lastSections[keys[j]]
		}
		if gr[keys[i]].Guide != gr[keys[j]].Guide {
			return gr[keys[i]].Guide
		}
		if oi, oj := gr[keys[i]].Order, gr[keys[j]].Order; oi != oj && (oi > 0 || oj > 0) {
			if oi == 0 || oj == 0 {
				return oi > 0
			}
			return oi < oj
		}
		return strings.ToLower(keys[i]) < strings.ToLower(keys[j])
	})
	return keys
}

func (t *Tree) WriteRecursive(lvl int, gr map[string]*DocNode, f *os.File) {
	for _, k := range SortedKeys(gr) {
		cur := gr[k]
		if cur.Docs == "" {
			f.WriteString(strings.Repeat("*", lvl) + " ")
			f.WriteString(k)
			f.WriteString("\n")
		} else {
			f.WriteString(cur.Docs)
			f.WriteString("\n")
		}
		if len(cur.Children) > 0 {
			t.WriteRecursive(lvl+1, cur.Children, f)
		}
	}
}

// WriteDocs appends the whole tree to output as one org document, the intro
// files first.
func (t *Tree) WriteDocs(output, title, theme string, introFiles []string) {
	if f, err := os.OpenFile(output, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600); err == nil {
		defer f.Close()
		f.WriteString("#+TITLE: " + title + "\n")
		f.WriteString("#+HTML_THEME: " + theme + "\n")

		for _, file := range introFiles {
			if r, err := os.Open(file); err == nil {
				defer r.Close()
				scanner := bufio.NewScanner(r)
				for scanner.Scan() {
					line := t.RebaseLinks(scanner.Text(), filepath.Dir(file))
					f.WriteString(line)
					f.WriteString("\n")
				}
			}
		}

		t.WriteRecursive(1, t.Groups, f)
	}
}

// Sources lists the source files under root, as cmd/docex always has: every
// directory, hidden ones included.
func (t *Tree) Sources(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && t.IsSource(path) {
				files = append(files, path)
			}
			return nil
		})
	return files, err
}
