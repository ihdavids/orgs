package notify
/* SDOC: Pollers

* Notify

	The notify plugin will display a message box on Windows or Mac
	as a scheduled appointment approached. This is a polling orgs
	module meaning it will run periodically and your polling interval
	determines the smallest granularity at which you can notify.

	Here we are polling every 60 seconds but only notifying when
	the appointment is less than 5 minutes away.

	The popup will have a unicorn org mode icon and the popup
	will play an annoying beep sound to catch your attention.


	#+BEGIN_SRC yaml
  - name: "notify"
    freq: 60
    beep: true
    notifybeforemins: 5
    icon: "c:/path/orgs/unicorn.png"
	#+END_SRC

EDOC */

import (
	"fmt"
	"time"

	"github.com/gen2brain/beeep"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/common"
)

// Platform specific notifications
// Notify reasonably close to an event.
// This is a poller plugin in that it will
// periodically check for things today
// that we need to notify about.

type Notify struct {
	Name             string
	Beep             bool
	NotifyBeforeMins int
	Icon             string
	// -----------------------
	haveNotified []common.Todo
	freq         int
}

func (self *Notify) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func contains(s []common.Todo, t common.Todo) bool {
	for _, v := range s {
		if v.Is(t) {
			return true
		}
	}
	return false
}

func max(a int, b int) int {
	if a < b {
		return b
	} else {
		return a
	}
}

func (self *Notify) Update(db plugs.ODb) {
	//fmt.Printf("Notify Update...%v\n", time.Now())

	curDate := time.Now()
	query := fmt.Sprintf(`!IsProject() && !IsArchived() && IsTodo() && OnDate("%s")`, curDate.Format("2006 02 01"))
	notWindow := max(self.NotifyBeforeMins, int(self.freq/60.0))

	if reply, err := db.QueryTodosExpr(query); err == nil {
		for _, v := range reply {
			minDiff := v.Date.Start.Minute() - curDate.Minute()

			if v.Date.Start.Hour() == curDate.Hour() && minDiff >= 0 && minDiff <= notWindow && !contains(self.haveNotified, v) {
				fmt.Printf("Less than %d minutes till: %s\n", minDiff, v.Headline)
				if self.Beep {
					beeep.Beep(beeep.DefaultFreq, beeep.DefaultDuration)
				}
				beeep.Alert(v.Headline, fmt.Sprintf("Less than %d minutes till!", minDiff), self.Icon)
				self.haveNotified = append(self.haveNotified, v)
			} else if minDiff < 0 && contains(self.haveNotified, v) {
				for i, t := range self.haveNotified {
					if v.Is(t) {
						self.haveNotified = append(self.haveNotified[:i], self.haveNotified[i+1:]...)
					}
				}
			}
		}
	}
}

func (self *Notify) Startup(freq int, manager *plugs.PluginManager, opts *plugs.PluginOpts) {
	self.freq = freq
}

// init function is called at boot
func init() {
	plugs.AddPoller("notify", func() plugs.Poller {
		return &Notify{Beep: true, NotifyBeforeMins: 10, Icon: "warning.png"}
	})
}
