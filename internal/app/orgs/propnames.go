package orgs

// The property names written in the org files, for a query box to offer.
//
// HasProperty("…") and MatchProperty("…", "…") are only as useful as the
// name typed into them, and nothing else on the server could say which names
// exist. This reads them off `Headline.Properties` - the same place the query
// functions look - so a name offered is a name a query can actually find. (A
// drawer written in column zero is invisible to both, for the reason in
// **Traps: column-zero drawers**; offering it would suggest a query that
// matches nothing.)

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/ihdavids/orgs/internal/common"
)

// PropertyUse is one property name, how many headings carry it, and the
// values it is most often given - which is what the second argument of
// MatchProperty wants.
type PropertyUse struct {
	Name   string
	Count  int
	Values []string
}

// More values than this is a list nobody reads, and a value longer than the
// other limit is a note rather than something to match against.
const (
	mostPropValues = 12
	longPropValue  = 60
)

type propPart map[string]map[string]int

var propParts = NewFileParts[propPart]()

func propertiesInUse() []PropertyUse {
	per := propParts.All(func(f *common.OrgFile) propPart {
		part := propPart{}
		for _, sec := range flattenSections(f) {
			keys, vals := sectionProps(sec)
			for _, k := range keys {
				if part[k] == nil {
					part[k] = map[string]int{}
				}
				part[k][vals[k]]++
			}
		}
		return part
	})

	all := map[string]map[string]int{}
	for _, part := range per {
		for k, vs := range part {
			if all[k] == nil {
				all[k] = map[string]int{}
			}
			for v, n := range vs {
				all[k][v] += n
			}
		}
	}

	out := make([]PropertyUse, 0, len(all))
	for k, vs := range all {
		use := PropertyUse{Name: k, Values: []string{}}
		type vn struct {
			v string
			n int
		}
		list := []vn{}
		for v, n := range vs {
			use.Count += n
			if v != "" && len(v) <= longPropValue {
				list = append(list, vn{v, n})
			}
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].n != list[j].n {
				return list[i].n > list[j].n
			}
			return list[i].v < list[j].v
		})
		for i := 0; i < len(list) && i < mostPropValues; i++ {
			use.Values = append(use.Values, list[i].v)
		}
		out = append(out, use)
	}
	// Commonest first: the name somebody is reaching for is usually one they
	// write a lot.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

/*
		SDOC: API

	  - GET /properties — List Property Names In Use
	    Every property name written in a heading's property drawer across the org files,
	    commonest first, with how many headings carry it and its most frequent values.
	    This is what the worg query box offers inside =HasProperty("…")= and
	    =MatchProperty("…", "…")=.

	    *Method:* =GET=

	    *Parameters:* None.

	    *Response:* =[{"Name": "EFFORT", "Count": 41, "Values": ["1:00", "0:30"]}]=
	    EDOC
*/
func RequestProperties(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(propertiesInUse())
}
