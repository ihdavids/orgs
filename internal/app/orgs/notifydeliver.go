package orgs

// One way for the server to tell somebody something.
//
// Reminders, the deadline digest, a watched query finding something and a
// focus phase ending all come here. Each is published as a `notify` event -
// so `orgs notify listen` can show it on the machine a person is sitting at,
// whichever machine the server is on - and handed to the notify plugin, which
// pops it on this machine unless its settings say not to (desktop off, quiet
// hours). With no notify plugin configured nothing pops here, but the events
// still go out: whether a desktop is interrupted is the yaml's decision.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/desktop"
)

// A notification ready to show (EventNotify), on /events.
const EventNotify = "notify"

// The notify plugin, as the server sees it.
type noteDeliverer interface {
	Deliver(n desktop.Note) error
}

// What the plugin answers beyond showing things: its settings, whether a
// note would pop now, where a click goes, whether focus phases are wanted.
type noteSettings interface {
	NotifySettings() common.NotifyStatus
	Popped(n desktop.Note) (bool, error)
	WorgBase() string
	FocusNotes() bool
}

func notifyPlugins() []noteDeliverer {
	var out []noteDeliverer
	if Conf().Server == nil {
		return out
	}
	for _, pd := range Conf().Server.Plugins {
		if d, ok := pd.Plugin.(noteDeliverer); ok {
			out = append(out, d)
		}
	}
	return out
}

func notifySettingsPlugin() noteSettings {
	for _, d := range notifyPlugins() {
		if s, ok := d.(noteSettings); ok {
			return s
		}
	}
	return nil
}

// Where a click on a notification goes: the plugin's `open`, else this server.
func notifyWorgBase() string {
	if s := notifySettingsPlugin(); s != nil {
		if b := s.WorgBase(); b != "" {
			return b
		}
	}
	if Conf().Server != nil && Conf().Server.Port > 0 {
		return fmt.Sprintf("http://localhost:%d", Conf().Server.Port)
	}
	return ""
}

// A note's text as one body, the subtitle leading, for the event: a listener
// on another platform may have no subtitle of its own.
func noteBody(n desktop.Note) string {
	switch {
	case n.Subtitle == "":
		return n.Body
	case n.Body == "":
		return n.Subtitle
	}
	return n.Subtitle + "\n" + n.Body
}

// deliverNote says one thing: as an event always, and on this machine's
// desktop when a notify plugin is there to (off the caller's goroutine: a
// notifier program can take a moment, and a reminder loop or a query checker
// must not wait on it).
func deliverNote(n desktop.Note, file, hash, headline string) {
	Publish(Event{Kind: EventNotify, Title: n.Title, Msg: noteBody(n), URL: n.URL, Group: n.Group,
		Urgency: common.UrgencyName(n.Urgency), Sound: n.Sound, File: file, Hash: hash, Headline: headline})
	for _, d := range notifyPlugins() {
		go func(d noteDeliverer) {
			if err := d.Deliver(n); err != nil {
				fmt.Fprintf(os.Stderr, "notify: %v\n", err)
			}
		}(d)
	}
}

// focusNote announces a focus phase ending, when the notify plugin wants
// them (focus: true, the default).
func focusNote(kind, title, file string, line int) {
	s := notifySettingsPlugin()
	if s == nil || !s.FocusNotes() {
		return
	}
	var n desktop.Note
	switch kind {
	case EventFocusEnd:
		n = desktop.Note{Title: "Time for a break", Subtitle: title, Body: "Focus session done.", Sound: true}
	case EventBreakEnd:
		n = desktop.Note{Title: "Break's over", Subtitle: title, Body: "Ready for the next one?", Sound: true}
	default:
		return
	}
	n.Group = "focus"
	n.URL = common.WorgLink(notifyWorgBase(), url.Values{"app": {"Focus"}})
	deliverNote(n, file, "", title)
}

/* SDOC: API
* GET /notify/status — How This Server Notifies

	What a notification from this server does: whether a notify plugin is
	configured, how this machine shows a notification (=Backend=, e.g.
	=macOS (terminal-notifier)=), whether it pops here (=Desktop=), the quiet
	hours and whether they are on now, the reminder times, the deadline digest
	time, sound, focus announcements, and where a click goes (=Open=).

	*Method:* =GET=

	*Response:* A =NotifyStatus= JSON object.
EDOC */
func RequestNotifyStatus(w http.ResponseWriter, r *http.Request) {
	st := common.NotifyStatus{Ok: true}
	if s := notifySettingsPlugin(); s != nil {
		st = s.NotifySettings()
		st.Ok = true
	} else {
		st.Msg = "no notify plugin in server.plugins: nothing pops here, but notify events still go out"
		st.Open = notifyWorgBase()
	}
	st.Backend = desktop.Default().Name()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(st)
}

/* SDOC: API
* POST /notify/test — Send a Test Notification

	Sends one notification through the same path reminders take: published as a
	=notify= event, and popped on the server's machine when the notify plugin is
	configured to (not in its quiet hours, desktop on).

	*Method:* =POST=

	*Request Body (JSON):* optional
	| Field   | Type   | Description                         |
	|---------+--------+-------------------------------------|
	| =Title= | string | Defaults to "orgs".                 |
	| =Body=  | string | Defaults to a line saying it works. |

	*Response:* A =NotifyTestResult=: =Popped= says whether it was shown on the
	server's machine, =Backend= how, =Msg= why not or what went wrong.
EDOC */
func PostNotifyTest(w http.ResponseWriter, r *http.Request) {
	var req struct{ Title, Body string }
	if body, _ := io.ReadAll(r.Body); len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			answerErr(w, err)
			return
		}
	}
	if req.Title == "" {
		req.Title = "orgs"
	}
	if req.Body == "" {
		req.Body = "Notifications are working."
	}
	n := desktop.Note{Title: req.Title, Body: req.Body, Sound: true, Group: "test",
		URL: common.WorgLink(notifyWorgBase(), nil)}
	res := common.NotifyTestResult{Ok: true, Backend: desktop.Default().Name()}

	Publish(Event{Kind: EventNotify, Title: n.Title, Msg: noteBody(n), URL: n.URL, Group: n.Group,
		Urgency: common.UrgencyName(n.Urgency), Sound: n.Sound})
	if s := notifySettingsPlugin(); s == nil {
		res.Msg = "published as a notify event; no notify plugin, so nothing pops on the server's machine"
	} else {
		popped, err := s.Popped(n)
		res.Popped = popped
		switch {
		case err != nil:
			res.Ok, res.Msg = false, err.Error()
		case !popped && s.NotifySettings().QuietNow:
			res.Msg = "quiet hours: published, not popped"
		case !popped:
			res.Msg = "desktop is off for the notify plugin: published, not popped"
		default:
			res.Msg = "shown on the server's machine and published"
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func registerNotifyRoutes(api *mux.Router) {
	api.HandleFunc("/notify/status", RequestNotifyStatus).Methods("GET")
	api.HandleFunc("/notify/test", PostNotifyTest).Methods("POST")
}
