package main

import (
	"flag"
	"fmt"
	"os"
	"bufio"
	"path/filepath"
	"strings"
	"regexp"
	"sort"
)

type arrayFlags []string

// String is an implementation of the flag.Value interface
func (i *arrayFlags) String() string {
    return fmt.Sprintf("%v", *i)
}

// Set is an implementation of the flag.Value interface
func (i *arrayFlags) Set(value string) error {
    *i = append(*i, value)
    return nil
}

var introFiles arrayFlags


func IsGoFile(filename string) bool {
	return filepath.Ext(filename) == ".go"
}
type DocNode struct {
	Name string
	Docs string
	Children map[string]*DocNode
	// A guide (an org file from the guides directory) is written ahead of
	// the reference sections beside it.
	Guide bool
}
var groups map[string]*DocNode = map[string]*DocNode{}


func ReMatch(regEx, txt string) (map[string]string) {

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

func ProcessFile(filename string) {
	//fmt.Printf("Processing %s\n", filename)
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
				if m := ReMatch(`^\s*(?P<EMatch>EDOC)\s*[*]/`, line); m != nil {
					inDoc = false
					docName = strings.TrimSpace(docName)
					if docName == "" {
						docName = "General"
					}
					docNames := strings.Split(docName, "::")
					// fmt.Printf("DOCNAME: %v\n", docNames)
					if out != "" {
						gr := groups
						for _,cur := range docNames {
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
							//fmt.Printf("CREATE: %s\n", heading)
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
					if m := ReMatch(`^\s*(?P<stars>[*]+)\s+(?P<heading>.+)`, line); m != nil {
						line = m["stars"] + "* " + m["heading"]
						if (heading == "" && len(m["stars"]) > 0) {
							heading = m["heading"]
						}
					}
					out += line + "\n"
				}

			} else {
				if m := ReMatch(`^\s*/[*]\s+SDOC(([:]\s+(?P<group>[:a-zA-Z0-9 ]+)\s*)|(?P<nogroup>\s*))`, line); m != nil {
					inDoc = true
					inBlock = false
					ok := false
					if docName,ok = m["group"]; !ok {
						docName = "General"
					}
				}
			}
		}
	}
}

// Walks (and creates) the section path a::b::c, returning its children.
func SectionChildren(section string) map[string]*DocNode {
	gr := groups
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

// Reads one hand written org guide into the tree. The keyword lines at the
// top of the file are dropped: #+TITLE becomes the guide's heading and
// #+DOC_SECTION (a::b, as with SDOC) says which section it sits in; without
// one the guide is a top level section of its own. Headings are pushed down
// to sit under the guide's heading. Lines inside blocks are left alone.
func ProcessGuide(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docex: %v\n", err)
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	title := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	section := ""
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
		} else if m := guideHeading.FindStringSubmatch(line); m != nil {
			line = strings.Repeat("*", depth+1) + line
		}
		out += line + "\n"
	}
	SectionChildren(section)[title] = &DocNode{Name: title, Docs: out, Children: map[string]*DocNode{}, Guide: true}
}

func WriteRecursive(lvl int, gr map[string]*DocNode, f *os.File) {
		keys := []string{}
		for k,_ := range gr {
			keys = append(keys, k)
		}
		//fmt.Printf("%v\n", keys)
		sort.Slice(keys, func(i, j int) bool {
			if gr[keys[i]].Guide != gr[keys[j]].Guide {
				return gr[keys[i]].Guide
			}
			return strings.ToLower(keys[i]) < strings.ToLower(keys[j])
		})
		for _,k := range keys {
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
				//fmt.Printf("Par: %s => %v\n", cur.Name, cur.Children)
				WriteRecursive(lvl + 1, cur.Children, f)
			}
		}
}

func WriteDocs(output string) {
	if f, err := os.OpenFile(output, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600); err == nil {
		defer f.Close()
		// TODO: Make this configurable
		f.WriteString("#+TITLE: Orgs\n")
		f.WriteString("#+HTML_THEME: docs\n")


		for _,file := range introFiles {
			if r, err := os.Open(file); err == nil {
				defer r.Close()
				scanner := bufio.NewScanner(r)
				for scanner.Scan() {
					line := scanner.Text()
					f.WriteString(line)
					f.WriteString("\n")
				}
			}
		}

		gr := groups
		WriteRecursive(1, gr, f)
	}
}

// An intro file named with -start is written whole at the top, never as a guide too.
func isIntro(file string) bool {
	abs, _ := filepath.Abs(file)
	for _, intro := range introFiles {
		if a, _ := filepath.Abs(intro); a == abs {
			return true
		}
	}
	return false
}

func main() {
	//args := flag.Args()
	output := ""
	rootDir := ""
	guideDir := ""

	flag.StringVar(&output, "out", "./out.org", "Output file to contain documentation")
	flag.StringVar(&rootDir, "src", "./", "Where to find source files.")
    flag.Var(&introFiles, "start", "Intro files to add first")
	flag.StringVar(&guideDir, "guides", "", "Directory of org guides to merge in (default <src>/docs, \"-\" for none)")

	flag.Parse()

	if output == "" || rootDir == "" {
		fmt.Printf("You must specify a source and destination directory")
	}

	var files []string
	err := filepath.Walk(rootDir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && IsGoFile(path) {
				files = append(files, path)
			}
			return nil
		})
	if err != nil {
		fmt.Println(err)
		return
	}


	// Ensure we do not accidentally process the existing output file
	os.Remove(output)
	for _, file := range files {
		ProcessFile(file)
	}
	if guideDir == "" {
		guideDir = filepath.Join(rootDir, "docs")
	}
	if guideDir != "-" {
		// Only the top of the directory: subdirectories hold data, not guides.
		guides, _ := filepath.Glob(filepath.Join(guideDir, "*.org"))
		for _, guide := range guides {
			if !isIntro(guide) {
				ProcessGuide(guide)
			}
		}
	}
	//fmt.Printf("%v\n", groups)
	WriteDocs(output)
}
