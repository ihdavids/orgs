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

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
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

func formatHeading(w *org.OrgWriter, sec *org.Section) {
	org.WriteNodes(w, *sec.Headline)
	for _, c := range sec.Children {
		formatHeading(w, c)
	}
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
	fmt.Printf("  [InsertSection]\n")
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
			fmt.Printf("Writing FILE: %v\n", to.Doc.Path)
			os.WriteFile(to.Doc.Path, []byte(fileContent), 0644)
			res.Ok = true
			res.Msg = "Insert successful"
		}
	}
}

func DeleteTree(filename string, sec *org.Section, res *common.ResultMsg) {
	fmt.Printf("[DeleteEntry]\n")
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
			fileContent := ""
			// Now iterate over the file and insert our content where it should go!
			for i, line := range lines {

				if i >= s.Row && i <= e.Row {
					continue
				}
				fileContent += line
				fileContent += "\n"
			}
			fmt.Printf("Writing FILE: %v\n", filename)
			os.WriteFile(filename, []byte(fileContent), 0644)
			res.Ok = true
			res.Msg = "Delete successful"
		}
	}
}

type ModifySourceFunc func(ofile *common.OrgFile, sec *org.Section) *org.Section

func Refile(db plugs.ODb, args *common.Refile, mod ModifySourceFunc, allowCreate bool) (common.ResultMsg, error) {
	var res common.ResultMsg = common.ResultMsg{}
	res.Ok = false
	res.Msg = "Refile: unknown failure, did not refile"
	fromFile, fromSecs := db.GetFromTarget(&args.FromId, false)
	if fromFile == nil || fromSecs == nil {
		res.Msg = fmt.Sprintf("Refile: could not find source target [%s]", args.FromId.Type)
		res.Ok = false
		fmt.Printf(">>> ERROR REFILE FROM NOT FOUND %s\n", res.Msg)
		return res, nil
	}
	toFile, toSecs := db.GetFromTarget(&args.ToId, allowCreate)
	if toFile == nil || toSecs == nil {
		res.Msg = fmt.Sprintf("Refile: could not find destination target [%s]", args.ToId.Type)
		res.Ok = false
		fmt.Printf(">>> ERROR REFILE TO NOT FOUND %s\n", res.Msg)
		return res, nil
	}
	if mod != nil {
		fromSecs = mod(fromFile, fromSecs)
	}
	InsertSection(toFile, fromSecs, toSecs, &res)
	if res.Ok {
		DeleteTree(fromFile.Doc.Path, fromSecs, &res)
	}
	return res, nil
}

func Delete(db plugs.ODb, tgt *common.Target) (common.ResultMsg, error) {
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
		requestedTargets = Conf().RefileTargets
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
