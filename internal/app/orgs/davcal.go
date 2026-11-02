package orgs

// The calendar a phone sees (dav.go says who may see it).
//
// One calendar, "Org agenda": an event per date a heading has - its SCHEDULED,
// its DEADLINE ("Due: ..."), its own active timestamps - as the ics exporter
// writes them (plugs/ics), so the phone and `orgs export -f ics` agree. The
// headings are `dav.calendarQuery`, by default every dated heading that is not
// archived or done.
//
// Each event is its own resource, named after its uid (the exporter's:
// :ICAL_UID:, :ID:, :CUSTOM_ID:, else a hash of file and outline path), with
// the date kind after it; a heading made from the phone remembers the name the
// phone gave it (:DAV_NAME:), so the phone finds it where it put it.
//
// From the phone:
//
//   - **Moving an event** changes that date on the heading, as a line edit
//     (changeDateLines, what /date/change does), keeping the stamp's repeater
//     and warning. A stamp written in the headline is changed there.
//   - **Renaming it** renames the heading (the stamp in a headline stays).
//     A heading whose uid came from its title is given :ICAL_UID: first, so a
//     rename does not turn into a delete and a new event on the phone.
//   - **Deleting it** takes the date off the heading. The heading stays.
//   - **A new event** becomes a heading in `dav.newEvents` (inbox.org in the
//     first org directory) with the time as its timestamp.
//   - **Moving one occurrence of a repeating event** is refused: org has no
//     exceptions to a repeater; move the series or the heading.
//
// Every write is checked against the ETag the phone had (If-Match), so a phone
// that has not seen an edit made elsewhere cannot overwrite it.

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	goical "github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/ics"
	"github.com/ihdavids/orgs/internal/common"
	oical "github.com/ihdavids/orgs/internal/common/ical"
)

// davEvent is one calendar resource.
type davEvent struct {
	Name  string // the resource's file name, "x.ics"
	File  string
	Hash  string
	Row   int
	Lvl   int
	Kind  string // scheduled, deadline, ts
	Index int    // which own timestamp, for ts
	Uid   string // the heading's uid, without the kind
	Text  string
	ETag  string
	Mod   time.Time
	ev    oical.Event
	title string
	// The heading's uid was made from its title: a rename would change it.
	madeUid bool
}

var davNameRe = regexp.MustCompile(`[^A-Za-z0-9._@-]+`)

func davObjectName(s string) string {
	return davNameRe.ReplaceAllString(s, "_")
}

func davCalendarQuery() string {
	q := strings.TrimSpace(Conf().Server.Dav.CalendarQuery)
	if q != "" {
		return q
	}
	if Conf().Server.Dav.Done {
		return "HasAnyDate() && !IsArchived()"
	}
	return "HasAnyDate() && !IsArchived() && !IsDone()"
}

var davCalCache struct {
	mu     sync.Mutex
	index  uint64
	events []davEvent
}

// davEvents is every event, worked out again only when a file has changed.
func davEvents(fresh bool) ([]davEvent, error) {
	db := GetDb()
	davCalCache.mu.Lock()
	defer davCalCache.mu.Unlock()
	if !fresh && davCalCache.events != nil && davCalCache.index == db.ReloadIndex {
		return davCalCache.events, nil
	}
	idx := db.ReloadIndex
	secs, err := plugs.Headings(&Db{}, davCalendarQuery())
	if err != nil {
		return nil, err
	}
	exp := &ics.Exporter{ScheduledAsEvents: true, DeadlinesAsEvents: true, Done: Conf().Server.Dav.Done}
	no := false
	exp.Todos = &no
	out := []davEvent{}
	seen := map[string]bool{}
	for _, sec := range secs {
		h := sec.Headline
		if h == nil {
			continue
		}
		f := db.FileFromSection(sec)
		if f == nil {
			continue
		}
		mod := time.Time{}
		if st, err := os.Stat(f.Filename); err == nil {
			mod = st.ModTime().UTC().Truncate(time.Second)
		}
		w := exp.Calendar([]*org.Section{sec}, mod)
		davName := davProp(h, "DAV_NAME")
		hasUid := davProp(h, "ICAL_UID") != "" || davProp(h, "ID") != "" || davProp(h, "CUSTOM_ID") != ""
		for _, e := range w.Events {
			ev := davEvent{File: f.Filename, Hash: sec.Hash, Row: h.Pos.Row, Lvl: h.Lvl, Mod: mod, ev: e, title: e.Summary, madeUid: !hasUid}
			switch {
			case strings.HasSuffix(e.UID, "-scheduled"):
				ev.Kind, ev.Uid = "scheduled", strings.TrimSuffix(e.UID, "-scheduled")
			case strings.HasSuffix(e.UID, "-deadline"):
				ev.Kind, ev.Uid = "deadline", strings.TrimSuffix(e.UID, "-deadline")
				ev.title = strings.TrimPrefix(e.Summary, "Due: ")
			default:
				ev.Kind = "ts"
				if i := strings.LastIndex(e.UID, "-"); i > 0 {
					ev.Uid = e.UID[:i]
					fmt.Sscan(e.UID[i+1:], &ev.Index)
				}
			}
			ev.Name = davObjectName(e.UID) + ".ics"
			if davName != "" && ev.Kind == "ts" && ev.Index == 0 {
				ev.Name = davName
			}
			if seen[ev.Name] {
				continue
			}
			seen[ev.Name] = true
			one := &oical.Writer{Now: mod, Events: []oical.Event{e}}
			ev.Text = one.String()
			sum := sha1.Sum([]byte(ev.Text))
			ev.ETag = hex.EncodeToString(sum[:10])
			out = append(out, ev)
		}
	}
	davCalCache.events, davCalCache.index = out, idx
	return out, nil
}

func davProp(h *org.Headline, k string) string {
	if h == nil || h.Properties == nil {
		return ""
	}
	v, _ := h.Properties.Get(k)
	return strings.TrimSpace(v)
}

// ---------------------------------------------------------------------------
// The backend
// ---------------------------------------------------------------------------

type calBackend struct{}

func (b *calBackend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	return "/dav/cal/" + url.PathEscape(davUser(ctx)) + "/", nil
}

func (b *calBackend) CalendarHomeSetPath(ctx context.Context) (string, error) {
	return "/dav/cal/" + url.PathEscape(davUser(ctx)) + "/calendars/", nil
}

func (b *calBackend) calendarPath(ctx context.Context) string {
	return "/dav/cal/" + url.PathEscape(davUser(ctx)) + "/calendars/org/"
}

func (b *calBackend) calendar(ctx context.Context) caldav.Calendar {
	return caldav.Calendar{Path: b.calendarPath(ctx), Name: "Org agenda",
		Description: "Dated headings from orgs", SupportedComponentSet: []string{"VEVENT"}, MaxResourceSize: 1 << 20}
}

func (b *calBackend) CreateCalendar(ctx context.Context, c *caldav.Calendar) error {
	return webdav.NewHTTPError(http.StatusForbidden, fmt.Errorf("orgs has one calendar"))
}

func (b *calBackend) ListCalendars(ctx context.Context) ([]caldav.Calendar, error) {
	return []caldav.Calendar{b.calendar(ctx)}, nil
}

func (b *calBackend) GetCalendar(ctx context.Context, p string) (*caldav.Calendar, error) {
	if strings.TrimSuffix(p, "/")+"/" != b.calendarPath(ctx) {
		return nil, webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("no such calendar"))
	}
	c := b.calendar(ctx)
	return &c, nil
}

func (b *calBackend) object(ctx context.Context, e davEvent) (caldav.CalendarObject, error) {
	cal, err := goical.NewDecoder(strings.NewReader(e.Text)).Decode()
	if err != nil {
		return caldav.CalendarObject{}, err
	}
	return caldav.CalendarObject{Path: b.calendarPath(ctx) + url.PathEscape(e.Name), ModTime: e.Mod,
		ContentLength: int64(len(e.Text)), ETag: e.ETag, Data: cal}, nil
}

func (b *calBackend) find(ctx context.Context, p string, fresh bool) (*davEvent, error) {
	dir, name := path.Split(p)
	if dir != b.calendarPath(ctx) {
		return nil, webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("no such calendar"))
	}
	if n, err := url.PathUnescape(name); err == nil {
		name = n
	}
	evs, err := davEvents(fresh)
	if err != nil {
		return nil, err
	}
	for i := range evs {
		if evs[i].Name == name {
			return &evs[i], nil
		}
	}
	return nil, nil
}

func (b *calBackend) GetCalendarObject(ctx context.Context, p string, req *caldav.CalendarCompRequest) (*caldav.CalendarObject, error) {
	e, err := b.find(ctx, p, false)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("no such event"))
	}
	o, err := b.object(ctx, *e)
	return &o, err
}

func (b *calBackend) ListCalendarObjects(ctx context.Context, p string, req *caldav.CalendarCompRequest) ([]caldav.CalendarObject, error) {
	if _, err := b.GetCalendar(ctx, p); err != nil {
		return nil, err
	}
	evs, err := davEvents(false)
	if err != nil {
		return nil, err
	}
	out := make([]caldav.CalendarObject, 0, len(evs))
	for _, e := range evs {
		o, err := b.object(ctx, e)
		if err != nil {
			continue
		}
		out = append(out, o)
	}
	return out, nil
}

func (b *calBackend) QueryCalendarObjects(ctx context.Context, p string, q *caldav.CalendarQuery) ([]caldav.CalendarObject, error) {
	all, err := b.ListCalendarObjects(ctx, p, &q.CompRequest)
	if err != nil {
		return nil, err
	}
	return caldav.Filter(q, all)
}

func davPrecondition(have *davEvent, etag string, ifMatch, ifNoneMatch webdav.ConditionalMatch) error {
	if ifNoneMatch.IsSet() && have != nil {
		return webdav.NewHTTPError(http.StatusPreconditionFailed, fmt.Errorf("that already exists"))
	}
	if ifMatch.IsSet() {
		if have == nil {
			return webdav.NewHTTPError(http.StatusPreconditionFailed, fmt.Errorf("that is not there any more"))
		}
		if !ifMatch.IsWildcard() {
			want, err := ifMatch.ETag()
			if err != nil || want != etag {
				return webdav.NewHTTPError(http.StatusPreconditionFailed, fmt.Errorf("it was changed since you read it"))
			}
		}
	}
	return nil
}

// master is the event's own VEVENT (not an override of one occurrence), and
// whether the calendar also moves single occurrences.
func masterEvent(cal *goical.Calendar) (*goical.Event, bool) {
	var m *goical.Event
	overrides := false
	for _, e := range cal.Events() {
		e := e
		if e.Props.Get(goical.PropRecurrenceID) != nil {
			overrides = true
			continue
		}
		if m == nil {
			m = &e
		}
	}
	return m, overrides
}

// eventTimes reads DTSTART and DTEND: all-day events' DTEND is the day after
// the last one, so it comes back as the last day itself.
func eventTimes(e *goical.Event) (time.Time, time.Time, bool, error) {
	sp := e.Props.Get(goical.PropDateTimeStart)
	if sp == nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("an event with no DTSTART")
	}
	allDay := sp.ValueType() == goical.ValueDate
	start, err := e.DateTimeStart(time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}
	end, err := e.DateTimeEnd(time.Local)
	if err != nil {
		end = time.Time{}
	}
	start = start.In(time.Local)
	if !end.IsZero() {
		end = end.In(time.Local)
	}
	if allDay && !end.IsZero() {
		end = end.AddDate(0, 0, -1)
		if !end.After(start) {
			end = time.Time{}
		}
	}
	return start, end, allDay, nil
}

var cookieRe = regexp.MustCompile(`\s((?:\.\+|\+\+|\+|-{1,2})\d+[hdwmy](?:/\d+[hdwmy])?)`)

// stampCookies is what follows the date and time in a stamp: repeaters and
// warnings, kept across a move.
func stampCookies(stamp string) string {
	first := stamp
	if i := strings.Index(stamp, "--<"); i > 0 {
		first = stamp[:i]
	}
	out := []string{}
	for _, m := range cookieRe.FindAllStringSubmatch(first, -1) {
		out = append(out, m[1])
	}
	return strings.Join(out, " ")
}

var headStampRe = regexp.MustCompile(`<\d{4}-\d{2}-\d{2}[^>]*>(?:--<\d{4}-\d{2}-\d{2}[^>]*>)?`)

// currentStamp is the stamp as written that an event came from.
func currentStamp(lines []string, e *davEvent) (string, int) {
	if e.Row >= len(lines) {
		return "", -1
	}
	switch e.Kind {
	case "scheduled", "deadline":
		re := planningKeyRe(strings.ToUpper(e.Kind))
		for i := e.Row + 1; i < len(lines) && i <= e.Row+2; i++ {
			if m := re.FindString(lines[i]); m != "" {
				return strings.TrimSpace(m[strings.Index(m, ":")+1:]), i
			}
		}
	default:
		if e.Index == 0 {
			if m := headStampRe.FindString(lines[e.Row]); m != "" {
				return m, e.Row
			}
		}
		n := 0
		end := subtreeRows(lines, e.Row)
		for i := e.Row; i <= end && i < len(lines); i++ {
			if i > e.Row && txLevel(lines[i]) > 0 {
				break
			}
			if txPlanningRe.MatchString(lines[i]) || strings.HasPrefix(strings.TrimSpace(lines[i]), ":") {
				continue
			}
			for _, m := range headStampRe.FindAllString(lines[i], -1) {
				if n == e.Index {
					return m, i
				}
				n++
			}
		}
	}
	return "", -1
}

// setEventDate writes stamp (or, empty, removes the date) for one event.
func setEventDate(lines []string, e *davEvent, stamp string) ([]string, error) {
	switch e.Kind {
	case "scheduled":
		return changeDateLines(lines, e.Row, e.Lvl, "SCHEDULED", stamp)
	case "deadline":
		return changeDateLines(lines, e.Row, e.Lvl, "DEADLINE", stamp)
	}
	old, at := currentStamp(lines, e)
	if at < 0 {
		return nil, fmt.Errorf("could not find that timestamp in %s", filepath.Base(e.File))
	}
	if at == e.Row || e.Index > 0 {
		l := strings.Replace(lines[at], old, stamp, 1)
		if stamp == "" {
			l = strings.TrimRight(strings.Replace(l, "  ", " ", 1), " \t")
			if strings.TrimSpace(l) == "" && at != e.Row {
				return append(lines[:at:at], lines[at+1:]...), nil
			}
		}
		lines[at] = l
		return lines, nil
	}
	return changeDateLines(lines, e.Row, e.Lvl, "TIMESTAMP", stamp)
}

func (b *calBackend) PutCalendarObject(ctx context.Context, p string, cal *goical.Calendar, opts *caldav.PutCalendarObjectOptions) (*caldav.CalendarObject, error) {
	have, err := b.find(ctx, p, true)
	if err != nil {
		return nil, err
	}
	etag := ""
	if have != nil {
		etag = have.ETag
	}
	if err := davPrecondition(have, etag, opts.IfMatch, opts.IfNoneMatch); err != nil {
		return nil, err
	}
	m, overrides := masterEvent(cal)
	if m == nil {
		return nil, webdav.NewHTTPError(http.StatusBadRequest, fmt.Errorf("no event in that calendar"))
	}
	start, end, allDay, err := eventTimes(m)
	if err != nil {
		return nil, webdav.NewHTTPError(http.StatusBadRequest, err)
	}
	summary := ""
	if s := m.Props.Get(goical.PropSummary); s != nil {
		summary = strings.TrimSpace(s.Value)
	}
	if have == nil {
		_, name := path.Split(p)
		if n, err := url.PathUnescape(name); err == nil {
			name = n
		}
		uid := ""
		if u := m.Props.Get(goical.PropUID); u != nil {
			uid = u.Value
		}
		desc := ""
		if d := m.Props.Get(goical.PropDescription); d != nil {
			desc = d.Value
		}
		if err := davNewEvent(name, uid, summary, desc, oical.OrgRange(start, end, allDay, "")); err != nil {
			return nil, err
		}
	} else {
		lines, err := readFileLines(have.File)
		if err != nil {
			return nil, err
		}
		if have.Row >= len(lines) || txLevel(lines[have.Row]) == 0 {
			return nil, webdav.NewHTTPError(http.StatusConflict, fmt.Errorf("the heading moved; sync again"))
		}
		before := strings.Join(lines, "\n")
		old, _ := currentStamp(lines, have)
		moved := !start.Equal(have.ev.Start) || allDay != have.ev.AllDay ||
			(!allDay && !end.IsZero() && !end.Equal(have.ev.End)) ||
			(allDay && !end.IsZero() && !end.Equal(have.ev.End) && !(have.ev.End.IsZero() && sameDate(end, start)))
		if overrides && !moved {
			return nil, webdav.NewHTTPError(http.StatusForbidden,
				fmt.Errorf("org repeats a heading as a whole: move the series, or the heading"))
		}
		renamed := summary != "" && summary != have.title
		if renamed && have.madeUid {
			// Pin the uid before the title it was made from changes.
			if sec := GetDb().FindByHash(have.Hash); sec != nil {
				lines = setPropLines(lines, have.Row, have.Lvl, "ICAL_UID", have.Uid)
			}
		}
		if moved {
			stamp := oical.OrgRange(start, end, allDay, stampCookies(old))
			if lines, err = setEventDate(lines, have, stamp); err != nil {
				return nil, err
			}
		}
		if renamed {
			l := lines[have.Row]
			keep := headStampRe.FindString(l)
			title := summary
			if keep != "" {
				title += " " + keep
			}
			lines[have.Row] = replaceHeadlineTitle(l, title)
		}
		if strings.Join(lines, "\n") != before {
			if err := writeLines(have.File, trimTrailing(lines)); err != nil {
				return nil, err
			}
		}
	}
	got, err := b.find(ctx, p, true)
	if err != nil || got == nil {
		// Filed, but under another name (a query that does not take it).
		return &caldav.CalendarObject{Path: p, ModTime: time.Now()}, nil
	}
	o, err := b.object(ctx, *got)
	return &o, err
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// readFileLines keeps a final empty element for a file ending in a newline;
// writeLines adds the newline back.
func trimTrailing(lines []string) []string {
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// setPropLines sets one property of the heading on row, as a line edit.
func setPropLines(lines []string, row, lvl int, key, val string) []string {
	out, err := setPropsIn(lines, row, subtreeRows(lines, row), indentOf(lvl), map[string]string{key: val})
	if err != nil {
		return lines
	}
	return out
}

func (b *calBackend) DeleteCalendarObject(ctx context.Context, p string) error {
	have, err := b.find(ctx, p, true)
	if err != nil {
		return err
	}
	if have == nil {
		return webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("no such event"))
	}
	lines, err := readFileLines(have.File)
	if err != nil {
		return err
	}
	out, err := setEventDate(lines, have, "")
	if err != nil {
		return err
	}
	return writeLines(have.File, trimTrailing(out))
}

// davNewEvent files an event made on the phone as a heading.
func davNewEvent(name, uid, summary, desc, stamp string) error {
	if summary == "" {
		summary = "(untitled)"
	}
	file := Conf().Server.Dav.NewEvents
	if file == "" {
		file = "inbox.org"
	}
	if len(Conf().Server.OrgDirs) == 0 {
		return fmt.Errorf("no org directory to file it in")
	}
	if !filepath.IsAbs(file) {
		root, _ := filepath.Abs(Conf().Server.OrgDirs[0])
		file = filepath.Join(root, file)
	}
	file = filepath.Clean(file)
	if _, ok := insideOrgDirs(file); !ok || !strings.HasSuffix(file, ".org") {
		return webdav.NewHTTPError(http.StatusForbidden, fmt.Errorf("dav.newEvents must be an org file in the org directories"))
	}
	lines := []string{}
	if b, err := os.ReadFile(file); err == nil {
		lines = trimTrailing(strings.Split(string(b), "\n"))
	}
	add := []string{"* " + strings.ReplaceAll(summary, "\n", " "), ":PROPERTIES:"}
	if uid != "" {
		add = append(add, ":ICAL_UID: "+uid)
	}
	add = append(add, ":DAV_NAME: "+name, ":ADDED: "+orgStamp(time.Now()), ":END:", stamp)
	for _, l := range strings.Split(strings.TrimSpace(desc), "\n") {
		if strings.TrimSpace(l) != "" {
			add = append(add, l)
		}
	}
	lines = append(lines, add...)
	var buf bytes.Buffer
	buf.WriteString(strings.Join(lines, "\n") + "\n")
	if err := os.WriteFile(file, buf.Bytes(), 0644); err != nil {
		return err
	}
	if GetDb().FindByFile(file) == nil {
		GetDb().LoadFile(file)
	} else {
		GetDb().ReloadFile(file)
	}
	return nil
}

var _ caldav.Backend = (*calBackend)(nil)
var _ common.ODb = (*Db)(nil)
