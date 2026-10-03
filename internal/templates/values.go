package templates

// The names a template answers by itself: the clock, a fresh id, the machine.
//
// There is one list and two kinds of template reading it. A capture template
// ({{now}} in a capture's `template:`) gets these as the defaults written in on
// the way out of /capture/templates - internal/common/captemplate.go asks
// AutoValue. A file template (newfile.tpl, the day page, an exporter's .tpl)
// gets them in its pongo2 context from standardContext. Before they shared this
// the two disagreed: {{weekday}} was Monday in one and Mon in the other, and
// the file template's {{date}} was written year-day-month.

/* SDOC: Editing
* Template values

  Every template can use these names without being given them: a capture
  template (where they become the editable default of the placeholder) and
  a file template - =newFileTemplate=, the day page, an exporter's =.tpl=.

  | Name                         | Becomes                                   |
  |------------------------------+-------------------------------------------|
  | ={{date}}=                   | =2026-10-03=                              |
  | ={{time}}=                   | =09:07=                                   |
  | ={{datetime}}=               | =2026-10-03 09:07=                        |
  | ={{today}}= / ={{active}}=   | =<2026-10-03 Sat>=                        |
  | ={{now}}= / ={{inactive}}=   | =[2026-10-03 Sat 09:07]=                  |
  | ={{tomorrow}}=               | =<2026-10-04 Sun>=                        |
  | ={{yesterday}}=              | =<2026-10-02 Fri>=                        |
  | ={{week}}=                   | =2026-W40= (ISO week)                     |
  | ={{week_start}}=             | =<2026-09-28 Mon>=, Monday of the week    |
  | ={{week_end}}=               | =<2026-10-04 Sun>=, Sunday of the week    |
  | ={{month}}=                  | =2026-10=                                 |
  | ={{year}}=                   | =2026=                                    |
  | ={{day}}=                    | =03=                                      |
  | ={{weekday}}=                | =Saturday=                                |
  | ={{dayname}}=                | =Sat=                                     |
  | ={{epoch}}= / ={{unix}}=     | =1791018420=                              |
  | ={{iso}}= / ={{rfc3339}}=    | =2026-10-03T09:07:00Z=                    |
  | ={{uuid}}= / ={{guid}}=      | a fresh v4 uuid                           |
  | ={{shortuid}}=               | =js3xiyiy=, 8 characters a-z and 2-7      |
  | ={{hostname}}=               | the machine holding the org files         |
  | ={{username}}= / ={{user}}=  | the login: whoever asked, for a capture;  |
  |                              | the account running the server otherwise  |

  A capture template also has ={{daypage}}=, the current day page relative
  to the first org directory.

  A file template also has:

  | Name             | Becomes                                                |
  |------------------+--------------------------------------------------------|
  | ={{author}}=     | =author:= from the config, else the account's name     |
  | ={{email}}=      | =email:= from the config                               |
  | ={{orgdir}}=     | the first org directory                                |
  | ={{filename}}=   | the file being made (new files and day pages)          |
  | ={{basename}}=   | the same, without directory or =.org=                  |
  | ={{env.HOME}}=   | an environment variable                                |
  | ={{ when(..) }}= | any date, read the way =orgs sched= reads one          |

  =when= takes what you would type at =orgs sched= - =tomorrow=, =+2w=,
  =fri=, =fri 14:00=, =2026-12-25= - and writes an org timestamp. A second
  argument names any of the clock values above to write it as that instead:

  #+BEGIN_SRC org
    SCHEDULED: {{ when("fri 16:00") }}       ->  <2026-10-09 Fri 16:00>
    Review by {{ when("+1w", "date") }}      ->  2026-10-10
    #+TITLE: Week of {{week_start}} - {{week_end}}
  #+END_SRC

  A date =when= cannot read is written into the file as the complaint, so it
  shows where it went wrong. Values come through unescaped, so an org
  timestamp keeps its =<>=; an html template putting one on a page wants
  ={{ today|escape|safe }}= (=escape= alone escapes twice).
EDOC */

import (
	"crypto/rand"
	"encoding/base32"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const orgDay = "2006-01-02 Mon"

// AutoNames is every name AutoValue answers. Keep it in step with the switch
// below; TestAutoNamesAllAnswer fails on a name here with no case there.
var AutoNames = []string{
	"date", "time", "datetime",
	"today", "now", "tomorrow", "yesterday",
	"week", "week_start", "week_end", "month", "year", "day", "weekday", "dayname",
	"epoch", "iso",
	"uuid", "shortuid", "hostname",
}

// Aliases are the other spellings of a name. Every one of these would answer
// identically anyway - they are all read off the same clock - except uuid and
// guid, which would otherwise be two numbers out of the hat in one template.
var Aliases = map[string]string{
	"guid":      "uuid",
	"user":      "username",
	"active":    "today",
	"inactive":  "now",
	"timestamp": "now",
	"unix":      "epoch",
	"rfc3339":   "iso",
}

// CanonName is the name a value is known by: trimmed, case folded, alias
// followed. {{ NOW }} and {{now}} are the same name.
func CanonName(name string) string {
	k := strings.ToLower(strings.TrimSpace(name))
	if c, ok := Aliases[k]; ok {
		return c
	}
	return k
}

// AutoValue is the value of one name at a moment, and whether it has one. The
// name is expected canonical (CanonName). False means "not one of these", so a
// capture leaves it for somebody to answer.
func AutoValue(name string, now time.Time) (string, bool) {
	switch name {

	// ── The clock ────────────────────────────────────────────────────────
	case "date":
		return now.Format("2006-01-02"), true
	case "time":
		return now.Format("15:04"), true
	case "datetime":
		return now.Format("2006-01-02 15:04"), true
	// The two org timestamps, named the way org names them: active is the one
	// that shows up on the agenda, inactive is the one that does not.
	case "today":
		return "<" + now.Format(orgDay) + ">", true
	case "now":
		return "[" + now.Format(orgDay+" 15:04") + "]", true
	// Active, because a date written on a capture is nearly always something to
	// be reminded of: {{tomorrow}} is what a SCHEDULED line wants.
	case "tomorrow":
		return "<" + now.AddDate(0, 0, 1).Format(orgDay) + ">", true
	case "yesterday":
		return "<" + now.AddDate(0, 0, -1).Format(orgDay) + ">", true
	case "week":
		y, w := now.ISOWeek()
		return strconv.Itoa(y) + "-W" + pad2(w), true
	// The Monday and Sunday of the ISO week, for a weekly page's title:
	// "Week of {{week_start}} - {{week_end}}".
	case "week_start":
		return "<" + weekStart(now).Format(orgDay) + ">", true
	case "week_end":
		return "<" + weekStart(now).AddDate(0, 0, 6).Format(orgDay) + ">", true
	case "month":
		return now.Format("2006-01"), true
	case "year":
		return now.Format("2006"), true
	case "day":
		return now.Format("02"), true
	case "weekday":
		return now.Format("Monday"), true
	case "dayname":
		return now.Format("Mon"), true
	// For anything that is going to be read by a program rather than a person:
	// a sort key, a filename, a field some other tool parses.
	case "epoch":
		return strconv.FormatInt(now.Unix(), 10), true
	case "iso":
		return now.Format(time.RFC3339), true

	// ── Ids and the machine ──────────────────────────────────────────────
	// A fresh v4 uuid. A capture wants the server to answer this: in a browser
	// crypto.randomUUID is secure-context only, so worg served over plain http
	// from another machine cannot.
	case "uuid":
		return uuid.New().String(), true
	// Eight characters out of the hat: enough to keep two temporary files from
	// colliding without a 36 character name. Lowercase base32 (a-z, 2-7), so it
	// is safe in a filename and on a filesystem that ignores case.
	case "shortuid":
		var b [5]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", false
		}
		return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])), true
	case "hostname":
		h, err := os.Hostname()
		if err != nil || h == "" {
			return "", false
		}
		return h, true
	}
	return "", false
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// weekStart is the Monday of t's ISO week, at midnight.
func weekStart(t time.Time) time.Time {
	back := (int(t.Weekday()) + 6) % 7 // Monday 0 ... Sunday 6
	y, m, d := t.AddDate(0, 0, -back).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
