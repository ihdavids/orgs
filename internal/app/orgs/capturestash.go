package orgs

// Where a capture's attachments go before the capture exists.
//
// A picture pasted onto a kanban card, or a recording spoken into one, has a
// heading to be linked from: the file is written, the link is appended to that
// heading's body, and both are done in one call. A capture has no heading yet
// - it is being composed in a dialog - so the same two things have to happen
// in the other order: keep the file now, hand back the link, and let the text
// being typed carry it into the file when the capture is finally made.
//
// The link has to be written relative to the file it will end up in, which is
// the capture template's target - so these take a template name rather than a
// filename. That is the one thing a client composing a capture knows and the
// one thing it cannot work out for itself.

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
)

// The file a capture made with this template would land in, for working out
// what a link inside it should be relative to.
//
// An unknown template, or one whose target cannot be resolved, is not an
// error: the answer is the org root, which is what a link written relative to
// nothing resolves against anyway.
func captureTargetFile(template, username string) string {
	root := "."
	if Conf().Server != nil && len(Conf().Server.OrgDirs) > 0 {
		root = Conf().Server.OrgDirs[0]
	}
	if template == "" {
		return filepath.Join(root, "x.org")
	}
	temp := FindCaptureTemplate(template, username)
	if temp == nil {
		return filepath.Join(root, "x.org")
	}
	// allowCreate is false: asking where a capture *would* go must not make
	// a file somebody has not captured into yet.
	file, _ := GetDb().GetFromTarget(&temp.CapTarget, false)
	if file != nil && file.Doc != nil && file.Doc.Path != "" {
		return file.Doc.Path
	}
	if temp.CapTarget.Filename != "" {
		if filepath.IsAbs(temp.CapTarget.Filename) {
			return temp.CapTarget.Filename
		}
		return filepath.Join(root, temp.CapTarget.Filename)
	}
	return filepath.Join(root, "x.org")
}

// linkRelativeTo is an absolute path written the way an org link inside
// `holder` should be written: relative when it can be, absolute when the two
// are not under one another.
func linkRelativeTo(path, holder string) string {
	if rel, err := filepath.Rel(filepath.Dir(holder), path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

/* SDOC: API
* GET /voice/link — The Org Link For A Recording

	=?id=voice-20260926-142233-ab12.webm&template=BasicEntry= answers with the
	link a capture should write to reach that recording, relative to the file
	the named template would file it in.

	It exists for the capture dialog, which records before it has a heading to
	hang the audio on. =/voice/note= and =/voice/append= work out the same link
	themselves, because they know the file they are writing to; a capture being
	composed does not, and this is how it asks.
EDOC */
func RequestVoiceLink(w http.ResponseWriter, r *http.Request) {
	res := struct {
		Ok   bool   `json:"Ok"`
		Msg  string `json:"Msg"`
		Link string `json:"link"`
		Url  string `json:"url"`
	}{}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	path, err := voicePath(id)
	if err != nil {
		res.Msg = err.Error()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
		return
	}
	holder := captureTargetFile(r.URL.Query().Get("template"), GetUsername(r))
	res.Ok = true
	res.Link = linkRelativeTo(path, holder)
	res.Url = "/voice/recording/" + id
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
