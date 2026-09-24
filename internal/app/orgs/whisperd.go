//lint:file-ignore ST1006 allow the use of self
package orgs

// Running go-whisper
//
// Orgs does not link whisper. The model needs cgo and a built whisper.cpp, and
// requiring those to compile orgs would put them on everybody who never records
// anything. So when the voice block names a models directory, orgs starts a
// gowhisper of its own - the same service you would run by hand - looks after
// it while the server is up, and stops it on the way out.
//
// Two things follow from that and are worth not undoing. A whisper already
// listening on the port is *adopted* rather than started again, so running one
// by hand still works and an orphan left by a hard kill is picked back up. And
// the child is stopped on a signal as well as on a clean exit, because a server
// that leaves a 500mb model resident after ctrl-c is a bad neighbour.

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// What the supervisor is doing, in the words the client shows.
const (
	WhisperOff      = "off"      // nothing configured to run
	WhisperStarting = "starting" // spawned, not answering yet
	WhisperReady    = "ready"    // answering
	WhisperAdopted  = "adopted"  // someone else's, already listening
	WhisperFailed   = "failed"   // did not start, or died and stayed dead
)

const whisperLogLines = 40

type whisperd struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	state    string
	msg      string
	bin      string
	lines    []string
	stopping bool
	starts   int
	stopOnce sync.Once
}

var whisper = &whisperd{state: WhisperOff}

// ----------------------------------------------------------------------------
// What is configured
// ----------------------------------------------------------------------------

// Whether orgs is the one running whisper. Naming a models directory is what
// asks for it; an explicit url is how you say somebody else's.
func whisperManaged() bool {
	v := voiceSettings()
	return strings.TrimSpace(v.Models) != "" && strings.TrimSpace(v.Url) == ""
}

func whisperPort() int {
	if p := voiceSettings().Port; p > 0 {
		return p
	}
	return 8081
}

// The base url of the whisper to talk to: the one configured by hand, or the
// one orgs runs.
func whisperBase() string {
	if u := strings.TrimSpace(voiceSettings().Url); u != "" {
		return strings.TrimRight(u, "/")
	}
	return fmt.Sprintf("http://localhost:%d", whisperPort())
}

// The models directory, with ~ expanded and made absolute, because the child is
// not guaranteed to inherit this process's idea of where it is.
func whisperModelsDir() (string, error) {
	p := strings.TrimSpace(voiceSettings().Models)
	if p == "" {
		return "", fmt.Errorf("no models directory is configured")
	}
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("the models directory %s is not there", abs)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is a file, not a models directory", abs)
	}
	return abs, nil
}

// Where the gowhisper binary might be. The configured path wins, then PATH,
// then the places it lands when it is built from source rather than installed.
func whisperBinary() (string, error) {
	if b := strings.TrimSpace(voiceSettings().Bin); b != "" {
		if strings.HasPrefix(b, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				b = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(b, "~"), "/"))
			}
		}
		abs, _ := filepath.Abs(b)
		if info, err := os.Stat(abs); err == nil && !info.IsDir() {
			return abs, nil
		}
		return "", fmt.Errorf("there is no gowhisper at %s", b)
	}
	if p, err := exec.LookPath("gowhisper"); err == nil {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		"./gowhisper",
		"../go-whisper/build/gowhisper",
	}
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, "go", "bin", "gowhisper"),
			filepath.Join(home, "dev", "go-whisper", "build", "gowhisper"),
		)
	}
	for _, c := range candidates {
		if abs, err := filepath.Abs(c); err == nil {
			if info, err := os.Stat(abs); err == nil && !info.IsDir() {
				return abs, nil
			}
		}
	}
	return "", fmt.Errorf("no gowhisper binary found - put one on your PATH, or say where it is with voice.bin")
}

// ----------------------------------------------------------------------------
// The supervisor
// ----------------------------------------------------------------------------

func (self *whisperd) say(format string, args ...interface{}) {
	line := fmt.Sprintf(format, args...)
	self.mu.Lock()
	self.lines = append(self.lines, line)
	if len(self.lines) > whisperLogLines {
		self.lines = self.lines[len(self.lines)-whisperLogLines:]
	}
	self.mu.Unlock()
}

func (self *whisperd) set(state, msg string) {
	self.mu.Lock()
	self.state = state
	self.msg = msg
	self.mu.Unlock()
}

// Status, for the client and for the log.
func WhisperStatus() (state string, msg string, log []string) {
	whisper.mu.Lock()
	defer whisper.mu.Unlock()
	out := make([]string, len(whisper.lines))
	copy(out, whisper.lines)
	return whisper.state, whisper.msg, out
}

// Is anything answering on the whisper url right now? Short timeout: this is
// asked on the way past, not as the answer to anything.
func whisperAnswering(timeout time.Duration) bool {
	client := &http.Client{Timeout: timeout}
	res, err := client.Get(whisperBase() + "/api/whisper/model")
	if err != nil {
		return false
	}
	defer res.Body.Close()
	io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	return res.StatusCode == http.StatusOK
}

// StartWhisper brings up the transcription service if the config asks for one.
// It never blocks the server starting: a model takes a while to load and the
// rest of orgs has nothing to do with it.
func StartWhisper() {
	if !whisperManaged() {
		if strings.TrimSpace(voiceSettings().Url) != "" {
			whisper.set(WhisperOff, "using the whisper at "+whisperBase())
		}
		return
	}
	go whisper.start()
	whisper.stopOnce.Do(func() {
		// A child is not taken down by its parent exiting, and the listener in
		// StartServer ends the process with log.Fatal rather than unwinding, so
		// the only reliable place to stop whisper is here.
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-ch
			fmt.Printf("\n[whisper] stopping\n")
			StopWhisper()
			os.Exit(0)
		}()
	})
}

func (self *whisperd) start() {
	// Somebody else's whisper on the same port is used as it is. That covers
	// the one you started by hand, and the one this process orphaned last time
	// it was killed outright.
	if whisperAnswering(2 * time.Second) {
		self.set(WhisperAdopted, "adopted the whisper already listening on "+whisperBase())
		fmt.Printf("[whisper] adopted the server already on %s\n", whisperBase())
		return
	}

	models, err := whisperModelsDir()
	if err != nil {
		self.set(WhisperFailed, err.Error())
		fmt.Printf("[whisper] %s\n", err)
		return
	}
	bin, err := whisperBinary()
	if err != nil {
		self.set(WhisperFailed, err.Error())
		fmt.Printf("[whisper] %s\n", err)
		return
	}

	v := voiceSettings()
	args := []string{"run",
		"--http.addr", fmt.Sprintf("localhost:%d", whisperPort()),
		"--models", models,
	}
	if v.Gpu == nil || *v.Gpu {
		args = append(args, "--whisper.gpu")
	}
	args = append(args, v.Args...)

	cmd := exec.Command(bin, args...)
	cmd.Dir = models
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	self.mu.Lock()
	self.cmd = cmd
	self.bin = bin
	self.stopping = false
	self.starts++
	attempt := self.starts
	self.mu.Unlock()
	self.set(WhisperStarting, fmt.Sprintf("starting %s on port %d", filepath.Base(bin), whisperPort()))

	if err := cmd.Start(); err != nil {
		self.set(WhisperFailed, fmt.Sprintf("could not run %s: %s", bin, err))
		fmt.Printf("[whisper] could not run %s: %s\n", bin, err)
		return
	}
	fmt.Printf("[whisper] %s run --http.addr localhost:%d --models %s\n", bin, whisperPort(), models)

	go self.drain(stdout)
	go self.drain(stderr)
	go self.wait(cmd, attempt)
	go self.awaitReady()
}

// Whisper's own output, kept for the client to show when something has gone
// wrong and echoed with a prefix so it is not mistaken for the server's.
func (self *whisperd) drain(r io.ReadCloser) {
	if r == nil {
		return
	}
	defer r.Close()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 16*1024), 512*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if line == "" {
			continue
		}
		self.say("%s", line)
		fmt.Printf("[whisper] %s\n", line)
	}
}

// The child exiting. On the way out that is expected; otherwise it is worth one
// more go, because the usual cause is a port that was still in TIME_WAIT or a
// model that was still being written.
func (self *whisperd) wait(cmd *exec.Cmd, attempt int) {
	err := cmd.Wait()
	self.mu.Lock()
	stopping := self.stopping
	starts := self.starts
	self.mu.Unlock()
	if stopping || attempt != starts {
		return
	}
	why := "whisper exited"
	if err != nil {
		why = fmt.Sprintf("whisper exited: %s", err)
	}
	fmt.Printf("[whisper] %s\n", why)
	if starts >= 3 {
		self.set(WhisperFailed, why+" - not starting it again. The last lines of its output are in the voice panel.")
		return
	}
	self.set(WhisperStarting, why+" - starting it again")
	time.Sleep(time.Duration(starts) * 2 * time.Second)
	self.start()
}

// Wait for it to answer. A large model on a cold cache is tens of seconds, so
// this is patient, and it gives up saying what it was waiting for rather than
// leaving the client on "starting" forever.
func (self *whisperd) awaitReady() {
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		self.mu.Lock()
		stopping := self.stopping
		state := self.state
		self.mu.Unlock()
		if stopping || state == WhisperFailed {
			return
		}
		if whisperAnswering(2 * time.Second) {
			self.set(WhisperReady, "")
			fmt.Printf("[whisper] ready on %s\n", whisperBase())
			return
		}
		time.Sleep(time.Second)
	}
	self.set(WhisperFailed, fmt.Sprintf("whisper did not answer on %s after three minutes", whisperBase()))
}

// StopWhisper takes the child down, politely first. Nothing happens to a
// whisper that was adopted rather than started - it was somebody else's.
func StopWhisper() {
	whisper.mu.Lock()
	cmd := whisper.cmd
	whisper.stopping = true
	whisper.cmd = nil
	whisper.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	whisper.set(WhisperOff, "stopped")
	_ = cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
	}
}

// RestartWhisper stops whatever is running and starts it again. For the button
// in the client, which is the right answer to a model that failed to load or a
// service that has wedged.
func RestartWhisper() error {
	if !whisperManaged() {
		return fmt.Errorf("orgs is not the one running whisper - set voice.models to have it start one")
	}
	StopWhisper()
	whisper.mu.Lock()
	whisper.stopping = false
	whisper.starts = 0
	whisper.lines = nil
	whisper.mu.Unlock()
	go whisper.start()
	return nil
}
