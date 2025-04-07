package orgs

/* SDOC: Editing
* Day Page

	The day page module is designed to quickly create a worklog
	file for you every day. It is driven off a template file:

	daypage.tpl

	This template file will expand into a new worklog file when asked.
	I tend to operate with a single day page per week as I find
	a daypage per day is to verbose and a daypage per month is to messy.


 
	#+BEGIN_SRC yaml
    dayPagePath: "C:/path/worklog/"
	#+END_SRC

	My personal day page template looks about like so at the moment.

	#+BEGIN_SRC org	
    #+TITLE:  {{day_page_title}} 
    #+AUTHOR: Me Myself

    * Inbox

    * Mon
    * Tue
    * Wed
    * Thu
    * Fri
	#+END_SRC

EDOC */

import (
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

func getDayPageAt(dt time.Time) time.Time {
	change := []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}
	if Conf().DayPageMode == "week" {
		firstDay := strings.ToLower(Conf().DayPageModeWeekDay)
		startAt := 0
		for i, v := range change {
			if strings.HasPrefix(v, firstDay) {
				startAt = i
			}
		}
		offset := int(dt.Weekday()) - startAt
		if offset != 0 {
			dt = dt.AddDate(0, 0, -offset)
		}
	}
	return dt
}

func getDayPageFilename(from time.Time) (string, string) {
	dt := getDayPageAt(from)
	title := dt.Format("Mon_2006_01_02")
	filename := title + ".org"
	filename = path.Join(Conf().DayPagePath, filename)
	return filename, title
}

func getPreviousDayPage(dt time.Time) (string, string) {
	offset := -1
	if Conf().DayPageMode == "week" {
		offset = -7
	}
	for i := 0; i < Conf().DayPageMaxSearchBack; i++ {

		dt = dt.AddDate(0, 0, offset)
		filename, title := getDayPageFilename(dt)
		if _, err := os.Stat(filename); err == nil {
			filename, _ = filepath.Abs(filename)
			fmt.Printf("Found old daypage: %s\n", filename)
			return filename, title
		}
	}
	fmt.Printf("Did not find old daypage!\n")
	return "", ""
}

func ParentIn(nodes []*org.Section, me *org.Section) bool {
	for parent := me.Parent; parent != nil; parent = parent.Parent {
		for _, v := range nodes {
			if v == parent {
				return true
			}
		}
	}
	return false
}

func CreateDayPage() (common.FileList, error) {

	template := Conf().DayPageTemplate
	dt := time.Now()
	filename, title := getDayPageFilename(dt)
	if _, err := os.Stat(filename); err != nil {
		var context map[string]interface{} = make(map[string]interface{})
		context["day_page_title"] = title
		context["weekday"] = dt.Format("Mon")
		context["day"] = fmt.Sprintf("%d", dt.Day())
		context["month"] = fmt.Sprintf("%d", dt.Month())
		context["year"] = fmt.Sprintf("%d", dt.Year())

		var nodes []*org.Section
		oldFn, _ := getPreviousDayPage(dt)
		//fmt.Printf("PREV DAY: %s\n", oldFn)
		if oldFn != "" {
			if ofile := GetDb().FindByFile(oldFn); ofile != nil {
				nodes, _ = QueryStringNodesOnFile("!IsArchived() && IsTask() && IsActive()", ofile)
				//fmt.Printf("WE HAVE %d NODES\n", len(nodes))

				// Now go archive the old page since we have a new page to work with.
				if AddFileTag("ARCHIVE", ofile.Doc) {
					WriteOutOrgFile(ofile)
				}
			}
		}

		todayData := Conf().PlugManager.Tempo.RenderTemplate(template, context)
		if len(nodes) > 0 {
			d := GetConfig().Parse(strings.NewReader(todayData), filename)
			for _, n := range nodes {
				// This is crazy, I was appending *nodes, it was working but not writing them out!
				// Watch out for that sillyness!
				//
				// Double adding happens when we add a node with children, then add its children!
				if !ParentIn(nodes, n) {
					fmt.Printf("APPENDING: %s\n", n.Headline.Title[0].String())
					d.Nodes = append(d.Nodes, *n.Headline)
				}
			}

			w := org.NewOrgWriter()
			d.Write(w)
			todayData = w.String()
		}
		fmt.Printf("WRITING TEMPLATE %s\n", filename)
		ioutil.WriteFile(filename, []byte(todayData), fs.ModePerm)
	}
	return []string{filename}, nil
}

func GetDayPageAt(dts *common.Date) (common.FileList, error) {
	if dt, err := dts.Get(); err == nil {
		filename, _ := getDayPageFilename(dt)
		return []string{filename}, nil
	} else {
		return nil, err
	}
}
