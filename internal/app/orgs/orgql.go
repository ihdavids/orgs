package orgs

// org-ql's dynamic block (features.orgQl): #+BEGIN: org-ql :query ...
// :columns ... :sort ... :take N, written as Emacs writes it, so a file with
// these blocks refreshes the same in both. The query is translated into
// orgs' full form by common.OrgQlToQuery and answered by the query engine.
// Guide: "Query blocks" in docs/org-packages.org.

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

func init() {
	dynGenerators["org-ql"] = dynOrgQl
}

// One heading an org-ql block found, with what its columns and sorts read.
type orgQlHit struct {
	file  *common.OrgFile
	sec   *org.Section
	props map[string]string
	todo  int // the keyword's place in its file's sequence, -1 for none
}

func dynOrgQl(c *dynCtx) ([]string, error) {
	if !featureOn("orgQl") {
		return nil, fmt.Errorf("org-ql blocks are turned off on this server: turn them on with `orgs features orgQl on` or in worg's settings")
	}
	qlText := strings.TrimSpace(c.param("query", c.param("q", "")))
	if qlText == "" || qlText == "t" {
		return nil, fmt.Errorf(`an org-ql block needs :query "the query"`)
	}
	full, err := common.OrgQlToQuery(qlText, c.Now)
	if err != nil {
		return nil, fmt.Errorf("org-ql: %v", err)
	}
	exp, err := ParseString(&common.StringQuery{Query: full})
	if err != nil {
		return nil, fmt.Errorf("org-ql: %s became %s, which does not parse: %v", qlText, full, err)
	}
	cols, err := common.OrgQlColumns(c.param("columns", "(heading todo)"))
	if err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf(":columns names no columns")
	}
	sorts, err := common.OrgQlWords(c.param("sort", ""))
	if err != nil {
		return nil, fmt.Errorf(":sort: %v", err)
	}
	take := 0
	if v := c.param("take", ""); v != "" && v != "nil" {
		if take, err = strconv.Atoi(v); err != nil {
			return nil, fmt.Errorf(":take is a number of rows, not %q", v)
		}
	}
	tsFormat := strings.Trim(c.param("ts-format", "%Y-%m-%d"), `"`)

	files := []string{}
	if c.param("scope", "") == "file" {
		files = append(files, c.File.Doc.Path)
	} else {
		registerAllSections()
		files, _ = filesForQuery(full)
		sort.Strings(files)
	}
	keys := &agendaKeys{}
	hits := []orgQlHit{}
	for _, name := range files {
		f := GetDb().GetFile(name)
		if f == nil || f.Doc == nil {
			continue
		}
		var found []*org.Section
		for _, v := range f.Doc.Outline.Children {
			found, _ = EvalForNodes(exp, v, f, found)
		}
		sort.SliceStable(found, func(a, b int) bool { return headlineRow(found[a]) < headlineRow(found[b]) })
		lines := fileLines(f.Doc.Path)
		active, done := keys.of(f)
		seq := append(append([]string{}, active...), done...)
		for _, s := range found {
			if s.Headline == nil || orgQlInBlock(c, f, s) {
				continue
			}
			h := orgQlHit{file: f, sec: s, props: propsOfSection(lines, s), todo: -1}
			for i, k := range seq {
				if k == s.Headline.Status {
					h.todo = i
				}
			}
			hits = append(hits, h)
		}
	}
	if err := orgQlSort(hits, sorts); err != nil {
		return nil, err
	}
	switch {
	case take > 0 && take < len(hits):
		hits = hits[:take]
	case take < 0 && -take < len(hits):
		hits = hits[len(hits)+take:]
	}

	header := []string{}
	for _, col := range cols {
		header = append(header, col.Title)
	}
	table := [][]string{header, nil}
	for _, h := range hits {
		row := []string{}
		for _, col := range cols {
			row = append(row, cellText(orgQlCell(c, h, col.Name, tsFormat)))
		}
		table = append(table, row)
	}
	return orgTableLines(table), nil
}

// The block's own heading is not a result of it: a query for open tasks
// written under a TODO heading would list the heading it is in.
func orgQlInBlock(c *dynCtx, f *common.OrgFile, s *org.Section) bool {
	return c.Owner != nil && s == c.Owner && f == c.File
}

func orgQlCell(c *dynCtx, h orgQlHit, col string, tsFormat string) string {
	hl := h.sec.Headline
	date := func(d *org.SDC) string {
		if d == nil || d.Date == nil {
			return ""
		}
		f := tsFormat
		if d.Date.HaveTime && f == "%Y-%m-%d" {
			f += " %H:%M"
		}
		return strftime(f, d.Date.Start)
	}
	switch strings.ToLower(col) {
	case "heading", "item":
		title := strings.TrimSpace(common.GetHeadlineTitle(hl))
		for k, v := range h.props {
			if strings.EqualFold(k, "ID") && strings.TrimSpace(v) != "" {
				desc := strings.NewReplacer("[", "{", "]", "}").Replace(title)
				return "[[id:" + strings.TrimSpace(v) + "][" + desc + "]]"
			}
		}
		return headingLink(c.File.Doc.Path, h.file.Doc.Path, title)
	case "todo":
		return hl.Status
	case "priority":
		return hl.Priority
	case "deadline":
		return date(hl.Deadline)
	case "scheduled":
		return date(hl.Scheduled)
	case "closed":
		return date(hl.Closed)
	case "tags":
		if len(hl.Tags) == 0 {
			return ""
		}
		return ":" + strings.Join(hl.Tags, ":") + ":"
	case "file":
		return relativeName(c.File.Doc.Path, h.file.Doc.Path)
	}
	for k, v := range h.props {
		if strings.EqualFold(k, col) {
			return v
		}
	}
	return ""
}

// orgQlSort orders hits by each sort in turn, the first deciding: dates
// earliest first and missing last, priority A first and none last, keywords
// in their file's order, titles alphabetically. random shuffles.
func orgQlSort(hits []orgQlHit, sorts []string) error {
	at := func(d *org.SDC) (time.Time, bool) {
		if d == nil || d.Date == nil {
			return time.Time{}, false
		}
		return d.Date.Start, true
	}
	dateOf := func(h orgQlHit, which string) (time.Time, bool) {
		hl := h.sec.Headline
		switch which {
		case "deadline":
			return at(hl.Deadline)
		case "scheduled":
			return at(hl.Scheduled)
		case "closed":
			return at(hl.Closed)
		}
		if t, ok := at(hl.Deadline); ok {
			return t, true
		}
		return at(hl.Scheduled)
	}
	cmps := []func(a, b orgQlHit) int{}
	for _, s := range sorts {
		switch s {
		case "date", "deadline", "scheduled", "closed":
			which := s
			cmps = append(cmps, func(a, b orgQlHit) int {
				ta, oka := dateOf(a, which)
				tb, okb := dateOf(b, which)
				switch {
				case oka && okb:
					return ta.Compare(tb)
				case oka:
					return -1
				case okb:
					return 1
				}
				return 0
			})
		case "priority":
			cmps = append(cmps, func(a, b orgQlHit) int {
				pa, pb := a.sec.Headline.Priority, b.sec.Headline.Priority
				switch {
				case pa == pb:
					return 0
				case pa == "":
					return 1
				case pb == "":
					return -1
				}
				return strings.Compare(pa, pb)
			})
		case "todo":
			cmps = append(cmps, func(a, b orgQlHit) int {
				ta, tb := a.todo, b.todo
				if ta < 0 {
					ta = 1 << 20
				}
				if tb < 0 {
					tb = 1 << 20
				}
				return ta - tb
			})
		case "heading":
			cmps = append(cmps, func(a, b orgQlHit) int {
				return strings.Compare(strings.ToLower(common.GetHeadlineTitle(a.sec.Headline)), strings.ToLower(common.GetHeadlineTitle(b.sec.Headline)))
			})
		case "random":
			rand.Shuffle(len(hits), func(i, j int) { hits[i], hits[j] = hits[j], hits[i] })
		case "reverse":
			// org-ql's (reverse): the order so far, turned round.
			defer func() {
				for i, j := 0, len(hits)-1; i < j; i, j = i+1, j-1 {
					hits[i], hits[j] = hits[j], hits[i]
				}
			}()
		default:
			return fmt.Errorf(":sort: no sort called %q (date, deadline, scheduled, closed, priority, todo, heading, random, reverse)", s)
		}
	}
	if len(cmps) == 0 {
		return nil
	}
	sort.SliceStable(hits, func(i, j int) bool {
		for _, c := range cmps {
			if r := c(hits[i], hits[j]); r != 0 {
				return r < 0
			}
		}
		return false
	})
	return nil
}
