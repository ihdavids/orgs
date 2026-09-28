package orgs

// Telling a client that something changed.
//
// The server has watched the org files since the beginning and has never had a
// way to say so. Every client polls or, more often, does not: `orgs agenda`
// draws what was true when it ran, the terminal has no way to show a clock
// ticking, and worg reloads when somebody presses something. The websocket API
// that would have carried this was commented out years ago.
//
// This is the smallest thing that fixes it: server-sent events on `GET /events`.
// One long-lived GET, text/event-stream, one JSON object per event. No protocol
// to implement on the client side - `curl -N` reads it, and so does any browser
// with three lines of EventSource.
//
// Four things about it are deliberate:
//
//  1. **A subscriber that is not reading is dropped, not waited for.** Each one
//     has a small buffer and a send that gives up rather than blocking: a
//     laptop that went to sleep with an `orgs watch` running must not be able
//     to stop the file watcher for everybody else.
//  2. **An event says what happened, never what the thing now is.** "notes.org
//     reloaded" rather than the file's contents: the client asks for what it
//     wants, which keeps the stream small and means no client is limited to the
//     fields somebody thought to put in the event.
//  3. **A heartbeat every twenty seconds**, as an SSE comment. Proxies and load
//     balancers close a connection that has said nothing for a minute, and the
//     reader cannot tell that from a quiet database.
//  4. **Reloads are coalesced.** Saving a file in an editor is often two or
//     three filesystem events, and a client redrawing three times looks like a
//     flicker rather than an update.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Event is one thing that happened. Kind is what to switch on; the rest is
// whatever that kind carries.
type Event struct {
	Kind string `json:"kind"`
	// The org file this is about, when it is about one.
	File string `json:"file,omitempty"`
	// The heading this is about, when it is about one.
	Hash     string `json:"hash,omitempty"`
	Headline string `json:"headline,omitempty"`
	// Free text for a person watching the stream.
	Msg string `json:"msg,omitempty"`
	// The database's reload counter as of this event, so a client can tell
	// whether anything it cached is still good.
	Index uint64 `json:"index"`
	At    string `json:"at"`
}

// The kinds an event can be. A client switches on these, so they are a promise:
// add to them rather than changing one.
const (
	// An org file was (re)read: anything derived from it may have moved.
	EventReload = "reload"
	// A clock was started or stopped.
	EventClockIn  = "clockin"
	EventClockOut = "clockout"
	// The stream opened. Sent once, so a client knows it is connected and what
	// the reload counter was when it arrived.
	EventHello = "hello"
)

type eventHub struct {
	mu   sync.Mutex
	subs map[chan Event]bool
	// The last reload announced, so a burst of filesystem events for one save
	// becomes one message.
	pending map[string]bool
	timer   *time.Timer
}

var hub = &eventHub{subs: map[chan Event]bool{}, pending: map[string]bool{}}

// Publish hands an event to everybody listening. It never blocks: a subscriber
// whose buffer is full is behind, and the choice is between dropping its event
// and holding up the file watcher.
func Publish(e Event) {
	if e.At == "" {
		e.At = time.Now().Format(time.RFC3339)
	}
	if e.Index == 0 {
		e.Index = GetDb().ReloadIndex
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	for ch := range hub.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// PublishReload announces that a file was re-read, coalescing a burst of them.
//
// An editor saving a file produces a write and often a rename and a chmod
// besides, and the watcher reloads on each. Three identical messages a
// millisecond apart make a client redraw three times, which reads as a flicker
// rather than as an update.
func PublishReload(filename string) {
	hub.mu.Lock()
	hub.pending[filename] = true
	if hub.timer == nil {
		hub.timer = time.AfterFunc(120*time.Millisecond, flushReloads)
	} else {
		hub.timer.Reset(120 * time.Millisecond)
	}
	hub.mu.Unlock()
}

func flushReloads() {
	hub.mu.Lock()
	files := make([]string, 0, len(hub.pending))
	for f := range hub.pending {
		files = append(files, f)
	}
	hub.pending = map[string]bool{}
	hub.timer = nil
	hub.mu.Unlock()

	for _, f := range files {
		Publish(Event{Kind: EventReload, File: f})
	}
}

func subscribe() chan Event {
	// Buffered, so a subscriber that is briefly busy does not miss anything and
	// a subscriber that has gone away is noticed rather than waited for.
	ch := make(chan Event, 32)
	hub.mu.Lock()
	hub.subs[ch] = true
	hub.mu.Unlock()
	return ch
}

func unsubscribe(ch chan Event) {
	hub.mu.Lock()
	delete(hub.subs, ch)
	hub.mu.Unlock()
	close(ch)
}

// Subscribers is how many clients are listening, for /doctor to report.
func Subscribers() int {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	return len(hub.subs)
}

/* SDOC: API
* GET /events — Watch for Changes
	A long-lived connection that says when something changed. One JSON object per
	event, in =text/event-stream= format (server-sent events), so a browser reads
	it with =EventSource= and a terminal reads it with =curl -N=.

	This is what makes a live view possible at all: the server has always watched
	the org files and until now had no way to tell anybody.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                             |
	|-----------+--------+----------+---------------------------------------------------------|
	| =kinds=   | string | no       | Comma separated event kinds to send; everything by default. |

	*Event kinds:*
	| Kind       | Meaning                                              |
	|------------+------------------------------------------------------|
	| =hello=    | Sent once when the stream opens.                     |
	| =reload=   | An org file was re-read; anything from it may have moved. |
	| =clockin=  | A clock was started on a heading.                    |
	| =clockout= | The running clock was stopped.                       |

	*Response:* =text/event-stream=. Each event is a =data:= line holding one
	JSON object, followed by a blank line. A =:= comment line arrives every
	twenty seconds as a heartbeat, which keeps proxies from closing an idle
	stream.

	#+BEGIN_SRC
	data: {"kind":"hello","index":41,"at":"2026-09-27T23:00:00-07:00"}

	data: {"kind":"reload","file":"/home/me/org/todo.org","index":42,"at":"2026-09-27T23:01:12-07:00"}
	#+END_SRC
EDOC */
func RequestEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "this server cannot stream", http.StatusInternalServerError)
		return
	}

	want := map[string]bool{}
	if kinds := r.URL.Query().Get("kinds"); kinds != "" {
		for _, k := range splitComma(kinds) {
			want[k] = true
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Nothing in this stream should be buffered by a proxy in front of it.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ch := subscribe()
	defer unsubscribe(ch)

	send := func(e Event) bool {
		if len(want) > 0 && !want[e.Kind] {
			return true
		}
		b, err := json.Marshal(e)
		if err != nil {
			return true
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	// Say hello at once, so a client knows it is connected rather than waiting
	// for the first thing to happen - which on a quiet database is never.
	send(Event{Kind: EventHello, At: time.Now().Format(time.RFC3339),
		Index: GetDb().ReloadIndex, Msg: "watching"})

	beat := time.NewTicker(20 * time.Second)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			if !send(e) {
				return
			}
		case <-beat.C:
			// An SSE comment. It keeps the connection alive through anything in
			// the middle and is ignored by every reader.
			if _, err := fmt.Fprint(w, ": beat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func splitComma(s string) []string {
	out := []string{}
	cur := ""
	for _, r := range s {
		if r == ',' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
