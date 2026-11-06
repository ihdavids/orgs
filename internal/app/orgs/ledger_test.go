package orgs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

const ledgerOrg = `* Money
#+NAME: opening
#+BEGIN_SRC ledger
2026/01/01 * Opening
    Assets:Bank   $1,000.00
    Equity:Opening
#+END_SRC

** Spending
#+BEGIN_SRC ledger :cmdline reg Expenses
2026/02/03 Grocer
    Expenses:Food   $50.00
    Assets:Bank     $-50.00 = $950.00

2026/03/01 Landlord
    Expenses:Rent   $800.00
    Assets:Bank
#+END_SRC

#+BEGIN: ledger :report balance :account Expenses :depth 1
#+END:
`

// A ledger feature on, over a folder that is the org directory.
func ledgerOn(t *testing.T, files map[string]string) map[string]string {
	t.Helper()
	if Conf().Server == nil {
		t.Skip("no server settings")
	}
	paths := loadOrg(t, files)
	var dir string
	for _, p := range paths {
		dir = filepath.Dir(p)
	}
	oldDirs, oldOn := Conf().Server.OrgDirs, Conf().Server.Features.Ledger
	Conf().Server.OrgDirs = []string{dir}
	Conf().Server.Features.Ledger = true
	t.Cleanup(func() { Conf().Server.OrgDirs, Conf().Server.Features.Ledger = oldDirs, oldOn })
	return paths
}

func ledgerGet(t *testing.T, h http.HandlerFunc, url string, into interface{}) {
	t.Helper()
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest("GET", url, nil))
	if err := json.Unmarshal(w.Body.Bytes(), into); err != nil {
		t.Fatalf("%s: %v\n%s", url, err, w.Body.String())
	}
}

func TestLedgerSourcesAndBalance(t *testing.T) {
	paths := ledgerOn(t, map[string]string{"money.org": ledgerOrg})
	dir := filepath.Dir(paths["money.org"])
	os.WriteFile(filepath.Join(dir, "cash.ledger"), []byte("2026/04/01 ATM\n  Assets:Cash  $20\n  Assets:Bank\n"), 0644)

	var src struct {
		Ok      bool
		Sources []LedgerSource
	}
	ledgerGet(t, RequestLedgerSources, "/ledger/sources", &src)
	if !src.Ok || len(src.Sources) != 2 || src.Sources[1].Name != "money.org" || len(src.Sources[1].Blocks) != 2 || src.Sources[1].Transactions != 3 {
		t.Fatalf("sources: %+v", src)
	}
	var bal struct {
		Ok       bool
		Msg      string
		Text     string
		Rows     []struct{ Account string }
		Problems []struct{ Line int }
	}
	ledgerGet(t, RequestLedgerBalance, "/ledger/balance?source=money.org&account=Expenses", &bal)
	if !bal.Ok || bal.Text != "             $850.00  Expenses\n              $50.00    Food\n             $800.00    Rent\n" || len(bal.Problems) != 0 {
		t.Errorf("balance: %+v", bal)
	}
	// Every source together: the cash file joins in.
	ledgerGet(t, RequestLedgerBalance, "/ledger/balance?account=Cash", &bal)
	if !strings.Contains(bal.Text, "$20.00  Assets:Cash") {
		t.Errorf("all: %q %s", bal.Text, bal.Msg)
	}
	// One block alone, by name: only the opening.
	ledgerGet(t, RequestLedgerBalance, "/ledger/balance?source=money.org&block=opening&account=Bank", &bal)
	if !strings.Contains(bal.Text, "$1,000.00  Assets:Bank") {
		t.Errorf("block: %q %s", bal.Text, bal.Msg)
	}
	// The second block alone fails its assertion, at the org file's line.
	var chk struct {
		Ok       bool
		Problems []struct {
			Line int
			Msg  string
		}
	}
	ledgerGet(t, RequestLedgerCheck, "/ledger/check?source=money.org&block=1", &chk)
	if len(chk.Problems) != 1 || chk.Problems[0].Line != 13 {
		t.Errorf("check one block: %+v", chk)
	}
	var reg struct {
		Rows []struct {
			Payee string
			Line  int
		}
	}
	ledgerGet(t, RequestLedgerRegister, "/ledger/register?source=money.org&account=^Assets:Bank$&period=2026", &reg)
	if len(reg.Rows) != 3 || reg.Rows[1].Payee != "Grocer" || reg.Rows[1].Line != 13 {
		t.Errorf("register: %+v", reg)
	}
	var mon struct {
		Months []string
		Series []struct {
			Account string
			Values  []float64
		}
	}
	ledgerGet(t, RequestLedgerMonthly, "/ledger/monthly?source=money.org&account=^Expenses&depth=2", &mon)
	if len(mon.Months) != 2 || len(mon.Series) != 2 || mon.Series[0].Values[0] != 50 {
		t.Errorf("monthly: %+v", mon)
	}
}

func TestLedgerOffDoesNothing(t *testing.T) {
	ledgerOn(t, map[string]string{"money.org": ledgerOrg})
	Conf().Server.Features.Ledger = false
	var a struct{ Ok, Off bool }
	ledgerGet(t, RequestLedgerBalance, "/ledger/balance", &a)
	if a.Ok || !a.Off {
		t.Errorf("off: %+v", a)
	}
}

func TestLedgerCheckPostedText(t *testing.T) {
	ledgerOn(t, map[string]string{"money.org": ledgerOrg})
	body := `{"File": "x.org", "Text": "* A\n#+begin_src ledger\n2026/01/01 x\n  A  $1\n  B  $-2\n#+end_src\n"}`
	w := httptest.NewRecorder()
	PostLedgerCheck(w, httptest.NewRequest("POST", "/ledger/check", strings.NewReader(body)))
	var a struct {
		Problems []struct {
			Line int
			Msg  string
		}
	}
	json.Unmarshal(w.Body.Bytes(), &a)
	if len(a.Problems) != 1 || a.Problems[0].Line != 3 || !strings.Contains(a.Problems[0].Msg, "off by $-1") {
		t.Errorf("posted: %s", w.Body.String())
	}
}

func TestLedgerDynamicBlock(t *testing.T) {
	paths := ledgerOn(t, map[string]string{"money.org": ledgerOrg})
	res := UpdateDynBlocks(&common.DynBlockRequest{Filename: paths["money.org"], All: true})
	if !res.Ok {
		t.Fatalf("update: %+v", res)
	}
	want := "#+BEGIN: ledger :report balance :account Expenses :depth 1\n| Account  | Balance |\n|----------+---------|\n| Expenses | $850.00 |\n|----------+---------|\n| *Total*  | $850.00 |\n#+END:"
	if got := readOrg(t, paths["money.org"]); !strings.HasSuffix(got, want) {
		t.Errorf("block:\n%s", got)
	}
}

// With ledger not installed, a block's bal/reg is answered by orgs itself:
// the block alone, as ob-ledger runs it.
func TestLedgerBlockRunsWithoutLedger(t *testing.T) {
	paths := ledgerOn(t, map[string]string{"money.org": ledgerOrg})
	babelOn(t, map[string][]string{"ledger": {"orgs-no-such-ledger", "-f"}})
	res := ExecBabel(&common.BabelExec{Filename: paths["money.org"], Line: 2})
	if !res.Ok {
		t.Fatalf("exec: %+v", res)
	}
	text := readOrg(t, paths["money.org"])
	if !strings.Contains(text, "#+RESULTS: opening\n:            $1,000.00  Assets:Bank\n:           $-1,000.00  Equity:Opening") {
		t.Errorf("bal result:\n%s", text)
	}
	// The second block alone fails its assertion, as ledger would.
	at := strings.Count(text[:strings.Index(text, ":cmdline reg")], "\n")
	ExecBabel(&common.BabelExec{Filename: paths["money.org"], Line: at})
	text = readOrg(t, paths["money.org"])
	if !strings.Contains(text, "balance assertion failed") {
		t.Errorf("reg result:\n%s", text)
	}
}
