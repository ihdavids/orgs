package orgs

// Editing the templates a new org file starts from, for worg's template
// editor (TemplateEditor.tsx). The templates are the `new/*.tpl` files on the
// template path plus the "Plain" one (`newFileTemplate:`, newfile.tpl unless
// the config says otherwise), the same ones /newtemplates lists and POST /file
// renders.
//
// Every write goes to the configured template folder (`templatePath`, which is
// searched first), never to the built-in copy: saving a built-in template makes
// your own copy that wins over it, and deleting that copy puts the built-in one
// back. A template is refused when it does not parse - a broken one would make
// every new file from it the complaint - or names a file outside the template
// folders (templates.UnsafeTags), since anybody logged in can write one.

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ihdavids/orgs/internal/assets"
	"github.com/ihdavids/orgs/internal/templates"
)

/* SDOC: API

  - GET /newtemplate/list — New-File Templates, With Where Each Comes From
    The templates =/newtemplates= names, each with whether it is built in,
    whether this server has its own copy, and which one is the Plain
    (=newFileTemplate=) one. The Plain template comes first.

    *Response:* a JSON array of ={Name, Label, Plain, Builtin, Edited, Writable}=.

  - GET /newtemplate?name=new/notes.tpl — One Template's Text
    *Response:* ={Name, Label, Plain, Builtin, Edited, Writable, Text, Path}=;
    =Path= is where saving writes it.

  - POST /newtemplate — Save a Template
    *Body:* ={Name, Text, Base, From}=. =Name= is =new/<name>.tpl= or a bare
    name; =Base= is the text the editor started from, and the save is refused
    (=409=) when the template has changed since; =From=, when it differs from
    =Name=, is the template this one is renamed from, whose own copy is
    removed. Refused (=400=) when the text does not parse, with =Line= and
    =Col=, or uses ={% ssi %}= or includes a file outside the template folders.

  - DELETE /newtemplate?name=new/notes.tpl — Delete a Template
    Removes this server's own copy; a built-in template of that name is used
    again. A built-in template with no copy cannot be deleted.

  - POST /newtemplate/preview — Render a Template That Is Not Saved
    *Body:* ={Name, Text, Title, Filename, Answers}=. *Response:* ={Ok, Text,
    Fields}= (=Fields= as =/newtemplate/fields= reads the text), or ={Ok: false,
    Msg, Line, Col, Fields}= saying where it broke.

  - GET /newtemplate/fields?name=new/meeting.tpl — What a Template Asks
    The template's =ask("Client", "Acme")= questions, in order, each once (an
    included template's in its place): ={Name, Default}=. The New File dialog
    shows a box for each and sends the answers as =answers= on =POST /file=.

  - GET /newtemplate/words — What a Template Can Name
    Every value (with what it would be now), function and filter, with what it
    does, for an editor to complete and explain.
EDOC */

type fileTemplateInfo struct {
	Name     string
	Label    string
	Plain    bool
	Builtin  bool
	Edited   bool
	Writable bool
	Text     string `json:",omitempty"`
	Path     string `json:",omitempty"`
}

type fileTemplateSave struct {
	Name string
	Text string
	Base *string
	From string
}

type fileTemplateAnswer struct {
	Ok     bool
	Msg    string               `json:",omitempty"`
	Line   int                  `json:",omitempty"`
	Col    int                  `json:",omitempty"`
	Text   string               `json:",omitempty"`
	Name   string               `json:",omitempty"`
	Fields []templates.AskField `json:",omitempty"`
}

// The folder templates are written to: the configured one.
func templateWriteDir() string {
	d := "./templates"
	if Conf().Server != nil && strings.TrimSpace(Conf().Server.TemplatePath) != "" {
		d = Conf().Server.TemplatePath
	}
	abs, err := filepath.Abs(d)
	if err != nil {
		return d
	}
	return abs
}

// The Plain template's name, when it is one that can be edited here (a path
// inside the template folder, not somewhere else on the disk).
func plainTemplate() (string, bool) {
	n := filepath.ToSlash(strings.TrimSpace(Conf().NewFileTemplate))
	if n == "" || filepath.IsAbs(n) || strings.Contains(n, "..") || !strings.HasSuffix(n, ".tpl") {
		return n, false
	}
	return n, true
}

var templateBase = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} _.-]{0,79}$`)

// The name a request means (new/x.tpl, or the Plain template's), and whether
// it is one that may be written.
func templateName(raw string) (string, error) {
	n := filepath.ToSlash(strings.TrimSpace(raw))
	if plain, ok := plainTemplate(); ok && n == plain {
		return n, nil
	}
	n = strings.TrimSuffix(strings.TrimPrefix(n, "new/"), ".tpl")
	if !templateBase.MatchString(n) || strings.HasSuffix(n, ".") || strings.Contains(n, "..") {
		return "", errors.New("a template's name is letters, digits, spaces, - _ and . (not at the start)")
	}
	return "new/" + n + ".tpl", nil
}

func templateLabel(name string) string {
	if plain, _ := plainTemplate(); name == plain {
		return "Plain"
	}
	base := strings.TrimSuffix(strings.TrimPrefix(name, "new/"), ".tpl")
	if base == "" {
		return name
	}
	return strings.ToUpper(base[:1]) + base[1:]
}

func fileTemplateOf(name string, withText bool) fileTemplateInfo {
	plain, plainOk := plainTemplate()
	info := fileTemplateInfo{Name: name, Label: templateLabel(name), Plain: name == plain}
	builtin := filepath.Join(assets.Dir("templates"), filepath.FromSlash(name))
	if st, err := os.Stat(builtin); err == nil && !st.IsDir() {
		info.Builtin = true
	}
	for _, d := range assets.UserTemplateDirs(Conf().Server.TemplatePath) {
		if st, err := os.Stat(filepath.Join(d, filepath.FromSlash(name))); err == nil && !st.IsDir() {
			info.Edited = true
			break
		}
	}
	info.Writable = !info.Plain || plainOk
	if withText {
		info.Path = filepath.Join(templateWriteDir(), filepath.FromSlash(name))
		if p := assets.FindTemplate(name, Conf().Server.TemplatePath); p != "" {
			if b, err := os.ReadFile(p); err == nil {
				info.Text = string(b)
			}
		}
	}
	return info
}

func answerTemplate(w http.ResponseWriter, status int, a fileTemplateAnswer) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(a)
}

func RequestFileTemplateList(w http.ResponseWriter, r *http.Request) {
	out := []fileTemplateInfo{}
	if plain, _ := plainTemplate(); plain != "" {
		out = append(out, fileTemplateOf(plain, false))
	}
	for _, n := range assets.GlobTemplates("new/*.tpl", Conf().Server.TemplatePath) {
		out = append(out, fileTemplateOf(n, false))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func RequestFileTemplate(w http.ResponseWriter, r *http.Request) {
	name, err := templateName(r.URL.Query().Get("name"))
	if err != nil {
		answerTemplate(w, http.StatusBadRequest, fileTemplateAnswer{Msg: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fileTemplateOf(name, true))
}

func PostFileTemplate(w http.ResponseWriter, r *http.Request) {
	var req fileTemplateSave
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		answerTemplate(w, http.StatusBadRequest, fileTemplateAnswer{Msg: "Invalid request: " + err.Error()})
		return
	}
	name, err := templateName(req.Name)
	if err != nil {
		answerTemplate(w, http.StatusBadRequest, fileTemplateAnswer{Msg: err.Error()})
		return
	}
	info := fileTemplateOf(name, true)
	if !info.Writable {
		answerTemplate(w, http.StatusBadRequest, fileTemplateAnswer{Msg: "newFileTemplate names a file outside the template folder; edit it there"})
		return
	}
	from := ""
	if strings.TrimSpace(req.From) != "" {
		if from, err = templateName(req.From); err != nil {
			answerTemplate(w, http.StatusBadRequest, fileTemplateAnswer{Msg: err.Error()})
			return
		}
	}
	renaming := from != "" && from != name
	// Renamed onto a template that is already there would quietly replace it.
	if renaming && (info.Edited || info.Builtin) {
		answerTemplate(w, http.StatusConflict, fileTemplateAnswer{Msg: "there is already a template called " + info.Label})
		return
	}
	// Somebody else (another tab, an editor on the server) saved it since this
	// one was opened: say so rather than throw their change away.
	if !renaming && req.Base != nil && *req.Base != info.Text {
		answerTemplate(w, http.StatusConflict, fileTemplateAnswer{Msg: info.Label + " has changed since you opened it; reload it to see what changed"})
		return
	}
	if _, e := Conf().PlugManager.Tempo.RenderDraft(info.Path, req.Text, newFileContext(exampleFile("example"), "Example", nil, true)); e != nil {
		answerTemplate(w, http.StatusBadRequest, fileTemplateAnswer{Msg: e.Msg, Line: e.Line, Col: e.Col})
		return
	}
	if err := os.MkdirAll(filepath.Dir(info.Path), 0o755); err != nil {
		answerErr(w, err)
		return
	}
	if err := os.WriteFile(info.Path, []byte(req.Text), 0o644); err != nil {
		answerErr(w, err)
		return
	}
	if renaming {
		old := filepath.Join(templateWriteDir(), filepath.FromSlash(from))
		if err := os.Remove(old); err != nil && !errors.Is(err, fs.ErrNotExist) {
			answerTemplate(w, http.StatusOK, fileTemplateAnswer{Ok: true, Name: name, Msg: "saved, but the old one could not be removed: " + err.Error()})
			return
		}
	}
	answerTemplate(w, http.StatusOK, fileTemplateAnswer{Ok: true, Name: name})
}

func DeleteFileTemplate(w http.ResponseWriter, r *http.Request) {
	name, err := templateName(r.URL.Query().Get("name"))
	if err != nil {
		answerTemplate(w, http.StatusBadRequest, fileTemplateAnswer{Msg: err.Error()})
		return
	}
	info := fileTemplateOf(name, true)
	if err := os.Remove(info.Path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			msg := "there is no " + info.Label + " template here to delete"
			if info.Builtin {
				msg = info.Label + " is built in; there is nothing of yours to delete"
			}
			answerTemplate(w, http.StatusNotFound, fileTemplateAnswer{Msg: msg})
			return
		}
		answerErr(w, err)
		return
	}
	answerTemplate(w, http.StatusOK, fileTemplateAnswer{Ok: true, Name: name})
}

type fileTemplatePreview struct {
	Name     string
	Text     string
	Title    string
	Filename string
	Answers  map[string]string
}

func PostFileTemplatePreview(w http.ResponseWriter, r *http.Request) {
	var req fileTemplatePreview
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		answerTemplate(w, http.StatusBadRequest, fileTemplateAnswer{Msg: "Invalid request: " + err.Error()})
		return
	}
	path := filepath.Join(templateWriteDir(), "new", "draft.tpl")
	if name, err := templateName(req.Name); err == nil {
		path = filepath.Join(templateWriteDir(), filepath.FromSlash(name))
	}
	fname := exampleFile(req.Filename)
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = titleFromFilename(fname)
	}
	fields := templates.AskFields(req.Text, path)
	out, e := Conf().PlugManager.Tempo.RenderDraft(path, req.Text, newFileContext(fname, title, req.Answers, true))
	if e != nil {
		answerTemplate(w, http.StatusOK, fileTemplateAnswer{Msg: e.Msg, Line: e.Line, Col: e.Col, Fields: fields})
		return
	}
	answerTemplate(w, http.StatusOK, fileTemplateAnswer{Ok: true, Text: out, Fields: fields})
}

func RequestFileTemplateFields(w http.ResponseWriter, r *http.Request) {
	fields := []templates.AskField{}
	name := r.URL.Query().Get("name")
	if name == "" {
		name = Conf().NewFileTemplate
	}
	if p := assets.FindTemplate(name, Conf().Server.TemplatePath); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			fields = templates.AskFields(string(b), p)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fields)
}

// The file a preview pretends to make: what the editor typed, in the first
// org directory unless it gave a whole path.
func exampleFile(name string) string {
	fname := strings.TrimSpace(name)
	if fname == "" {
		fname = "example"
	}
	if !strings.HasSuffix(fname, ".org") {
		fname += ".org"
	}
	if !filepath.IsAbs(fname) && len(Conf().Server.OrgDirs) > 0 {
		fname = filepath.Join(Conf().Server.OrgDirs[0], fname)
	}
	return fname
}

func RequestFileTemplateWords(w http.ResponseWriter, r *http.Request) {
	words := Conf().PlugManager.Tempo.Vocabulary()
	if words == nil {
		words = []templates.Word{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(words)
}
