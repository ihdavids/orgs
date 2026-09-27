//lint:file-ignore ST1006 allow the use of self
package orgs

/* SDOC: Editing

* Capture
  The capture tool can take a quick snippet of text
  and turn it into a targetted org entry in a file
  of your choosing. With Refile capability Capture
  can be a major asset when helping you organize your
  thoughts.

  Configuration for the system is done through
  the captureTemplates entry in your orgs.yaml file:

  #+BEGIN_SRC yaml
    captureTemplates:
      - name: "BasicEntry"
        type: "entry"
        target:
          type: "file+headline"
          filename: "test.org"
          id: "Captures"
      - name: "BasicItem"
        type: "item"
        target:
          type: "file+headline"
          filename: "test.org"
          id: "Captures"
      - name: "BasicCheckItem"
        type: "checkitem"
        target:
          type: "file+headline"
          filename: "test.org"
          id: "Captures"
      - name: "BasicPlain"
        type: "plain"
        target:
          type: "file+headline"
          filename: "test.org"
          id: "Captures"
      - name: "BasicTable"
        type: "table-line"
        target:
          type: "file+headline"
          filename: "test.org"
          id: "Captures"
      - name: "Datetree"
        type: "entry"
        target:
          type: "file+datetree"
          filename: "test.org"
          id: "Captures"
      - name: "OlpDatetree"
        type: "entry"
        target:
          type: "file+olp+datetree"
          filename: "test.org"
          id: "Captures::Level1::Level2"
    #+END_SRC

** The template string

   A template may also carry a =template:= of its own, which is the shape of
   what gets captured. The server does nothing with it - it hands the client's
   headline and content to the file exactly as they arrive - so the string is a
   *form for the client to put up*: worg reads it in =worg/src/capture.ts= and
   turns it into boxes to fill in, and anything else talking to =/capture= is
   free to do the same or to ignore it.

   #+BEGIN_SRC yaml
      - name: "QuickNote"
        type: "entry"
        target:
          type: "file+headline"
          filename: "notes.org"
          id: "Inbox"
        template: |-
          :PROPERTIES:
          :CREATED: {{now}}
          :SOURCE:  {{source|Where did this come from?}}
          :END:
          {{CONTENT}}
   #+END_SRC

   Note the =|-=. A yaml scalar written over several lines *without* it folds
   its newlines into spaces, so the whole template arrives as one line - which
   is a property drawer that is not a property drawer. Use a block scalar for
   anything longer than one line.

   The syntax is three rules:

   | Written                | Means                                             |
   |------------------------+---------------------------------------------------|
   | ={{CONTENT}}=          | The body - one big box, and where a dictation or  |
   |                        | a pasted picture lands                            |
   | ={{name}}=             | A value to fill in, labelled from the name        |
   | ={{name\vert prompt}}= | The same, asked for in your own words             |

   A handful of names fill themselves in, and stay editable - a thing captured
   today did not necessarily happen today:

   | Name                      | Becomes                  |
   |---------------------------+--------------------------|
   | ={{date}}=                | =2026-09-26=             |
   | ={{time}}=                | =14:05=                  |
   | ={{datetime}}=            | =2026-09-26 14:05=       |
   | ={{today}}= / ={{active}}=| =<2026-09-26 Sat>=       |
   | ={{now}}= / ={{inactive}}=| =[2026-09-26 Sat 14:05]= |
   | ={{week}}=                | =2026-W39=               |
   | ={{month}}=               | =2026-09=                |
   | ={{year}}=                | =2026=                   |

   A line whose *only* content was a placeholder nobody filled in is dropped
   rather than written empty, so a template offering three optional properties
   writes the one that was answered.

   A template with no string at all asks for a headline and a body, which is
   what every example above is.
EDOC */

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

type FindInsertPosition func(sec *org.Section, typeName string) *org.Pos

func FindCaptureTemplate(name string, username string) *common.CaptureTemplate {
	// Check user-specific templates first
	if username != "" && GetExtensions() != nil {
		for _, cap := range GetExtensions().GetUserCaptureTemplates(username) {
			if cap.Name == name {
				return &cap
			}
		}
	}
	// Fall back to global templates
	for _, cap := range Conf().Server.CaptureTemplates {
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
	fmt.Printf("[InsertEntryUsingTemplate]: %s\n", filename)
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
					indent := strings.Repeat(" ", sec.Headline.Lvl+2)
					if tname == "entry" {
						head := strings.Repeat("*", sec.Headline.Lvl+1) + " " + args.NewNode.Headline
						if t := captureTags(args.NewNode.Tags); t != "" {
							head += "  " + t
						}
						fileContent += head + "\n"
					}
					if tname == "plain" || tname == "entry" {
						fileContent += indentEachLine(args.NewNode.Content, indent)
					} else if tname == "table-line" {
						fileContent += indent + "- [ ] " + args.NewNode.Content + "\n"
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

// Every line of a capture's content at the body indent.
//
// It used to be one concatenation - indent, the whole content, a newline -
// which is right for the one line somebody types into the terminal client and
// wrong for everything a template produces. A template's content is several
// lines, and only the first of them came out indented: the rest landed at
// column zero, where org reads a property drawer as belonging to the document
// rather than to the heading above it.
func indentEachLine(content, indent string) string {
	out := ""
	for _, line := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			out += "\n"
			continue
		}
		out += indent + strings.TrimRight(line, " \t") + "\n"
	}
	return out
}

// The tags of a captured entry, written the way org writes them. Kept out of
// the headline when there are none rather than leaving `::` behind.
func captureTags(tags []string) string {
	clean := []string{}
	for _, t := range tags {
		if t = strings.TrimSpace(t); t != "" {
			clean = append(clean, t)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	return ":" + strings.Join(clean, ":") + ":"
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

func Capture(db common.ODb, args *common.Capture, username string) (common.ResultMsg, error) {
	var res common.ResultMsg = common.ResultMsg{}
	temp := FindCaptureTemplate(args.Template, username)
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

func QueryCaptureTemplates(username string) ([]common.CaptureTemplate, error) {
	var res []common.CaptureTemplate
	// Start with global templates
	if Conf().Server.CaptureTemplates != nil {
		res = append(res, Conf().Server.CaptureTemplates...)
	}
	// Merge in user-specific templates (user templates with the same name override global)
	if username != "" && GetExtensions() != nil {
		userTemplates := GetExtensions().GetUserCaptureTemplates(username)
		for _, ut := range userTemplates {
			found := false
			for i := range res {
				if res[i].Name == ut.Name {
					res[i] = ut
					found = true
					break
				}
			}
			if !found {
				res = append(res, ut)
			}
		}
	}
	if len(res) > 0 {
		return res, nil
	}
	return []common.CaptureTemplate{}, fmt.Errorf("Capture: failed to find any capture templates")
}
