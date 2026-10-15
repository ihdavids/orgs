package orgs

/* SDOC: Editing
* Timesheets and Invoices

  Clocked time, cut by who it was for. =orgs timesheet= lists every sitting in
  a range with its client, rate and amount; =orgs invoice= adds them up into
  one invoice per client, as a terminal summary, csv, html or pdf.

  Time belongs to a client by the first of:

  1. a =:CLIENT:= property on the heading or any heading above it, or the
     file's =#+PROPERTY: CLIENT acme= (or =#+CLIENT: acme=);
  2. a tag on the heading or above it naming a client in =invoice.clients= (or
     its =aliases=), or listed in =invoice.clientTags=.

  With =-by tag= only tags count (the first one in =-tags=, else in the
  settings, nearest the heading first), and =-by project= / =-by file= bill by
  the nearest =:project:= heading or by file.

  | Property     | Inherited | Means                                        |
  |--------------+-----------+----------------------------------------------|
  | =CLIENT=     | yes       | whose time this is                           |
  | =RATE=       | yes       | an hour, overriding the client's rate        |
  | =BILLABLE=   | yes       | =no= (or =nil=, =false=) keeps it off invoices |

  A tag =nobill= or =nonbillable= does what =BILLABLE: no= does. Unbillable
  time is listed on a timesheet and left off an invoice.

  Rounding is per invoice line - a task's time in the range, or with
  =-lines day= a task's time on each day - never per sitting, so ten
  five-minute sittings bill as an hour's quarter, not as ten.

  The look of an html or pdf invoice is a theme (=orgs invoice -list-themes=):
  =classic=, =modern=, =minimal=, =ledger=, =studio=, =letterpress=. A theme
  is one stylesheet in =invoice/themes/= on the template path, so a new one
  is a file, no code; =-accent=, =-paper= and =invoice.logo= are on top of
  any of them, and =-css= adds your own rules last.
EDOC */

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/assets"
	"github.com/ihdavids/orgs/internal/common"
)

// What a request asks of a timesheet, past the range.
type timesheetOpts struct {
	By         string   // client (default), tag, project, file
	Tags       []string // the tags that are clients, for by=tag
	Client     string   // only this client
	Rate       float64  // an hour, when neither the heading nor the client says
	Round      int      // minutes; 0 = the settings', -1 = none
	RoundMode  string
	Lines      string // task (default) or day
	Date       time.Time
	Number     string
	Unbillable bool // put unbillable time on invoice lines too, at no charge
}

// What is inherited down the outline.
type tsScope struct {
	client   string
	rate     float64
	hasRate  bool
	billable bool
	project  string
	tags     []string // nearest first
}

func tsProp(sec *org.Section, name string) (string, bool) {
	if sec.Headline == nil || sec.Headline.Properties == nil {
		return "", false
	}
	for _, p := range sec.Headline.Properties.Properties {
		if strings.EqualFold(p[0], name) {
			return strings.TrimSpace(p[1]), true
		}
	}
	return "", false
}

// A file's own word for a property: #+PROPERTY: NAME value, or #+NAME: value.
func tsFileProp(doc *org.Document, name string) string {
	for _, p := range strings.Split(doc.Get("PROPERTY"), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(p), " ")
		if ok && strings.EqualFold(k, name) {
			return strings.TrimSpace(v)
		}
	}
	return strings.TrimSpace(doc.Get(strings.ToUpper(name)))
}

func tsNo(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "no", "nil", "false", "0", "n", "off":
		return true
	}
	return false
}

func tsRate(v string) (float64, bool) {
	v = strings.TrimLeft(strings.TrimSpace(v), "$€£¥")
	f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", ""), 64)
	return f, err == nil
}

// roundMins rounds a line's minutes to the billing increment.
func roundMins(mins float64, inc int, mode string) float64 {
	if inc <= 0 || mins <= 0 {
		return math.Round(mins)
	}
	// To the minute first: clock lines are written to the minute, so a
	// fraction is a running clock's seconds or float dust, and a second over
	// the quarter is not another quarter.
	n := math.Round(mins) / float64(inc)
	switch mode {
	case "nearest":
		n = math.Round(n)
	case "down":
		n = math.Floor(n)
	default:
		n = math.Ceil(n)
	}
	return n * float64(inc)
}

func cents(x float64) float64 { return math.Round(x*100) / 100 }

// The client key an entry belongs to, by the request's way of deciding.
func (o *timesheetOpts) clientOf(sc *tsScope, sets *common.InvoiceSettings, file string) string {
	isClientTag := func(t string) bool {
		if len(o.Tags) > 0 {
			for _, x := range o.Tags {
				if strings.EqualFold(x, t) {
					return true
				}
			}
			return false
		}
		if k, _ := sets.ClientFor(t); k != "" {
			return true
		}
		for _, x := range sets.ClientTags {
			if strings.EqualFold(x, t) {
				return true
			}
		}
		return false
	}
	byTag := func(anyTag bool) string {
		for _, t := range sc.tags {
			if isClientTag(t) {
				return t
			}
		}
		if anyTag {
			// Nothing named the clients, so any tag is one - except the ones
			// that mark what a heading is rather than who it is for.
			for _, t := range sc.tags {
				switch strings.ToLower(t) {
				case "project", "archive", "archived", "nobill", "nonbillable":
					continue
				}
				return t
			}
		}
		return ""
	}
	switch o.By {
	case "tag":
		return byTag(len(o.Tags) == 0 && len(sets.Clients) == 0 && len(sets.ClientTags) == 0)
	case "project":
		return sc.project
	case "file":
		return strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	}
	if sc.client != "" {
		return sc.client
	}
	return byTag(false)
}

// buildTimesheet walks every file's clocks in the range.
func buildTimesheet(files []*common.OrgFile, sets *common.InvoiceSettings, r clockRange, o timesheetOpts, now time.Time) *common.Timesheet {
	if sets == nil {
		sets = &common.InvoiceSettings{}
	}
	if o.By == "" {
		o.By = "client"
	}
	if o.Lines == "" {
		o.Lines = "task"
	}
	if o.Round == 0 {
		o.Round = sets.Round
	}
	if o.Round < 0 {
		o.Round = 0
	}
	if o.RoundMode == "" {
		o.RoundMode = sets.RoundMode
	}
	if o.RoundMode == "" {
		o.RoundMode = "up"
	}
	if o.Date.IsZero() {
		o.Date = now
	}
	ts := &common.Timesheet{Ok: true, By: o.By, Round: o.Round, RoundMode: o.RoundMode, Lines: o.Lines,
		Issuer: sets.From, Payment: sets.Payment, Range: r.Label}
	if r.Set {
		if !r.From.IsZero() {
			ts.From = r.From.Format("2006-01-02")
		}
		ts.To = r.To.Add(-time.Second).Format("2006-01-02")
	}

	byKey := map[string]*common.TimesheetClient{}
	order := []string{}
	clientFor := func(key string) *common.TimesheetClient {
		canon := key
		if k, _ := sets.ClientFor(key); k != "" {
			canon = k
		}
		id := strings.ToLower(canon)
		if c, ok := byKey[id]; ok {
			return c
		}
		c := &common.TimesheetClient{Key: canon, Name: canon}
		if canon == "" {
			c.Name = "(no client)"
		}
		byKey[id] = c
		order = append(order, id)
		return c
	}

	var walk func(sec *org.Section, f *common.OrgFile, sc tsScope)
	walk = func(sec *org.Section, f *common.OrgFile, sc tsScope) {
		h := sec.Headline
		if h == nil {
			return
		}
		if v, ok := tsProp(sec, "CLIENT"); ok && v != "" {
			sc.client = v
		}
		if v, ok := tsProp(sec, "RATE"); ok {
			if x, ok := tsRate(v); ok {
				sc.rate, sc.hasRate = x, true
			}
		}
		if v, ok := tsProp(sec, "BILLABLE"); ok {
			sc.billable = !tsNo(v)
		}
		title := common.GetSectionTitle(sec)
		if v, ok := tsProp(sec, "PROJECT"); ok && v != "" {
			sc.project = v
		} else if IsProjectByTag(sec) || sc.project == "" {
			// The nearest :project: heading, else the top of the outline.
			sc.project = title
		}
		own := append([]string{}, h.Tags...)
		sc.tags = append(own, sc.tags...)
		billable := sc.billable
		for _, t := range h.Tags {
			if strings.EqualFold(t, "nobill") || strings.EqualFold(t, "nonbillable") {
				billable = false
			}
		}
		sc.billable = billable

		project := sc.project
		if project == title && (sec.Parent == nil || sec.Parent.Headline == nil) {
			// A top-level heading is its own project only when something is
			// under it; a lone task has no project to name.
			if len(sec.Children) == 0 {
				project = ""
			}
		}

		for _, sp := range clockSessionsIn(h, r, now) {
			key := o.clientOf(&sc, sets, f.Filename)
			c := clientFor(key)
			start, _ := time.ParseInLocation("2006-01-02T15:04", sp.Start, now.Location())
			e := common.TimesheetEntry{
				Date: start.Format("2006-01-02"), Start: start.Format("15:04"), Mins: sp.Mins,
				Client: c.Key, Project: project, Task: title, Hash: sec.Hash,
				Filename: f.Filename, LineNum: h.Pos.Row, Tags: append([]string{}, sc.tags...),
				Billable: billable,
			}
			if sp.End != "" {
				if end, err := time.ParseInLocation("2006-01-02T15:04", sp.End, now.Location()); err == nil {
					e.End = end.Format("15:04")
				}
			}
			e.Rate = o.rateFor(&sc, sets, c.Key)
			if billable {
				e.Amount = cents(sp.Mins / 60 * e.Rate)
			}
			c.Entries = append(c.Entries, e)
		}
		for _, ch := range sec.Children {
			walk(ch, f, sc)
		}
	}

	for _, f := range files {
		if f == nil || f.Doc == nil {
			continue
		}
		sc := tsScope{billable: true, client: tsFileProp(f.Doc, "CLIENT")}
		if v := tsFileProp(f.Doc, "RATE"); v != "" {
			sc.rate, sc.hasRate = tsRate(v)
		}
		if v := tsFileProp(f.Doc, "BILLABLE"); v != "" {
			sc.billable = !tsNo(v)
		}
		for _, t := range strings.Split(f.Doc.Get("FILETAGS"), ":") {
			if t = strings.TrimSpace(t); t != "" {
				sc.tags = append(sc.tags, t)
			}
		}
		for _, sec := range f.Doc.Outline.Children {
			walk(sec, f, sc)
		}
	}

	for _, id := range order {
		c := byKey[id]
		if o.Client != "" {
			if k, _ := sets.ClientFor(o.Client); !(strings.EqualFold(c.Key, o.Client) || (k != "" && strings.EqualFold(k, c.Key))) {
				continue
			}
		}
		o.finish(c, sets, r)
		ts.Mins += c.Mins
		ts.BilledMins += c.BilledMins
		if c.Key == "" {
			ts.NoClientMins += c.Mins
		}
		ts.Clients = append(ts.Clients, *c)
	}
	// Clients by name, the unattributed time last where it is noticed.
	sort.SliceStable(ts.Clients, func(a, b int) bool {
		x, y := ts.Clients[a], ts.Clients[b]
		if (x.Key == "") != (y.Key == "") {
			return y.Key == ""
		}
		return strings.ToLower(x.Name) < strings.ToLower(y.Name)
	})
	if ts.NoClientMins > 0 {
		ts.Warnings = append(ts.Warnings, fmt.Sprintf("%s clocked with no client: give it a CLIENT property or a client's tag",
			clockHM(ts.NoClientMins)))
	}
	if o.Client != "" && len(ts.Clients) == 0 {
		ts.Warnings = append(ts.Warnings, fmt.Sprintf("nothing clocked for %s in %s", o.Client, ts.Range))
	}
	if days := sets.TermDays; days > 0 {
		ts.Terms = fmt.Sprintf("Net %d", days)
	}
	return ts
}

func (o *timesheetOpts) rateFor(sc *tsScope, sets *common.InvoiceSettings, key string) float64 {
	if sc.hasRate {
		return sc.rate
	}
	if _, c := sets.ClientFor(key); c != nil && c.Rate > 0 {
		return c.Rate
	}
	if o.Rate > 0 {
		return o.Rate
	}
	return sets.Rate
}

// finish adds a client's sessions up into invoice lines and totals.
func (o *timesheetOpts) finish(c *common.TimesheetClient, sets *common.InvoiceSettings, r clockRange) {
	_, cc := sets.ClientFor(c.Key)
	if cc != nil {
		c.BillTo = cc.InvoiceParty
		if cc.Name != "" {
			c.Name = cc.Name
		}
		c.PO, c.Notes = cc.PO, cc.Notes
	}
	if c.BillTo.Name == "" {
		c.BillTo.Name = c.Name
	}
	if c.Notes == "" {
		c.Notes = sets.Notes
	}
	c.Currency = strings.ToUpper(sets.Currency)
	if cc != nil && cc.Currency != "" {
		c.Currency = strings.ToUpper(cc.Currency)
	}
	if c.Currency == "" {
		c.Currency = "USD"
	}
	c.CurrencySymbol = common.CurrencySymbols[c.Currency]
	if sets.CurrencySymbol != "" && (cc == nil || cc.Currency == "" || strings.EqualFold(cc.Currency, sets.Currency)) {
		c.CurrencySymbol = sets.CurrencySymbol
	}
	c.Rate = o.Rate
	if c.Rate == 0 {
		c.Rate = sets.Rate
	}
	if cc != nil && cc.Rate > 0 {
		c.Rate = cc.Rate
	}
	c.TaxRate, c.TaxName = sets.TaxRate, sets.TaxName
	if cc != nil && cc.TaxRate != nil {
		c.TaxRate = *cc.TaxRate
	}
	if cc != nil && cc.TaxName != "" {
		c.TaxName = cc.TaxName
	}
	if c.TaxName == "" {
		c.TaxName = "Tax"
	}

	sort.SliceStable(c.Entries, func(a, b int) bool {
		x, y := c.Entries[a], c.Entries[b]
		if x.Date != y.Date {
			return x.Date < y.Date
		}
		return x.Start < y.Start
	})

	type lineKey struct{ hash, task, day string }
	lines := map[lineKey]*common.InvoiceLine{}
	lineOrder := []lineKey{}
	for _, e := range c.Entries {
		c.Mins += e.Mins
		if !e.Billable {
			c.UnbillableMins += e.Mins
			if !o.Unbillable {
				continue
			}
		}
		k := lineKey{hash: e.Hash, task: e.Project + "\x00" + e.Task}
		if o.Lines == "day" {
			k.day = e.Date
		}
		ln, ok := lines[k]
		if !ok {
			ln = &common.InvoiceLine{Description: e.Task, Project: e.Project, Date: e.Date, Hash: e.Hash, Rate: e.Rate}
			if !e.Billable {
				ln.Rate = 0
			}
			lines[k] = ln
			lineOrder = append(lineOrder, k)
		}
		ln.Mins += e.Mins
		ln.LastDate = e.Date
		ln.Sessions++
	}
	for _, k := range lineOrder {
		ln := lines[k]
		ln.BilledMins = roundMins(ln.Mins, o.Round, o.RoundMode)
		ln.Hours = ln.BilledMins / 60
		ln.Amount = cents(ln.Hours * ln.Rate)
		c.BilledMins += ln.BilledMins
		c.Subtotal += ln.Amount
		c.Lines = append(c.Lines, *ln)
	}
	// Grouped by project, each project's lines in the order they were worked.
	sort.SliceStable(c.Lines, func(a, b int) bool {
		x, y := c.Lines[a], c.Lines[b]
		if x.Project != y.Project {
			return strings.ToLower(x.Project) < strings.ToLower(y.Project)
		}
		return x.Date < y.Date
	})
	c.Hours = c.BilledMins / 60
	c.Subtotal = cents(c.Subtotal)
	c.Tax = cents(c.Subtotal * c.TaxRate / 100)
	c.Total = cents(c.Subtotal + c.Tax)

	c.Date = o.Date.Format("2006-01-02")
	days := 30
	if sets.TermDays > 0 {
		days = sets.TermDays
	}
	if cc != nil && cc.TermDays != nil {
		days = *cc.TermDays
	}
	c.Due = o.Date.AddDate(0, 0, days).Format("2006-01-02")

	pattern := o.Number
	if pattern == "" && cc != nil {
		pattern = cc.Number
	}
	if pattern == "" {
		pattern = sets.Number
	}
	if pattern == "" {
		pattern = "{CLIENT}-{yyyy}{mm}"
	}
	// The period's month, not the day it was sent: October's invoice is
	// October's even when it goes out on the 2nd of November.
	when := o.Date
	if r.Set && !r.From.IsZero() {
		when = r.From
	}
	c.Number = invoiceNumber(pattern, c.Key, when)
}

var invoiceSlugRe = regexp.MustCompile(`[^A-Za-z0-9]+`)

func invoiceNumber(pattern, key string, when time.Time) string {
	slug := strings.Trim(invoiceSlugRe.ReplaceAllString(key, ""), "-")
	if slug == "" {
		slug = "INV"
	}
	return strings.NewReplacer(
		"{client}", strings.ToLower(slug), "{CLIENT}", strings.ToUpper(slug),
		"{yyyy}", when.Format("2006"), "{yy}", when.Format("06"),
		"{mm}", when.Format("01"), "{dd}", when.Format("02"),
	).Replace(pattern)
}

// ----------------------------------------------------------------------------
// csv
// ----------------------------------------------------------------------------

func hoursStr(mins float64) string {
	return strconv.FormatFloat(math.Round(mins/60*100)/100, 'f', 2, 64)
}

func moneyStr(x float64) string { return strconv.FormatFloat(x, 'f', 2, 64) }

// timesheetCSV: a row a sitting - what a timesheet is - for a spreadsheet or
// another tool's import (Harvest, Toggl, a payroll sheet).
func timesheetCSV(ts *common.Timesheet) []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.Write([]string{"Date", "Start", "End", "Client", "Project", "Task", "Hours", "Duration", "Billable",
		"Rate", "Amount", "Currency", "Tags", "File", "Line"})
	for _, c := range ts.Clients {
		for _, e := range c.Entries {
			w.Write([]string{e.Date, e.Start, e.End, c.Name, e.Project, e.Task, hoursStr(e.Mins), clockHM(e.Mins),
				yes(e.Billable), moneyStr(e.Rate), moneyStr(e.Amount), c.Currency, strings.Join(e.Tags, " "),
				e.Filename, strconv.Itoa(e.LineNum + 1)}) // 1-based, as an editor counts
		}
	}
	w.Flush()
	return b.Bytes()
}

// invoiceCSV: a row an invoice line, for an accounting package's import
// (QuickBooks, Xero and FreshBooks all take a line per row with the invoice
// number repeated).
func invoiceCSV(ts *common.Timesheet) []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.Write([]string{"Invoice", "Invoice Date", "Due Date", "Client", "Project", "Description", "First Date", "Last Date",
		"Hours", "Clocked", "Rate", "Amount", "Currency", "Tax Rate"})
	for _, c := range ts.Clients {
		if c.Key == "" {
			continue
		}
		for _, l := range c.Lines {
			w.Write([]string{c.Number, c.Date, c.Due, c.Name, l.Project, l.Description, l.Date, l.LastDate,
				hoursStr(l.BilledMins), clockHM(l.Mins), moneyStr(l.Rate), moneyStr(l.Amount), c.Currency,
				strconv.FormatFloat(c.TaxRate, 'f', -1, 64)})
		}
	}
	w.Flush()
	return b.Bytes()
}

func yes(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// ----------------------------------------------------------------------------
// html
// ----------------------------------------------------------------------------

// InvoiceThemes is every stylesheet in invoice/themes on the template path.
func InvoiceThemes() []string {
	out := []string{}
	for _, n := range assets.GlobTemplates("invoice/themes/*.css", Conf().Server.TemplatePath) {
		out = append(out, strings.TrimSuffix(filepath.Base(n), ".css"))
	}
	return out
}

type invoiceLook struct {
	Kind   string // invoice or timesheet
	Theme  string
	Accent string
	Paper  string
	Logo   string
	Title  string
}

var cssColorRe = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|[a-zA-Z]{3,20}|(rgb|hsl|oklch)a?\([0-9.,% /a-z]+\))$`)

// A logo as a data: url, so the page (and the pdf made from it) carries it.
func logoURL(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", nil
	}
	if strings.HasPrefix(spec, "http://") || strings.HasPrefix(spec, "https://") || strings.HasPrefix(spec, "data:") {
		return spec, nil
	}
	if strings.HasPrefix(spec, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			spec = filepath.Join(home, spec[2:])
		}
	}
	b, err := os.ReadFile(spec)
	if err != nil {
		return "", err
	}
	typ := mime.TypeByExtension(strings.ToLower(filepath.Ext(spec)))
	if typ == "" {
		typ = http.DetectContentType(b)
	}
	return "data:" + typ + ";base64," + base64.StdEncoding.EncodeToString(b), nil
}

func fmtMoney(x float64, symbol, code string) string {
	neg := x < 0
	s := strconv.FormatFloat(math.Abs(x), 'f', 2, 64)
	whole, frac, _ := strings.Cut(s, ".")
	var g []string
	for len(whole) > 3 {
		g = append([]string{whole[len(whole)-3:]}, g...)
		whole = whole[:len(whole)-3]
	}
	g = append([]string{whole}, g...)
	s = strings.Join(g, ",") + "." + frac
	if symbol != "" {
		s = symbol + s
	} else if code != "" {
		s += " " + code
	}
	if neg {
		s = "−" + s
	}
	return s
}

func longDate(d string) string {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return d
	}
	return t.Format("January 2, 2006")
}

func shortDate(d string) string {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return d
	}
	return t.Format("Jan 2")
}

var invoiceFuncs = template.FuncMap{
	"money": func(c common.TimesheetClient, x float64) string { return fmtMoney(x, c.CurrencySymbol, c.Currency) },
	"hours": func(mins float64) string { return hoursStr(mins) },
	"hm":    clockHM,
	"long":  longDate,
	"short": shortDate,
	"pct":   func(x float64) string { return strconv.FormatFloat(x, 'f', -1, 64) },
	"span": func(a, b string) string {
		if b == "" || a == b {
			return shortDate(a)
		}
		return shortDate(a) + " – " + shortDate(b)
	},
	// The lines of a client, a group per project, so the template need not
	// remember the previous row.
	"byProject": func(lines []common.InvoiceLine) [][]common.InvoiceLine {
		out := [][]common.InvoiceLine{}
		for i, l := range lines {
			if i == 0 || l.Project != lines[i-1].Project {
				out = append(out, nil)
			}
			out[len(out)-1] = append(out[len(out)-1], l)
		}
		return out
	},
	"billed": func(cs []common.TimesheetClient) []common.TimesheetClient {
		out := []common.TimesheetClient{}
		for _, c := range cs {
			if c.Key != "" && len(c.Lines) > 0 {
				out = append(out, c)
			}
		}
		return out
	},
	"days": func(es []common.TimesheetEntry) [][]common.TimesheetEntry {
		out := [][]common.TimesheetEntry{}
		for i, e := range es {
			if i == 0 || e.Date != es[i-1].Date {
				out = append(out, nil)
			}
			out[len(out)-1] = append(out[len(out)-1], e)
		}
		return out
	},
	"sum": func(es []common.TimesheetEntry) float64 {
		t := 0.0
		for _, e := range es {
			t += e.Mins
		}
		return t
	},
	// The masthead's two values, since a template call takes one.
	"dict2": func(t *common.Timesheet, logo template.URL) map[string]any {
		return map[string]any{"T": t, "Logo": logo}
	},
	"weekday": func(d string) string {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			return ""
		}
		return t.Format("Monday")
	},
}

// timesheetHTML renders the invoice (or timesheet) page: one template, one
// theme's stylesheet inlined so the page stands alone.
func timesheetHTML(ts *common.Timesheet, look invoiceLook) ([]byte, error) {
	tplPath := assets.FindTemplate("invoice/invoice.html", Conf().Server.TemplatePath)
	if tplPath == "" {
		return nil, fmt.Errorf("no invoice/invoice.html on the template path")
	}
	if look.Theme == "" {
		look.Theme = "classic"
	}
	cssPath := assets.FindTemplate("invoice/themes/"+filepath.Base(look.Theme)+".css", Conf().Server.TemplatePath)
	if cssPath == "" {
		return nil, fmt.Errorf("no invoice theme %q (themes: %s)", look.Theme, strings.Join(InvoiceThemes(), ", "))
	}
	css, err := os.ReadFile(cssPath)
	if err != nil {
		return nil, err
	}
	base := []byte{}
	if p := assets.FindTemplate("invoice/base.css", Conf().Server.TemplatePath); p != "" {
		if base, err = os.ReadFile(p); err != nil {
			return nil, err
		}
	}
	tpl, err := template.New(filepath.Base(tplPath)).Funcs(invoiceFuncs).ParseFiles(tplPath)
	if err != nil {
		return nil, err
	}
	page := ""
	switch strings.ToLower(look.Paper) {
	case "a4":
		page = "A4"
	case "", "letter":
		page = "letter"
	case "legal":
		page = "legal"
	default:
		return nil, fmt.Errorf("paper %q: letter, a4 or legal", look.Paper)
	}
	accent := ""
	if a := strings.TrimSpace(look.Accent); a != "" {
		if !cssColorRe.MatchString(a) {
			return nil, fmt.Errorf("accent %q is not a colour", a)
		}
		accent = a
	}
	logo, err := logoURL(look.Logo)
	if err != nil {
		return nil, fmt.Errorf("logo: %v", err)
	}
	title := look.Title
	if title == "" {
		title = "Timesheet"
		if look.Kind == "invoice" {
			title = "Invoice"
			if b := (invoiceFuncs["billed"].(func([]common.TimesheetClient) []common.TimesheetClient))(ts.Clients); len(b) == 1 {
				title = "Invoice " + b[0].Number
			}
		}
	}
	var out bytes.Buffer
	err = tpl.Execute(&out, map[string]any{
		"T":      ts,
		"Kind":   look.Kind,
		"Theme":  look.Theme,
		"Base":   template.CSS(base),
		"CSS":    template.CSS(css),
		"Accent": template.CSS(accent),
		"Page":   template.CSS(page),
		"Logo":   template.URL(logo),
		"Title":  title,
	})
	return out.Bytes(), err
}

// ----------------------------------------------------------------------------
// The endpoint
// ----------------------------------------------------------------------------

/* SDOC: API
* GET /timesheet — Clocked Time by Client, as a Timesheet or Invoice

	Every CLOCK line in a range, attributed to a client and priced (see
	*Timesheets and Invoices* for how a client is decided). Answers json by
	default; =format=csv= or =format=html= with =kind=timesheet= (a row a
	sitting) or =kind=invoice= (a row an invoice line, one invoice per client).

	*Method:* =GET=

	*Query Parameters:*
	| Parameter    | Description                                                         |
	|--------------+---------------------------------------------------------------------|
	| =block=      | the range, as a clock table writes it (=lastmonth=, =2026-W41=, ...)  |
	| =tstart=, =tend= | the range's edges as dates, narrowing or replacing =block=      |
	| =by=         | client (default), tag, project, file                                |
	| =tags=       | with by=tag, the tags that are clients, comma separated             |
	| =client=     | only this client                                                    |
	| =rate=       | an hour, where neither heading nor client says                      |
	| =round=      | minutes each line is rounded to (=0= for the settings', =-1= none)  |
	| =roundMode=  | up (default), nearest, down                                         |
	| =lines=      | task (default) or day                                               |
	| =date=       | the invoice date (default today)                                    |
	| =number=     | the invoice number, or a pattern with ={CLIENT}= ={yyyy}= ={mm}=    |
	| =unbillable= | =t= to list unbillable time on invoices at no charge                |
	| =format=     | json (default), csv, html                                           |
	| =kind=       | invoice (default) or timesheet                                      |
	| =theme=, =accent=, =paper=, =logo= | the html's look, over =invoice:= settings      |
	| =themes=     | =t= answers the theme names instead                                 |

	*Response:* a =Timesheet= object, =text/csv= or =text/html=.
EDOC */
func RequestTimesheet(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("themes") == "t" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(InvoiceThemes())
		return
	}
	now := time.Now()
	sets := &Conf().Server.Invoice
	params := map[string]string{"block": q.Get("block"), "tstart": q.Get("tstart"), "tend": q.Get("tend")}
	rng, err := clockRangeOf(params, now)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	o := timesheetOpts{
		By: strings.ToLower(q.Get("by")), Client: q.Get("client"), RoundMode: strings.ToLower(q.Get("roundMode")),
		Lines: strings.ToLower(q.Get("lines")), Number: q.Get("number"), Unbillable: q.Get("unbillable") == "t",
	}
	switch o.By {
	case "", "client", "tag", "project", "file":
	default:
		http.Error(w, fmt.Sprintf("by %q: client, tag, project or file", o.By), http.StatusBadRequest)
		return
	}
	switch o.Lines {
	case "", "task", "day":
	default:
		http.Error(w, fmt.Sprintf("lines %q: task or day", o.Lines), http.StatusBadRequest)
		return
	}
	switch o.RoundMode {
	case "", "up", "nearest", "down":
	default:
		http.Error(w, fmt.Sprintf("roundMode %q: up, nearest or down", o.RoundMode), http.StatusBadRequest)
		return
	}
	for _, t := range strings.Split(q.Get("tags"), ",") {
		if t = strings.TrimSpace(t); t != "" {
			o.Tags = append(o.Tags, strings.TrimPrefix(t, ":"))
		}
	}
	if v := q.Get("rate"); v != "" {
		if o.Rate, err = strconv.ParseFloat(v, 64); err != nil {
			http.Error(w, fmt.Sprintf("rate %q is not a number", v), http.StatusBadRequest)
			return
		}
	}
	if v := q.Get("round"); v != "" {
		if o.Round, err = strconv.Atoi(v); err != nil {
			http.Error(w, fmt.Sprintf("round %q is not a number of minutes", v), http.StatusBadRequest)
			return
		}
	}
	if v := q.Get("date"); v != "" {
		if o.Date, err = time.ParseInLocation("2006-01-02", v, now.Location()); err != nil {
			http.Error(w, fmt.Sprintf("date %q: YYYY-MM-DD", v), http.StatusBadRequest)
			return
		}
	}

	files := []*common.OrgFile{}
	for _, fname := range GetDb().GetFiles() {
		files = append(files, GetDb().GetFile(fname))
	}
	registerAllSections()
	ts := buildTimesheet(files, sets, rng, o, now)

	kind := strings.ToLower(q.Get("kind"))
	if kind == "" {
		kind = "invoice"
	}
	if kind != "invoice" && kind != "timesheet" {
		http.Error(w, fmt.Sprintf("kind %q: invoice or timesheet", kind), http.StatusBadRequest)
		return
	}
	switch strings.ToLower(q.Get("format")) {
	case "", "json":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ts)
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		if kind == "invoice" {
			w.Write(invoiceCSV(ts))
		} else {
			w.Write(timesheetCSV(ts))
		}
	case "html":
		look := invoiceLook{Kind: kind, Theme: sets.Theme, Accent: sets.Accent, Paper: sets.Paper, Logo: sets.Logo}
		for k, p := range map[string]*string{"theme": &look.Theme, "accent": &look.Accent, "paper": &look.Paper, "title": &look.Title} {
			if v := q.Get(k); v != "" {
				*p = v
			}
		}
		// A logo from a request is a url: a path would let any client read
		// any file of the server's into the page. Paths come from the yaml.
		if v := strings.TrimSpace(q.Get("logo")); v != "" {
			if !(strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "data:image/")) {
				http.Error(w, "logo: a request can only name an http(s) or data:image url; a file goes in invoice.logo", http.StatusBadRequest)
				return
			}
			look.Logo = v
		}
		page, err := timesheetHTML(ts, look)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page)
	default:
		http.Error(w, fmt.Sprintf("format %q: json, csv or html", q.Get("format")), http.StatusBadRequest)
	}
}
