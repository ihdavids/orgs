package dnd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// pdfLatexCandidates are the usual install locations, tried in order when no
// pdflatex property is configured and the binary is not on the PATH.
var pdfLatexCandidates = []string{
	"/Library/TeX/texbin/pdflatex",
	"/usr/local/texlive/bin/pdflatex",
	"/usr/bin/pdflatex",
	"/usr/local/bin/pdflatex",
	"/opt/homebrew/bin/pdflatex",
}

// FindPdfLatex locates a pdflatex binary, returning "" when there is none.
func FindPdfLatex() string {
	if p, err := exec.LookPath("pdflatex"); err == nil {
		return p
	}
	for _, c := range pdfLatexCandidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

// RunPdfLatex compiles a .tex file in place. It runs twice because the sheet
// uses forward references for its page count, and it reports the relevant
// part of the log when compilation fails.
func RunPdfLatex(pdflatex, dir, src string) error {
	var lastOut []byte
	for i := 0; i < 2; i++ {
		cmd := exec.Command(pdflatex, "-interaction=nonstopmode", "-halt-on-error",
			"-output-directory", dir, src)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		lastOut = out
		if err != nil {
			if _, statErr := os.Stat(filepath.Join(dir, "sheet.pdf")); statErr == nil && i > 0 {
				// second pass hiccuped but we still have a pdf, keep it
				return nil
			}
			return fmt.Errorf("pdflatex failed: %s\n%s", err, latexErrors(string(lastOut)))
		}
	}
	return nil
}

// latexErrors trims a pdflatex log down to the lines that matter.
func latexErrors(log string) string {
	lines := strings.Split(log, "\n")
	keep := []string{}
	for i, l := range lines {
		if strings.HasPrefix(l, "!") || strings.HasPrefix(l, "l.") {
			start := i
			end := i + 3
			if end > len(lines) {
				end = len(lines)
			}
			keep = append(keep, strings.Join(lines[start:end], "\n"))
		}
	}
	if len(keep) == 0 {
		if len(lines) > 25 {
			return strings.Join(lines[len(lines)-25:], "\n")
		}
		return log
	}
	return strings.Join(keep, "\n")
}
