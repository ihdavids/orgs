//lint:file-ignore ST1006 allow the use of self
package orgs

/* SDOC: Editing
* Refile

  TODO: Fill in information on orgs server refiling
EDOC */

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

func CopySection(toCopy *org.Section) *org.Section {
	s := new(org.Section)
	*s = *toCopy
	var children []*org.Section
	for _, c := range s.Children {
		children = append(children, CopySection(c))
	}
	s.Children = children
	return s
}

func fixUpLevel(s *org.Section, lvl int) {
	s.Headline.Lvl = lvl
	for _, c := range s.Children {
		fixUpLevel(c, lvl+1)
	}
}

// Write one heading and everything under it.
//
// The recursion over `sec.Children` that used to be here wrote the subtree a
// second time. `WriteHeadline` ends with `WriteNodesLB(1, w, h.Children...)`
// and a headline's children include the headlines nested inside it, so the
// whole subtree is already on the page by the time it returns.
//
// It was not a duplicate but a doubling *per level*: refiling a heading with a
// child and a grandchild wrote the child twice and the grandchild three times.
// Every refile of anything deeper than one level had been quietly corrupting
// the subtree, which is the sort of thing only noticed once a batch makes it
// happen to five headings at once.
func formatHeading(w *org.OrgWriter, sec *org.Section) {
	org.WriteNodes(w, *sec.Headline)
}

func formatHeadingAt(dest *org.Section, src *org.Section) string {
	res := ""
	// TODO: I need to copy the entire data structure
	lvl := dest.Headline.Lvl
	srcCpy := CopySection(src)
	fixUpLevel(srcCpy, lvl+1)

	w := org.NewOrgWriter()
	formatHeading(w, srcCpy)
	res += w.String()
	return res
}

func InsertSection(to *common.OrgFile, toInsert *org.Section, destination *org.Section, res *common.ResultMsg) {
	fmt.Fprintf(os.Stderr, "  [InsertSection]\n")
	if r, err := os.Open(to.Doc.Path); err == nil {
		defer r.Close()
		// Split the file into lines of text
		var lines []string
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		endLine := len(lines) - 1
		// We had a problem with the scanner?
		if err := scanner.Err(); err != nil {
			res.Msg = "Insert: failed to open file " + err.Error()
		} else {
			p := findInsertPos(destination)
			row := p.Row
			fileContent := ""
			var emptyLines []string = []string{}
			didAdd := false
			for i, line := range lines {

				if i == row+1 {
					fileContent += formatHeadingAt(destination, toInsert)
					didAdd = true
				}

				if isEmpty(line) {
					emptyLines = append(emptyLines, line)
				} else {
					for _, emptyLine := range emptyLines {
						fileContent += emptyLine
						fileContent += "\n"
					}
					emptyLines = []string{}
					fileContent += line
					fileContent += "\n"
				}

				// Last line of file has to be added after
				if !didAdd && p != nil && p.Row == endLine && (i == p.Row) {
					fileContent += formatHeadingAt(destination, toInsert)
				}
			}
			fmt.Fprintf(os.Stderr, "Writing FILE: %v\n", to.Doc.Path)
			os.WriteFile(to.Doc.Path, []byte(fileContent), 0644)
			res.Ok = true
			res.Msg = "Insert successful"
		}
	}
}

// The last row of a heading's subtree: everything up to the next heading at the
// same level or above, or the end of the file.
//
// This exists because `Headline.GetEnd()` under-reports for a heading whose body
// is only a planning line and a property drawer - go-org keeps the drawer in
// `Headline.Properties` rather than among the body nodes it measures, so the end
// lands on the SCHEDULED line and a delete leaves the drawer behind, orphaned
// under the parent. A heading with any other body, or with children, measures
// correctly, which is why this only ever *extends* the range it is given and
// never shrinks it.
func subtreeEndRow(lines []string, startRow int, lvl int, atLeast int) int {
	end := len(lines) - 1
	for i := startRow + 1; i < len(lines); i++ {
		line := lines[i]
		stars := 0
		for stars < len(line) && line[stars] == '*' {
			stars++
		}
		// A heading is stars followed by a space; `**bold**` at column zero is not one.
		if stars > 0 && stars <= lvl && stars < len(line) && line[stars] == ' ' {
			end = i - 1
			break
		}
	}
	if end < atLeast {
		return atLeast
	}
	return end
}

func DeleteTree(filename string, sec *org.Section, res *common.ResultMsg) {
	fmt.Fprintf(os.Stderr, "[DeleteEntry]\n")
	if r, err := os.Open(filename); err == nil {
		defer r.Close()
		// Split the file into lines of text
		var lines []string
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		// We had a problem with the scanner?
		if err := scanner.Err(); err != nil {
			res.Msg = "Delete: failed to open file " + err.Error()
		} else {
			s, e := findDeletePos(sec)
			end := subtreeEndRow(lines, s.Row, sec.Headline.Lvl, e.Row)
			fileContent := ""
			// Now iterate over the file and insert our content where it should go!
			for i, line := range lines {

				if i >= s.Row && i <= end {
					continue
				}
				fileContent += line
				fileContent += "\n"
			}
			fmt.Fprintf(os.Stderr, "Writing FILE: %v\n", filename)
			os.WriteFile(filename, []byte(fileContent), 0644)
			res.Ok = true
			res.Msg = "Delete successful"
		}
	}
}

type ModifySourceFunc func(ofile *common.OrgFile, sec *org.Section) *org.Section

func Refile(db common.ODb, args *common.Refile, mod ModifySourceFunc, allowCreate bool) (common.ResultMsg, error) {
	var res common.ResultMsg = common.ResultMsg{}
	res.Ok = false
	res.Msg = "Refile: unknown failure, did not refile"
	fromFile, fromSecs := db.GetFromTarget(&args.FromId, false)
	if fromFile == nil || fromSecs == nil {
		res.Msg = fmt.Sprintf("Refile: could not find source target [%s]", args.FromId.Type)
		res.Ok = false
		fmt.Fprintf(os.Stderr, ">>> ERROR REFILE FROM NOT FOUND %s\n", res.Msg)
		return res, nil
	}
	toFile, toSecs := db.GetFromTarget(&args.ToId, allowCreate)
	if toFile == nil || toSecs == nil {
		res.Msg = fmt.Sprintf("Refile: could not find destination target [%s]", args.ToId.Type)
		res.Ok = false
		fmt.Fprintf(os.Stderr, ">>> ERROR REFILE TO NOT FOUND %s\n", res.Msg)
		return res, nil
	}
	// Where the source is, said in a way that survives the file being
	// rewritten. Taken now, because this is the last moment the parse tree and
	// the file on disk agree with each other - see the note on the re-resolve
	// below.
	srcFile := fromFile.Filename
	srcOlp := outlineOf(fromSecs)

	if mod != nil {
		fromSecs = mod(fromFile, fromSecs)
	}
	InsertSection(toFile, fromSecs, toSecs, &res)
	if !res.Ok {
		return res, nil
	}

	// Find the source again, in the file as it now stands, before deleting it.
	//
	// `fromSecs` cannot be used for the delete, and had been used for it since
	// the beginning. Two things go wrong with it, and they compound:
	//
	//  1. **Its rows are stale.** `DeleteTree` re-reads the file, which
	//     `InsertSection` has just rewritten, but measures the subtree from
	//     rows taken off the parse tree beforehand. Insert above the source in
	//     the same file - which is what "refile this up to the Inbox" is - and
	//     every row below the insertion point has moved down by the length of
	//     what was written.
	//  2. **Its level has been changed underneath it.** `formatHeadingAt`
	//     copies the section and calls `fixUpLevel` to put the copy at its new
	//     depth, but `CopySection` shares the `*org.Headline` with the original
	//     (it has to: the headlines nested in `Headline.Children` are those
	//     same pointers, and renumbering the subtree for writing depends on
	//     it). So after the insert the source section claims the
	//     *destination's* depth, and `subtreeEndRow` - which walks forward to
	//     the next heading at that level or above - runs straight through the
	//     source's own siblings.
	//
	// Together they deleted whole neighbouring subtrees. Refiling `Alpha` out
	// of `Projects/Kitchen` into `Inbox` took `Kitchen`, `Gamma` and
	// `Gamma child` with it, reported success, and left nothing to say so.
	//
	// Re-reading the file and finding the heading again by its outline path
	// answers both at once: a fresh parse has the real rows and the real level,
	// and an outline path - unlike a hash - still names the same heading after
	// the file has been written to.
	GetDb().ReloadFile(srcFile)
	found := common.Target{Type: "file+olp", Filename: srcFile, Id: olpString(srcOlp)}
	delFile, delSecs := db.GetFromTarget(&found, false)
	if delFile == nil || delSecs == nil {
		res.Ok = false
		res.Msg = fmt.Sprintf("Refile: wrote the heading to its destination but could not find [%s] again to remove it; it is now in both places", olpString(srcOlp))
		return res, nil
	}
	DeleteTree(delFile.Doc.Path, delSecs, &res)
	return res, nil
}

// Copy is Refile without the delete: the heading is written at the destination
// and left where it was.
//
// Two things it does that a refile does not, both about **identity**:
//
//   - It copies the section before inserting, because `InsertSection` rewrites
//     the levels of what it is given (`fixUpLevel`) and doing that to the live
//     parse tree would renumber the original in memory as a side effect of
//     copying it.
//   - It takes the `ID` and `CUSTOM_ID` off the copy. Those are how an
//     `[[id:...]]` link finds a heading, and two headings answering to one id
//     is not a duplicate - it is a link that now points at whichever of them
//     the database happened to register last. A copy is a new thing and can be
//     given a new id; silently minting an ambiguity is the one outcome nobody
//     would choose.
func Copy(db common.ODb, args *common.Refile, allowCreate bool) (common.ResultMsg, error) {
	var res common.ResultMsg = common.ResultMsg{}
	res.Ok = false
	res.Msg = "Copy: unknown failure, did not copy"
	fromFile, fromSecs := db.GetFromTarget(&args.FromId, false)
	if fromFile == nil || fromSecs == nil {
		res.Msg = fmt.Sprintf("Copy: could not find source target [%s]", args.FromId.Type)
		return res, nil
	}
	toFile, toSecs := db.GetFromTarget(&args.ToId, allowCreate)
	if toFile == nil || toSecs == nil {
		res.Msg = fmt.Sprintf("Copy: could not find destination target [%s]", args.ToId.Type)
		return res, nil
	}
	dup := CopySection(fromSecs)
	stripIds(dup)
	InsertSection(toFile, dup, toSecs, &res)
	if res.Ok {
		res.Msg = "Copy successful"
	}
	// `CopySection` copies the sections and *shares* their headlines, so the
	// level fixup `InsertSection` does to place the copy at its new depth has
	// renumbered the original in memory as well. A refile gets away with that
	// because the source is deleted and the file re-read; a copy leaves the
	// source where it is, so the file on disk is right and the parse tree is
	// not. Re-reading it is the cheapest way to make them agree again.
	GetDb().ReloadFile(fromFile.Filename)
	return res, nil
}

// Take the ids off a subtree about to be written down a second time.
//
// The property drawer is shared with the original by `CopySection` - it copies
// the struct, and the drawer is a pointer - so the drawer is replaced rather
// than edited, or taking the id off the copy would take it off the heading
// being copied.
func stripIds(sec *org.Section) {
	if sec == nil {
		return
	}
	if sec.Headline != nil && sec.Headline.Properties != nil {
		kept := &org.PropertyDrawer{}
		for _, p := range sec.Headline.Properties.Properties {
			if len(p) > 0 {
				name := strings.ToUpper(strings.TrimSpace(p[0]))
				if name == "ID" || name == "CUSTOM_ID" {
					continue
				}
			}
			kept.Properties = append(kept.Properties, p)
		}
		if len(kept.Properties) == 0 {
			sec.Headline.Properties = nil
		} else {
			sec.Headline.Properties = kept
		}
	}
	for _, c := range sec.Children {
		stripIds(c)
	}
}

func Delete(db common.ODb, tgt *common.Target) (common.ResultMsg, error) {
	var res common.ResultMsg = common.ResultMsg{}
	res.Ok = false
	res.Msg = "Delete: unknown failure, did not delete"
	file, secs := db.GetFromTarget(tgt, false)
	if file == nil || secs == nil {
		res.Msg = fmt.Sprintf("Delete: could not find target [%s]", tgt.Type)
		res.Ok = false
		return res, nil
	}
	DeleteTree(file.Doc.Path, secs, &res)
	return res, nil
}

//////////////////////////////////////////////////////////////////
// Query a list of potential targets using a list of filename regexs
// This uses the RefileTargets configuration parameter
//
// FORMAT: <filename>|Heading1|Heading2|Heading3
//////////////////////////////////////////////////////////////////

func getTargetHeadings(ofile *common.OrgFile, path string, sec *org.Section, results []string, depth int) []string {
	// We have to do this to make sure this section is known in our by hash lookup
	odb.RegisterSection(sec.Hash, sec, ofile)
	if sec.Headline.Lvl > depth {
		return results
	}
	var title string
	for _, n := range sec.Headline.Title {
		title += n.String()
	}
	tpath := path + "|" + title
	results = append(results, tpath)
	for _, c := range sec.Children {
		results = getTargetHeadings(ofile, tpath, c, results, depth)
	}
	return results
}

func GetRefileTargetsList(requestedTargets []string) []string {
	if len(requestedTargets) == 0 {
		requestedTargets = Conf().Server.RefileTargets
	}
	if len(requestedTargets) == 0 {
		return []string{}
	}
	matchingFiles := []string{}
	for _, file := range odb.GetFiles() {
		for _, m := range requestedTargets {
			// Check for files matching our regex target
			if res, err := regexp.Match(m, []byte(file)); m == file || (err == nil && res) {
				matchingFiles = append(matchingFiles, file)
				break
			}
		}
	}
	depth := 3
	results := []string{}
	// Once we have our files list we need our headings list
	for _, file := range matchingFiles {

		ofile := odb.GetFile(file)
		for _, c := range ofile.Doc.Outline.Children {
			results = getTargetHeadings(ofile, file, c, results, depth)
		}
	}
	return results
}
