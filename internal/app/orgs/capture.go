//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/common"
)

type FindInsertPosition func(sec *org.Section, typeName string) *org.Pos

func FindCaptureTemplate(name string) *common.CaptureTemplate {
	for _, cap := range Conf().CaptureTemplates {
		if cap.Name == name {
			return &cap
		}
	}
	return nil
}

// Drill down to find the lowest child of the last child, this is where we will append?
// This might not be the right behaviour here.
func GetLastChild(sec *org.Section) *org.Section {
	if len(sec.Children) > 0 {
		sec = sec.Children[len(sec.Children)-1]
	}
	if len(sec.Children) > 0 {
		return GetLastChild(sec)
	}
	return sec
}

func GetEndOfHeadline(n *org.Headline) *org.Pos {
	/*
		if len(n.Children) > 0 {
			pos := n.Children[len(n.Children)-1].GetPos()
			fmt.Printf("GOT END: %v\n", pos)
			return &pos
		}
		pos := n.GetPos()
	*/
	pos := n.GetEnd()
	return &pos
}

func EndRow(sec *org.Section, _typeName string) *org.Pos {
	sec = GetLastChild(sec)
	return GetEndOfHeadline(sec.Headline)
}

func findDeletePos(sec *org.Section) (*org.Pos, *org.Pos) {
	s := sec.Headline.GetPos()
	e := sec.Headline.GetEnd()
	return &s, &e
}

func findInsertPos(sec *org.Section) *org.Pos {
	e := sec.Headline.GetEnd()
	return &e
}

/*
	type List struct {
		Kind  string
		Pos   Pos
		Items []Node
	}
*/
// There are 2 kinds of list:
// return t.kind == "unorderedList" || t.kind == "orderedList"
func isRightListType(checked bool, lst org.List) bool {
	if len(lst.Items) > 0 {
		itm := lst.Items[0].(org.ListItem)
		shouldBeChecked := (itm.Status == " " || itm.Status == "X" || itm.Status == "x" || itm.Status == "-")
		return (checked == shouldBeChecked)
	}
	return true
}

func GetListRow(sec *org.Section, subType string, tname string) (*org.Pos, *org.ListItem) {
	checked := tname == "checkitem"
	if sec != nil {
		if subType == "" {
			subType = "unordered"
		}
		// Check me
		for _, node := range sec.Headline.Children {
			//fmt.Printf("NODE: %v\n", node)
			// Find the first list in the nodes of the section
			if lst, ok := node.(org.List); ok {
				//fmt.Printf("This IS a list: %v %v\n", subType, lst.Kind)
				if subType == lst.Kind && isRightListType(checked, lst) {
					item := lst.Items[len(lst.Items)-1]
					pos := item.GetPos()
					litem := item.(org.ListItem)
					return &pos, &litem
				}
			}
		}

		// Check my children
		for _, s := range sec.Children {
			// Check my children
			p, l := GetListRow(s, subType, tname)
			if l != nil {
				return p, l
			}
		}

		// Okay we didn't find anything to insert against so create one!
		p := GetEndOfHeadline(sec.Headline)
		return p, nil
	}
	return nil, nil
}

func GetTableRow(sec *org.Section, tname string) (*org.Pos, *org.Table) {
	if sec != nil {
		for _, node := range sec.Headline.Children {
			// Find the first list in the nodes of the section
			if tbl, ok := node.(org.Table); ok {
				pos := tbl.GetEnd()
				end := tbl.GetPos()
				fmt.Printf("GOT TABLE!!!!!!!!!!!!!!!!!!!!! %v %v\n", pos, end)
				for _, r := range tbl.Rows {
					fmt.Printf("%v %v\n", r.GetPos(), r.GetEnd())
					for _, c := range r.Columns {
						fmt.Printf("| %v %v |\n", c.GetPos(), c.GetEnd())
					}
				}
				return &pos, &tbl
			}
		}

		// Check my children
		for _, s := range sec.Children {
			// Check my children
			p, l := GetTableRow(s, tname)
			if l != nil {
				return p, l
			}
		}

		// Okay we didn't find anything to insert against so create one!
		p := GetEndOfHeadline(sec.Headline)
		return p, nil
	}
	return nil, nil
}

func InsertEntryUsingTemplate(args *common.Capture, filename string, sec *org.Section, res *common.ResultMsg, tname string, findInsertPos FindInsertPosition) {
	fmt.Printf("[InsertEntryUsingTemplate]\n")
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
			res.Msg = "Capture: failed to open file " + err.Error()
		} else {
			// This gives us the row of the section we want to add to.
			subtype := ""
			if tname == "item" {
				subtype = "unorderedList"
			}
			p := findInsertPos(sec, subtype)
			fileContent := ""
			// Now iterate over the file and insert our content where it should go!
			for i, line := range lines {

				fileContent += line
				fileContent += "\n"

				// Last line of file has to be added after
				if i == p.Row {
					// fmt.Printf("WRITING: i %d row %d endLine %d", i, p.Row, len(lines))
					if tname == "entry" {
						fileContent += strings.Repeat("*", sec.Headline.Lvl+1) + " " + args.NewNode.Headline + "\n"
					}
					if tname == "plain" || tname == "entry" {
						fileContent += strings.Repeat(" ", sec.Headline.Lvl+2) + args.NewNode.Content + "\n"
					} else if tname == "table-line" {
						fileContent += strings.Repeat(" ", sec.Headline.Lvl+2) + "- [ ] " + args.NewNode.Content + "\n"
					}
				}
			}
			fmt.Printf("Writing FILE: %v\n", filename)
			os.WriteFile(filename, []byte(fileContent), 0644)
			res.Ok = true
			res.Msg = "Capture successful"
		}
	}
}

func isEmpty(s string) bool {
	return strings.TrimSpace(s) == ""
}

func InsertItemUsingTemplate(args *common.Capture, filename string, sec *org.Section, res *common.ResultMsg, tname string) {
	fmt.Printf("  [InsertItemUsingTemplate]\n")
	if r, err := os.Open(filename); err == nil {
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
			res.Msg = "Capture: failed to open file " + err.Error()
		} else {
			// This gives us the row of the section we want to add to.
			subtype := "unordered"
			var p *org.Pos = nil
			var litem *org.ListItem = nil
			var tbl *org.Table = nil
			row := 0
			if tname == "table-line" {
				p, tbl = GetTableRow(sec, tname)
				row = p.Row
			} else if tname != "plain" {
				p, litem = GetListRow(sec, subtype, tname)
				row = p.Row
			}
			// If this is an empty item then move up one line to ensure this ends up in the heading
			// vs in the next heading.
			if litem == nil && tbl == nil {
				// Before first child heading
				if len(sec.Children) > 0 {
					pend := sec.Children[0].Headline.GetPos()
					row = pend.Row - 1
					if p == nil {
						p = &pend
					}
					// After last line of node
				} else if len(sec.Headline.Children) > 0 {
					pend := sec.Headline.Children[len(sec.Headline.Children)-1].GetEnd()
					row = pend.Row
					if p == nil {
						p = &pend
					}
				} else {
					pend := sec.Headline.GetTokenEnd()
					row = pend.Row
					if p == nil {
						p = &pend
					}
				}
			}

			// fmt.Printf("Have some stuff: %v %v\n", p, litem)
			fileContent := ""
			// Now iterate over the file and insert our content where it should go!
			var emptyLines []string = []string{}
			didAdd := false
			for i, line := range lines {

				if i == row+1 {
					bullet := "-"
					if litem != nil {
						bullet = litem.Bullet
					}
					if tname == "item" {
						fileContent += strings.Repeat(" ", sec.Headline.Lvl+2) + bullet + " " + args.NewNode.Content + "\n"
						didAdd = true
					} else if tname == "checkitem" {
						fileContent += strings.Repeat(" ", sec.Headline.Lvl+2) + bullet + " [ ] " + args.NewNode.Content + "\n"
						didAdd = true
					} else if tname == "plain" {
						fileContent += strings.Repeat(" ", sec.Headline.Lvl+2) + args.NewNode.Content + "\n"
					} else if tname == "table-line" {
						fileContent += strings.Repeat(" ", sec.Headline.Lvl+2) + args.NewNode.Content + "\n"
					}
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
					bullet := "-"
					if litem != nil {
						bullet = litem.Bullet
					}
					if tname == "item" {
						fileContent += strings.Repeat(" ", sec.Headline.Lvl+2) + bullet + " " + args.NewNode.Content + "\n"
					} else if tname == "checkitem" {
						fileContent += strings.Repeat(" ", sec.Headline.Lvl+2) + bullet + " [ ] " + args.NewNode.Content + "\n"
					} else if tname == "plain" {
						fileContent += strings.Repeat(" ", sec.Headline.Lvl+2) + args.NewNode.Content + "\n"
					} else if tname == "table-line" {
						fileContent += strings.Repeat(" ", sec.Headline.Lvl+2) + args.NewNode.Content + "\n"
					}
				}
			}
			fmt.Printf("Writing FILE: %v\n", filename)
			os.WriteFile(filename, []byte(fileContent), 0644)
			res.Ok = true
			res.Msg = "Capture successful"
		}
	}
}

func Capture(db plugs.ODb, args *common.Capture) (common.ResultMsg, error) {
	var res common.ResultMsg = common.ResultMsg{}
	temp := FindCaptureTemplate(args.Template)
	res.Ok = false
	res.Msg = "Capture: unknown failure, did not capture"
	if temp != nil {
		file, secs := db.GetFromTarget(&temp.CapTarget, true)
		if file == nil || secs == nil {
			res.Msg = fmt.Sprintf("Capture: could not find target [%s]", temp.CapTarget.Type)
			res.Ok = false
			return res, nil
		}
		tname := strings.ToLower(temp.Type)
		if tname == "" || tname == "entry" {
			InsertEntryUsingTemplate(args, file.Doc.Path, secs, &res, tname, EndRow)
		} else if tname == "item" {
			InsertItemUsingTemplate(args, file.Doc.Path, secs, &res, tname)
		} else if tname == "checkitem" {
			InsertItemUsingTemplate(args, file.Doc.Path, secs, &res, tname)
		} else if tname == "table-line" {
			InsertItemUsingTemplate(args, file.Doc.Path, secs, &res, tname)
		} else if tname == "plain" {
			InsertItemUsingTemplate(args, file.Doc.Path, secs, &res, tname)
		} else {
			fmt.Printf("Capture: invalid capture type [%s]\n", temp.Type)
			res.Msg = fmt.Sprintf("Capture: invalid capture type  [%s]", temp.Type)
		}
		return res, nil
	} else {
		fmt.Printf("Failed to find capture template [%s]\n", args.Template)
		res.Msg = fmt.Sprintf("failed to find capture template [%s]", args.Template)
		res.Ok = false
		return res, nil
	}
}

func QueryCaptureTemplates() ([]common.CaptureTemplate, error) {

	var res []common.CaptureTemplate = Conf().CaptureTemplates
	if res != nil {
		return res, nil
	} else {
		return []common.CaptureTemplate{}, fmt.Errorf("Capture: failed to find any capture templates")
	}
}
