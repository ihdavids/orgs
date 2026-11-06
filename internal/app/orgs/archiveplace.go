package orgs

// Where a file or a heading archives to, for the links panel's archive button:
// the same rule Archive follows (FindArchiveTarget), answered as a place to
// open rather than a move to make.

import (
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"
)

type ArchivePlace struct {
	Ok       bool
	Msg      string
	Rule     string // the `file::heading` that was read
	From     string // where the rule was said: heading, file or default
	Filename string // the archive file
	Heading  string // the heading archived things go under, if the rule names one
	Line     int    // zero based line of that heading in the archive file, else 0
	Exists   bool   // whether the archive file is there yet
	Loaded   bool   // whether the server has it loaded (a rendered view needs it)
	SameFile bool   // an archive sibling heading in the file itself
}

// A headline's tags, to compare titles without them.
var archiveTagsRe = regexp.MustCompile(`\s+:[\w@#%:]+:\s*$`)

// archiveHeadingLine is the zero based line of the level lvl heading titled
// title in lines, or -1. Any level matches when lvl is 0.
func archiveHeadingLine(lines []string, title string, lvl int) int {
	want := strings.TrimSpace(title)
	for i, l := range lines {
		m := headlineRegexp.FindStringSubmatch(l)
		if m == nil || (lvl > 0 && len(m[1]) != lvl) {
			continue
		}
		if strings.TrimSpace(archiveTagsRe.ReplaceAllString(m[2], "")) == want {
			return i
		}
	}
	return -1
}

/* SDOC: API
* GET /archive/where — Where A File Or Heading Archives To

	The archive file (and heading in it) that archiving would send a heading
	to, worked out by the same rules as =/archive=: the heading's =ARCHIVE=
	property, then the file's =#+ARCHIVE:= line, then =archiveDefaultTarget=.
	Asked about a file, the heading part is left out. Nothing is written; the
	links panel uses it to open the archive.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                                  |
	|------------+--------+----------+--------------------------------------------------------------|
	| =hash=     | string | one of   | The heading, by hash (query-escaped, not base64 again).      |
	| =filename= | string | one of   | A file. With nothing else, the file is the target.           |
	| =line=     | int    | no       | With =filename=: the heading this zero based line is under.  |
	| =heading=  | string | no       | With =filename=: a heading by its text.                      |

	*Response:* An =ArchivePlace= object: =Filename=, =Heading= and its zero
	based =Line= there (0 when it is not there yet), =Rule= and =From=
	(=heading=, =file= or =default=), =Exists=, =Loaded= and =SameFile=.
EDOC */
func RequestArchivePlace(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(archivePlaceOf(r))
}

func archivePlaceOf(r *http.Request) ArchivePlace {
	f, sec, err := requestedTarget(r)
	if err != nil {
		return ArchivePlace{Msg: err.Error()}
	}
	rule, from := archiveRule(f, sec)
	res := ArchivePlace{Rule: rule, From: from}
	tgt := archiveTargetOf(f, rule)
	if tgt == nil {
		res.Msg = "no archive location: the rule " + strings.TrimSpace(rule) + " names no file or heading to archive to"
		if strings.TrimSpace(rule) == "" {
			res.Msg = "no archive location: archiveDefaultTarget is not set and the file has no #+ARCHIVE: line"
		}
		return res
	}
	res.Filename, res.Heading = tgt.Filename, tgt.Id
	res.SameFile = tgt.Filename == f.Filename
	if _, err := os.Stat(tgt.Filename); err != nil {
		res.Msg = "nothing has been archived there yet: " + tgt.Filename + " does not exist"
		return res
	}
	res.Exists = true
	res.Loaded = GetDb().FindByFile(tgt.Filename) != nil
	if res.Heading != "" {
		if lines, err := readOrgLines(tgt.Filename); err == nil {
			if n := archiveHeadingLine(lines, res.Heading, tgt.Lvl); n >= 0 {
				res.Line = n
			}
		}
	}
	res.Ok = true
	return res
}
