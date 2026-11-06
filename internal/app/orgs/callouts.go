package orgs

// Callouts (features.callouts): titled callout boxes as org special blocks,
// `#+begin_warning Mind the gap :fold closed`. The type table and the reading
// of a begin line are internal/common/callout.go, because the exporters (in
// plugs/, which cannot import this package) draw them; this file turns the
// writers on with the feature and answers the two endpoints.

import (
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common"
)

func init() {
	common.CalloutsOn = func() bool { return featureOn("callouts") }
}

func registerCalloutRoutes(api *mux.Router) {
	api.HandleFunc("/callouts/types", RequestCalloutTypes).Methods("GET")
	api.HandleFunc("/callouts", RequestCallouts).Methods("GET")
}

// CalloutAt is a callout and the heading it sits under.
type CalloutAt struct {
	common.Callout
	Heading string `json:",omitempty"` // the heading's title as written, "" above the first
	Shown   string // the title a reader sees
}

// CalloutFile is one file's callouts.
type CalloutFile struct {
	File     string
	Callouts []CalloutAt
}

var calloutHeadRe = regexp.MustCompile(`^(\*+)[ \t]+(.*)$`)

// calloutsOfFile lists a file's callouts from its lines (the file as it is on
// disk, so the lines are the editor's).
func calloutsOfFile(filename string) []CalloutAt {
	lines := fileLines(filename)
	out := []CalloutAt{}
	if lines == nil {
		return out
	}
	cs := common.CalloutsInLines(lines)
	heading, row := "", 0
	for _, c := range cs {
		for ; row < c.Line-1 && row < len(lines); row++ {
			if m := calloutHeadRe.FindStringSubmatch(lines[row]); m != nil {
				heading = strings.TrimSpace(m[2])
			}
		}
		out = append(out, CalloutAt{Callout: c, Heading: heading, Shown: c.ShownTitle()})
	}
	return out
}

/* SDOC: API
* GET /callouts/types — The Callout Types

	#+BEGIN_EXAMPLE
	GET /callouts/types
	-> {"Ok": true, "Types": [{"Name": "note", "Title": "Note", "Aliases": null, "Icon": "pencil",
	    "Glyph": "✎", "Color": "#086ddd", "Beamer": "block"},
	    {"Name": "warning", "Title": "Warning", "Aliases": ["caution", "attention"], ...}, ...]}
	#+END_EXAMPLE

	The callout types, each a block name (=#+begin_warning=) with its
	aliases, its lucide icon, a character for clients without
	icons, its colour and the beamer block it becomes. The one list worg, VS
	Code and the command line show. Needs =features: callouts:= on.
EDOC */
func RequestCalloutTypes(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "callouts") {
		return
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Types": common.CalloutTypes})
}

/* SDOC: API
* GET /callouts — Every Callout In A File

	#+BEGIN_EXAMPLE
	GET /callouts?file=notes.org
	-> {"Ok": true, "File": "/notes/notes.org", "Callouts": [{"Type": "warning", "Name": "warning",
	    "Title": "Mind the gap", "Fold": "closed", "Icon": "", "Color": "", "Unknown": "",
	    "Line": 12, "EndLine": 15, "Heading": "Travel", "Shown": "Mind the gap"}]}
	GET /callouts
	-> {"Ok": true, "Files": [{"File": "...", "Callouts": [...]}, ...]}   every file with one
	#+END_EXAMPLE

	A callout is a special block whose name is a callout type or alias
	(=#+begin_tip=), or =#+begin_callout Title :type tip=. =Line= and =EndLine=
	count from 1; =Depth= is how many callouts it sits inside; =Unknown= names a
	type no row of =/callouts/types= has (drawn as a note), which is what
	=orgs callouts check= warns about. Read from the file on disk.
EDOC */
func RequestCallouts(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "callouts") {
		return
	}
	name := r.URL.Query().Get("file")
	if name == "" {
		name = r.URL.Query().Get("filename")
	}
	if name != "" {
		f := GetDb().FindByFile(name)
		if f == nil {
			dynJson(w, map[string]interface{}{"Ok": false, "Msg": "no org file called " + name})
			return
		}
		dynJson(w, map[string]interface{}{"Ok": true, "File": f.Filename, "Callouts": calloutsOfFile(f.Filename)})
		return
	}
	files := []CalloutFile{}
	names := GetDb().GetFiles()
	sort.Strings(names)
	for _, n := range names {
		if cs := calloutsOfFile(n); len(cs) > 0 {
			files = append(files, CalloutFile{File: n, Callouts: cs})
		}
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Files": files})
}
