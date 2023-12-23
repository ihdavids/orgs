// This pulls constants out of the current org file header
package orgs

import (
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

func ProcessConsts(m *map[string]interface{}, data string) {
	if data != "" {
		params := strings.Split(data, " ")
		for _, p := range params {
			p = strings.TrimSpace(p)
			if p != "" {
				if k, v, ok := strings.Cut(p, "="); ok {
					(*m)["$"+k] = v
				}
			}
		}
	}
}

func GetConstants(constMap *map[string]interface{}, ofile *common.OrgFile, sec *org.Section) {

	// Process the global constant options
	consts := ofile.Doc.Get("CONSTANTS")
	ProcessConsts(constMap, consts)
	consts = ofile.Doc.Get("Constants")
	ProcessConsts(constMap, consts)
	consts = ofile.Doc.Get("constants")
	ProcessConsts(constMap, consts)

	// Process the node specific options if present
	if sec != nil && sec.Headline != nil && sec.Headline.Properties != nil && sec.Headline.Properties.Properties != nil {
		for _, p := range sec.Headline.Properties.Properties {
			if len(p) == 2 {
				k, v := p[0], p[1]
				(*constMap)["$PROP_"+k] = v
			}
		}
	}
}
