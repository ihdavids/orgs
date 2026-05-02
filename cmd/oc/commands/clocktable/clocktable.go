package clocktable

import (
	"flag"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/koki-develop/go-fzf"
)

var blockChoices = []string{
	"today",
	"yesterday",
	"thisweek",
	"lastweek",
	"thismonth",
	"lastmonth",
}

type ClockTable struct {
	Block string
}

func (self *ClockTable) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *ClockTable) StartPlugin(manager *common.PluginManager) {
}

func (self *ClockTable) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&self.Block, "block", "", "time range (today, yesterday, thisweek, lastweek, thismonth, lastmonth)")
}

func fmtDuration(mins float64) string {
	h := int(mins / 60)
	m := int(math.Mod(mins, 60))
	return fmt.Sprintf("%2d:%02d", h, m)
}

func (self *ClockTable) Exec(core *commands.Core) {
	block := self.Block
	if block == "" {
		f, err := fzf.New(
			fzf.WithPrompt("Time range> "),
		)
		if err != nil {
			log.Fatal(err)
		}
		idxs, err := f.Find(blockChoices, func(i int) string { return blockChoices[i] })
		if err != nil {
			log.Fatal(err)
		}
		if len(idxs) == 0 {
			fmt.Println("No time range selected")
			return
		}
		block = blockChoices[idxs[0]]
	}

	var qry map[string]string = map[string]string{"block": block}
	var report common.ClockReport
	commands.SendReceiveGet(core, "clockreport", qry, &report)

	if len(report.Entries) == 0 {
		fmt.Printf("No clock entries for: %s\n", block)
		return
	}

	// Group entries by file
	type fileGroup struct {
		file    string
		entries []common.ClockEntry
		total   float64
	}
	grouped := map[string]*fileGroup{}
	var fileOrder []string
	for _, e := range report.Entries {
		g, ok := grouped[e.Filename]
		if !ok {
			g = &fileGroup{file: e.Filename}
			grouped[e.Filename] = g
			fileOrder = append(fileOrder, e.Filename)
		}
		g.entries = append(g.entries, e)
		g.total += e.Mins
	}
	sort.Strings(fileOrder)

	// Determine column widths
	maxHeading := 8
	for _, e := range report.Entries {
		indent := ""
		if e.Level > 1 {
			indent = strings.Repeat("  ", e.Level-1)
		}
		l := len(indent) + len(e.Headline)
		if l > maxHeading {
			maxHeading = l
		}
	}
	if maxHeading > 60 {
		maxHeading = 60
	}
	timeCol := 7
	totalWidth := maxHeading + timeCol + 5

	// Title
	divider := strings.Repeat("─", totalWidth)
	titleLabel := fmt.Sprintf("Clock Report: %s", block)

	fmt.Println()
	fmt.Println(divider)
	fmt.Printf("  %-*s %*s\n", maxHeading, titleLabel, timeCol, fmtDuration(report.TotalMin))
	fmt.Println(divider)

	for _, fname := range fileOrder {
		g := grouped[fname]
		shortFile := filepath.Base(fname)
		fmt.Printf("  %-*s %*s\n", maxHeading, "File: "+shortFile, timeCol, fmtDuration(g.total))
		fmt.Println("  " + strings.Repeat("╌", totalWidth-2))

		for _, e := range g.entries {
			indent := ""
			if e.Level > 1 {
				indent = strings.Repeat("  ", e.Level-1)
			}
			label := indent + e.Headline
			if len(label) > maxHeading {
				label = label[:maxHeading-1] + "…"
			}
			fmt.Printf("  %-*s %*s\n", maxHeading, label, timeCol, fmtDuration(e.Mins))
		}
		fmt.Println(divider)
	}
	fmt.Println()
}

// init function is called at boot
func init() {
	commands.AddCmd("clocktable", "show a clock report for a time range",
		func() commands.Cmd {
			return &ClockTable{}
		})
}
