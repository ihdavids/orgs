package common

import (
	"strings"
	"testing"
)

type sickPlugin struct{}

func (sickPlugin) PluginHealth() string { return "no token" }

// A panic in Startup or Update is caught and written down, not the end of the
// server; a plugin that reports on itself is asked.
func TestPluginHealth(t *testing.T) {
	StartPlugin("poller", "t-broken", nil, func() { panic("bad config") })
	StartPlugin("poller", "t-sick", sickPlugin{}, func() {})
	UpdatePlugin("t-sick", func() {})
	UpdatePlugin("t-sick", func() { panic("lost the network") })
	got := map[string]PluginState{}
	for _, s := range PluginStates() {
		got[s.Name] = s
	}
	if b := got["t-broken"]; b.Started || !strings.Contains(b.StartErr, "bad config") {
		t.Errorf("broken = %+v", b)
	}
	s := got["t-sick"]
	if !s.Started || s.Updates != 2 || s.LastUpdate.IsZero() || !strings.Contains(s.LastErr, "lost the network") || s.Health != "no token" {
		t.Errorf("sick = %+v", s)
	}
}
