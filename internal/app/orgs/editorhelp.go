package orgs

// The help page for worg's file editor. The org source is docs/editor.org,
// embedded at build time, so the help ships with the server that has the
// editor's save endpoint and cannot go missing with the working directory.

import (
	"net/http"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/docs"
)

type editorHelpResult struct {
	Ok   bool
	Msg  string
	Html string
}

/* SDOC: API
* GET /editor/help — The File Editor's Keys and Snippets

	The help for worg's Files tab editor (keys, snippets, org commands), from
	the org file embedded in the server at build time, rendered to html.

	*Method:* =GET=

	*Response:* ={"Ok": true, "Html": "..."}=
EDOC */
func RequestEditorHelp(w http.ResponseWriter, r *http.Request) {
	doc := org.New().Parse(strings.NewReader(docs.EditorHelp), "editor.org")
	html, err := doc.Write(org.NewHTMLWriter())
	if err != nil {
		columnJson(w, editorHelpResult{Ok: false, Msg: err.Error()})
		return
	}
	columnJson(w, editorHelpResult{Ok: true, Html: html})
}
