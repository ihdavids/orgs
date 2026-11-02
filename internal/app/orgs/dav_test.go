package orgs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	goical "github.com/emersion/go-ical"
	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
	"github.com/emersion/go-webdav/carddav"
	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common"
)

// davServer is a server with CalDAV/CardDAV on, an app password for "ian",
// and the files given loaded.
func davServer(t *testing.T, files map[string]string, ro bool) (*httptest.Server, map[string]string, string) {
	t.Helper()
	if Conf().Server == nil {
		t.Skip("no server settings")
	}
	paths := loadOrg(t, files)
	dir := ""
	for _, p := range paths {
		dir = p[:strings.LastIndex(p, "/")]
	}
	old, oldDirs := Conf().Server.Dav, Conf().Server.OrgDirs
	Conf().Server.Dav = common.DavSettings{Enable: true, ReadOnly: ro}
	Conf().Server.OrgDirs = []string{dir}
	davStore = &davPasswordStore{path: dir + "/dav_passwords.json", Users: map[string][]davPassword{}}
	davStoreOnce.Do(func() {})
	davCalCache.events, davCardCache.contacts = nil, nil
	davThrottle.by = map[string]*davFails{}
	t.Cleanup(func() { Conf().Server.Dav, Conf().Server.OrgDirs = old, oldDirs })
	_, secret, err := davStore.add("ian", "test")
	if err != nil {
		t.Fatal(err)
	}
	r := mux.NewRouter()
	registerDavRoutes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, paths, secret
}

const davAgenda = `* TODO Dentist :health:
  SCHEDULED: <2026-11-03 Tue 10:00 +1y>
Bring the form.
* Meeting with Jo <2026-11-05 Thu 14:00-15:00>
* DONE Old thing
  SCHEDULED: <2026-10-01 Thu>
* Undated
`

func TestTheCalendarListsDatedHeadingsAndMovesThem(t *testing.T) {
	srv, paths, secret := davServer(t, map[string]string{"agenda.org": davAgenda}, false)
	ctx := context.Background()
	hc := webdav.HTTPClientWithBasicAuth(http.DefaultClient, "ian", secret)
	cl, err := caldav.NewClient(hc, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	home, err := cl.FindCalendarHomeSet(ctx, "/dav/cal/ian/")
	if err != nil {
		t.Fatalf("home set: %v", err)
	}
	cals, err := cl.FindCalendars(ctx, home)
	if err != nil || len(cals) != 1 {
		t.Fatalf("calendars: %v %+v", err, cals)
	}
	objs, err := cl.QueryCalendar(ctx, cals[0].Path, &caldav.CalendarQuery{CompFilter: caldav.CompFilter{Name: "VCALENDAR"}})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(objs) != 2 {
		t.Fatalf("want the dentist and the meeting, got %d", len(objs))
	}
	var dentist *caldav.CalendarObject
	for i := range objs {
		ev := objs[i].Data.Events()[0]
		s, _ := ev.Props.Text(goical.PropSummary)
		if s == "Dentist" {
			dentist = &objs[i]
		}
		if s == "Meeting with Jo" {
			start, _ := ev.DateTimeStart(time.Local)
			if start.In(time.Local).Format("15:04") != "14:00" {
				t.Errorf("meeting at %v", start)
			}
		}
	}
	if dentist == nil {
		t.Fatalf("no dentist")
	}
	// Move it to the 4th at 11:00 and rename it.
	cal := dentist.Data
	ev := cal.Events()[0]
	ev.Props.SetDateTime(goical.PropDateTimeStart, time.Date(2026, 11, 4, 11, 0, 0, 0, time.Local))
	ev.Props.SetDateTime(goical.PropDateTimeEnd, time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local))
	ev.Props.SetText(goical.PropSummary, "Dentist (check-up)")
	cal.Children[0] = ev.Component
	if _, err := cl.PutCalendarObject(ctx, dentist.Path, cal); err != nil {
		t.Fatalf("put: %v", err)
	}
	got := readOrg(t, paths["agenda.org"])
	if !strings.Contains(got, "* TODO Dentist (check-up)") || !strings.Contains(got, "SCHEDULED: <2026-11-04 Wed 11:00-12:00 +1y>") {
		t.Errorf("after the move:\n%s", got)
	}
	if !strings.Contains(got, ":ICAL_UID:") {
		t.Errorf("the uid was not pinned before the rename:\n%s", got)
	}
	if !strings.Contains(got, "Bring the form.") || !strings.Contains(got, "* DONE Old thing") {
		t.Errorf("something else changed:\n%s", got)
	}
	// The renamed heading is still the same resource.
	if _, err := cl.GetCalendarObject(ctx, dentist.Path); err != nil {
		t.Errorf("the event is not where it was after the rename: %v", err)
	}

	// Deleting takes the date off and leaves the heading.
	req, _ := http.NewRequest("DELETE", srv.URL+dentist.Path, nil)
	req.SetBasicAuth("ian", secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode >= 300 {
		t.Fatalf("delete: %v %v", err, resp)
	}
	got = readOrg(t, paths["agenda.org"])
	if strings.Contains(got, "SCHEDULED: <2026-11-04") || !strings.Contains(got, "* TODO Dentist (check-up)") {
		t.Errorf("after the delete:\n%s", got)
	}
}

func TestANewEventBecomesAHeadingInTheInbox(t *testing.T) {
	srv, paths, secret := davServer(t, map[string]string{"agenda.org": davAgenda, "inbox.org": "* Inbox\n"}, false)
	ctx := context.Background()
	cl, _ := caldav.NewClient(webdav.HTTPClientWithBasicAuth(http.DefaultClient, "ian", secret), srv.URL)
	cal := goical.NewCalendar()
	cal.Props.SetText(goical.PropVersion, "2.0")
	cal.Props.SetText(goical.PropProductID, "-//test//EN")
	ev := goical.NewEvent()
	ev.Props.SetText(goical.PropUID, "phone-uid-1")
	ev.Props.SetDateTime(goical.PropDateTimeStamp, time.Now().UTC())
	ev.Props.SetText(goical.PropSummary, "Lunch with Sam")
	ev.Props.SetDateTime(goical.PropDateTimeStart, time.Date(2026, 11, 6, 12, 30, 0, 0, time.Local))
	ev.Props.SetDateTime(goical.PropDateTimeEnd, time.Date(2026, 11, 6, 13, 30, 0, 0, time.Local))
	cal.Children = append(cal.Children, ev.Component)
	p := "/dav/cal/ian/calendars/org/phone-uid-1.ics"
	if _, err := cl.PutCalendarObject(ctx, p, cal); err != nil {
		t.Fatalf("put: %v", err)
	}
	got := readOrg(t, paths["inbox.org"])
	if !strings.Contains(got, "* Lunch with Sam") || !strings.Contains(got, "<2026-11-06 Fri 12:30-13:30>") ||
		!strings.Contains(got, ":DAV_NAME: phone-uid-1.ics") {
		t.Errorf("inbox:\n%s", got)
	}
	if _, err := cl.GetCalendarObject(ctx, p); err != nil {
		t.Errorf("the phone cannot find what it made: %v", err)
	}
}

func TestTheGateRefusesWithoutTheAppPassword(t *testing.T) {
	srv, _, secret := davServer(t, map[string]string{"agenda.org": davAgenda}, true)
	get := func(user, pass, path, method string) int {
		var body *strings.Reader
		if method == "PUT" {
			body = strings.NewReader("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")
		} else {
			body = strings.NewReader("")
		}
		req, _ := http.NewRequest(method, srv.URL+path, body)
		if user != "" {
			req.SetBasicAuth(user, pass)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if c := get("", "", "/dav/cal/ian/", "PROPFIND"); c != 401 {
		t.Errorf("no password: %d", c)
	}
	if c := get("ian", secret, "/dav/cal/someone/", "PROPFIND"); c != 403 {
		t.Errorf("another user's: %d", c)
	}
	if c := get("ian", secret, "/dav/cal/ian/calendars/org/x.ics", "PUT"); c != 403 {
		t.Errorf("a write when read only: %d", c)
	}
	if c := get("ian", secret, "/dav/cal/ian/", "PROPFIND"); c >= 400 {
		t.Errorf("the right password: %d", c)
	}
	for i := 0; i < 6; i++ {
		get("ian", "wrong-wrong", "/dav/cal/ian/", "PROPFIND")
	}
	if c := get("ian", secret, "/dav/cal/ian/", "PROPFIND"); c != 429 {
		t.Errorf("after six wrong passwords the address should wait: %d", c)
	}
	// Off: nothing there at all.
	Conf().Server.Dav.Enable = false
	davThrottle.by = map[string]*davFails{}
	if c := get("ian", secret, "/dav/cal/ian/", "PROPFIND"); c != 404 {
		t.Errorf("off: %d", c)
	}
}

const davPeople = `* Contacts
:PROPERTIES:
:COLLECTION: contact
:END:
** Jane Roe
:PROPERTIES:
:RECORD: contact
:EMAIL: jane@example.com
:PHONE_MOBILE: +1 555 0100
:BIRTHDAY: 1990-05-15
:COMPANY: Acme
:MET_AT: the conference
:END:
Likes tea.
* Plumber
:PROPERTIES:
:PHONE: 555 0199
:END:
`

func TestContactsGoBothWays(t *testing.T) {
	srv, paths, secret := davServer(t, map[string]string{"people.org": davPeople}, false)
	ctx := context.Background()
	cl, _ := carddav.NewClient(webdav.HTTPClientWithBasicAuth(http.DefaultClient, "ian", secret), srv.URL)
	home, err := cl.FindAddressBookHomeSet(ctx, "/dav/card/ian/")
	if err != nil {
		t.Fatal(err)
	}
	books, err := cl.FindAddressBooks(ctx, home)
	if err != nil || len(books) != 1 {
		t.Fatalf("books: %v %+v", err, books)
	}
	objs, err := cl.QueryAddressBook(ctx, books[0].Path, &carddav.AddressBookQuery{})
	if err != nil || len(objs) != 2 {
		t.Fatalf("contacts: %v %d", err, len(objs))
	}
	var jane *carddav.AddressObject
	for i := range objs {
		if objs[i].Card.PreferredValue(vcard.FieldFormattedName) == "Jane Roe" {
			jane = &objs[i]
		}
	}
	if jane == nil {
		t.Fatal("no Jane")
	}
	before := readOrg(t, paths["people.org"])
	// Saved unchanged: nothing changes.
	if _, err := cl.PutAddressObject(ctx, jane.Path, jane.Card); err != nil {
		t.Fatalf("put unchanged: %v", err)
	}
	if got := readOrg(t, paths["people.org"]); got != before {
		t.Errorf("an unchanged card changed the file:\n%s", got)
	}
	// A new number, the old one gone, a work email.
	card := jane.Card
	card[vcard.FieldTelephone] = []*vcard.Field{{Value: "+1 555 0123", Params: vcard.Params{vcard.ParamType: {"cell"}}}}
	card.Add(vcard.FieldEmail, &vcard.Field{Value: "jane@acme.example", Params: vcard.Params{vcard.ParamType: {"work"}}})
	if _, err := cl.PutAddressObject(ctx, jane.Path, card); err != nil {
		t.Fatalf("put: %v", err)
	}
	got := readOrg(t, paths["people.org"])
	for _, want := range []string{"+1 555 0123", "jane@acme.example", ":MET_AT:", "the conference", "Likes tea.", "from CardDAV"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "+1 555 0100") {
		t.Errorf("the old number is still there:\n%s", got)
	}
	// Deleting a contact is refused.
	req, _ := http.NewRequest("DELETE", srv.URL+jane.Path, nil)
	req.SetBasicAuth("ian", secret)
	if resp, _ := http.DefaultClient.Do(req); resp == nil || resp.StatusCode != 403 {
		t.Errorf("delete was not refused: %v", resp)
	}
}
