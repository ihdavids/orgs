package orgs

import (
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
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
	for i := 0; i < 10; i++ {

		dt = dt.AddDate(0, 0, offset)
		filename, title := getDayPageFilename(dt)
		if _, err := os.Stat(filename); err != nil {
			return filename, title
		}
	}
	return "", ""
}

func CreateDayPage() (common.FileList, error) {

	template := Conf().DayPageTemplate
	dt := time.Now()
	filename, title := getDayPageFilename(dt)
	if _, err := os.Stat(filename); err != nil {
		var context map[string]string = make(map[string]string)
		fmt.Printf("TRYING TO CREATE DAY PAGE: %s\n", title)
		context["day_page_title"] = title
		context["weekday"] = dt.Format("Mon")
		context["day"] = fmt.Sprintf("%d", dt.Day())
		context["month"] = fmt.Sprintf("%d", dt.Month())
		context["year"] = fmt.Sprintf("%d", dt.Year())

		var nodes []*org.Section
		oldFn, _ := getPreviousDayPage(dt)
		if oldFn != "" {
			if ofile := GetDb().FindByFile(oldFn); ofile != nil {
				nodes, _ = QueryStringNodesOnFile("!IsArchived() && IsTask() && IsActive()", ofile)

				// Now go archive the old page since we have a new page to work with.
				if AddFileTag("ARCHIVE", ofile.doc) {
					WriteOutOrgFile(ofile)
				}
			}
		}

		todayData := RenderTemplate(template, context)
		if len(nodes) > 0 {
			d := GetConfig().Parse(strings.NewReader(todayData), filename)
			d.Outline.Children = append(d.Outline.Children, nodes...)
		}

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
