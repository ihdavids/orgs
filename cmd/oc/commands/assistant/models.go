package assistant

// orgs models - the assistant's local models, from a shell.
//
//	orgs models                         every runtime, its state, its models
//	orgs models suggested               what each runtime suggests pulling
//	orgs models pull qwen3:8b           fetch one (progress until it is done)
//	orgs models pull -runtime llamacpp unsloth/Qwen3-8B-GGUF:Q4_K_M
//	orgs models start|stop [-runtime r]
//	orgs models install -runtime mlx
//	orgs models cancel <model>          stop a pull
//	orgs models delete <model>          asks first; -yes to not
//
// Ollama and the local runtimes (llama.cpp, LM Studio, MLX) are two sets of
// endpoints on the server - `/ollama/*` and `/local/{runtime}/*` - because
// they were built one after the other. Here they are one table, with ollama
// as one more runtime, since which one holds a model is not something a
// person picking one should have to remember.
//
// Model names are not checked here. The server checks each runtime's spelling
// (`llamaRefRe`, `hfRepoRe`, `lmsKeyRe`) and worg checks it again before
// sending; a third copy would only be a third place to drift, and the
// server's refusal says what the name should look like.
//
// A pull and an install run in the background on the server, so this polls
// the status for progress, as worg's manager does. Leaving with ctrl-c leaves
// the pull running; `orgs models cancel` is what stops it.

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Models struct {
	tf      commands.TargetFlags
	Runtime string
}

func (self *Models) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Models) StartPlugin(m *common.PluginManager)       {}
func (self *Models) HelpGroup() string                         { return "Writing" }

func (self *Models) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&self.Runtime, "runtime", "", "ollama, llamacpp, lmstudio or mlx (else the only one, or local's default)")
	commands.AddTargetFlags(fset, &self.tf)
}

type model struct {
	Name       string
	Size       int64
	Params     string
	Quant      string
	Tools      bool
	ContextMax int
	Loaded     bool
}

type pull struct {
	Model    string
	Status   string
	Done     int64
	Total    int64
	Error    string
	Finished bool
}

type suggestion struct {
	Name  string
	About string
	Size  int64
}

// Runtime is one row of `orgs models -json`: ollama's status and each of the
// local runtimes' read into the same shape.
type Runtime struct {
	Id         string
	Label      string
	Enabled    bool
	Supported  bool
	Why        string `json:",omitempty"`
	State      string
	Msg        string `json:",omitempty"`
	Url        string `json:",omitempty"`
	Binary     string `json:",omitempty"`
	CanInstall bool
	Install    map[string]int64 `json:",omitempty"`
	Models     []model
	Pulls      []pull
	Suggested  []suggestion `json:",omitempty"`
	CanManage  bool
}

type server struct {
	Name   string
	Url    string
	Ready  bool
	Msg    string
	Models []string
}

type allStatus struct {
	Runtimes     []Runtime
	Servers      []server `json:",omitempty"`
	LocalDefault string   `json:",omitempty"`
}

func fetchStatus(core *commands.Core) allStatus {
	out := allStatus{}
	ol, oerr := commands.SendReceiveGetErr[struct {
		Ok        bool
		Enabled   bool
		State     string
		Msg       string
		Url       string
		Binary    string
		CanManage bool
		CanInstall bool
		Install   map[string]int64
		Models    []model
		Pulls     []pull
		Suggested []suggestion
	}](core, "ollama/status", nil)
	if oerr == nil && ol.Ok {
		out.Runtimes = append(out.Runtimes, Runtime{Id: "ollama", Label: "Ollama", Enabled: ol.Enabled, Supported: true,
			State: ol.State, Msg: ol.Msg, Url: ol.Url, Binary: ol.Binary, CanInstall: ol.CanInstall, Install: ol.Install,
			Models: ol.Models, Pulls: ol.Pulls, Suggested: ol.Suggested, CanManage: ol.CanManage})
	}
	lo, lerr := commands.SendReceiveGetErr[struct {
		Ok        bool
		Enabled   bool
		Default   string
		CanManage bool
		Runtimes  []Runtime
		Servers   []server
	}](core, "local/status", nil)
	if lerr == nil && lo.Ok {
		for _, r := range lo.Runtimes {
			r.Enabled, r.CanManage = lo.Enabled, lo.CanManage
			out.Runtimes = append(out.Runtimes, r)
		}
		out.Servers = lo.Servers
		out.LocalDefault = lo.Default
	}
	if oerr != nil && lerr != nil {
		commands.Fail("could not ask the server: %v", oerr)
	}
	return out
}

func (self *Models) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("models").Flags)
	sub := "ls"
	if len(words) > 0 {
		sub, words = words[0], words[1:]
	}
	switch sub {
	case "ls", "list", "status":
		self.list(core)
	case "suggested", "suggest":
		self.suggested(core)
	case "pull", "cancel", "delete", "rm":
		if sub == "rm" {
			sub = "delete"
		}
		if len(words) != 1 {
			commands.Fail("orgs models %s <model>", sub)
		}
		self.act(core, sub, words[0])
	case "start", "stop", "install":
		if len(words) > 0 && self.Runtime == "" {
			self.Runtime = words[0]
		}
		self.act(core, sub, "")
	default:
		commands.Fail("no subcommand %s: ls, suggested, pull, cancel, delete, start, stop, install", sub)
	}
}

// runtimeFor is the runtime a command means: the one named, else the one a
// model is already in, else the only one turned on, else local's default.
func (self *Models) runtimeFor(st allStatus, name string) Runtime {
	if self.Runtime != "" {
		for _, r := range st.Runtimes {
			if r.Id == self.Runtime {
				return r
			}
		}
		commands.Fail("no runtime %s: %s", self.Runtime, strings.Join(ids(st), ", "))
	}
	if name != "" {
		for _, r := range st.Runtimes {
			for _, m := range r.Models {
				if m.Name == name {
					return r
				}
			}
			for _, p := range r.Pulls {
				if p.Model == name {
					return r
				}
			}
		}
	}
	on := []Runtime{}
	for _, r := range st.Runtimes {
		if r.Enabled && r.Supported {
			on = append(on, r)
		}
	}
	ollamaOn := false
	for _, r := range on {
		ollamaOn = ollamaOn || r.Id == "ollama"
	}
	if !ollamaOn && st.LocalDefault != "" {
		for _, r := range on {
			if r.Id == st.LocalDefault {
				return r
			}
		}
	}
	if len(on) == 1 {
		return on[0]
	}
	if len(on) == 0 {
		commands.Fail("no local runtime is turned on: set assistant.ollama.enabled or assistant.local.enabled in orgs.yaml")
	}
	names := []string{}
	for _, r := range on {
		names = append(names, r.Id)
	}
	commands.Fail("say which runtime with -runtime: %s", strings.Join(names, ", "))
	return Runtime{}
}

func ids(st allStatus) []string {
	out := []string{}
	for _, r := range st.Runtimes {
		out = append(out, r.Id)
	}
	return out
}

func actionPath(rt, action string) string {
	if rt == "ollama" {
		return "ollama/" + action
	}
	return "local/" + rt + "/" + action
}

func (self *Models) act(core *commands.Core, action, name string) {
	st := fetchStatus(core)
	rt := self.runtimeFor(st, name)
	if action == "delete" {
		if !commands.Confirm(&self.tf, "delete %s from %s", name, rt.Label) {
			commands.Fail("left alone")
		}
	}
	req := struct{ Model string }{name}
	res, err := commands.SendReceivePostErr[struct{ Model string }, common.ResultMsg](core, actionPath(rt.Id, action), &req)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		commands.Fail("%s: %s", rt.Label, res.Msg)
	}
	// A pull and an install carry on in the background; anything else is
	// done when the server answers.
	if (action == "pull" || action == "install") && !commands.Machine() {
		if action == "pull" {
			self.follow(core, rt.Id, name)
		} else {
			self.followInstall(core, rt.Id)
		}
		return
	}
	if commands.RenderOne(res, nil) {
		return
	}
	msg := res.Msg
	if msg == "" {
		msg = fmt.Sprintf("%s: %s %s", rt.Label, action, name)
	}
	fmt.Println(strings.TrimSpace(msg))
}

func find(st allStatus, id string) *Runtime {
	for i := range st.Runtimes {
		if st.Runtimes[i].Id == id {
			return &st.Runtimes[i]
		}
	}
	return nil
}

// follow draws a pull's progress on stderr until the server says it ended.
func (self *Models) follow(core *commands.Core, id, name string) {
	fmt.Fprintf(os.Stderr, "%spulling %s (ctrl-c leaves it pulling on the server)%s\n",
		commands.C(commands.AnsiDim), name, commands.C(commands.AnsiReset))
	seen := false
	for i := 0; ; i++ {
		time.Sleep(time.Second)
		rt := find(fetchStatus(core), id)
		if rt == nil {
			commands.Fail("%s went away", id)
		}
		var p *pull
		for j := range rt.Pulls {
			if rt.Pulls[j].Model == name {
				p = &rt.Pulls[j]
			}
		}
		if p == nil {
			// A pull the server finished and forgot between two looks, or one
			// it never had: the model list says which.
			for _, m := range rt.Models {
				if m.Name == name || strings.HasPrefix(m.Name, name) {
					fmt.Fprintln(os.Stderr)
					fmt.Printf("%s is ready in %s\n", name, rt.Label)
					return
				}
			}
			if seen || i > 15 {
				fmt.Fprintln(os.Stderr)
				commands.Fail("the pull of %s is no longer on the server, and the model is not there either", name)
			}
			continue
		}
		seen = true
		fmt.Fprintf(os.Stderr, "\r\033[K%s", progress(p.Status, p.Done, p.Total))
		if p.Finished {
			fmt.Fprintln(os.Stderr)
			if p.Error != "" {
				commands.Fail("%s: %s", name, p.Error)
			}
			fmt.Printf("%s is ready in %s\n", name, rt.Label)
			return
		}
	}
}

func (self *Models) followInstall(core *commands.Core, id string) {
	for {
		time.Sleep(time.Second)
		rt := find(fetchStatus(core), id)
		if rt == nil {
			commands.Fail("%s went away", id)
		}
		if rt.State != "installing" {
			fmt.Fprintln(os.Stderr)
			if rt.State == "failed" {
				commands.Fail("%s: %s", rt.Label, rt.Msg)
			}
			fmt.Printf("%s: %s\n", rt.Label, firstOf(rt.Msg, rt.State))
			return
		}
		fmt.Fprintf(os.Stderr, "\r\033[K%s", progress(firstOf(rt.Msg, "installing"), rt.Install["Done"], rt.Install["Total"]))
	}
}

func progress(what string, done, total int64) string {
	if total <= 0 {
		return what
	}
	const w = 30
	n := int(float64(done) / float64(total) * w)
	if n > w {
		n = w
	}
	return fmt.Sprintf("%s %s%s%s %3d%% %s / %s", what, commands.C(commands.AnsiGreen), strings.Repeat("█", n)+strings.Repeat("░", w-n),
		commands.C(commands.AnsiReset), int(float64(done)/float64(total)*100), size(done), size(total))
}

func size(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0fM", float64(n)/(1<<20))
	case n > 0:
		return fmt.Sprintf("%.0fK", float64(n)/(1<<10))
	}
	return ""
}

func firstOf(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}

func (self *Models) list(core *commands.Core) {
	st := fetchStatus(core)
	if self.Runtime != "" {
		rt := self.runtimeFor(st, "")
		st.Runtimes = []Runtime{rt}
	}
	if commands.JsonOut {
		commands.RenderOne(st, nil)
		return
	}
	// -format runs per model, which is what a script choosing one wants.
	if commands.FormatOut != "" {
		type row struct {
			Runtime string
			model
		}
		rows := []row{}
		for _, r := range st.Runtimes {
			for _, m := range r.Models {
				rows = append(rows, row{r.Id, m})
			}
		}
		commands.Render(rows, nil)
		return
	}
	for _, r := range st.Runtimes {
		state := r.State
		col := commands.AnsiDim
		switch {
		case !r.Supported:
			state = "unsupported"
		case !r.Enabled:
			state = "off"
		case r.State == "running" || r.State == "adopted":
			col = commands.AnsiGreen
		case r.State == "failed" || r.State == "unreachable":
			col = commands.AnsiRed
		case r.State == "missing":
			state = "not installed"
		}
		fmt.Printf("%s%s%s %s%s%s", commands.C(commands.AnsiBold), r.Label, commands.C(commands.AnsiReset),
			commands.C(col), state, commands.C(commands.AnsiReset))
		if r.Url != "" && r.Enabled {
			fmt.Printf(" %s%s%s", commands.C(commands.AnsiDim), r.Url, commands.C(commands.AnsiReset))
		}
		fmt.Println()
		if !r.Supported && r.Why != "" {
			fmt.Printf("  %s%s%s\n", commands.C(commands.AnsiDim), r.Why, commands.C(commands.AnsiReset))
			continue
		}
		if r.Msg != "" && r.State != "running" && r.State != "adopted" {
			fmt.Printf("  %s%s%s\n", commands.C(commands.AnsiDim), r.Msg, commands.C(commands.AnsiReset))
		}
		sort.Slice(r.Models, func(i, j int) bool { return r.Models[i].Name < r.Models[j].Name })
		for _, m := range r.Models {
			flags := []string{}
			if m.Loaded {
				flags = append(flags, commands.C(commands.AnsiGreen)+"loaded"+commands.C(commands.AnsiReset))
			}
			if m.Tools {
				flags = append(flags, "tools")
			}
			if m.ContextMax > 0 {
				flags = append(flags, fmt.Sprintf("%dk ctx", m.ContextMax/1024))
			}
			fmt.Printf("  %-40s %6s %s%-6s %-8s%s %s\n", commands.Ellipsis(m.Name, 40), size(m.Size), commands.C(commands.AnsiDim),
				m.Params, m.Quant, commands.C(commands.AnsiReset), strings.Join(flags, " "))
		}
		for _, p := range r.Pulls {
			line := progress(p.Status, p.Done, p.Total)
			if p.Error != "" {
				line = commands.C(commands.AnsiRed) + p.Error + commands.C(commands.AnsiReset)
			} else if p.Finished {
				line = "done"
			}
			fmt.Printf("  %s↓ %s%s %s\n", commands.C(commands.AnsiCyan), p.Model, commands.C(commands.AnsiReset), line)
		}
		if r.Enabled && r.Supported && r.State != "missing" && len(r.Models) == 0 && len(r.Pulls) == 0 {
			fmt.Printf("  %sno models: orgs models suggested%s\n", commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
		}
	}
	for _, s := range st.Servers {
		mark := commands.C(commands.AnsiGreen) + "✓" + commands.C(commands.AnsiReset)
		if !s.Ready {
			mark = commands.C(commands.AnsiRed) + "✗" + commands.C(commands.AnsiReset)
		}
		fmt.Printf("%s%s%s %s %s%s%s\n", commands.C(commands.AnsiBold), s.Name, commands.C(commands.AnsiReset), mark,
			commands.C(commands.AnsiDim), s.Url, commands.C(commands.AnsiReset))
		if !s.Ready && s.Msg != "" {
			fmt.Printf("  %s\n", s.Msg)
		}
		for _, m := range s.Models {
			fmt.Printf("  %s\n", m)
		}
	}
}

func (self *Models) suggested(core *commands.Core) {
	st := fetchStatus(core)
	type row struct {
		Runtime string
		suggestion
	}
	rows := []row{}
	for _, r := range st.Runtimes {
		if self.Runtime != "" && r.Id != self.Runtime {
			continue
		}
		for _, s := range r.Suggested {
			rows = append(rows, row{r.Id, s})
		}
	}
	commands.Render(rows, func() {
		last := ""
		for _, r := range rows {
			if r.Runtime != last {
				fmt.Printf("%s%s%s\n", commands.C(commands.AnsiBold), r.Runtime, commands.C(commands.AnsiReset))
				last = r.Runtime
			}
			fmt.Printf("  %-44s %6s %s%s%s\n", r.Name, size(r.Size), commands.C(commands.AnsiDim), r.About, commands.C(commands.AnsiReset))
		}
	})
}
