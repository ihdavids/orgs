package beamer

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// CompileOptions says how a deck becomes a pdf.
type CompileOptions struct {
	// pdflatex, xelatex, lualatex, latexmk, or a path to one. Empty picks
	// the first of pdflatex, xelatex and lualatex that is installed.
	Engine string
	// minted runs pygments, which latex is only allowed to do with
	// -shell-escape; nothing else asks for it.
	ShellEscape bool
	// How long one run may take; a minute when zero.
	Timeout time.Duration
}

// ErrNoTeX is a machine with no latex to compile with.
var ErrNoTeX = errors.New("no TeX is installed here (pdflatex, xelatex or lualatex): install TeX Live or MacTeX, or export the .tex and compile it elsewhere")

// Where TeX installs on a mac, which a server started by launchd does not
// have on its PATH.
var texDirs = []string{"/Library/TeX/texbin", "/usr/local/texlive/bin", "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin"}

func findTool(name string) (string, error) {
	if filepath.IsAbs(name) {
		if _, err := os.Stat(name); err == nil {
			return name, nil
		}
		return "", fmt.Errorf("%s is not there", name)
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	for _, d := range texDirs {
		p := filepath.Join(d, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s is not installed", name)
}

// Engine is the latex Compile would use, and whether there is one.
func Engine(asked string) (string, error) {
	if asked != "" {
		p, err := findTool(asked)
		if err != nil {
			return "", fmt.Errorf("beamer: the configured latex %v", err)
		}
		return p, nil
	}
	for _, e := range []string{"pdflatex", "xelatex", "lualatex"} {
		if p, err := findTool(e); err == nil {
			return p, nil
		}
	}
	return "", ErrNoTeX
}

// An error line, as latex writes it with and without -file-line-error.
var texErrRe = regexp.MustCompile(`(?m)^(! |[^\s:]+:\d+: ).*$`)

// Compile turns a deck into a pdf in a directory of its own, run twice so
// the outline, frame numbers and labels are right. Pictures are named by
// absolute paths in the tex, so where it compiles does not matter.
func Compile(tex string, o CompileOptions) ([]byte, error) {
	engine, err := Engine(o.Engine)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "orgs-beamer-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	src := filepath.Join(dir, "deck.tex")
	if err := os.WriteFile(src, []byte(tex), 0644); err != nil {
		return nil, err
	}
	timeout := o.Timeout
	if timeout == 0 {
		timeout = time.Minute
	}
	base := strings.TrimSuffix(filepath.Base(engine), filepath.Ext(engine))
	args := []string{"-interaction=nonstopmode", "-halt-on-error", "-file-line-error"}
	if o.ShellEscape {
		args = append(args, "-shell-escape")
	}
	runs := 2
	if base == "latexmk" {
		// latexmk runs as often as it takes on its own.
		args = append([]string{"-pdf"}, args...)
		runs = 1
	}
	args = append(args, "deck.tex")
	var log []byte
	for i := 0; i < runs; i++ {
		cmd := exec.Command(engine, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "TEXINPUTS=.:"+os.Getenv("TEXINPUTS"))
		out := bytes.Buffer{}
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("beamer: %s: %v", base, err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err = <-done:
		case <-time.After(timeout):
			cmd.Process.Kill()
			<-done
			return nil, fmt.Errorf("beamer: %s took longer than %s", base, timeout)
		}
		log = out.Bytes()
		if err != nil {
			return nil, fmt.Errorf("beamer: %s failed: %s", base, texError(log))
		}
	}
	pdf, err := os.ReadFile(filepath.Join(dir, "deck.pdf"))
	if err != nil {
		return nil, fmt.Errorf("beamer: %s made no pdf: %s", base, texError(log))
	}
	return pdf, nil
}

// The part of a latex log a person needs: the first error and the lines
// after it, else the end of the log.
func texError(log []byte) string {
	s := string(log)
	if loc := texErrRe.FindStringIndex(s); loc != nil {
		end := loc[0] + 600
		if end > len(s) {
			end = len(s)
		}
		return strings.TrimSpace(s[loc[0]:end])
	}
	if len(s) > 600 {
		s = "…" + s[len(s)-600:]
	}
	return strings.TrimSpace(s)
}
