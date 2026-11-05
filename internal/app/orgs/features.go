package orgs

// Optional features: org packages orgs does the work of, each off until it is
// turned on. One switch per package, kept in the yaml's `features:` block and
// flipped from any client through POST /features, which writes the yaml the
// way POST /transclusion does - so worg's settings, `orgs features` and the
// VS Code command all turn on the same thing.
//
// A feature that is off does nothing at all: its properties are left alone,
// its endpoints answer `{Ok: false, Off: true}` and its background work does
// not start. That is what makes them safe to ship in one binary with people
// whose files already carry a TRIGGER or EXPIRY property for some other
// reason.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

// Feature describes one optional feature for a client to list.
type Feature struct {
	Name   string
	Title  string
	Org    string // the Emacs package it does the work of
	About  string
	Enable bool
}

type featureDef struct {
	name, title, org, about string
	get                     func(*common.FeatureSettings) *bool
}

var featureDefs = []featureDef{
	{"checklistReset", "Reset checkboxes on repeat", "org-checklist",
		"A repeating heading with :RESET_CHECK_BOXES: t has every checkbox in its subtree unticked when it comes round again.",
		func(f *common.FeatureSettings) *bool { return &f.ChecklistReset }},
	{"edna", "Triggers and blockers", "org-edna",
		"TRIGGER properties act on other headings when a heading is done; BLOCKER properties stop a heading being done until other headings are.",
		func(f *common.FeatureSettings) *bool { return &f.Edna }},
	{"noter", "PDF notes", "org-noter",
		"Notes on a PDF kept under a heading with :NOTER_DOCUMENT:, each note at its :NOTER_PAGE:, read beside the document.",
		func(f *common.FeatureSettings) *bool { return &f.Noter }},
	{"remark", "Highlights and margin notes", "org-remark",
		"Highlight a passage of any file or web page and keep a note on it in a marginalia file.",
		func(f *common.FeatureSettings) *bool { return &f.Remark }},
	{"expiry", "Expiring headings", "org-expiry",
		"A heading with an :EXPIRY: date (or +2w from its CREATED) is archived, tagged or given a keyword once that date passes.",
		func(f *common.FeatureSettings) *bool { return &f.Expiry }},
	{"autoArchive", "Archive old done items", "org-archive",
		"Headings finished longer ago than autoArchive.after are archived on a schedule.",
		func(f *common.FeatureSettings) *bool { return &f.AutoArchive }},
}

func featureDefOf(name string) *featureDef {
	for i := range featureDefs {
		if strings.EqualFold(featureDefs[i].name, name) {
			return &featureDefs[i]
		}
	}
	return nil
}

// featureOn is whether the named feature is turned on.
func featureOn(name string) bool {
	s := Conf().Server
	d := featureDefOf(name)
	if s == nil || d == nil {
		return false
	}
	return *d.get(&s.Features)
}

func featureList() []Feature {
	out := []Feature{}
	for _, d := range featureDefs {
		out = append(out, Feature{Name: d.name, Title: d.title, Org: d.org, About: d.about, Enable: featureOn(d.name)})
	}
	return out
}

// featureOff answers a request for a feature that is turned off, the same
// way from every endpoint, so a client can tell "off" from "failed".
func featureOff(w http.ResponseWriter, name string) bool {
	if featureOn(name) {
		return false
	}
	d := featureDefOf(name)
	title := name
	if d != nil {
		title = d.title + " (" + d.org + ")"
	}
	dynJson(w, map[string]interface{}{"Ok": false, "Off": true,
		"Msg": fmt.Sprintf("%s is turned off on this server: turn it on with `orgs features %s on` or in worg's settings", title, name)})
	return true
}

// setFeature turns a feature on or off now and writes it into the yaml. The
// error is only about the yaml: the feature is set either way.
func setFeature(name string, on bool) (string, error) {
	d := featureDefOf(name)
	if d == nil || Conf().Server == nil {
		return "", fmt.Errorf("no feature called %q", name)
	}
	*d.get(&Conf().Server.Features) = on
	featureChanged(d.name, on)
	file, _, err := editConfigFile(".before-features", func(lines []string) ([]string, error) {
		return setYamlBool(lines, []string{"server", "features", d.name}, on)
	})
	Publish(Event{Kind: "features", Msg: fmt.Sprintf("%s %s", d.name, map[bool]string{true: "on", false: "off"}[on])})
	if err != nil {
		return "", fmt.Errorf("on until the server restarts: could not write %s: %v", filepath.Base(file), err)
	}
	return file, nil
}

// setYamlBool is setYamlPath for a bool: setYamlPath quotes what it writes,
// and "true" is a string to the yaml reader, not a bool.
func setYamlBool(lines []string, path []string, on bool) ([]string, error) {
	v := strconv.FormatBool(on)
	out, err := setYamlPath(lines, path, v)
	if err != nil {
		return nil, err
	}
	leaf := path[len(path)-1] + `: "` + v + `"`
	for i, l := range out {
		if strings.TrimSpace(l) == leaf {
			out[i] = strings.Replace(l, `"`+v+`"`, v, 1)
		}
	}
	return out, nil
}

// featureChanged starts or stops what a feature runs in the background.
func featureChanged(name string, on bool) {
	switch name {
	case "expiry", "autoArchive":
		startExpiry()
	}
}

/* SDOC: API
* GET /features — The Optional Features
* POST /features — Turn One On Or Off

	#+BEGIN_EXAMPLE
	GET  /features   {"Ok": true, "Features": [{"Name": "edna", "Title": "Triggers and blockers",
	                  "Org": "org-edna", "About": "...", "Enable": false}, ...]}
	POST /features   {"Name": "edna", "Enable": true}
	#+END_EXAMPLE

	The POST takes effect at once, is written into the server's yaml
	(=server: features: edna: true=, keeping the file as it was in
	=<file>.before-features=) and sends a =features= event. Only an
	administrator may turn one on or off, unless the server runs with =noAuth=.
EDOC */
func RequestFeatures(w http.ResponseWriter, r *http.Request) {
	type answer struct {
		Ok       bool
		Msg      string `json:",omitempty"`
		Features []Feature
	}
	if Conf().Server == nil {
		dynJson(w, answer{Msg: "no server settings", Features: []Feature{}})
		return
	}
	if r.Method == http.MethodPost {
		who := GetUsername(r)
		if !Conf().Server.NoAuth && !GetKeystore().IsAdmin(who) {
			w.WriteHeader(http.StatusForbidden)
			dynJson(w, answer{Msg: "only an administrator may turn features on or off", Features: featureList()})
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Name   string
			Enable bool
		}
		if err := json.Unmarshal(body, &req); err != nil {
			dynJson(w, answer{Msg: err.Error(), Features: featureList()})
			return
		}
		if featureDefOf(req.Name) == nil {
			dynJson(w, answer{Msg: fmt.Sprintf("no feature called %q", req.Name), Features: featureList()})
			return
		}
		msg := ""
		if _, err := setFeature(req.Name, req.Enable); err != nil {
			msg = err.Error()
		}
		dynJson(w, answer{Ok: true, Msg: msg, Features: featureList()})
		return
	}
	dynJson(w, answer{Ok: true, Features: featureList()})
}
