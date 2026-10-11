package orgs

/* SDOC: Editing
* Clocking

  TODO: Fill in information on how to use the orgs clocking features
EDOC */

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

type OrgsClock struct {
	Time   *org.OrgDate
	Target *common.Target
}

// Clocking in and out are published on /events from here, not from the REST
// handlers: the auto clock-out plugin and the implicit clock-out a clock-in
// does both come through these, and when only the handlers published, nothing
// said the clock had stopped - a status line had to poll /clock to find out.
func (self *OrgsClock) ClockIn(tgt *common.Target) (common.ResultMsg, error) {
	self.ClockOut()
	asked := tgt.Id
	if err := GetDb().ConvertTargetToOlp(tgt); err != nil {
		return common.ResultMsg{Ok: false, Msg: fmt.Sprintf("Could not clock in, could not convert target: %s", err.Error())}, err
	}
	self.Time = org.NewOrgDateNow()
	self.Time.HaveTime = true
	self.Target = tgt
	self.WriteOutClock()
	Publish(Event{Kind: EventClockIn, Hash: asked, Msg: "Clock is now active"})
	return common.ResultMsg{Ok: true, Msg: "Clock is now active"}, nil
}

func (self *OrgsClock) IsClockActive() bool {
	return self.Target != nil
}

func (self *OrgsClock) GetTarget() *common.Target {
	return self.Target
}

func (self *OrgsClock) GetTime() *org.OrgDate {
	return self.Time
}

func (self *OrgsClock) ClockOut() (common.ResultMsg, error) {
	if self.IsClockActive() {
		var msg common.ResultMsg
		if ofile, sec := GetDb().GetFromTarget(self.Target, false); sec != nil && ofile != nil {
			if err := writeClockLine(ofile, sec, self.Time.Start, time.Now()); err != nil {
				msg = common.ResultMsg{Ok: false, Msg: "Clocked out, but the clock line was not written: " + err.Error()}
			}
		}
		// Clean out the clock
		self.Time = nil
		self.Target = nil
		self.WriteOutClock()
		Publish(Event{Kind: EventClockOut, Msg: "Clocked out okay"})
		if msg.Msg != "" {
			return msg, nil
		}
		return common.ResultMsg{Ok: true, Msg: "Clocked out okay"}, nil
	}
	return common.ResultMsg{Ok: false, Msg: "Clock was not active"}, nil
}

// The finished clock as one line, spliced in where org would put it: at the top
// of the heading's clock drawer (newest first), or in a new drawer after the
// planning line and the property drawer. A line edit, like every other write
// here.
//
// It used to be a document edit - the clock appended to the parsed heading and
// the whole file written back - which went wrong three ways at once: the
// drawer was put in front of the heading's planning, so a DEADLINE under it
// stopped being read as one; the clock was added both to the drawer and to the
// heading, so it was written twice; and every other line of the file was
// re-indented on the way.
func writeClockLine(f *common.OrgFile, sec *org.Section, start, end time.Time) error {
	if f == nil || sec == nil || sec.Headline == nil {
		return fmt.Errorf("no heading to clock into")
	}
	lines, from, to, ok := recordLines(f.Filename, sec)
	if !ok {
		return fmt.Errorf("could not read %s", f.Filename)
	}
	ind := ownBodyIndent(lines, from, to, sec.Headline.Lvl)
	lines = logInto(lines, from, to, ind, Conf().ClockIntoDrawer, []string{clockLine(ind, start, end)})
	return writeLines(f.Filename, lines)
}

// The indent the heading's own body uses - its first line before any child
// heading - so a clock under a column-zero heading is written at column zero,
// and under an indented one indented. Two past the stars when it has none.
func ownBodyIndent(lines []string, from, to, lvl int) string {
	for i := from + 1; i <= to && i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, "*") {
			break
		}
		return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	}
	return indentOf(lvl)
}

// A CLOCK line as org writes it, duration right-aligned in five:
// `CLOCK: [2026-10-10 Sat 09:00]--[2026-10-10 Sat 10:30] =>  1:30`.
func clockLine(ind string, start, end time.Time) string {
	if end.Before(start) {
		end = start
	}
	mins := int(end.Sub(start).Minutes())
	dur := fmt.Sprintf("%d:%02d", mins/60, mins%60)
	return fmt.Sprintf("%sCLOCK: %s--%s => %5s", ind, orgStamp(start), orgStamp(end), dur)
}

func GetClockPath() string {
	return path.Join(Conf().PlugManager.HomeDir, "clock_data.json")
}

func (self *OrgsClock) WriteOutClock() {
	file, _ := json.MarshalIndent(self, "", " ")
	_ = os.WriteFile(GetClockPath(), file, 0644)
}

func (self *OrgsClock) ReadInClock() {
	if data, err := os.ReadFile(GetClockPath()); err == nil {
		json.Unmarshal(data, self)
	}
}

var orgClock *OrgsClock = nil

func Clock() *OrgsClock {
	if orgClock == nil {
		orgClock = &OrgsClock{}
		orgClock.ReadInClock()
	}
	return orgClock
}
