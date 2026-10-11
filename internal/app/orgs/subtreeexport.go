package orgs

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// Exporting one subtree rather than the whole file (org's C-c C-e C-s). Every
// exporter reads its file through the database, so the subtree is handed to
// them as the file: its #+ settings, a #+TITLE: from the heading - as org's
// subtree export titles it - and the subtree's own lines. No exporter needs to
// know.

// A database that reads one file as something else.
type subtreeDb struct {
	common.ODb
	file *common.OrgFile
}

func (s *subtreeDb) is(filename string) bool {
	f := GetDb().FindByFile(filename)
	return f != nil && f.Filename == s.file.Filename
}

func (s *subtreeDb) FindByFile(filename string) *org.Document {
	if s.is(filename) {
		return s.file.Doc
	}
	return s.ODb.FindByFile(filename)
}

func (s *subtreeDb) GetFile(filename string) *common.OrgFile {
	if s.is(filename) {
		return s.file
	}
	return s.ODb.GetFile(filename)
}

var subtreeHeadingRe = regexp.MustCompile(`^(\*+)\s`)
var subtreeTitleRe = regexp.MustCompile(`(?i)^\s*#\+TITLE:`)

func findSectionByHash(secs []*org.Section, hash string) *org.Section {
	for _, s := range secs {
		if s.Hash == hash {
			return s
		}
		if f := findSectionByHash(s.Children, hash); f != nil {
			return f
		}
	}
	return nil
}

// The database, with `query`'s file reading as just the subtree `hash` starts.
func subtreeView(base common.ODb, query, hash string) (common.ODb, error) {
	f := GetDb().FindByFile(query)
	if f == nil || f.Doc == nil {
		return nil, fmt.Errorf("no org file called %s", query)
	}
	sec := findSectionByHash(f.Doc.Outline.Children, hash)
	if sec == nil || sec.Headline == nil {
		return nil, fmt.Errorf("no heading with that hash in %s - it may have moved; save and try again", query)
	}
	lines, err := readFileLines(f.Filename)
	if err != nil {
		return nil, err
	}
	start := sec.Headline.Pos.Row
	if start < 0 || start >= len(lines) {
		return nil, fmt.Errorf("the heading is not where the file has it")
	}
	end := start + 1
	for end < len(lines) {
		if m := subtreeHeadingRe.FindStringSubmatch(lines[end]); m != nil && len(m[1]) <= sec.Headline.Lvl {
			break
		}
		end++
	}
	// The file's settings come along: #+OPTIONS:, #+HTML_HEAD: and the rest.
	preamble := []string{}
	for _, l := range lines {
		if subtreeHeadingRe.MatchString(l) {
			break
		}
		if strings.HasPrefix(strings.TrimSpace(l), "#+") && !subtreeTitleRe.MatchString(l) {
			preamble = append(preamble, l)
		}
	}
	title := ""
	for _, n := range sec.Headline.Title {
		title += n.String()
	}
	text := "#+TITLE: " + strings.TrimSpace(title) + "\n" + strings.Join(preamble, "\n") + "\n" + strings.Join(lines[start:end], "\n") + "\n"
	doc := org.New().Silent().Parse(strings.NewReader(text), f.Filename)
	return &subtreeDb{ODb: base, file: &common.OrgFile{Filename: f.Filename, Doc: doc}}, nil
}
