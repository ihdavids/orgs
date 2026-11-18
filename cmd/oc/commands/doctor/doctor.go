package doctor

// orgs doctor - why is it not working.
//
//	orgs doctor          check everything and say what is wrong
//	orgs doctor -json    for a script
//
// Three failures look identical from the outside and have completely different
// fixes: the server is not running, the server is running and the token has
// expired, and the server is running and this command's exporter was never put
// in the yaml. Each of them answers "could not" and none of them says which.
//
// So this asks every question at once and prints the answers together. It is
// the first thing to run when something is not working and the first thing to
// paste into a bug report.
//
// It never writes anything and never fails: a check that cannot be run is a
// finding, not an error, and `orgs doctor` refusing to run would be the joke
// that writes itself.

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Doctor struct {
	Verbose bool
}

func (self *Doctor) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Doctor) StartPlugin(m *common.PluginManager)       {}
func (self *Doctor) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.Verbose, "v", false, "say the checks that passed as well")
}

// NeedsNoServer, which is not quite true - it asks one plenty - but it must not
// be *stopped* by the checks on the way in. An expired token and an unreachable
// server are two of the things this command is for, and being refused by the
// first of them before printing anything is the one outcome that helps nobody.
func (self *Doctor) NeedsNoServer() bool { return true }

// A finding. Level is what to do about it rather than how bad it sounds:
// "ok" needs nothing, "note" is worth knowing, "warn" will bite later, "fail"
// is why the thing you tried did not work.
type Check struct {
	Name   string
	Level  string
	Detail string
	Fix    string `json:",omitempty"`
}

const (
	ok   = "ok"
	note = "note"
	warn = "warn"
	fail = "fail"
)

func (self *Doctor) Exec(core *commands.Core) {
	// A short leash on every request: the point of this command is to find out
	// that the server is not answering, and it must reach that conclusion in
	// seconds rather than hanging the way every other command does.
	core.Rest.Timeout = 5 * time.Second

	checks := []Check{}
	add := func(c Check) { checks = append(checks, c) }

	// --- the configuration --------------------------------------------------
	if core.ConfigFile == "" {
		add(Check{"config", note, "running without a configuration file",
			"orgs initconfig > orgs.yaml"})
	} else if _, err := os.Stat(core.ConfigFile); err != nil {
		add(Check{"config", warn, fmt.Sprintf("%s is not there", core.ConfigFile),
			"orgs initconfig > " + core.ConfigFile})
	} else {
		add(Check{"config", ok, core.ConfigFile, ""})
	}

	if core.Rest.Url == "" {
		add(Check{"server url", fail, "no server url configured",
			"set url: in the config, or pass -url"})
	} else {
		add(Check{"server url", ok, core.Rest.Url, ""})
	}

	// --- the server ---------------------------------------------------------
	states, err := commands.SendReceiveGetErr[common.TodoStatesResult](core, "status", nil)
	// An expired token is renewed before the first command that needs it, but
	// this one is NeedsNoServer and skips that - so try the renewal the other
	// commands would have before calling it a login problem.
	renewed := false
	if err != nil && strings.Contains(err.Error(), "Unauthorized") && core.Refresh != nil && core.Refresh() {
		renewed = true
		states, err = commands.SendReceiveGetErr[common.TodoStatesResult](core, "status", nil)
	}
	reachable := err == nil && len(states.Active)+len(states.Done) > 0
	switch {
	case reachable:
		add(Check{"server", ok, fmt.Sprintf("answering, %d todo keywords",
			len(states.Active)+len(states.Done)), ""})
	case err != nil && strings.Contains(err.Error(), "connection refused"):
		add(Check{"server", fail, "nothing is listening on " + core.Rest.Url,
			"orgs serve  (or run this command with -local)"})
	default:
		// A server that answers but not with keywords is usually one that
		// answered 401, which is a different problem from a dead one.
		add(Check{"server", fail, describe(err),
			"if that is a 401: orgs login"})
	}

	// --- authentication -----------------------------------------------------
	switch {
	case core.Rest.Header.Get("Authorization") == "":
		if reachable {
			add(Check{"token", note, "no token, and the server answered anyway - noAuth is on", ""})
		} else {
			add(Check{"token", note, "no token saved", "orgs login"})
		}
	case renewed:
		add(Check{"token", ok, "had run out, and was renewed", ""})
	default:
		add(Check{"token", ok, "a token is being sent", ""})
	}

	if reachable {
		self.serverChecks(core, add)
	}

	// --- this machine -------------------------------------------------------
	for _, tool := range []struct{ bin, why string }{
		{"fzf", "the pickers (orgs code, links, tables, rec, and narrowing a query)"},
		{"bat", "colouring a file in the grep and code panes"},
	} {
		if _, err := exec.LookPath(tool.bin); err != nil {
			add(Check{tool.bin, note, "not on PATH - " + tool.why,
				"install " + tool.bin})
		} else {
			add(Check{tool.bin, ok, "on PATH", ""})
		}
	}
	if len(core.EditorTemplate) == 0 {
		add(Check{"editor", note, "no editorTemplate configured - -open and -edit will do nothing",
			`editorTemplate: ["code", "-g", "{filename}:{linenum}"]`})
	} else {
		add(Check{"editor", ok, strings.Join(core.EditorTemplate, " "), ""})
	}

	self.report(core, checks)
}

// serverChecks are the ones only worth asking a server that is answering.
func (self *Doctor) serverChecks(core *commands.Core, add func(Check)) {
	files := commands.SendReceiveGetOr[common.FileList](core, "files", nil)
	switch {
	case len(files) == 0:
		add(Check{"org files", fail, "the server is watching no files",
			"check orgDirs: in the server's config"})
	default:
		// Whether the files are on *this* machine matters for -check, -o and
		// the editor, and is the thing people are surprised by when the server
		// is somewhere else.
		here := 0
		for _, f := range files {
			if _, err := os.Stat(f); err == nil {
				here++
			}
		}
		detail := fmt.Sprintf("%d files", len(files))
		if here == 0 {
			detail += ", none of them on this machine"
		} else if here < len(files) {
			detail += fmt.Sprintf(", %d of them on this machine", here)
		}
		add(Check{"org files", ok, detail, ""})
	}

	// The event stream, which everything live depends on and which an older
	// server does not have.
	if _, _, code, err := core.Rest.GetRaw("events", map[string]string{"kinds": "none"}); err != nil {
		// A stream that opens and then times out is a *working* stream: the
		// timeout is this command's own, and there was nothing to send.
		if strings.Contains(err.Error(), "Timeout") || strings.Contains(err.Error(), "deadline") {
			add(Check{"events", ok, "streaming", ""})
		} else {
			add(Check{"events", warn, describe(err), "orgs watch will not work"})
		}
	} else if code == 404 {
		add(Check{"events", warn, "this server has no /events",
			"the server is older than the client - rebuild it"})
	} else {
		add(Check{"events", ok, "streaming", ""})
	}

	exp := commands.SendReceiveGetOr[struct {
		Ok    bool
		Names []string
	}](core, "exporters", nil)
	if len(exp.Names) == 0 {
		// The distinction that eats an afternoon: an exporter compiled into the
		// binary but not named in the yaml does not exist as far as a request
		// is concerned, and the refusal reads exactly like one for an exporter
		// that was never written.
		add(Check{"exporters", note, "none configured - orgs export has nothing to call",
			"add them under server.exporters: in the config"})
	} else {
		add(Check{"exporters", ok, strings.Join(exp.Names, ", "), ""})
	}

	// Babel is off unless asked for, and the refusal for a block somebody tried
	// to run says so - but only after they have tried.
	if _, err := commands.SendReceiveGetErr[struct {
		Ok bool
	}](core, "code", nil); err == nil {
		add(Check{"code", ok, "source blocks are readable", ""})
	}

	voice := commands.SendReceiveGetOr[struct {
		Ok    bool
		State string
		Msg   string
		Model string
	}](core, "voice/config", nil)
	switch voice.State {
	case "":
		add(Check{"voice", note, "no transcription configured", ""})
	case "ready", "adopted":
		add(Check{"voice", ok, voice.State + " " + voice.Model, ""})
	case "starting":
		add(Check{"voice", note, "whisper is still loading a model", ""})
	case "off":
		// Off is the default and not a problem; only a state it got stuck in is.
		add(Check{"voice", note, "transcription is off", ""})
	default:
		add(Check{"voice", warn, voice.State + " " + voice.Msg, ""})
	}

	self.statusChecks(core, add)

	if qs := commands.SendReceiveGetOr[[]struct {
		Name string `json:"name"`
	}](core, "ext/queries", nil); len(qs) > 0 {
		add(Check{"saved queries", ok, fmt.Sprintf("%d saved", len(qs)), ""})
	}
}

// statusChecks read GET /server/status: what the server itself found while
// running, which nothing on this side can see - a file it could not read, a
// plugin that panicked, the certificate running out, a worg build older than
// the checkout beside it, a browser holding too many streams.
func (self *Doctor) statusChecks(core *commands.Core, add func(Check)) {
	st, err := commands.SendReceiveGetErr[common.ServerStatus](core, "server/status", nil)
	switch {
	case err != nil && strings.Contains(err.Error(), "404"):
		add(Check{"server status", note, "this server has no /server/status",
			"the server is older than the client - rebuild it"})
		return
	case err != nil || st.Started.IsZero():
		add(Check{"server status", warn, "could not ask the server how it is: " + describe(err), ""})
		return
	}
	add(Check{"server", ok, fmt.Sprintf("%s, up %s", versionWords(st.Version), st.Uptime), ""})
	mine := common.BuildVersion()
	if mine.Commit != "" && st.Version.Commit != "" && mine.Commit != st.Version.Commit {
		add(Check{"versions", note, fmt.Sprintf("this client is %s, the server %s",
			versionWords(mine), versionWords(st.Version)), "restart the server to run this build"})
	}

	// --- files
	for _, d := range st.Files.MissingDirs {
		add(Check{"org dir", fail, d + " cannot be read", "check orgDirs: in the server's config"})
	}
	for _, e := range st.Files.Unreadable {
		// The error from open already names the file: say it once.
		why := strings.TrimPrefix(strings.ReplaceAll(e.Error, e.File+": ", ""), "open ")
		add(Check{"org file", warn, e.File + ": " + why, "it is left out of every query until it loads"})
	}
	for _, e := range st.Files.WatchErrors {
		add(Check{"watcher", warn, e.File + ": " + e.Error,
			"changes there are not seen until the server restarts - fix the permissions"})
	}
	watching := "watching for changes"
	if !st.Files.Watching {
		watching = "not watching for changes"
	}
	add(Check{"files", ok, fmt.Sprintf("%d loaded, %d reads since start, %s",
		st.Files.Loaded, st.Files.ReloadIndex, watching), ""})

	// --- plugins
	for _, p := range st.Plugins {
		name := p.Kind + " " + p.Name
		switch {
		case !p.Started:
			add(Check{name, fail, "did not start: " + p.StartErr, "see the server's log for the stack"})
		case p.LastErr != "" && p.LastErrAt.After(p.LastUpdate):
			add(Check{name, warn, fmt.Sprintf("its last update failed (%s): %s",
				p.LastErrAt.Local().Format("Jan 2 15:04"), p.LastErr), ""})
		case p.Health != "":
			add(Check{name, warn, p.Health, ""})
		case p.Kind == "poller" && p.Updates > 0:
			add(Check{name, ok, fmt.Sprintf("%d updates, last %s", p.Updates,
				p.LastUpdate.Local().Format("Jan 2 15:04")), ""})
		default:
			add(Check{name, ok, "started", ""})
		}
	}

	// --- event streams. A browser has six connections to a host across all
	// its tabs; a page that holds too many of them as streams hangs its own
	// requests with no error anywhere.
	byAddr := map[string]int{}
	for _, s := range st.Streams {
		if s.Addr == "" {
			continue
		}
		host := s.Addr
		if i := strings.LastIndex(host, ":"); i > 0 {
			host = host[:i]
		}
		byAddr[host]++
	}
	clients := 0
	for host, n := range byAddr {
		clients += n
		if n >= 4 {
			add(Check{"streams", warn, fmt.Sprintf("%s holds %d event streams - a browser has 6 connections to a server in all", host, n),
				"close tabs, or a page is opening a stream of its own instead of sharing one"})
		}
	}
	detail := fmt.Sprintf("%d open", clients)
	if st.DroppedEvents > 0 {
		detail += fmt.Sprintf(", %d events dropped by a slow reader", st.DroppedEvents)
	}
	add(Check{"streams", ok, detail, ""})

	// --- the certificate
	if c := st.Cert; c != nil {
		switch {
		case c.Error != "":
			add(Check{"certificate", fail, c.File + ": " + c.Error, "orgs cert"})
		case c.NotAfter.Before(time.Now()):
			add(Check{"certificate", fail, fmt.Sprintf("%s ran out on %s", c.File, c.NotAfter.Local().Format("Jan 2 2006")), "orgs cert"})
		case c.DaysLeft < 14:
			add(Check{"certificate", warn, fmt.Sprintf("%s runs out in %d days", c.File, c.DaysLeft), "orgs cert"})
		default:
			add(Check{"certificate", ok, fmt.Sprintf("%s, %d days left", c.File, c.DaysLeft), ""})
		}
	}

	// --- the embedded worg
	add(worgCheck(st.Worg, os.Getenv("WORG")))

	add(Check{"tokens", ok, fmt.Sprintf("good for %s, renewable for %s after", st.Auth.TokenExpiry, st.Auth.Refresh), ""})
	if st.Auth.NoAuth {
		add(Check{"auth", note, "noAuth is on: anybody who can reach the server is admin", ""})
	}
}

func versionWords(v common.VersionInfo) string {
	if v.Commit != "" {
		return v.Version + " (" + v.Commit + ")"
	}
	return v.Version
}

// worgCheck says whether the worg in the binary is what the checkout beside it
// would build. The checkout is $WORG, else ../worg when this is run from the
// orgs repo (where tools/buildworg.sh looks for it); with neither, only the
// stamp itself is reported.
func worgCheck(b common.WorgBuild, worgDir string) Check {
	if !b.Present {
		return Check{"worg", fail, "the server has no worg built into it", "tools/buildworg.sh, then rebuild orgs"}
	}
	if b.WorgCommit == "" {
		return Check{"worg", note, "built before builds were stamped - its source is unknown", "tools/buildworg.sh"}
	}
	desc := "built from worg " + b.WorgCommit
	if b.WorgDirty {
		desc += " with uncommitted changes"
	}
	if b.BuiltAt != "" {
		desc += " at " + b.BuiltAt
	}
	if worgDir == "" {
		if _, err := os.Stat("worg/embed.go"); err == nil {
			worgDir = "../worg"
		}
	}
	if worgDir != "" {
		if out, err := exec.Command("git", "-C", worgDir, "rev-parse", "--short", "HEAD").Output(); err == nil {
			head := strings.TrimSpace(string(out))
			if !strings.HasPrefix(head, b.WorgCommit) && !strings.HasPrefix(b.WorgCommit, head) {
				return Check{"worg", warn, fmt.Sprintf("%s, but %s is at %s", desc, worgDir, head),
					"tools/buildworg.sh, then rebuild and restart orgs"}
			}
		}
	}
	if b.WorgDirty {
		return Check{"worg", note, desc, ""}
	}
	return Check{"worg", ok, desc, ""}
}

func (self *Doctor) report(core *commands.Core, checks []Check) {
	if commands.QuickfixFormat() {
		commands.RenderDiagnostics(self.diagnostics(core, checks), nil)
		if worst(checks) == fail {
			os.Exit(1)
		}
		return
	}
	if commands.Render(checks, nil) {
		// Exit code says whether anything failed, so a script can branch on it
		// without reading the json back.
		if worst(checks) == fail {
			os.Exit(1)
		}
		return
	}

	width := 0
	for _, c := range checks {
		if len(c.Name) > width {
			width = len(c.Name)
		}
	}
	shown := 0
	for _, c := range checks {
		if c.Level == ok && !self.Verbose {
			continue
		}
		shown++
		fmt.Printf("%s%s%s %s%-*s%s %s\n", commands.C(ink(c.Level)), mark(c.Level),
			commands.C(commands.AnsiReset), commands.C(commands.AnsiBold), width, c.Name,
			commands.C(commands.AnsiReset), c.Detail)
		if c.Fix != "" && c.Level != ok {
			fmt.Printf("   %s%s%s\n", commands.C(commands.AnsiDim), c.Fix,
				commands.C(commands.AnsiReset))
		}
	}
	if shown == 0 {
		fmt.Printf("%s✓%s everything checks out. %s-v to see all %d checks%s\n",
			commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset),
			commands.C(commands.AnsiDim), len(checks), commands.C(commands.AnsiReset))
	} else if !self.Verbose {
		fmt.Printf("\n%s%d checks, %d worth saying. -v for all of them%s\n",
			commands.C(commands.AnsiDim), len(checks), shown, commands.C(commands.AnsiReset))
	}
	if worst(checks) == fail {
		os.Exit(1)
	}
}

// diagnostics is -format quickfix. None of these findings is about a line of
// an org file, so each points at the config file, which is where nearly every
// fix goes; with no config file the name it would have is used, since a
// quickfix line without a file is one an editor drops. Passing checks are left
// out unless -v, the same as the listing.
func (self *Doctor) diagnostics(core *commands.Core, checks []Check) []commands.Diagnostic {
	file := core.ConfigFile
	if file == "" {
		file = "orgs.yaml"
	}
	out := []commands.Diagnostic{}
	for _, c := range checks {
		if c.Level == ok && !self.Verbose {
			continue
		}
		sev := map[string]string{fail: "error", warn: "warning", note: "info", ok: "info"}[c.Level]
		msg := c.Detail
		if c.Fix != "" && c.Level != ok {
			msg += " (" + c.Fix + ")"
		}
		out = append(out, commands.Diagnostic{File: file, Severity: sev, Rule: c.Name, Msg: msg})
	}
	return out
}

func worst(checks []Check) string {
	for _, c := range checks {
		if c.Level == fail {
			return fail
		}
	}
	return ok
}

func mark(level string) string {
	switch level {
	case fail:
		return "✗"
	case warn:
		return "!"
	case note:
		return "·"
	}
	return "✓"
}

func ink(level string) string {
	switch level {
	case fail:
		return commands.AnsiRed
	case warn:
		return commands.AnsiGold
	case note:
		return commands.AnsiCyan
	}
	return commands.AnsiGreen
}

// describe trims a Go http error down to the part a person needs. The wrapped
// form names the method and the whole url twice before saying what happened.
func describe(err error) string {
	if err == nil {
		return "no answer"
	}
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i > 0 && i < len(s)-2 {
		return s[i+2:]
	}
	return s
}

func init() {
	commands.AddCmd("doctor", "check the configuration, the server and this machine",
		func() commands.Cmd { return &Doctor{} })
}
