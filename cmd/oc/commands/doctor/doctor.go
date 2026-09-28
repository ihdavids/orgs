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

	self.report(checks)
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

	if qs := commands.SendReceiveGetOr[[]struct {
		Name string `json:"name"`
	}](core, "ext/queries", nil); len(qs) > 0 {
		add(Check{"saved queries", ok, fmt.Sprintf("%d saved", len(qs)), ""})
	}
}

func (self *Doctor) report(checks []Check) {
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
