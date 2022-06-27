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

func getDayPageToday() time.Time {
	dt := time.Now()
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
			dt = time.Now().AddDate(0, 0, -offset)
		}
	}
	return dt
}

func CreateDayPage() (common.Result, error) {

	template := Conf().DayPageTemplate
	dt := getDayPageToday()
	title := dt.Format("Mon_2006_01_02")
	var context map[string]string = make(map[string]string)
	fmt.Printf("TRYING TO CREATE DAY PAGE: %s\n", title)
	context["today_title"] = title
	todayData := RenderTemplate(template, context)
	filename := title + ".org"
	filename = path.Join(Conf().DayPagePath, filename)
	if _, err := os.Stat(filename); err != nil {
		ioutil.WriteFile(filename, []byte(todayData), fs.ModePerm)
	}
	return common.Result{true}, nil
}
