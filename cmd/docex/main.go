package main

import (
	"flag"
	"fmt"
	"os"
	"bufio"
	"path/filepath"
	"strings"
)

func IsGoFile(filename string) bool {
	return filepath.Ext(filename) == ".go"
}

func ProcessFile(filename string, output string) {
	fmt.Printf("Processing %s\n", filename)
	if r, err := os.Open(filename); err == nil {
		defer r.Close()
		scanner := bufio.NewScanner(r)
		inDoc := false
		out := ""
		for scanner.Scan() {
			line := scanner.Text()
			if inDoc {
				if strings.Contains(line, "EDOC */") {
					inDoc = false
				} else {
					out += line + "\n"
				}

			} else {
				if strings.Contains(line, "/* SDOC") {
					inDoc = true
				}
			}
		}
		if out != "" {
			if f, err := os.OpenFile(output, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600); err == nil {
				defer f.Close()
				f.WriteString(out)
				f.WriteString("\n")
			}
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
		ProcessFile(file, output)
	}
}

