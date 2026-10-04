package orgs

// Setting and clearing a heading's SCHEDULED, DEADLINE, CLOSED or own
// timestamp, as a line edit (see Traps: line edits in CLAUDE.md).
//
// This used to change the parsed headline and write the whole document back
// out, which re-indented every drawer in the file and put the planning line
// *below* the property drawer, where org no longer reads it as planning. It
// also found the heading with a bare ByHash read, which answers with whatever
// the last parse left there - after another edit shifted the lines, a gantt
// bar dragged in worg moved a different heading.

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

var anyStampRe = regexp.MustCompile(`[<\[]\d{4}-\d{2}-\d{2}[^>\]]*[>\]](--[<\[]\d{4}-\d{2}-\d{2}[^>\]]*[>\]])?`)

func planningKeyRe(key string) *regexp.Regexp {
	return regexp.MustCompile(key + `:\s*` + anyStampRe.String())
}

// The stamp to write, from what a caller sent: a stamp as written in a file
// ("<2026-01-01 Mon>", kept as it is so repeaters and spellings survive), or a
// bare date ("2026-01-01 10:00"), which is read and written in org's form.
// CLOSED is inactive; everything else is active.
func dateChangeStamp(name, value string) (string, error) {
	v := strings.TrimSpace(value)
	if anyStampRe.MatchString(v) && anyStampRe.FindString(v) == v {
		if d, _, _ := org.ParseTimestamp(v); d == nil {
			return "", fmt.Errorf("%q is not a date org can read", value)
		}
		return v, nil
	}
	open, close := "<", ">"
	if name == "CLOSED" {
		open, close = "[", "]"
	}
	d, _, _ := org.ParseTimestamp(open + v + close)
	if d == nil {
		return "", fmt.Errorf("%q is not a date org can read", value)
	}
	return d.ToString(), nil
}

// The indent a new line under the heading gets: whatever its planning line or
// first drawer already uses, so a file written column zero (as Emacs has since
// org 9.5) stays that way; otherwise the usual indent for its level.
func headIndent(lines []string, from, own, lvl int) string {
	if from+1 <= own && from+1 < len(lines) {
		l := lines[from+1]
		if planningRe.MatchString(l) || drawerOpenRe.MatchString(l) {
			return l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		}
	}
	return indentOf(lvl)
}

// Set (or with stamp "" remove) one planning keyword in a heading's own
// lines [from, own]. The planning line is the line straight after the
// headline; one is made there when the heading has none.
func setPlanning(lines []string, from, own int, ind, key, stamp string) []string {
	re := planningKeyRe(key)
	planning := -1
	if from+1 <= own && from+1 < len(lines) && planningRe.MatchString(lines[from+1]) {
		planning = from + 1
	}
	if stamp == "" {
		if planning < 0 || !re.MatchString(lines[planning]) {
			return lines
		}
		l := lines[planning]
		lead := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		rest := strings.Join(strings.Fields(re.ReplaceAllString(l, "")), " ")
		if rest == "" {
			return append(lines[:planning:planning], lines[planning+1:]...)
		}
		lines[planning] = lead + rest
		return lines
	}
	if planning >= 0 {
		if re.MatchString(lines[planning]) {
			lines[planning] = re.ReplaceAllLiteralString(lines[planning], key+": "+stamp)
		} else {
			lines[planning] = strings.TrimRight(lines[planning], " \t") + " " + key + ": " + stamp
		}
		return lines
	}
	return splice(lines, from+1, []string{ind + key + ": " + stamp})
}

// Set (or with stamp "" remove) the heading's own timestamp: the first active
// stamp in its own lines outside the planning line and drawers - the one
// go-org reads as its date. A new one goes after the planning line and drawers.
func setOwnStamp(lines []string, from, own int, ind, stamp string) []string {
	inDrawer := false
	for i := from + 1; i <= own && i < len(lines); i++ {
		if i == from+1 && planningRe.MatchString(lines[i]) {
			continue
		}
		// The end first: `:END:` also reads as a drawer opening.
		if inDrawer {
			if drawerEndRe.MatchString(lines[i]) {
				inDrawer = false
			}
			continue
		}
		if drawerOpenRe.MatchString(lines[i]) {
			inDrawer = true
			continue
		}
		loc := markerStampRe.FindStringIndex(lines[i])
		if loc == nil {
			continue
		}
		if stamp != "" {
			lines[i] = lines[i][:loc[0]] + stamp + lines[i][loc[1]:]
			return lines
		}
		left := lines[i][:loc[0]] + lines[i][loc[1]:]
		if strings.TrimSpace(left) == "" {
			return append(lines[:i:i], lines[i+1:]...)
		}
		lines[i] = strings.TrimRight(left, " \t")
		return lines
	}
	if stamp == "" {
		return lines
	}
	return splice(lines, markerHeadEnd(lines, from, own), []string{ind + stamp})
}

// The whole edit on a file's lines: the heading starts at row `from`.
func changeDateLines(lines []string, from, lvl int, name, stamp string) ([]string, error) {
	to := subtreeEndRow(lines, from, lvl, from)
	if to >= len(lines) {
		to = len(lines) - 1
	}
	own := ownLinesEnd(lines, from, to)
	ind := headIndent(lines, from, own, lvl)
	switch name {
	case "SCHEDULED", "DEADLINE", "CLOSED":
		return setPlanning(lines, from, own, ind, name, stamp), nil
	case "TIMESTAMP":
		return setOwnStamp(lines, from, own, ind, stamp), nil
	}
	return nil, fmt.Errorf("%q is not a date a heading has: SCHEDULED, DEADLINE, CLOSED or TIMESTAMP", name)
}

func ChangeDate(query *common.TodoDateChange) (common.Result, error) {
	name := strings.ToUpper(strings.TrimSpace(query.Name))
	stamp := ""
	if strings.TrimSpace(query.Value) != "" {
		var err error
		if stamp, err = dateChangeStamp(name, query.Value); err != nil {
			return common.Result{Ok: false}, err
		}
	}
	// FindByHash rather than a bare ByHash read: sections are registered
	// lazily, and the map answers with whatever the last parse left there.
	sec := GetDb().FindByHash(string(query.Hash))
	if sec == nil || sec.Headline == nil {
		return common.Result{Ok: false}, fmt.Errorf("no heading with that hash - it may have moved; refresh and try again")
	}
	f := GetDb().FileFromSection(sec)
	if f == nil {
		return common.Result{Ok: false}, fmt.Errorf("could not find the file that heading is in")
	}
	lines, from, _, ok := recordLines(f.Doc.Path, sec)
	if !ok {
		return common.Result{Ok: false}, fmt.Errorf("could not read %s", f.Doc.Path)
	}
	lines, err := changeDateLines(lines, from, sec.Headline.Lvl, name, stamp)
	if err != nil {
		return common.Result{Ok: false}, err
	}
	if err := writeLines(f.Doc.Path, lines); err != nil {
		return common.Result{Ok: false}, err
	}
	return common.Result{Ok: true}, nil
}
