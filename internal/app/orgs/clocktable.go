package orgs

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

type ClockTableState struct {
	Scope     string
	Durations map[*org.Section]*ClockTableDuration
	MaxLevel  int
	SkipEmpty bool
	Match     *MatchExpr
	Block     *BlockTest
}

/*
‘2007-12-31’	New year eve 2007
‘2007-12’	December 2007
‘2007-W50’	ISO-week 50 in 2007
‘2007-Q2’	2nd quarter in 2007
‘2007’	the year 2007
‘today’, ‘yesterday’, ‘today-N’	a relative day
‘thisweek’, ‘lastweek’, ‘thisweek-N’	a relative week
‘thismonth’, ‘lastmonth’, ‘thismonth-N’	a relative month
‘thisyear’, ‘lastyear’, ‘thisyear-N’	a relative year
‘untilnow’77	all clocked time ever
*/
type BlockTest struct {
	Date *org.OrgDate
}

func ParseBlock(blk string) *BlockTest {
	if blk == "today" {
		b := &BlockTest{}
		b.Date = &org.OrgDate{Start: time.Now(), End: time.Now(), HaveTime: true}
		t := b.Date.Start
		b.Date.Start = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		t = b.Date.End
		b.Date.End = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, t.Location())
		return b
	} else if blk == "yesterday" {
		b := &BlockTest{}
		b.Date = &org.OrgDate{Start: time.Now().AddDate(0, 0, -1), End: time.Now().AddDate(0, 0, -1), HaveTime: true}
		t := b.Date.Start
		b.Date.Start = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		t = b.Date.End
		b.Date.End = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, t.Location())
		return b
	} else if blk == "thisweek" {
		b := &BlockTest{}
		now := time.Now()
		w := now.Weekday()
		fmt.Printf("Weekday: %d\n", int(w))
		b.Date = &org.OrgDate{Start: time.Now().AddDate(0, 0, -int(w)), End: time.Now(), HaveTime: true}
		t := b.Date.Start
		b.Date.Start = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		t = b.Date.End
		b.Date.End = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, t.Location())
		return b
	} else if blk == "lastweek" {
		b := &BlockTest{}
		now := time.Now()
		w := now.Weekday()
		b.Date = &org.OrgDate{Start: time.Now().AddDate(0, 0, -7+-int(w)), End: time.Now().AddDate(0, 0, -int(w)), HaveTime: true}
		t := b.Date.Start
		b.Date.Start = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		t = b.Date.End
		b.Date.End = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, t.Location())
		return b
	} else if blk == "thismonth" {
		b := &BlockTest{}
		b.Date = &org.OrgDate{Start: time.Now(), End: time.Now(), HaveTime: true}
		t := b.Date.Start
		b.Date.Start = time.Date(t.Year(), t.Month(), 0, 0, 0, 0, 0, t.Location())
		t = b.Date.Start
		b.Date.Start = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		t = b.Date.End
		b.Date.End = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, t.Location())
		return b
	} else if blk == "lastmonth" {
		b := &BlockTest{}
		b.Date = &org.OrgDate{Start: time.Now(), End: time.Now(), HaveTime: true}
		t := b.Date.Start
		month := t.Month()
		year := t.Year()

		if month > 1 {
			month = month - 1
		} else {
			month = 12
			year = year - 1
		}
		b.Date.Start = time.Date(year, month, 0, 0, 0, 0, 0, t.Location())
		b.Date.End = b.Date.Start.AddDate(0, 1, 0)
		t = b.Date.End
		b.Date.End = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, t.Location())
		return b
	}
	return nil
}

func (s *ClockTableState) Validate() error {
	if s.Scope != "subtree" && s.Scope != "file" {
		return fmt.Errorf("clocktable - Only subtree or file mode is supported at this time!")
	}
	return nil
}

func (s *ClockTableState) ParseParams(params map[string]string) error {
	ok := true
	var err error
	var temp string
	if s.Scope, ok = params[":scope"]; !ok {
		s.Scope = "subtree"
	}
	if temp, ok = params[":maxlevel"]; ok {
		if s.MaxLevel, err = strconv.Atoi(temp); err != nil {
			s.MaxLevel = 0
			return err
		}
	} else {
		s.MaxLevel = 0
	}
	if temp, ok = params[":skip0"]; ok {
		s.SkipEmpty = temp == "t" || temp == ""
	}
	if temp, ok = params[":match"]; ok {
		s.Match = NewMatchExpr(temp)
	}
	if temp, ok = params[":block"]; ok {
		s.Block = ParseBlock(temp)
	}
	return nil
}

func (s *ClockTableState) CalcClockDuration(sec *org.Section) *common.OrgDuration {
	local := &common.OrgDuration{Mins: 0}
	drawer := sec.Headline.FindDrawer(Conf().ClockIntoDrawer)
	if drawer != nil && drawer.Children != nil {
		for _, c := range drawer.Children {
			if c.GetType() == org.ClockNode {
				clk := c.(org.Clock)
				if s.Block == nil || s.Block.Date.DateTimeInRange(clk.Date.Start) || s.Block.Date.DateTimeInRange(clk.Date.End) {
					// TODO: Add validation tests in computing this value
					dur := common.NewDuration(float64(clk.Date.DurationMins))
					local = local.Add(&dur)
				}
			}
		}
	}
	return local
}

type ClockTableDuration struct {
	Local *common.OrgDuration
	Total *common.OrgDuration
}

func NewClockDuration() *ClockTableDuration {
	return &ClockTableDuration{&common.OrgDuration{Mins: 0}, &common.OrgDuration{Mins: 0}}
}

func (s *ClockTableState) GenerateForSubtree(ofile *common.OrgFile, sec *org.Section) *common.OrgDuration {
	ct := NewClockDuration()
	ct.Local = s.CalcClockDuration(sec)
	for _, c := range sec.Children {
		d := s.GenerateForSubtree(ofile, c)
		ct.Total = ct.Total.Add(d)
	}
	ct.Total = ct.Total.Add(ct.Local)

	if s.Match == nil || s.Match.EvalSection(ofile, sec) {
		s.Durations[sec] = ct
		if s.Match != nil {
			s.Match.Reset()
		}
		return ct.Total
	}
	return &common.OrgDuration{}
}

func (s *ClockTableState) flattenData(ofile *common.OrgFile, data *[]map[string]interface{}, sec *org.Section, blvl int) {
	if s.MaxLevel <= 0 || sec.Headline.Lvl <= s.MaxLevel {
		if sd, ok := s.Durations[sec]; ok {
			d := map[string]interface{}{}
			d["total"] = sd.Total.ToString()
			if sec.Headline.Lvl == s.MaxLevel {
				d["local"] = sd.Total.ToString()
			} else {
				d["local"] = sd.Local.ToString()
			}
			hindent := ""
			lvl := sec.Headline.Lvl - blvl
			if lvl > 0 {
				hindent = "\\_"
			}
			if lvl > 1 {
				hindent = hindent + strings.Repeat("__", lvl-1)
			}
			if hindent != "" {
				hindent = hindent + " "
			}
			d["title"] = hindent + common.GetSectionTitle(sec)
			d["lvl"] = sec.Headline.Lvl
			if !s.SkipEmpty || sd.Local.Mins != 0 || ((s.MaxLevel <= 0 || sec.Headline.Lvl < s.MaxLevel) && len(sec.Children) > 0) {
				*data = append(*data, d)
			}
		}
		if s.MaxLevel <= 0 || sec.Headline.Lvl < s.MaxLevel {
			for _, c := range sec.Children {
				s.flattenData(ofile, data, c, blvl)
			}
		}
	}
}

func (s *ClockTableState) GenerateTableForSubtree(ofile *common.OrgFile, sec *org.Section) string {
	res := ""

	data := []map[string]interface{}{}
	for _, c := range sec.Children {
		s.flattenData(ofile, &data, c, sec.Headline.Lvl)
	}

	template := "defaultclocktable.tpl"
	ctx := make(map[string]interface{})
	ctx["data"] = data
	res = Conf().PlugManager.Tempo.RenderTemplate(template, ctx)
	//fmt.Printf("TABLE\n%s\n", res)
	return res
}

// MAIN - Clock table entry
func GenerateClockTable(ofile *common.OrgFile, parent *org.Section, blk *org.Block) *common.ResultMsg {
	var res common.ResultMsg
	res.Ok = false
	res.Msg = "Unknown error"
	ct := ClockTableState{}
	ct.Durations = map[*org.Section]*ClockTableDuration{}
	if err := ct.ParseParams(blk.ParameterMap()); err != nil {
		res.Msg = err.Error()
		return &res
	}
	if err := ct.Validate(); err != nil {
		res.Msg = err.Error()
		return &res
	}

	// Use file level scope
	if ct.Scope == "file" {
		parent = ofile.Doc.Outline.Section
	}
	// Now we have clock table data
	ct.GenerateForSubtree(ofile, parent)
	resStr := ct.GenerateTableForSubtree(ofile, parent)

	if resStr != "" {
		res.Ok = true
		res.Msg = resStr
	}
	return &res
}

func init() {
	Conf().PlugManager.AddBlockMethod("clocktable", GenerateClockTable)
}
