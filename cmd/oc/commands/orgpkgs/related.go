package orgpkgs

// Related notes (features.related), from the command line.
//
//	orgs related 'Upgrading the cluster'   the notes most like a heading (a query or a hash)
//	orgs related ops.org                    like a whole file (-line 12: the heading on line 12)
//	orgs related ops.org -link 2            link the second one from it ([[id:…]] in a Related: list)
//	orgs related -engine embeddings ...     words or embeddings (default: the server's)
//	orgs related -linked ...                keep results already linked, marked
//	orgs related text 'some words'          like some text (or text on stdin)
//	orgs related pairs                      alike, and not linked, across everything
//	orgs related status                     words indexed, embeddings progress
//	orgs related reindex [-embeddings]      read everything again (-embeddings: ask for every vector again)
//
// Needs `orgs features related on`.

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Related struct {
	base
	tf         commands.TargetFlags
	N          int
	Engine     string
	Linked     bool
	Link       int
	Line       int
	Embeddings bool
}

func (self *Related) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.IntVar(&self.N, "n", 10, "how many")
	fset.StringVar(&self.Engine, "engine", "", "words or embeddings (default: the server's related.engine)")
	fset.BoolVar(&self.Linked, "linked", false, "keep notes a link already joins to it, marked linked")
	fset.IntVar(&self.Link, "link", 0, "link the Nth result from the heading (or file), as [[id:…][Title]] in a Related: list")
	fset.IntVar(&self.Line, "line", 0, "with a file: the heading on this line (from 1)")
	fset.BoolVar(&self.Embeddings, "embeddings", false, "reindex: forget every vector and ask for them again")
	commands.AddTargetFlags(fset, &self.tf)
}

type relatedResult = common.RelatedResult
type relatedStatus = common.RelatedStatus

func relFail(ok bool, msg string) {
	if !ok {
		commands.Fail("%s", msg)
	}
}

func relWhere(it common.RelatedItem) string {
	where := commands.BaseName(it.Filename)
	if len(it.Olp) > 0 {
		where += " › " + strings.Join(it.Olp, " › ")
	}
	if it.Line >= 0 {
		where += ":" + strconv.Itoa(it.Line+1)
	}
	return where
}

func printRelated(res common.RelatedResult) {
	t := res.Target
	what := t.Headline
	if what == "" {
		what = commands.BaseName(t.Filename)
	}
	fmt.Printf("%s %s %s\n", dim("like"), commands.C(commands.AnsiBold)+what+commands.C(commands.AnsiReset),
		dim(fmt.Sprintf("(%s, %d words)", res.Engine, t.Words)))
	if len(res.Items) == 0 {
		fmt.Println("nothing like it")
	}
	for i, it := range res.Items {
		title := it.Headline
		if it.Kind == "file" {
			title = commands.BaseName(it.Filename) + " (the file)"
		}
		mark := ""
		if it.Linked {
			mark = commands.C(commands.AnsiCyan) + "  linked" + commands.C(commands.AnsiReset)
		}
		fmt.Printf("%3d  %s%.2f%s  %s  %s%s\n", i+1, commands.C(commands.AnsiGold), it.Score, commands.C(commands.AnsiReset),
			title, dim(relWhere(it)), mark)
		why := strings.Join(it.Terms, ", ")
		if it.Snippet != "" {
			if why != "" {
				why += " — "
			}
			why += "“" + it.Snippet + "”"
		}
		if why != "" {
			fmt.Println("       " + dim(why))
		}
	}
	if res.Skipped > 0 {
		fmt.Println(dim(fmt.Sprintf("%d already linked, left out (-linked keeps them)", res.Skipped)))
	}
	if res.Msg != "" {
		fmt.Println(dim(res.Msg))
	}
}

func looksLikeFile(w string) bool {
	if strings.HasSuffix(strings.ToLower(w), ".org") {
		return true
	}
	if st, err := os.Stat(w); err == nil && !st.IsDir() {
		return true
	}
	return false
}

func (self *Related) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 && self.tf.Hash == "" {
		switch words[0] {
		case "status", "reindex", "pairs", "text":
			verb, words = words[0], words[1:]
		}
	}
	switch verb {
	case "status":
		res, err := commands.SendReceiveGetErr[relatedStatus](core, "related/status", nil)
		if err != nil {
			commands.Fail("%v", err)
		}
		relFail(res.Ok, res.Msg)
		self.status(res)
		return
	case "reindex":
		req := struct{ Embeddings bool }{self.Embeddings}
		res, err := commands.SendReceivePostErr[struct{ Embeddings bool }, relatedStatus](core, "related/reindex", &req)
		if err == commands.ErrDryRun {
			return
		}
		if err != nil {
			commands.Fail("%v", err)
		}
		relFail(res.Ok, res.Msg)
		self.status(res)
		return
	case "pairs":
		type pairsAnswer = common.RelatedPairs
		res, err := commands.SendReceiveGetErr[pairsAnswer](core, "related/pairs", map[string]string{"n": strconv.Itoa(self.N * 2)})
		if err != nil {
			commands.Fail("%v", err)
		}
		relFail(res.Ok, res.Msg)
		commands.Render(res.Pairs, func() {
			if len(res.Pairs) == 0 {
				fmt.Println("no pairs alike enough")
			}
			for i, p := range res.Pairs {
				fmt.Printf("%3d  %s%.2f%s  %s  %s\n", i+1, commands.C(commands.AnsiGold), p.Score, commands.C(commands.AnsiReset), p.A.Headline, dim(relWhere(p.A)))
				fmt.Printf("     %s  %s  %s\n", dim("  ↔ "), p.B.Headline, dim(relWhere(p.B)))
				if len(p.B.Terms) > 0 {
					fmt.Println("           " + dim(strings.Join(p.B.Terms, ", ")))
				}
			}
		})
		return
	case "text":
		text := strings.Join(words, " ")
		if text == "" || text == "-" {
			b, _ := io.ReadAll(os.Stdin)
			text = string(b)
		}
		req := common.RelatedTextRequest{Text: text, N: self.N, Engine: self.Engine, Linked: self.Linked, Line: -1}
		res, err := commands.SendReceivePostErr[common.RelatedTextRequest, relatedResult](core, "related/text", &req)
		if err != nil {
			commands.Fail("%v", err)
		}
		relFail(res.Ok, res.Msg)
		self.show(res)
		return
	}

	// A heading or a file.
	params := map[string]string{"n": strconv.Itoa(self.N)}
	if self.Engine != "" {
		params["engine"] = self.Engine
	}
	if self.Linked || self.Link > 0 {
		// Numbers given to -link count what is shown, so linked ones stay
		// out unless asked for.
		if self.Linked {
			params["linked"] = "1"
		}
	}
	var linkFrom common.RelatedLinkRequest
	if self.tf.Hash == "" && len(words) == 1 && looksLikeFile(words[0]) {
		file := words[0]
		if abs, err := filepath.Abs(file); err == nil {
			if _, err := os.Stat(abs); err == nil {
				file = abs
			}
		}
		params["file"] = file
		linkFrom.Filename = file
		if self.Line > 0 {
			params["line"] = strconv.Itoa(self.Line - 1)
			l := self.Line - 1
			linkFrom.Line = &l
		}
	} else {
		found := commands.Resolve(core, &self.tf, words, commands.TargetOpts{Prompt: "Related to> ", Quiet: true})
		if len(found) != 1 {
			commands.Fail("name one heading (found %d)", len(found))
		}
		params["hash"] = found[0].Hash
		linkFrom.Hash = found[0].Hash
	}
	res, err := commands.SendReceiveGetErr[relatedResult](core, "related", params)
	if err != nil {
		commands.Fail("%v", err)
	}
	relFail(res.Ok, res.Msg)
	if self.Link > 0 {
		if self.Link > len(res.Items) {
			commands.Fail("there are %d related notes; %d is not one of them", len(res.Items), self.Link)
		}
		it := res.Items[self.Link-1]
		req := linkFrom
		if it.Hash != "" {
			req.TargetHash = it.Hash
		} else {
			req.TargetFile = it.Filename
		}
		var lr common.RelatedLinkResult
		commands.SendReceivePost(core, "related/link", &req, &lr)
		if commands.DryRun {
			return
		}
		relFail(lr.Ok, lr.Msg)
		if commands.RenderOne(lr, nil) {
			return
		}
		fmt.Printf("%s  %s\n", lr.Msg, dim(lr.Link+"  "+commands.BaseName(lr.Filename)+":"+strconv.Itoa(lr.Line+1)))
		return
	}
	self.show(res)
}

func (self *Related) show(res common.RelatedResult) {
	if commands.JsonOut {
		commands.RenderOne(res, nil)
		return
	}
	commands.Render(res.Items, func() { printRelated(res) })
}

func (self *Related) status(st common.RelatedStatus) {
	commands.RenderOne(st, func() {
		if st.Msg != "" && st.Files > 0 && strings.HasPrefix(st.Msg, "read ") {
			fmt.Println(st.Msg)
		}
		fmt.Printf("words       %d headings in %d files, %d distinct words\n", st.Chunks, st.Files, st.Terms)
		e := st.Embeddings
		if !e.Configured {
			fmt.Println("embeddings  " + dim("not configured (related.embeddings.backend: ollama, local, openai or gemini)"))
		} else {
			pct := 0
			if e.Total > 0 {
				pct = e.Done * 100 / e.Total
			}
			fmt.Printf("embeddings  %s %s: %d of %d (%d%%), %s", e.Backend, e.Model, e.Done, e.Total, pct, e.State)
			if e.Msg != "" {
				fmt.Print(" - " + e.Msg)
			}
			fmt.Println()
			if e.LastError != "" {
				fmt.Println("            " + commands.C(commands.AnsiRed) + e.LastError + commands.C(commands.AnsiReset))
			}
			fmt.Println("            " + dim(e.Store))
		}
		fmt.Printf("answers by  %s\n", st.Engine)
		if st.Msg != "" && !strings.HasPrefix(st.Msg, "read ") {
			fmt.Println(dim(st.Msg))
		}
	})
}

func init() {
	commands.AddCmd("related", "notes like a heading or file: by words or embeddings; -link N, status, reindex, pairs, text",
		func() commands.Cmd { return &Related{} })
}
