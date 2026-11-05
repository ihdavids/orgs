package orgs

// org-edna, run: a heading's BLOCKER is asked before it may be marked done
// (ChangeStatus, beside enforceTodoDependencies, and the IsBlocked() query),
// and its TRIGGER is run after it has been. Only with features.edna on. The
// language is read by common.ParseEdna.
//
// Two things here are forced by the rest of the server:
//
//  1. **A target is held as file + outline path, never as a section or a
//     hash.** Every action is a write, and a write changes the hashes of
//     every heading after it (see Traps: hashes) and leaves the old sections
//     describing a file that is gone. So each action looks its target up
//     again just before it writes, the way Move does.
//  2. **A trigger can set off a trigger.** `todo!(DONE)` on a heading with a
//     TRIGGER of its own runs that one too, as edna does through org-todo,
//     to a depth of five, and never the same heading twice in one chain - a
//     cycle of triggers would otherwise write until the disk filled.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/orgdate"
)

const ednaMaxDepth = 5

// ednaRef is a heading as somewhere to find it again after a write.
type ednaRef struct {
	file     string
	olp      []string
	headline string
}

type ednaHit struct {
	f   *common.OrgFile
	sec *org.Section
}

func refOf(f *common.OrgFile, sec *org.Section) ednaRef {
	return ednaRef{file: f.Filename, olp: outlineOf(sec), headline: common.GetSectionTitle(sec)}
}

func (r ednaRef) key() string { return r.file + "\x00" + olpString(r.olp) }

func (r ednaRef) find() (*common.OrgFile, *org.Section) {
	f, sec := GetDb().GetFromTarget(&common.Target{Type: "file+olp", Filename: r.file, Id: olpString(r.olp)}, false)
	if f == nil || sec == nil || sec.Headline == nil || sec.Headline.Lvl == 0 {
		return nil, nil
	}
	return f, sec
}

// ednaPropOf is a heading's own TRIGGER or BLOCKER (edna does not inherit them).
func ednaPropOf(sec *org.Section, f *common.OrgFile, name string) string {
	if sec == nil || sec.Headline == nil {
		return ""
	}
	if sec.Headline.Properties != nil {
		if v, ok := sec.Headline.Properties.Get(name); ok {
			return strings.TrimSpace(v)
		}
	}
	if f != nil {
		if lines, from, end, ok := recordLines(f.Filename, sec); ok {
			for k, v := range propsFromLines(lines, from, end) {
				if strings.EqualFold(k, name) {
					return strings.TrimSpace(v)
				}
			}
		}
	}
	return ""
}

func isRealHeading(sec *org.Section) bool {
	return sec != nil && sec.Headline != nil && sec.Headline.Lvl > 0
}

// The headings directly under a section's parent, the top level for a
// top-level heading.
func siblingsOf(f *common.OrgFile, sec *org.Section) []*org.Section {
	if sec.Parent != nil {
		return sec.Parent.Children
	}
	if f != nil && f.Doc != nil && f.Doc.Outline.Section != nil {
		return f.Doc.Outline.Children
	}
	return nil
}

func indexIn(list []*org.Section, sec *org.Section) int {
	for i, s := range list {
		if s == sec {
			return i
		}
	}
	return -1
}

func ednaHasTag(sec *org.Section, tag string) bool {
	for _, t := range sec.Headline.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// ednaFind runs one finder from the heading it belongs to.
func ednaFind(f *common.OrgFile, self *org.Section, form common.EdnaForm) ([]ednaHit, error) {
	hits := []ednaHit{}
	add := func(file *common.OrgFile, secs ...*org.Section) {
		for _, s := range secs {
			if isRealHeading(s) {
				hits = append(hits, ednaHit{file, s})
			}
		}
	}
	switch form.Name {
	case "self":
		add(f, self)
	case "parent":
		add(f, self.Parent)
	case "children":
		add(f, self.Children...)
	case "first-child":
		if len(self.Children) > 0 {
			add(f, self.Children[0])
		}
	case "last-child":
		if len(self.Children) > 0 {
			add(f, self.Children[len(self.Children)-1])
		}
	case "descendants":
		var walk func(s *org.Section)
		walk = func(s *org.Section) {
			for _, c := range s.Children {
				add(f, c)
				walk(c)
			}
		}
		walk(self)
	case "ancestors":
		for p := self.Parent; isRealHeading(p); p = p.Parent {
			add(f, p)
		}
	case "siblings", "siblings-wrap", "rest-of-siblings", "next-sibling", "next-sibling-wrap", "previous-sibling":
		sibs := siblingsOf(f, self)
		at := indexIn(sibs, self)
		if at < 0 {
			break
		}
		switch form.Name {
		case "siblings":
			add(f, sibs[:at]...)
			add(f, sibs[at+1:]...)
		case "siblings-wrap":
			add(f, sibs[at+1:]...)
			add(f, sibs[:at]...)
		case "rest-of-siblings":
			add(f, sibs[at+1:]...)
		case "next-sibling":
			if at+1 < len(sibs) {
				add(f, sibs[at+1])
			}
		case "next-sibling-wrap":
			if at+1 < len(sibs) {
				add(f, sibs[at+1])
			} else if at > 0 {
				add(f, sibs[0])
			}
		case "previous-sibling":
			if at > 0 {
				add(f, sibs[at-1])
			}
		}
	case "ids":
		for _, id := range form.Args {
			id = strings.TrimPrefix(strings.TrimSpace(id), "id:")
			s := GetDb().FindByAnyId(id)
			if s == nil {
				return hits, fmt.Errorf("ids: no heading has the ID %q", id)
			}
			add(GetDb().FileFromSection(s), s)
		}
	case "match":
		q, ok := orgMatchToFull(form.Args[0])
		if !ok {
			q = form.Args[0]
		}
		todos, err := QueryStringTodos(&common.StringQuery{Query: q})
		if err != nil {
			return hits, fmt.Errorf("match(%q): %v", form.Args[0], err)
		}
		if todos != nil {
			for _, t := range *todos {
				if s := GetDb().FindByHash(string(t.Hash)); s != nil {
					add(GetDb().FileFromSection(s), s)
				}
			}
		}
	case "olp":
		file := GetDb().FindByFile(form.Args[0])
		if file == nil {
			return hits, fmt.Errorf("olp: no org file %q", form.Args[0])
		}
		path := strings.Join(strings.Split(strings.Trim(form.Args[1], "/"), "/"), "::")
		_, s := GetDb().GetFromTarget(&common.Target{Type: "file+olp", Filename: file.Filename, Id: path}, false)
		if s == nil {
			return hits, fmt.Errorf("olp: no heading %q in %s", form.Args[1], form.Args[0])
		}
		add(file, s)
	}
	if form.Name != "ids" && form.Name != "match" && form.Name != "olp" {
		hits = filterEdnaHits(hits, form.Args)
	}
	return hits, nil
}

func filterEdnaHits(hits []ednaHit, opts []string) []ednaHit {
	if len(opts) == 0 {
		return hits
	}
	out := []ednaHit{}
	for _, h := range hits {
		done, active := keywordSets(h.f)
		keep := true
		for _, o := range opts {
			switch strings.ToLower(o) {
			case "todo-only":
				keep = keep && active[h.sec.Headline.Status]
			case "todo-and-done-only":
				keep = keep && (active[h.sec.Headline.Status] || done[h.sec.Headline.Status])
			case "no-comment":
				keep = keep && !h.sec.Headline.IsComment
			case "no-archive":
				keep = keep && !ednaHasTag(h.sec, "ARCHIVE")
			}
		}
		if keep {
			out = append(out, h)
		}
	}
	return out
}

// ednaTest asks a condition of one heading.
func ednaTest(h ednaHit, form common.EdnaForm) (bool, error) {
	hl := h.sec.Headline
	var ok bool
	switch form.Name {
	case "done?":
		done, _ := keywordSets(h.f)
		ok = done[hl.Status]
	case "todo-state?":
		ok = strings.EqualFold(hl.Status, form.Args[0])
	case "has-property?":
		v := ""
		if hl.Properties != nil {
			v, _ = hl.Properties.Get(form.Args[0])
		}
		ok = strings.TrimSpace(v) == strings.TrimSpace(form.Args[1])
	case "has-tags?":
		for _, t := range form.Args {
			ok = ok || ednaHasTag(h.sec, t)
		}
	case "matches?":
		q, isMatch := orgMatchToFull(form.Args[0])
		if !isMatch {
			q = form.Args[0]
		}
		secs, err := QueryStringNodesOnFile(q, h.f)
		if err != nil {
			return false, fmt.Errorf("matches?(%q): %v", form.Args[0], err)
		}
		for _, s := range secs {
			ok = ok || s == h.sec
		}
	case "re-search?":
		re, err := regexp.Compile(form.Args[0])
		if err != nil {
			return false, fmt.Errorf("re-search?: %v", err)
		}
		if lines, from, _, found := recordLines(h.f.Filename, h.sec); found {
			ok = re.MatchString(strings.Join(lines[from:ownLinesEnd(lines, from, len(lines)-1)+1], "\n"))
		}
	}
	if form.Neg {
		ok = !ok
	}
	return ok, nil
}

// ednaConsidered is whether a condition holds over the headings found, under
// a consideration: any (edna's default), all, at least a count, or at least a
// fraction of them.
func ednaConsidered(hits []ednaHit, cond common.EdnaForm, consider string) (bool, []ednaHit, error) {
	if cond.Name == "headings?" {
		yes := len(hits) > 0
		if cond.Neg {
			yes = !yes
		}
		return yes, hits, nil
	}
	which := []ednaHit{}
	for _, h := range hits {
		ok, err := ednaTest(h, cond)
		if err != nil {
			return false, nil, err
		}
		if ok {
			which = append(which, h)
		}
	}
	switch c := strings.ToLower(strings.TrimSpace(consider)); {
	case c == "" || c == "any":
		return len(which) > 0, which, nil
	case c == "all":
		return len(hits) > 0 && len(which) == len(hits), which, nil
	case strings.Contains(c, "."):
		frac, err := strconv.ParseFloat(c, 64)
		if err != nil {
			return false, nil, fmt.Errorf("consider(%s): not a fraction", consider)
		}
		return len(hits) > 0 && float64(len(which))/float64(len(hits)) >= frac, which, nil
	default:
		n, err := strconv.Atoi(c)
		if err != nil {
			return false, nil, fmt.Errorf("consider(%s): any, all, a count or a fraction", consider)
		}
		return len(which) >= n, which, nil
	}
}

// ---------------------------------------------------------------------------
// BLOCKER
// ---------------------------------------------------------------------------

// ednaBlocker is why a heading's BLOCKER holds it back, or "". A BLOCKER
// that cannot be read holds it back too, saying so: a misspelt blocker that
// let everything through would be found out only once it mattered.
func ednaBlocker(f *common.OrgFile, sec *org.Section) string {
	text := ednaPropOf(sec, f, "BLOCKER")
	if text == "" {
		return ""
	}
	if err := common.ValidateEdna("BLOCKER", text); err != nil {
		return "its BLOCKER cannot be read: " + err.Error()
	}
	forms, _ := common.ParseEdna(text)
	hits := []ednaHit{}
	fresh := false
	consider := ""
	check := func(cond common.EdnaForm) (string, error) {
		blocked, which, err := ednaConsidered(hits, cond, consider)
		if err != nil || !blocked {
			return "", err
		}
		names := []string{}
		for _, h := range which {
			names = append(names, fmt.Sprintf("%q", strings.TrimSpace(headlineTitle(h.sec))))
		}
		switch {
		case cond.Name == "done?" && cond.Neg && len(names) > 0:
			return "waiting on " + strings.Join(names, ", "), nil
		case len(names) > 0:
			return fmt.Sprintf("its BLOCKER (%s) holds for %s", text, strings.Join(names, ", ")), nil
		}
		return fmt.Sprintf("its BLOCKER (%s) holds", text), nil
	}
	for _, form := range forms {
		switch form.Kind {
		case common.EdnaFinder:
			if fresh {
				hits, fresh = []ednaHit{}, false
			}
			more, err := ednaFind(f, sec, form)
			if err != nil {
				return "its BLOCKER cannot be followed: " + err.Error()
			}
			hits = append(hits, more...)
		case common.EdnaConsider:
			consider = form.Args[0]
		case common.EdnaCondition:
			why, err := check(form)
			if err != nil {
				return "its BLOCKER cannot be followed: " + err.Error()
			}
			if why != "" {
				return why
			}
			fresh, consider = true, ""
		}
	}
	if !fresh {
		// Finders with no condition after them: edna's default, !done?.
		why, err := check(common.EdnaForm{Kind: common.EdnaCondition, Name: "done?", Neg: true})
		if err != nil {
			return "its BLOCKER cannot be followed: " + err.Error()
		}
		return why
	}
	return ""
}

// ednaStatusBlocked is statusBlocked's edna half.
func ednaStatusBlocked(f *common.OrgFile, sec *org.Section, to string) string {
	if !featureOn("edna") || sec == nil || sec.Headline == nil {
		return ""
	}
	done, _ := keywordSets(f)
	if !done[to] || done[sec.Headline.Status] {
		return ""
	}
	why := ednaBlocker(f, sec)
	if why == "" {
		return ""
	}
	return fmt.Sprintf("%q cannot be %s yet: %s", strings.TrimSpace(headlineTitle(sec)), to, why)
}

// ---------------------------------------------------------------------------
// TRIGGER
// ---------------------------------------------------------------------------

// EdnaStep is one thing a trigger did, or would do.
type EdnaStep struct {
	Action   string
	Headline string
	Filename string
	Ok       bool
	Msg      string `json:",omitempty"`
}

type ednaRun struct {
	dry     bool
	now     time.Time
	steps   []EdnaStep
	seen    map[string]bool
	archive []ednaRef
}

// runEdnaTrigger runs the TRIGGER of the heading at ref, which has just been
// marked done. dry only says what it would do.
func runEdnaTrigger(ref ednaRef, dry bool) []EdnaStep {
	run := &ednaRun{dry: dry, now: time.Now(), seen: map[string]bool{}}
	run.trigger(ref, 0)
	if len(run.archive) > 0 && !dry {
		req := common.MoveRequest{Op: common.MoveArchive}
		for _, a := range run.archive {
			req.From = append(req.From, common.Target{Type: "file+olp", Filename: a.file, Id: olpString(a.olp)})
		}
		res, _ := Move(db, &req)
		for _, r := range res.Results {
			run.steps = append(run.steps, EdnaStep{Action: "archive!", Headline: r.Headline, Filename: r.Filename, Ok: r.Ok, Msg: r.Msg})
		}
	}
	return run.steps
}

func (run *ednaRun) say(action string, ref ednaRef, ok bool, msg string) {
	run.steps = append(run.steps, EdnaStep{Action: action, Headline: ref.headline, Filename: ref.file, Ok: ok, Msg: msg})
}

func (run *ednaRun) trigger(ref ednaRef, depth int) {
	if depth >= ednaMaxDepth || run.seen[ref.key()] {
		return
	}
	run.seen[ref.key()] = true
	f, sec := ref.find()
	if f == nil {
		return
	}
	text := ednaPropOf(sec, f, "TRIGGER")
	if text == "" {
		return
	}
	if err := common.ValidateEdna("TRIGGER", text); err != nil {
		run.say("TRIGGER", ref, false, "cannot be read: "+err.Error())
		return
	}
	forms, _ := common.ParseEdna(text)
	run.forms(ref, forms, depth)
}

// forms runs a stretch of a trigger. An if is run here by finding its then,
// else and endif and running the side that applies.
func (run *ednaRun) forms(self ednaRef, forms []common.EdnaForm, depth int) {
	targets := []ednaRef{}
	fresh := false
	for i := 0; i < len(forms); i++ {
		form := forms[i]
		f, sec := self.find()
		if f == nil {
			run.say(form.Name, self, false, "the heading the trigger belongs to has gone")
			return
		}
		switch form.Kind {
		case common.EdnaFinder:
			if fresh {
				targets, fresh = []ednaRef{}, false
			}
			hits, err := ednaFind(f, sec, form)
			if err != nil {
				run.say(form.Name, self, false, err.Error())
				return
			}
			for _, h := range hits {
				targets = append(targets, refOf(h.f, h.sec))
			}
		case common.EdnaAction:
			for _, t := range targets {
				run.act(self, t, form, depth)
			}
			fresh = true
		case common.EdnaKeyword:
			if form.Name != "if" {
				continue
			}
			then, els, end := ednaIfParts(forms, i)
			ok, err := run.ifHolds(self, forms[i+1:then])
			if err != nil {
				run.say("if", self, false, err.Error())
				return
			}
			switch {
			case ok:
				run.forms(self, forms[then+1:ifEnd(els, end)], depth)
			case els >= 0:
				run.forms(self, forms[els+1:end], depth)
			}
			i = end
			fresh = true
		}
	}
}

// Where a then's actions stop: its else, or its endif.
func ifEnd(els, end int) int {
	if els >= 0 {
		return els
	}
	return end
}

// ednaIfParts finds the then, else (-1 if none) and endif of the if at i.
func ednaIfParts(forms []common.EdnaForm, i int) (then, els, end int) {
	then, els, end = -1, -1, len(forms)-1
	depth := 0
	for j := i; j < len(forms); j++ {
		if forms[j].Kind != common.EdnaKeyword {
			continue
		}
		switch forms[j].Name {
		case "if":
			depth++
		case "then":
			if depth == 1 && then < 0 {
				then = j
			}
		case "else":
			if depth == 1 {
				els = j
			}
		case "endif":
			depth--
			if depth == 0 {
				return then, els, j
			}
		}
	}
	return then, els, end
}

// ifHolds is whether every condition between if and then holds, each over
// the headings found before it.
func (run *ednaRun) ifHolds(self ednaRef, forms []common.EdnaForm) (bool, error) {
	f, sec := self.find()
	hits := []ednaHit{}
	consider := ""
	fresh := false
	asked := false
	for _, form := range forms {
		switch form.Kind {
		case common.EdnaFinder:
			if fresh {
				hits, fresh = []ednaHit{}, false
			}
			more, err := ednaFind(f, sec, form)
			if err != nil {
				return false, err
			}
			hits = append(hits, more...)
		case common.EdnaConsider:
			consider = form.Args[0]
		case common.EdnaCondition:
			ok, _, err := ednaConsidered(hits, form, consider)
			if err != nil || !ok {
				return false, err
			}
			asked, fresh, consider = true, true, ""
		}
	}
	if !asked {
		return false, fmt.Errorf("if with no condition before its then")
	}
	return true, nil
}

// act does one action to one target, looking the target up afresh.
func (run *ednaRun) act(self, target ednaRef, form common.EdnaForm, depth int) {
	f, sec := target.find()
	if f == nil {
		run.say(form.Name, target, false, "could not find it again")
		return
	}
	describe := form.Name + "(" + strings.Join(form.Args, " ") + ")"
	if form.Name == "archive!" {
		describe = "archive!"
	}
	if run.dry {
		run.say(describe, target, true, "")
		return
	}
	err := run.write(self, target, f, sec, form, depth)
	if err != nil {
		run.say(describe, target, false, err.Error())
		return
	}
	if form.Name == "archive!" {
		return // said when the archive is done, at the end
	}
	run.say(describe, target, true, "")
	if form.Name == "todo!" {
		// That heading may have a trigger of its own.
		if done, _ := keywordSets(f); done[form.Args[0]] {
			run.trigger(target, depth+1)
		}
	}
}

func (run *ednaRun) write(self, target ednaRef, f *common.OrgFile, sec *org.Section, form common.EdnaForm, depth int) error {
	switch form.Name {
	case "todo!":
		kw := form.Args[0]
		if !keywordKnown(f, kw) {
			return fmt.Errorf("%s is not a keyword %s allows", kw, f.Filename)
		}
		_, err := applyStatusChange(f, sec, kw, "", run.now)
		return err
	case "scheduled!", "deadline!":
		name := map[string]string{"scheduled!": "SCHEDULED", "deadline!": "DEADLINE"}[form.Name]
		stamp, err := run.ednaStamp(self, sec, name, form.Args[0])
		if err != nil {
			return err
		}
		lines, from, _, ok := recordLines(f.Filename, sec)
		if !ok {
			return fmt.Errorf("could not read %s", f.Filename)
		}
		out, err := changeDateLines(lines, from, sec.Headline.Lvl, name, stamp)
		if err != nil {
			return err
		}
		return writeLines(f.Filename, out)
	case "tag!":
		tags := []string{}
		for _, t := range strings.Split(form.Args[0], ":") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
		return run.headline(f, sec, nil, &tags)
	case "set-priority!":
		p := ednaPriority(sec.Headline.Priority, form.Args[0])
		return run.headline(f, sec, &p, nil)
	case "set-property!":
		return setHeadingProperty(f, sec, form.Args[0], form.Args[1])
	case "delete-property!":
		return setHeadingProperty(f, sec, form.Args[0], "")
	case "set-effort!":
		return setHeadingProperty(f, sec, "EFFORT", form.Args[0])
	case "chain!":
		sf, ssec := self.find()
		v := ednaPropOf(ssec, sf, form.Args[0])
		return setHeadingProperty(f, sec, form.Args[0], v)
	case "archive!":
		// Done last, all together: an archive takes the heading out of the
		// file, and anything after it in this trigger would look for it.
		run.archive = append(run.archive, target)
		return nil
	}
	return fmt.Errorf("%s is not something orgs can do", form.Name)
}

func keywordKnown(f *common.OrgFile, kw string) bool {
	done, active := keywordSets(f)
	return done[kw] || active[kw]
}

func (run *ednaRun) headline(f *common.OrgFile, sec *org.Section, prio *string, tags *[]string) error {
	lines, from, _, ok := recordLines(f.Filename, sec)
	if !ok {
		return fmt.Errorf("could not read %s", f.Filename)
	}
	lines[from] = setHeadlineParts(lines[from], sec.Headline.Status, prio, tags)
	return writeLines(f.Filename, lines)
}

// ednaPriority is set-priority!'s argument applied to the priority there now.
func ednaPriority(cur, arg string) string {
	const order = "ABC"
	at := strings.Index(order, strings.ToUpper(cur))
	switch strings.ToLower(arg) {
	case "remove", "rm", "nil":
		return ""
	case "up":
		if at < 0 {
			return "B"
		}
		if at > 0 {
			at--
		}
		return order[at : at+1]
	case "down":
		if at < 0 {
			return "C"
		}
		if at < len(order)-1 {
			at++
		}
		return order[at : at+1]
	}
	return strings.ToUpper(arg)
}

var ednaStepRe = regexp.MustCompile(`^(\+\+|\+|-|--)(\d+)([hdwmy])$`)

// ednaStamp is the stamp scheduled!/deadline! writes: "" to remove.
//
//	"+2d"  two days after the date it has now (or today, with none)
//	"++2d" two days from now
//	"-1w"  a week before the date it has now
//	"cp"   the trigger's own heading's date
//	"rm"   no date
//	anything else is read as a date is typed (orgs sched): mon, tomorrow 10:00, 2026-12-01
func (run *ednaRun) ednaStamp(self ednaRef, sec *org.Section, name, arg string) (string, error) {
	arg = strings.TrimSpace(arg)
	sdc := func(s *org.Section) *org.SDC {
		if name == "SCHEDULED" {
			return s.Headline.Scheduled
		}
		return s.Headline.Deadline
	}
	switch strings.ToLower(arg) {
	case "rm", "remove":
		return "", nil
	case "cp", "copy":
		_, ssec := self.find()
		if ssec == nil {
			return "", fmt.Errorf("cp: the trigger's heading has gone")
		}
		if d := sdc(ssec); d != nil && d.Date != nil {
			return d.Date.ToString(), nil
		}
		return "", fmt.Errorf("cp: %q has no %s to copy", self.headline, name)
	}
	if m := ednaStepRe.FindStringSubmatch(arg); m != nil {
		n, _ := strconv.Atoi(m[2])
		if strings.HasPrefix(m[1], "-") {
			n = -n
		}
		base, haveTime := run.now, false
		if d := sdc(sec); d != nil && d.Date != nil && m[1] != "++" && m[1] != "--" {
			base, haveTime = d.Date.Start, d.Date.HaveTime
		} else if m[3] == "h" {
			haveTime = true
		}
		switch m[3] {
		case "h":
			base = base.Add(time.Duration(n) * time.Hour)
		case "d":
			base = base.AddDate(0, 0, n)
		case "w":
			base = base.AddDate(0, 0, 7*n)
		case "m":
			base = base.AddDate(0, n, 0)
		case "y":
			base = base.AddDate(n, 0, 0)
		}
		layout := "2006-01-02 Mon"
		if haveTime {
			layout += " 15:04"
		}
		return "<" + base.Format(layout) + ">", nil
	}
	stamp, err := orgdate.ParseDateToOrg(arg, run.now)
	if err != nil {
		return "", fmt.Errorf("%s(%q): %v", strings.ToLower(name), arg, err)
	}
	return stamp, nil
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

// EdnaExplain is what /edna/explain answers.
type EdnaExplain struct {
	Ok           bool
	Off          bool `json:",omitempty"`
	Msg          string `json:",omitempty"`
	Headline     string
	Trigger      string
	TriggerError string `json:",omitempty"`
	// What marking it done would do, worked out without doing it.
	Steps        []EdnaStep
	Blocker      string
	BlockerError string `json:",omitempty"`
	Blocked      string `json:",omitempty"`
}

/* SDOC: API
* POST /edna/explain — What A Heading's TRIGGER And BLOCKER Would Do

	#+BEGIN_EXAMPLE
	POST /edna/explain   {"Hash": "..."}
	-> {"Ok": true, "Headline": "Write draft", "Trigger": "next-sibling todo!(NEXT)",
	    "Steps": [{"Action": "todo!(NEXT)", "Headline": "Review draft", "Ok": true}],
	    "Blocker": "previous-sibling", "Blocked": "waiting on \"Outline\""}
	#+END_EXAMPLE

	Nothing is written: =Steps= is what marking the heading done would do
	now. =Blocked= is why it cannot be done yet, empty when it can.
	Needs =features: edna:=.

* POST /edna/check — Read A TRIGGER Or BLOCKER

	#+BEGIN_EXAMPLE
	POST /edna/check   {"Property": "BLOCKER", "Text": "ids(x) !done?"}
	-> {"Ok": true, "Forms": [...]}  or  {"Ok": false, "Msg": "...", "Start": 0, "End": 6}
	#+END_EXAMPLE

	Works whether or not edna is on, so an editor can point at a misspelt one.
EDOC */
func PostEdnaExplain(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "edna") {
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct{ Hash string }
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, EdnaExplain{Msg: err.Error()})
		return
	}
	sec := GetDb().FindByHash(req.Hash)
	if sec == nil || sec.Headline == nil {
		dynJson(w, EdnaExplain{Msg: "no heading with that hash - it may have moved; refresh and try again"})
		return
	}
	f := GetDb().FileFromSection(sec)
	if f == nil {
		dynJson(w, EdnaExplain{Msg: "could not find the file that heading is in"})
		return
	}
	out := EdnaExplain{Ok: true, Headline: strings.TrimSpace(headlineTitle(sec)), Steps: []EdnaStep{},
		Trigger: ednaPropOf(sec, f, "TRIGGER"), Blocker: ednaPropOf(sec, f, "BLOCKER")}
	if out.Trigger != "" {
		if err := common.ValidateEdna("TRIGGER", out.Trigger); err != nil {
			out.TriggerError = err.Error()
		} else {
			out.Steps = runEdnaTrigger(refOf(f, sec), true)
		}
	}
	if out.Blocker != "" {
		if err := common.ValidateEdna("BLOCKER", out.Blocker); err != nil {
			out.BlockerError = err.Error()
		}
		out.Blocked = ednaBlocker(f, sec)
	}
	dynJson(w, out)
}

func PostEdnaCheck(w http.ResponseWriter, r *http.Request) {
	type answer struct {
		Ok    bool
		Msg   string `json:",omitempty"`
		Start int
		End   int
		Forms []common.EdnaForm
	}
	body, _ := io.ReadAll(r.Body)
	var req struct{ Property, Text string }
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, answer{Msg: err.Error(), Forms: []common.EdnaForm{}})
		return
	}
	if req.Property == "" {
		req.Property = "TRIGGER"
	}
	forms, _ := common.ParseEdna(req.Text)
	if err := common.ValidateEdna(req.Property, req.Text); err != nil {
		a := answer{Msg: err.Error(), Forms: forms}
		if e, ok := err.(*common.EdnaError); ok {
			a.Start, a.End = e.Start, e.End
		}
		dynJson(w, a)
		return
	}
	dynJson(w, answer{Ok: true, Forms: forms})
}
