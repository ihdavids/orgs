package orgs

import (
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"

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
		todayData := RenderTemplate(template, context)
		ioutil.WriteFile(filename, []byte(todayData), fs.ModePerm)
	}
	return []string{filename}, nil
}

func GetDayPageAt(dt *common.DateTime) (common.FileList, error) {
	filename, _ := getDayPageFilename(time.Time(*dt))
	return []string{filename}, nil
}
