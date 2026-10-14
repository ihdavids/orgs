package orgs

// POST /gantt/link: one dependency written or taken away on the server.
//
// worg draws an arrow with two /property writes (an ID on the heading waited
// on, then AFTER on the waiter; ganttops.ts, dependencyOp). An agent applying
// twenty suggestions should not have to know that recipe, nor how an ORDERED
// parent chains a heading behind its sibling, so this does both halves in one
// request, with the dependency rules of insights.go (buildDepGraph): AFTER
// names one heading, by hash, ID or CUSTOM_ID exactly as written, so a second
// dependency cannot join the first and is refused rather than written over it.
//
// Both writes are setHeadingProperty line edits; a property drawer is not part
// of any title, so no hash moves, but the second heading is looked up again
// after the first write all the same (Traps: re-measure).

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

var ganttIdSlugRe = regexp.MustCompile(`[^a-z0-9]+`)
var ganttIdTailRe = regexp.MustCompile(`[^a-zA-Z0-9]`)

// ganttIdFor is a readable id for a heading that has none: its own words, plus
// enough of its hash to keep two headings with the same name apart. The same
// recipe as idFor in worg's ganttops.ts (see Traps: logic said twice).
func ganttIdFor(title, hash string) string {
	slug := strings.Trim(ganttIdSlugRe.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(slug) > 24 {
		slug = slug[:24]
	}
	if slug == "" {
		slug = "task"
	}
	tail := ganttIdTailRe.ReplaceAllString(hash, "")
	if len(tail) > 6 {
		tail = tail[:6]
	}
	tail = strings.ToLower(tail)
	if tail == "" {
		return slug
	}
	return slug + "-" + tail
}

func ganttLink(req common.GanttLinkReq) (common.ResultMsg, error) {
	res := common.ResultMsg{}
	if strings.TrimSpace(req.Task) == "" || strings.TrimSpace(req.After) == "" {
		return res, fmt.Errorf("gantt link: need the Task and the After it waits on, both by hash")
	}
	g := buildDepGraph()
	tn, an := g.byKey[req.Task], g.byKey[req.After]
	if tn == nil {
		return res, fmt.Errorf("gantt link: no heading with the Task hash")
	}
	if an == nil {
		return res, fmt.Errorf("gantt link: no heading with the After hash")
	}
	tTitle, aTitle := insightTitle(tn.sec), insightTitle(an.sec)
	cur := g.edges[tn]

	if req.Remove {
		if cur.to != an {
			res.Ok = true
			res.Msg = fmt.Sprintf("%s does not wait on %s; nothing to take away", tTitle, aTitle)
			return res, nil
		}
		sec := GetDb().FindByHash(tn.sec.Hash)
		if sec == nil || sec.Headline == nil {
			return res, fmt.Errorf("gantt link: no heading with the Task hash")
		}
		f := GetDb().FileFromSection(sec)
		msg := fmt.Sprintf("%s no longer waits on %s", tTitle, aTitle)
		noBlock := cur.via == "ORDERED"
		if cur.via == "AFTER" {
			if err := setHeadingProperty(f, sec, "AFTER", ""); err != nil {
				return res, err
			}
			// Clearing AFTER hands it back to its ORDERED chain; if that chain
			// waits on the same heading, it has to come out of it too.
			if ch := g.chained(tn, map[*depNode]bool{}); ch.to == an {
				noBlock = true
			} else if ch.to != nil {
				msg += fmt.Sprintf("; its ORDERED parent now puts it after %s", insightTitle(ch.to.sec))
			}
		}
		if noBlock {
			sec = GetDb().FindByHash(tn.sec.Hash)
			if sec == nil || sec.Headline == nil {
				return res, fmt.Errorf("gantt link: lost the Task heading after writing it")
			}
			if err := setHeadingProperty(GetDb().FileFromSection(sec), sec, "NOBLOCKING", "t"); err != nil {
				return res, err
			}
			msg += " (NOBLOCKING takes it out of its ORDERED chain)"
		}
		res.Ok = true
		res.Msg = msg
		return res, nil
	}

	if tn == an {
		return res, fmt.Errorf("gantt link: a heading cannot wait on itself")
	}
	if g.isAncestor(an, tn) {
		return res, fmt.Errorf("gantt link: %s is part of %s, which cannot finish before it does", tTitle, aTitle)
	}
	seen := map[*depNode]bool{}
	for n := an; n != nil && !seen[n]; n = g.edges[n].to {
		if n == tn {
			return res, fmt.Errorf("gantt link: %s already waits (through its chain) on %s; this would make a circle", aTitle, tTitle)
		}
		seen[n] = true
	}
	if cur.to == an {
		res.Ok = true
		res.Msg = fmt.Sprintf("%s already waits on %s", tTitle, aTitle)
		return res, nil
	}
	// AFTER holds one heading. Writing over one that names another would
	// drop a dependency nobody asked to drop, so it is refused; an ORDERED
	// chain is not written anywhere on the task, and AFTER is how a step is
	// taken out of it, so that is allowed and said.
	if tn.after != "" {
		was := tn.after
		if cur.to != nil {
			was = fmt.Sprintf("%s (AFTER %s)", insightTitle(cur.to.sec), tn.after)
		} else {
			was = fmt.Sprintf("AFTER %s, which names no heading", tn.after)
		}
		return res, fmt.Errorf("gantt link: %s already waits on %s, and AFTER holds one heading: remove that first", tTitle, was)
	}

	notes := []string{}
	id := insightProp(an.sec, "ID")
	if id == "" {
		id = insightProp(an.sec, "CUSTOM_ID")
	}
	if id == "" {
		base := ganttIdFor(aTitle, an.sec.Hash)
		id = base
		for i := 2; g.byKey[id] != nil && g.byKey[id] != an; i++ {
			id = fmt.Sprintf("%s-%d", base, i)
		}
		asec := GetDb().FindByHash(an.sec.Hash)
		if asec == nil || asec.Headline == nil {
			return res, fmt.Errorf("gantt link: no heading with the After hash")
		}
		if err := setHeadingProperty(GetDb().FileFromSection(asec), asec, "ID", id); err != nil {
			return res, err
		}
		notes = append(notes, fmt.Sprintf("gave %s the ID %s", aTitle, id))
	}
	// Found again: the ID drawer may have moved it down a line or two.
	sec := GetDb().FindByHash(tn.sec.Hash)
	if sec == nil || sec.Headline == nil {
		return res, fmt.Errorf("gantt link: lost the Task heading after writing the ID")
	}
	if err := setHeadingProperty(GetDb().FileFromSection(sec), sec, "AFTER", id); err != nil {
		return res, err
	}
	if cur.to != nil {
		notes = append(notes, fmt.Sprintf("instead of %s, which its ORDERED parent put it after", insightTitle(cur.to.sec)))
	}
	res.Ok = true
	res.Msg = fmt.Sprintf("%s now comes after %s", tTitle, aTitle)
	if len(notes) > 0 {
		res.Msg += " (" + strings.Join(notes, "; ") + ")"
	}
	return res, nil
}

/* SDOC: API
* POST /gantt/link — Make One Task Wait on Another
	Writes the dependency a gantt arrow stands for: =AFTER= on =Task=, naming
	=After= by its =ID= (or =CUSTOM_ID=). A heading with neither is given an
	=ID= first, made from its title and hash the way worg makes one. An
	ORDERED parent's chain is overridden (that is what AFTER on a step does),
	and the answer says so.

	Refused: a heading waiting on itself, on a heading it is part of, or on
	one that already waits (through any chain) on it; and a task whose AFTER
	already names another heading, because AFTER holds one: remove that
	first.

	With =Remove=, =Task= stops waiting on =After=: an AFTER naming it is
	cleared, and when the wait comes from an ORDERED parent (or clearing the
	AFTER would hand it straight back to such a chain) =NOBLOCKING: t= takes
	it out of the chain. Nothing is written when it did not wait on =After=.

	Both are line edits of the property drawers; no hash changes.

	*Method:* =POST=

	*Request Body:* A =GanttLinkReq=.
	| Field    | Type   | Description                                  |
	|----------+--------+----------------------------------------------|
	| =Task=   | string | Hash of the heading that waits.              |
	| =After=  | string | Hash of the heading it waits on.             |
	| =Remove= | bool   | Take the dependency away instead of adding.  |

	*Response:* A =ResultMsg=: =Ok=, and =Msg= saying what was written.
	EDOC */
func PostGanttLink(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.GanttLinkReq
	if err := json.Unmarshal(body, &req); err != nil {
		answerErr(w, err)
		return
	}
	res, err := ganttLink(req)
	if err != nil {
		res.Ok = false
		res.Msg = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
