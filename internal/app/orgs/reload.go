//lint:file-ignore ST1006 allow the use of self
package orgs

/* SDOC: API
* POST /reloadconfig — Reload The Configuration
	Reads the server's yaml file again and takes from it everything that can
	change under a running server: capture templates, filters, tag groups,
	refile targets, goLinksFile/goLinksHeading, todo states, day page, archive, date tree and clock
	settings, the template path, babel, columns, log and the dnd paths.

	What a running server is built around is left as it is, and named in
	=needsRestart= when the file now says something different: ports,
	certificates, orgDirs, auth and keystore settings, voice, and the plugin,
	exporter and updater lists (a plugin is started once, with its settings).

	A file that does not parse changes nothing.

	*Method:* =POST=

	*Parameters:* None.

	*Response:*
	#+BEGIN_SRC json
	{"ok": true, "msg": "configuration reloaded", "file": "/home/me/orgs.yaml",
	 "reloaded": ["captureTemplates", "filters", ...],
	 "needsRestart": ["port"]}
	#+END_SRC
EDOC */

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"

	"github.com/ihdavids/orgs/internal/common"
	"gopkg.in/yaml.v2"
)

func PostReloadConfig(w http.ResponseWriter, r *http.Request) {
	res, err := ReloadConfig()
	status := http.StatusOK
	if err != nil {
		res.Ok = false
		res.Msg = err.Error()
		status = http.StatusConflict
	}
	fmt.Fprintf(os.Stderr, "RELOAD CONFIG (%s): %s\n", GetUsername(r), res.Msg)
	AccessControl(&w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(res)
}

// ReloadConfig reads the configuration file again into a fresh Config and
// copies across the settings that are read where they are used. The fresh copy
// is never started: its plugins are made by the unmarshal and dropped.
func ReloadConfig() (common.ReloadResult, error) {
	cur := Conf()
	res := common.ReloadResult{File: cur.loadedFrom}
	if cur.loadedFrom == "" {
		return res, fmt.Errorf("this server was not started from a configuration file, so there is nothing to reload")
	}
	data, err := ioutil.ReadFile(cur.loadedFrom)
	if err != nil {
		return res, fmt.Errorf("reading %s: %v", cur.loadedFrom, err)
	}
	fresh := new(Config)
	fresh.Server = &common.ServerSettings{}
	fresh.Defaults()
	if err := yaml.Unmarshal(data, fresh); err != nil {
		return res, fmt.Errorf("%s did not parse, nothing was changed: %v", cur.loadedFrom, err)
	}
	if fresh.Server == nil {
		return res, fmt.Errorf("%s has no server: section, nothing was changed", cur.loadedFrom)
	}
	if !fresh.NoInternalFilters {
		fresh.AddInternalFilters()
	}
	if !fresh.NoInternalTagGroups {
		fresh.AddInternalTagGroups()
	}

	o, n := cur.Server, fresh.Server
	res.NeedsRestart = restartOnly(&cur.loadedServer, n)

	// Top level.
	cur.Author = fresh.Author
	cur.NewFileTemplate = fresh.NewFileTemplate
	cur.ArchiveDefaultTarget = fresh.ArchiveDefaultTarget
	cur.ArchiveSaveContextInfo = fresh.ArchiveSaveContextInfo
	cur.ArchiveSkipEmptyProperties = fresh.ArchiveSkipEmptyProperties
	cur.ArchiveMarkDone = fresh.ArchiveMarkDone
	cur.DateTreeYearFormat = fresh.DateTreeYearFormat
	cur.DateTreeMonthFormat = fresh.DateTreeMonthFormat
	cur.DateTreeDayFormat = fresh.DateTreeDayFormat
	cur.ClockIntoDrawer = fresh.ClockIntoDrawer
	cur.TemplateImagesPath = fresh.TemplateImagesPath
	cur.TemplateFontPath = fresh.TemplateFontPath
	cur.NoInternalFilters = fresh.NoInternalFilters
	cur.NoInternalTagGroups = fresh.NoInternalTagGroups
	cur.Filters = fresh.Filters
	cur.TagGroups = fresh.TagGroups
	cur.Aliases = fresh.Aliases
	cur.EditorTemplate = fresh.EditorTemplate

	// Server.
	o.CaptureTemplates = n.CaptureTemplates
	o.RefileTargets = n.RefileTargets
	o.GoLinksFile = n.GoLinksFile
	o.GoLinksHeading = n.GoLinksHeading
	o.DefaultTodoStates = n.DefaultTodoStates
	o.DefaultNextStates = n.DefaultNextStates
	o.UseTagForProjects = n.UseTagForProjects
	o.TemplatePath = n.TemplatePath
	o.DayPageTemplate = n.DayPageTemplate
	o.DayPagePath = n.DayPagePath
	o.DayPageMode = n.DayPageMode
	o.DayPageModeWeekDay = n.DayPageModeWeekDay
	o.DayPageMaxSearchBack = n.DayPageMaxSearchBack
	o.DndPaths = n.DndPaths
	o.DndSessionPath = n.DndSessionPath
	o.DndSessionTemplate = n.DndSessionTemplate
	o.AccessControl = n.AccessControl
	o.Log = n.Log
	o.Columns = n.Columns
	o.Babel = n.Babel

	// The plugin manager holds its own copies, and every plugin holds the
	// manager, so updating it here reaches them all.
	if m := cur.PlugManager; m != nil {
		m.Filters = cur.Filters
		m.TagGroups = cur.TagGroups
		if m.Tempo != nil {
			m.Tempo.TemplatePath = o.TemplatePath
		}
	}

	res.Reloaded = []string{
		"author", "newFileTemplate", "archive*", "dateTree*", "clockIntoDrawer",
		"templateImagesPath", "templateFontPath", "filters", "tagGroups", "aliases", "editorTemplate",
		"server.captureTemplates", "server.refileTargets", "server.goLinks*", "server.defaultTodoStates",
		"server.defaultNextStates", "server.useProjectTag", "server.templatePath", "server.dayPage*",
		"server.dndPaths", "server.dndSession*", "server.accessControl", "server.log",
		"server.columns", "server.babel",
	}
	res.Ok = true
	res.Msg = "configuration reloaded"
	if len(res.NeedsRestart) > 0 {
		res.Msg += fmt.Sprintf("; %d changed setting(s) need a restart", len(res.NeedsRestart))
	}
	return res, nil
}

// restartOnly names the settings the file now says differently from when the
// server started, that a running server cannot take up. Against the start
// rather than the last reload, so a pending restart is still named next time. The jws/jwe keys and salt are not compared: left out
// of the file they are random in every fresh read, and a difference would
// mean nothing.
func restartOnly(o, n *common.ServerSettings) []string {
	var out []string
	diff := func(name string, a, b interface{}) {
		if !reflect.DeepEqual(a, b) {
			out = append(out, name)
		}
	}
	diff("server.port", o.Port, n.Port)
	diff("server.tlsport", o.TLSPort, n.TLSPort)
	diff("server.servercrt", o.ServerCrt, n.ServerCrt)
	diff("server.serverkey", o.ServerKey, n.ServerKey)
	diff("server.servepath", o.ServePath, n.ServePath)
	diff("server.orgDirs", o.OrgDirs, n.OrgDirs)
	diff("server.allowHttp", o.AllowHttp, n.AllowHttp)
	diff("server.allowHttps", o.AllowHttps, n.AllowHttps)
	diff("server.noAuth", o.NoAuth, n.NoAuth)
	diff("server.requireHashedLogin", o.RequireHashedLogin, n.RequireHashedLogin)
	diff("server.keystore", o.Keystore, n.Keystore)
	diff("server.tokenExpiry", o.TokenExpiry, n.TokenExpiry)
	diff("server.voice", o.Voice, n.Voice)
	diff("server.attach", o.Attach, n.Attach)
	diff("server.crypt", o.Crypt, n.Crypt)
	diff("server.plugins", pluginNames(o.Plugins), pluginNames(n.Plugins))
	diff("server.exporters", exporterNames(o.Exporters), exporterNames(n.Exporters))
	diff("server.updaters", updaterNames(o.Updaters), updaterNames(n.Updaters))
	return out
}

func pluginNames(ps []common.PluginDef) []string {
	out := []string{}
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

func exporterNames(ps []common.ExportDef) []string {
	out := []string{}
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

func updaterNames(ps []common.UpdaterDef) []string {
	out := []string{}
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}
