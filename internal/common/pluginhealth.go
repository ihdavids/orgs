package common

// What each server plugin has been doing, for /server/status and orgs doctor.
//
// Plugins report nothing back: Startup and Update return no error, and a
// plugin that cannot reach its service logs a line to stderr and carries on.
// What the server can see is a panic, and before this a panic in Update took
// the whole server down from inside the plugin's goroutine. Both are now
// recovered and written down here, with the time of the plugin's last good
// update, so a plugin that has gone quiet is visible without reading logs.
//
// A plugin that knows more can say so by implementing PluginHealthReporter.

import (
	"fmt"
	"os"
	"runtime/debug"
	"sort"
	"sync"
	"time"
)

// PluginHealthReporter is optional: a plugin that can tell it is not working
// (no token, the service refusing it) answers why, or "" when it is fine.
type PluginHealthReporter interface {
	PluginHealth() string
}

type PluginState struct {
	Name string
	// poller, exporter or updater.
	Kind    string
	Started bool
	// Why Startup failed (a recovered panic), "" when it did not.
	StartErr string `json:",omitempty"`
	Updates  int    `json:",omitempty"`
	// The last Update that finished, zero for a plugin that has not run one.
	LastUpdate time.Time `json:",omitempty"`
	// The last Update that panicked, and why.
	LastErr   string    `json:",omitempty"`
	LastErrAt time.Time `json:",omitempty"`
	// What the plugin says about itself (PluginHealthReporter).
	Health string `json:",omitempty"`
	plugin interface{}
}

var pluginStates = struct {
	sync.Mutex
	m map[string]*PluginState
}{m: map[string]*PluginState{}}

func pluginState(kind, name string) *PluginState {
	key := kind + ":" + name
	s, ok := pluginStates.m[key]
	if !ok {
		s = &PluginState{Name: name, Kind: kind}
		pluginStates.m[key] = s
	}
	return s
}

// StartPlugin runs a plugin's Startup, recovering a panic and noting it, so
// one broken plugin in the yaml does not stop the server starting.
func StartPlugin(kind, name string, plugin interface{}, startup func()) {
	err := recovered(startup)
	pluginStates.Lock()
	defer pluginStates.Unlock()
	s := pluginState(kind, name)
	s.plugin = plugin
	s.Started = err == nil
	s.StartErr = ""
	if err != nil {
		s.StartErr = err.Error()
	}
}

// UpdatePlugin runs one Update, recovering a panic and noting it.
func UpdatePlugin(name string, update func()) {
	err := recovered(update)
	pluginStates.Lock()
	defer pluginStates.Unlock()
	s := pluginState("poller", name)
	s.Updates++
	if err != nil {
		s.LastErr, s.LastErrAt = err.Error(), time.Now()
	} else {
		s.LastUpdate = time.Now()
	}
}

// PluginStates is every plugin started, by kind and name.
func PluginStates() []PluginState {
	pluginStates.Lock()
	defer pluginStates.Unlock()
	out := make([]PluginState, 0, len(pluginStates.m))
	for _, s := range pluginStates.m {
		c := *s
		if r, ok := s.plugin.(PluginHealthReporter); ok && s.Started {
			c.Health = r.PluginHealth()
		}
		c.plugin = nil
		out = append(out, c)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Kind != out[b].Kind {
			return out[a].Kind < out[b].Kind
		}
		return out[a].Name < out[b].Name
	})
	return out
}

func recovered(f func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
			fmt.Fprintf(os.Stderr, "PLUGIN PANIC: %v\n%s\n", r, debug.Stack())
		}
	}()
	f()
	return nil
}
