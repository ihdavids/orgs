package orgpkgs

// org-recur from the command line (features.recur).
//
//	orgs recur                          every heading with a |rule|, its rule in words and next date
//	orgs recur -done                    done ones too
//	orgs recur set 'Water' '|Mon,Fri|'  give a heading a rule (a query or hash, then the rule)
//	orgs recur clear 'Water'            take it off
//	orgs recur explain '|1,15|' -n 6    what a rule means and its next dates (no server needed)

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Recur struct {
	base
	tf  commands.TargetFlags
	All bool
	N   int
}

func (self *Recur) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	commands.AddTargetFlags(fset, &self.tf)
	fset.BoolVar(&self.All, "done", false, "list headings in a done keyword too")
	fset.IntVar(&self.N, "n", 0, "how many next dates to show (3 in the list, 5 for explain)")
}

type recurItem struct {
	Hash, Headline, Title, Filename string
	Line                            int
	Status, Rule, Words, Err        string
	Stamp, Current, Next            string
	Upcoming                        []string
}

func (self *Recur) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 {
		verb = strings.ToLower(words[0])
	}
	switch verb {
	case "explain":
		self.explain(words[1:])
	case "set":
		if len(words) < 3 {
			commands.Fail("orgs recur set HEADING '|Mon,Fri|'  (a query or hash, then the rule)")
		}
		self.set(core, words[1:len(words)-1], words[len(words)-1])
	case "clear", "rm", "off":
		self.set(core, words[1:], "")
	case "", "ls", "list":
		self.list(core)
	default:
		commands.Fail("orgs recur [ls | set HEADING RULE | clear HEADING | explain RULE]")
	}
}

func (self *Recur) list(core *commands.Core) {
	n := self.N
	if n <= 0 {
		n = 3
	}
	params := map[string]string{"n": fmt.Sprint(n)}
	if self.All {
		params["all"] = "1"
	}
	res, err := commands.SendReceiveGetErr[struct {
		offAnswer
		Weekdays string
		Items    []recurItem
	}](core, "recur", params)
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(res.offAnswer)
	commands.Render(res.Items, func() {
		for _, it := range res.Items {
			rule := commands.C(commands.AnsiGold) + "|" + it.Rule + "|" + commands.C(commands.AnsiReset)
			where := dim(fmt.Sprintf("%s:%d", commands.BaseName(it.Filename), it.Line+1))
			if it.Err != "" {
				fmt.Printf("%s %s  %s %s\n", where, it.Headline, rule, commands.C(commands.AnsiRed)+it.Err+commands.C(commands.AnsiReset))
				continue
			}
			kw := ""
			if it.Status != "" {
				kw = it.Status + " "
			}
			now := ""
			if it.Current != "" {
				now = dim(" now " + it.Stamp + " " + it.Current)
			}
			fmt.Printf("%s %s%s  %s %s  next %s%s\n", where, kw, it.Headline, rule, dim(it.Words), strings.Join(it.Upcoming, ", "), now)
		}
		if len(res.Items) == 0 {
			fmt.Println("no heading has a |rule| in its title")
		}
	})
}

func (self *Recur) set(core *commands.Core, target []string, rule string) {
	if rule != "" {
		if _, err := common.ParseRecur(rule, ""); err != nil {
			commands.Fail("%v", err)
		}
	}
	todos := commands.Resolve(core, &self.tf, target, commands.TargetOpts{Prompt: "Recur> "})
	lines := []string{}
	for _, t := range todos {
		var reply struct {
			offAnswer
		}
		commands.SendReceivePost(core, "recur/set", &struct{ Hash, Rule string }{t.Hash, rule}, &reply)
		if commands.DryRun {
			continue
		}
		failIfOff(reply.offAnswer)
		lines = append(lines, commands.Describe(t)+" "+dim(reply.Msg))
	}
	what := "set"
	if rule == "" {
		what = "cleared"
	}
	commands.Ok(len(lines), what, lines)
}

type recurExplain struct {
	Ok    bool
	Msg   string `json:",omitempty"`
	Rule  string
	Words string
	Next  []string
}

// explain works without a server: the rule is read by internal/common, the
// same code the server reads it with. wkdy is mon-fri here; a server with
// recur.weekdays set may differ.
func (self *Recur) explain(words []string) {
	if len(words) == 0 {
		commands.Fail("orgs recur explain '|Mon,Thu|'")
	}
	n := self.N
	if n <= 0 {
		n = 5
	}
	r, err := common.ParseRecur(strings.Join(words, " "), "")
	if err != nil {
		commands.Fail("%v", err)
	}
	out := recurExplain{Ok: true, Rule: r.Text, Words: r.Words(), Next: []string{}}
	for _, d := range r.NextN(time.Now(), n) {
		out.Next = append(out.Next, d.String())
	}
	commands.RenderOne(out, func() {
		fmt.Printf("%s  %s\n", commands.C(commands.AnsiGold)+"|"+r.Text+"|"+commands.C(commands.AnsiReset), out.Words)
		for _, d := range out.Next {
			fmt.Println("  " + d)
		}
	})
}

func init() {
	commands.AddCmd("recur", "repeats written in the title, |Mon,Thu| or |+2| (org-recur)",
		func() commands.Cmd { return &Recur{} })
}
