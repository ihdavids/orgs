//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// Markers are ordinary headings - a release, an offsite, a code freeze - that a
// gantt view draws as a line down the chart, or a band when the date is a
// range. Everything here is a line edit of the heading's own lines (see the
// root CLAUDE.md, Traps: line edits): the headline, the date and nothing else.

var (
	markerTimeRe = regexp.MustCompile(`^\d{1,2}:\d{2}$`)
	// An active stamp, or a range of two, anywhere on a line.
	markerStampRe = regexp.MustCompile(`<\d{4}-\d{2}-\d{2}[^>]*>(--<\d{4}-\d{2}-\d{2}[^>]*>)?`)
	markerDeadRe  = regexp.MustCompile(`DEADLINE:\s*<[^>]*>`)
	markerSchedRe = regexp.MustCompile(`SCHEDULED:\s*<[^>]*>`)
	markerHeadRe  = regexp.MustCompile(`^(\*+)\s`)
)

// One end of a marker as org writes it: <2026-10-14 Wed>, <… 10:00>, or
// <… 10:00-11:00>.
func markerStamp(d common.GanttMarkerDay) (string, error) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(d.Date))
	if err != nil {
		return "", fmt.Errorf("%q is not a date (YYYY-MM-DD)", d.Date)
	}
	s := t.Format("2006-01-02 Mon")
	if tm := strings.TrimSpace(d.Time); tm != "" {
		if !markerTimeRe.MatchString(tm) {
			return "", fmt.Errorf("%q is not a time (HH:MM)", tm)
		}
		s += " " + tm
		if u := strings.TrimSpace(d.Until); u != "" {
			if !markerTimeRe.MatchString(u) {
				return "", fmt.Errorf("%q is not a time (HH:MM)", u)
			}
			s += "-" + u
		}
	}
	return "<" + s + ">", nil
}

// The whole date: one stamp, or two joined by -- for a range.
func markerDateText(e *common.GanttMarkerEdit) (string, error) {
	a, err := markerStamp(e.Start)
	if err != nil {
		return "", err
	}
	if e.End == nil {
		return a, nil
	}
	if e.End.Date < e.Start.Date {
		return "", fmt.Errorf("a range has to end on or after the day it starts")
	}
	b, err := markerStamp(*e.End)
	if err != nil {
		return "", err
	}
	return a + "--" + b, nil
}

// The headline with a new title and, when asked, new tags - keeping the
// stars, the keyword and the priority cookie as they were.
func markerHeadline(line, status, priority, title string, tags []string, setTags bool) string {
	m := markerHeadRe.FindStringSubmatch(line)
	stars := "*"
	if m != nil {
		stars = m[1]
	}
	head := stars
	if status != "" {
		head += " " + status
	}
	if priority != "" {
		head += " [#" + priority + "]"
	}
	head += " " + strings.TrimSpace(title)
	if !setTags {
		if tm := regexp.MustCompile(`\s+(:[^\s]+:)\s*$`).FindStringSubmatch(line); tm != nil {
			head += "  " + tm[1]
		}
	} else if clean := cleanTags(tags); len(clean) > 0 {
		head += "  :" + strings.Join(clean, ":") + ":"
	}
	return head
}

func cleanTags(tags []string) []string {
	out := []string{}
	for _, t := range tags {
		t = strings.Trim(strings.TrimSpace(t), ":")
		if t != "" && !strings.ContainsAny(t, " \t:") {
			out = append(out, t)
		}
	}
	return out
}

// Where a heading's head ends: past its planning line and drawers, which is
// where a plain timestamp goes when it has none.
func markerHeadEnd(lines []string, from, own int) int {
	i := from + 1
	for i <= own && i < len(lines) {
		line := lines[i]
		if planningRe.MatchString(line) {
			i++
			continue
		}
		if drawerOpenRe.MatchString(line) {
			j := i + 1
			for j <= own && j < len(lines) && !drawerEndRe.MatchString(lines[j]) {
				j++
			}
			if j > own || j >= len(lines) {
				return i
			}
			i = j + 1
			continue
		}
		break
	}
	return i
}

// Set the marker's date in a heading's own lines [from, own]. Returns the
// lines; the caller writes them.
func setMarkerDate(lines []string, from, own int, ind, kind, date string, isRange bool) []string {
	planning := -1
	if from+1 <= own && from+1 < len(lines) && planningRe.MatchString(lines[from+1]) {
		planning = from + 1
	}
	if kind == "deadline" && !isRange {
		if planning >= 0 {
			if markerDeadRe.MatchString(lines[planning]) {
				lines[planning] = markerDeadRe.ReplaceAllString(lines[planning], "DEADLINE: "+date)
			} else {
				lines[planning] = strings.TrimRight(lines[planning], " ") + " DEADLINE: " + date
			}
			return lines
		}
		return splice(lines, from+1, []string{ind + "DEADLINE: " + date})
	}
	// The heading's own timestamp: the first active stamp in its body, outside
	// the planning line and drawers - the one go-org reads as its date.
	inDrawer := false
	for i := from + 1; i <= own && i < len(lines); i++ {
		if i == planning {
			continue
		}
		if drawerOpenRe.MatchString(lines[i]) {
			inDrawer = true
			continue
		}
		if inDrawer {
			if drawerEndRe.MatchString(lines[i]) {
				inDrawer = false
			}
			continue
		}
		if loc := markerStampRe.FindStringIndex(lines[i]); loc != nil {
			lines[i] = lines[i][:loc[0]] + date + lines[i][loc[1]:]
			return lines
		}
	}
	// None of its own. A single day on a heading dated only by SCHEDULED
	// moves its SCHEDULED; a range cannot live there, so the SCHEDULED is
	// taken off (or the marker would carry two dates) and the range goes in
	// as the heading's own stamp.
	if planning >= 0 && markerSchedRe.MatchString(lines[planning]) {
		if !isRange {
			lines[planning] = markerSchedRe.ReplaceAllString(lines[planning], "SCHEDULED: "+date)
			return lines
		}
		rest := strings.TrimSpace(markerSchedRe.ReplaceAllString(lines[planning], ""))
		if rest == "" {
			lines = append(lines[:planning], lines[planning+1:]...)
			own--
		} else {
			lines[planning] = ind + rest
		}
	}
	at := markerHeadEnd(lines, from, own)
	return splice(lines, at, []string{ind + date})
}

/* SDOC: API
* POST /gantt/marker — Add, Change or Delete a Marker

	A marker is a heading whose date a gantt view draws as a line down the
	chart (one day) or a band (a range of days). This writes one: a new
	heading, or the title, tags and date of an existing one, or deletes it.
	Only the heading's own lines are touched.

	*Method:* =POST=

	*Request Body:* A =GanttMarkerEdit= object.
	| Field        | Type   | Description                                                 |
	|--------------+--------+-------------------------------------------------------------|
	| =Hash=       | string | The marker to change or delete. Empty adds a new one.       |
	| =ParentHash= | string | Add under this heading, as its last child.                  |
	| =Filename=   | string | Or: add at the end of this file.                            |
	| =Delete=     | bool   | Delete the marker named by =Hash=.                          |
	| =Headline=   | string | The title.                                                  |
	| =Tags=       | list   | Its tags, written when =SetTags= is true.                   |
	| =Kind=       | string | ="date"= (its own timestamp) or ="deadline"=.               |
	| =Start=      | object | ={Date, Time, Until}=: a day, an optional time and the end  |
	|              |        | of a span within that day.                                  |
	| =End=        | object | The same for the last day of a range; absent for one day.   |

	A range is written as =<a>--<b>=, and each end may be a span of its day,
	=<2004-08-23 Mon 10:00-11:00>--<2004-08-26 Thu 10:00-11:00>=.

	*Response:* A =ResultMsg=.
EDOC */
func PostGanttMarker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	res := common.ResultMsg{Ok: false}
	fail := func(msg string) {
		res.Msg = "marker: " + msg
		json.NewEncoder(w).Encode(res)
	}
	body, _ := io.ReadAll(r.Body)
	var e common.GanttMarkerEdit
	if err := json.Unmarshal(body, &e); err != nil {
		fail("could not read request: " + err.Error())
		return
	}

	// --- an existing marker -------------------------------------------------
	if e.Hash != "" {
		sec := GetDb().FindByHash(e.Hash)
		if sec == nil || sec.Headline == nil {
			fail("no heading with that hash - it may have moved; refresh and try again")
			return
		}
		f := GetDb().FileFromSection(sec)
		if f == nil {
			fail("could not find the file that heading is in")
			return
		}
		lines, from, to, ok := recordLines(f.Doc.Path, sec)
		if !ok {
			fail("could not read " + f.Doc.Path)
			return
		}
		if e.Delete {
			lines = append(lines[:from], lines[to+1:]...)
			if err := writeLines(f.Doc.Path, lines); err != nil {
				fail(err.Error())
				return
			}
			res.Ok, res.Msg = true, "deleted"
			json.NewEncoder(w).Encode(res)
			return
		}
		date, err := markerDateText(&e)
		if err != nil {
			fail(err.Error())
			return
		}
		if strings.TrimSpace(e.Headline) != "" {
			lines[from] = markerHeadline(lines[from], sec.Headline.Status, sec.Headline.Priority, e.Headline, e.Tags, e.SetTags)
		} else if e.SetTags {
			title := ""
			for _, n := range sec.Headline.Title {
				title += n.String()
			}
			lines[from] = markerHeadline(lines[from], sec.Headline.Status, sec.Headline.Priority, title, e.Tags, true)
		}
		own := ownLinesEnd(lines, from, to)
		lines = setMarkerDate(lines, from, own, indentOf(sec.Headline.Lvl), e.Kind, date, e.End != nil)
		if err := writeLines(f.Doc.Path, lines); err != nil {
			fail(err.Error())
			return
		}
		res.Ok, res.Msg = true, "updated"
		json.NewEncoder(w).Encode(res)
		return
	}

	// --- a new marker -------------------------------------------------------
	if strings.TrimSpace(e.Headline) == "" {
		fail("a marker needs a name")
		return
	}
	date, err := markerDateText(&e)
	if err != nil {
		fail(err.Error())
		return
	}
	var filename string
	var row, lvl int
	if e.ParentHash != "" {
		sec := GetDb().FindByHash(e.ParentHash)
		if sec == nil || sec.Headline == nil {
			fail("no heading with that hash to add under")
			return
		}
		f := GetDb().FileFromSection(sec)
		if f == nil {
			fail("could not find the file that heading is in")
			return
		}
		filename = f.Doc.Path
		lvl = sec.Headline.Lvl + 1
		lines, lerr := ganttFileLines(filename)
		if lerr != nil {
			fail(lerr.Error())
			return
		}
		row = subtreeEndRow(lines, sec.Headline.Pos.Row, sec.Headline.Lvl, sec.Headline.Pos.Row)
	} else if e.Filename != "" {
		target := common.Target{Type: "file", Filename: e.Filename}
		f, _ := GetDb().GetFromTarget(&target, false)
		if f == nil {
			fail(fmt.Sprintf("no file [%s]", e.Filename))
			return
		}
		filename = f.Doc.Path
		lvl = 1
		row = ganttLastRow(filename)
	} else {
		fail("need a heading to put it under, or a file")
		return
	}
	head := strings.Repeat("*", lvl) + " " + strings.TrimSpace(e.Headline)
	if tags := cleanTags(e.Tags); len(tags) > 0 {
		head += "  :" + strings.Join(tags, ":") + ":"
	}
	ind := indentOf(lvl)
	dateLine := ind + date
	if e.Kind == "deadline" && e.End == nil {
		dateLine = ind + "DEADLINE: " + date
	}
	if _, err := insertLinesAt(filename, row, []string{head, dateLine}); err != nil {
		fail(err.Error())
		return
	}
	res.Ok, res.Msg = true, "added"
	json.NewEncoder(w).Encode(res)
}
