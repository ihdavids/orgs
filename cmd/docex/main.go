package main

// docex - the documentation out of the sources as one org document: the
// SDOC / EDOC blocks in the Go files plus the guides in docs/*.org. The
// reading is internal/docex, shared with the bundle `orgs help` carries.
//
//	docex -src . -out docs.org -start docs/overview.org
//	docex -src . -bundle cmd/oc/commands/help/docs.json

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/internal/docex"
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
	output := ""
	rootDir := ""
	guideDir := ""
	bundle := ""
	title := "Orgs"
	theme := "docs"

	flag.StringVar(&output, "out", "./out.org", "Output file to contain documentation")
	flag.StringVar(&rootDir, "src", "./", "Where to find source files.")
	flag.Var(&introFiles, "start", "Intro files to add first")
	flag.StringVar(&guideDir, "guides", "", "Directory of org guides to merge in (default <src>/docs, \"-\" for none)")
	exts := ".go"
	flag.StringVar(&exts, "ext", ".go", "Source file extensions to read SDOC blocks from, comma separated (.go,.ts)")
	flag.StringVar(&title, "title", "Orgs", "The document's #+TITLE")
	flag.StringVar(&theme, "theme", "docs", "The document's #+HTML_THEME")
	flag.StringVar(&bundle, "bundle", "", "Write the json bundle `orgs help` embeds to this file instead of the org document")

	flag.Parse()

	if bundle != "" {
		data, err := docex.BundleJSON(rootDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "docex: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(bundle, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "docex: %v\n", err)
			os.Exit(1)
		}
		return
	}

	t := docex.NewTree()
	t.Extensions = map[string]bool{}
	for _, e := range strings.Split(exts, ",") {
		if e = strings.TrimSpace(e); e != "" {
			if !strings.HasPrefix(e, ".") {
				e = "." + e
			}
			t.Extensions[e] = true
		}
	}

	if output == "" || rootDir == "" {
		fmt.Printf("You must specify a source and destination directory")
	}

	files, err := t.Sources(rootDir)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Ensure we do not accidentally process the existing output file
	os.Remove(output)
	if abs, err := filepath.Abs(filepath.Dir(output)); err == nil {
		t.OutputDir = abs
	}
	for _, file := range files {
		t.ProcessFile(file)
	}
	if guideDir == "" {
		guideDir = filepath.Join(rootDir, "docs")
	}
	if guideDir != "-" {
		// Only the top of the directory: subdirectories hold data, not guides.
		guides, _ := filepath.Glob(filepath.Join(guideDir, "*.org"))
		for _, guide := range guides {
			if !isIntro(guide) {
				t.ProcessGuide(guide)
			}
		}
	}
	t.WriteDocs(output, title, theme, introFiles)
}
