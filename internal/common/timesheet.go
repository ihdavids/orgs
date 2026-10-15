package common

import "strings"

// The wire types behind /timesheet, and the invoice: settings that say who is
// billing whom.
//
// A timesheet is the clock table cut the other way: not "where did my time
// go" by file and heading, but "whose time was it" - every CLOCK line in a
// range, attributed to one client, priced at that client's rate and rounded
// the way it is billed. An invoice is the same answer with the sessions added
// up into lines.

// One CLOCK line's part of the range: a row of a timesheet.
type TimesheetEntry struct {
	Date     string // YYYY-MM-DD the session started on
	Start    string // HH:MM
	End      string // HH:MM, empty while the clock is still running
	Mins     float64
	Client   string // the client's key: a CLIENT value or a tag
	Project  string
	Task     string
	Hash     string
	Filename string
	LineNum  int
	Tags     []string
	Billable bool
	Rate     float64
	// The session's own time at the rate, before rounding: rounding is done
	// on invoice lines, where it is billed, never on each sitting.
	Amount float64
}

// One line of an invoice: a task's time in the range (or, with lines=day, a
// task's time on one day), rounded up to the billing increment and priced.
type InvoiceLine struct {
	Description string
	Project     string
	Date        string // the first day worked, or the day with lines=day
	LastDate    string
	Hash        string
	Mins        float64 // as clocked
	BilledMins  float64 // after rounding
	Hours       float64 // BilledMins / 60
	Rate        float64
	Amount      float64
	Sessions    int
}

// Who an invoice is from or to: a name and the lines under it.
type InvoiceParty struct {
	Name    string   `yaml:"name" json:"Name,omitempty"`
	Contact string   `yaml:"contact" json:"Contact,omitempty"`
	Address []string `yaml:"address" json:"Address,omitempty"`
	Email   string   `yaml:"email" json:"Email,omitempty"`
	Phone   string   `yaml:"phone" json:"Phone,omitempty"`
	Website string   `yaml:"website" json:"Website,omitempty"`
	TaxId   string   `yaml:"taxId" json:"TaxId,omitempty"`
}

// One client's part of a timesheet, and the invoice it adds up to.
type TimesheetClient struct {
	Key  string // as written on the headings
	Name string // from the settings, else Key
	// Who the invoice is to, when the settings say.
	BillTo InvoiceParty

	Entries []TimesheetEntry
	Lines   []InvoiceLine

	Mins           float64 // every session
	UnbillableMins float64 // of which nobody pays for
	BilledMins     float64 // the invoice's rounded hours, in minutes
	Hours          float64

	Currency       string
	CurrencySymbol string
	Rate           float64 // the client's rate (lines may differ by a RATE property)

	Number   string // the invoice's number
	Date     string // YYYY-MM-DD issued
	Due      string // YYYY-MM-DD
	PO       string // the client's purchase order, if they gave one
	Subtotal float64
	TaxName  string
	TaxRate  float64 // percent
	Tax      float64
	Total    float64
	Notes    string
}

// The answer to /timesheet as json.
type Timesheet struct {
	Ok    bool
	Msg   string
	Range string // "October 2026", "week 2026-W41"
	From  string // YYYY-MM-DD, inclusive
	To    string // YYYY-MM-DD, inclusive
	By    string // client, tag, project or file
	// Rounding as applied: every line up (or to the nearest) N minutes.
	Round     int
	RoundMode string
	Lines     string // task or day

	Issuer  InvoiceParty
	Payment []string
	Terms   string
	Clients []TimesheetClient

	Mins       float64
	BilledMins float64
	// Time with no client, and so on nobody's invoice: said rather than lost.
	NoClientMins float64
	Warnings     []string
}

// One client as the settings describe it. Every field may be left out.
type InvoiceClient struct {
	InvoiceParty `yaml:",inline"`
	// Other names this client is written under: tags, CLIENT values.
	Aliases  []string `yaml:"aliases"`
	Rate     float64  `yaml:"rate"`
	Currency string   `yaml:"currency"`
	TaxRate  *float64 `yaml:"taxRate"`
	TaxName  string   `yaml:"taxName"`
	TermDays *int     `yaml:"termDays"`
	PO       string   `yaml:"po"`
	Notes    string   `yaml:"notes"`
	Number   string   `yaml:"number"`
}

// InvoiceSettings is server.invoice in the yaml.
type InvoiceSettings struct {
	From     InvoiceParty `yaml:"from"`
	Currency string       `yaml:"currency"`
	// A symbol for a currency this does not know, or to write "US$".
	CurrencySymbol string  `yaml:"currencySymbol"`
	Rate           float64 `yaml:"rate"`
	// Minutes every invoice line is rounded to, and which way.
	Round     int    `yaml:"round"`
	RoundMode string `yaml:"roundMode"` // up (default), nearest, down
	TaxRate   float64 `yaml:"taxRate"`
	TaxName   string  `yaml:"taxName"`
	TermDays  int     `yaml:"termDays"`
	// The invoice number: {client} {CLIENT} {yyyy} {yy} {mm} {dd} are filled in.
	Number  string   `yaml:"number"`
	Notes   string   `yaml:"notes"`
	Payment []string `yaml:"payment"`
	// The look: a theme name, an accent colour, letter or a4, and a logo
	// (a path on the server, or an http(s) url).
	Theme  string `yaml:"theme"`
	Accent string `yaml:"accent"`
	Paper  string `yaml:"paper"`
	Logo   string `yaml:"logo"`
	// Tags that are a client when the timesheet goes by tag. Clients named
	// below count as well.
	ClientTags []string                  `yaml:"clientTags"`
	Clients    map[string]InvoiceClient `yaml:"clients"`
}

// ClientFor finds a client by key or alias, without caring about case.
func (self *InvoiceSettings) ClientFor(key string) (string, *InvoiceClient) {
	if self == nil {
		return "", nil
	}
	for k, c := range self.Clients {
		if strings.EqualFold(k, key) {
			c := c
			return k, &c
		}
		for _, a := range c.Aliases {
			if strings.EqualFold(a, key) {
				c := c
				return k, &c
			}
		}
	}
	return "", nil
}

// CurrencySymbols are the ones written in front of an amount. A currency not
// here is written as its code after the number ("1,200.00 SEK").
var CurrencySymbols = map[string]string{
	"USD": "$", "CAD": "$", "AUD": "$", "NZD": "$", "SGD": "$", "HKD": "$",
	"EUR": "€", "GBP": "£", "JPY": "¥", "CNY": "¥", "INR": "₹", "KRW": "₩",
	"ILS": "₪", "BRL": "R$", "ZAR": "R", "MXN": "$",
}
