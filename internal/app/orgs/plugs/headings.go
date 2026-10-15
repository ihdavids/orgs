package plugs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// Headings is what an exporter of headings (rather than of a document) is
// asked for: every heading of a file, when the query names one, and otherwise
// the headings the query expression finds, in the order it finds them.
//
// The todo.txt and calendar exporters take either, because both are as useful
// for "this project file" as for "everything due this month".
func Headings(db common.ODb, query string) ([]*org.Section, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("say which file, or which headings (a query)")
	}
	if f := db.FindByFile(query); f != nil {
		out := []*org.Section{}
		var walk func(s *org.Section)
		walk = func(s *org.Section) {
			if s.Headline != nil {
				out = append(out, s)
			}
			for _, c := range s.Children {
				walk(c)
			}
		}
		for _, s := range f.Outline.Children {
			walk(s)
		}
		return out, nil
	}
	tds, err := db.QueryTodosExpr(query)
	if err != nil {
		return nil, err
	}
	out := []*org.Section{}
	for _, td := range tds {
		if _, sec := db.GetFromTarget(&common.Target{Type: "hash", Id: td.Hash}, false); sec != nil && sec.Headline != nil {
			out = append(out, sec)
		}
	}
	return out, nil
}

// PlainText is org inline markup as a person would read it aloud: emphasis
// markers gone, a link as its description followed by where it goes (a
// description alone loses the url, and the url is usually the point), and
// timestamps as written.
func PlainText(nodes []org.Node) string {
	b := strings.Builder{}
	var walk func(ns []org.Node)
	walk = func(ns []org.Node) {
		for _, n := range ns {
			switch v := n.(type) {
			case org.Text:
				b.WriteString(v.Content)
			case org.Emphasis:
				walk(v.Content)
			case org.RegularLink:
				if v.Description != nil {
					walk(v.Description)
					if v.Protocol != "" && v.Protocol != "file" && v.Protocol != "id" {
						b.WriteString(" " + v.URL)
					}
				} else {
					b.WriteString(v.URL)
				}
			case org.LineBreak:
				b.WriteString(" ")
			case org.ExplicitLineBreak:
				b.WriteString(" ")
			case org.StatisticToken:
				// [2/5] says how far a checklist got, which reads as noise
				// anywhere but in org.
			default:
				b.WriteString(org.String(n))
			}
		}
	}
	walk(nodes)
	return strings.Join(strings.Fields(b.String()), " ")
}

// Title is a heading's title as plain text.
func Title(h *org.Headline) string {
	if h == nil {
		return ""
	}
	return PlainText(h.Title)
}

// WriteExportFile is how an exporter that makes text writes it for
// `export -local`: at the path given, its folder made if need be.
func WriteExportFile(to, text string) error {
	if strings.TrimSpace(to) == "" {
		return fmt.Errorf("no file to write to")
	}
	if dir := filepath.Dir(to); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(to, []byte(text), 0o644)
}
