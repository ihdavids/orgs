package autoclockout

/* SDOC: Pollers

* Auto Clock Out

	The autoclockout plugin will automatically clock out of any active
	clock entry at a configured time of day. This is useful for ensuring
	you don't accidentally leave a clock running overnight.

	Configure it in your orgs.yaml under server.plugins:

	#+BEGIN_SRC yaml
  - name: "autoclockout"
    freq: 60
    clockouttimes:
      - "17:00"
      - "23:59"
	#+END_SRC

	The clockouttimes field accepts a list of times in HH:MM (24-hour) format.
	The plugin checks every freq seconds whether the current time has passed
	one of the configured times and clocks out if so.

EDOC */

import (
	"fmt"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

type AutoClockOut struct {
	Name          string
	ClockOutTimes []string `yaml:"clockouttimes"`
	// Track the last date+time we triggered for each slot to avoid repeat clock-outs
	lastTriggered map[string]string
	freq          int
	clock         ClockAccessor
}

// ClockAccessor abstracts access to the clock singleton so the plugin
// doesn't import internal/app/orgs directly.
type ClockAccessor interface {
	IsClockActive() bool
	ClockOut() (common.ResultMsg, error)
}

var clockAccessor ClockAccessor

// RegisterClockAccessor is called by the server startup to provide clock access.
func RegisterClockAccessor(c ClockAccessor) {
	clockAccessor = c
}

func (self *AutoClockOut) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *AutoClockOut) Update(db common.ODb) {
	if self.clock == nil {
		if clockAccessor != nil {
			self.clock = clockAccessor
		} else {
			return
		}
	}
	if !self.clock.IsClockActive() {
		return
	}
	now := time.Now()
	today := now.Format("2006-01-02")
	nowMins := now.Hour()*60 + now.Minute()

	for _, t := range self.ClockOutTimes {
		parsed, err := time.Parse("15:04", t)
		if err != nil {
			fmt.Printf("autoclockout: invalid time format %q, expected HH:MM\n", t)
			continue
		}
		targetMins := parsed.Hour()*60 + parsed.Minute()
		key := today + "/" + t

		if self.lastTriggered[key] != "" {
			continue
		}

		// Trigger if we are within the polling window past the target time
		diff := nowMins - targetMins
		if diff >= 0 && diff < (self.freq/60+1) {
			fmt.Printf("autoclockout: clocking out at configured time %s\n", t)
			reply, _ := self.clock.ClockOut()
			if reply.Ok {
				fmt.Printf("autoclockout: %s\n", reply.Msg)
			} else {
				fmt.Printf("autoclockout: clock out failed: %s\n", reply.Msg)
			}
			self.lastTriggered[key] = today
		}
	}
}

func (self *AutoClockOut) Startup(freq int, manager *common.PluginManager, opts *common.PluginOpts) {
	self.freq = freq
	self.lastTriggered = make(map[string]string)
}

// init function is called at boot
func init() {
	common.AddPoller("autoclockout", func() common.Poller {
		return &AutoClockOut{ClockOutTimes: []string{"17:00"}}
	})
}
