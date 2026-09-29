package orgs

// Searching the text of every org file.
//
// `/search` queries the parsed database and knows what a heading is. This reads
// the files as text and knows nothing at all, which is the point: a phrase you
// half remember could be in a drawer, in a table, in a source block or in the
// middle of a paragraph, and the thing that finds it is a regular expression
// over the lines.
//
// `/grep` has always done something close to this, but it answers with lines of
// the form "file:12:text" - which cannot be taken apart again when the text
// holds a colon, and says nothing about how many matches were left out. This
// answers with the pieces.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

// How many matching lines are kept per file, and how many files are kept, when
// the client asks for no particular number.
//
// A cap is not a detail: `.` matches every line of every file, and somebody
// typing a pattern types `.` on the way to something. What matters is that the
// count is of everything that matched and only the *lines* are capped, so the
// answer never lies about how much it found.
const defaultFileSearchMax = 40
const maxFileSearchMax = 500

/* SDOC: API
* GET /files/search — Search the Text of Every Org File

	A regular-expression search over the raw text of every org file the server is
	watching, answering with the lines that matched and where they are.

	This is the text-level sibling of =/search=: that one queries the parsed
	database and understands headings, keywords and dates; this one reads lines
	and understands nothing, which is what finds a phrase written in a drawer, a
	table or a source block.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter     | Type   | Required | Description                                                       |
	|---------------+--------+----------+-------------------------------------------------------------------|
	| =q=           | string | yes      | The regular expression (Go syntax)                                |
	| =ignoreCase=  | string | no       | =t= to match without regard to case                               |
	| =max=         | number | no       | Matching lines kept per file (default 40, capped at 500)          |
	| =file=        | string | no       | Only search this file (basename or path)                          |

	*Response:* A =FileSearchResult= JSON object.
	| Field       | Type    | Description                                              |
	|-------------+---------+----------------------------------------------------------|
	| =Ok=        | bool    | False when the pattern does not compile                  |
	| =Msg=       | string  | Why it does not compile                                  |
	| =Files=     | array   | One entry per file with at least one match               |
	| =FileCount= | number  | How many files matched                                   |
	| =Total=     | number  | How many lines matched, across every file                |

	Each file carries =Filename=, =Count= (every matching line, even the ones
	past the cap), =Truncated=, and =Matches= of =Line= (zero based), =Text=,
	=Start= and =End=.

	A pattern that does not compile answers =200= with =Ok: false=, because a
	half-typed pattern is an ordinary state of a box somebody is typing in rather
	than a failed request.
EDOC */
func RequestFileSearch(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	w.Header().Set("Content-Type", "application/json")

	query := r.URL.Query().Get("q")
	if query == "" {
		query = r.URL.Query().Get("query")
	}
	out := common.FileSearchResult{Ok: true, Query: query, Files: []common.FileSearchFile{}}
	if strings.TrimSpace(query) == "" {
		json.NewEncoder(w).Encode(out)
		return
	}

	pattern := query
	if r.URL.Query().Get("ignoreCase") == "t" {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		out.Ok = false
		out.Msg = err.Error()
		json.NewEncoder(w).Encode(out)
		return
	}

	max := defaultFileSearchMax
	if n, err := strconv.Atoi(r.URL.Query().Get("max")); err == nil && n > 0 {
		max = n
	}
	if max > maxFileSearchMax {
		max = maxFileSearchMax
	}

	only := strings.TrimSpace(r.URL.Query().Get("file"))
	if only != "" {
		if f, err := resolveRequestedFile(only); err == nil {
			only = f
		}
	}

	// Which files could possibly match, from the trigram index.
	//
	// nil means it could not tell - a pattern with no mandatory literal run,
	// `.` or `a|b` or anything starting with a character class - and then every
	// file is scanned, which is what happened before the index existed. The
	// index only ever narrows, and the real expression still runs on whatever
	// survives, so a search cannot be made wrong by it. See trigram.go.
	all := GetDb().GetFiles()
	maybe := getTrigramIndex().candidates(pattern, all)
	out.Scanned = 0

	for _, fname := range all {
		if only != "" && fname != only {
			continue
		}
		if maybe != nil && !maybe[fname] {
			continue
		}
		out.Scanned++
		hits, err := searchOneFile(fname, re, max)
		if err != nil || hits.Count == 0 {
			continue
		}
		out.Files = append(out.Files, hits)
		out.FileCount++
		out.Total += hits.Count
	}

	sort.Slice(out.Files, func(i, j int) bool {
		return out.Files[i].Filename < out.Files[j].Filename
	})
	json.NewEncoder(w).Encode(out)
}

// The matching lines of one file, keeping at most max of them but counting all
// of them.
func searchOneFile(fname string, re *regexp.Regexp, max int) (common.FileSearchFile, error) {
	out := common.FileSearchFile{Filename: fname, Matches: []common.FileSearchMatch{}}
	fh, err := os.Open(fname)
	if err != nil {
		return out, err
	}
	defer fh.Close()

	scanner := bufio.NewScanner(fh)
	// An org file can hold a very long line - a base64 image, a minified blob -
	// and the default 64k limit stops the scan dead at the first one, silently
	// losing every match after it.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	row := -1
	for scanner.Scan() {
		row++
		line := scanner.Text()
		loc := re.FindStringIndex(line)
		if loc == nil {
			continue
		}
		out.Count++
		if len(out.Matches) >= max {
			out.Truncated = true
			continue
		}
		// A line of a thousand characters is not worth sending whole, and the
		// interesting part of it is around the match.
		text, start, end := trimAround(line, loc[0], loc[1])
		out.Matches = append(out.Matches, common.FileSearchMatch{
			Line: row, Text: text, Start: start, End: end,
		})
	}
	if err := scanner.Err(); err != nil {
		return out, fmt.Errorf("%s: %v", fname, err)
	}
	return out, nil
}

// A window of a long line around the match, with the offsets moved to match.
// Short lines are left exactly as they are, which is nearly all of them.
const lineWindow = 300
const lineLead = 80

func trimAround(line string, start, end int) (string, int, int) {
	if len(line) <= lineWindow {
		return line, start, end
	}
	from := start - lineLead
	if from < 0 {
		from = 0
	}
	to := from + lineWindow
	if to > len(line) {
		to = len(line)
	}
	// Never cut the match itself in half.
	if end > to {
		to = end
		if to > len(line) {
			to = len(line)
		}
	}
	out := line[from:to]
	prefix := ""
	if from > 0 {
		prefix = "…"
	}
	if to < len(line) {
		out += "…"
	}
	return prefix + out, start - from + len(prefix), end - from + len(prefix)
}
