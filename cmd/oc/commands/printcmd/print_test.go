package printcmd

import (
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
)

func day(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestFileNames(t *testing.T) {
	wed := day(t, "2026-10-14")
	cases := []struct {
		view   string
		start  time.Weekday
		format string
		want   string
	}{
		{"week", time.Monday, "pdf", "week-2026-10-12.pdf"},
		{"week", time.Sunday, "html", "week-2026-10-11.html"},
		{"day", time.Monday, "pdf", "day-2026-10-14.pdf"},
		{"month", time.Monday, "pdf", "month-2026-10.pdf"},
	}
	for _, c := range cases {
		if got := defaultName(c.view, firstDay(c.view, wed, c.start), c.format); got != c.want {
			t.Errorf("%s/%v/%s: %s, want %s", c.view, c.start, c.format, got, c.want)
		}
	}
}

// The request carries what the flags said, and the page is written where -out
// says.
func TestExecWritesThePage(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agenda/print" {
			http.NotFound(w, r)
			return
		}
		got = map[string]string{}
		for k := range r.URL.Query() {
			got[k] = r.URL.Query().Get(k)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<!doctype html><p>planner</p>"))
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "plan.html")
	p := &Print{}
	fs := flag.NewFlagSet("print", flag.ContinueOnError)
	p.SetupParameters(fs)
	if err := fs.Parse([]string{"day", "-from", "2026-10-14", "-days", "3", "-theme", "minimal", "-paper", "letter", "-no-habits", "-out", out}); err != nil {
		t.Fatal(err)
	}
	p.Exec(commands.NewCore(srv.URL, nil))

	b, err := os.ReadFile(out)
	if err != nil || !strings.Contains(string(b), "planner") {
		t.Fatalf("page not written: %v %q", err, b)
	}
	want := map[string]string{"view": "day", "from": "2026-10-14", "pages": "3", "theme": "minimal", "paper": "letter", "habits": "f"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s=%q, want %q (all: %v)", k, got[k], v, got)
		}
	}
}
