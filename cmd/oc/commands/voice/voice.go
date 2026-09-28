package voice

// orgs voice - say it, and have it filed.
//
//	orgs voice                     record until you press enter, then file it
//	orgs voice -on 'the migration' append to that heading instead
//	orgs voice -f take.m4a         a recording you already have
//	orgs voice ls                  what the server is holding
//	orgs voice rm <id>             throw one away
//	orgs voice config              what transcription is available
//
// The whole server side of this existed and nothing but a browser could reach
// it: `/voice/recording` keeps the audio, `/voice/transcribe` asks go-whisper,
// `/voice/note` files the heading, `/voice/append` puts it on a heading that is
// already there. What was missing was a client that could hold a microphone.
//
// It does not record audio itself. Recording is `sox`, `ffmpeg` or `arecord` -
// programs that already exist, are already installed on machines where people
// record things, and are not worth linking into an org server. The same
// reasoning whisperd.go gives for supervising go-whisper rather than linking it.
//
// The order is the voice notes' order and for the same reason: **the recording
// is saved before anything is attempted on it, and the transcript is shown
// before anything is written.** A transcription fails in a dozen ways and none
// of them should cost the words that were said - so the audio goes up first, and
// what gets written into the org file is what was on screen.

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Voice struct {
	tf commands.TargetFlags

	File     string
	On       string
	Headline string
	Tags     string
	Keep     bool
	Yes      bool
	Language string
	Model    string
	Seconds  int
}

func (self *Voice) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Voice) StartPlugin(m *common.PluginManager)       {}

func (self *Voice) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&self.File, "f", "", "file a recording you already have, instead of recording one")
	fset.StringVar(&self.On, "on", "",
		"append to the heading this query finds, instead of filing a new one")
	fset.StringVar(&self.Headline, "headline", "", "the heading text; the first line of the transcript otherwise")
	fset.StringVar(&self.Tags, "tags", "", "tags for the new heading, comma separated")
	fset.BoolVar(&self.Keep, "keep", true, "keep the audio and link to it")
	fset.BoolVar(&self.Yes, "yes", false, "do not show the transcript for approval before filing")
	fset.StringVar(&self.Language, "lang", "", "the language to transcribe as")
	fset.StringVar(&self.Model, "model", "", "the whisper model to use")
	fset.IntVar(&self.Seconds, "for", 0, "record for this many seconds instead of until enter")
	commands.AddTargetFlags(fset, &self.tf)
}

func (self *Voice) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("voice").Flags)
	sub := ""
	if len(words) > 0 {
		sub = words[0]
	}
	switch sub {
	case "ls", "list":
		self.list(core)
		return
	case "rm", "delete":
		self.remove(core, words[1:])
		return
	case "config":
		self.config(core)
		return
	}

	// Recording takes a while and transcription takes longer; nothing here
	// should give up on the server in the middle of it.
	cfg := commands.SendReceiveGetOr[common.VoiceConfig](core, "voice/config", nil)
	if !cfg.Ok {
		commands.Fail("this server has no voice support configured")
	}

	var audio []byte
	var name string
	if self.File != "" {
		b, err := os.ReadFile(self.File)
		if err != nil {
			commands.Fail("could not read %s: %v", self.File, err)
		}
		audio, name = b, filepath.Base(self.File)
	} else {
		audio, name = self.record(cfg)
	}
	if len(audio) == 0 {
		commands.Fail("nothing was recorded")
	}
	if cfg.MaxMb > 0 && len(audio) > cfg.MaxMb<<20 {
		commands.Fail("that recording is %d mb and the server's limit is %d",
			len(audio)>>20, cfg.MaxMb)
	}

	// 1. The audio goes up first and on its own. Everything after this can fail
	//    without costing what was said.
	if commands.Wrote(fmt.Sprintf("POST /voice/recording (%s, %d kb)", name, len(audio)>>10), nil) {
		return
	}
	rec, err := common.RestPostFile[common.VoiceRecording](&core.Rest, "voice/recording",
		"audio", name, audio, nil)
	if err != nil || rec.Id == "" {
		// Worth being loud about: the recording is the only copy and it has
		// just failed to leave this machine.
		self.rescue(audio, name)
		commands.Fail("the recording did not reach the server: %v", err)
	}
	if !commands.Machine() {
		fmt.Fprintf(os.Stderr, "%ssaved %s (%.0fs)%s\n", commands.C(commands.AnsiDim), rec.Id,
			rec.Seconds, commands.C(commands.AnsiReset))
	}

	// 2. Transcribe. A failure here leaves the recording on the server, and the
	//    id is printed so it can be tried again.
	if !cfg.Ready {
		commands.Fail("the recording is saved as %s, but transcription is %s - orgs voice config",
			rec.Id, orUnknown(cfg.State))
	}
	tr, err := commands.SendReceivePostErr[common.VoiceTranscribeRequest,
		common.VoiceTranscription](core, "voice/transcribe", &common.VoiceTranscribeRequest{
		Id: rec.Id, Model: self.Model, Language: self.Language,
	})
	if err != nil || !tr.Ok {
		commands.Fail("the recording is saved as %s, but transcribing it failed: %s %v",
			rec.Id, tr.Msg, err)
	}
	text := strings.TrimSpace(tr.Text)
	if text == "" {
		// Whisper heard nothing. The recording is still filed, which is the
		// point of doing it in this order.
		fmt.Fprintf(os.Stderr, "%snothing was transcribed - the recording is kept as %s%s\n",
			commands.C(commands.AnsiGold), rec.Id, commands.C(commands.AnsiReset))
	}

	// 3. Show it before writing it. A transcript nobody has read is a guess,
	//    and an org file is not where to find that out.
	text = self.approve(text)
	if strings.TrimSpace(text) == "" {
		commands.Fail("nothing to file - the recording is kept as %s", rec.Id)
	}

	if self.On != "" {
		self.append(core, rec, tr, text)
		return
	}
	self.file(core, cfg, rec, tr, text)
}

// record shells out to whatever this machine has. Nothing is converted: the
// extension follows what was actually recorded, and go-whisper decodes through
// ffmpeg, so a wav, an m4a and an opus all go straight through.
func (self *Voice) record(cfg common.VoiceConfig) ([]byte, string) {
	if !commands.Interactive() && self.Seconds == 0 {
		commands.Fail("recording needs a terminal - use -f a-file, or -for <seconds>")
	}
	tool, args, ext := recorder()
	if tool == "" {
		commands.Fail("no recorder on PATH - install sox (rec), ffmpeg or arecord, or use -f")
	}

	tmp, err := os.CreateTemp("", "orgs-voice-*"+ext)
	if err != nil {
		commands.Fail("%v", err)
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)

	cmd := exec.Command(tool, append(args, path)...)
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		commands.Fail("could not start %s: %v", tool, err)
	}

	started := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		switch {
		case self.Seconds > 0:
			time.Sleep(time.Duration(self.Seconds) * time.Second)
		default:
			fmt.Fprintf(os.Stderr, "%s● recording — press enter to stop%s\n",
				commands.C(commands.AnsiRed), commands.C(commands.AnsiReset))
			bufio.NewReader(os.Stdin).ReadString('\n')
		}
	}()

	// ctrl-c stops the recorder rather than killing this process and leaving it
	// holding the microphone.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-done:
	case <-sig:
		fmt.Fprintln(os.Stderr)
	}
	signal.Stop(sig)

	// Asked to stop rather than killed, so it can close the file it is writing:
	// a wav killed mid-write has no length in its header and decodes as silence.
	_ = cmd.Process.Signal(syscall.SIGINT)
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	select {
	case <-waited:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-waited
	}

	b, err := os.ReadFile(path)
	if err != nil {
		commands.Fail("could not read the recording back: %v", err)
	}
	if !commands.Machine() {
		fmt.Fprintf(os.Stderr, "%s%.0fs, %d kb%s\n", commands.C(commands.AnsiDim),
			time.Since(started).Seconds(), len(b)>>10, commands.C(commands.AnsiReset))
	}
	return b, "note" + ext
}

// recorder is the first of the usual three that is on PATH, with the arguments
// that make it record from the default input until it is asked to stop.
func recorder() (string, []string, string) {
	for _, c := range []struct {
		bin  string
		args []string
		ext  string
	}{
		// sox: one channel, 16kHz, which is what whisper wants anyway and a
		// tenth of the bytes of a stereo 44.1k take.
		{"rec", []string{"-q", "-c", "1", "-r", "16000"}, ".wav"},
		{"ffmpeg", []string{"-hide_banner", "-loglevel", "error", "-f", defaultAudioIn(),
			"-i", "default", "-ac", "1", "-ar", "16000", "-y"}, ".wav"},
		{"arecord", []string{"-q", "-f", "S16_LE", "-c", "1", "-r", "16000"}, ".wav"},
	} {
		if _, err := exec.LookPath(c.bin); err == nil {
			return c.bin, c.args, c.ext
		}
	}
	return "", nil, ""
}

// ffmpeg needs to be told which capture api to use and it differs per platform.
func defaultAudioIn() string {
	switch runtime.GOOS {
	case "darwin":
		return "avfoundation"
	case "windows":
		return "dshow"
	}
	return "alsa"
}

// approve shows the transcript and lets it be corrected before anything is
// written. -yes skips it, which is what a script wants.
func (self *Voice) approve(text string) string {
	if self.Yes || !commands.Interactive() {
		return text
	}
	fmt.Printf("\n%s%s%s\n\n", commands.C(commands.AnsiBold), text,
		commands.C(commands.AnsiReset))
	fmt.Fprintf(os.Stderr, "%s[enter] file it · [e] edit it · [q] keep the audio and stop%s ",
		commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "q", "n":
		return ""
	case "e":
		return editText(text)
	}
	return text
}

func editText(text string) string {
	f, err := os.CreateTemp("", "orgs-voice-*.org")
	if err != nil {
		return text
	}
	defer os.Remove(f.Name())
	fmt.Fprintf(f, "%s\n", text)
	f.Close()
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command(editor, f.Name())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return text
	}
	b, err := os.ReadFile(f.Name())
	if err != nil {
		return text
	}
	return strings.TrimSpace(string(b))
}

// file writes a new heading, through the endpoint that already knows where the
// configured target is.
func (self *Voice) file(core *commands.Core, cfg common.VoiceConfig,
	rec common.VoiceRecording, tr common.VoiceTranscription, text string) {
	headline := self.Headline
	body := text
	if headline == "" {
		// The first line is the heading and the rest is the body, which is what
		// somebody dictating a note does without being told to.
		headline, body = splitFirstLine(text)
	}
	tags := []string{}
	for _, t := range strings.Split(self.Tags, ",") {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}

	req := common.VoiceNoteRequest{
		Id: rec.Id, Headline: headline, Text: body, Tags: tags,
		KeepAudio: self.Keep, Model: tr.Model, Seconds: tr.Duration,
	}
	res, err := commands.SendReceivePostErr[common.VoiceNoteRequest,
		common.VoiceNoteResult](core, "voice/note", &req)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil || !res.Ok {
		commands.Fail("the recording is saved as %s, but filing it failed: %s %v",
			rec.Id, res.Msg, err)
	}
	if commands.RenderOne(res, nil) {
		return
	}
	fmt.Printf("%s✓%s %s%s:%d%s %s\n", commands.C(commands.AnsiGreen),
		commands.C(commands.AnsiReset), commands.C(commands.AnsiDim),
		commands.BaseName(res.Filename), res.Line, commands.C(commands.AnsiReset), res.Headline)
}

// append puts the words on a heading that already exists, through /voice/append.
func (self *Voice) append(core *commands.Core, rec common.VoiceRecording,
	tr common.VoiceTranscription, text string) {
	todos := commands.Resolve(core, &self.tf, []string{self.On}, commands.TargetOpts{
		Prompt: "Append to> ",
		Multi:  false,
	})
	t := todos[0]

	req := struct {
		Hash      string  `json:"hash"`
		Id        string  `json:"id"`
		Text      string  `json:"text"`
		KeepAudio bool    `json:"keepAudio"`
		Model     string  `json:"model"`
		Seconds   float64 `json:"seconds"`
	}{t.Hash, rec.Id, text, self.Keep, tr.Model, tr.Duration}

	res, err := commands.SendReceivePostErr[any, common.VoiceNoteResult](core, "voice/append",
		anyOf(req))
	if err == commands.ErrDryRun {
		return
	}
	if err != nil || !res.Ok {
		commands.Fail("the recording is saved as %s, but appending it failed: %s %v",
			rec.Id, res.Msg, err)
	}
	fmt.Printf("%s✓%s %s %s%s%s\n", commands.C(commands.AnsiGreen),
		commands.C(commands.AnsiReset), commands.Describe(t), commands.C(commands.AnsiDim),
		commands.Ellipsis(oneLine(text), 40), commands.C(commands.AnsiReset))
}

func anyOf[T any](v T) *any {
	var a any = v
	return &a
}

// rescue keeps a recording that could not be uploaded. Losing the only copy of
// something somebody said, because the network was down, is the one outcome
// this command must never have.
func (self *Voice) rescue(audio []byte, name string) {
	dest := filepath.Join(os.TempDir(), fmt.Sprintf("orgs-rescued-%d-%s", time.Now().Unix(), name))
	if err := os.WriteFile(dest, audio, 0o600); err == nil {
		fmt.Fprintf(os.Stderr, "%sthe recording is here: %s%s\n", commands.C(commands.AnsiGold),
			dest, commands.C(commands.AnsiReset))
		fmt.Fprintf(os.Stderr, "%sorgs voice -f %s when the server is back%s\n",
			commands.C(commands.AnsiDim), dest, commands.C(commands.AnsiReset))
	}
}

// ---------------------------------------------------------------------------
// The subcommands
// ---------------------------------------------------------------------------

func (self *Voice) list(core *commands.Core) {
	recs := commands.SendReceiveGetOr[[]common.VoiceRecording](core, "voice/recordings", nil)
	if len(recs) == 0 {
		commands.Fail("the server is holding no recordings")
	}
	commands.Render(recs, func() {
		for _, r := range recs {
			filed := commands.C(commands.AnsiDim) + "not filed" + commands.C(commands.AnsiReset)
			if r.Filed != "" {
				filed = commands.C(commands.AnsiGreen) + commands.BaseName(r.Filed) +
					commands.C(commands.AnsiReset)
			}
			fmt.Printf("%s%-34s%s %6.0fs %6dkb  %s\n", commands.C(commands.AnsiBold), r.Id,
				commands.C(commands.AnsiReset), r.Seconds, r.Bytes>>10, filed)
		}
	})
}

func (self *Voice) remove(core *commands.Core, ids []string) {
	if len(ids) == 0 {
		commands.Fail("orgs voice rm <id>  -  orgs voice ls for the ids")
	}
	for _, id := range ids {
		res, err := commands.SendReceiveDelete[common.ResultMsg](core, "voice/recording/"+id, nil)
		if err == commands.ErrDryRun {
			continue
		}
		if err != nil || !res.Ok {
			commands.Fail("could not remove %s: %s %v", id, res.Msg, err)
		}
		fmt.Printf("%s✓%s removed %s\n", commands.C(commands.AnsiGreen),
			commands.C(commands.AnsiReset), id)
	}
}

func (self *Voice) config(core *commands.Core) {
	cfg := commands.SendReceiveGetOr[common.VoiceConfig](core, "voice/config", nil)
	if commands.RenderOne(cfg, nil) {
		return
	}
	state := orUnknown(cfg.State)
	ink := commands.AnsiGold
	if cfg.Ready {
		ink = commands.AnsiGreen
	}
	fmt.Printf("  %-10s %s%s%s\n", "state", commands.C(ink), state, commands.C(commands.AnsiReset))
	fmt.Printf("  %-10s %s\n", "whisper", cfg.Url)
	fmt.Printf("  %-10s %s\n", "model", cfg.Model)
	if len(cfg.Models) > 0 {
		fmt.Printf("  %-10s %s\n", "available", strings.Join(cfg.Models, ", "))
	}
	fmt.Printf("  %-10s %s\n", "language", cfg.Language)
	fmt.Printf("  %-10s %d mb\n", "limit", cfg.MaxMb)
	if len(cfg.Log) > 0 {
		fmt.Printf("\n%swhisper said:%s\n", commands.C(commands.AnsiDim),
			commands.C(commands.AnsiReset))
		for _, l := range cfg.Log {
			fmt.Printf("  %s%s%s\n", commands.C(commands.AnsiDim), l,
				commands.C(commands.AnsiReset))
		}
	}
	tool, _, _ := recorder()
	if tool == "" {
		fmt.Printf("\n%sno recorder on PATH - install sox, ffmpeg or arecord%s\n",
			commands.C(commands.AnsiGold), commands.C(commands.AnsiReset))
	} else {
		fmt.Printf("\n%srecording with %s%s\n", commands.C(commands.AnsiDim), tool,
			commands.C(commands.AnsiReset))
	}
}

// ---------------------------------------------------------------------------

func splitFirstLine(text string) (string, string) {
	lines := strings.SplitN(strings.TrimSpace(text), "\n", 2)
	head := strings.TrimSpace(lines[0])
	// A dictated note is very often one long paragraph with no newline in it at
	// all, and a heading the width of a page is not a heading. The first
	// sentence is the better guess when there is one.
	if len(lines) == 1 && len(head) > 72 {
		if i := strings.IndexAny(head, ".?!"); i > 0 && i < 72 {
			return strings.TrimSpace(head[:i+1]), strings.TrimSpace(head[i+1:])
		}
		return commands.Ellipsis(head, 72), head
	}
	if len(lines) == 1 {
		return head, ""
	}
	return head, strings.TrimSpace(lines[1])
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

func init() {
	commands.AddCmd("voice", "record a note, have it transcribed, and file it",
		func() commands.Cmd { return &Voice{} })
}
