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
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

type OrgsClock struct {
	Time   *org.OrgDate
	Target *common.Target
}

func (self *OrgsClock) ClockIn(tgt *common.Target) (common.ResultMsg, error) {
	self.ClockOut()
	if err := GetDb().ConvertTargetToOlp(tgt); err != nil {
		return common.ResultMsg{Ok: false, Msg: fmt.Sprintf("Could not clock in, could not convert target: %s", err.Error())}, err
	}
	self.Time = org.NewOrgDateNow()
	self.Time.HaveTime = true
	self.Target = tgt
	self.WriteOutClock()
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
		self.Time.End = time.Now()
		clk := &org.OrgDateClock{OrgDate: *self.Time}
		clk.RecalcDuration()
		clock := org.Clock{Date: clk}
		// TODO: Update target with the clock
		if ofile, secs := GetDb().GetFromTarget(self.Target, false); secs != nil {
			drawer := secs.Headline.FindDrawer(Conf().ClockIntoDrawer)
			if drawer != nil {
				drawer.Append(secs.Headline, clock)
				/*
					fmt.Printf("HAVE DRAWER\n")
					for _, c := range drawer.Children {
						fmt.Printf("[%s] %s ENTRY: %s\n", c.GetTypeName(), Conf().ClockIntoDrawer, c.String())
					}
				*/
			} else {
				drawer := &org.Drawer{Name: Conf().ClockIntoDrawer, Children: []org.Node{clock}}
				secs.Headline.AddDrawer(drawer)
			}
			WriteOutOrgFile(ofile)
		}
		// Clean out the clock
		self.Time = nil
		self.Target = nil
		self.WriteOutClock()
		return common.ResultMsg{Ok: true, Msg: "Clocked out okay"}, nil
	}
	return common.ResultMsg{Ok: false, Msg: "Clock was not active"}, nil
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
