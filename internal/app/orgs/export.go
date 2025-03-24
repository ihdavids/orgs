package orgs

import (
	"fmt"
	"log"

	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/common"
)

func ExportToFile(db plugs.ODb, args *common.ExportToFile) (common.ResultMsg, error) {
	fmt.Printf("EXPORT CALLED!\n")
	var didWrite = false
	msg := "Unknown Error"
	for _, exp := range Conf().Exporters {
		if exp.Name == args.Name {
			err := exp.Plugin.Export(db, args.Query, args.Filename, args.Opts, args.Props)
			if err == nil {
				didWrite = true
				msg = "Success"
			} else {
				didWrite = false
				msg = err.Error()
			}
			log.Printf("EXPORT: %s\n", exp.Name)
			break
		}
	}
	if !didWrite {
		msg = fmt.Sprintf("ERROR: Did not export is %s setup in the config file?\n", args.Name)
		log.Printf("%v", msg)
	}
	return common.ResultMsg{Ok: didWrite, Msg: msg}, nil
}

func ExportToString(db plugs.ODb, args *common.ExportToFile) (common.ResultMsg, error) {
	fmt.Printf("EXPORT String CALLED!\n")
	var didWrite = false
	msg := "Unknown Error"
	for _, exp := range Conf().Exporters {
		if exp.Name == args.Name {
			err, txt := exp.Plugin.ExportToString(db, args.Query, args.Opts, args.Props)
			if err == nil {
				didWrite = true
				msg = txt
			} else {
				didWrite = false
				msg = err.Error()
			}
			log.Printf("EXPORT: %s\n", exp.Name)
			break
		}
	}
	if !didWrite {
		msg = fmt.Sprintf("ERROR: Did not export is %s setup in the config file?\n", args.Name)
		log.Printf("%v", msg)
	}
	return common.ResultMsg{Ok: didWrite, Msg: msg}, nil
}

func PluginUpdateTarget(db plugs.ODb, args *common.Target, name string) (common.ResultMsg, error) {
	fmt.Printf("UPDATE CALLED!\n")
	var didWrite = false
	msg := "Unknown Error"
	for _, exp := range Conf().Updaters {
		if exp.Name == name {
			res, err := exp.Plugin.UpdateTarget(db, args, Conf().PlugManager)
			if err == nil {
				didWrite = true
				msg = res.Msg
			} else {
				didWrite = false
				msg = err.Error()
			}
			log.Printf("UPDATE: %s\n", exp.Name)
			break
		}
	}
	if !didWrite {
		msg = fmt.Sprintf("ERROR: Did not update is %s setup in the config file?\n", name)
		log.Printf("%v", msg)
	}
	return common.ResultMsg{Ok: didWrite, Msg: msg}, nil
}
