package orgs

/* SDOC: Editing
* Bable Block Execution

  TODO: Fill in information on babel and literate programming
EDOC */

import (
	"fmt"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

func ExecBlock(db common.ODb, t *common.PreciseTarget) (common.ResultMsg, error) {
	res := common.ResultMsg{Ok: false, Msg: "Unknown block exec error"}
	ofile, sec, block := db.GetFromPreciseTarget(t, org.BlockNode)
	if block != nil {
		blk := block.(*org.Block)
		if blk.Name == "SRC" {
			Log().Infof("Babel Block Execution\n")
			// TODO Babel by name
			if lang, ok := blk.ParameterMap()[":lang"]; ok {
				fmt.Printf("Running language: %s\n", lang)
			}
		} else if blk.Name == "DYN" {
			Log().Infof("Dynamic Block Execution\n")
			if lang, ok := blk.ParameterMap()[":lang"]; ok {
				fmt.Printf("Function name: %s\n", lang)
				if blockExec, ok := Conf().PlugManager.BlockExec[lang]; ok {
					fmt.Printf("Have function\n")
					res = *blockExec(ofile, sec, blk)
					if res.Ok {
						blk.Children = []org.Node{org.Text{Content: res.Msg}}
						WriteOutOrgFile(ofile)
					}
				}
			}
			for k, v := range blk.ParameterMap() {
				fmt.Printf("KEY: %s VAL: %v\n", k, v)
			}
		}
	}
	return res, nil
}
