//lint:file-ignore ST1006 allow the use of self
package orgs

// Voice notes
//
// A recording is made in the client, saved here, handed to a go-whisper server
// for transcription, and filed as an org heading. Orgs transcribes nothing
// itself: whisper is a service reached over http, which is what keeps the
// model and the hardware it wants out of this program entirely.
//
// The recording is saved before anything else happens to it, and is kept after
// filing when the note links to it. That ordering is the whole design - a
// transcription can fail in a dozen ways (whisper not running, a model still
// loading, a take longer than the timeout) and none of them should cost the
// words that were actually said.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
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

// voiceLock serialises the read/modify/write cycle when a note is filed.
var voiceLock sync.Mutex

// A recording id is its own filename, so it has to be one: no separators, no
// dots leading anywhere, and an extension we wrote ourselves.
var voiceIdRe = regexp.MustCompile(`^voice-[0-9]{8}-[0-9]{6}-[a-z0-9]{4}\.[a-z0-9]{2,5}$`)

func voiceSettings() common.VoiceSettings {
	if Conf().Server == nil {
		return common.VoiceSettings{}
	}
	return Conf().Server.Voice
}

// voiceDir resolves the recording folder, creating it on first use. A relative
// path hangs off the first orgDir so that a note's audio sits in the org
// database beside the heading that links to it.
func voiceDir() (string, error) {
	v := voiceSettings()
	p := strings.TrimSpace(v.Dir)
	root := "."
	if Conf().Server != nil && len(Conf().Server.OrgDirs) > 0 {
		root = Conf().Server.OrgDirs[0]
	}
	if p == "" {
		p = "audio"
	}
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	if err := os.MkdirAll(p, 0755); err != nil {
		return "", fmt.Errorf("could not create the voice folder %s: %s", p, err)
	}
	return p, nil
}

// voicePath maps a recording id onto its file, refusing anything that would
// escape the folder.
func voicePath(id string) (string, error) {
	if !voiceIdRe.MatchString(id) {
		return "", fmt.Errorf("%q is not a recording id", id)
	}
	dir, err := voiceDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id), nil
}

// The extension to save a recording under. The browser decides the container
// it can record in - webm on chrome, mp4 on safari - and whisper decodes
// through ffmpeg, so this follows the recording rather than converting it.
func voiceExt(mime, filename string) string {
	if ext := strings.ToLower(filepath.Ext(filename)); len(ext) > 1 && len(ext) <= 6 {
		if ok, _ := regexp.MatchString(`^\.[a-z0-9]{2,5}$`, ext); ok {
			return ext
		}
	}
	m := strings.ToLower(mime)
	if i := strings.IndexAny(m, ";"); i >= 0 {
		m = m[:i]
	}
	switch strings.TrimSpace(m) {
	case "audio/webm", "video/webm":
		return ".webm"
	case "audio/ogg", "audio/opus":
		return ".ogg"
	case "audio/mp4", "audio/m4a", "audio/x-m4a":
		return ".m4a"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/wav", "audio/wave", "audio/x-wav":
		return ".wav"
	case "audio/flac":
		return ".flac"
	}
	return ".webm"
}

var voiceMimes = map[string]string{
	".webm": "audio/webm",
	".ogg":  "audio/ogg",
	".m4a":  "audio/mp4",
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".flac": "audio/flac",
}

func voiceMime(id string) string {
	if m, ok := voiceMimes[strings.ToLower(filepath.Ext(id))]; ok {
		return m
	}
	return "application/octet-stream"
}

const voiceLetters = "abcdefghijklmnopqrstuvwxyz0123456789"

// A new id. The time is in it so the folder reads in order in any file
// browser; the four random characters are only there so that two recordings
// finished in the same second cannot land on the same name.
func newVoiceId(now time.Time, ext string) string {
	n := now.UnixNano()
	tail := make([]byte, 4)
	for i := range tail {
		tail[i] = voiceLetters[n%int64(len(voiceLetters))]
		n /= int64(len(voiceLetters))
	}
	return fmt.Sprintf("voice-%s-%s%s", now.Format("20060102-150405"), string(tail), ext)
}

// The seconds a recording ran to, kept beside it. The browser knows this and
// the container often does not say, so rather than decode the audio here to
// find out, the number it reports is written to a small sidecar file.
func voiceMetaPath(path string) string { return path + ".meta" }

func writeVoiceMeta(path string, seconds float64) {
	if seconds <= 0 {
		return
	}
	_ = os.WriteFile(voiceMetaPath(path), []byte(strconv.FormatFloat(seconds, 'f', 2, 64)), 0644)
}

func readVoiceMeta(path string) float64 {
	b, err := os.ReadFile(voiceMetaPath(path))
	if err != nil {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
	if err != nil {
		return 0
	}
	return f
}

// ----------------------------------------------------------------------------
// The whisper server
// ----------------------------------------------------------------------------

func whisperUrl(path string) string {
	return whisperBase() + "/api/whisper" + path
}

// whisperModels asks the server what it can transcribe with. Kept on a short
// timeout of its own: this is asked every time the client opens the voice
// panel, and a whisper server that is not running should say so at once rather
// than hang the panel.
func whisperModels() ([]string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Get(whisperUrl("/model"))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return nil, fmt.Errorf("whisper answered %s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	// Two shapes are in the wild here: the api doc describes an object with a
	// models field, and the server this was tested against answers with the
	// bare array. Both are read, because guessing wrong is silent - an empty
	// model list looks exactly like a server with nothing installed.
	type model struct {
		Id string `json:"id"`
	}
	var list []model
	if err := json.Unmarshal(body, &list); err != nil {
		var wrapped struct {
			Models []model `json:"models"`
		}
		if err := json.Unmarshal(body, &wrapped); err != nil {
			return nil, fmt.Errorf("whisper sent back something that is not a model list: %s", err)
		}
		list = wrapped.Models
	}
	names := []string{}
	for _, m := range list {
		if m.Id != "" {
			names = append(names, m.Id)
		}
	}
	return names, nil
}

// The end of a transport error, which is the part that says what happened.
func whisperReason(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 && i+2 < len(msg) {
		tail := strings.TrimSpace(msg[i+2:])
		if tail != "" {
			return tail
		}
	}
	return msg
}

// The model a transcription should use: the configured one, or the server's
// first when nothing is configured - which is right for a machine with one
// model installed and deliberately arbitrary as soon as there are two, so the
// setting is worth filling in.
func whisperModel(asked string) (string, error) {
	if m := strings.TrimSpace(asked); m != "" {
		return m, nil
	}
	if m := strings.TrimSpace(voiceSettings().Model); m != "" {
		return m, nil
	}
	models, err := whisperModels()
	if err != nil {
		return "", err
	}
	if len(models) == 0 {
		return "", fmt.Errorf("the whisper server has no models installed - download one with `gowhisper download-model ggml-medium-q5_0`")
	}
	return models[0], nil
}

// whisperTranscribe posts one recording and reads the transcription back.
//
// The file is streamed into the request rather than read into memory first: a
// long take is tens of megabytes and there is no reason for this process to
// hold a copy of it.
func whisperTranscribe(path, model, language, prompt string) (*common.VoiceTranscription, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		var werr error
		defer func() { pw.CloseWithError(werr) }()
		part, err := mw.CreateFormFile("audio", filepath.Base(path))
		if err != nil {
			werr = err
			return
		}
		if _, err := io.Copy(part, f); err != nil {
			werr = err
			return
		}
		fields := map[string]string{
			"model":    model,
			"filename": filepath.Base(path),
		}
		if language != "" {
			fields["language"] = language
		}
		if prompt != "" {
			fields["prompt"] = prompt
		}
		for k, v := range fields {
			if err := mw.WriteField(k, v); err != nil {
				werr = err
				return
			}
		}
		werr = mw.Close()
	}()

	timeout := voiceSettings().Timeout
	if timeout <= 0 {
		timeout = 600
	}
	req, err := http.NewRequest("POST", whisperUrl("/transcribe"), pr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach the whisper server at %s: %s", voiceSettings().Url, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		msg := strings.TrimSpace(string(body))
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(msg), &e) == nil && e.Error != "" {
			msg = e.Error
		}
		return nil, fmt.Errorf("whisper answered %s: %s", res.Status, msg)
	}

	var out struct {
		Text     string  `json:"text"`
		Language string  `json:"language"`
		Duration float64 `json:"duration"`
		Segments []struct {
			Id      int     `json:"id"`
			Start   float64 `json:"start"`
			End     float64 `json:"end"`
			Text    string  `json:"text"`
			Speaker string  `json:"speaker"`
		} `json:"segments"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("whisper sent back something that is not a transcription: %s", err)
	}

	t := &common.VoiceTranscription{
		Ok:       true,
		Text:     strings.TrimSpace(out.Text),
		Language: out.Language,
		Duration: out.Duration,
		Model:    model,
		Segments: []common.VoiceSegment{},
	}
	for _, s := range out.Segments {
		t.Segments = append(t.Segments, common.VoiceSegment{
			Id: s.Id, Start: s.Start, End: s.End,
			Text: strings.TrimSpace(s.Text), Speaker: s.Speaker,
		})
	}
	// Whisper fills in text or segments depending on how it was asked; when
	// only the segments came back the text is what they say, joined.
	if t.Text == "" && len(t.Segments) > 0 {
		parts := []string{}
		for _, s := range t.Segments {
			if s.Text != "" {
				parts = append(parts, s.Text)
			}
		}
		t.Text = strings.Join(parts, " ")
	}
	return t, nil
}

// ----------------------------------------------------------------------------
// Writing the note
// ----------------------------------------------------------------------------

// A duration said the way a person would: 0:42, 3:07, 1:02:30.
func voiceDuration(seconds float64) string {
	if seconds <= 0 {
		return ""
	}
	t := int(seconds + 0.5)
	h, m, s := t/3600, (t/60)%60, t%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// A timestamp inside a transcript, which is a position in the recording rather
// than a time of day, so it is written mm:ss and not as an org timestamp.
func voiceStamp(seconds float64) string {
	t := int(seconds)
	return fmt.Sprintf("%d:%02d", t/60, t%60)
}

// The text of the heading a voice note becomes.
//
// Built here as lines rather than handed to the org writer, because writing it
// through go-org would rewrite the whole target file - every drawer re-indented
// and every table reflowed - to add one heading to the end of it.
func voiceNoteLines(req *common.VoiceNoteRequest, lvl int, audioLink string, now time.Time) []string {
	indent := strings.Repeat(" ", lvl+1)
	head := strings.Repeat("*", lvl) + " " + req.Headline
	if len(req.Tags) > 0 {
		head += "  :" + strings.Join(req.Tags, ":") + ":"
	}
	lines := []string{head}

	props := [][2]string{{"CREATED", now.Format("[2006-01-02 Mon 15:04]")}}
	if audioLink != "" {
		props = append(props, [2]string{"AUDIO", "[[file:" + audioLink + "]]"})
	}
	if d := voiceDuration(req.Seconds); d != "" {
		props = append(props, [2]string{"DURATION", d})
	}
	if req.Model != "" {
		props = append(props, [2]string{"WHISPER", req.Model})
	}
	for k, v := range req.Props {
		if k != "" && v != "" {
			props = append(props, [2]string{strings.ToUpper(k), v})
		}
	}
	lines = append(lines, indent+":PROPERTIES:")
	for _, p := range props {
		lines = append(lines, indent+":"+p[0]+": "+p[1])
	}
	lines = append(lines, indent+":END:")

	for _, l := range strings.Split(strings.TrimRight(req.Text, "\n"), "\n") {
		l = strings.TrimRight(l, " \t")
		if l == "" {
			lines = append(lines, "")
		} else {
			lines = append(lines, indent+l)
		}
	}

	if req.WithTimes && len(req.Segments) > 0 {
		lines = append(lines, "")
		for _, s := range req.Segments {
			if strings.TrimSpace(s.Text) == "" {
				continue
			}
			who := ""
			if s.Speaker != "" {
				who = s.Speaker + ": "
			}
			lines = append(lines, indent+"- "+voiceStamp(s.Start)+" :: "+who+strings.TrimSpace(s.Text))
		}
	}
	return lines
}

// insertLinesAt splices lines into a file after the given row, and answers
// with the line number the first of them landed on.
//
// The rows either side are left exactly as they were: a voice note is added to
// a file somebody else is also editing, and the least this can do is not
// rewrite the parts of it that have nothing to do with the note.
func insertLinesAt(filename string, row int, add []string) (int, error) {
	f, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	serr := scanner.Err()
	f.Close()
	if serr != nil {
		return 0, serr
	}

	at := row + 1
	if at < 0 {
		at = 0
	}
	if at > len(lines) {
		at = len(lines)
	}
	out := make([]string, 0, len(lines)+len(add))
	out = append(out, lines[:at]...)
	out = append(out, add...)
	out = append(out, lines[at:]...)

	body := strings.Join(out, "\n")
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	if err := os.WriteFile(filename, []byte(body), 0644); err != nil {
		return 0, err
	}
	return at + 1, nil
}

// ----------------------------------------------------------------------------
// REST handlers
// ----------------------------------------------------------------------------

func voiceJson(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

/* SDOC: API
* GET /voice/config — What Voice Notes Can Do Right Now
	Answers with the voice note settings and whether the transcription service is
	actually there, so that a client can say "whisper is not running" before somebody
	records three minutes of audio and finds out afterwards.

	*Method:* =GET=

	*Parameters:* None.

	*Response:* A =VoiceConfig= JSON object:
	| Field      | Type     | Description                                                |
	|------------+----------+------------------------------------------------------------|
	| =ready=    | bool     | The whisper server answered and has at least one model.    |
	| =url=      | string   | Where the whisper server is configured to be.              |
	| =model=    | string   | The model a transcription would use right now.             |
	| =models=   | []string | Every model the whisper server has.                        |
	| =language= | string   | The configured two letter language code, or empty.         |
	| =maxMb=    | int      | The largest recording that will be accepted.               |
	| =tags=     | []string | Tags put on every voice note heading.                      |
	| =target=   | Target   | Where a note is filed when the client does not say.        |

	=Msg= carries the reason when =ready= is false - unreachable, no models, an
	error from whisper itself - and is meant to be shown to the user as it is.

	*Errors:* =401= if not authenticated. A whisper server that is down is not an
	error here: it is =ready: false= with a reason.
	EDOC */
func RequestVoiceConfig(w http.ResponseWriter, r *http.Request) {
	v := voiceSettings()
	state, stateMsg, log := WhisperStatus()
	out := common.VoiceConfig{
		Ok:       true,
		Url:      whisperBase(),
		Language: v.Language,
		MaxMb:    v.MaxMb,
		Tags:     v.Tags,
		Target:   v.Target,
		Models:   []string{},
		Managed:  whisperManaged(),
		State:    state,
		Log:      []string{},
	}
	models, err := whisperModels()
	if err != nil {
		// Said the way it will be read: the client puts this in front of
		// somebody who is about to record, and "dial tcp: connect: connection
		// refused" does not tell them which of their two servers is down.
		// While orgs is still starting whisper that is not a fault at all, so
		// the supervisor gets to speak first.
		switch {
		case out.Managed && state == WhisperStarting:
			out.Msg = stateMsg
			if out.Msg == "" {
				out.Msg = "starting whisper - a large model takes a little while to load"
			}
		case out.Managed && stateMsg != "":
			out.Msg = stateMsg
			out.Log = log
		case strings.TrimSpace(v.Models) == "" && strings.TrimSpace(v.Url) == "":
			// Nothing has been configured at all, so the useful thing to say is
			// how to configure it rather than which server did not answer.
			out.Msg = "voice is not set up yet: point voice.models at a whisper models directory and orgs will run whisper itself."
		default:
			out.Msg = fmt.Sprintf("no answer from the whisper server at %s - %s", out.Url, whisperReason(err))
			out.Log = log
		}
		voiceJson(w, out)
		return
	}
	out.Models = models
	if m := strings.TrimSpace(v.Model); m != "" {
		out.Model = m
	} else if len(models) > 0 {
		out.Model = models[0]
	}
	if len(models) == 0 {
		out.Msg = "the whisper server is running but has no models installed"
	} else {
		out.Ready = true
	}
	voiceJson(w, out)
}

/* SDOC: API
* POST /voice/recording — Save a Recording
	Takes one recording and keeps it. Nothing is transcribed and nothing is written
	into an org file: this exists so that the words are safe on disk before anything
	that can fail is attempted on them.

	*Method:* =POST= (=multipart/form-data=)

	*Form Fields:*
	| Field      | Type   | Required | Description                                          |
	|------------+--------+----------+------------------------------------------------------|
	| =audio=    | file   | yes      | The recording, in whatever container the client has. |
	| =seconds=  | number | no       | How long the take ran, as the client timed it.       |
	| =filename= | string | no       | The client's own name for it, used for its extension.|

	The container is not converted - whisper decodes through ffmpeg, so webm from a
	browser, m4a from a phone and wav from a recorder are all acceptable.

	*Response:* A =VoiceRecording= JSON object, whose =id= addresses it in every
	other call here.

	*Errors:*
	- =401= if not authenticated.
	- =400= if there is no audio field.
	- =413= if the recording is larger than the configured =maxMb=.
	EDOC */
func PostVoiceRecording(w http.ResponseWriter, r *http.Request) {
	maxMb := voiceSettings().MaxMb
	if maxMb <= 0 {
		maxMb = 64
	}
	limit := int64(maxMb) << 20
	r.Body = http.MaxBytesReader(w, r.Body, limit+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		voiceJson(w, common.ResultMsg{Ok: false, Msg: fmt.Sprintf("that recording is too big - the limit is %d mb", maxMb)})
		return
	}
	file, header, err := r.FormFile("audio")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		voiceJson(w, common.ResultMsg{Ok: false, Msg: "no audio was sent"})
		return
	}
	defer file.Close()

	dir, err := voiceDir()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		voiceJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	mime := ""
	name := ""
	if header != nil {
		mime = header.Header.Get("Content-Type")
		name = header.Filename
	}
	if n := strings.TrimSpace(r.FormValue("filename")); n != "" {
		name = n
	}
	id := newVoiceId(time.Now(), voiceExt(mime, name))
	path := filepath.Join(dir, id)

	out, err := os.Create(path)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		voiceJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	written, cerr := io.Copy(out, io.LimitReader(file, limit+1))
	out.Close()
	if cerr != nil || written > limit {
		os.Remove(path)
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		voiceJson(w, common.ResultMsg{Ok: false, Msg: fmt.Sprintf("that recording is too big - the limit is %d mb", maxMb)})
		return
	}

	seconds, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("seconds")), 64)
	writeVoiceMeta(path, seconds)

	voiceJson(w, common.VoiceRecording{
		Id:      id,
		Bytes:   written,
		Seconds: seconds,
		Created: time.Now().Format(time.RFC3339),
		Mime:    voiceMime(id),
	})
}

/* SDOC: API
* GET /voice/recordings — List Saved Recordings
	Every recording the server is holding, newest first. The list is the folder
	itself rather than an index kept beside it, so a recording deleted by hand is
	gone here too and nothing can fall out of step.

	*Method:* =GET=

	*Response:* A JSON array of =VoiceRecording= objects.

	*Errors:* =401= if not authenticated.
	EDOC */
func RequestVoiceRecordings(w http.ResponseWriter, r *http.Request) {
	dir, err := voiceDir()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		voiceJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		voiceJson(w, []common.VoiceRecording{})
		return
	}
	out := []common.VoiceRecording{}
	for _, e := range entries {
		if e.IsDir() || !voiceIdRe.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, e.Name())
		out = append(out, common.VoiceRecording{
			Id:      e.Name(),
			Bytes:   info.Size(),
			Seconds: readVoiceMeta(path),
			Created: info.ModTime().Format(time.RFC3339),
			Mime:    voiceMime(e.Name()),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Id > out[j].Id })
	voiceJson(w, out)
}

/* SDOC: API
* GET /voice/recording/{id} — Play a Recording Back
	Serves one recording's bytes, with its own content type and range requests
	supported, so that a client can seek within a long take rather than fetch the
	whole of it to hear the end.

	*Method:* =GET=

	*Path Parameters:*
	| Parameter | Type   | Description                                  |
	|-----------+--------+----------------------------------------------|
	| =id=      | string | The recording id, as =/voice/recordings= lists it. |

	*Response:* The audio itself.

	*Errors:*
	- =401= if not authenticated.
	- =404= if there is no recording with that id.
	EDOC */
func RequestVoiceAudio(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	path, err := voicePath(id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		voiceJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	f, err := os.Open(path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		voiceJson(w, common.ResultMsg{Ok: false, Msg: "no recording with that id"})
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", voiceMime(id))
	// ServeContent does the range handling, which is what lets a player seek.
	http.ServeContent(w, r, id, info.ModTime(), f)
}

/* SDOC: API
* DELETE /voice/recording/{id} — Throw a Recording Away
	Deletes one recording. This is not undoable and nothing else is touched: a note
	already filed keeps its heading and its transcript, and is left with an =AUDIO=
	link pointing at a file that is no longer there.

	*Method:* =DELETE=

	*Path Parameters:*
	| Parameter | Type   | Description        |
	|-----------+--------+--------------------|
	| =id=      | string | The recording id.  |

	*Response:* A =ResultMsg=.

	*Errors:*
	- =401= if not authenticated.
	- =404= if there is no recording with that id.
	EDOC */
func DeleteVoiceRecording(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	path, err := voicePath(id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		voiceJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	if err := os.Remove(path); err != nil {
		w.WriteHeader(http.StatusNotFound)
		voiceJson(w, common.ResultMsg{Ok: false, Msg: "no recording with that id"})
		return
	}
	os.Remove(voiceMetaPath(path))
	voiceJson(w, common.ResultMsg{Ok: true, Msg: fmt.Sprintf("recording %s deleted", id)})
}

/* SDOC: API
* POST /voice/transcribe — Transcribe a Saved Recording
	Hands one saved recording to the go-whisper server and answers with what it
	heard. Nothing is written into an org file here - the transcript goes back to
	the client to be read and corrected first, because a transcript nobody has
	looked at is a guess, and an org file is not the place to find that out.

	*Method:* =POST=

	*Request Body (JSON):*
	| Field      | Type   | Required | Description                                             |
	|------------+--------+----------+---------------------------------------------------------|
	| =id=       | string | yes      | The recording to transcribe.                            |
	| =model=    | string | no       | Override the configured model for this one call.        |
	| =language= | string | no       | Two letter code; empty lets whisper detect it.          |
	| =prompt=   | string | no       | Names and jargon to expect, which biases whisper's guesses. |

	*Response:* A =VoiceTranscription= with the whole =text=, the =language= whisper
	settled on, the =duration= it decoded, and the =segments= with their start and
	end in seconds.

	*Errors:*
	- =401= if not authenticated.
	- =404= if there is no recording with that id.
	- =502= if whisper could not be reached, has no model, or failed on the audio.
	  =Msg= is whisper's own words and is meant to be shown as it is.
	EDOC */
func PostVoiceTranscribe(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.VoiceTranscribeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		voiceJson(w, common.VoiceTranscription{Ok: false, Msg: err.Error()})
		return
	}
	path, err := voicePath(req.Id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		voiceJson(w, common.VoiceTranscription{Ok: false, Msg: err.Error()})
		return
	}
	if _, err := os.Stat(path); err != nil {
		w.WriteHeader(http.StatusNotFound)
		voiceJson(w, common.VoiceTranscription{Ok: false, Msg: "no recording with that id"})
		return
	}
	model, err := whisperModel(req.Model)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		voiceJson(w, common.VoiceTranscription{Ok: false, Msg: fmt.Sprintf("no answer from the whisper server at %s - %s", voiceSettings().Url, whisperReason(err))})
		return
	}
	lang := req.Language
	if lang == "" {
		lang = voiceSettings().Language
	}
	t, err := whisperTranscribe(path, model, lang, req.Prompt)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		voiceJson(w, common.VoiceTranscription{Ok: false, Msg: err.Error(), Model: model})
		return
	}
	// Whisper knows how long the audio really was; the browser only knew how
	// long it had been recording for, so this is the better number to keep.
	if t.Duration > 0 {
		writeVoiceMeta(path, t.Duration)
	}
	voiceJson(w, t)
}

/* SDOC: API
* POST /voice/note — File a Voice Note as an Org Heading
	Writes the note into an org file: a heading, a property drawer carrying the link
	to the recording and what transcribed it, and the text underneath.

	The text written is the text sent, never a fresh transcription - what lands in
	the file has to be what was on screen when the button was pressed, corrections
	and all.

	*Method:* =POST=

	*Request Body (JSON):* A =VoiceNoteRequest=.
	| Field       | Type      | Required | Description                                                  |
	|-------------+-----------+----------+--------------------------------------------------------------|
	| =headline=  | string    | yes      | The heading text.                                            |
	| =text=      | string    | no       | The body, as edited. Line breaks are kept.                   |
	| =id=        | string    | no       | The recording this came from.                                |
	| =target=    | Target    | no       | Where to file it; the configured target when left out.       |
	| =tags=      | []string  | no       | Tags for the heading, on top of the configured ones.         |
	| =props=     | map       | no       | Extra properties for the drawer.                             |
	| =segments=  | []Segment | no       | The timestamped segments, written as a list when =withTimes=.|
	| =withTimes= | bool      | no       | Write the segments under the text.                           |
	| =keepAudio= | bool      | no       | Keep the recording and link to it. Off deletes it.           |
	| =model=     | string    | no       | What transcribed it, written as the =WHISPER= property.      |
	| =seconds=   | number    | no       | The length, written as the =DURATION= property.              |

	The =AUDIO= property is a file link written relative to the org file that holds
	it, so moving the org directory as a whole does not break it.

	*Response:* A =VoiceNoteResult= with the =filename= and the =line= the heading
	landed on.

	*Errors:*
	- =401= if not authenticated.
	- =400= if there is no headline.
	- =404= if the target cannot be found or created.
	EDOC */
func PostVoiceNote(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.VoiceNoteRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		voiceJson(w, common.VoiceNoteResult{Ok: false, Msg: err.Error()})
		return
	}
	req.Headline = strings.TrimSpace(req.Headline)
	if req.Headline == "" {
		w.WriteHeader(http.StatusBadRequest)
		voiceJson(w, common.VoiceNoteResult{Ok: false, Msg: "a voice note needs a headline"})
		return
	}

	v := voiceSettings()
	target := req.Target
	if target.Type == "" && target.Filename == "" && target.Id == "" {
		target = v.Target
	}
	if target.Type == "" && target.Filename != "" {
		if target.Id != "" {
			target.Type = "file+headline"
		} else {
			target.Type = "file"
		}
	}
	if target.Type == "" {
		w.WriteHeader(http.StatusBadRequest)
		voiceJson(w, common.VoiceNoteResult{Ok: false, Msg: "there is nowhere to file this - say a target, or set voice.target in the config"})
		return
	}

	// The configured tags and the client's, without repeating one.
	tags := []string{}
	seen := map[string]bool{}
	for _, t := range append(append([]string{}, v.Tags...), req.Tags...) {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		tags = append(tags, t)
	}
	req.Tags = tags

	voiceLock.Lock()
	defer voiceLock.Unlock()

	file, sec := GetDb().GetFromTarget(&target, true)
	if file == nil || sec == nil || file.Doc == nil {
		w.WriteHeader(http.StatusNotFound)
		voiceJson(w, common.VoiceNoteResult{Ok: false, Msg: fmt.Sprintf("could not find anywhere to file this [%s %s %s]", target.Type, target.Filename, target.Id)})
		return
	}
	filename := file.Doc.Path

	// The link is written relative to the org file that holds it, so that
	// moving the whole org directory keeps every note's audio.
	audioLink := ""
	audioPath := ""
	if req.Id != "" && req.KeepAudio {
		if p, err := voicePath(req.Id); err == nil {
			audioPath = p
			if rel, err := filepath.Rel(filepath.Dir(filename), p); err == nil && !strings.HasPrefix(rel, "..") {
				audioLink = filepath.ToSlash(rel)
			} else {
				audioLink = filepath.ToSlash(p)
			}
		}
	}

	lvl := 1
	if sec.Headline != nil && sec.Headline.Lvl > 0 {
		lvl = sec.Headline.Lvl + 1
	}
	lines := voiceNoteLines(&req, lvl, audioLink, time.Now())

	pos := EndRow(sec, "")
	row := 0
	if pos != nil {
		row = pos.Row
	}
	line, err := insertLinesAt(filename, row, lines)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		voiceJson(w, common.VoiceNoteResult{Ok: false, Msg: err.Error()})
		return
	}

	// The recording is only thrown away once the words are safely in the file.
	if req.Id != "" && !req.KeepAudio {
		if p, err := voicePath(req.Id); err == nil {
			os.Remove(p)
			os.Remove(voiceMetaPath(p))
		}
	} else if audioPath != "" {
		writeVoiceMeta(audioPath, req.Seconds)
	}

	voiceJson(w, common.VoiceNoteResult{
		Ok:       true,
		Msg:      fmt.Sprintf("filed under %s", filepath.Base(filename)),
		Filename: filename,
		Line:     line,
		Headline: req.Headline,
		Audio:    audioLink,
	})
}

/* SDOC: API
* POST /voice/whisper/restart — Start the Transcription Service Again
	Stops the go-whisper server orgs is running and starts it again. This is the
	answer to a model that failed to load or a service that has stopped answering,
	and it is deliberately the only control over it: what to run is configuration,
	not something a client gets to choose.

	*Method:* =POST=

	*Request Body:* None.

	*Response:* A =ResultMsg=. The restart is not waited for - a large model takes
	a while to load - so watch =GET /voice/config= for =state= to come back to
	=ready=.

	*Errors:*
	- =401= if not authenticated.
	- =400= if orgs is not the one running whisper, which is the case when
	  =voice.models= is unset or =voice.url= names somebody else's server.
	EDOC */
func PostWhisperRestart(w http.ResponseWriter, r *http.Request) {
	if err := RestartWhisper(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		voiceJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	voiceJson(w, common.ResultMsg{Ok: true, Msg: "starting whisper again"})
}
