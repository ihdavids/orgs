//go:build linux

package desktop

import (
	"fmt"
	"os/exec"
	"sync"

	"github.com/godbus/dbus/v5"
)

// The org.freedesktop.Notifications service directly, when there is a session
// bus: it is what notify-send itself talks to, and talking to it directly is
// what makes a click open the heading (the server says which action was
// invoked, on a signal notify-send cannot wait for without blocking) and lets
// a group replace its last notification in place.
type dbusNotifier struct {
	mu      sync.Mutex
	conn    *dbus.Conn
	actions bool              // the server draws actions at all
	byGroup map[string]uint32 // the last id shown per group, to replace
	urls    map[uint32]string // what a click on each id opens
}

const (
	notifyDest  = "org.freedesktop.Notifications"
	notifyPath  = "/org/freedesktop/Notifications"
	notifyIface = "org.freedesktop.Notifications"
)

func (d *dbusNotifier) Name() string { return "Linux (D-Bus notifications)" }

func (d *dbusNotifier) connect() error {
	if d.conn != nil && d.conn.Connected() {
		return nil
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	d.conn = conn
	d.byGroup = map[string]uint32{}
	d.urls = map[uint32]string{}
	var caps []string
	if err := conn.Object(notifyDest, notifyPath).Call(notifyIface+".GetCapabilities", 0).Store(&caps); err == nil {
		for _, c := range caps {
			if c == "actions" {
				d.actions = true
			}
		}
	}
	// The click, and the close that ends the need to remember the url.
	conn.AddMatchSignal(dbus.WithMatchInterface(notifyIface), dbus.WithMatchMember("ActionInvoked"))
	conn.AddMatchSignal(dbus.WithMatchInterface(notifyIface), dbus.WithMatchMember("NotificationClosed"))
	ch := make(chan *dbus.Signal, 16)
	conn.Signal(ch)
	go d.listen(ch)
	return nil
}

func (d *dbusNotifier) listen(ch chan *dbus.Signal) {
	for s := range ch {
		if len(s.Body) == 0 {
			continue
		}
		id, ok := s.Body[0].(uint32)
		if !ok {
			continue
		}
		d.mu.Lock()
		url := d.urls[id]
		if s.Name == notifyIface+".NotificationClosed" {
			delete(d.urls, id)
		}
		d.mu.Unlock()
		if s.Name == notifyIface+".ActionInvoked" && url != "" {
			// xdg-open hands it to the person's own browser.
			exec.Command("xdg-open", url).Start()
		}
	}
}

func (d *dbusNotifier) Send(n Note) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.connect(); err != nil {
		return err
	}
	var replaces uint32
	if n.Group != "" {
		replaces = d.byGroup[n.Group]
	}
	var actions []string
	if n.URL != "" && d.actions {
		// "default" is the click on the notification itself.
		actions = []string{"default", "Open", "open", "Open"}
	}
	hints := map[string]dbus.Variant{
		"urgency": dbus.MakeVariant(linuxUrgencyByte(n.Urgency)),
	}
	if n.Sound {
		hints["sound-name"] = dbus.MakeVariant("message-new-instant")
	}
	timeout := int32(-1) // the server's own
	if n.Urgency == Critical {
		timeout = 0 // until dismissed
	}
	var id uint32
	err := d.conn.Object(notifyDest, notifyPath).Call(notifyIface+".Notify", 0,
		"orgs", replaces, n.Icon,
		clip(n.Title, maxTitle), markupEscape(clip(bodyWithSubtitle(n), maxBody)),
		actions, hints, timeout).Store(&id)
	if err != nil {
		d.conn = nil
		return fmt.Errorf("D-Bus notify: %w", err)
	}
	if n.Group != "" {
		d.byGroup[n.Group] = id
	}
	if n.URL != "" {
		d.urls[id] = n.URL
	}
	return nil
}

func linuxUrgencyByte(u Urgency) byte {
	switch u {
	case Low:
		return 0
	case Critical:
		return 2
	}
	return 1
}

// The D-Bus service when there is a session bus with a notification server on
// it, notify-send otherwise. Checked once, by asking the server who it is.
func Detect() Notifier {
	if conn, err := dbus.ConnectSessionBus(); err == nil {
		var name, vendor, version, spec string
		err := conn.Object(notifyDest, notifyPath).Call(notifyIface+".GetServerInformation", 0).Store(&name, &vendor, &version, &spec)
		if err == nil {
			conn.Close()
			return &dbusNotifier{}
		}
		conn.Close()
	}
	if have("notify-send") {
		return notifySendNotifier{}
	}
	return noNotifier{why: "no notification server on the session bus and no notify-send"}
}
