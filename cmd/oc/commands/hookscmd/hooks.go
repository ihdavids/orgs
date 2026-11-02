package hookscmd

// `orgs hooks`: the server's hooks, what they did, and running one by hand.
//
//	orgs hooks                          the rules, and the last runs
//	orgs hooks runs                     every run kept (the last hundred)
//	orgs hooks test done-log to=DONE headline="Ship it"   run one now on a made-up event
//
// Rules live in the server's yaml (`hooks: enable: true` and `rules:`); see
// internal/app/orgs/hooks.go for what each event carries and how a hook gets it.

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Hooks struct {
	fset *flag.FlagSet
}

func (self *Hooks) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Hooks) StartPlugin(manager *common.PluginManager)       {}
func (self *Hooks) SetupParameters(fset *flag.FlagSet)              { self.fset = fset }
func (self *Hooks) HelpGroup() string                                { return "Setup" }

type run struct {
	Hook   string
	Kind   string
	At     time.Time
	Ok     bool
	Msg    string
	Output string
	Millis int64
}

type state struct {
	Ok     bool
	Enable bool
	Rules  []struct {
		Name  string
		On    []string
		Match map[string]string
		Does  string
	}
	Runs []run
}

func (self *Hooks) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 {
		verb = words[0]
	}
	switch verb {
	case "", "ls", "runs":
		s, err := commands.SendReceiveGetErr[state](core, "hooks", nil)
		if err != nil {
			commands.Fail("%v", err)
		}
		commands.RenderOne(s, func() {
			if verb != "runs" {
				if !s.Enable {
					fmt.Println("Hooks are off: hooks: enable: true in the server's yaml")
				}
				for _, r := range s.Rules {
					m := []string{}
					for k, v := range r.Match {
						m = append(m, k+"="+v)
					}
					fmt.Printf("%s%-16s%s on %s %s\n    %s\n", commands.C(commands.AnsiBold), r.Name, commands.C(commands.AnsiReset),
						strings.Join(r.On, ","), strings.Join(m, " "), r.Does)
				}
				if len(s.Rules) == 0 {
					fmt.Println("No rules.")
				}
			}
			runs := s.Runs
			if verb != "runs" && len(runs) > 10 {
				runs = runs[len(runs)-10:]
			}
			if len(runs) > 0 {
				fmt.Println("\nRuns:")
			}
			for _, r := range runs {
				printRun(r)
			}
		})
	case "test":
		if len(words) < 2 {
			commands.Fail("orgs hooks test NAME [field=value ...]  (kind=statechange headline=... from=TODO to=DONE ...)")
		}
		ev := map[string]interface{}{"kind": "test"}
		for _, w := range words[2:] {
			k, v, ok := strings.Cut(w, "=")
			if !ok {
				commands.Fail("%s: say field=value", w)
			}
			if k == "tags" {
				ev[k] = strings.FieldsFunc(v, func(r rune) bool { return r == ':' || r == ',' })
				continue
			}
			ev[strings.ToLower(k)] = v
		}
		req := map[string]interface{}{"Hook": words[1], "Event": ev}
		res, err := commands.SendReceivePostErr[map[string]interface{}, run](core, "hooks/test", &req)
		if err == commands.ErrDryRun {
			return
		}
		if err != nil {
			commands.Fail("%v", err)
		}
		commands.RenderOne(res, func() { printRun(res) })
		if !res.Ok {
			commands.Fail("the hook failed")
		}
	default:
		commands.Fail("orgs hooks [ls | runs | test NAME field=value ...]")
	}
}

func printRun(r run) {
	mark := commands.C(commands.AnsiGreen) + "✓" + commands.C(commands.AnsiReset)
	if !r.Ok {
		mark = commands.C(commands.AnsiRed) + "✗" + commands.C(commands.AnsiReset)
	}
	when := ""
	if !r.At.IsZero() {
		when = r.At.Local().Format("01-02 15:04:05") + " "
	}
	fmt.Printf("%s %s%s%s %s on %s (%dms) %s\n", mark, commands.C(commands.AnsiDim), when, commands.C(commands.AnsiReset),
		r.Hook, r.Kind, r.Millis, r.Msg)
	if r.Output != "" {
		for _, l := range strings.Split(r.Output, "\n") {
			fmt.Println("    " + l)
		}
	}
}

func init() {
	commands.AddCmd("hooks", "the server's hooks (commands and webhooks run on events), their runs, and testing one",
		func() commands.Cmd { return &Hooks{} })
}
