package ledger

import (
	"regexp"
	"strings"
)

// OrgBlock is a ledger or hledger source block of an org file.
type OrgBlock struct {
	Index  int    // among the file's ledger blocks, from 0
	Name   string // its #+NAME:
	Lang   string // "ledger" or "hledger"
	Begin  int    // the #+BEGIN_SRC line, from 1
	End    int    // the #+END_SRC line
	Header string // what follows the language
	Text   string // the journal, dedented and unescaped
}

var srcBeginRe = regexp.MustCompile(`(?i)^(\s*)#\+begin_src\s+(\S+)(.*)$`)
var srcEndRe = regexp.MustCompile(`(?i)^\s*#\+end_src\b`)
var otherBeginRe = regexp.MustCompile(`(?i)^\s*#\+begin_(example|export|comment)\b`)
var nameRe = regexp.MustCompile(`(?i)^\s*#\+name:\s*(.*?)\s*$`)
var affiliatedRe = regexp.MustCompile(`(?i)^\s*#\+(header|headers|attr_\w+|caption|plot|results)\b`)

// IsLedgerLang is whether a source block's language is a ledger journal.
func IsLedgerLang(l string) bool {
	l = strings.ToLower(l)
	return l == "ledger" || l == "hledger"
}

// OrgBlocks finds every ledger and hledger source block in an org file's
// text. A block inside an example, export or comment block is an example of
// one, not one.
func OrgBlocks(text string) []OrgBlock {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := []OrgBlock{}
	for i := 0; i < len(lines); i++ {
		if m := otherBeginRe.FindStringSubmatch(lines[i]); m != nil {
			end := regexp.MustCompile(`(?i)^\s*#\+end_` + m[1] + `\b`)
			for i+1 < len(lines) && !end.MatchString(lines[i+1]) {
				i++
			}
			i++
			continue
		}
		m := srcBeginRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		begin := i
		j := i + 1
		for j < len(lines) && !srcEndRe.MatchString(lines[j]) {
			j++
		}
		if j >= len(lines) {
			break
		}
		i = j
		if !IsLedgerLang(m[2]) {
			continue
		}
		indent := m[1]
		body := []string{}
		for _, l := range lines[begin+1 : j] {
			l = strings.TrimPrefix(l, indent)
			// Org escapes a line that would read as org with a comma.
			if strings.HasPrefix(l, ",*") || strings.HasPrefix(l, ",#+") {
				l = l[1:]
			}
			body = append(body, l)
		}
		name := ""
		for k := begin - 1; k >= 0; k-- {
			if nm := nameRe.FindStringSubmatch(lines[k]); nm != nil {
				name = nm[1]
				break
			}
			if !affiliatedRe.MatchString(lines[k]) {
				break
			}
		}
		out = append(out, OrgBlock{
			Index: len(out), Name: name, Lang: strings.ToLower(m[2]), Begin: begin + 1, End: j + 1,
			Header: strings.TrimSpace(m[3]), Text: strings.Join(body, "\n"),
		})
	}
	return out
}

// Chunks is the blocks as journal text that knows its org lines: each
// block's first line is the line after its #+BEGIN_SRC.
func Chunks(file string, blocks []OrgBlock) []Chunk {
	out := []Chunk{}
	for _, b := range blocks {
		out = append(out, Chunk{File: file, Line: b.Begin + 1, Text: b.Text})
	}
	return out
}
