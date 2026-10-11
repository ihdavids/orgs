package contact

// orgs contact import: people from a vCard file.
//
//	orgs contact import contacts.vcf                 add the new, fill in the known
//	orgs contact import contacts.vcf -dry-run        what it would do, nothing written
//	orgs contact import a.vcf b.vcf -mode skip       leave people already there alone
//	orgs contact import old.vcf -mode overwrite      the card wins where they differ
//	cat x.vcf | orgs contact import -                from stdin
//
// The server does the work (POST /contacts/import) - the matching, the
// merging, the photos - so the terminal and worg cannot import the same file
// two ways. This reads the files, sends them, and says what happened.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

func (self *Contact) importCards(core *commands.Core, files []string) {
	if len(files) == 0 {
		commands.Fail("orgs contact import <file.vcf>... - which file? (- reads stdin)")
	}
	var text strings.Builder
	names := []string{}
	for _, f := range files {
		var b []byte
		var err error
		if f == "-" {
			b, err = io.ReadAll(os.Stdin)
			f = "stdin"
		} else {
			b, err = os.ReadFile(f)
		}
		if err != nil {
			commands.Fail("could not read %s: %v", f, err)
		}
		// Each file decoded on its own: one old Outlook file in Windows-1252
		// beside a UTF-8 one from a phone is the usual mix.
		text.WriteString(common.VCardText(b))
		text.WriteString("\n")
		names = append(names, filepath.Base(f))
	}

	req := common.ContactImportRequest{
		Text: text.String(), Mode: self.Mode, Type: self.Type, Filename: self.File,
		DryRun: commands.DryRun,
	}
	// A dry run is answered by the server, which plans without writing - so
	// it is sent rather than stopped at SendReceivePost the way a dry run of
	// any other write is. The plan is the point of asking.
	var res common.ContactImportResult
	var err error
	if commands.DryRun {
		res, err = common.RestPost[common.ContactImportResult](&core.Rest, "contacts/import", &req)
	} else {
		res, err = commands.SendReceivePostErr[common.ContactImportRequest, common.ContactImportResult](core, "contacts/import", &req)
	}
	if err != nil {
		commands.Fail("could not import: %v", err)
	}
	if !res.Ok {
		if res.Msg == "" {
			res.Msg = "the server said no, and not why"
		}
		if commands.JsonOut {
			commands.RenderOne(res, nil)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Could not import that: %s\n", res.Msg)
		if strings.Contains(res.Msg, "collection yet") {
			self.suggestCollection(core)
		}
		os.Exit(1)
	}
	if commands.RenderOne(res, nil) {
		return
	}
	self.printImport(res, strings.Join(names, ", "))
}

func (self *Contact) printImport(res common.ContactImportResult, from string) {
	head := "Imported"
	if res.DryRun {
		head = "Would import"
	}
	fmt.Printf("\n  %s%s%s %s  %s(%s, %d %s)%s\n\n",
		self.c(cBold), head, self.c(cReset), from,
		self.c(cDim), res.Mode, res.Cards, plural(res.Cards, "card", "cards"), self.c(cReset))

	w := 4
	for _, it := range res.Items {
		if n := len([]rune(it.Name)); n > w {
			w = n
		}
	}
	if w > 28 {
		w = 28
	}
	for _, it := range res.Items {
		mark, colour := "=", cDim
		switch it.Action {
		case "add":
			mark, colour = "+", cGreen
		case "update":
			mark, colour = "~", cCyan
		case "error":
			mark, colour = "!", cGold
		}
		name := it.Name
		if name == "" {
			name = fmt.Sprintf("card %d", it.Card)
		}
		fmt.Printf("  %s%s%s %s%s%s  %s\n",
			self.c(colour), mark, self.c(cReset),
			self.c(cBold), padRunes(name, w), self.c(cReset),
			self.describe(it))
		if self.All {
			for _, ch := range it.Changes {
				old := ""
				switch {
				case ch.How == "kept":
					old = self.c(cDim) + "  (the record says " + ch.Old + ")" + self.c(cReset)
				case ch.Old != "":
					old = self.c(cDim) + "  (was " + ch.Old + ")" + self.c(cReset)
				}
				fmt.Printf("  %*s   %s%-7s%s %-18s %s%s\n", w, "",
					self.c(cDim), ch.How, self.c(cReset), ch.Key, trunc(ch.Value, 60), old)
			}
		}
		for _, warn := range it.Warnings {
			fmt.Printf("  %*s   %s%s%s\n", w, "", self.c(cGold), warn, self.c(cReset))
		}
	}

	fmt.Printf("\n  %s%d added%s, %s%d updated%s, %d skipped",
		self.c(cGreen), res.Added, self.c(cReset), self.c(cCyan), res.Updated, self.c(cReset), res.Skipped)
	if res.Failed > 0 {
		fmt.Printf(", %s%d failed%s", self.c(cGold), res.Failed, self.c(cReset))
	}
	fmt.Println(".")
	if res.Target != "" && res.Added > 0 {
		fmt.Printf("  %sNew people go to %s%s\n", self.c(cDim), res.Target, self.c(cReset))
	}
	if res.DryRun {
		fmt.Printf("  %sNothing was written. Run it again without -dry-run to import.%s\n", self.c(cDim), self.c(cReset))
	} else if !self.All && (res.Added > 0 || res.Updated > 0) {
		fmt.Printf("  %s-all lists every field.%s\n", self.c(cDim), self.c(cReset))
	}
	fmt.Println()
}

// describe is one person's line: what happens and, briefly, to what.
func (self *Contact) describe(it common.ContactImportItem) string {
	adds, replaces, kept := []string{}, []string{}, []string{}
	for _, ch := range it.Changes {
		switch ch.How {
		case "add":
			adds = append(adds, ch.Key)
		case "replace":
			replaces = append(replaces, ch.Key)
		case "kept":
			kept = append(kept, ch.Key)
		}
	}
	photo := false
	for _, ch := range it.Changes {
		if ch.Key == "IMAGE" && strings.HasPrefix(ch.Value, "photo (") {
			photo = true
		}
	}
	parts := []string{}
	switch it.Action {
	case "add":
		n := len(adds)
		if photo {
			n--
		}
		parts = append(parts, fmt.Sprintf("new, %d %s", n, plural(n, "field", "fields")))
		if photo {
			parts = append(parts, "photo")
		}
	case "update":
		parts = append(parts, it.Reason)
		if len(adds) > 0 {
			parts = append(parts, "adds "+listOf(adds))
		}
		if len(replaces) > 0 {
			parts = append(parts, "replaces "+listOf(replaces))
		}
	default:
		parts = append(parts, it.Reason)
	}
	if it.Cards > 1 {
		parts = append(parts, fmt.Sprintf("%d cards in the file", it.Cards))
	}
	if len(it.Tags) > 0 {
		parts = append(parts, "tags "+strings.Join(it.Tags, " "))
	}
	if it.Notes {
		parts = append(parts, "a note")
	}
	if len(kept) > 0 && it.Action == "update" {
		parts = append(parts, "keeps its own "+listOf(kept))
	}
	return self.c(cDim) + strings.Join(parts, " · ") + self.c(cReset)
}

// padRunes cuts or pads to a width counted in characters, not bytes, so a
// column of names with Müller in it stays a column.
func padRunes(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		return string(r[:w-1]) + "…"
	}
	return s + strings.Repeat(" ", w-len(r))
}

func listOf(keys []string) string {
	if len(keys) > 4 {
		return strings.Join(keys[:4], ", ") + fmt.Sprintf(" +%d", len(keys)-4)
	}
	return strings.Join(keys, ", ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
