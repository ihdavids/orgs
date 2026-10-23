package orgs

// What worg's capture template editor (CaptureTemplateEditor.tsx) needs beyond
// the per-user store in extensions.go: every template a user sees with where
// it comes from, whether a target is somewhere a capture can land, and the
// names a capture template fills in by itself with what they would be now.
//
// Templates come from three places, merged by name in QueryCaptureTemplates:
// captureTemplates: in the yaml (the server's, for everybody), the user's own
// (the extensions file), and the built-in ones (snippets, go links). Only the
// user's own are written from here. Editing one of the others saves a copy of
// your own under the same name, which wins over it; deleting that copy brings
// it back. The yaml is the server owner's file, and it is not rewritten over
// the wire.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/templates"
)

/* SDOC: API

  - GET /ext/capture/templates/all — Every Capture Template, With Where It Comes From
    Every template the user captures with, as written (nothing filled in),
    each as ={Template, Source, Shadows}=: =Source= is =mine=, =config= (the
    yaml's =captureTemplates:=) or =builtin=; =Shadows= is the source of the
    template one of yours replaces by having its name.

  - POST /capture/target/check — Where a Capture Would Land
    *Body:* a =Target= (={Type, Filename, Id, Lvl}=). *Response:*
    ={Ok, Msg, File, FileExists, Found, Root}=: =File= is the file as the server
    resolves it, =Root= the first org directory a relative filename is read
    from, and =Msg= says in words what a capture would do - file under
    the heading, make the file, make the heading. Nothing is written.

  - GET /capture/words — The Names a Capture Template Answers Itself
    ={Name, Doc, Value, Alias}= for each of ={{date}}=, ={{now}}=, ={{uuid}}=,
    ={{daypage}}= and the rest, with what they would be now.
EDOC */

type captureEntry struct {
	Template common.CaptureTemplate
	Source   string
	Shadows  string `json:",omitempty"`
}

func RequestAllCaptureTemplates(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	var config []common.CaptureTemplate
	if Conf().Server != nil {
		config = Conf().Server.CaptureTemplates
	}
	builtin := builtinCaptureTemplates()
	var mine []common.CaptureTemplate
	if username != "" && GetExtensions() != nil {
		mine = GetExtensions().GetUserCaptureTemplates(username)
	}
	sourceOf := map[string]string{}
	for _, t := range builtin {
		sourceOf[t.Name] = "builtin"
	}
	for _, t := range config {
		sourceOf[t.Name] = "config"
	}
	out := []captureEntry{}
	have := map[string]bool{}
	for _, t := range mine {
		out = append(out, captureEntry{Template: t, Source: "mine", Shadows: sourceOf[t.Name]})
		have[t.Name] = true
	}
	for _, t := range config {
		if !have[t.Name] {
			out = append(out, captureEntry{Template: t, Source: "config"})
			have[t.Name] = true
		}
	}
	for _, t := range builtin {
		if !have[t.Name] {
			out = append(out, captureEntry{Template: t, Source: "builtin"})
			have[t.Name] = true
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

type targetCheck struct {
	Ok         bool
	Msg        string
	File       string `json:",omitempty"`
	FileExists bool
	Found      bool
	Root       string `json:",omitempty"`
}

func RequestCaptureTargetCheck(w http.ResponseWriter, r *http.Request) {
	var t common.Target
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		answerErr(w, err)
		return
	}
	res := checkCaptureTarget(t, GetUsername(r))
	if Conf().Server != nil && len(Conf().Server.OrgDirs) > 0 {
		res.Root, _ = filepath.Abs(Conf().Server.OrgDirs[0])
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

// checkCaptureTarget says what a capture with this target would do, without
// doing any of it: GetFromTarget is asked with allowCreate off, and only for
// a file that is there (with a missing file, file+regexp has nothing to read).
func checkCaptureTarget(t common.Target, username string) targetCheck {
	kind := strings.ToLower(strings.TrimSpace(t.Type))
	switch kind {
	case "clock":
		return targetCheck{Ok: true, Found: true, Msg: "Under whatever heading the clock is running on"}
	case "id", "customid":
		if strings.TrimSpace(t.Id) == "" {
			return targetCheck{Msg: "Give the " + strings.ToUpper(kind) + " of the heading to file under"}
		}
		registerAllSections()
		if file, sec := GetDb().GetFromTarget(&t, false); file != nil && sec != nil {
			return targetCheck{Ok: true, Found: true, FileExists: true, File: file.Filename,
				Msg: "Under " + common.GetSectionTitle(sec) + " in " + filepath.Base(file.Filename)}
		}
		return targetCheck{Msg: "No heading has that " + strings.ToUpper(kind) + "; a capture would fail"}
	case "file", "file+headline", "file+olp", "file+datetree", "file+olp+datetree", "file+regexp":
	case "hash":
		return targetCheck{Msg: "A hash changes whenever its file is edited; name the heading by id or by file and heading instead"}
	default:
		return targetCheck{Msg: "Not a kind of target orgs knows: " + t.Type}
	}
	if strings.TrimSpace(t.Filename) == "" {
		return targetCheck{Msg: "Name the file it goes into"}
	}
	fn, err := common.CapExpandFilename(t.Filename, time.Now(), username)
	if err != nil {
		return targetCheck{Msg: err.Error()}
	}
	path := GetDb().GetFilepath(fn)
	// A capture may go anywhere (day pages often live outside the org
	// directories); it is only said, since search and the agenda will not see it.
	outside := ""
	if _, ok := insideOrgDirs(path); !ok {
		outside = " - outside the org directories, so search and the agenda will not see it"
	}
	ret := func(c targetCheck) targetCheck { c.Msg += outside; return c }
	t.Filename = fn
	file := GetDb().FindByFile(fn)
	if _, err := os.Stat(path); err != nil || file == nil {
		msg := filepath.Base(path) + " is not there yet; the first capture makes it"
		if kind == "file+headline" {
			msg += ", with a " + t.Id + " heading"
		}
		if kind == "file+regexp" {
			msg = filepath.Base(path) + " is not there, and nothing in it can match yet; a capture would fail"
			return ret(targetCheck{Msg: msg, File: path})
		}
		return ret(targetCheck{Ok: true, Msg: msg, File: path})
	}
	where := filepath.Base(path)
	switch kind {
	case "file":
		return ret(targetCheck{Ok: true, Found: true, FileExists: true, File: path, Msg: "At the end of " + where})
	case "file+datetree":
		return ret(targetCheck{Ok: true, Found: true, FileExists: true, File: path, Msg: "Under today's date in " + where + ", made if it is not there"})
	case "file+regexp":
		if _, err := regexp.Compile(t.Id); err != nil {
			return ret(targetCheck{FileExists: true, File: path, Msg: "Not a pattern: " + err.Error()})
		}
	}
	if strings.TrimSpace(t.Id) == "" {
		return ret(targetCheck{FileExists: true, File: path, Msg: "Name the heading in " + where + " to file under"})
	}
	if _, sec := GetDb().GetFromTarget(&t, false); sec != nil {
		msg := "Under " + common.GetSectionTitle(sec) + " in " + where
		if kind == "file+olp+datetree" {
			msg += ", under today's date"
		}
		return ret(targetCheck{Ok: true, Found: true, FileExists: true, File: path, Msg: msg})
	}
	switch kind {
	case "file+headline":
		return ret(targetCheck{Ok: true, FileExists: true, File: path, Msg: where + " has no " + t.Id + " heading yet; the first capture adds it"})
	case "file+regexp":
		return ret(targetCheck{FileExists: true, File: path, Msg: "No heading in " + where + " matches; a capture would fail"})
	}
	return ret(targetCheck{Ok: true, FileExists: true, File: path, Msg: "The outline " + strings.ReplaceAll(t.Id, "::", " › ") + " is not in " + where + " yet; the first capture makes it"})
}

type captureWord struct {
	Name  string
	Doc   string
	Value string `json:",omitempty"`
	Alias string `json:",omitempty"`
}

func RequestCaptureWords(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	now := time.Now()
	docs := map[string]string{}
	for _, wd := range Conf().PlugManager.Tempo.Vocabulary() {
		if wd.Kind == "value" && wd.Alias == "" {
			docs[wd.Name] = wd.Doc
		}
	}
	docs["daypage"] = "Today's day page, from the first org directory - a filename: of {{daypage}} captures into it"
	docs["username"] = "Whoever is capturing"
	names := append([]string{}, templates.AutoNames...)
	names = append(names, "username", "daypage")
	aliases := []string{}
	for a := range templates.Aliases {
		aliases = append(aliases, a)
	}
	sort.Strings(aliases)
	out := []captureWord{}
	for _, n := range names {
		v, _ := common.CapAutoValue(n, now, username)
		out = append(out, captureWord{Name: n, Doc: docs[n], Value: v})
	}
	for _, a := range aliases {
		c := templates.Aliases[a]
		v, _ := common.CapAutoValue(a, now, username)
		out = append(out, captureWord{Name: a, Doc: docs[c], Value: v, Alias: c})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
