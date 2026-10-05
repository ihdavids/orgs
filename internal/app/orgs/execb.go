package orgs

/* SDOC: Editing
* Bable Block Execution

  TODO: Fill in information on babel and literate programming
EDOC */

import (
	"fmt"
	"os"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

func ExecBlock(db common.ODb, t *common.PreciseTarget) (common.ResultMsg, error) {
	res := common.ResultMsg{Ok: false, Msg: "Unknown block exec error"}
	ofile, _, block := db.GetFromPreciseTarget(t, org.BlockNode)
	if block != nil {
		blk := block.(*org.Block)
		if blk.Name == "SRC" {
			Log().Infof("Babel Block Execution\n")
			// TODO Babel by name
			if lang, ok := blk.ParameterMap()[":lang"]; ok {
				fmt.Fprintf(os.Stderr, "Running language: %s\n", lang)
			}
		} else if blk.Name == "DYN" {
			// Rewritten as lines between BEGIN and END (dynblock.go). This
			// used to put the result in the parsed block and write the whole
			// document back, re-indenting every drawer in the file.
			out, err := RefreshDynBlockAt(ofile.Doc.Path, blk.Pos.Row)
			if err != nil {
				return common.ResultMsg{Ok: false, Msg: err.Error()}, nil
			}
			res = common.ResultMsg{Ok: true, Msg: "refreshed"}
			for _, b := range out.Blocks {
				if !b.Ok {
					res = common.ResultMsg{Ok: false, Msg: b.Msg}
				}
			}
		}
	}
	return res, nil
}
