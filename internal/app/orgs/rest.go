//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	b64 "encoding/base64"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/tangle"
	"github.com/ihdavids/orgs/internal/common"

	//"log"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"math/rand"
	"time"
)

func RestApi(router *mux.Router) {
	router.Use(loggingMiddleware)

	// Public routes (no auth required)
	router.HandleFunc("/login", login).Methods("POST")

	// Protected routes - all endpoints below require a valid JWS token
	api := router.PathPrefix("").Subrouter()
	if !Conf().Server.NoAuth {
		api.Use(authenticate)
	} else {
		fmt.Println("WARNING: Authentication is disabled (noAuth: true)")
	}

	api.HandleFunc("/refresh", refresh).Methods("POST")
	api.HandleFunc("/orgfile", RequestOrgFile)
	api.HandleFunc("/findfile", RequestFindFileInDb)
	api.HandleFunc("/files", RequestFiles)
	api.HandleFunc("/file", CreateFile).Methods("POST")
	api.HandleFunc("/dirs", RequestDirs)
	api.HandleFunc("/newtemplates", RequestNewTemplates)
	api.HandleFunc("/file/{type}", RequestFile)               // html etc
	api.HandleFunc("/filecontents/headings", RequestHeadings) // Get all todos in file
	api.HandleFunc("/filters", RequestFilters)                // Get all stored filters from the server
	api.HandleFunc("/taggroups", RequestTagGroups)
	api.HandleFunc("/grep", RequestGrep)
	api.HandleFunc("/search", RequestTodosExpr)
	api.HandleFunc("/lookuphash", RequestHash)
	api.HandleFunc("/todohtml/{hash}", RequestFullTodoHtml)
	api.HandleFunc("/logbook/{hash}", RequestLogbook)
	api.HandleFunc("/filehtml/{hash}", RequestFullFileHtml)
	api.HandleFunc("/todofull/{hash}", RequestFullTodo)
	api.HandleFunc("/hash/{hash}", RequestByHash)
	api.HandleFunc("/next/{hash}", RequestNextSibling)
	api.HandleFunc("/prev/{hash}", RequestPrevSibling)
	api.HandleFunc("/child/{hash}", RequestLastChild)
	api.HandleFunc("/id/{id}", RequestByAnyId)
	api.HandleFunc("/daypage/increment", RequestDayPageIncrement).Methods("GET")
	api.HandleFunc("/daypage/{date}", RequestDayPageAt).Methods("GET")
	api.HandleFunc("/daypage", PostCreateDayPage).Methods("POST")
	api.HandleFunc("/status/change", PostChangeStatus).Methods("POST")
	api.HandleFunc("/headline/change", PostRenameHeadline).Methods("POST")
	api.HandleFunc("/body/change", PostChangeBody).Methods("POST")
	api.HandleFunc("/status/{hash}", RequestValidStatus)
	api.HandleFunc("/date/change", PostChangeDate).Methods("POST")
	api.HandleFunc("/date/change", DeleteDate).Methods("DELETE")
	api.HandleFunc("/property", PostChangeProperty).Methods("POST")
	api.HandleFunc("/alltags", RequestTags)
	api.HandleFunc("/tags", PostToggleTags).Methods("POST")
	api.HandleFunc("/capture", PostCapture).Methods("POST")
	api.HandleFunc("/capture/templates", RequestCaptureTemplates)
	api.HandleFunc("/delete", PostDelete).Methods("POST")
	api.HandleFunc("/refilefiles", RequestRefileTargets)
	api.HandleFunc("/refile", PostRefile).Methods("POST")
	api.HandleFunc("/archive", PostArchive).Methods("POST")
	api.HandleFunc("/reformat", PostReformat).Methods("POST")
	api.HandleFunc("/setexclusivemarker", PostMarker).Methods("POST")
	api.HandleFunc("/exclusivemarker", RequestMarker)
	api.HandleFunc("/update", PostUpdate).Methods("POST")
	api.HandleFunc("/clockin", PostClockIn).Methods("POST")
	api.HandleFunc("/clockout", PostClockOut).Methods("POST")
	api.HandleFunc("/clock", RequestClock)
	api.HandleFunc("/clockreport", RequestClockReport)
	api.HandleFunc("/execb", PostExecb).Methods("POST")
	api.HandleFunc("/exectable", PostExect).Methods("POST")
	api.HandleFunc("/execalltables", PostExecAllT).Methods("POST")
	api.HandleFunc("/tableformulainfo", PostFormulaInfo).Methods("POST")
	api.HandleFunc("/tablerandomget", RequestTableRandomGet)
	api.HandleFunc("/tablenames", RequestTableNames)
	api.HandleFunc("/tangle", RequestTangle)

	// Per-user extensions: stored queries
	api.HandleFunc("/ext/queries", RequestStoredQueries).Methods("GET")
	api.HandleFunc("/ext/query", RequestStoredQuery).Methods("GET")
	api.HandleFunc("/ext/query", PostStoredQuery).Methods("POST")
	api.HandleFunc("/ext/query", DeleteStoredQuery).Methods("DELETE")

	// Per-user extensions: capture templates
	api.HandleFunc("/ext/capture/templates", RequestUserCaptureTemplates).Methods("GET")
	api.HandleFunc("/ext/capture/template", PostUserCaptureTemplate).Methods("POST")
	api.HandleFunc("/ext/capture/template", DeleteUserCaptureTemplate).Methods("DELETE")

}

type MiddlewareFunc func(http.Handler) http.Handler

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Do stuff here
		log.Println("RQST: ", r.RequestURI)
		// Call the next handler, which can be another middleware in the chain, or the final handler.
		next.ServeHTTP(w, r)
	})
}

func AccessControl(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
}

// Extract a hash parameter from an incomming URL, they have to be base64 encoded
// as the hash could have slashes or other non URL friendly characters
func GetHash(vars map[string]string, name string) (string, error) {
	if h, err := b64.URLEncoding.DecodeString(vars[name]); err == nil {
		return string(h), err
	} else {
		return "", err
	}
}

/* SDOC: API
* GET /files — List All Org Files
	Returns a JSON array of absolute file paths for every org file the server is currently tracking.
	These are the files discovered in the configured =orgDirs= directories. The list updates as the
	server's file watcher detects new or removed files.

	*Method:* =GET=

	*Parameters:* None.

	*Response:* A JSON array of strings, each being an absolute path to an org file.
	#+BEGIN_SRC json
	["/home/user/org/todo.org", "/home/user/org/notes.org"]
	#+END_SRC
	EDOC */
func RequestFiles(w http.ResponseWriter, r *http.Request) {
	//vars := mux.Vars(r)
	//key := vars["id"]
	res := GetDb().GetFiles()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

/* SDOC: API
* GET /findfile — Locate a File in the Database
	Searches the server's in-memory file database for a file matching the given filename.
	Useful for resolving a short or relative filename into the full absolute path that the
	server knows about. If the file is not tracked by the server, an error is returned.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                                         |
	|------------+--------+----------+---------------------------------------------------------------------|
	| =filename= | string | yes      | The filename to search for. Can be a basename or a partial path.    |

	*Response:* A =ResultMsg= JSON object.
	- On success: ={"status": true, "msg": "/absolute/path/to/file.org"}=
	- On failure: ={"status": false, "msg": "error description"}=
	EDOC */
func RequestFindFileInDb(w http.ResponseWriter, r *http.Request) {
	fname := r.URL.Query().Get("filename")
	if res, err := FindFileInDb(fname); err != nil {
		w.Header().Set("Content-Type", "application/json")
		msg := common.ResultMsg{Ok: false, Msg: err.Error()}
		json.NewEncoder(w).Encode(msg)
	} else {
		w.Header().Set("Content-Type", "application/json")
		msg := common.ResultMsg{Ok: true, Msg: res}
		json.NewEncoder(w).Encode(msg)
	}
}

/* SDOC: API
* GET /grep — Search Across All Org Files
	Performs a regular-expression search across the raw text of every org file the server
	is tracking. Results are returned as an array of strings, each containing the filename,
	line number, and matched line separated by the chosen delimiter (default =:=).

	*Method:* =GET=

	*Query Parameters:*
	| Parameter    | Type   | Required | Description                                                            |
	|--------------+--------+----------+------------------------------------------------------------------------|
	| =query=      | string | yes      | The regular expression to search for.                                  |
	| =delimeter=  | string | no       | Separator between filename, line number, and content. Defaults to =:=. |

	*Response:* A JSON array of match strings.
	#+BEGIN_SRC json
	["todo.org:12:TODO Buy groceries", "notes.org:45:Meeting with team"]
	#+END_SRC
	On error, returns an empty array.
	EDOC */
func RequestGrep(w http.ResponseWriter, r *http.Request) {
	qry := r.URL.Query().Get("query")
	del := r.URL.Query().Get("delimeter")
	if del == "" {
		del = ":"
	}
	w.Header().Set("Content-Type", "application/json")
	if res, err := Grep(qry, del); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		json.NewEncoder(w).Encode([]string{})
	} else {
		json.NewEncoder(w).Encode(res)
	}
}

/* SDOC: API
* GET /orgfile — Read Raw Org File Contents
	Reads the raw text content of an org file from disk and returns it as a string.
	This returns the file contents verbatim (not parsed), which is useful for editors
	that need the original source text.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                       |
	|------------+--------+----------+---------------------------------------------------|
	| =filename= | string | yes      | Absolute path to the org file to read.             |

	*Response:* A =ResultMsg= JSON object.
	- On success: ={"status": true, "msg": "...raw file contents..."}=
	- On failure: ={"status": false, "msg": "error description"}=
	EDOC */
func RequestOrgFile(w http.ResponseWriter, r *http.Request) {
	//vars := mux.Vars(r)
	filename := r.URL.Query().Get("filename")
	w.Header().Set("Content-Type", "application/json")
	if f, err := ioutil.ReadFile(filename); err == nil {
		msg := common.ResultMsg{Ok: true, Msg: string(f)}
		json.NewEncoder(w).Encode(msg)
	} else {
		res := fmt.Sprintf("[RequestOrgFile] error opening file: %v", err)
		log.Printf(res)
		msg := common.ResultMsg{Ok: false, Msg: res}
		json.NewEncoder(w).Encode(msg)
	}
}

/* SDOC: API
* GET /filecontents/headings — List All Headings in a File
	Returns every heading (TODO item or otherwise) found in the specified org file.
	The result is an array of =Todo= objects containing the headline text, status,
	priority, tags, filename, line position, hash, and other metadata.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                      |
	|------------+--------+----------+--------------------------------------------------|
	| =filename= | string | yes      | The filename (basename or path) of the org file.  |

	*Response:* A JSON array of =Todo= objects.
	EDOC */
func RequestHeadings(w http.ResponseWriter, r *http.Request) {
	//vars := mux.Vars(r)
	fname := r.URL.Query().Get("filename")
	res, _ := GetAllTodosInFile(fname)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

/* SDOC: API
* GET /taggroups — List Tag Groups
	Returns the tag groups configured in the server's YAML config. Tag groups are named
	sets of tags that can be referenced in queries using handlebars syntax (e.g. ={{ WORK }}).
	A default set is seeded automatically unless =noInternalTagGroups= is set in config.

	*Method:* =GET=

	*Parameters:* None.

	*Response:* A JSON object mapping group names to their tag expressions.
	EDOC */
func RequestTagGroups(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Conf().TagGroups)
}

/* SDOC: API
* GET /filters — List Filters
	Returns the named filters configured in the server's YAML config. Filters are reusable
	query fragments that can be substituted into search expressions using handlebars syntax
	(e.g. ={{ AllTasks }}=). A default set (=AllTasks=, =HomeTasks=, =WorkTasks=, etc.) is
	seeded unless =noInternalFilters= is set.

	*Method:* =GET=

	*Parameters:* None.

	*Response:* A JSON object mapping filter names to their query expressions.
	EDOC */
func RequestFilters(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Conf().Filters)
}

/* SDOC: API
* GET /file/{type} — Export a File via an Exporter Plugin
	Runs the named exporter plugin against an org file and returns the exported result.
	The ={type}= path segment selects the exporter (e.g. =html=, =latex=, =revealjs=,
	=impressjs=, =gantt=, =mermaid=, =tangle=, etc.). The exporter must be enabled in the
	server's config under =server.exporters=.

	By default, the exported content is returned as a string in the response body.
	When =local=t= is set, the exporter writes to the file specified by =filename=
	instead and the response reports success/failure.

	*Method:* =GET=

	*Path Parameters:*
	| Parameter | Type   | Description                                            |
	|-----------+--------+--------------------------------------------------------|
	| ={type}=  | string | Name of the exporter plugin to use (e.g. =html=).     |

	*Query Parameters:*
	| Parameter      | Type   | Required | Description                                                                        |
	|----------------+--------+----------+------------------------------------------------------------------------------------|
	| =filename=     | string | no       | Output filename when =local=t=. Ignored otherwise.                                 |
	| =query=        | string | yes      | The org file to export (basename or path).                                         |
	| =local=        | string | no       | Set to =t= to write the result to =filename= on disk instead of returning it.     |
	| =filelinks=    | string | no       | Set to =t= to include file-style links in the export.                              |
	| =httpslinks=   | string | no       | Set to =t= to convert links to https-style links.                                  |
	| =parent=       | string | no       | A parent property passed to the exporter (exporter-specific).                      |

	*Response:* A =ResultMsg= JSON object.
	- When =local= is not set: ={"status": true, "msg": "...exported content..."}=
	- When =local=t=: ={"status": true, "msg": "Success"}= or ={"status": false, "msg": "error"}=
	EDOC */
func RequestFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ptype := vars["type"]
	fname := r.URL.Query().Get("filename")
	query := r.URL.Query().Get("query")
	local := r.URL.Query().Get("local")
	filelinks := r.URL.Query().Get("filelinks")
	httpslinks := r.URL.Query().Get("httpslinks")
	props := map[string]string{}
	props["parent"] = r.URL.Query().Get("parent")
	opts := common.ExportToFile{Name: ptype, Filename: fname, Query: query, Opts: "", Props: props}
	if filelinks == "t" {
		opts.Opts += "filelinks;"
	}
	if httpslinks == "t" {
		opts.Opts += "httpslinks;"
	}
	var res common.ResultMsg
	if local == "t" {
		res, _ = ExportToFile(db, &opts)
	} else {
		res, _ = ExportToString(db, &opts)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

/* SDOC: API
* GET /tangle — Tangle Source Blocks from an Org File
	Extracts source code blocks from an org file following Org mode tangle conventions.
	Source blocks with a =:tangle= header argument are collected, grouped by target file,
	and their content is assembled. Noweb references (=<<name>>=) are expanded when
	=:noweb yes= is set on a block.

	By default, the assembled content is returned in the JSON response *without* writing
	any files to disk. Set =write=t= to also write the tangled files.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                                                  |
	|------------+--------+----------+------------------------------------------------------------------------------|
	| =filename= | string | yes      | The org file to tangle (basename or path).                                   |
	| =write=    | string | no       | Set to =t= to write tangled output files to disk in addition to returning.   |

	*Response:* A JSON object containing an array of tangled files, each with the assembled content:
	#+BEGIN_SRC json
	{
	  "files": [
	    {
	      "filename": "/path/to/output.py",
	      "content": "#!/usr/bin/env python\nprint('hello')\n",
	      "lang": "python",
	      "lines": 2
	    }
	  ]
	}
	#+END_SRC

	*Supported Block Header Arguments:*
	| Header Arg     | Description                                                                    |
	|----------------+--------------------------------------------------------------------------------|
	| =:tangle=      | Output file path (relative to the org file directory, or absolute). =no= to skip. |
	| =:noweb=       | Set to =yes= to expand =<<name>>= references in the block.                    |
	| =:noweb-ref=   | Defines this block as a reusable fragment that others can reference.            |
	| =:noweb-sep=   | Separator when concatenating multiple blocks with the same =:noweb-ref=.       |
	| =:mkdirp=      | Set to =yes= to create parent directories if they do not exist.                |
	| =:padline=     | Set to =no= to suppress the blank line inserted between blocks.                |
	| =:shebang=     | A shebang line (e.g. =#!/bin/bash=) prepended to the output file.              |
	| =:tangle-mode= | File permissions, e.g. =(identity #o755)= or =0644=.                           |
	| =:comments=    | Set to =link= or =both= to insert source-link comments.                       |

	*Errors:*
	- =400= if =filename= is missing.
	- =500= if the file is not found or tangling fails.
	EDOC */
func RequestTangle(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("filename")
	if query == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "filename parameter required"})
		return
	}
	writeToDisk := r.URL.Query().Get("write") == "t"
	result, err := tangle.Tangle(db, query, writeToDisk)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

/* SDOC: API
* GET /refilefiles — List Valid Refile Targets
	Returns a list of headings across all org files that are valid targets for refiling.
	The set of files considered is controlled by the =refileTargets= config setting,
	which accepts a list of filename regex patterns (defaults to =.*\\.org=).

	*Method:* =GET=

	*Parameters:* None.

	*Response:* A JSON array of refile target objects, each describing a heading that
	can receive refiled content.
	EDOC */
func RequestRefileTargets(w http.ResponseWriter, r *http.Request) {
	//vars := mux.Vars(r)
	//ptype := vars["type"]
	//fname := r.URL.Query().Get("filename")
	//query := r.URL.Query().Get("query")
	//local := r.URL.Query().Get("local")
	targets := GetRefileTargetsList([]string{})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(targets)
}

/* SDOC: API
* GET /filehtml/{hash} — Render Full File as HTML
	Given the base64-URL-encoded hash of any heading in a file, renders the entire
	containing org file as HTML and returns it. Useful for previewing a full document
	when you only have a reference to one of its headings.

	*Method:* =GET=

	*Path Parameters:*
	| Parameter | Type   | Description                                                          |
	|-----------+--------+----------------------------------------------------------------------|
	| ={hash}=  | string | Base64-URL-encoded hash of a heading in the target file.             |

	*Response:* The rendered HTML string, or an error object on failure.
	EDOC */
func RequestFullFileHtml(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var hash common.TodoHash = common.TodoHash(h)
		reply, err := QueryFullFileHtml(&hash)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			json.NewEncoder(w).Encode(err)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* GET /todohtml/{hash} — Render a Single Heading as HTML
	Renders the heading identified by the given hash as HTML. Only the subtree
	rooted at that heading is rendered, not the full file.

	*Method:* =GET=

	*Path Parameters:*
	| Parameter | Type   | Description                                              |
	|-----------+--------+----------------------------------------------------------|
	| ={hash}=  | string | Base64-URL-encoded hash of the heading to render.        |

	*Response:* The rendered HTML string, or an error object on failure.
	EDOC */
func RequestFullTodoHtml(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var hash common.TodoHash = common.TodoHash(h)
		reply, err := QueryFullTodoHtml(&hash)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			json.NewEncoder(w).Encode(err)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* GET /todofull/{hash} — Get Full Heading Data
	Returns the complete =Todo= data for the heading identified by the given hash.
	This includes the headline text, status, priority, tags, properties, scheduling
	timestamps, filename, position, and all other metadata the server tracks.

	*Method:* =GET=

	*Path Parameters:*
	| Parameter | Type   | Description                                              |
	|-----------+--------+----------------------------------------------------------|
	| ={hash}=  | string | Base64-URL-encoded hash of the heading.                  |

	*Response:* A full =Todo= JSON object, or an error on failure.
	EDOC */
func RequestFullTodo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var hash common.TodoHash = common.TodoHash(string(h))
		reply, err := QueryFullTodo(&hash)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Println("ERROR during request", err)
			json.NewEncoder(w).Encode(err)
		}
	} else {
		fmt.Println("ERROR getting hash value", err)
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* POST /file — Create a New Org File
	Creates a new org file on disk, optionally from a template, and adds it to the server's
	file database. If no title is given, the filename (without extension) is used as the title.
	Templates are looked up from the =new/= subdirectory of the configured =templatePath=.

	*Method:* =POST=

	*Request Body (JSON):*
	| Field      | Type   | Required | Description                                                      |
	|------------+--------+----------+------------------------------------------------------------------|
	| =filename= | string | yes      | Absolute path where the new org file should be created.           |
	| =title=    | string | no       | Title for the file. Defaults to the basename without extension.   |
	| =template= | string | no       | Template name (e.g. =new/journal.tpl=) to use for initial content.|

	*Response:*
	- On success (=200=): A JSON array containing the new file's absolute path.
	- On failure (=400= / =500=): A =ResultMsg= with =status: false=.
	EDOC */
func CreateFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	body, _ := io.ReadAll(r.Body)
	var req common.NewFileRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: fmt.Sprintf("Invalid request: %s", err)})
		return
	}
	if req.Filename == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: "Filename is required"})
		return
	}
	title := req.Title
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(req.Filename), filepath.Ext(req.Filename))
	}
	file := GetDb().CreateOrgFileFromTemplate(req.Filename, title, req.Template)
	if file != nil {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(common.FileList{file.Filename})
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: "Failed to create file"})
	}
}

/* SDOC: API
* GET /dirs — List All Known Directories
	Returns a deduplicated list of directories that contain org files. This includes
	the configured =orgDirs= plus the parent directory of every individual file the
	server is tracking. Useful for file-browser UIs that need to know where org content lives.

	*Method:* =GET=

	*Parameters:* None.

	*Response:* A JSON array of absolute directory paths.
	EDOC */
func RequestDirs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	dirSet := map[string]bool{}
	files := GetDb().GetFiles()
	for _, f := range files {
		dirSet[filepath.Dir(f)] = true
	}
	for _, d := range Conf().Server.OrgDirs {
		dirSet[d] = true
	}
	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	json.NewEncoder(w).Encode(dirs)
}

/* SDOC: API
* GET /newtemplates — List New-File Templates
	Returns the list of available templates that can be used when creating a new org file
	via =POST /file=. Templates are =.tpl= files found in the =new/= subdirectory of the
	configured =templatePath=.

	*Method:* =GET=

	*Parameters:* None.

	*Response:* A JSON array of template names (e.g. =["new/journal.tpl", "new/project.tpl"]=).
	Returns an empty array if no templates are found.
	EDOC */
func RequestNewTemplates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	newDir := filepath.Join(Conf().Server.TemplatePath, "new")
	var templates []string
	entries, err := os.ReadDir(newDir)
	if err != nil {
		json.NewEncoder(w).Encode(templates)
		return
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".tpl") {
			templates = append(templates, "new/"+e.Name())
		}
	}
	json.NewEncoder(w).Encode(templates)
}

/* SDOC: API
* POST /status/change — Change Heading Status
	Changes the TODO/DONE status keyword of a heading identified by its hash.
	The new status value must be a valid keyword for the file (as defined by the
	file's =#+TODO= line or the server's =defaultTodoStates= / =defaultNextStates=).

	*Method:* =POST=

	*Request Body (JSON):*
	| Field   | Type   | Required | Description                                                  |
	|---------+--------+----------+--------------------------------------------------------------|
	| =Hash=  | string | yes      | The dynamic hash identifying the heading.                    |
	| =Value= | string | yes      | The new status keyword (e.g. =DONE=, =TODO=, =NEXT=, =""=). |

	*Response:* A =Result= JSON object with ={"status": true}= on success.
	EDOC */
func PostChangeStatus(w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	var args common.TodoItemChange
	var err = json.Unmarshal(body, &args)
	if err == nil {
		var reply common.Result
		reply, err = ChangeStatus(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			json.NewEncoder(w).Encode(err)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* POST /headline/change — Rename a Heading
	Changes the headline text of a heading identified by its hash. The new text
	replaces the headline title (the part after the status keyword and priority).

	*Method:* =POST=

	*Request Body (JSON):*
	| Field   | Type   | Required | Description                              |
	|---------+--------+----------+------------------------------------------|
	| =Hash=  | string | yes      | The dynamic hash identifying the heading.|
	| =Value= | string | yes      | The new headline text.                   |

	*Response:* A =Result= JSON object with ={"status": true}= on success.
	EDOC */
func PostRenameHeadline(w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	var args common.TodoItemChange
	var err = json.Unmarshal(body, &args)
	if err == nil {
		var reply common.Result
		reply, err = RenameHeadline(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			json.NewEncoder(w).Encode(err)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* POST /body/change — Replace Heading Body Content
	Replaces the body content (everything below the headline, before the next heading)
	of a heading identified by its hash. The value should be raw org-mode text.

	*Method:* =POST=

	*Request Body (JSON):*
	| Field   | Type   | Required | Description                                     |
	|---------+--------+----------+-------------------------------------------------|
	| =Hash=  | string | yes      | The dynamic hash identifying the heading.        |
	| =Value= | string | yes      | The new body content (raw org-mode text).        |

	*Response:* A =Result= JSON object with ={"status": true}= on success.
	EDOC */
func PostChangeBody(w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	var args common.TodoItemChange
	var err = json.Unmarshal(body, &args)
	if err == nil {
		var reply common.Result
		reply, err = ChangeBody(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			json.NewEncoder(w).Encode(err)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* POST /date/change — Set or Change a Date on a Heading
	Sets or updates a scheduling timestamp on a heading. Supports =SCHEDULED=, =DEADLINE=,
	=CLOSED=, and =TIMESTAMP= date types. Pass an empty =Value= to clear the date.

	*Method:* =POST=

	*Request Body (JSON):*
	| Field   | Type   | Required | Description                                                                   |
	|---------+--------+----------+-------------------------------------------------------------------------------|
	| =Hash=  | string | yes      | The dynamic hash identifying the heading.                                     |
	| =Name=  | string | yes      | The date type: =SCHEDULED=, =DEADLINE=, =CLOSED=, or =TIMESTAMP=.            |
	| =Value= | string | yes      | Org date string (e.g. =<2024-01-15 Mon>=) or =""= to clear.                  |

	*Response:* A =Result= JSON object with ={"status": true}= on success.
	EDOC */
func PostChangeDate(w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	var args common.TodoDateChange
	var err = json.Unmarshal(body, &args)
	if err == nil {
		var reply common.Result
		reply, err = ChangeDate(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			json.NewEncoder(w).Encode(err)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* DELETE /date/change — Remove a Date from a Heading
	Removes a scheduling timestamp from a heading. The =Value= field in the request
	body is ignored; the date identified by =Name= is unconditionally cleared.

	*Method:* =DELETE=

	*Request Body (JSON):*
	| Field   | Type   | Required | Description                                                          |
	|---------+--------+----------+----------------------------------------------------------------------|
	| =Hash=  | string | yes      | The dynamic hash identifying the heading.                            |
	| =Name=  | string | yes      | The date type to remove: =SCHEDULED=, =DEADLINE=, =CLOSED=, or =TIMESTAMP=. |

	*Response:* A =Result= JSON object with ={"status": true}= on success.
	EDOC */
func DeleteDate(w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	var args common.TodoDateChange
	var err = json.Unmarshal(body, &args)
	if err == nil {
		args.Value = ""
		var reply common.Result
		reply, err = ChangeDate(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			json.NewEncoder(w).Encode(err)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* POST /property — Set or Change a Property on a Heading
	Sets or updates a property in the property drawer of a heading. If the heading
	does not have a property drawer, one is created. Common properties include
	=EFFORT=, =CUSTOM_ID=, =CATEGORY=, etc., but any key/value pair is accepted.

	*Method:* =POST=

	*Request Body (JSON):*
	| Field   | Type   | Required | Description                                     |
	|---------+--------+----------+-------------------------------------------------|
	| =Hash=  | string | yes      | The dynamic hash identifying the heading.        |
	| =Name=  | string | yes      | The property key (e.g. =EFFORT=, =CUSTOM_ID=).  |
	| =Value= | string | yes      | The property value.                              |

	*Response:* A =Result= JSON object with ={"status": true}= on success.
	EDOC */
func PostChangeProperty(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostChangeProperty")
	body, _ := ioutil.ReadAll(r.Body)
	var args common.TodoPropertyChange
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("Deserialized")
		var reply common.Result
		reply, err = ChangeProperty(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			json.NewEncoder(w).Encode(err)
		}
	} else {
		fmt.Println("Failed to deserialize")
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* POST /tags — Toggle Tags on a Heading
	Toggles a tag on a heading identified by its hash. If the tag is present, it is
	removed; if absent, it is added. The =Value= field contains the tag name to toggle
	(without the surrounding colons).

	*Method:* =POST=

	*Request Body (JSON):*
	| Field   | Type   | Required | Description                                     |
	|---------+--------+----------+-------------------------------------------------|
	| =Hash=  | string | yes      | The dynamic hash identifying the heading.        |
	| =Value= | string | yes      | The tag name to toggle (e.g. =WORK=, =urgent=). |

	*Response:* A =Result= JSON object with ={"status": true}= on success.
	EDOC */
func PostToggleTags(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostToggleTags")
	body, _ := ioutil.ReadAll(r.Body)
	var args common.TodoItemChange
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("Deserialized")
		var reply common.Result
		reply, err = ToggleTag(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			json.NewEncoder(w).Encode(err)
		}
	} else {
		fmt.Println("Failed to deserialize")
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* POST /reformat — Reformat Org Files
	Re-serializes and re-saves the specified org files, normalizing whitespace,
	indentation, and heading structure. This is a lossless rewrite that ensures
	the files conform to the parser's canonical output format.

	*Method:* =POST=

	*Request Body (JSON):* A JSON array of absolute file paths to reformat.
	#+BEGIN_SRC json
	["/home/user/org/todo.org", "/home/user/org/notes.org"]
	#+END_SRC

	*Response:* A =Result= JSON object with ={"status": true}= on success.
	EDOC */
func PostReformat(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostReformat")
	body, _ := ioutil.ReadAll(r.Body)
	var args common.FileList
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("Deserialized")
		var reply common.Result
		reply, err = Reformat(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			json.NewEncoder(w).Encode(err)
		}
	} else {
		fmt.Println("Failed to deserialize")
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* GET /search — Search Headings by Query Expression
	Searches all tracked org files for headings matching the given query expression.
	The query language supports TODO status filtering, tag matching, property comparisons,
	and boolean logic. Filters and tag groups defined in config can be referenced with
	handlebars syntax (e.g. ={{ AllTasks }}=).

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                              |
	|-----------+--------+----------+----------------------------------------------------------|
	| =query=   | string | yes      | The search expression (e.g. =TODO="TODO"+HOME=).         |

	*Response:* A JSON array of =Todo= objects matching the query. Returns an error
	object on failure.
	EDOC */
func RequestTodosExpr(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	var args common.StringQuery
	args.Query = query
	reply, err := QueryStringTodos(&args)
	w.Header().Set("Content-Type", "application/json")
	if err == nil {
		json.NewEncoder(w).Encode(reply)
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* GET /lookuphash — Look Up Hash by File Position
	Given a filename and a line number (row), returns the hash of the heading that
	contains that position. This is useful for editors that know a cursor position
	and need to map it to the server's internal hash identifier.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                       |
	|------------+--------+----------+---------------------------------------------------|
	| =filename= | string | yes      | The org filename (basename or path).               |
	| =pos=      | int    | yes      | The 1-based line number in the file.               |

	*Response:* A =Todo= JSON object for the heading at that position, or an error string.
	EDOC */
func RequestHash(w http.ResponseWriter, r *http.Request) {
	strPos := r.URL.Query().Get("pos")
	fname := r.URL.Query().Get("filename")
	pos, serr := strconv.Atoi(strPos)
	w.Header().Set("Content-Type", "application/json")
	if serr != nil {
		json.NewEncoder(w).Encode("Failed to convert position value")
		return
	}
	reply, err := FindNodeInFile(pos, fname)
	if err == nil {
		json.NewEncoder(w).Encode(reply)
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* GET /hash/{hash} — Get Heading by Hash
	Retrieves the =Todo= data for a heading by its dynamic hash. The hash must be
	base64-URL-encoded in the URL path because it may contain characters that are
	not URL-safe (such as =/=).

	*Method:* =GET=

	*Path Parameters:*
	| Parameter | Type   | Description                                              |
	|-----------+--------+----------------------------------------------------------|
	| ={hash}=  | string | Base64-URL-encoded dynamic hash of the heading.          |

	*Response:* A =Todo= JSON object, or an error string if not found.
	EDOC */
func RequestByHash(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	w.Header().Set("Content-Type", "application/json")
	if h, err := GetHash(vars, "hash"); err == nil {
		var hash common.TodoHash = common.TodoHash(h)
		var err error = nil
		res := FindByHash(&hash)
		if res == nil || err != nil {
			if err == nil {
				err = fmt.Errorf("")
			}
			json.NewEncoder(w).Encode(fmt.Sprintf("could not find hash %s %s", hash, err))
		} else {
			json.NewEncoder(w).Encode(res)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* GET /id/{id} — Get Heading by ID or CUSTOM_ID
	Retrieves the =Todo= data for a heading by its =ID= or =CUSTOM_ID= property.
	The server searches all tracked files for a heading whose property drawer
	contains a matching =ID= or =CUSTOM_ID= value.

	*Method:* =GET=

	*Path Parameters:*
	| Parameter | Type   | Description                                              |
	|-----------+--------+----------------------------------------------------------|
	| ={id}=    | string | The =ID= or =CUSTOM_ID= value to search for.            |

	*Response:* A =Todo= JSON object, or an error string if not found.
	EDOC */
func RequestByAnyId(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var hash common.TodoHash = common.TodoHash(vars["id"])
	var err error = nil
	res := FindByAnyId(&hash)

	if res == nil || err != nil {
		if err == nil {
			err = fmt.Errorf("")
		}
		json.NewEncoder(w).Encode(fmt.Sprintf("could not find by any id %s %s", hash, err))
	} else {
		json.NewEncoder(w).Encode(res)
	}
}

/* SDOC: API
* POST /daypage — Create Today's Day Page
	Creates a new day page for the current date using the configured =dayPageTemplate=.
	Day pages are generated in the =dayPagePath= directory. If a day page already exists
	for today, the existing file is returned. The =dayPageMode= setting controls whether
	pages are created daily or weekly.

	*Method:* =POST=

	*Request Body:* Empty (no body required).

	*Response:*
	- On success (=200=): A JSON object with the day page file path.
	- On failure (=400=): An error string.
	EDOC */
func PostCreateDayPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	res, err := CreateDayPage()
	if res == nil || err != nil {
		if err == nil {
			err = fmt.Errorf("")
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(fmt.Sprintf("Failed creating day page %s", err))
	} else {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(res)
	}
}

/* SDOC: API
* GET /daypage/{date} — Get Day Page at a Specific Date
	Returns the day page file corresponding to the given date. The date format is
	=YYYY-DD-MM=. If no day page exists for that date, the response includes an error.

	*Method:* =GET=

	*Path Parameters:*
	| Parameter | Type   | Description                                              |
	|-----------+--------+----------------------------------------------------------|
	| ={date}=  | string | Date in =YYYY-DD-MM= format.                            |

	*Response:* A JSON object with the day page data, or an error string.
	EDOC */
func RequestDayPageAt(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var args common.Date = common.Date(vars["date"])
	res, err := GetDayPageAt(&args)
	if res == nil || err != nil {
		if err == nil {
			err = fmt.Errorf("")
		}
		json.NewEncoder(w).Encode(fmt.Sprintf("RequestDayPageAt could not get day page %s %s", args, err))
	} else {
		json.NewEncoder(w).Encode(res)
	}
}

/* SDOC: API
* GET /daypage/increment — Get Day Page Increment
	Returns the number of days between day pages. When =dayPageMode= is =week=,
	returns =7=; otherwise returns =1=. Clients use this to calculate the next/previous
	day page dates for navigation.

	*Method:* =GET=

	*Parameters:* None.

	*Response:* A JSON integer (=1= or =7=).
	EDOC */
func RequestDayPageIncrement(w http.ResponseWriter, r *http.Request) {
	if Conf().Server.DayPageMode == "week" {
		fmt.Println("DAYPAGE INC: 7")
		json.NewEncoder(w).Encode(7)
	} else {
		fmt.Println("DAYPAGE INC: 1")
		json.NewEncoder(w).Encode(1)
	}
}

/* SDOC: API
* GET /capture/templates — List Capture Templates
	Returns the list of capture templates available to the authenticated user. This merges
	templates defined in the server config (=captureTemplates=) with any per-user templates
	stored in the extensions file. Templates define the target location, heading type, and
	template text used by the capture system.

	*Method:* =GET=

	*Parameters:* None (user identity is derived from the auth token).

	*Response:* A JSON array of =CaptureTemplate= objects:
	#+BEGIN_SRC json
	[
	  {
	    "name": "Todo",
	    "type": "entry",
	    "target": {"Filename": "todo.org", "Id": "Tasks", "Type": "file+headline"},
	    "template": "* TODO %?"
	  }
	]
	#+END_SRC
	EDOC */
func RequestCaptureTemplates(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	res, err := QueryCaptureTemplates(username)
	if err != nil {
		fmt.Printf("QueryCaptureTemplates: %s", err.Error())
	}
	if res == nil || err != nil {
		if err == nil {
			err = fmt.Errorf("")
		}
		json.NewEncoder(w).Encode(fmt.Sprintf("could not get capture template list %s", err))
	} else {
		json.NewEncoder(w).Encode(res)
	}
}

/* SDOC: API
* POST /capture — Capture a New Entry
	Creates a new heading or entry using the capture template system. The template
	is expanded with the provided data, and the result is inserted at the target
	location defined by the template. This is the server-side equivalent of
	=org-capture= in Emacs.

	*Method:* =POST=

	*Request Body (JSON):*
	| Field      | Type      | Required | Description                                              |
	|------------+-----------+----------+----------------------------------------------------------|
	| =Template= | string    | yes      | Name of the capture template to use.                     |
	| =NewNode=  | NewNode   | yes      | Object containing the data for the new entry.            |

	*Response:* A =ResultMsg= JSON object.
	- ={"status": true, "msg": "..."}= on success.
	- An error on failure.
	EDOC */
func PostCapture(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostCapture")
	username := GetUsername(r)
	body, _ := io.ReadAll(r.Body)
	var args common.Capture
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("  Deserialized", args)
		var reply common.ResultMsg
		reply, err = Capture(db, &args, username)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Println("Capture failed to operate")
			json.NewEncoder(w).Encode(err)
		}
	} else {
		fmt.Println("Failed to deserialize", err, string(body))
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* GET /exclusivemarker — Get Exclusive Marker
	Retrieves the heading that currently holds the named exclusive marker tag. Exclusive
	markers are tags that can only be present on one heading at a time across all files,
	acting as named bookmarks (e.g. =:NOW:=, =:FOCUS:=).

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                        |
	|-----------+--------+----------+------------------------------------|
	| =name=    | string | yes      | The marker tag name (e.g. =NOW=).  |

	*Response:* The =Todo= object of the heading holding the marker, or an error string
	if no heading has the marker.
	EDOC */
func RequestMarker(w http.ResponseWriter, r *http.Request) {
	// This a parameter rather than path
	args := r.URL.Query().Get("name")
	res, err := GetMarkerTag(args)
	if err != nil {
		Log().Errorf("GetMarkerErr: %s", err.Error())
	}
	if res == nil || err != nil {
		if err == nil {
			err = fmt.Errorf("GetMarker Error")
		}
		json.NewEncoder(w).Encode(fmt.Sprintf("could not get exclusive marker %s", err))
	} else {
		json.NewEncoder(w).Encode(res)
	}
}

/* SDOC: API
* POST /setexclusivemarker — Set Exclusive Marker
	Moves an exclusive marker tag to a new heading. The tag is first removed from
	whatever heading currently holds it (if any), then applied to the target heading.
	This ensures at most one heading in the entire database has the marker at any time.

	*Method:* =POST=

	*Request Body (JSON):*
	| Field  | Type   | Required | Description                                                   |
	|--------+--------+----------+---------------------------------------------------------------|
	| =Name= | string | yes      | The marker tag name (e.g. =NOW=, =FOCUS=).                   |
	| =ToId= | Target | yes      | A =Target= identifying the heading to receive the marker.     |

	The =Target= object has fields: =Filename=, =Id=, =Type= (one of =file+headline=,
	=id=, =customid=, =hash=, =file+line=), and optionally =Lvl=.

	*Response:* A =Result= JSON object with ={"status": true}= on success.
	EDOC */
func PostMarker(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostMarker")
	body, _ := io.ReadAll(r.Body)
	var args common.ExclusiveTagMarker
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("  Deserialized", args)
		var reply common.Result
		reply, err = SetMarkerTag(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Println("Today failed to operate")
			json.NewEncoder(w).Encode(err)
		}
	} else {
		fmt.Println("Failed to deserialize", err, string(body))
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* POST /delete — Delete a Heading
	Removes the heading (and its entire subtree) identified by the target from the org file.
	The file is re-saved to disk after the deletion. This operation is destructive and
	cannot be undone through the API.

	*Method:* =POST=

	*Request Body (JSON):* A =Target= object identifying the heading to delete.
	| Field      | Type   | Required | Description                                                      |
	|------------+--------+----------+------------------------------------------------------------------|
	| =Filename= | string | varies   | The org filename (used with =file+headline= and =file+line=).    |
	| =Id=       | string | varies   | The identifier (headline text, hash, id, or line number).        |
	| =Type=     | string | yes      | One of =file+headline=, =id=, =customid=, =hash=, =file+line=.  |
	| =Lvl=      | int    | no       | If non-zero, only match headings at this level.                  |

	*Response:* A =ResultMsg= JSON object.
	EDOC */
func PostDelete(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostDelete")
	body, _ := io.ReadAll(r.Body)
	var args common.Target
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("  Deserialized", args)
		var reply common.ResultMsg
		reply, err = Delete(db, &args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Println("Delete failed to operate")
			json.NewEncoder(w).Encode(err)
		}
	} else {
		fmt.Println("Delete to deserialize", err, string(body))
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* POST /update — Run an Updater Plugin on a Heading
	Invokes a named updater plugin against the heading identified by the target.
	Updater plugins perform external synchronization (e.g. the =jira= updater pushes
	changes to Jira). The updater must be enabled in the server's config under
	=server.updaters=.

	*Method:* =POST=

	*Request Body (JSON):*
	| Field    | Type   | Required | Description                                               |
	|----------+--------+----------+-----------------------------------------------------------|
	| =Name=   | string | yes      | The updater plugin name (e.g. =jira=).                    |
	| =Target= | Target | yes      | A =Target= identifying the heading to update.             |

	*Response:* A =ResultMsg= JSON object.
	EDOC */
func PostUpdate(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostUpdate")
	body, _ := io.ReadAll(r.Body)
	var args common.Update
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("  Deserialized", args)
		var reply common.ResultMsg
		reply, err = PluginUpdateTarget(db, &args.Target, args.Name)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Println("Update failed to operate")
			json.NewEncoder(w).Encode(err)
		}
	} else {
		fmt.Println("Update to deserialize", err, string(body))
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* POST /refile — Refile a Heading to a New Location
	Moves a heading (and its subtree) from its current location to a new target.
	The heading is removed from the source file and inserted as a child of the
	target heading. Both files are re-saved to disk.

	*Method:* =POST=

	*Request Body (JSON):*
	| Field    | Type   | Required | Description                                              |
	|----------+--------+----------+----------------------------------------------------------|
	| =FromId= | Target | yes      | A =Target= identifying the heading to move.              |
	| =ToId=   | Target | yes      | A =Target= identifying the destination heading.          |

	*Response:* A =ResultMsg= JSON object.
	EDOC */
func PostRefile(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostRefile")
	body, _ := io.ReadAll(r.Body)
	var args common.Refile
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("  Deserialized", args)
		var reply common.ResultMsg
		reply, err = Refile(db, &args, nil, false)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Println("Refile failed to operate")
			json.NewEncoder(w).Encode(err)
		}
	} else {
		fmt.Println("Refile to deserialize", err, string(body))
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* POST /archive — Archive a Heading
	Archives the heading identified by the target. The heading is moved from its current
	file into the corresponding =_archive= file (e.g. =todo.org_archive=) following
	standard Org mode archiving conventions. The original file is re-saved.

	*Method:* =POST=

	*Request Body (JSON):* A =Target= object identifying the heading to archive.
	| Field      | Type   | Required | Description                                                      |
	|------------+--------+----------+------------------------------------------------------------------|
	| =Filename= | string | varies   | The org filename.                                                |
	| =Id=       | string | varies   | The identifier.                                                  |
	| =Type=     | string | yes      | One of =file+headline=, =id=, =customid=, =hash=, =file+line=.  |

	*Response:* A =ResultMsg= JSON object.
	EDOC */
func PostArchive(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostArchive")
	body, _ := io.ReadAll(r.Body)
	var args common.Target
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("  Deserialized", args)
		var reply common.ResultMsg
		reply, err = Archive(db, &args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Println("Archive failed to operate")
			json.NewEncoder(w).Encode(err)
		}
	} else {
		fmt.Println("Archive to deserialize", err, string(body))
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* GET /clock — Get Current Clock Status
	Returns the current clocking state. If a heading is actively being clocked,
	the response includes the start time, the target heading, and =Active: true=.
	If no clock is running, =Active= is =false=.

	*Method:* =GET=

	*Parameters:* None.

	*Response:* A JSON object:
	#+BEGIN_SRC json
	{
	  "Active": true,
	  "Time": { "start": "...", "end": "..." },
	  "Target": { "Filename": "...", "Id": "...", "Type": "..." }
	}
	#+END_SRC
	EDOC */
func RequestClock(w http.ResponseWriter, r *http.Request) {
	type ClockData struct {
		Active bool
		Time   org.OrgDate
		Target common.Target
	}
	data := ClockData{}
	active := Clock().IsClockActive()
	data.Active = active
	if active {
		data.Time = *Clock().GetTime()
		data.Target = *Clock().GetTarget()
	}
	json.NewEncoder(w).Encode(data)
}

/* SDOC: API
* POST /clockin — Clock In to a Heading
	Starts a clock on the heading identified by the target. If another heading is
	currently clocked in, it is automatically clocked out first. A =CLOCK:= entry
	with the start time is added to the heading's logbook drawer.

	*Method:* =POST=

	*Request Body (JSON):* A =Target= object identifying the heading to clock into.
	| Field      | Type   | Required | Description                                                      |
	|------------+--------+----------+------------------------------------------------------------------|
	| =Filename= | string | varies   | The org filename.                                                |
	| =Id=       | string | varies   | The identifier.                                                  |
	| =Type=     | string | yes      | One of =file+headline=, =id=, =customid=, =hash=, =file+line=.  |

	*Response:* A =ResultMsg= JSON object.
	EDOC */
func PostClockIn(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostClockIn")
	body, _ := io.ReadAll(r.Body)
	var args common.Target
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("  Deserialized", args)
		var reply common.ResultMsg
		reply, err = Clock().ClockIn(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Println("ClockIn failed to operate")
			json.NewEncoder(w).Encode(err)
		}
	} else {
		fmt.Println("ClockIn to deserialize", err, string(body))
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* POST /clockout — Clock Out
	Stops the currently active clock. The end time is recorded on the open =CLOCK:=
	entry and the duration is calculated. If no clock is active, this is a no-op.

	*Method:* =POST=

	*Request Body:* Ignored (body is read but not used).

	*Response:* A =ResultMsg= JSON object.
	EDOC */
func PostClockOut(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostClockOut")
	body, err := io.ReadAll(r.Body)
	if err == nil {
		var reply common.ResultMsg
		reply, err = Clock().ClockOut()
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Println("ClockOut failed to operate")
			json.NewEncoder(w).Encode(err)
		}
	} else {
		fmt.Println("ClockOut to deserialize", err, string(body))
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* GET /clockreport — Generate a Clock Report
	Generates a summary of clocked time across all headings for the specified time block.
	Supports blocks like =today=, =yesterday=, =thisweek=, =lastweek=, =thismonth=, etc.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                                  |
	|-----------+--------+----------+--------------------------------------------------------------|
	| =block=   | string | no       | Time block to report on. Defaults to =today=.                |

	*Response:* A JSON array of =ClockEntry= objects, each containing =headline=,
	=filename=, =level=, and =mins= (total minutes clocked).
	EDOC */
func RequestClockReport(w http.ResponseWriter, r *http.Request) {
	block := r.URL.Query().Get("block")
	if block == "" {
		block = "today"
	}
	report := GenerateClockReport(block)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

/* SDOC: API
* GET /logbook/{hash} — Get Logbook Entries for a Heading
	Returns the LOGBOOK clock entries for a heading identified by its hash. Each entry
	contains the start time, end time (if clocked out), and duration in minutes.

	*Method:* =GET=

	*Path Parameters:*
	| Parameter | Type   | Description                                              |
	|-----------+--------+----------------------------------------------------------|
	| ={hash}=  | string | Base64-URL-encoded hash of the heading.                  |

	*Response:* A =Logbook= JSON object:
	#+BEGIN_SRC json
	{
	  "entries": [
	    {"start": "2024-01-15T09:00:00Z", "end": "2024-01-15T10:30:00Z", "mins": 90}
	  ],
	  "totalMin": 90
	}
	#+END_SRC
	Returns =404= if the hash is not found, =400= on invalid hash encoding.
	EDOC */
func RequestLogbook(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		hash := string(h)
		if s, ok := GetDb().ByHash[hash]; ok {
			var logbook common.Logbook
			drawer := s.Headline.FindDrawer(Conf().ClockIntoDrawer)
			if drawer != nil && drawer.Children != nil {
				for _, c := range drawer.Children {
					if c.GetType() == org.ClockNode {
						clk := c.(org.Clock)
						entry := common.LogbookEntry{
							Start: clk.Date.Start.Format(time.RFC3339),
							Mins:  float64(clk.Date.DurationMins),
						}
						if !clk.Date.End.IsZero() {
							entry.End = clk.Date.End.Format(time.RFC3339)
						}
						logbook.Entries = append(logbook.Entries, entry)
						logbook.TotalMin += entry.Mins
					}
				}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(logbook)
		} else {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		}
	} else {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* POST /execb — Execute a Source Block
	Executes the source block at the specified position in an org file. The block is
	identified by a =PreciseTarget= which combines a =Target= (to find the heading)
	with a =Row= offset (to locate the specific block within the heading's body).
	The block is executed according to its language and the result is returned.

	*Method:* =POST=

	*Request Body (JSON):*
	| Field          | Type    | Required | Description                                                |
	|----------------+---------+----------+------------------------------------------------------------|
	| =Target=       | Target  | yes      | Identifies the heading containing the block.               |
	| =Row=          | int     | yes      | Line offset within the heading to locate the block.        |

	*Response:* A =ResultMsg= JSON object. On success, =msg= contains the execution result.
	EDOC */
func PostExecb(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostExecb")
	body, err := io.ReadAll(r.Body)
	if err == nil {
		var args common.PreciseTarget
		var err = json.Unmarshal(body, &args)
		if err == nil {
			var reply common.ResultMsg
			reply, err = ExecBlock(db, &args)
			if err == nil {
				json.NewEncoder(w).Encode(reply)
			} else {
				fmt.Println("BlockExec failed to exec")
				json.NewEncoder(w).Encode(err)
			}
		} else {
			fmt.Println("BlockExec failed to deserialize", err, string(body))
			json.NewEncoder(w).Encode(err)
		}
	} else {
		fmt.Println("BlockExec failed to read body", err)
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* GET /tablerandomget — Get a Random Row from a Named Table
	Selects a random row from a named table (one that has a =#+NAME:= keyword above it)
	and returns it as a pipe-delimited org table row string. Useful for flashcard-style
	random selection from data tables.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                             |
	|-----------+--------+----------+---------------------------------------------------------|
	| =name=    | string | yes      | The =#+NAME:= of the table to select from.              |

	*Response:* A =ResultMsg= JSON object. On success, =msg= contains a row like =| col1 | col2 |=.
	EDOC */
func RequestTableRandomGet(w http.ResponseWriter, r *http.Request) {
	fmt.Println("RequestTableRandomGet")
	/*
		vars := mux.Vars(r)
	*/
	name := r.URL.Query().Get("name")
	name = strings.TrimSpace(name)
	var rep common.ResultMsg
	if name != "" {
		tables := GetDb().GetNamedTables(name)
		if len(tables) > 0 {
			table := tables[0]
			max := len(table.Table.Rows)
			rowIdx := rand.Intn(max)
			row := table.Table.Rows[rowIdx]
			w := org.NewOrgWriter()

			res := "|"
			for _, col := range row.Columns {
				res += " "
				res += w.WriteNodesAsString(col.Children...)
				res += " |"
			}
			if len(res) == 1 {
				res += "|"
			}
			rep = common.ResultMsg{Ok: true, Msg: res}
			fmt.Printf("RESULT: %v\n", rep)
		} else {
			rep = common.ResultMsg{Ok: false, Msg: fmt.Sprintf("Failed to find tables with name %s", name)}
		}
	} else {
		fmt.Println("ERR: Name must be specified for table query!")
		rep = common.ResultMsg{Ok: false, Msg: "Failed to find table did you specify a name?"}
	}
	json.NewEncoder(w).Encode(rep)
	//json.NewEncoder(w).Encode(data)
}

/* SDOC: API
* GET /tablenames — List All Named Tables
	Returns the names of all tables across all org files that have a =#+NAME:= keyword.
	These names can be used with other table endpoints like =/tablerandomget=.

	*Method:* =GET=

	*Parameters:* None.

	*Response:* A JSON object:
	#+BEGIN_SRC json
	{"Ok": true, "NamedTables": ["vocabulary", "contacts", "inventory"]}
	#+END_SRC
	Returns ={"Ok": false, "NamedTables": null}= if no named tables exist.
	EDOC */
func RequestTableNames(w http.ResponseWriter, r *http.Request) {
	fmt.Println("RequestTableNames")
	/*
		vars := mux.Vars(r)
	*/
	type ResultTableNames struct {
		Ok          bool
		NamedTables []string
	}
	var rep ResultTableNames
	tables := GetDb().GetTableNames()
	if len(tables) > 0 {
		keys := make([]string, 0, len(tables))
		for k := range tables {
			keys = append(keys, k)
		}
		rep = ResultTableNames{Ok: true, NamedTables: keys}
	} else {
		rep = ResultTableNames{Ok: false, NamedTables: nil}
	}
	json.NewEncoder(w).Encode(rep)
}

/* SDOC: API
* POST /tableformulainfo — Get Table Formula Details
	Returns detailed information about the table and its formulas at the specified position
	in an org file. This includes the table structure, cell references, and any =#+TBLFM:=
	formula lines attached to the table.

	*Method:* =POST=

	*Request Body (JSON):* A =PreciseTarget= object.
	| Field    | Type   | Required | Description                                            |
	|----------+--------+----------+--------------------------------------------------------|
	| =Target= | Target | yes      | Identifies the heading containing the table.           |
	| =Row=    | int    | yes      | Line offset within the heading to locate the table.    |

	*Response:* A =ResultTableDetailsMsg= JSON object on success, or a =ResultMsg=
	with ={"status": false}= on failure.
	EDOC */
func PostFormulaInfo(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostFormulaInfo")
	body, err := io.ReadAll(r.Body)
	if err == nil {
		var args common.PreciseTarget
		var err = json.Unmarshal(body, &args)
		if err == nil {
			var reply common.ResultTableDetailsMsg
			reply, err = FormulaDetailsAt(db, &args)
			if err == nil {
				fmt.Printf("RETURNING OK\n")
				json.NewEncoder(w).Encode(reply)
			} else {
				fmt.Println("Table Info failed: ", err.Error())
				rep := common.ResultMsg{Ok: false, Msg: err.Error()}
				json.NewEncoder(w).Encode(rep)
			}
		} else {
			fmt.Println("TableInfo failed to deserialize", err, string(body))
			rep := common.ResultMsg{Ok: false, Msg: err.Error()}
			json.NewEncoder(w).Encode(rep)
		}
	} else {
		fmt.Println("TableFailed failed to read body", err)
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* POST /exectable — Execute Table Formulas
	Evaluates the =#+TBLFM:= formulas on the table at the specified position in an
	org file and updates the table cells with the computed results. The file is
	re-saved to disk after the update.

	*Method:* =POST=

	*Request Body (JSON):* A =PreciseTarget= object.
	| Field    | Type   | Required | Description                                            |
	|----------+--------+----------+--------------------------------------------------------|
	| =Target= | Target | yes      | Identifies the heading containing the table.           |
	| =Row=    | int    | yes      | Line offset within the heading to locate the table.    |

	*Response:* A =ResultMsg= JSON object.
	EDOC */
func PostExect(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostExecT")
	body, err := io.ReadAll(r.Body)
	if err == nil {
		var args common.PreciseTarget
		var err = json.Unmarshal(body, &args)
		if err == nil {
			var reply common.ResultMsg
			reply, err = ExecTableAt(db, &args)
			if err == nil {
				json.NewEncoder(w).Encode(reply)
			} else {
				fmt.Println("Table Execution failed: ", err.Error())
				rep := common.ResultMsg{Ok: false, Msg: err.Error()}
				json.NewEncoder(w).Encode(rep)
			}
		} else {
			fmt.Println("TableExec failed to deserialize", err, string(body))
			rep := common.ResultMsg{Ok: false, Msg: err.Error()}
			json.NewEncoder(w).Encode(rep)
		}
	} else {
		fmt.Println("TableExec failed to read body", err)
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* POST /execalltables — Execute All Table Formulas in a File
	Evaluates =#+TBLFM:= formulas on every table in the specified org file and updates
	all table cells with the computed results. The file is re-saved to disk after the update.

	*Method:* =POST=

	*Request Body (JSON):* A JSON string containing the org filename.
	#+BEGIN_SRC json
	"todo.org"
	#+END_SRC

	*Response:* A =ResultMsg= JSON object on success, or a =ResultMsg= with
	={"status": false}= containing concatenated error messages on failure.
	EDOC */
func PostExecAllT(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostExecAllT")
	body, err := io.ReadAll(r.Body)
	if err == nil {
		var args string
		var err = json.Unmarshal(body, &args)
		if err == nil {
			reply, errs := ExecAllTables(db, args)
			if len(errs) <= 0 {
				json.NewEncoder(w).Encode(reply)
			} else {
				msg := ""
				for _, e := range errs {
					msg += e.Error() + "\n"
				}
				err = fmt.Errorf(msg)
				fmt.Println("All Table Execution failed: ", err.Error())
				rep := common.ResultMsg{Ok: false, Msg: err.Error()}
				json.NewEncoder(w).Encode(rep)
			}
		} else {
			fmt.Println("AllTableExec failed to deserialize", err, string(body))
			rep := common.ResultMsg{Ok: false, Msg: err.Error()}
			json.NewEncoder(w).Encode(rep)
		}
	} else {
		fmt.Println("AllTableExec failed to read body", err)
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* GET /status/{hash} — Get Valid Status Keywords for a Heading
	Returns the list of valid TODO status keywords that can be applied to the heading
	identified by the given hash. This takes into account both the global =defaultTodoStates= /
	=defaultNextStates= from server config and any file-level =#+TODO:= or =#+SEQ_TODO:=
	definitions.

	*Method:* =GET=

	*Path Parameters:*
	| Parameter | Type   | Description                                              |
	|-----------+--------+----------------------------------------------------------|
	| ={hash}=  | string | Base64-URL-encoded hash of the heading.                  |

	*Response:* A JSON array of valid status keyword strings (e.g. =["TODO", "NEXT", "DONE"]=),
	or an error string.
	EDOC */
func RequestValidStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var args common.TodoHash = common.TodoHash(h)
		res, err := ValidStatus(&args)
		if err != nil {
			if err == nil {
				err = fmt.Errorf("")
			}
			json.NewEncoder(w).Encode(fmt.Sprintf("could not get valid status for hash %s %s", args, err))
		} else {
			json.NewEncoder(w).Encode(res)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* GET /next/{hash} — Get Next Sibling Heading
	Returns the =Todo= data for the next sibling heading (the heading at the same level
	immediately following the given heading). Returns an error string if there is no
	next sibling.

	*Method:* =GET=

	*Path Parameters:*
	| Parameter | Type   | Description                                              |
	|-----------+--------+----------------------------------------------------------|
	| ={hash}=  | string | Base64-URL-encoded hash of the heading.                  |

	*Response:* A =Todo= JSON object, or an error string.
	EDOC */
func RequestNextSibling(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var args common.TodoHash = common.TodoHash(h)
		res := NextSibling(&args)
		if res == nil {
			json.NewEncoder(w).Encode(fmt.Sprintf("could not get next sibling for hash %s", args))
		} else {
			json.NewEncoder(w).Encode(res)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* GET /prev/{hash} — Get Previous Sibling Heading
	Returns the =Todo= data for the previous sibling heading (the heading at the same level
	immediately before the given heading). Returns an error string if there is no
	previous sibling.

	*Method:* =GET=

	*Path Parameters:*
	| Parameter | Type   | Description                                              |
	|-----------+--------+----------------------------------------------------------|
	| ={hash}=  | string | Base64-URL-encoded hash of the heading.                  |

	*Response:* A =Todo= JSON object, or an error string.
	EDOC */
func RequestPrevSibling(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var args common.TodoHash = common.TodoHash(h)
		res := PrevSibling(&args)
		if res == nil {
			json.NewEncoder(w).Encode(fmt.Sprintf("could not get prev sibling for hash %s", args))
		} else {
			json.NewEncoder(w).Encode(res)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* GET /child/{hash} — Get Last Child Heading
	Returns the =Todo= data for the last child heading of the given heading.
	Returns an error string if the heading has no children.

	*Method:* =GET=

	*Path Parameters:*
	| Parameter | Type   | Description                                              |
	|-----------+--------+----------------------------------------------------------|
	| ={hash}=  | string | Base64-URL-encoded hash of the parent heading.           |

	*Response:* A =Todo= JSON object, or an error string.
	EDOC */
func RequestLastChild(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var args common.TodoHash = common.TodoHash(h)
		res := LastChild(&args)
		if res == nil {
			json.NewEncoder(w).Encode(fmt.Sprintf("could not get last child for hash %s", args))
		} else {
			json.NewEncoder(w).Encode(res)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

/* SDOC: API
* GET /alltags — List All Tags
	Returns a deduplicated list of every tag found across all headings in all tracked
	org files. Useful for building tag-completion UIs.

	*Method:* =GET=

	*Parameters:* None.

	*Response:* A JSON array of tag strings (e.g. =["WORK", "HOME", "urgent", "PROJECT"]=),
	or an error string if the tag list cannot be retrieved.
	EDOC */
func RequestTags(w http.ResponseWriter, r *http.Request) {
	res := GetDb().GetAllTags()
	if res == nil {
		json.NewEncoder(w).Encode("could not get tags list")
	} else {
		json.NewEncoder(w).Encode(res)
	}
}
