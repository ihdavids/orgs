//lint:file-ignore ST1006 allow the use of self
package capture

// orgs cap - a capture, through the template rather than past it.
//
// The template string has always been the client's to put up (the server hands
// it over and files what comes back), and this command used to ignore it
// completely: two unlabelled text areas, a headline and a body, whatever the
// template said. So a template with a property drawer in it worked in worg and
// did nothing at a prompt, which is the wrong way round for the half of this
// tool that lives in a terminal.
//
// Now the template *is* the form. See form.go for what that means on screen.
// The grammar and the arithmetic are `internal/common/captemplate.go`, shared
// with the server end that expands it, so there is one understanding of what a
// placeholder is rather than one per client.

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/cmd/oc/commands/orghl"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/koki-develop/go-fzf"
)

// NeedsHeading says whether this kind of capture is a heading of its own. An
// entry is; an item, a checkitem, a table line and a plain capture all go into
// somebody else's.
func NeedsHeading(typeName string) bool {
	switch typeName {
	case "", "entry":
		return true
	}
	return false
}

type setFlag []string

func (self *setFlag) String() string { return strings.Join(*self, ",") }
func (self *setFlag) Set(v string) error {
	*self = append(*self, v)
	return nil
}

type Capture struct {
	Template string
	Head     string
	Cont     string
	Tags     string
	Set      setFlag
	NoForm   bool
	fset     *flag.FlagSet
}

func (self *Capture) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Capture) StartPlugin(manager *common.PluginManager) {
}

func (self *Capture) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&(self.Template), "temp", "", "template name (or name it as the first word)")
	fset.StringVar(&(self.Head), "head", "", "the headline")
	fset.StringVar(&(self.Cont), "cont", "", "the body")
	fset.StringVar(&(self.Tags), "tags", "", "tags for the new heading, spaces or commas between them")
	fset.Var(&(self.Set), "set", "answer one of the template's fields: -set source=email (repeatable)")
	fset.BoolVar(&(self.NoForm), "no-form", false, "do not open the form: take the defaults and whatever the flags said")
}

func (self *Capture) Exec(core *commands.Core) {
	// Taken once: FreeArgs consumes the arguments as it parses, so asking the
	// flag set afterwards gets nothing.
	words := commands.FreeArgs(self.fset)
	if self.Template == "" && len(words) > 0 {
		self.Template = words[0]
	}

	var temps []common.CaptureTemplate
	commands.SendReceiveGet(core, "capture/templates", map[string]string{}, &temps)
	if len(temps) == 0 {
		commands.Fail("no capture templates - they go under captureTemplates: in your orgs.yaml, or are added per user through /ext/capture/template")
	}

	tpl, ok := pickTemplate(temps, self.Template)
	if !ok {
		return
	}

	wantsHead := NeedsHeading(tpl.Type)
	fields := buildFields(tpl, wantsHead)

	// The flags answer what they answer before anybody is asked, so a run with
	// every field given on the command line never opens a form, and a run with
	// some of them opens one with those already filled in.
	if self.Head != "" {
		setField(fields, "HEADLINE", self.Head)
	}
	if self.Cont != "" {
		setField(fields, "CONTENT", self.Cont)
	}
	if self.Tags != "" {
		setField(fields, "TAGS", self.Tags)
	}
	for _, kv := range self.Set {
		k, v, found := strings.Cut(kv, "=")
		if !found {
			commands.Fail("-set wants name=value, not %q", kv)
		}
		if !setField(fields, strings.TrimSpace(k), v) {
			commands.Fail("the %s template has no field called %q - it asks for: %s", tpl.Name, k, fieldNames(fields))
		}
	}

	if commands.Interactive() && !self.NoForm {
		level := 1
		if tpl.CapTarget.Lvl > 0 {
			level = tpl.CapTarget.Lvl + 1
		}
		if !runForm(tpl, fields, level, keywordState(core)) {
			fmt.Fprintf(os.Stderr, "nothing captured\n")
			return
		}
	}

	head := fieldValue(fields, "HEADLINE")
	if wantsHead && strings.TrimSpace(head) == "" {
		commands.Fail("%s captures a heading, so it needs a headline: pass -head", tpl.Name)
	}

	values := map[string]string{}
	for _, f := range fields {
		if f.Kind == fieldTemplate {
			values[f.Key] = f.String()
		}
	}
	var query common.Capture
	query.Template = tpl.Name
	query.NewNode.Headline = strings.TrimSpace(head)
	query.NewNode.Content = common.FillCapTemplate(tpl.Template, values)
	query.NewNode.Tags = tagWords(fieldValue(fields, "TAGS"))

	reply, err := commands.SendReceivePostErr[common.Capture, common.ResultMsg](core, "capture", &query)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	if !reply.Ok {
		commands.Fail("%s", reply.Msg)
	}
	commands.RenderOne(reply, func() {
		fmt.Printf("%sCaptured%s into %s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset), targetText(tpl))
	})
}

// pickTemplate finds the one that was named, or asks. One template is not a
// choice, so it is taken; with no name and nobody to ask, the refusal says what
// there was to choose from.
func pickTemplate(temps []common.CaptureTemplate, name string) (common.CaptureTemplate, bool) {
	if name != "" {
		for _, t := range temps {
			if t.Name == name {
				return t, true
			}
		}
		for _, t := range temps {
			if strings.EqualFold(t.Name, name) {
				return t, true
			}
		}
		commands.Fail("no capture template called %q - this server has: %s", name, templateNames(temps))
	}
	if len(temps) == 1 {
		return temps[0], true
	}
	if !commands.Interactive() {
		commands.Fail("which template? name it, or pass -temp. This server has: %s", templateNames(temps))
	}
	f, err := fzf.New(
		fzf.WithNoLimit(false),
		fzf.WithCountViewEnabled(true),
		fzf.WithCountView(func(meta fzf.CountViewMeta) string {
			return fmt.Sprintf("templates: %d", meta.ItemsCount)
		}),
	)
	if err != nil {
		commands.Fail("%v", err)
	}
	idx, err := f.Find(temps, func(i int) string {
		return temps[i].Name + "  →  " + targetText(temps[i])
	})
	if err != nil || len(idx) == 0 {
		fmt.Fprintf(os.Stderr, "nothing captured\n")
		return common.CaptureTemplate{}, false
	}
	return temps[idx[0]], true
}

// keywordState asks the server which keywords it accepts, so a headline typed
// as "TODO buy milk" is coloured as a task rather than as a word. A server that
// will not say is not a reason to refuse to capture, so the failure is silent
// and the heading is drawn without a keyword.
func keywordState(core *commands.Core) orghl.State {
	st := commands.SendReceiveGetOr[common.TodoStatesResult](core, "status", nil)
	return orghl.State{Active: st.Active, Done: st.Done}
}

func setField(fields []*field, key, value string) bool {
	for _, f := range fields {
		if strings.EqualFold(f.Key, key) {
			f.set(value)
			return true
		}
	}
	return false
}

func fieldValue(fields []*field, key string) string {
	for _, f := range fields {
		if strings.EqualFold(f.Key, key) {
			return f.String()
		}
	}
	return ""
}

func fieldNames(fields []*field) string {
	names := []string{}
	for _, f := range fields {
		names = append(names, f.Key)
	}
	return strings.Join(names, ", ")
}

func templateNames(temps []common.CaptureTemplate) string {
	names := []string{}
	for _, t := range temps {
		names = append(names, t.Name)
	}
	return strings.Join(names, ", ")
}

// tagWords takes tags however somebody wrote them - spaces, commas, or org's
// own colons - and answers with the words.
func tagWords(s string) []string {
	words := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t' || r == ':'
	})
	if len(words) == 0 {
		return nil
	}
	return words
}

// targetText is where a capture made with this template lands, said the way
// somebody would say it rather than as a target struct.
func targetText(t common.CaptureTemplate) string {
	tg := t.CapTarget
	file := tg.Filename
	if i := strings.LastIndexAny(file, "/\\"); i >= 0 {
		file = file[i+1:]
	}
	id := strings.ReplaceAll(tg.Id, "::", " › ")
	switch strings.ToLower(tg.Type) {
	case "file":
		if file == "" {
			return "a file"
		}
		return file
	case "file+datetree", "file+olp+datetree":
		if id == "" {
			return file + " › today"
		}
		return file + " › " + id + " › today"
	case "clock":
		return "wherever the clock is running"
	}
	if file == "" {
		return id
	}
	if id == "" {
		return file
	}
	return file + " › " + id
}

type CaptureTemplate struct {
}

func (self *CaptureTemplate) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *CaptureTemplate) StartPlugin(manager *common.PluginManager) {
}

func (self *CaptureTemplate) SetupParameters(*flag.FlagSet) {
}

// orgs listcap: what there is to capture with, and what each one is going to
// write. The template is drawn as the org it is, because that is the question
// somebody runs this to answer - not "what are these called" but "which of
// these is the one with the drawer in it".
func (self *CaptureTemplate) Exec(core *commands.Core) {
	var temps []common.CaptureTemplate
	commands.SendReceiveGet(core, "capture/templates", map[string]string{}, &temps)
	if commands.Render(temps, nil) {
		return
	}
	if len(temps) == 0 {
		fmt.Fprintf(os.Stderr, "no capture templates - they go under captureTemplates: in your orgs.yaml\n")
		return
	}
	for i, t := range temps {
		if i > 0 {
			fmt.Printf("\n")
		}
		fmt.Printf("%s%s%s  %s→%s %s%s%s\n",
			commands.C(commands.AnsiBold), t.Name, commands.C(commands.AnsiReset),
			commands.C(commands.AnsiDim), commands.C(commands.AnsiReset),
			commands.C(commands.AnsiCyan), targetText(t), commands.C(commands.AnsiReset))
		if t.Type != "" && t.Type != "entry" {
			fmt.Printf("  %s%s%s\n", commands.C(commands.AnsiDim), t.Type, commands.C(commands.AnsiReset))
		}
		if strings.TrimSpace(t.Template) == "" {
			fmt.Printf("  %sa headline and a body%s\n", commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
			continue
		}
		st := orghl.State{}
		for _, line := range strings.Split(t.Template, "\n") {
			fmt.Printf("  %s\n", orghl.ANSI(orghl.Line(line, &st), commands.Colour()))
		}
	}
}

// init function is called at boot
func init() {
	commands.AddCmd("cap", "quick capture idea",
		func() commands.Cmd {
			return &Capture{}
		})
	commands.AddCmd("listcap", "list capture templates",
		func() commands.Cmd {
			return &CaptureTemplate{}
		})
}
