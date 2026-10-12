package version

// Prints this binary's version, and the server's beside it with whether a
// newer release is out. The server is the one that asks github, so a client
// with no network of its own still finds out, and -client skips the server.

import (
	"flag"
	"fmt"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Version struct {
	ClientOnly bool
}

// Answer is the -json shape: this binary, and the server when it answered.
type Answer struct {
	Client      common.VersionInfo  `json:"client"`
	Server      *common.VersionInfo `json:"server,omitempty"`
	ServerError string              `json:"serverError,omitempty"`
}

func (self *Version) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Version) StartPlugin(manager *common.PluginManager) {
}

func (self *Version) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.ClientOnly, "client", false, "only this binary's version; do not ask the server")
}

func (self *Version) Exec(core *commands.Core) {
	ans := Answer{Client: common.BuildVersion()}
	if !self.ClientOnly {
		if sv, err := commands.SendReceiveGetErr[common.VersionInfo](core, "version", map[string]string{}); err == nil && sv.Version != "" {
			ans.Server = &sv
		} else if err != nil {
			ans.ServerError = err.Error()
		} else {
			ans.ServerError = "the server did not say (older than this command?)"
		}
	}
	commands.RenderOne(ans, func() {
		fmt.Printf("orgs %s\n", describe(ans.Client))
		if self.ClientOnly {
			return
		}
		if ans.Server == nil {
			fmt.Printf("server %s: %s\n", core.Rest.Url, ans.ServerError)
			return
		}
		fmt.Printf("server %s (%s)\n", describe(*ans.Server), core.Rest.Url)
		switch {
		case ans.Server.UpdateAvailable:
			fmt.Printf("%supdate available:%s %s %s\n", commands.C("33"), commands.C("0"), ans.Server.Latest, ans.Server.ReleaseUrl)
		case ans.Server.Latest != "" && !common.IsRelease(ans.Server.Version):
			fmt.Printf("latest release: %s (a development build is not compared)\n", ans.Server.Latest)
		case ans.Server.Latest != "":
			fmt.Printf("%sup to date%s (latest %s)\n", commands.C("32"), commands.C("0"), ans.Server.Latest)
		case ans.Server.CheckError != "":
			fmt.Printf("update check: %s\n", ans.Server.CheckError)
		}
	})
}

func describe(v common.VersionInfo) string {
	s := v.Version
	if v.Commit != "" {
		s += " " + v.Commit
	}
	if v.Date != "" {
		s += " " + v.Date
	}
	return s + fmt.Sprintf(" %s/%s %s", v.Os, v.Arch, v.Go)
}

func init() {
	commands.AddCmd("version", "this binary's version, the server's, and whether a newer release is out",
		func() commands.Cmd {
			return &Version{}
		})
}
