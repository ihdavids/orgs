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

func IsGoFile(filename string) bool {
	return filepath.Ext(filename) == ".go"
}

var groups map[string]string = map[string]string{}


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
	fmt.Printf("Processing %s\n", filename)
	if r, err := os.Open(filename); err == nil {
		defer r.Close()
		scanner := bufio.NewScanner(r)
		inDoc := false
		docName := ""
		out := ""
		for scanner.Scan() {
			line := scanner.Text()
			if inDoc {
				if m := ReMatch(`^\s*EDOC\s+[*]/`, line); m != nil {
					inDoc = false
					if docName == "" {
						docName = "General"
					}
					groups[docName] = out
					out = ""
					docName = ""
				} else {
					// Reindent for grouping
					if m := ReMatch(`^\s*(?P<stars>[*]+)\s+(?P<heading>.+)`, line); m != nil {
						line = m["stars"] + "* " + m["heading"]
					}
					out += line + "\n"
				}

			} else {
				if m := ReMatch(`^\s*/[*]\s+SDOC(([:]\s+(?P<group>[a-zA-Z0-9 ]+)\s*)|(?P<nogroup>\s+)`, line); m != nil {
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

func WriteDocs(output string) {
	if f, err := os.OpenFile(output, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600); err == nil {
		defer f.Close()
		keys := make([]string, len(groups))
		for k,_ := range groups {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return strings.ToLower(keys[i]) < strings.ToLower(keys[j]) }) 
		for _,k := range keys {
			v := groups[k]
			f.WriteString("* ")
			f.WriteString(k)
			f.WriteString("\n")
			f.WriteString(v)
			f.WriteString("\n")
		}
	}
}

func main() {
	//args := flag.Args()
	output := ""
	rootDir := ""

	flag.StringVar(&output, "out", "./out.org", "Output file to contain documentation")
	flag.StringVar(&rootDir, "src", "./", "Where to find source files.")
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
	for _, file := range files {
		ProcessFile(file)
	}
	WriteDocs(output)
}
