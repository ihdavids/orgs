//lint:file-ignore ST1006 allow the use of self
package orgs

// A file's history, for worg's Diff tab.
//
// The diffing itself happens in the browser (worg/src/orgdiff.ts): it has to
// re-run as the reader flips whitespace and context options, and the outline
// diff wants both texts anyway. All the server does is what the browser
// cannot: ask git which commits touched a file, and hand over the file as it
// was at one of them, in the index, or on disk.
//
// Only files the OrgDb knows can be asked about. A revision is a word handed
// to git, so it is checked against a narrow pattern first and never allowed to
// start with a dash, where git would read it as an option.

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The pseudo revisions: the file as it is on disk, and as it is staged.
const (
	revWorking = "WORKING"
	revIndex   = "INDEX"
)

type HistoryCommit struct {
	Hash    string
	Short   string
	Parents []string // rewritten to the commits that touched this file
	Author  string
	Email   string
	When    int64 // unix seconds
	Subject string
	Refs    []string // "HEAD -> main", "origin/main", "tag: v1"
	Added   int      // lines this commit added to the file
	Removed int
}

type HistoryResult struct {
	Ok     bool
	Msg    string
	File   string // as the OrgDb names it
	InRepo bool   // false: the file is not under git, and only WORKING exists
	Repo   string // the repository's top level
	Path   string // the file relative to Repo
	Branch string // "" when HEAD is detached
	Head   string
	// The newest commit reachable from HEAD that touched the file: what the
	// uncommitted changes sit on. Not always Commits[0], with all=1.
	Last string
	// What has not been committed: Untracked means git has never seen it;
	// Staged and Modified are the index against HEAD and the disk against the
	// index, as git status spells them, with the lines either way.
	Untracked   bool
	Staged      bool
	StagedAdd   int
	StagedDel   int
	Modified    bool
	ModifiedAdd int
	ModifiedDel int
	Commits     []HistoryCommit
	Truncated   bool // there are older commits than Limit
}

type FileAtRev struct {
	Ok      bool
	Msg     string
	Rev     string
	Text    string
	Missing bool // the file did not exist at that revision
}

var revPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/~^@{}-]{0,199}$`)

func validRev(rev string) bool {
	return revPattern.MatchString(rev) && !strings.Contains(rev, "..")
}

// git run in dir. Stderr is kept for the error, since git's own message
// ("fatal: bad revision") is the useful part.
func gitIn(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), errors.New(msg)
	}
	return out.String(), nil
}

// The repository a file lives in and its path there, or ok false.
func repoOf(file string) (repo, rel string, ok bool) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return "", "", false
	}
	// Symlinked directories (macOS's /tmp, a notes folder linked into home)
	// make the toplevel and the file disagree about their prefix.
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		abs = r
	}
	top, err := gitIn(filepath.Dir(abs), "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", false
	}
	repo = strings.TrimSpace(top)
	if r, err := filepath.EvalSymlinks(repo); err == nil {
		repo = r
	}
	rel, err = filepath.Rel(repo, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", "", false
	}
	return repo, filepath.ToSlash(rel), true
}

// "12\t3\tpath" -> 12, 3. Binary files say "-" and count as nothing.
func numstat(line string) (int, int, bool) {
	f := strings.SplitN(line, "\t", 3)
	if len(f) < 3 {
		return 0, 0, false
	}
	a, _ := strconv.Atoi(f[0])
	d, _ := strconv.Atoi(f[1])
	return a, d, true
}

func sumNumstat(out string) (int, int) {
	add, del := 0, 0
	for _, l := range strings.Split(out, "\n") {
		if a, d, ok := numstat(l); ok {
			add += a
			del += d
		}
	}
	return add, del
}

func fileHistory(file string, limit int, all bool) HistoryResult {
	res := HistoryResult{Ok: true, File: file}
	repo, rel, ok := repoOf(file)
	if !ok {
		return res
	}
	res.InRepo, res.Repo, res.Path = true, repo, rel

	if b, err := gitIn(repo, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil {
		res.Branch = strings.TrimSpace(b)
	}
	if h, err := gitIn(repo, "rev-parse", "--verify", "--quiet", "HEAD"); err == nil {
		res.Head = strings.TrimSpace(h)
	}

	if st, err := gitIn(repo, "status", "--porcelain=v1", "--", rel); err == nil {
		for _, l := range strings.Split(st, "\n") {
			if len(l) < 2 {
				continue
			}
			if l[:2] == "??" {
				res.Untracked = true
				continue
			}
			res.Staged = res.Staged || (l[0] != ' ' && l[0] != '?')
			res.Modified = res.Modified || (l[1] != ' ' && l[1] != '?')
		}
	}
	if res.Staged {
		out, _ := gitIn(repo, "diff", "--cached", "--numstat", "--", rel)
		res.StagedAdd, res.StagedDel = sumNumstat(out)
	}
	if res.Modified {
		out, _ := gitIn(repo, "diff", "--numstat", "--", rel)
		res.ModifiedAdd, res.ModifiedDel = sumNumstat(out)
	}
	if res.Head == "" {
		// A repository with no commits yet has nothing to log.
		return res
	}

	if l, err := gitIn(repo, "log", "-1", "--format=%H", "HEAD", "--", rel); err == nil {
		res.Last = strings.TrimSpace(l)
	}
	if limit <= 0 {
		limit = 300
	}
	// Each record starts with \x1e so the numstat lines git prints after it
	// stay with it; fields are split by \x1f, which no subject contains.
	args := []string{"log", "--date-order", "--parents", "--numstat",
		"--format=%x1e%H%x1f%P%x1f%an%x1f%ae%x1f%at%x1f%D%x1f%s",
		"-n", strconv.Itoa(limit + 1)}
	if all {
		args = append(args, "--all")
	}
	args = append(args, "--", rel)
	out, err := gitIn(repo, args...)
	if err != nil {
		res.Ok, res.Msg = false, err.Error()
		return res
	}
	res.Commits = parseHistory(out)
	if len(res.Commits) > limit {
		res.Commits = res.Commits[:limit]
		res.Truncated = true
	}
	return res
}

func parseHistory(out string) []HistoryCommit {
	commits := []HistoryCommit{}
	for _, rec := range strings.Split(out, "\x1e") {
		if strings.TrimSpace(rec) == "" {
			continue
		}
		lines := strings.Split(rec, "\n")
		f := strings.SplitN(lines[0], "\x1f", 7)
		if len(f) < 7 {
			continue
		}
		c := HistoryCommit{Hash: f[0], Author: f[2], Email: f[3], Subject: f[6]}
		if len(c.Hash) >= 7 {
			c.Short = c.Hash[:7]
		}
		// With --parents, %P is rewritten to the nearest ancestors that touched
		// the file, which is what makes the history drawable as a graph.
		c.Parents = strings.Fields(f[1])
		c.When, _ = strconv.ParseInt(f[4], 10, 64)
		for _, r := range strings.Split(f[5], ", ") {
			if r = strings.TrimSpace(r); r != "" {
				c.Refs = append(c.Refs, r)
			}
		}
		for _, l := range lines[1:] {
			if a, d, ok := numstat(l); ok {
				c.Added += a
				c.Removed += d
			}
		}
		commits = append(commits, c)
	}
	return commits
}

func fileAt(file, rev string) FileAtRev {
	res := FileAtRev{Ok: true, Rev: rev}
	if rev == "" || rev == revWorking {
		b, err := os.ReadFile(file)
		if err != nil {
			if os.IsNotExist(err) {
				res.Missing = true
				return res
			}
			return FileAtRev{Ok: false, Msg: err.Error(), Rev: rev}
		}
		res.Text = string(b)
		return res
	}
	repo, rel, ok := repoOf(file)
	if !ok {
		return FileAtRev{Ok: false, Msg: file + " is not in a git repository", Rev: rev}
	}
	spec := ":" + rel // the index
	if rev != revIndex {
		if !validRev(rev) {
			return FileAtRev{Ok: false, Msg: "not a revision: " + rev, Rev: rev}
		}
		spec = rev + ":" + rel
	}
	out, err := gitIn(repo, "show", spec)
	if err != nil {
		// A commit from before the file existed: not a failure, an empty side.
		if strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "exists on disk, but not in") {
			res.Missing = true
			return res
		}
		return FileAtRev{Ok: false, Msg: err.Error(), Rev: rev}
	}
	res.Text = out
	return res
}

// The OrgDb's name for a file, or "" when it is not one of the server's.
func historyFile(r *http.Request) string {
	name := r.URL.Query().Get("file")
	if name == "" {
		return ""
	}
	f := GetDb().FindByFile(name)
	if f == nil {
		return ""
	}
	return f.Filename
}

/* SDOC: API
* GET /history — A File's Git History

	Whether an org file is under git, what is uncommitted, and the commits
	that touched it, newest first, with parents rewritten to that history so
	a client can draw it as a graph. Used by worg's Diff tab.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                   |
	|-----------+--------+----------+-----------------------------------------------|
	| =file=    | string | yes      | An org file the server reads.                 |
	| =limit=   | int    | no       | How many commits at most (default 300).       |
	| =all=     | bool   | no       | Every branch, not just the one checked out.   |

	*Response:* =Ok=, =Msg=, =InRepo=, =Repo=, =Path=, =Branch=, =Head=, =Last= (the newest commit
	from HEAD that touched the file),
	=Untracked=, =Staged= / =StagedAdd= / =StagedDel=, =Modified= /
	=ModifiedAdd= / =ModifiedDel=, =Truncated=, and =Commits=: each with
	=Hash=, =Short=, =Parents=, =Author=, =Email=, =When= (unix seconds),
	=Subject=, =Refs=, and the lines it =Added= and =Removed= in this file.
	A file not under git answers =Ok= with =InRepo= false.
EDOC */
func RequestHistory(w http.ResponseWriter, r *http.Request) {
	file := historyFile(r)
	if file == "" {
		columnJson(w, HistoryResult{Ok: false, Msg: "no org file called " + r.URL.Query().Get("file")})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	all := r.URL.Query().Get("all")
	columnJson(w, fileHistory(file, limit, all == "1" || all == "true"))
}

/* SDOC: API
* GET /history/text — An Org File at a Revision

	The text of an org file at a git revision, as staged, or on disk.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                              |
	|-----------+--------+----------+----------------------------------------------------------|
	| =file=    | string | yes      | An org file the server reads.                            |
	| =rev=     | string | no       | A commit or ref, =INDEX=, or =WORKING= (the default).    |

	*Response:* =Ok=, =Msg=, =Rev=, =Text=, and =Missing= when the file did
	not exist at that revision (=Text= is then empty, and =Ok= still true).
EDOC */
func RequestHistoryText(w http.ResponseWriter, r *http.Request) {
	file := historyFile(r)
	if file == "" {
		columnJson(w, FileAtRev{Ok: false, Msg: "no org file called " + r.URL.Query().Get("file")})
		return
	}
	columnJson(w, fileAt(file, r.URL.Query().Get("rev")))
}
