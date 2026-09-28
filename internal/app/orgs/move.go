//lint:file-ignore ST1006 allow the use of self
package orgs

// Refile, copy and archive - one heading or twenty, in one request.
//
// The whole reason this is not a loop in the client is the hash. A heading's
// hash is accumulated from the document name and the chain of headline titles
// the parser has walked (see `go-org/org/headline.go`), so it is stable while
// the file's *structure* is, and moving a heading out of a file changes the
// hash of every heading after it. Twenty hashes collected from one search and
// posted one at a time: the first works, and the rest address headings that are
// not there any more - or headings that have since inherited those hashes.
//
// So this resolves every source before it writes anything, and then addresses
// each one by its **outline path** rather than by its hash. An olp survives a
// sibling being moved out from under it, which a hash does not.

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// A source, as it was before anything moved.
type moveSource struct {
	filename string
	olp      []string
	headline string
}

// Move is refile, copy and archive. `Op` picks which.
func Move(db common.ODb, req *common.MoveRequest) (common.MoveResponse, error) {
	res := common.MoveResponse{Results: []common.MoveResult{}}

	op := strings.ToLower(strings.TrimSpace(req.Op))
	switch op {
	case common.MoveRefile, common.MoveCopy, common.MoveArchive:
	case "":
		op = common.MoveRefile
	default:
		res.Msg = fmt.Sprintf("there is no operation called %q", req.Op)
		return res, nil
	}
	if len(req.From) == 0 {
		res.Msg = "nothing to move"
		return res, nil
	}

	// ---- resolve everything first, while the hashes still mean something
	sources := []moveSource{}
	for i := range req.From {
		tgt := req.From[i]
		file, sec := db.GetFromTarget(&tgt, false)
		if file == nil || sec == nil {
			res.Results = append(res.Results, common.MoveResult{
				Headline: describeTarget(&tgt),
				Ok:       false,
				Msg:      "could not find that heading",
			})
			res.Failed++
			continue
		}
		sources = append(sources, moveSource{
			filename: file.Filename,
			olp:      outlineOf(sec),
			headline: common.GetSectionTitle(sec),
		})
	}
	if len(sources) == 0 {
		res.Msg = "none of those headings could be found"
		return res, nil
	}

	// ---- an ancestor takes its descendants with it
	//
	// Selecting a project and one of its tasks and refiling both would move the
	// project, and then try to move the task out of the project that has just
	// moved. Which is at best a no-op and at worst finds a different heading of
	// the same name somewhere else.
	keep := []moveSource{}
	for i, s := range sources {
		if under := ancestorOf(sources, i); under >= 0 {
			res.Results = append(res.Results, common.MoveResult{
				Headline: s.headline,
				Filename: s.filename,
				Ok:       true,
				Skipped:  true,
				Msg:      fmt.Sprintf("goes with %q", sources[under].headline),
			})
			res.Skipped++
			continue
		}
		keep = append(keep, s)
	}

	// ---- and now do them, one at a time, each addressed by its outline path
	//
	// **Every file touched is re-read before the next one goes.** A refile
	// writes the file and nothing re-parses it until the watcher gets round to
	// it, so a second operation started immediately resolves its target against
	// the document as it was *before* the first - and then writes at row numbers
	// that moved underneath it. That is not a failed refile, it is a corrupted
	// file: the first batch this was tested on reported four successes and left
	// one heading behind, the destination heading gone, and three headings
	// nowhere at all.
	//
	// `ReloadFile` is the synchronous re-parse the dnd endpoints already use
	// after each of their writes, and for the same reason.
	for _, s := range keep {
		from := common.Target{Type: "file+olp", Filename: s.filename, Id: olpString(s.olp)}
		var one common.ResultMsg
		var err error

		// Worked out before the operation, because afterwards the destination
		// may be a heading that has only just been created.
		touched := map[string]bool{s.filename: true}

		switch op {
		case common.MoveArchive:
			// Each heading may archive somewhere different - the target comes
			// from its own `:ARCHIVE:` property before the file's and the
			// server's - so it is asked per heading rather than once.
			if at := FindArchiveTarget(db, &from); at != nil && at.Filename != "" {
				touched[at.Filename] = true
			}
			one, err = Archive(db, &from)
		case common.MoveCopy:
			touched[destFile(db, &req.To)] = true
			one, err = Copy(db, &common.Refile{FromId: from, ToId: req.To}, req.Create)
		default:
			touched[destFile(db, &req.To)] = true
			one, err = Refile(db, &common.Refile{FromId: from, ToId: req.To}, nil, req.Create)
		}
		for f := range touched {
			if f != "" {
				GetDb().ReloadFile(f)
			}
		}

		r := common.MoveResult{Headline: s.headline, Filename: s.filename, Ok: one.Ok, Msg: one.Msg}
		if err != nil {
			r.Ok = false
			r.Msg = err.Error()
		}
		if r.Ok {
			res.Done++
			// A refile's "Delete successful" is about the mechanism rather than
			// about the heading, and reads as an alarm in a list of results.
			r.Msg = ""
		} else {
			res.Failed++
		}
		res.Results = append(res.Results, r)
	}

	res.Ok = res.Failed == 0 && res.Done > 0
	switch {
	case res.Failed == 0 && res.Done > 0:
		res.Msg = fmt.Sprintf("%s %d heading%s", pastTense(op), res.Done, plural(res.Done))
	case res.Done > 0:
		res.Msg = fmt.Sprintf("%s %d, %d could not be", pastTense(op), res.Done, res.Failed)
	default:
		res.Msg = "nothing moved"
	}
	return res, nil
}

// The file a destination target lands in, or "" when it cannot be resolved -
// in which case the operation is about to fail anyway and there is nothing to
// re-read.
func destFile(db common.ODb, to *common.Target) string {
	if to.Filename != "" {
		return to.Filename
	}
	if file, _ := db.GetFromTarget(to, false); file != nil {
		return file.Filename
	}
	return ""
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func pastTense(op string) string {
	switch op {
	case common.MoveCopy:
		return "copied"
	case common.MoveArchive:
		return "archived"
	}
	return "refiled"
}

// Which other source, if any, this one lives under. -1 when it stands alone.
func ancestorOf(all []moveSource, i int) int {
	me := all[i]
	for j, other := range all {
		if j == i || other.filename != me.filename {
			continue
		}
		if len(other.olp) >= len(me.olp) {
			continue
		}
		isUnder := true
		for k, name := range other.olp {
			if me.olp[k] != name {
				isUnder = false
				break
			}
		}
		if isUnder {
			return j
		}
	}
	return -1
}

// The outline path of a section, outermost first.
//
// The root of a document is a section too, with a headline that has no title,
// and walking up to it naively puts an empty first element on every path - so
// `file+olp` looks for a heading called "" and finds nothing. `BuildOutlinePath`
// already knows to drop it; this is the same walk kept as a slice, because the
// nesting check below needs the parts rather than the joined string.
//
// The two must agree, and `olpString` is what makes sure they do: the target is
// addressed with the join of this, so a difference between them would be a
// refile that resolves to the wrong heading rather than to none.
func outlineOf(sec *org.Section) []string {
	out := []string{}
	for s := sec; s != nil && s.Headline != nil; s = s.Parent {
		title := common.GetSectionTitle(s)
		if title == "" && s.Parent == nil {
			// The document root, which is not part of anybody's path.
			break
		}
		out = append([]string{title}, out...)
	}
	return out
}

func olpString(olp []string) string { return strings.Join(olp, "::") }

// Enough of a target to name it in a failure, for the case where it could not
// be resolved and so has no headline to report.
func describeTarget(t *common.Target) string {
	if t.Id != "" {
		return t.Id
	}
	if t.Filename != "" {
		return t.Filename
	}
	return "(unnamed target)"
}

// ---------------------------------------------------------------------------
// Where a heading can go
// ---------------------------------------------------------------------------

// The most targets to walk before giving up. A database of any size has more
// headings than anybody will scroll past, and the client filters as you type -
// but a list that stops early has to say so rather than quietly being short.
const maxRefileTargets = 4000

// RefileTargets answers with every place a heading could be moved to.
//
// It is the structured sibling of `/refilefiles`, which answers the same
// question as `"file|H1|H2"` strings. Three things that form cannot do:
//
//   - **Say which heading.** Two headings called "Notes" in one file are the
//     same string, and a refile addressed by `file+headline` finds whichever
//     comes first. A target here carries its hash and its outline path.
//   - **Say what kind of place it is.** What tells two candidates apart is
//     usually the keyword or the tags on them rather than the name, and the
//     flat string has nowhere to put either.
//   - **Go deeper than three levels.** The old walk hardcodes it; this takes a
//     depth.
//
// Both default to the `refileTargets` setting, which itself defaults to every
// org file - so neither is empty on a fresh server. `files` overrides it, for a
// client that wants to offer one file's headings.
func RefileTargets(files []string, depth int) common.RefileTargetList {
	out := common.RefileTargetList{Ok: true, Targets: []common.RefileTarget{}}
	if depth <= 0 {
		depth = 3
	}

	wanted := files
	if len(wanted) == 0 {
		wanted = Conf().Server.RefileTargets
	}
	matching := matchingFiles(wanted)
	out.Files = len(matching)

	for _, fname := range matching {
		ofile := GetDb().GetFile(fname)
		if ofile == nil || ofile.Doc == nil {
			continue
		}
		// The file itself: refiling to the end of a file is a real thing to
		// want and the flat endpoint could say it only by accident.
		out.Targets = append(out.Targets, common.RefileTarget{Filename: fname})
		for _, c := range ofile.Doc.Outline.Children {
			if collectTargets(ofile, c, nil, depth, &out) {
				out.Truncated = true
				return out
			}
		}
	}
	return out
}

// Walk one heading and its children. Answers true when the cap was reached.
func collectTargets(ofile *common.OrgFile, sec *org.Section, olp []string, depth int, out *common.RefileTargetList) bool {
	if sec == nil || sec.Headline == nil {
		return false
	}
	// Registered on the way past for the reason everything that walks sections
	// has to: they go into the db's hash maps lazily, as queries touch them, so
	// a hash handed out by a walk of its own belongs to a heading nothing can
	// look up afterwards.
	GetDb().RegisterSection(sec.Hash, sec, ofile)

	title := common.GetSectionTitle(sec)
	path := append(append([]string{}, olp...), title)

	out.Targets = append(out.Targets, common.RefileTarget{
		Hash:     sec.Hash,
		Filename: ofile.Filename,
		Olp:      path,
		Level:    sec.Headline.Lvl,
		Todo:     sec.Headline.Status,
		Tags:     sec.Headline.Tags,
	})
	if len(out.Targets) >= maxRefileTargets {
		return true
	}
	if sec.Headline.Lvl >= depth {
		return false
	}
	for _, c := range sec.Children {
		if collectTargets(ofile, c, path, depth, out) {
			return true
		}
	}
	return false
}

// The files a list of patterns names, or every file when the list is empty.
func matchingFiles(patterns []string) []string {
	all := GetDb().GetFiles()
	if len(patterns) == 0 {
		return all
	}
	out := []string{}
	for _, file := range all {
		for _, m := range patterns {
			if m == file {
				out = append(out, file)
				break
			}
			if ok, err := regexp.MatchString(m, file); err == nil && ok {
				out = append(out, file)
				break
			}
		}
	}
	return out
}
