package orgs

// Saving a whole org file from worg's editor.
//
// Every other write in the server splices the lines it changes, because
// writing a parsed document back reformats the whole file. This one is
// different in kind rather than an exception to that rule: the text is what
// somebody typed, so it is written exactly as it arrived and nothing is parsed
// on the way.
//
// What it does guard against is the file having moved underneath the editor.
// The editor sends the text it started from as Base; if the file on disk is no
// longer that, something else wrote it (a capture, a kanban drag, Emacs) and
// saving would silently throw that away. It refuses instead and says so, and
// the editor offers to overwrite, which sends Force.

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/ihdavids/orgs/internal/common"
)

type orgFileSave struct {
	Filename string
	Text     string
	Base     string
	Force    bool
}

/* SDOC: API
* POST /orgfile — Save an Org File's Text

	Writes the whole text of an org file, exactly as given. Used by worg's
	editor. Only a file the server already reads can be written, and the write
	is refused when the file on disk is no longer =Base= (the text the editor
	started from), unless =Force= is set.

	*Method:* =POST=

	*Request Body (JSON):*
	| Field      | Type   | Required | Description                                        |
	|------------+--------+----------+----------------------------------------------------|
	| =Filename= | string | yes      | The org file to write.                             |
	| =Text=     | string | yes      | The new contents.                                  |
	| =Base=     | string | no       | What the file held when editing began.             |
	| =Force=    | bool   | no       | Write even when the file has changed since =Base=. |

	*Response:* A =ResultMsg=. A refusal because the file changed has
	=Msg= set to ="changed"=, so a client can tell it from a failure.
EDOC */
func PostOrgFile(w http.ResponseWriter, r *http.Request) {
	var req orgFileSave
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		columnJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	// The read endpoint takes any path; the write one must not, or it is a way
	// to write anywhere the server can.
	if req.Filename == "" || GetDb().FindByFile(req.Filename) == nil {
		columnJson(w, common.ResultMsg{Ok: false, Msg: "no org file called "+req.Filename})
		return
	}
	info, err := os.Stat(req.Filename)
	if err != nil {
		columnJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	if !req.Force {
		now, err := os.ReadFile(req.Filename)
		if err != nil {
			columnJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
			return
		}
		if string(now) != req.Base {
			columnJson(w, common.ResultMsg{Ok: false, Msg: "changed"})
			return
		}
	}
	if err := os.WriteFile(req.Filename, []byte(req.Text), info.Mode().Perm()); err != nil {
		columnJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	columnJson(w, common.ResultMsg{Ok: true})
}
