package orgs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

)

// Speaking into a heading that already exists.
//
// `/voice/note` files a recording as a **new heading** under a target, which
// is what the Voice Notes tab wants. A kanban card is the other case: the
// heading is already there and is what the card is about, so the words go into
// its own body and the recording is linked from its own `:AUDIO:` property.
//
// The recording is already saved by `/voice/recording` before any of this is
// attempted - the same order the voice notes take, and for the same reason: a
// transcription fails in a dozen ways and none of them should cost the words
// that were said. And like the voice notes, the text written is **the text
// sent**, never a fresh transcription, because what lands in the file has to
// be what was on screen.
//
// Every write here is a line splice rather than a go-org rewrite. Appending a
// paragraph to one heading should not reformat every drawer and table in the
// file it happens to live in.

type VoiceAppendRequest struct {
	Hash string `json:"hash"`
	// What to write. Sent rather than transcribed again here.
	Text string `json:"text"`
	// The recording this came from, if it is being kept.
	Id        string  `json:"id"`
	KeepAudio bool    `json:"keepAudio"`
	Seconds   float64 `json:"seconds"`
	Model     string  `json:"model"`
}

type VoiceAppendResult struct {
	Ok       bool   `json:"Ok"`
	Msg      string `json:"Msg"`
	Filename string `json:"filename"`
	Line     int    `json:"line"`
	Audio    string `json:"audio"`
}

var propsOpenRe = regexp.MustCompile(`(?i)^(\s*):PROPERTIES:\s*$`)
var propsEndRe = regexp.MustCompile(`(?i)^\s*:END:\s*$`)
var audioPropRe = regexp.MustCompile(`(?i)^(\s*):AUDIO:\s`)

// The indent a heading's body is written at: whatever its first body line
// uses, and two spaces past the stars when it has none. A note appended at
// column zero under an indented heading reads as a different heading's text.
func bodyIndent(lines []string, from, to, lvl int) string {
	for i := from; i <= to && i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		trimmed := strings.TrimLeft(line, " \t")
		return line[:len(line)-len(trimmed)]
	}
	return strings.Repeat(" ", lvl+1)
}

func PostVoiceAppend(w http.ResponseWriter, r *http.Request) {
	var res VoiceAppendResult
	body, _ := io.ReadAll(r.Body)
	var req VoiceAppendRequest
	if err := json.Unmarshal(body, &req); err != nil {
		res.Msg = err.Error()
		voiceAppendJson(w, res)
		return
	}
	if strings.TrimSpace(req.Text) == "" && !req.KeepAudio {
		res.Msg = "nothing to write"
		voiceAppendJson(w, res)
		return
	}

	voiceLock.Lock()
	defer voiceLock.Unlock()

	sec := GetDb().FindByHash(req.Hash)
	if sec == nil || sec.Headline == nil {
		res.Msg = "no heading with that hash"
		voiceAppendJson(w, res)
		return
	}
	filename, lines, from, to, err := headingBodyLines(req.Hash)
	if err != nil {
		res.Msg = err.Error()
		voiceAppendJson(w, res)
		return
	}
	res.Filename = filename
	lvl := sec.Headline.Lvl
	indent := bodyIndent(lines, from, to, lvl)

	// The link is written relative to the org file that holds it, so that
	// moving the whole org directory keeps every note's audio.
	audioLink := ""
	if req.Id != "" && req.KeepAudio {
		if p, perr := voicePath(req.Id); perr == nil {
			if _, serr := os.Stat(p); serr == nil {
				if rel, rerr := filepath.Rel(filepath.Dir(filename), p); rerr == nil && !strings.HasPrefix(rel, "..") {
					audioLink = filepath.ToSlash(rel)
				} else {
					audioLink = filepath.ToSlash(p)
				}
			}
		}
	}

	// A heading that already has an `:AUDIO:` keeps it. The property holds one
	// link and this is the second recording somebody has spoken into the same
	// heading, so the new one goes in the body beside its own words - losing a
	// link to a recording quietly is worse than having them in two places, and
	// the recording is the thing that cannot be made again.
	hasAudio := false
	for i := from; i <= to && i < len(lines); i++ {
		if audioPropRe.MatchString(lines[i]) {
			hasAudio = true
			break
		}
	}

	now := time.Now()
	add := []string{}
	if strings.TrimSpace(req.Text) != "" || (audioLink != "" && hasAudio) {
		add = append(add, "")
		add = append(add, indent+now.Format("[2006-01-02 Mon 15:04]"))
		if audioLink != "" && hasAudio {
			add = append(add, indent+"[[file:"+audioLink+"]]")
		}
	}
	if strings.TrimSpace(req.Text) != "" {
		for _, l := range strings.Split(strings.TrimRight(req.Text, "\n"), "\n") {
			l = strings.TrimRight(l, " \t")
			if l == "" {
				add = append(add, "")
			} else {
				add = append(add, indent+l)
			}
		}
	}

	// Append after the heading's own body, before any child heading - which is
	// what `to` already is.
	at := to + 1
	if at > len(lines) {
		at = len(lines)
	}
	out := append([]string{}, lines[:at]...)
	out = append(out, add...)
	out = append(out, lines[at:]...)

	// And the property, written into the drawer the heading already has or a
	// new one right under the headline - only when there was not one already.
	if audioLink != "" {
		if !hasAudio {
			out = setAudioProperty(out, sec.Headline.Pos.Row, indent, "[[file:"+audioLink+"]]")
		}
		res.Audio = audioLink
	}

	if werr := os.WriteFile(filename, []byte(strings.Join(out, "\n")+"\n"), 0644); werr != nil {
		res.Msg = werr.Error()
		voiceAppendJson(w, res)
		return
	}
	res.Ok = true
	res.Line = at + 1
	res.Msg = fmt.Sprintf("appended %d line(s)", len(add))
	voiceAppendJson(w, res)
}

// Put `:AUDIO:` on the heading at `headRow`, replacing whatever it said.
//
// Three cases: the heading has a drawer with the property, has a drawer
// without it, or has no drawer at all. The third writes one directly under the
// headline, which is where org puts it and where the readers here look.
func setAudioProperty(lines []string, headRow int, indent, value string) []string {
	if headRow < 0 || headRow >= len(lines) {
		return lines
	}
	// The drawer, if there is one, is within the heading's own lines - stop at
	// the next heading of any level.
	end := len(lines)
	for i := headRow + 1; i < len(lines); i++ {
		stars := 0
		for stars < len(lines[i]) && lines[i][stars] == '*' {
			stars++
		}
		if stars > 0 && stars < len(lines[i]) && lines[i][stars] == ' ' {
			end = i
			break
		}
	}

	openAt, endAt := -1, -1
	for i := headRow + 1; i < end; i++ {
		if openAt < 0 && propsOpenRe.MatchString(lines[i]) {
			openAt = i
			continue
		}
		if openAt >= 0 && propsEndRe.MatchString(lines[i]) {
			endAt = i
			break
		}
	}

	line := indent + "  :AUDIO: " + value
	if openAt >= 0 && endAt > openAt {
		for i := openAt + 1; i < endAt; i++ {
			if audioPropRe.MatchString(lines[i]) {
				lines[i] = line
				return lines
			}
		}
		out := append([]string{}, lines[:endAt]...)
		out = append(out, line)
		return append(out, lines[endAt:]...)
	}

	out := append([]string{}, lines[:headRow+1]...)
	out = append(out, indent+":PROPERTIES:", line, indent+":END:")
	return append(out, lines[headRow+1:]...)
}

func voiceAppendJson(w http.ResponseWriter, res VoiceAppendResult) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
