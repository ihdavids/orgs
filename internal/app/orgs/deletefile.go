package orgs

// Deleting org files, from worg's Files tab.
//
// The one write in the server that cannot be undone by writing again, so it
// is not for every account: an administrator may, and so may an account given
// `delete: true` in the keystore (`orgs user candelete <name>` on the server's
// machine). With noAuth on everybody is the one user, as with the AI keys.
//
// Only a file the OrgDb holds and that sits inside an org directory can go,
// so a path from a request cannot reach anything else on the disk. The file is
// forgotten straight away (UnloadFile) rather than left for the watcher, so the
// file list the client asks for next no longer has it.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// mayDeleteFiles is nil when this request may delete org files, else why not.
func mayDeleteFiles(r *http.Request) error {
	if Conf().Server != nil && Conf().Server.NoAuth {
		return nil
	}
	who := GetUsername(r)
	if who == "" || !GetKeystore().MayDelete(who) {
		return fmt.Errorf("%q may not delete files on this server. An administrator can, or an account given "+
			"the permission with  orgs user candelete %s  on the server's machine, followed by a restart", who, who)
	}
	return nil
}

type fileDeleteRequest struct {
	Files []string
}

type fileDeleteResult struct {
	Ok bool
	// Why nothing was deleted, or a summary of what was not.
	Msg string
	// The files that are gone.
	Deleted []string
	// Each file that was not deleted, and why.
	Failed map[string]string
}

/* SDOC: API
* GET /file/delete — May I Delete Files?

	Answers whether the logged-in account may delete org files, so a client
	can offer the action only to somebody who may use it.

	*Method:* =GET=

	*Response:* ={"Ok": true, "CanDelete": true}=, with =Msg= saying why not
	when =CanDelete= is false.

* POST /file/delete — Delete Org Files

	Deletes org files from disk and forgets them. For an administrator, or an
	account marked =delete: true= in the keystore (=orgs user candelete
	<name>=); anyone when =noAuth= is on. Each file must be one the server
	reads and must sit inside one of the =orgDirs=. A file that cannot be
	deleted does not stop the others.

	*Method:* =POST=

	*Request Body (JSON):*
	| Field   | Type     | Required | Description                          |
	|---------+----------+----------+--------------------------------------|
	| =Files= | string[] | yes      | Absolute paths, as =/files= gives them. |

	*Response:* ={"Ok", "Msg", "Deleted": [...], "Failed": {"path": "why"}}=.
	=Ok= is true only when every file was deleted. Refused with =403= when the
	account may not delete.
EDOC */
func RequestMayDeleteFiles(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"Ok": true, "CanDelete": true}
	if err := mayDeleteFiles(r); err != nil {
		out["CanDelete"] = false
		out["Msg"] = err.Error()
	}
	columnJson(w, out)
}

func PostDeleteFiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := mayDeleteFiles(r); err != nil {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(fileDeleteResult{Ok: false, Msg: err.Error()})
		return
	}
	var req fileDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(fileDeleteResult{Ok: false, Msg: err.Error()})
		return
	}
	if len(req.Files) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(fileDeleteResult{Ok: false, Msg: "no files to delete"})
		return
	}
	who := GetUsername(r)
	res := fileDeleteResult{Ok: true, Deleted: []string{}, Failed: map[string]string{}}
	for _, f := range req.Files {
		if err := deleteOrgFile(f); err != nil {
			res.Failed[f] = err.Error()
			continue
		}
		fmt.Fprintf(os.Stderr, "DELETE FILE: %s (by %q)\n", f, who)
		res.Deleted = append(res.Deleted, f)
	}
	if len(res.Failed) > 0 {
		res.Ok = false
		var why []string
		for f, e := range res.Failed {
			why = append(why, f+": "+e)
		}
		res.Msg = strings.Join(why, "; ")
	}
	json.NewEncoder(w).Encode(res)
}

func deleteOrgFile(f string) error {
	db := GetDb()
	// Exactly the path the OrgDb holds, the one /files handed out. FindByFile
	// also answers for a lower-cased or merely similar name, which is fine for
	// reading and not for choosing what to delete.
	of := db.FindByFile(f)
	if f == "" || of == nil || of.Filename != f {
		return fmt.Errorf("not an org file this server reads")
	}
	p, ok := insideOrgDirs(f)
	if !ok || p != f {
		return fmt.Errorf("not inside one of the org directories")
	}
	if !IsOrgFile(p) {
		return fmt.Errorf("not an org file")
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	db.UnloadFile(p)
	return nil
}
