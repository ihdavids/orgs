package orgs

/* SDOC: Editing
* Babel: Python Sessions

  =:session= keeps one python running for a file, so that a block can use
  what an earlier block made - the way the cells of a notebook share a kernel:

  #+BEGIN_EXAMPLE
  #+PROPERTY: header-args:python :session analysis :results output

  #+BEGIN_SRC python
  import statistics
  xs = [3, 1, 4, 1, 5]
  #+END_SRC

  #+BEGIN_SRC python
  statistics.mean(xs)
  #+END_SRC

  #+RESULTS:
  : 2.8
  #+END_EXAMPLE

  Only python has sessions; a =:session= on any other language is ignored
  and each block runs on its own, as before. =:session none=, or no
  =:session= at all, is a fresh python for every block.

  - A session belongs to its name and its org file. Two files with
    =:session main= have one each.
  - Like a notebook, the value of a block's last line is shown after what it
    printed (=:results output=), or is the whole result (=:results value=). A
    line ending in =;= is not shown.
  - A matplotlib figure the block drew is saved where =:file= says (further
    figures get =-2=, =-3=...), or under =<file>_files/= when it says
    nowhere, and linked in the result.
  - A pandas table that is the block's value comes back as an org table.
  - An exception is written as the result, the traceback in place of the
    output, as a notebook shows it. The run still reports that it failed.
  - =!ls=, =%pip install=, =%matplotlib inline=, =%%bash= and their
    commonest relatives work, so a cell pasted from Jupyter runs.
  - A block that runs past =babel.timeout= is interrupted (a
    =KeyboardInterrupt=, which keeps the session); one that ignores that is
    killed, and the session starts again next time.
  - A session left alone for an hour stops, and every session stops with the
    server. =POST /babel/sessions/stop= stops one now, which is how a
    notebook is run again from the top.
  - =:python /path/to/venv/bin/python= picks the interpreter, as in org;
    =babel.commands.python= does it for every file.
EDOC */

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common"
)

// How long a session may sit unused before it is stopped. An hour is longer
// than any pause in working through a notebook and shorter than leaving a
// dataframe of somebody's in memory overnight.
const babelSessionIdle = time.Hour

// How long an interrupted block is given to stop before its python is killed.
const babelInterruptGrace = 5 * time.Second

// The python that holds a session. It reads requests on fd 3 and answers on
// fd 4, each a length on a line followed by that many bytes of json - framed,
// so that nothing the code prints can be mistaken for an answer, and nothing
// has to be scraped off a prompt. While a block runs, fd 1 and fd 2 point at
// one temporary file, so what the code prints, what a subprocess it starts
// prints, and its warnings all land in it in the order they happened.
const pythonSessionDriver = `
import sys, os, io, ast, json, re, linecache, tempfile, traceback, warnings, subprocess

REQ = os.fdopen(3, 'rb', buffering=0)
REP = os.fdopen(4, 'wb', buffering=0)
DRIVER = sys._getframe().f_code.co_filename
G = {'__name__': '__main__', '__builtins__': __builtins__}
warnings.filterwarnings('ignore', message='.*non-interactive.*')
warnings.filterwarnings('ignore', message='.*FigureCanvasAgg.*')

def _read(n):
    buf = b''
    while len(buf) < n:
        c = REQ.read(n - len(buf))
        if not c:
            return None
        buf += c
    return buf

def read_msg():
    line = b''
    while not line.endswith(b'\n'):
        c = REQ.read(1)
        if not c:
            return None
        line += c
    body = _read(int(line))
    return None if body is None else json.loads(body.decode('utf-8'))

def write_msg(obj):
    b = json.dumps(obj).encode('utf-8')
    REP.write(str(len(b)).encode() + b'\n' + b)

def __orgs_sh(cmd, shell=None):
    sys.stdout.flush(); sys.stderr.flush()
    if shell:
        subprocess.run([shell, '-c', cmd])
    else:
        subprocess.run(cmd, shell=True)

def __orgs_writefile(name, text):
    with open(os.path.expanduser(name), 'w') as f:
        f.write(text)
    print('Writing %s' % name)

def table_of(v):
    mod = type(v).__module__ or ''
    if not mod.startswith('pandas'):
        return None
    name = type(v).__name__
    if name == 'Series':
        v = v.to_frame()
    elif name != 'DataFrame':
        return None
    if len(v) > 500 or len(v.columns) > 50:
        return None
    rows = [[str(v.index.name or '')] + [str(c) for c in v.columns]]
    for i, row in zip(v.index, v.itertuples(index=False)):
        rows.append([str(i)] + [str(x) for x in row])
    return rows

def display(*objs, **kw):
    for o in objs:
        print(repr(o))

G['display'] = display
G['__orgs_sh'] = __orgs_sh
G['__orgs_writefile'] = __orgs_writefile

class Magic(Exception):
    pass

SKIP = ('matplotlib', 'config', 'load_ext', 'reload_ext', 'autoreload', 'precision', 'pylab', 'xmode')

def demagic(src):
    lines = src.split('\n')
    if lines and lines[0].lstrip().startswith('%%'):
        words = lines[0].lstrip()[2:].split()
        name = words[0] if words else ''
        body = '\n'.join(lines[1:])
        if name in ('time', 'timeit', 'capture', 'prun'):
            lines = lines[1:]
        elif name in ('bash', 'sh', 'zsh'):
            return '__orgs_sh(%r, %r)' % (body, name)
        elif name == 'script' and len(words) > 1:
            return '__orgs_sh(%r, %r)' % (body, words[1])
        elif name == 'writefile' and len(words) > 1:
            return '__orgs_writefile(%r, %r)' % (words[-1], body + '\n')
        else:
            raise Magic('%%' + name)
    out = []
    for l in lines:
        m = re.match(r'^(\s*)([!%])(.*)$', l)
        if not m:
            out.append(l)
            continue
        ind, kind, rest = m.groups()
        if kind == '!':
            out.append(ind + '__orgs_sh(%r)' % rest)
            continue
        words = rest.split()
        word = words[0] if words else ''
        arg = rest.strip()[len(word):].strip()
        if word in SKIP:
            out.append(ind + 'pass')
        elif word == 'pip':
            out.append(ind + '__orgs_sh(%r)' % ('"%s" -m pip %s' % (sys.executable, arg)))
        elif word == 'conda':
            out.append(ind + '__orgs_sh(%r)' % ('conda ' + arg))
        elif word in ('time', 'timeit', 'prun'):
            out.append(ind + arg)
        elif word == 'cd':
            out.append(ind + '__import__("os").chdir(%r)' % os.path.expanduser(arg or '~'))
        elif word == 'env' and '=' in arg:
            k, v = arg.split('=', 1)
            out.append(ind + '__import__("os").environ[%r] = %r' % (k.strip(), v.strip()))
        else:
            raise Magic('%' + word)
    return '\n'.join(out)

def error_of(e):
    tb = e.__traceback__
    while tb is not None and tb.tb_frame.f_code.co_filename == DRIVER:
        tb = tb.tb_next
    text = ''.join(traceback.format_exception(type(e), e, tb))
    return {'ename': type(e).__name__, 'evalue': str(e), 'traceback': text}

def figures(req, res):
    plt = sys.modules.get('matplotlib.pyplot')
    if plt is None:
        return
    nums = plt.get_fignums()
    base = req.get('figure')
    if nums and base:
        root, ext = os.path.splitext(base)
        if not ext:
            ext = '.png'
            base = root + ext
        d = os.path.dirname(base)
        if d:
            os.makedirs(d, exist_ok=True)
        for i, n in enumerate(nums):
            path = base if i == 0 else '%s-%d%s' % (root, i + 1, ext)
            try:
                plt.figure(n).savefig(path, bbox_inches='tight')
                res['figures'].append(path)
            except Exception as e:
                print('could not save figure %d: %s' % (n, e), file=sys.stderr)
    if nums:
        plt.close('all')

def run(req):
    code = req.get('code', '')
    fname = '<%s>' % (req.get('label') or 'block')
    res = {'out': '', 'value': None, 'table': None, 'figures': [], 'error': None}
    tmp = tempfile.TemporaryFile()
    sys.stdout.flush(); sys.stderr.flush()
    s1, s2 = os.dup(1), os.dup(2)
    os.dup2(tmp.fileno(), 1)
    os.dup2(tmp.fileno(), 2)
    try:
        try:
            if req.get('cwd'):
                os.chdir(req['cwd'])
            try:
                tree = ast.parse(code, fname, 'exec')
            except SyntaxError:
                again = demagic(code)
                if again == code:
                    raise
                code = again
                tree = ast.parse(code, fname, 'exec')
            linecache.cache[fname] = (len(code), None, code.splitlines(True), fname)
            last = None
            quiet = code.rstrip().endswith(';')
            if tree.body and isinstance(tree.body[-1], ast.Expr):
                last = ast.Expression(tree.body.pop().value)
            exec(compile(tree, fname, 'exec'), G)
            if last is not None:
                v = eval(compile(last, fname, 'eval'), G)
                if v is not None and not quiet:
                    G['_'] = v
                    t = table_of(v)
                    if t is not None:
                        res['table'] = t
                    else:
                        try:
                            res['value'] = repr(v)
                        except Exception:
                            res['value'] = '<%s that cannot be printed>' % type(v).__name__
        except Magic as m:
            res['error'] = {'ename': 'UsageError', 'evalue': 'IPython magic %s is not something orgs can run' % m, 'traceback': 'UsageError: IPython magic %s is not something orgs can run\n' % m}
        except BaseException as e:
            res['error'] = error_of(e)
        try:
            figures(req, res)
        except BaseException as e:
            print('could not save figures: %s' % e, file=sys.stderr)
    finally:
        try:
            sys.stdout.flush(); sys.stderr.flush()
        except Exception:
            pass
        os.dup2(s1, 1); os.dup2(s2, 2)
        os.close(s1); os.close(s2)
    tmp.seek(0)
    res['out'] = tmp.read().decode('utf-8', 'replace')
    tmp.close()
    return res

while True:
    try:
        req = read_msg()
        if req is None:
            break
        if req.get('op') == 'hello':
            write_msg({'pid': os.getpid(), 'version': sys.version.split()[0]})
            continue
        write_msg(run(req))
    except KeyboardInterrupt:
        continue
`

// One request to a session, and its answer. The json keys are the driver's.
type sessionRequest struct {
	Code   string `json:"code"`
	Cwd    string `json:"cwd,omitempty"`
	Label  string `json:"label,omitempty"`
	Figure string `json:"figure,omitempty"`
}

type sessionError struct {
	Name      string `json:"ename"`
	Value     string `json:"evalue"`
	Traceback string `json:"traceback"`
}

type sessionReply struct {
	Out     string        `json:"out"`
	Value   *string       `json:"value"`
	Table   [][]string    `json:"table"`
	Figures []string      `json:"figures"`
	Error   *sessionError `json:"error"`
}

// A running python and what is known about it.
type babelSession struct {
	key, name, file, python string

	cmd   *exec.Cmd
	reqW  *os.File
	rep   *bufio.Reader
	repR  *os.File
	stray *capped // what the python itself said outside any block: a crash

	// One block at a time: the code shares one set of globals.
	run      sync.Mutex
	started  time.Time
	lastUsed time.Time
	runs     int
	pid      int
	version  string
	exited   chan struct{}
}

// The last few kilobytes written to it, for saying why a python died.
type capped struct {
	mu  sync.Mutex
	buf []byte
}

func (c *capped) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, p...)
	if len(c.buf) > 8192 {
		c.buf = c.buf[len(c.buf)-8192:]
	}
	return len(p), nil
}

func (c *capped) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.TrimSpace(string(c.buf))
}

var (
	sessionsMu    sync.Mutex
	babelSessions = map[string]*babelSession{}
	reaperOnce    sync.Once
)

// The session a job asks for, by name, or "" when it asks for none. A bare
// :session is org's default session.
func (j *babelJob) sessionName() string {
	v, ok := j.arg("session")
	if !ok {
		return ""
	}
	v = strings.TrimSpace(v)
	if strings.EqualFold(v, "none") {
		return ""
	}
	if v == "" {
		return "default"
	}
	return v
}

func sessionKey(name, orgPath string) string {
	abs, err := filepath.Abs(orgPath)
	if err != nil {
		abs = orgPath
	}
	return "python\x00" + name + "\x00" + abs
}

// The session for a name in a file, started when there is none or the one
// there was has died.
func sessionFor(python []string, name, orgPath, dir string) (*babelSession, error) {
	reaperOnce.Do(func() { go reapSessions() })
	key := sessionKey(name, orgPath)
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	if s, ok := babelSessions[key]; ok {
		select {
		case <-s.exited:
			delete(babelSessions, key)
		default:
			if s.python == strings.Join(python, " ") {
				return s, nil
			}
			// The interpreter asked for changed (:python, or the yaml):
			// the old one would answer for the wrong environment.
			go s.stop()
			delete(babelSessions, key)
		}
	}
	s, err := startSession(python, dir)
	if err != nil {
		return nil, err
	}
	s.key, s.name, s.file = key, name, orgPath
	babelSessions[key] = s
	return s, nil
}

func startSession(python []string, dir string) (*babelSession, error) {
	if len(python) == 0 {
		python = []string{"python3"}
	}
	reqR, reqW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	repR, repW, err := os.Pipe()
	if err != nil {
		reqR.Close()
		reqW.Close()
		return nil, err
	}
	args := append(append([]string{}, python[1:]...), "-u", "-c", pythonSessionDriver)
	cmd := exec.Command(python[0], args...)
	cmd.Dir = dir
	// Agg: a figure is drawn into memory and saved, never shown in a window
	// on the server's screen.
	cmd.Env = append(os.Environ(), "MPLBACKEND=Agg", "PYTHONIOENCODING=utf-8", "PYTHONUNBUFFERED=1")
	cmd.ExtraFiles = []*os.File{reqR, repW}
	s := &babelSession{
		python: strings.Join(python, " "), cmd: cmd, reqW: reqW, repR: repR,
		rep: bufio.NewReader(repR), stray: &capped{}, exited: make(chan struct{}),
		started: time.Now(), lastUsed: time.Now(),
	}
	cmd.Stdout = s.stray
	cmd.Stderr = s.stray
	if err := cmd.Start(); err != nil {
		reqR.Close()
		reqW.Close()
		repR.Close()
		repW.Close()
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%s is not installed here (or not on the server's PATH). "+
				"Say where it is with :python, or in orgs.yaml:\n\n  babel:\n    commands:\n      python: [\"/path/to/python\"]", python[0])
		}
		return nil, err
	}
	// The child holds its own ends now; the parent keeping them open would
	// stop an exit from ever reading as end of file.
	reqR.Close()
	repW.Close()
	go func() {
		cmd.Wait()
		close(s.exited)
	}()
	var hello struct {
		Pid     int    `json:"pid"`
		Version string `json:"version"`
	}
	if err := s.exchange(map[string]string{"op": "hello"}, &hello, 20*time.Second); err != nil {
		s.kill()
		return nil, fmt.Errorf("the python session did not start: %v", err)
	}
	s.pid, s.version = hello.Pid, hello.Version
	return s, nil
}

// One request and its reply, given at most timeout. A reply that does not
// come is interrupted, then killed.
func (s *babelSession) exchange(req interface{}, reply interface{}, timeout time.Duration) error {
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if _, err := s.reqW.Write(append([]byte(strconv.Itoa(len(b))+"\n"), b...)); err != nil {
		return s.died(err)
	}
	type answer struct {
		body []byte
		err  error
	}
	got := make(chan answer, 1)
	go func() {
		line, err := s.rep.ReadString('\n')
		if err != nil {
			got <- answer{err: err}
			return
		}
		n, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil {
			got <- answer{err: fmt.Errorf("the session answered something unframed: %q", line)}
			return
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(s.rep, body); err != nil {
			got <- answer{err: err}
			return
		}
		got <- answer{body: body}
	}()
	var a answer
	select {
	case a = <-got:
	case <-time.After(timeout):
		// Interrupted the way a notebook's stop button does it: the block
		// gets a KeyboardInterrupt and everything it made before stays.
		s.cmd.Process.Signal(os.Interrupt)
		select {
		case a = <-got:
			if a.err == nil {
				err := json.Unmarshal(a.body, reply)
				if err == nil {
					return errInterrupted{timeout}
				}
			}
		case <-time.After(babelInterruptGrace):
			s.kill()
			return fmt.Errorf("it was still running after %s and did not stop when interrupted, so the session was stopped (what it held is gone)", timeout)
		}
	}
	if a.err != nil {
		return s.died(a.err)
	}
	return json.Unmarshal(a.body, reply)
}

// A block that ran past the timeout and stopped when interrupted. Its reply,
// with the KeyboardInterrupt in it, is still worth writing.
type errInterrupted struct{ after time.Duration }

func (e errInterrupted) Error() string {
	return fmt.Sprintf("it was still running after %s and was interrupted; the session keeps what it had", e.after)
}

func (s *babelSession) died(err error) error {
	// Give Wait a moment to see the exit, so the message has its reason.
	select {
	case <-s.exited:
	case <-time.After(500 * time.Millisecond):
	}
	s.kill()
	why := s.stray.String()
	if why != "" {
		return fmt.Errorf("the python session died: %s", why)
	}
	return fmt.Errorf("the python session died: %v", err)
}

// Stopped politely: told there is nothing more, which ends its read loop.
func (s *babelSession) stop() {
	s.reqW.Close()
	select {
	case <-s.exited:
	case <-time.After(2 * time.Second):
		s.kill()
	}
	s.repR.Close()
}

func (s *babelSession) kill() {
	s.reqW.Close()
	if s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
	sessionsMu.Lock()
	if babelSessions[s.key] == s {
		delete(babelSessions, s.key)
	}
	sessionsMu.Unlock()
}

func reapSessions() {
	for range time.Tick(time.Minute) {
		sessionsMu.Lock()
		idle := []*babelSession{}
		for k, s := range babelSessions {
			if s.run.TryLock() {
				if time.Since(s.lastUsed) > babelSessionIdle {
					idle = append(idle, s)
					delete(babelSessions, k)
				}
				s.run.Unlock()
			}
		}
		sessionsMu.Unlock()
		for _, s := range idle {
			s.stop()
		}
	}
}

// StopBabelSessions stops every python session; the server calls it on its
// way down. A server that dies without calling it leaves nothing behind
// either: each python reads end of file when its parent goes, and exits.
func StopBabelSessions() {
	sessionsMu.Lock()
	all := []*babelSession{}
	for k, s := range babelSessions {
		all = append(all, s)
		delete(babelSessions, k)
	}
	sessionsMu.Unlock()
	var wg sync.WaitGroup
	for _, s := range all {
		wg.Add(1)
		go func(s *babelSession) { defer wg.Done(); s.stop() }(s)
	}
	wg.Wait()
}

// stopSessionsFor stops the sessions of one file (every file when empty),
// only the named one when name is given. Returns how many stopped.
func stopSessionsFor(file, name string) int {
	abs := ""
	if file != "" {
		abs, _ = filepath.Abs(file)
	}
	sessionsMu.Lock()
	hit := []*babelSession{}
	for k, s := range babelSessions {
		sf, _ := filepath.Abs(s.file)
		if (abs == "" || sf == abs) && (name == "" || s.name == name) {
			hit = append(hit, s)
			delete(babelSessions, k)
		}
	}
	sessionsMu.Unlock()
	for _, s := range hit {
		s.stop()
	}
	return len(hit)
}

// ----------------------------------------------------------------------------
// Running a job in its session
// ----------------------------------------------------------------------------

// The interpreter for a python job: :python when the block says, otherwise
// what the yaml or the built in runner says.
func (j *babelJob) pythonCommand(r runner) []string {
	if p, ok := j.arg("python"); ok && strings.TrimSpace(p) != "" {
		return shellWords(p)
	}
	return r.cmd
}

// Where a figure drawn by a job goes: :file when it says, otherwise a name
// made from the code under <file>_files/, so running the same code again
// draws over the same picture rather than leaving the last one behind.
func (j *babelJob) figurePath(code string) (abs string) {
	if _, a, ok := j.fileArg(); ok {
		return a
	}
	sum := sha1.Sum([]byte(code))
	base := strings.TrimSuffix(filepath.Base(j.OrgPath), filepath.Ext(j.OrgPath))
	return filepath.Join(filepath.Dir(j.OrgPath), base+"_files", "fig-"+hex.EncodeToString(sum[:])[:8]+".png")
}

// runInSession runs a python job in its session and reads the reply the way
// runJob reads a program's output.
func runInSession(j babelJob, r runner, name, program, collect string, timeout time.Duration) (common.CodeResult, error) {
	res := common.CodeResult{}
	s, err := sessionFor(j.pythonCommand(r), name, j.OrgPath, j.Dir)
	if err != nil {
		res.Msg = err.Error()
		return res, nil
	}
	s.run.Lock()
	defer s.run.Unlock()
	s.lastUsed = time.Now()
	s.runs++

	if collect == "value" && pyReturnRe.MatchString(program) {
		// org's value blocks end in a return. In a session the block's last
		// expression is its value, so the body becomes a function and the
		// call to it that expression.
		lines := []string{"def __orgs_main():"}
		for _, l := range strings.Split(program, "\n") {
			lines = append(lines, "    "+l)
		}
		program = strings.Join(append(lines, "__orgs_main()"), "\n")
	}
	req := sessionRequest{Code: program, Cwd: j.Dir, Label: j.Where, Figure: j.figurePath(j.Code)}
	var rep sessionReply
	start := time.Now()
	err = s.exchange(req, &rep, timeout)
	res.Seconds = time.Since(start).Seconds()
	var interrupted errInterrupted
	if err != nil && !errors.As(err, &interrupted) {
		res.Msg = err.Error()
		return res, nil
	}
	sessionResult(&res, rep, j.results(), collect, j.OrgPath)
	if err != nil {
		res.Ok, res.Msg = false, err.Error()
	}
	describeResultFile(&res, j.OrgPath)
	return res, nil
}

var pyReturnRe = regexp.MustCompile(`(?m)^return\b`)

// sessionResult turns a session's reply into a result: what was printed, the
// value, a table, figures and a traceback, in the shapes runJob gives a
// program's output - and, when there is more than one of them, all of them in
// a :results: drawer, the way emacs-jupyter writes a cell's outputs.
func sessionResult(res *common.CodeResult, rep sessionReply, want []string, collect string, orgPath string) {
	text := ""
	if collect != "value" {
		text = rep.Out
	}
	if rep.Value != nil {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += *rep.Value
	}
	res.Ok = rep.Error == nil
	if rep.Error != nil {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += rep.Error.Traceback
		res.Msg = rep.Error.Name
		if rep.Error.Value != "" {
			res.Msg += ": " + rep.Error.Value
		}
	}
	res.Raw = strings.TrimRight(text, "\n")
	links := []string{}
	for _, f := range rep.Figures {
		links = append(links, "[[file:"+relToOrg(f, orgPath)+"]]")
	}
	// :results file and graphics say where a figure goes; they are not a
	// shape for the text.
	shapeWants := []string{}
	for _, w := range want {
		if w != "file" && w != "graphics" {
			shapeWants = append(shapeWants, w)
		}
	}
	res.Kind, res.Result = composeResult(strings.TrimRight(text, "\n"), rep.Table, links, shapeWants)
	if rep.Error != nil && res.Kind != "drawer" {
		// Written like text, and marked so the writer knows a failed run has
		// something to show.
		res.Kind = "error"
	}
}

// The result of several outputs - text, a table, pictures - as org writes
// them: one of them alone in its own shape, several in a :results: drawer.
// Shared by sessions and the notebook importer, so a cell imported and the
// same cell run again come out the same.
func composeResult(text string, table [][]string, links []string, want []string) (kind, result string) {
	parts := 0
	if strings.TrimSpace(text) != "" {
		parts++
	}
	if table != nil {
		parts++
	}
	parts += len(links)
	switch {
	case parts == 0:
		return "text", ""
	case parts == 1 && table != nil:
		return "table", tableResultText(table)
	case parts == 1 && len(links) == 1:
		return "file", links[0]
	case parts == 1:
		k := babelShape(want, text)
		return k, babelFormat(k, text)
	}
	body := []string{":results:"}
	if strings.TrimSpace(text) != "" {
		body = append(body, resultBody(&common.CodeResult{Kind: "text", Result: text}, nil, "")...)
	}
	if table != nil {
		body = append(body, tableResultText(table))
	}
	body = append(body, links...)
	body = append(body, ":end:")
	return "drawer", strings.Join(body, "\n")
}

// A table from a session or a notebook: the first row a header under a rule,
// and a | inside a cell written the way org escapes it.
func tableResultText(rows [][]string) string {
	out := [][]string{}
	for i, r := range rows {
		cells := []string{}
		for _, c := range r {
			c = strings.ReplaceAll(strings.ReplaceAll(c, "\n", " "), "|", `\vert{}`)
			cells = append(cells, c)
		}
		out = append(out, cells)
		if i == 0 && len(rows) > 1 {
			out = append(out, nil)
		}
	}
	return orgTableText(out)
}

// A path as a link in the org file says it: relative to the file's folder
// when it is under it, absolute otherwise.
func relToOrg(path, orgPath string) string {
	dir := filepath.Dir(orgPath)
	if rel, err := filepath.Rel(dir, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return path
}

// ----------------------------------------------------------------------------
// The endpoints
// ----------------------------------------------------------------------------

func registerBabelSessionRoutes(api *mux.Router) {
	api.HandleFunc("/babel/sessions", RequestBabelSessions).Methods("GET")
	api.HandleFunc("/babel/sessions/stop", PostBabelSessionsStop).Methods("POST")
}

// BabelSessionInfo is one running session, as GET /babel/sessions lists it.
type BabelSessionInfo struct {
	Name     string
	File     string
	Python   string
	Version  string
	Pid      int
	Runs     int
	Started  time.Time
	LastUsed time.Time
}

/* SDOC: API
* GET /babel/sessions — The Python Sessions Running

	The sessions =:session= started, one per name per file: =Name=, =File=,
	=Python= (the command), =Version=, =Pid=, =Runs=, =Started=, =LastUsed=.
EDOC */
func RequestBabelSessions(w http.ResponseWriter, r *http.Request) {
	sessionsMu.Lock()
	out := []BabelSessionInfo{}
	for _, s := range babelSessions {
		out = append(out, BabelSessionInfo{Name: s.name, File: s.file, Python: s.python, Version: s.version,
			Pid: s.pid, Runs: s.runs, Started: s.started, LastUsed: s.lastUsed})
	}
	sessionsMu.Unlock()
	sort.Slice(out, func(a, b int) bool {
		if out[a].File != out[b].File {
			return out[a].File < out[b].File
		}
		return out[a].Name < out[b].Name
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

/* SDOC: API
* POST /babel/sessions/stop — Stop Python Sessions

	#+BEGIN_EXAMPLE
	{ "Filename": "/path/analysis.org" }                    every session of that file
	{ "Filename": "/path/analysis.org", "Name": "analysis" } that one
	{ }                                                     all of them
	#+END_EXAMPLE

	What a notebook's "restart kernel" is: the next block to run starts a
	fresh python. Answers how many stopped.
EDOC */
func PostBabelSessionsStop(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filename string
		Name     string
	}
	body, _ := io.ReadAll(r.Body)
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			answerErr(w, err)
			return
		}
	}
	file := req.Filename
	if file != "" {
		if f := GetDb().FindByFile(file); f != nil && f.Doc != nil {
			file = f.Doc.Path
		}
	}
	n := stopSessionsFor(file, req.Name)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(common.ResultMsg{Ok: true, Msg: fmt.Sprintf("%d stopped", n)})
}
