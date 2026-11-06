package orgpkgs

// org-review from the command line (features.review).
//
//	orgs reviews                        what is due for review, most overdue first
//	orgs reviews -within 7              and what falls due in the next week
//	orgs reviews -all                   every heading in review
//	orgs reviews walk                   one at a time: r reviewed, s snooze, c cadence, n next, u undo, o open
//	orgs reviews done 2                 mark the second one reviewed (-note 'why', -date yesterday)
//	orgs reviews done 'Project X'       by its title, a query or a hash
//	orgs reviews set 'Project X' +2w    its cadence (REVIEW_DELAY); -next fri, -clear takes it out
//	orgs reviews snooze 'Project X' +1w put it off (NEXT_REVIEW only)
//
// Not `orgs review`: that is the weekly review wizard, which also offers what
// is due here (and its walk is the same walker, through /review/act, so a
// reviewed heading can be undone with u).

import (
	"flag"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/cmd/oc/commands/triage"
	"github.com/ihdavids/orgs/internal/common"
)

type Reviews struct {
	base
	tf     commands.TargetFlags
	Within int
	All    bool
	Note   string
	Date   string
	Next   string
	Clear  bool
}

func (self *Reviews) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.IntVar(&self.Within, "within", 0, "also list what falls due in the next N days")
	// Before the target flags, which skip a name already taken: -all lists
	// every heading in review, and for done/set/snooze is -all as they mean it.
	fset.BoolVar(&self.All, "all", false, "list every heading in review, due or not; with done/set/snooze, act on every match")
	fset.StringVar(&self.Note, "note", "", "done: a note to log with the review")
	fset.StringVar(&self.Date, "date", "", "done: the day it was reviewed (default today; yesterday, fri, 2026-11-02)")
	fset.StringVar(&self.Next, "next", "", "set: when it is next due (fri, +3w, 2026-12-01; - takes NEXT_REVIEW off)")
	fset.BoolVar(&self.Clear, "clear", false, "set: take the heading out of review altogether")
	commands.AddTargetFlags(fset, &self.tf)
}

type reviewList struct {
	offAnswer
	Today string
	Due   int
	Items []common.OrgReviewItem
}

type reviewResult struct {
	offAnswer
	Hash string
	Item *common.OrgReviewItem
}

var reviewSpanArg = regexp.MustCompile(`^\+?\d+[dwmy]?$`)

func (self *Reviews) list(core *commands.Core, within int, all bool) reviewList {
	params := map[string]string{}
	if within > 0 {
		params["within"] = strconv.Itoa(within)
	}
	if all {
		params["all"] = "t"
	}
	res, err := commands.SendReceiveGetErr[reviewList](core, "reviews", params)
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(res.offAnswer)
	return res
}

func reviewLine(n int, it common.OrgReviewItem) string {
	col := commands.AnsiGold
	if it.Late > 7 || it.Due == "" {
		col = commands.AnsiRed
	} else if !it.IsDue {
		col = commands.AnsiDim
	}
	where := commands.BaseName(it.Filename)
	if len(it.Olp) > 0 {
		where += " › " + strings.Join(it.Olp, " › ")
	}
	return fmt.Sprintf("%3d  %s  %s%s%s", n, it.Headline, commands.C(col), it.Why, commands.C(commands.AnsiReset)) +
		"  " + dim(where+":"+strconv.Itoa(it.LineNum+1))
}

// targets is the headings words name: a number from the list, a title in it,
// else whatever a query or hash finds.
func (self *Reviews) targets(core *commands.Core, words []string) []common.Todo {
	if self.tf.Hash == "" && len(words) > 0 {
		list := self.list(core, self.Within, true)
		due := self.list(core, self.Within, false).Items
		if n, err := strconv.Atoi(strings.Join(words, " ")); err == nil {
			if n < 1 || n > len(due) {
				commands.Fail("there are %d due for review; %d is not one of them", len(due), n)
			}
			return []common.Todo{due[n-1].Todo}
		}
		want := strings.ToLower(strings.Join(words, " "))
		var hits []common.Todo
		for _, it := range list.Items {
			if strings.ToLower(it.Headline) == want {
				hits = append(hits, it.Todo)
			}
		}
		if len(hits) == 0 {
			for _, it := range list.Items {
				if strings.Contains(strings.ToLower(it.Headline), want) {
					hits = append(hits, it.Todo)
				}
			}
		}
		if len(hits) == 1 {
			return hits
		}
	}
	return commands.Resolve(core, &self.tf, words, commands.TargetOpts{Prompt: "Review> ", Query: "HasReview()"})
}

func reviewPost[R any](core *commands.Core, path string, req *R) string {
	var res reviewResult
	commands.SendReceivePost(core, path, req, &res)
	if commands.DryRun {
		return ""
	}
	failIfOff(res.offAnswer)
	line := res.Msg
	if res.Item != nil && res.Item.Why != "" {
		line += dim(" · " + res.Item.Why)
	}
	return line
}

func (self *Reviews) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	self.tf.All = self.All
	verb := ""
	if len(words) > 0 {
		switch words[0] {
		case "done", "mark", "reviewed", "set", "every", "snooze", "walk", "ls", "list":
			verb, words = words[0], words[1:]
		}
	}
	switch verb {
	case "", "ls", "list":
		res := self.list(core, self.Within, self.All)
		commands.Render(res.Items, func() {
			for i, it := range res.Items {
				fmt.Println(reviewLine(i+1, it))
			}
			switch {
			case len(res.Items) == 0:
				fmt.Println("nothing is due for review")
			case res.Due > 0:
				fmt.Println(dim(fmt.Sprintf("%d due · orgs reviews done N marks one reviewed · orgs reviews walk goes through them", res.Due)))
			}
		})
	case "done", "mark", "reviewed":
		lines := []string{}
		for _, t := range self.targets(core, words) {
			if l := reviewPost(core, "reviews/mark", &common.OrgReviewMark{Hash: t.Hash, Date: self.Date, Note: self.Note}); l != "" {
				lines = append(lines, l)
			}
		}
		if !commands.DryRun {
			commands.Ok(len(lines), "reviewed", lines)
		}
	case "set", "every":
		req := common.OrgReviewSet{Clear: self.Clear}
		if n := len(words); n > 0 && reviewSpanArg.MatchString(words[n-1]) {
			d := words[n-1]
			req.Delay, words = &d, words[:n-1]
		}
		if flagSet(self.fset, "next") {
			v := self.Next
			if v == "-" {
				v = ""
			}
			req.Next = &v
		}
		if req.Delay == nil && req.Next == nil && !req.Clear {
			commands.Fail("orgs reviews set HEADING +2w   (or -next fri, or -clear)")
		}
		lines := []string{}
		for _, t := range self.targets(core, words) {
			req.Hash = t.Hash
			if l := reviewPost(core, "reviews/set", &req); l != "" {
				lines = append(lines, l)
			}
		}
		if !commands.DryRun {
			commands.Ok(len(lines), "set", lines)
		}
	case "snooze":
		until := ""
		if n := len(words); n > 0 && reviewSpanArg.MatchString(words[n-1]) {
			until, words = words[n-1], words[:n-1]
		}
		lines := []string{}
		for _, t := range self.targets(core, words) {
			if l := reviewPost(core, "reviews/snooze", &common.OrgReviewSnooze{Hash: t.Hash, Until: until}); l != "" {
				lines = append(lines, l)
			}
		}
		if !commands.DryRun {
			commands.Ok(len(lines), "snoozed", lines)
		}
	case "walk":
		if commands.Machine() || !commands.Interactive() {
			commands.Fail("orgs reviews walk needs a terminal; orgs reviews -json lists them")
		}
		self.list(core, 0, false) // says so when the feature is off
		w := triage.NewWalker(core, nil)
		w.Walk(ReviewDeck(core))
		w.Summary()
	}
}

// ReviewDeck walks what is due for review with the triage walker; every
// decision is a /review/act action, so u undoes it.
func ReviewDeck(core *commands.Core) triage.Deck {
	return triage.Deck{
		Title: "Due for review",
		Fetch: func() ([]common.ReviewItem, error) {
			res, err := commands.SendReceiveGetErr[reviewList](core, "reviews", nil)
			if err != nil {
				return nil, err
			}
			if !res.Ok {
				return nil, fmt.Errorf("%s", res.Msg)
			}
			out := []common.ReviewItem{}
			for _, it := range res.Items {
				out = append(out, it.ReviewItem)
			}
			return out, nil
		},
		Extra: func(it common.ReviewItem) []string {
			return []string{commands.C(commands.AnsiGold) + it.SinceFrom + commands.C(commands.AnsiReset)}
		},
		Empty: "Nothing is due for review.",
		Keys: []triage.Key{
			{Key: "r", Label: "reviewed", Do: func(w *triage.Walker, it common.ReviewItem) triage.Outcome {
				return w.Act(it, common.ReviewAct{Action: "reviewed"}, true)
			}},
			{Key: "s", Label: "snooze", Do: func(w *triage.Walker, it common.ReviewItem) triage.Outcome {
				when := w.Ask("again on:", "+1w, +3d, fri, 2026-12-01  ·  enter cancels")
				if when == "" {
					return triage.Stay
				}
				return w.Act(it, common.ReviewAct{Action: "review-snooze", When: when}, true)
			}},
			{Key: "c", Label: "cadence", Do: func(w *triage.Walker, it common.ReviewItem) triage.Outcome {
				every := w.Ask("review every:", "+1w, +2w, +1m, +3m, +1y  ·  enter cancels")
				if every == "" {
					return triage.Stay
				}
				return w.Act(it, common.ReviewAct{Action: "review-every", Text: every}, true)
			}},
		},
	}
}

func init() {
	commands.AddCmd("reviews", "headings due to be read again (org-review): list, done N, set HEADING +2w, snooze, walk",
		func() commands.Cmd { return &Reviews{} })
}
