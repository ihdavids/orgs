//lint:file-ignore ST1006 allow the use of self
package main

import (
	b64 "encoding/base64"
	"encoding/json"
	"io"
	"log"
	"strconv"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs"
	"github.com/ihdavids/orgs/internal/common"

	//"log"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	//"time"
)

func restApi(router *mux.Router) {
	router.Use(loggingMiddleware)
	router.HandleFunc("/orgfile", RequestOrgFile)
	router.HandleFunc("/files", RequestFiles)
	router.HandleFunc("/file", CreateFile).Methods("POST")
	router.HandleFunc("/file/{type}", RequestFile) // html etc
	router.HandleFunc("/search", RequestTodosExpr)
	router.HandleFunc("/lookuphash", RequestHash)
	router.HandleFunc("/todohtml/{hash}", RequestFullTodoHtml)
	router.HandleFunc("/filehtml/{hash}", RequestFullFileHtml)
	router.HandleFunc("/todofull/{hash}", RequestFullTodo)
	router.HandleFunc("/hash/{hash}", RequestByHash)
	router.HandleFunc("/next/{hash}", RequestNextSibling)
	router.HandleFunc("/prev/{hash}", RequestPrevSibling)
	router.HandleFunc("/child/{hash}", RequestLastChild)
	router.HandleFunc("/id/{id}", RequestByAnyId)
	router.HandleFunc("/daypage/increment", RequestDayPageIncrement).Methods("GET")
	router.HandleFunc("/daypage/{date}", RequestDayPageAt).Methods("GET")
	router.HandleFunc("/daypage", PostCreateDayPage).Methods("POST")
	router.HandleFunc("/status/change", PostChangeStatus).Methods("POST")
	router.HandleFunc("/status/{hash}", RequestValidStatus)
	router.HandleFunc("/property", PostChangeProperty).Methods("POST")
	router.HandleFunc("/alltags", RequestTags)
	router.HandleFunc("/tags", PostToggleTags).Methods("POST")
	router.HandleFunc("/capture", PostCapture).Methods("POST")
	router.HandleFunc("/capture/templates", RequestCaptureTemplates)
	router.HandleFunc("/delete", PostDelete).Methods("POST")
	router.HandleFunc("/refilefiles", RequestRefileTargets)
	router.HandleFunc("/refile", PostRefile).Methods("POST")
	router.HandleFunc("/archive", PostArchive).Methods("POST")
	router.HandleFunc("/reformat", PostReformat).Methods("POST")
	router.HandleFunc("/setexclusivemarker", PostMarker).Methods("POST")
	router.HandleFunc("/exclusivemarker", RequestMarker)
	router.HandleFunc("/update", PostUpdate).Methods("POST")
	router.HandleFunc("/clockin", PostClockIn).Methods("POST")
	router.HandleFunc("/clockout", PostClockOut).Methods("POST")
	router.HandleFunc("/clock", RequestClock)
	router.HandleFunc("/execb", PostExecb).Methods("POST")
	router.HandleFunc("/exectable", PostExect).Methods("POST")
	router.HandleFunc("/execalltables", PostExecAllT).Methods("POST")
	router.HandleFunc("/tableformulainfo", PostFormulaInfo).Methods("POST")
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

// Request the list of files.
func RequestFiles(w http.ResponseWriter, r *http.Request) {
	//vars := mux.Vars(r)
	//key := vars["id"]
	res := orgs.GetDb().GetFiles()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}


// Request the contents of a file as a raw file
// returns a ResultMsg with ok and error or ok and message
// https://.../orgfile?filename="blah"
func RequestOrgFile(w http.ResponseWriter, r *http.Request) {
	//vars := mux.Vars(r)
	filename := r.URL.Query().Get("filename")
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

// Request the contents of a file in a given encoding.
func RequestFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ptype := vars["type"]
	fname := r.URL.Query().Get("filename")
	query := r.URL.Query().Get("query")
	local := r.URL.Query().Get("local")
	filelinks := r.URL.Query().Get("filelinks")
	httpslinks := r.URL.Query().Get("httpslinks")
	opts := common.ExportToFile{Name: ptype, Filename: fname, Query: query, Opts: ""}
	if filelinks == "t" {
		opts.Opts += "filelinks;"
	}
	if httpslinks == "t" {
		opts.Opts += "httpslinks;"
	}
	var res common.ResultMsg
	if local == "t" {
		res, _ = orgs.ExportToFile(db, &opts)
	} else {
		res, _ = orgs.ExportToString(db, &opts)
	}
	json.NewEncoder(w).Encode(res)
}

func RequestRefileTargets(w http.ResponseWriter, r *http.Request) {
	//vars := mux.Vars(r)
	//ptype := vars["type"]
	//fname := r.URL.Query().Get("filename")
	//query := r.URL.Query().Get("query")
	//local := r.URL.Query().Get("local")
	targets := orgs.GetRefileTargetsList([]string{})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(targets)
}

func RequestFullFileHtml(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var hash common.TodoHash = common.TodoHash(h)
		reply, err := orgs.QueryFullFileHtml(&hash)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			json.NewEncoder(w).Encode(err)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

func RequestFullTodoHtml(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var hash common.TodoHash = common.TodoHash(h)
		reply, err := orgs.QueryFullTodoHtml(&hash)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			json.NewEncoder(w).Encode(err)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

func RequestFullTodo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var hash common.TodoHash = common.TodoHash(string(h))
		reply, err := orgs.QueryFullTodo(&hash)
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

func CreateFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	body, _ := io.ReadAll(r.Body)
	// TODO: Handle this and allow creation of files.
	fmt.Println(string(body))
	/*
		res, err := orgs.CreateDayPage()
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
	*/
}

// Change the status TODO,DONE etc in a todo head by hash
func PostChangeStatus(w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	var args common.TodoItemChange
	var err = json.Unmarshal(body, &args)
	if err == nil {
		var reply common.Result
		reply, err = orgs.ChangeStatus(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			json.NewEncoder(w).Encode(err)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

// Change a property on a node. (EFFORT?)
func PostChangeProperty(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostChangeProperty")
	body, _ := ioutil.ReadAll(r.Body)
	var args common.TodoPropertyChange
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("Deserialized")
		var reply common.Result
		reply, err = orgs.ChangeProperty(&args)
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

func PostToggleTags(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostToggleTags")
	body, _ := ioutil.ReadAll(r.Body)
	var args common.TodoItemChange
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("Deserialized")
		var reply common.Result
		reply, err = orgs.ToggleTag(&args)
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

func PostReformat(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostReformat")
	body, _ := ioutil.ReadAll(r.Body)
	var args common.FileList
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("Deserialized")
		var reply common.Result
		reply, err = orgs.Reformat(&args)
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

// Request all nodes matching a query expression
func RequestTodosExpr(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	var args common.StringQuery
	args.Query = query
	reply, err := orgs.QueryStringTodos(&args)
	if err == nil {
		json.NewEncoder(w).Encode(reply)
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

// Request the hash of a node at a specific row in the file
func RequestHash(w http.ResponseWriter, r *http.Request) {
	strPos := r.URL.Query().Get("pos")
	fname := r.URL.Query().Get("filename")
	pos, serr := strconv.Atoi(strPos)
	if serr != nil {
		json.NewEncoder(w).Encode("Failed to convert position value")
		return
	}
	reply, err := orgs.FindNodeInFile(pos, fname)
	if err == nil {
		json.NewEncoder(w).Encode(reply)
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

// Request node by active dynamic hash
func RequestByHash(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var hash common.TodoHash = common.TodoHash(h)
		var err error = nil
		res := orgs.FindByHash(&hash)
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

// Request node by custom_id or id property
func RequestByAnyId(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var hash common.TodoHash = common.TodoHash(vars["id"])
	var err error = nil
	res := orgs.FindByAnyId(&hash)

	if res == nil || err != nil {
		if err == nil {
			err = fmt.Errorf("")
		}
		json.NewEncoder(w).Encode(fmt.Sprintf("could not find by any id %s %s", hash, err))
	} else {
		json.NewEncoder(w).Encode(res)
	}
}

func PostCreateDayPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	res, err := orgs.CreateDayPage()
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

// Request the filename of the current daypage.
func RequestDayPageAt(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var args common.Date = common.Date(vars["date"])
	res, err := orgs.GetDayPageAt(&args)
	if res == nil || err != nil {
		if err == nil {
			err = fmt.Errorf("")
		}
		json.NewEncoder(w).Encode(fmt.Sprintf("RequestDayPageAt could not get day page %s %s", args, err))
	} else {
		json.NewEncoder(w).Encode(res)
	}
}

// Request the daypage increment from orgs.
func RequestDayPageIncrement(w http.ResponseWriter, r *http.Request) {
	if orgs.Conf().DayPageMode == "week" {
		fmt.Println("DAYPAGE INC: 7")
		json.NewEncoder(w).Encode(7)
	} else {
		fmt.Println("DAYPAGE INC: 1")
		json.NewEncoder(w).Encode(1)
	}
}

// Request a list of capture templates defined on the server
func RequestCaptureTemplates(w http.ResponseWriter, r *http.Request) {
	res, err := orgs.QueryCaptureTemplates()
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

func PostCapture(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostCapture")
	body, _ := io.ReadAll(r.Body)
	var args common.Capture
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("  Deserialized", args)
		var reply common.ResultMsg
		reply, err = orgs.Capture(db, &args)
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

func RequestMarker(w http.ResponseWriter, r *http.Request) {
	// This a parameter rather than path
	args := r.URL.Query().Get("name")
	res, err := orgs.GetMarkerTag(args)
	if err != nil {
		orgs.Log().Errorf("GetMarkerErr: %s", err.Error())
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

// Exclusive markers are tags that can only be on one heading at a time.
func PostMarker(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostMarker")
	body, _ := io.ReadAll(r.Body)
	var args common.ExclusiveTagMarker
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("  Deserialized", args)
		var reply common.Result
		reply, err = orgs.SetMarkerTag(&args)
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

func PostDelete(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostDelete")
	body, _ := io.ReadAll(r.Body)
	var args common.Target
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("  Deserialized", args)
		var reply common.ResultMsg
		reply, err = orgs.Delete(db, &args)
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

func PostUpdate(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostUpdate")
	body, _ := io.ReadAll(r.Body)
	var args common.Update
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("  Deserialized", args)
		var reply common.ResultMsg
		reply, err = orgs.PluginUpdateTarget(db, &args.Target, args.Name)
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

func PostRefile(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostRefile")
	body, _ := io.ReadAll(r.Body)
	var args common.Refile
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("  Deserialized", args)
		var reply common.ResultMsg
		reply, err = orgs.Refile(db, &args, nil, false)
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

func PostArchive(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostArchive")
	body, _ := io.ReadAll(r.Body)
	var args common.Target
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("  Deserialized", args)
		var reply common.ResultMsg
		reply, err = orgs.Archive(db, &args)
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

func RequestClock(w http.ResponseWriter, r *http.Request) {
	type ClockData struct {
		Active bool
		Time   org.OrgDate
		Target common.Target
	}
	data := ClockData{}
	active := orgs.Clock().IsClockActive()
	data.Active = active
	if active {
		data.Time = *orgs.Clock().GetTime()
		data.Target = *orgs.Clock().GetTarget()
	}
	json.NewEncoder(w).Encode(data)
}

func PostClockIn(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostClockIn")
	body, _ := io.ReadAll(r.Body)
	var args common.Target
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Println("  Deserialized", args)
		var reply common.ResultMsg
		reply, err = orgs.Clock().ClockIn(&args)
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

func PostClockOut(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostClockOut")
	body, err := io.ReadAll(r.Body)
	if err == nil {
		var reply common.ResultMsg
		reply, err = orgs.Clock().ClockOut()
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

func PostExecb(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostExecb")
	body, err := io.ReadAll(r.Body)
	if err == nil {
		var args common.PreciseTarget
		var err = json.Unmarshal(body, &args)
		if err == nil {
			var reply common.ResultMsg
			reply, err = orgs.ExecBlock(db, &args)
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

func PostFormulaInfo(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostFormulaInfo")
	body, err := io.ReadAll(r.Body)
	if err == nil {
		var args common.PreciseTarget
		var err = json.Unmarshal(body, &args)
		if err == nil {
			var reply common.ResultTableDetailsMsg
			reply, err = orgs.FormulaDetailsAt(db, &args)
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

func PostExect(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostExecT")
	body, err := io.ReadAll(r.Body)
	if err == nil {
		var args common.PreciseTarget
		var err = json.Unmarshal(body, &args)
		if err == nil {
			var reply common.ResultMsg
			reply, err = orgs.ExecTableAt(db, &args)
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

func PostExecAllT(w http.ResponseWriter, r *http.Request) {
	fmt.Println("PostExecAllT")
	body, err := io.ReadAll(r.Body)
	if err == nil {
		var args string
		var err = json.Unmarshal(body, &args)
		if err == nil {
			reply, errs := orgs.ExecAllTables(db, args)
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

// Get the valid status list for a given node (takes into account global and file status lists)
func RequestValidStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var args common.TodoHash = common.TodoHash(h)
		res, err := orgs.ValidStatus(&args)
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

func RequestNextSibling(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var args common.TodoHash = common.TodoHash(h)
		res := orgs.NextSibling(&args)
		if res == nil {
			json.NewEncoder(w).Encode(fmt.Sprintf("could not get next sibling for hash %s", args))
		} else {
			json.NewEncoder(w).Encode(res)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

func RequestPrevSibling(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var args common.TodoHash = common.TodoHash(h)
		res := orgs.PrevSibling(&args)
		if res == nil {
			json.NewEncoder(w).Encode(fmt.Sprintf("could not get prev sibling for hash %s", args))
		} else {
			json.NewEncoder(w).Encode(res)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

func RequestLastChild(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var args common.TodoHash = common.TodoHash(h)
		res := orgs.LastChild(&args)
		if res == nil {
			json.NewEncoder(w).Encode(fmt.Sprintf("could not get last child for hash %s", args))
		} else {
			json.NewEncoder(w).Encode(res)
		}
	} else {
		json.NewEncoder(w).Encode(err)
	}
}

func RequestTags(w http.ResponseWriter, r *http.Request) {
	res := orgs.GetDb().GetAllTags()
	if res == nil {
		json.NewEncoder(w).Encode("could not get tags list")
	} else {
		json.NewEncoder(w).Encode(res)
	}
}
