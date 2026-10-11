package notify

/* SDOC: Pollers

* Notify

	The notify plugin tells you about what is coming up: a native
	notification before each timed appointment on today's agenda, and once a
	day a digest of the deadlines that are due or overdue. It shows them on
	the machine the server runs on - Notification Center on macOS, the
	desktop's notification service on Linux, a toast on Windows - and
	publishes every one as a =notify= event, so =orgs notify listen= can show
	them on another machine when the server lives somewhere else.

	A click on a notification opens the heading in worg (on macOS this needs
	=terminal-notifier=: =brew install terminal-notifier=). Saved-query
	watches with =desktop= and the end of each focus (pomodoro) phase go out
	the same way, with the same quiet hours.

	#+BEGIN_SRC yaml
  - name: "notify"
    freq: 60                # seconds between looks at the agenda
    remind: [10, 2]         # minutes before a timed appointment; one reminder each
    deadlines: "08:30"      # the day's deadline digest; "" for none
    quiet: "22:00-07:00"    # no popups in these hours (events still go out)
    sound: true             # the system's notification sound
    focus: true             # announce the end of each focus / break phase
    desktop: true           # pop on this machine; false: only publish events
    open: "http://localhost:8010"  # where a click goes; the server itself by default
    icon: "/path/to/icon.png"      # optional
	#+END_SRC

	The older =notifybeforemins= and =beep= settings are still read: they
	mean a single =remind= and =sound=. Run =orgs notify test= to see a
	notification, and =orgs notify status= for what this server will do.

	Each reminder is sent once, and the server remembers which it has sent
	(=notify_data.json= beside the clock's state), so a restart does not
	repeat them. If a poll happens to cross two reminder times at once - a
	server started a minute before a meeting - only the later reminder is
	sent.

EDOC */

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/desktop"
)

type Notify struct {
	Name string
	// Minutes before a timed appointment, one reminder each. Empty: the old
	// NotifyBeforeMins alone.
	Remind []int `yaml:"remind"`
	// When the day's deadline digest goes out, "08:30"; empty for never.
	Deadlines string `yaml:"deadlines"`
	// Quiet hours, "22:00-07:00": no popups (critical ones aside).
	Quiet string `yaml:"quiet"`
	// The system's notification sound. Nil: the old Beep.
	Sound *bool `yaml:"sound"`
	// Announce the end of a focus phase. Nil means yes.
	Focus *bool `yaml:"focus"`
	// Pop on this machine. Nil means yes; false leaves it to listeners.
	Desktop *bool `yaml:"desktop"`
	// Worg's address for a click; the server's own when empty.
	Open string `yaml:"open"`
	Icon string `yaml:"icon"`

	// The settings this plugin had before it grew the ones above.
	Beep             bool `yaml:"beep"`
	NotifyBeforeMins int  `yaml:"notifybeforemins"`

	// -----------------------
	freq    int
	manager *common.PluginManager
	quiet   common.QuietHours
	now     func() time.Time // the clock; tests replace it

	mu    sync.Mutex
	state notifyState
	path  string // where state is kept; "" in tests
}

// What has been sent, kept across restarts so a reminder goes out once.
type notifyState struct {
	// Reminder key -> unix time after which it can be forgotten.
	Reminded map[string]int64 `json:"reminded"`
	// The day the deadline digest last went out, YYYY-MM-DD.
	Digest string `json:"digest"`
}

func (self *Notify) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

// --- settings ----------------------------------------------------------------

// The reminder times, largest first. Old configs had one number; new ones a
// list; with neither, 10 minutes.
func (self *Notify) offsets() []int {
	seen := map[int]bool{}
	var out []int
	add := func(m int) {
		if m >= 0 && !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	for _, m := range self.Remind {
		add(m)
	}
	if len(out) == 0 {
		add(self.NotifyBeforeMins)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}

func (self *Notify) sound() bool {
	if self.Sound != nil {
		return *self.Sound
	}
	return self.Beep
}

func (self *Notify) desktopOn() bool { return self.Desktop == nil || *self.Desktop }

// FocusNotes says whether the end of a focus phase is announced; the focus
// timer asks before saying anything.
func (self *Notify) FocusNotes() bool { return self.Focus == nil || *self.Focus }

// Where a click goes: the configured address, or this server.
func (self *Notify) WorgBase() string {
	if self.Open != "" {
		return self.Open
	}
	if self.manager != nil && self.manager.Port > 0 {
		return fmt.Sprintf("http://localhost:%d", self.manager.Port)
	}
	return ""
}

// The icon only when it is a file that exists: the old default "warning.png"
// was a relative name that never resolved.
func (self *Notify) icon() string {
	if self.Icon == "" {
		return ""
	}
	if _, err := os.Stat(self.Icon); err != nil {
		return ""
	}
	return self.Icon
}

func (self *Notify) clock() time.Time {
	if self.now != nil {
		return self.now()
	}
	return time.Now()
}

// NotifySettings is what GET /notify/status reports about this plugin.
func (self *Notify) NotifySettings() common.NotifyStatus {
	return common.NotifyStatus{
		Plugin: true, Desktop: self.desktopOn(), Quiet: self.Quiet,
		QuietNow: self.quiet.In(self.clock()), Remind: self.offsets(),
		Deadlines: self.Deadlines, Sound: self.sound(), Focus: self.FocusNotes(),
		Open: self.WorgBase(),
	}
}

// --- showing one ---------------------------------------------------------------

// Deliver pops a note on this machine, as this plugin's settings say: not at
// all with desktop off, not in quiet hours unless it is critical, and silent
// when the sound is off. Everything the server says goes through here (see
// deliverNote in the server), so the quiet hours hold for all of it.
func (self *Notify) Deliver(n desktop.Note) error {
	_, err := self.deliver(n)
	return err
}

// Popped says whether Deliver would put something on screen now; the test
// endpoint reports it.
func (self *Notify) Popped(n desktop.Note) (bool, error) { return self.deliver(n) }

func (self *Notify) deliver(n desktop.Note) (bool, error) {
	if !self.desktopOn() {
		return false, nil
	}
	if n.Urgency != desktop.Critical && self.quiet.In(self.clock()) {
		return false, nil
	}
	if !self.sound() {
		n.Sound = false
	}
	if n.Icon == "" {
		n.Icon = self.icon()
	}
	return true, desktop.Send(n)
}

// Alert pops one notification now. Kept for anything still calling it the
// old way; new code goes through the server's deliverNote.
func (self *Notify) Alert(title, body string) error {
	return self.Deliver(desktop.Note{Title: title, Body: body, Sound: true})
}

// Out through the server when there is one - published as an event as well
// as popped - and straight to the desktop in a plugin's own tests.
func (self *Notify) send(n desktop.Note, file, hash, headline string) {
	if self.manager != nil && self.manager.Deliver != nil {
		self.manager.Deliver(n, file, hash, headline)
		return
	}
	if err := self.Deliver(n); err != nil {
		fmt.Fprintf(os.Stderr, "notify: %v\n", err)
	}
}

// --- reminders --------------------------------------------------------------

// One appointment coming up.
type upcoming struct {
	Key      string // which occurrence, so it is announced once
	Headline string
	Mins     int // minutes from now until it starts
	Start    time.Time
	End      time.Time // zero when it has none
	File     string
	Line     int
	Hash     string
}

// The timed appointments of today's agenda starting within `window` minutes
// of now, plus the keys of those already started (to forget them). Only
// today's own timed occurrences count: a repeat lands on its real day, and
// an overdue item carried to today is not an appointment at its old time,
// which is what reading Todo.Date used to announce, every day.
func dueSoon(res common.AgendaResult, now time.Time, window int) (soon []upcoming, started []string) {
	for _, d := range res.Days {
		for _, e := range d.Entries {
			if !e.HasTime || e.Done || e.Late != 0 || e.Node < 0 || e.Node >= len(res.Nodes) {
				continue
			}
			at, err := time.ParseInLocation("2006-01-02T15:04", e.Start, now.Location())
			if err != nil {
				continue
			}
			n := res.Nodes[e.Node]
			key := fmt.Sprintf("%s:%d:%s:%s", n.Filename, n.LineNum, n.Headline, e.Start)
			// Rounded up, so an appointment 30 seconds away is in a window of 0.
			mins := int(at.Sub(now).Minutes() + 0.999)
			if at.Before(now) {
				started = append(started, key)
			} else if mins <= window {
				u := upcoming{Key: key, Headline: n.Headline, Mins: mins, Start: at,
					File: n.Filename, Line: n.LineNum, Hash: n.Hash}
				if end, err := time.ParseInLocation("2006-01-02T15:04", e.End, now.Location()); err == nil && end.After(at) {
					u.End = end
				}
				soon = append(soon, u)
			}
		}
	}
	return soon, started
}

// Which reminder an appointment this many minutes away is due, given the
// reminder times (largest first) and which of them already went out: the
// smallest time it has crossed, unless that one or a smaller one was sent.
// Crossing two at once (the server started late) sends only the later one.
func reminderDue(mins int, offsets []int, sent func(int) bool) (int, bool) {
	due := -1
	for _, o := range offsets {
		if mins <= o {
			due = o
		}
	}
	if due < 0 {
		return 0, false
	}
	for _, o := range offsets {
		if o <= due && sent(o) {
			return 0, false
		}
	}
	return due, true
}

// The words for a reminder: "in 10 minutes", "in 1 minute", "now".
func whenWords(mins int) string {
	switch {
	case mins <= 0:
		return "now"
	case mins == 1:
		return "in 1 minute"
	case mins < 60:
		return fmt.Sprintf("in %d minutes", mins)
	}
	h, m := mins/60, mins%60
	if m == 0 {
		return fmt.Sprintf("in %d h", h)
	}
	return fmt.Sprintf("in %d h %d min", h, m)
}

func reminderNote(u upcoming, sound bool, base string) desktop.Note {
	at := u.Start.Format("15:04")
	if !u.End.IsZero() {
		at += "–" + u.End.Format("15:04")
	}
	body := at
	if u.File != "" {
		body += " · " + strings.TrimSuffix(filepath.Base(u.File), filepath.Ext(u.File))
	}
	return desktop.Note{
		Title:    u.Headline,
		Subtitle: whenWords(u.Mins),
		Body:     body,
		URL:      common.HeadingLink(base, u.File, u.Line, u.Headline),
		Sound:    sound,
		// The reminders for one appointment replace each other.
		Group: "remind:" + u.Key,
	}
}

// --- the deadline digest --------------------------------------------------------

type dueItem struct {
	Headline string
	Late     int // days overdue; 0 when due today or coming
	DueIn    int // days until due, -1 when overdue or not warning
}

// The deadlines worth a morning's mention: overdue, due today, and inside
// their warning period, most urgent first.
func deadlinesDue(res common.AgendaResult) []dueItem {
	seen := map[int]bool{}
	var out []dueItem
	for _, d := range res.Days {
		for _, e := range d.Entries {
			if e.Kind != common.AgendaDeadline || e.Done || e.Node < 0 || e.Node >= len(res.Nodes) || seen[e.Node] {
				continue
			}
			if e.Late <= 0 && e.DueIn < 0 {
				continue
			}
			seen[e.Node] = true
			out = append(out, dueItem{Headline: res.Nodes[e.Node].Headline, Late: e.Late, DueIn: e.DueIn})
		}
	}
	rank := func(i dueItem) int {
		if i.Late > 0 {
			return -i.Late // the most overdue first
		}
		return i.DueIn
	}
	sort.SliceStable(out, func(a, b int) bool { return rank(out[a]) < rank(out[b]) })
	return out
}

func digestNote(items []dueItem, base string) desktop.Note {
	over, today, soon := 0, 0, 0
	for _, i := range items {
		switch {
		case i.Late > 0:
			over++
		case i.DueIn == 0:
			today++
		default:
			soon++
		}
	}
	var parts []string
	if over > 0 {
		parts = append(parts, fmt.Sprintf("%d overdue", over))
	}
	if today > 0 {
		parts = append(parts, fmt.Sprintf("%d due today", today))
	}
	if soon > 0 {
		parts = append(parts, fmt.Sprintf("%d coming up", soon))
	}
	var lines []string
	for i, it := range items {
		if i == 4 {
			lines = append(lines, fmt.Sprintf("and %d more", len(items)-4))
			break
		}
		lines = append(lines, it.Headline)
	}
	n := desktop.Note{
		Title: "Deadlines: " + strings.Join(parts, ", "),
		Body:  strings.Join(lines, "\n"),
		URL:   common.WorgLink(base, url.Values{"app": {"Agenda"}}),
		Group: "deadlines",
	}
	if over > 0 {
		n.Sound = true
	}
	return n
}

// --- the poll -------------------------------------------------------------------

func (self *Notify) Update(db common.ODb) {
	now := self.clock()
	offsets := self.offsets()
	window := 0
	if len(offsets) > 0 {
		window = offsets[0]
	}
	// A poll every freq seconds must not step over the window.
	if w := self.freq / 60; w > window {
		window = w
	}

	self.mu.Lock()
	if self.state.Reminded == nil {
		self.state.Reminded = map[string]int64{}
	}
	changed := self.forgetOld(now)
	self.mu.Unlock()

	res, err := db.Agenda(now, now, now, `!IsProject() && !IsArchived() && IsTodo()`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notify: %v\n", err)
		return
	}
	base := self.WorgBase()
	soon, _ := dueSoon(res, now, window)
	for i, u := range soon {
		self.mu.Lock()
		o, ok := reminderDue(u.Mins, offsets, func(o int) bool {
			_, done := self.state.Reminded[fmt.Sprintf("%s@%d", u.Key, o)]
			return done
		})
		if ok {
			// Every larger reminder counts as sent too: it is too late for it.
			keep := u.Start.Add(24 * time.Hour).Unix()
			for _, x := range offsets {
				if x >= o {
					self.state.Reminded[fmt.Sprintf("%s@%d", u.Key, x)] = keep
				}
			}
			changed = true
		}
		self.mu.Unlock()
		if !ok {
			continue
		}
		// The last reminder before it starts is the one that makes a sound.
		last := o == offsets[len(offsets)-1]
		self.send(reminderNote(soon[i], last, base), u.File, u.Hash, u.Headline)
	}

	if self.digestDue(now) {
		if items := deadlinesDue(res); len(items) > 0 {
			self.send(digestNote(items, base), "", "", "")
		}
		self.mu.Lock()
		self.state.Digest = now.Format("2006-01-02")
		changed = true
		self.mu.Unlock()
	}
	if changed {
		self.save()
	}
}

// Once a day, at or after the digest time, and not again that day.
func (self *Notify) digestDue(now time.Time) bool {
	if strings.TrimSpace(self.Deadlines) == "" {
		return false
	}
	at, err := common.ParseQuiet(self.Deadlines + "-" + self.Deadlines)
	if err != nil {
		return false
	}
	self.mu.Lock()
	sent := self.state.Digest == now.Format("2006-01-02")
	self.mu.Unlock()
	return !sent && now.Hour()*60+now.Minute() >= at.From
}

// Drop reminders whose appointments are a day gone. Called with mu held.
func (self *Notify) forgetOld(now time.Time) bool {
	changed := false
	for k, until := range self.state.Reminded {
		if now.Unix() > until {
			delete(self.state.Reminded, k)
			changed = true
		}
	}
	return changed
}

func (self *Notify) load() {
	if self.path == "" {
		return
	}
	if b, err := os.ReadFile(self.path); err == nil {
		json.Unmarshal(b, &self.state)
	}
}

func (self *Notify) save() {
	if self.path == "" {
		return
	}
	self.mu.Lock()
	b, err := json.MarshalIndent(self.state, "", " ")
	self.mu.Unlock()
	if err == nil {
		os.WriteFile(self.path, b, 0644)
	}
}

func (self *Notify) Startup(freq int, manager *common.PluginManager, opts *common.PluginOpts) {
	self.freq = freq
	self.manager = manager
	q, err := common.ParseQuiet(self.Quiet)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notify: %v - no quiet hours\n", err)
	}
	self.quiet = q
	if manager != nil && manager.HomeDir != "" {
		self.path = filepath.Join(manager.HomeDir, "notify_data.json")
	}
	self.load()
}

// init function is called at boot
func init() {
	common.AddPoller("notify", func() common.Poller {
		return &Notify{Beep: true, NotifyBeforeMins: 10}
	})
}
