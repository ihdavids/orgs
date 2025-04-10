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
					ok := false
					if docName,ok = m["group"]; !ok {
						docName = "General"
					}
				}
			}
		}
	}
}

func WriteRecursive(lvl int, gr map[string]*DocNode, f *os.File) {
		keys := []string{}
		for k,_ := range gr {
			keys = append(keys, k)
		}
		//fmt.Printf("%v\n", keys)
		sort.Slice(keys, func(i, j int) bool { return strings.ToLower(keys[i]) < strings.ToLower(keys[j]) }) 
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

func main() {
	//args := flag.Args()
	output := ""
	rootDir := ""

	flag.StringVar(&output, "out", "./out.org", "Output file to contain documentation")
	flag.StringVar(&rootDir, "src", "./", "Where to find source files.")
    flag.Var(&introFiles, "start", "Intro files to add first")

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
	//fmt.Printf("%v\n", groups)
	WriteDocs(output)
}
