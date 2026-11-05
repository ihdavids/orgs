package orgs

// org-checklist: a repeating heading whose :RESET_CHECK_BOXES: is t has every
// checkbox in its subtree unticked when it comes round again, so a weekly
// checklist starts the next week empty instead of already done. Only with
// features.checklistReset on (features.go); the reset is done inside
// applyStatusChange, as part of the same write that moves the date on.
//
// Also POST /checklist/reset, org's org-reset-checkbox-state-subtree, for
// starting a list again by hand whatever the feature says.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

// wantsCheckboxReset is whether a heading asks for its boxes to be reset on
// repeat: its own :RESET_CHECK_BOXES:, else the file's #+PROPERTY: line.
func wantsCheckboxReset(f *common.OrgFile, props map[string]string) bool {
	if !featureOn("checklistReset") {
		return false
	}
	v, ok := "", false
	for k, pv := range props {
		if strings.EqualFold(k, "RESET_CHECK_BOXES") {
			v, ok = pv, true
		}
	}
	if !ok {
		v, ok = fileProperty(f, "RESET_CHECK_BOXES")
	}
	v = strings.ToLower(strings.TrimSpace(v))
	return ok && v != "" && v != "nil"
}

// resetCheckboxes unticks every box in rows from..to (a heading's subtree)
// and says how many it changed. `[-]` (partly done) counts. Boxes inside a
// source or example block are text, not boxes, and are left alone.
func resetCheckboxes(lines []string, from, to int) int {
	n := 0
	inBlock := false
	for i := from + 1; i <= to && i < len(lines); i++ {
		t := strings.ToLower(strings.TrimSpace(lines[i]))
		if strings.HasPrefix(t, "#+begin_") {
			inBlock = true
			continue
		}
		if strings.HasPrefix(t, "#+end_") {
			inBlock = false
			continue
		}
		if inBlock {
			continue
		}
		m := checkItem(lines[i])
		if m == nil || m[3] == " " {
			continue
		}
		at := strings.Index(lines[i], "["+m[3]+"]")
		if at < 0 {
			continue
		}
		lines[i] = lines[i][:at] + "[ ]" + lines[i][at+3:]
		n++
	}
	return n
}

/* SDOC: API
* POST /checklist/reset — Untick A Heading's Checkboxes

	#+BEGIN_EXAMPLE
	POST /checklist/reset   {"Hash": "..."}
	-> {"Ok": true, "Msg": "3 checkboxes unticked"}
	#+END_EXAMPLE

	org's =org-reset-checkbox-state-subtree=: every checkbox in the heading
	and the headings under it is unticked, and their =[n/m]= cookies brought
	up to date. Works whether or not =features: checklistReset:= is on; that
	setting is about doing this by itself when a heading repeats.
EDOC */
func PostChecklistReset(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct{ Hash string }
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	sec := GetDb().FindByHash(req.Hash)
	if sec == nil || sec.Headline == nil {
		dynJson(w, common.Result{Msg: "no heading with that hash - it may have moved; refresh and try again"})
		return
	}
	f := GetDb().FileFromSection(sec)
	if f == nil {
		dynJson(w, common.Result{Msg: "could not find the file that heading is in"})
		return
	}
	lines, from, end, ok := recordLines(f.Filename, sec)
	if !ok {
		dynJson(w, common.Result{Msg: "could not read " + f.Filename})
		return
	}
	n := resetCheckboxes(lines, from, end)
	if n == 0 {
		dynJson(w, common.Result{Ok: true, Msg: "nothing was ticked"})
		return
	}
	done, active := keywordSets(f)
	for i := end; i >= from; i-- {
		if i < len(lines) && isHeadingLine(lines[i]) {
			lines = refreshCookiesAround(lines, i, done, active)
		}
	}
	if err := writeLines(f.Filename, lines); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	dynJson(w, common.Result{Ok: true, Msg: fmt.Sprintf("%d checkbox%s unticked", n, map[bool]string{true: "", false: "es"}[n == 1])})
}
