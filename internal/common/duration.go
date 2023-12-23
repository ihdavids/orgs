package common

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func reMatchParams(regEx, txt string) (paramsMap map[string]string) {

	var compRegEx = regexp.MustCompile(regEx)
	match := compRegEx.FindStringSubmatch(txt)

	// Return nil if we did not match anything!
	if match == nil || len(match) <= 0 {
		return nil
	}

	empty := true
	for _, m := range match {
		if m != "" {
			empty = false
			break
		}
	}
	if empty {
		return nil
	}
	// Otherwise build a param map out of the match
	paramsMap = make(map[string]string)
	for i, name := range compRegEx.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}
	return paramsMap
}

type OrgDuration struct {
	Mins float64
}

func NewDuration(mins float64) OrgDuration {
	return OrgDuration{Mins: mins}
}

func (self *OrgDuration) ToString() string {
	r := ""
	y := int(self.Mins / 525600.0)
	if y > 0 {
		r += fmt.Sprintf("%dy ", y)
	}
	days := math.Mod(self.Mins, 525600.0)
	d := int(days / 1440.0)
	if d > 0 {
		r += fmt.Sprintf("%dd ", d)
	}
	hours := math.Mod(days, 1440.0)
	h := int(hours / 60.0)
	if h > 0 {
		r += fmt.Sprintf("%dh ", h)
	}
	mins := int(math.Mod(hours, 60))
	if mins > 0 {
		r += fmt.Sprintf("%dmins", mins)
	}
	return strings.TrimSpace(r)
}

func (self *OrgDuration) Days() float64 {
	return self.Mins / 1440.0
}

func (self *OrgDuration) Duration() time.Duration {
	return time.Duration(float64(time.Minute) * self.Mins)
}

func (self *OrgDuration) Sub(o *OrgDuration) *OrgDuration {
	t := NewDuration(self.Mins - o.Mins)
	return &t
}

func (self *OrgDuration) Add(o *OrgDuration) *OrgDuration {
	t := NewDuration(self.Mins + o.Mins)
	return &t
}

func ParseDuration(txt string) *OrgDuration {
	m := reMatchParams(`\s*((?P<years>[0-9.]+)y)?\s*((?P<weeks>[0-9.]+)w)?\s*((?P<days>[0-9.]+)d)?\s*((?P<hours>[0-9.]+)h)?\s*((?P<mins>[0-9.]+)min)?\s*((?P<thours>[0-9]+)[:](?P<tmins>[0-9]+)([:](?P<tsecs>[0-9]+))?)?`, txt)
	if m != nil {
		mtot := 0.0
		got := false
		if y, ok := m["years"]; ok {
			x, _ := strconv.ParseFloat(y, 64)
			mtot += x * 525600.0
			got = true
		}
		if w, ok := m["weeks"]; ok {
			x, _ := strconv.ParseFloat(w, 64)
			mtot += x * 7200.0
			got = true
		}
		if d, ok := m["days"]; ok {
			x, _ := strconv.ParseFloat(d, 64)
			mtot += x * 1440.0
			got = true
		}
		if h, ok := m["hours"]; ok {
			x, _ := strconv.ParseFloat(h, 64)
			mtot += x * 60.0
			got = true
		}
		if mins, ok := m["mins"]; ok {
			x, _ := strconv.ParseFloat(mins, 64)
			mtot += x
			got = true
		}
		if thours, ok := m["thours"]; ok {
			x, _ := strconv.ParseFloat(thours, 64)
			mtot += x * 60.0
			got = true
		}
		if tmins, ok := m["tmins"]; ok {
			x, _ := strconv.ParseFloat(tmins, 64)
			mtot += x
			got = true
		}
		if secs, ok := m["tsecs"]; ok {
			x, _ := strconv.ParseFloat(secs, 64)
			mtot += x * 0.01666667
			got = true
		}
		if got {
			d := OrgDuration{Mins: mtot}
			return &d
		}
	}
	return nil
}

func ParseWorkDuration(txt string) *OrgDuration {
	m := reMatchParams(`\s*((?P<years>[0-9.]+)y)?\s*((?P<weeks>[0-9.]+)w)?\s*((?P<days>[0-9.]+)d)?\s*((?P<hours>[0-9.]+)h)?\s*((?P<mins>[0-9.]+)min)?\s*((?P<thours>[0-9]+)[:](?P<tmins>[0-9]+)([:](?P<tsecs>[0-9]+))?)?`, txt)
	if m != nil {
		mtot := 0.0
		got := false
		if y, ok := m["years"]; ok {
			x, _ := strconv.ParseFloat(y, 64)
			mtot += x * 170880.0
			got = true
		}
		if w, ok := m["weeks"]; ok {
			x, _ := strconv.ParseFloat(w, 64)
			mtot += x * 2400.0
			got = true
		}
		if d, ok := m["days"]; ok {
			x, _ := strconv.ParseFloat(d, 64)
			mtot += x * 480.0
			got = true
		}
		if h, ok := m["hours"]; ok {
			x, _ := strconv.ParseFloat(h, 64)
			mtot += x * 60.0
			got = true
		}
		if mins, ok := m["mins"]; ok {
			x, _ := strconv.ParseFloat(mins, 64)
			mtot += x
			got = true
		}
		if thours, ok := m["thours"]; ok {
			x, _ := strconv.ParseFloat(thours, 64)
			mtot += x * 60.0
			got = true
		}
		if tmins, ok := m["tmins"]; ok {
			x, _ := strconv.ParseFloat(tmins, 64)
			mtot += x
			got = true
		}
		if secs, ok := m["tsecs"]; ok {
			x, _ := strconv.ParseFloat(secs, 64)
			mtot += x * 0.01666667
			got = true
		}
		if got {
			d := OrgDuration{Mins: mtot}
			return &d
		}
	}
	return nil
}

/*
class OrgDuration:


    def timedelta(self):
        y     = int(self.mins / 525600.0)
        days  = math.fmod(self.mins,525600.0)
        d     = int(days / 1440.0)
        hours = math.fmod(days,1440.0)
        h     = int(hours/60.0)
        mins  = int(math.fmod(hours,60))
        td = datetime.timedelta(days=d+(y*365),hours=h,minutes=mins)
        return td

    @staticmethod
    def FromTimedelta(td: datetime.timedelta):
        return OrgDuration(td.days*1440 + (td.seconds/60.0))

*/
