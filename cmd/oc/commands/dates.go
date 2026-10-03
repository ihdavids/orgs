package commands

// The date parser lives in internal/orgdate, where the template function
// when() can reach it too: `orgs sched +2w` and {{ when("+2w") }} are one
// reading of one sentence, not two that drift apart.

import (
	"time"

	"github.com/ihdavids/orgs/internal/orgdate"
)

type OrgDate = orgdate.OrgDate

func ParseDate(s string, now time.Time) (OrgDate, bool, error) {
	return orgdate.ParseDate(s, now)
}

func ParseDateToOrg(s string, now time.Time) (string, error) {
	return orgdate.ParseDateToOrg(s, now)
}
