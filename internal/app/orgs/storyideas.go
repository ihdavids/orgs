package orgs

import (
	"net/http"

	"github.com/ihdavids/orgs/internal/common/storygen"
)

// Writing mode's idea generators (internal/common/storygen): people, places,
// cultures, faiths, organisations and story prompts. Nothing is written here;
// the editor puts a result into the draft, a note or the compendium itself.

/* SDOC: API
* GET /writing/ideas — List the Idea Generators
	The generators writing mode's Ideas pane offers (characters, names, relationships,
	settlements, scene settings, cultures, religions, organisations, premises, complications,
	conflicts, openings, objects, weather), each with the options its form takes. Most take
	a =genre=: =fantasy=, =sf=, =modern= or =any=.

	*Method:* =GET=

	*Response:* ={ok, generators: [{id, name, group, summary, count, fields: [{id, label, kind, choices, default}]}]}=
	EDOC */
func RequestStoryIdeas(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	dndJson(w, map[string]interface{}{"ok": true, "generators": storygen.Generators()})
}

/* SDOC: API
* POST /writing/ideas — Make Ideas
	Makes =count= results of one generator. Each is a title, a summary, labelled =facts=,
	the same facts as an org description list (=body=), and =prose=: the idea as a short
	paragraph to put into a draft. =entry= is the compendium kind it would be filed as
	(=character=, =place=, =group=, =thing=), absent for a prompt. The same =seed= gives
	the same results.

	*Method:* =POST=

	*Request Body:* ={type, options: {name: value}, count, seed}=

	*Response:* ={ok, type, seed, items: [{kind, title, summary, facts: [{key, value}], body, prose, entry, tags}]}=
	EDOC */
func PostStoryIdeas(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req storygen.Request
	if !dndBody(w, r, &req) {
		return
	}
	res, err := storygen.Generate(req)
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	dndJson(w, res)
}
