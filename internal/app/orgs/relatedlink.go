package orgs

// POST /related/link: link a heading (or file) to one of its related notes,
// as a line edit. The target gets an :ID: when it has none (mentions.go's
// ensureHeadingId), and the source gets `- [[id:…][Title]]` in a `Related:`
// list at the end of its own text (before its first child), made when there
// is none:
//
//	** Draining nodes
//	We cordon each node first...
//
//	Related:
//	- [[id:5b1c…][Kubernetes upgrades]]

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/go-org/org"
)

var (
	relRelatedLineRe = regexp.MustCompile(`(?i)^(\s*)(?:related|related notes|see also):\s*$`)
	relListItemRe    = regexp.MustCompile(`^(\s*)[-+]\s`)
	relIndentRe      = regexp.MustCompile(`^\s*`)
)

// relEndpoint names a heading by hash, or a file with an optional line.
func relEndpoint(hash, file string, line *int) (*common.OrgFile, *org.Section, error) {
	db := GetDb()
	if h := strings.ReplaceAll(strings.TrimSpace(hash), " ", "+"); h != "" {
		sec := db.FindByHash(h)
		if sec == nil || sec.Headline == nil {
			return nil, nil, fmt.Errorf("no heading with that hash")
		}
		f := db.FileFromSection(sec)
		if f == nil {
			return nil, nil, fmt.Errorf("that heading has no file")
		}
		return f, sec, nil
	}
	fname, err := resolveRequestedFile(file)
	if err != nil {
		return nil, nil, err
	}
	f := db.FindByFile(fname)
	if f == nil || f.Doc == nil {
		return nil, nil, fmt.Errorf("file not found in the database: %s", fname)
	}
	if line != nil && *line >= 0 {
		return f, findHeadingByLine(f, *line), nil
	}
	return f, nil, nil
}

// relInsertLink writes item into the Related list of the region [from, end]
// (from -1: a file's preamble), making the list when there is none. Answers
// the new lines and the row written.
func relInsertLink(lines []string, from, end int, item string, target string) ([]string, int, error) {
	lo := from + 1
	if from < 0 {
		lo = 0
	}
	for i := lo; i <= end && i < len(lines); i++ {
		if strings.Contains(lines[i], "[["+target+"]") {
			return nil, i, fmt.Errorf("already linked")
		}
	}
	insert := func(at int, add []string) []string {
		out := make([]string, 0, len(lines)+len(add))
		out = append(out, lines[:at]...)
		out = append(out, add...)
		return append(out, lines[at:]...)
	}
	for i := lo; i <= end && i < len(lines); i++ {
		m := relRelatedLineRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		ind := m[1]
		j := i
		for j+1 <= end && j+1 < len(lines) && relListItemRe.MatchString(lines[j+1]) {
			j++
			ind = relListItemRe.FindStringSubmatch(lines[j])[1]
		}
		return insert(j+1, []string{ind + "- " + item}), j + 1, nil
	}
	// No list: one after the last line of text.
	p := end
	if p >= len(lines) {
		p = len(lines) - 1
	}
	for p >= lo && strings.TrimSpace(lines[p]) == "" {
		p--
	}
	if p < lo {
		// Nothing below the headline (or an empty preamble).
		at := lo
		ind := ""
		return insert(at, []string{ind + "Related:", ind + "- " + item}), at + 1, nil
	}
	ind := ""
	if !anyHeadRe.MatchString(lines[p]) {
		ind = relIndentRe.FindString(lines[p])
		if relListItemRe.MatchString(lines[p]) {
			ind = relListItemRe.FindStringSubmatch(lines[p])[1]
		}
	}
	return insert(p+1, []string{"", ind + "Related:", ind + "- " + item}), p + 3, nil
}

func RelatedLink(req common.RelatedLinkRequest) common.RelatedLinkResult {
	res := common.RelatedLinkResult{}
	sf, ssec, err := relEndpoint(req.Hash, req.Filename, req.Line)
	if err != nil {
		res.Msg = "the heading to link from: " + err.Error()
		return res
	}
	tf, tsec, err := relEndpoint(req.TargetHash, req.TargetFile, req.TargetLine)
	if err != nil {
		res.Msg = "the note to link to: " + err.Error()
		return res
	}
	if sf.Filename == tf.Filename && (ssec == tsec || (ssec != nil && tsec != nil && ssec.Hash == tsec.Hash)) {
		res.Msg = "that is the same heading"
		return res
	}
	srcHash := ""
	srcRow := -1
	if ssec != nil {
		srcHash, srcRow = ssec.Hash, ssec.Headline.Pos.Row
	}

	// How the link is written, giving the target an id if need be.
	var target, desc string
	if tsec != nil {
		id, minted, row, delta, err := ensureHeadingId(tf.Filename, tsec)
		if err != nil {
			res.Msg = err.Error()
			return res
		}
		res.Minted = minted
		target, desc = "id:"+id, common.GetHeadlineTitle(tsec.Headline)
		// The id drawer went in under the target's headline: a source further
		// down the same file has moved (Traps: re-measure).
		if delta != 0 && tf.Filename == sf.Filename && srcRow > row {
			srcRow += delta
		}
	} else {
		lines, _ := readOrgLines(tf.Filename)
		kw, props := filePreamble(lines)
		desc = kw["TITLE"]
		if desc == "" {
			desc = strings.TrimSuffix(fileBase(tf.Filename), ".org")
		}
		if props["ID"] != "" {
			target = "id:" + props["ID"]
		} else {
			target = "file:" + fileLinkPath(sf.Filename, tf.Filename)
		}
	}
	item := "[[" + target + "][" + linkDesc(desc) + "]]"
	res.Link = item

	// The source again, as the file now stands.
	db := GetDb()
	lines, err := readOrgLines(sf.Filename)
	if err != nil {
		res.Msg = err.Error()
		return res
	}
	from, end := -1, len(lines)-1
	if srcRow >= 0 {
		var sec *org.Section
		if srcHash != "" {
			sec = db.FindByHash(srcHash)
		}
		if sec == nil || sec.Headline == nil || sec.Headline.Pos.Row != srcRow {
			if f := db.FindByFile(sf.Filename); f != nil {
				sec = findHeadingByLine(f, srcRow)
			}
		}
		if sec == nil || sec.Headline == nil {
			res.Msg = "could not find the heading to link from again"
			return res
		}
		var to int
		var ok bool
		lines, from, to, ok = recordLines(sf.Filename, sec)
		if !ok {
			res.Msg = "could not read the heading to link from"
			return res
		}
		end = ownLinesEnd(lines, from, to)
	} else {
		for i, l := range lines {
			if anyHeadRe.MatchString(l) {
				end = i - 1
				break
			}
		}
	}
	out, row, err := relInsertLink(lines, from, end, item, target)
	res.Filename, res.Line = sf.Filename, row
	if err != nil {
		res.Ok, res.Msg = true, err.Error()
		return res
	}
	if err := writeLines(sf.Filename, out); err != nil {
		res.Msg = err.Error()
		return res
	}
	res.Ok = true
	res.Msg = "linked " + desc
	if res.Minted {
		res.Msg += " (gave it an :ID:)"
	}
	return res
}

func fileBase(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

/* SDOC: API
* POST /related/link — Link A Related Note

	#+BEGIN_EXAMPLE
	POST /related/link  {"Hash": "<the heading being read>", "TargetHash": "<a result's hash>"}
	#+END_EXAMPLE

	Writes =- [[id:…][Title]]= into a =Related:= list at the end of the
	source heading's own text (before its first child), making the list when
	there is none, and gives the target an =:ID:= first when it has none. A
	source or target may instead be a file (=Filename= / =TargetFile=), with
	=Line= / =TargetLine= naming the heading that zero-based line is in; a file
	target without an =ID= is linked =[[file:./x.org][Title]]=. Answers
	=Link=, =Filename=, =Line= (where it was written) and =Minted=. A link
	already in the list is not written twice (=Ok= with =Msg= "already linked").
EDOC */
func PostRelatedLink(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "related") {
		return
	}
	var req common.RelatedLinkRequest
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.RelatedLinkResult{Msg: err.Error()})
		return
	}
	dynJson(w, RelatedLink(req))
}
