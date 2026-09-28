package common

// Reading the server's event stream.
//
// The other half of `GET /events`. Deliberately small - server-sent events is a
// line protocol and pulling in a library for it would be more code than this,
// not less. What it does do is the part that is easy to get wrong: reconnecting.
//
// A watch is meant to be left running for a day. In that day the server will be
// restarted, the laptop will sleep, and the wifi will drop. A reader that ends
// on the first of those is a reader nobody trusts enough to leave running, so
// this one reconnects with a backoff and says so rather than exiting.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OrgEvent is one thing that happened, as /events describes it.
type OrgEvent struct {
	Kind     string `json:"kind"`
	File     string `json:"file,omitempty"`
	Hash     string `json:"hash,omitempty"`
	Headline string `json:"headline,omitempty"`
	Msg      string `json:"msg,omitempty"`
	Index    uint64 `json:"index"`
	At       string `json:"at"`
}

// EventOpts is what to watch and how loudly to complain about losing it.
type EventOpts struct {
	// Only these kinds, or everything when empty.
	Kinds []string
	// Called when the connection drops and again when it comes back, so a
	// caller can say so on screen. Optional.
	OnDisconnect func(err error)
	OnReconnect  func()
	// Give up rather than reconnecting. For a one-shot wait.
	Once bool
}

// Events calls onEvent for every event until the context is cancelled or
// onEvent answers false.
//
// It reconnects on its own. The backoff is a second growing to thirty, because
// the two things being waited for - a server being restarted, and a laptop
// waking up - are seconds and hours respectively, and hammering a server that
// is down helps neither.
func Events(ctx context.Context, rest *Rest, opts EventOpts, onEvent func(OrgEvent) bool) error {
	backoff := time.Second
	for {
		err := readStream(ctx, rest, opts, onEvent)
		if ctx.Err() != nil {
			return nil
		}
		if err == errStop {
			return nil
		}
		if opts.Once {
			return err
		}
		if opts.OnDisconnect != nil {
			opts.OnDisconnect(err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// errStop is how onEvent says it has had enough. Not an error to report.
var errStop = fmt.Errorf("stopped")

func readStream(ctx context.Context, rest *Rest, opts EventOpts, onEvent func(OrgEvent) bool) error {
	u := rest.Url + "/events"
	if len(opts.Kinds) > 0 {
		u += "?" + url.Values{"kinds": {strings.Join(opts.Kinds, ",")}}.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	req.Header = rest.Header.Clone()
	req.Header.Set("Accept", "text/event-stream")

	// No timeout on the client: the whole point is a connection that stays open
	// with nothing on it. The heartbeat and the context are what end it.
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the server answered %s", resp.Status)
	}
	if opts.OnReconnect != nil {
		opts.OnReconnect()
	}

	sc := bufio.NewScanner(resp.Body)
	// An event is one line of json and a heading can be long; the 64k default
	// is the same trap the file search hit.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		// A line starting with a colon is a comment - the heartbeat - and a
		// blank line ends an event. Neither carries anything to act on.
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var e OrgEvent
		if err := json.Unmarshal([]byte(data), &e); err != nil {
			continue
		}
		if !onEvent(e) {
			return errStop
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return fmt.Errorf("the stream ended")
}
