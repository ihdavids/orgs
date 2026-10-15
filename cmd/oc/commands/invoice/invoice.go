package invoice

// orgs invoice / orgs timesheet - clocked time by client, as an invoice or a
// timesheet, in the terminal or as csv, html or pdf.
//
//	orgs invoice                                last month, one invoice per client
//	orgs invoice -client acme -f pdf            Acme's, as ACME-202609.pdf
//	orgs invoice -f pdf -theme letterpress -accent '#335c67' -paper a4
//	orgs invoice -by tag -tags acme,globex -round 6 -f csv > lines.csv
//	orgs invoice -f pdf -split -out invoices/   a pdf a client
//	orgs timesheet -block thisweek -f pdf       this week's sittings, signed off
//	orgs timesheet -block 2026-10 -f csv        a row a sitting, for a spreadsheet
//	orgs invoice -list-themes
//
// Everything is worked out by the server (GET /timesheet), which knows the
// clocks and the invoice: settings; this asks and writes the answer here. A pdf
// is the server's html printed by a headless Chrome on this machine, so every
// theme looks the same on paper as on screen, and -css adds rules of your own.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"golang.org/x/term"
)

type Invoice struct {
	kind string

	Block      string
	From       string
	To         string
	By         string
	Tags       string
	Client     string
	Rate       float64
	Round      int
	RoundMode  string
	Lines      string
	Date       string
	Number     string
	Unbillable bool

	Format     string
	Out        string
	Split      bool
	Theme      string
	Accent     string
	Paper      string
	Logo       string
	Css        string
	Title      string
	ListThemes bool
	Open       bool
}

func (self *Invoice) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Invoice) StartPlugin(m *common.PluginManager)       {}

func (self *Invoice) SetupParameters(fset *flag.FlagSet) {
	def := "lastmonth"
	if self.kind == "timesheet" {
		def = "thisweek"
	}
	fset.StringVar(&self.Block, "block", def, "the range, as a clock table writes it: lastmonth, thisweek, 2026-10, 2026-W41, 2026-Q3...")
	fset.StringVar(&self.From, "from", "", "first day (a date, mon, -2w...), narrowing or replacing -block")
	fset.StringVar(&self.To, "to", "", "the day after the last, as a clock table's :tend")
	fset.StringVar(&self.By, "by", "client", "who time belongs to: client (CLIENT property, else a client's tag), tag, project or file")
	fset.StringVar(&self.Tags, "tags", "", "with -by tag, the tags that are clients (comma separated)")
	fset.StringVar(&self.Client, "client", "", "only this client")
	fset.Float64Var(&self.Rate, "rate", 0, "an hour, where neither the heading's RATE nor the client's settings say")
	fset.IntVar(&self.Round, "round", 0, "bill each line in N-minute increments (default: invoice.round; -1 for none)")
	fset.StringVar(&self.RoundMode, "round-mode", "", "up (default), nearest or down")
	fset.StringVar(&self.Lines, "lines", "task", "an invoice line a task, or a task a day (day)")
	fset.StringVar(&self.Date, "date", "", "the invoice date (default today)")
	fset.StringVar(&self.Number, "number", "", "the invoice number, or a pattern: {CLIENT} {client} {yyyy} {yy} {mm} {dd}")
	fset.BoolVar(&self.Unbillable, "unbillable", false, "list unbillable time on the invoice, at no charge")

	fset.StringVar(&self.Format, "f", "", "text (default), csv, html or pdf")
	fset.StringVar(&self.Out, "out", "", "the file to write (a folder with -split)")
	fset.BoolVar(&self.Split, "split", false, "a file a client, named by invoice number")
	fset.StringVar(&self.Theme, "theme", "", "html/pdf look: classic, modern, minimal, ledger, studio, letterpress (-list-themes)")
	fset.StringVar(&self.Accent, "accent", "", "the theme's accent colour (#7a2e3a, teal...)")
	fset.StringVar(&self.Paper, "paper", "", "letter, a4 or legal")
	fset.StringVar(&self.Logo, "logo", "", "a logo: a file here, or an http(s) url (a file on the server goes in invoice.logo)")
	fset.StringVar(&self.Css, "css", "", "a stylesheet of your own, applied after the theme")
	fset.StringVar(&self.Title, "title", "", "the page's title")
	fset.BoolVar(&self.ListThemes, "list-themes", false, "list the themes the server has")
	fset.BoolVar(&self.Open, "open", false, "open what was written")
}

func (self *Invoice) params() map[string]string {
	p := map[string]string{"kind": self.kind}
	set := func(k, v string) {
		if v = strings.TrimSpace(v); v != "" {
			p[k] = v
		}
	}
	set("block", self.Block)
	if self.From != "" || self.To != "" {
		set("tstart", self.when(self.From))
		set("tend", self.when(self.To))
	}
	set("by", self.By)
	set("tags", self.Tags)
	set("client", self.Client)
	if self.Rate > 0 {
		p["rate"] = strconv.FormatFloat(self.Rate, 'f', -1, 64)
	}
	if self.Round != 0 {
		p["round"] = strconv.Itoa(self.Round)
	}
	set("roundMode", self.RoundMode)
	set("lines", self.Lines)
	set("date", self.when(self.Date))
	set("number", self.Number)
	if self.Unbillable {
		p["unbillable"] = "t"
	}
	set("theme", self.Theme)
	set("accent", self.Accent)
	set("paper", self.Paper)
	set("title", self.Title)
	return p
}

// when reads a date the way orgs sched does, and hands the server a plain one.
func (self *Invoice) when(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	d, _, err := commands.ParseDate(s, time.Now())
	if err != nil {
		commands.Fail("%s: cannot read %q as a date: %v", self.kind, s, err)
	}
	return d.Day.Format("2006-01-02")
}

func (self *Invoice) Exec(core *commands.Core) {
	if self.ListThemes {
		themes := commands.SendReceiveGetOr[[]string](core, "timesheet", map[string]string{"themes": "t"})
		if commands.Render(themes, func() {
			for _, t := range themes {
				fmt.Println(t)
			}
		}) {
			return
		}
		return
	}

	ps := self.params()
	if self.Logo != "" {
		logo, err := localLogo(self.Logo)
		if err != nil {
			commands.Fail("%s: logo: %v", self.kind, err)
		}
		ps["logo"] = logo
	}

	ts, err := self.fetch(core, ps)
	if err != nil {
		commands.Fail("%s: %v", self.kind, err)
	}
	for _, w := range ts.Warnings {
		fmt.Fprintf(os.Stderr, "%s%s%s\n", commands.C(commands.AnsiGold), w, commands.C(commands.AnsiReset))
	}
	if commands.RenderOne(ts, nil) {
		return
	}

	format := strings.ToLower(self.Format)
	if format == "" || format == "text" {
		self.text(ts)
		return
	}
	switch format {
	case "csv", "html", "pdf":
	default:
		commands.Fail("%s: -f %s: text, csv, html or pdf", self.kind, self.Format)
	}

	if self.Split {
		self.split(core, ts, ps, format)
		return
	}
	out := self.Out
	if out == "" {
		// csv and html go down a pipe as well as anything; a pdf, or a page
		// to a terminal, is a file named after what it is.
		if format == "csv" || (format == "html" && !term.IsTerminal(int(os.Stdout.Fd()))) {
			os.Stdout.Write(self.render(core, ps, format, ""))
			return
		}
		out = self.defaultName(ts, format)
	}
	self.write(out, self.render(core, ps, format, out))
}

func (self *Invoice) fetch(core *commands.Core, ps map[string]string) (*common.Timesheet, error) {
	q := map[string]string{}
	for k, v := range ps {
		q[k] = v
	}
	q["format"] = "json"
	b, typ, status, err := core.Rest.GetRaw("timesheet", q)
	if err != nil {
		return nil, err
	}
	if status != 200 || !strings.Contains(typ, "json") {
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(b)))
	}
	ts := &common.Timesheet{}
	if err := json.Unmarshal(b, ts); err != nil {
		return nil, err
	}
	return ts, nil
}

// render asks for the page or the csv, and makes the pdf here.
func (self *Invoice) render(core *commands.Core, ps map[string]string, format, out string) []byte {
	q := map[string]string{}
	for k, v := range ps {
		q[k] = v
	}
	q["format"] = format
	if format == "pdf" {
		q["format"] = "html"
	}
	b, _, status, err := core.Rest.GetRaw("timesheet", q)
	if err != nil {
		commands.Fail("%s: %v", self.kind, err)
	}
	if status != 200 {
		commands.Fail("%s: %s", self.kind, strings.TrimSpace(string(b)))
	}
	if format == "csv" {
		return b
	}
	if self.Css != "" {
		css, err := os.ReadFile(self.Css)
		if err != nil {
			commands.Fail("%s: -css: %v", self.kind, err)
		}
		// Last in the head, so its rules win over the theme's.
		safe := strings.ReplaceAll(string(css), "</style", "<\\/style")
		b = bytes.Replace(b, []byte("</head>"), []byte("<style>\n"+safe+"\n</style>\n</head>"), 1)
	}
	if format == "html" {
		return b
	}
	if commands.DryRun {
		return nil
	}
	pdf, err := commands.PrintPDF(b)
	if err != nil {
		commands.Fail("%s: %v\n  -f html writes the page; print it from a browser instead", self.kind, err)
	}
	return pdf
}

func (self *Invoice) write(out string, b []byte) {
	if commands.Wrote("write "+out, nil) {
		return
	}
	if dir := filepath.Dir(out); dir != "" {
		os.MkdirAll(dir, 0o755)
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		commands.Fail("%s: %v", self.kind, err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", out)
	if self.Open {
		openFile(out)
	}
}

func (self *Invoice) split(core *commands.Core, ts *common.Timesheet, ps map[string]string, format string) {
	dir := self.Out
	if dir == "" {
		dir = "."
	}
	n := 0
	for _, c := range ts.Clients {
		if self.kind == "invoice" && (c.Key == "" || len(c.Lines) == 0) {
			continue
		}
		if c.Key == "" {
			// No client cannot be asked for by name; it is in the
			// unsplit timesheet.
			continue
		}
		q := map[string]string{}
		for k, v := range ps {
			q[k] = v
		}
		q["client"] = c.Key
		name := c.Number
		if self.kind == "timesheet" {
			name = "timesheet-" + slug(c.Key) + "-" + ts.From
		}
		out := filepath.Join(dir, name+"."+format)
		self.write(out, self.render(core, q, format, out))
		n++
	}
	if n == 0 {
		commands.Fail("%s: nothing to write for %s", self.kind, ts.Range)
	}
}

func (self *Invoice) defaultName(ts *common.Timesheet, format string) string {
	if self.kind == "invoice" {
		billed := []common.TimesheetClient{}
		for _, c := range ts.Clients {
			if c.Key != "" && len(c.Lines) > 0 {
				billed = append(billed, c)
			}
		}
		if len(billed) == 1 {
			return billed[0].Number + "." + format
		}
		return "invoices-" + ts.From + "." + format
	}
	if self.Client != "" {
		return "timesheet-" + slug(self.Client) + "-" + ts.From + "." + format
	}
	return "timesheet-" + ts.From + "." + format
}

func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// localLogo turns a file here into a data: url, since the server cannot read
// this machine's files (and is not allowed to be told a path of its own).
func localLogo(spec string) (string, error) {
	if strings.HasPrefix(spec, "http://") || strings.HasPrefix(spec, "https://") || strings.HasPrefix(spec, "data:") {
		return spec, nil
	}
	b, err := os.ReadFile(spec)
	if err != nil {
		return "", err
	}
	typ := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
		".svg": "image/svg+xml", ".gif": "image/gif", ".webp": "image/webp"}[strings.ToLower(filepath.Ext(spec))]
	if typ == "" {
		return "", fmt.Errorf("%s: png, jpg, svg, gif or webp", spec)
	}
	return "data:" + typ + ";base64," + base64.StdEncoding.EncodeToString(b), nil
}

// ----------------------------------------------------------------------------
// The terminal
// ----------------------------------------------------------------------------

func money(c common.TimesheetClient, x float64) string {
	s := strconv.FormatFloat(math.Abs(x), 'f', 2, 64)
	whole, frac, _ := strings.Cut(s, ".")
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "," + whole[i:]
	}
	s = whole + "." + frac
	if c.CurrencySymbol != "" {
		s = c.CurrencySymbol + s
	} else {
		s += " " + c.Currency
	}
	if x < 0 {
		s = "-" + s
	}
	return s
}

func hm(mins float64) string {
	m := int(mins + 0.5)
	return fmt.Sprintf("%d:%02d", m/60, m%60)
}

func hours(mins float64) string { return strconv.FormatFloat(mins/60, 'f', 2, 64) }

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func (self *Invoice) text(ts *common.Timesheet) {
	B, D, R, A := commands.C(commands.AnsiBold), commands.C(commands.AnsiDim), commands.C(commands.AnsiReset), commands.C(commands.AnsiGold)
	width := 72
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 50 {
		width = min(w, 100)
	}
	desc := width - 34
	if self.kind == "timesheet" {
		desc = width - 30
	}
	rule := D + strings.Repeat("─", width) + R

	shown := 0
	for _, c := range ts.Clients {
		if self.kind == "invoice" && (c.Key == "" || len(c.Lines) == 0) {
			continue
		}
		shown++
		fmt.Println()
		if self.kind == "invoice" {
			fmt.Printf("%s%s%s  %s%s · due %s%s\n", B, c.Name, R, D, c.Number, c.Due, R)
			fmt.Println(rule)
			project := "\x00"
			for _, l := range c.Lines {
				if l.Project != project {
					project = l.Project
					if project != "" {
						fmt.Printf("%s%s%s\n", A, cut(project, width), R)
					}
				}
				fmt.Printf("  %-*s %7s h %10s %12s\n", desc, cut(l.Description, desc), hours(l.BilledMins),
					money(c, l.Rate), money(c, l.Amount))
			}
			fmt.Println(rule)
			fmt.Printf("  %-*s %7s h %10s %12s\n", desc, "Subtotal", hours(c.BilledMins), "", money(c, c.Subtotal))
			if c.TaxRate != 0 {
				fmt.Printf("  %-*s %7s   %10s %12s\n", desc, fmt.Sprintf("%s %s%%", c.TaxName,
					strconv.FormatFloat(c.TaxRate, 'f', -1, 64)), "", "", money(c, c.Tax))
			}
			fmt.Printf("  %s%-*s %7s   %10s %12s%s\n", B, desc, "Total "+c.Currency, "", "", money(c, c.Total), R)
			if c.UnbillableMins > 0 {
				fmt.Printf("  %s%s not billable%s\n", D, hm(c.UnbillableMins), R)
			}
			continue
		}
		fmt.Printf("%s%s%s  %s%s clocked%s\n", B, c.Name, R, D, hm(c.Mins), R)
		fmt.Println(rule)
		day := ""
		for _, e := range c.Entries {
			if e.Date != day {
				day = e.Date
				t, _ := time.Parse("2006-01-02", day)
				fmt.Printf("%s%s%s\n", A, t.Format("Mon Jan 2"), R)
			}
			end := e.End
			if end == "" {
				end = "now"
			}
			mark := ""
			if !e.Billable {
				mark = D + " (not billed)" + R
			}
			fmt.Printf("  %s–%-5s %-*s %6s%s\n", e.Start, end, desc, cut(e.Task, desc), hm(e.Mins), mark)
		}
		fmt.Println(rule)
	}
	if shown == 0 {
		what := "billable time"
		if self.kind == "timesheet" {
			what = "time"
		}
		fmt.Printf("No %s clocked in %s.\n", what, ts.Range)
		return
	}
	fmt.Println()
	if self.kind == "invoice" && ts.Round > 0 {
		fmt.Printf("%slines rounded %s to %d minutes · %s%s\n", D, ts.RoundMode, ts.Round, ts.Range, R)
	} else {
		fmt.Printf("%s%s%s\n", D, ts.Range, R)
	}
}

// ----------------------------------------------------------------------------
// pdf
// ----------------------------------------------------------------------------

// Printing is commands.PrintPDF (chromepdf.go), shared with orgs print: a
// headless Chrome, Chromium or Edge with a profile of its own.

func openFile(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	cmd.Start()
}

func init() {
	commands.AddCmd("invoice", "clocked time by client as invoices: text, csv, html or pdf, in several styles",
		func() commands.Cmd { return &Invoice{kind: "invoice"} })
	commands.AddCmd("timesheet", "clocked time by client, a row a sitting: text, csv, html or pdf",
		func() commands.Cmd { return &Invoice{kind: "timesheet"} })
}
