package orgs

import (
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

const tsOrg = `#+TODO: TODO NEXT | DONE
* Acme site  :project:
:PROPERTIES:
:CLIENT: acme
:END:
** Discovery
:LOGBOOK:
CLOCK: [2026-09-02 Wed 09:00]--[2026-09-02 Wed 11:40] =>  2:40
CLOCK: [2026-09-03 Thu 13:10]--[2026-09-03 Thu 14:05] =>  0:55
:END:
** Retro  :nobill:
:LOGBOOK:
CLOCK: [2026-09-18 Fri 16:00]--[2026-09-18 Fri 16:45] =>  0:45
:END:
* Pipeline  :globex:
** Ingest
:PROPERTIES:
:RATE: 175
:END:
:LOGBOOK:
CLOCK: [2026-09-10 Thu 08:45]--[2026-09-10 Thu 12:15] =>  3:30
CLOCK: [2026-08-31 Mon 23:00]--[2026-09-01 Tue 01:00] =>  2:00
:END:
* Admin
:LOGBOOK:
CLOCK: [2026-09-30 Wed 17:00]--[2026-09-30 Wed 17:50] =>  0:50
:END:
`

func tsFixture(t *testing.T) ([]*common.OrgFile, *common.InvoiceSettings, clockRange, time.Time) {
	paths := loadOrg(t, map[string]string{"work.org": tsOrg})
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.Local)
	r, err := clockBlockRange("2026-09", now)
	if err != nil {
		t.Fatal(err)
	}
	tax := 0.0
	sets := &common.InvoiceSettings{
		Rate: 100, Round: 15, TaxRate: 15, TaxName: "HST", Currency: "CAD",
		Clients: map[string]common.InvoiceClient{
			"acme":   {InvoiceParty: common.InvoiceParty{Name: "Acme Corp"}, Rate: 150, Currency: "usd", TaxRate: &tax},
			"globex": {Aliases: []string{"gx"}},
		},
	}
	return []*common.OrgFile{GetDb().GetFile(paths["work.org"])}, sets, r, now
}

func tsClient(ts *common.Timesheet, key string) *common.TimesheetClient {
	for i := range ts.Clients {
		if ts.Clients[i].Key == key {
			return &ts.Clients[i]
		}
	}
	return nil
}

func TestTimesheetByClient(t *testing.T) {
	files, sets, r, now := tsFixture(t)
	ts := buildTimesheet(files, sets, r, timesheetOpts{}, now)

	acme := tsClient(ts, "acme")
	if acme == nil || acme.Name != "Acme Corp" || acme.Currency != "USD" || acme.CurrencySymbol != "$" {
		t.Fatalf("acme: %+v", acme)
	}
	// 3:35 rounds up to 3:45; the nobill retro is listed but not invoiced.
	if len(acme.Lines) != 1 || acme.Lines[0].BilledMins != 225 || acme.Lines[0].Amount != 562.5 {
		t.Errorf("acme lines: %+v", acme.Lines)
	}
	if acme.UnbillableMins != 45 || len(acme.Entries) != 3 || acme.Tax != 0 || acme.Total != 562.5 {
		t.Errorf("acme totals: %+v", acme)
	}
	if acme.Number != "ACME-202609" || acme.Due != "2026-11-13" {
		t.Errorf("acme number %s due %s", acme.Number, acme.Due)
	}

	// By tag, since nothing says CLIENT; the RATE property beats the default,
	// and a clock across the range's edge counts only its part inside.
	gx := tsClient(ts, "globex")
	if gx == nil || len(gx.Lines) != 1 {
		t.Fatalf("globex: %+v", gx)
	}
	if l := gx.Lines[0]; l.Mins != 210+60 || l.Rate != 175 || l.Amount != 787.5 {
		t.Errorf("globex line: %+v", l)
	}
	if gx.Tax != 118.13 || gx.Total != 905.63 {
		t.Errorf("globex tax %v total %v", gx.Tax, gx.Total)
	}

	if ts.NoClientMins != 50 || len(ts.Warnings) == 0 {
		t.Errorf("no client: %v %v", ts.NoClientMins, ts.Warnings)
	}
	if last := ts.Clients[len(ts.Clients)-1]; last.Key != "" {
		t.Errorf("unattributed time should come last, got %q", last.Key)
	}
}

func TestTimesheetOptions(t *testing.T) {
	files, sets, r, now := tsFixture(t)

	ts := buildTimesheet(files, sets, r, timesheetOpts{Client: "gx", Round: -1}, now)
	if len(ts.Clients) != 1 || ts.Clients[0].Key != "globex" || ts.Clients[0].BilledMins != 270 {
		t.Fatalf("an alias picks its client, -1 does not round: %+v", ts.Clients)
	}

	ts = buildTimesheet(files, sets, r, timesheetOpts{Lines: "day"}, now)
	if acme := tsClient(ts, "acme"); len(acme.Lines) != 2 || acme.Lines[0].BilledMins != 165 || acme.Lines[1].BilledMins != 60 {
		t.Errorf("a line a day, each rounded: %+v", acme.Lines)
	}

	ts = buildTimesheet(files, sets, r, timesheetOpts{By: "project"}, now)
	if c := tsClient(ts, "Acme site"); c == nil || c.Mins != 260 {
		t.Errorf("by project: %+v", ts.Clients)
	}

	ts = buildTimesheet(files, sets, r, timesheetOpts{Number: "{client}/{yy}{mm}", Unbillable: true}, now)
	acme := tsClient(ts, "acme")
	if acme.Number != "acme/2609" || len(acme.Lines) != 2 || acme.Total != 562.5 {
		t.Errorf("number and unbillable lines: %s %+v", acme.Number, acme.Lines)
	}
}

func TestRoundMins(t *testing.T) {
	for _, c := range []struct {
		mins float64
		inc  int
		mode string
		want float64
	}{
		{61, 15, "up", 75}, {60, 15, "up", 60}, {60.0000001, 15, "up", 60},
		{67, 15, "nearest", 60}, {68, 15, "nearest", 75}, {74, 15, "down", 60},
		{7, 6, "up", 12}, {42.4, 0, "up", 42},
	} {
		if got := roundMins(c.mins, c.inc, c.mode); got != c.want {
			t.Errorf("roundMins(%v, %d, %s) = %v, want %v", c.mins, c.inc, c.mode, got, c.want)
		}
	}
}

func TestInvoiceCSVAndMoney(t *testing.T) {
	files, sets, r, now := tsFixture(t)
	ts := buildTimesheet(files, sets, r, timesheetOpts{}, now)
	csv := string(invoiceCSV(ts))
	if strings.Contains(csv, "Admin") || !strings.Contains(csv, "ACME-202609,2026-10-14,2026-11-13,Acme Corp,Acme site,Discovery") {
		t.Errorf("invoice csv:\n%s", csv)
	}
	if got := fmtMoney(1234567.891, "$", "USD"); got != "$1,234,567.89" {
		t.Errorf("money %s", got)
	}
	if got := fmtMoney(12, "", "SEK"); got != "12.00 SEK" {
		t.Errorf("money %s", got)
	}
}
