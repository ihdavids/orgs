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
	if Conf().Server.DayPageMode == "week" {
		firstDay := strings.ToLower(Conf().Server.DayPageModeWeekDay)
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
	filename = path.Join(Conf().Server.DayPagePath, filename)
	return filename, title
}

// dayPageRelPath answers {{daypage}} in a capture template: the day page in
// force at that moment, relative to the first org directory, with / for a
// separator so it also reads as an org file: link. A day page outside every org
// directory comes back absolute.
func dayPageRelPath(now time.Time) (string, bool) {
	if Conf().Server == nil || len(Conf().Server.OrgDirs) == 0 {
		return "", false
	}
	filename, _ := getDayPageFilename(now)
	abs, err := filepath.Abs(filename)
	if err != nil {
		return "", false
	}
	root, err := filepath.Abs(Conf().Server.OrgDirs[0])
	if err != nil {
		return filepath.ToSlash(abs), true
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(abs), true
	}
	return filepath.ToSlash(rel), true
}

// isCurrentDayPage reports whether a file is the day page for now, so a
// capture into it creates it from the day page template rather than a blank
// new file.
func isCurrentDayPage(fname string, now time.Time) bool {
	dp, _ := getDayPageFilename(now)
	a, err1 := filepath.Abs(fname)
	b, err2 := filepath.Abs(dp)
	return err1 == nil && err2 == nil && a == b
}

func init() {
	common.CapDayPage = dayPageRelPath
}

func getPreviousDayPage(dt time.Time) (string, string) {
	offset := -1
	if Conf().Server.DayPageMode == "week" {
		offset = -7
	}
	for i := 0; i < Conf().Server.DayPageMaxSearchBack; i++ {

		dt = dt.AddDate(0, 0, offset)
		filename, title := getDayPageFilename(dt)
		if _, err := os.Stat(filename); err == nil {
			filename, _ = filepath.Abs(filename)
			fmt.Fprintf(os.Stderr, "Found old daypage: %s\n", filename)
			return filename, title
		}
	}
	fmt.Fprintf(os.Stderr, "Did not find old daypage!\n")
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

	template := Conf().Server.DayPageTemplate
	dt := time.Now()
	filename, title := getDayPageFilename(dt)
	if _, err := os.Stat(filename); err != nil {
		var context map[string]interface{} = make(map[string]interface{})
		// The clock names (date, weekday, week_start ...) come from the standard
		// context, which reads the same clock dt was taken from.
		context["day_page_title"] = title
		context["filename"] = filename
		context["basename"] = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))

		var nodes []*org.Section
		oldFn, _ := getPreviousDayPage(dt)
		//fmt.Fprintf(os.Stderr, "PREV DAY: %s\n", oldFn)
		if oldFn != "" {
			if ofile := GetDb().FindByFile(oldFn); ofile != nil {
				nodes, _ = QueryStringNodesOnFile("!IsArchived() && IsTask() && IsActive()", ofile)
				//fmt.Fprintf(os.Stderr, "WE HAVE %d NODES\n", len(nodes))

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
					fmt.Fprintf(os.Stderr, "APPENDING: %s\n", n.Headline.Title[0].String())
					d.Nodes = append(d.Nodes, *n.Headline)
				}
			}

			w := org.NewOrgWriter()
			d.Write(w)
			todayData = w.String()
		}
		fmt.Fprintf(os.Stderr, "WRITING TEMPLATE %s\n", filename)
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
