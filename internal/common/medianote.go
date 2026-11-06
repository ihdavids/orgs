package common

// org-media-note's links, read and written the way it does, so a file of notes
// made in Emacs opens here and the other way round:
//
//	- [[video:lectures/week3.mp4#0:12:30][0:12:30]] the proof starts here
//	- [[audio:talk.mp3#0:01:23-0:01:40][0:01:23-0:01:40]] the bit worth hearing again
//	- [[videocite:smith2020#0:05:00][0:05:00]] a cited recording (its bib entry's file or url)
//	- [[video:https://www.youtube.com/watch?v=dQw4w9WgXcQ#0:00:43][0:00:43]]
//
// The time is org-timer's H:MM:SS (org-media-note-timestamp-pattern hms), or
// H:MM:SS.fff (hmsf); an A-B loop is two of them joined by a dash. MM:SS and
// plain seconds are read too, because people type them.
//
// Pure: used by the server (medianote.go), the html exporter and the CLI. The
// same rules are in worg's src/medianote.ts and vscode's
// src/media-note-model.ts, pinned by the same cases in each test.

import (
	"fmt"
	"math"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// The link types org-media-note defines.
var MediaLinkTypes = []string{"video", "audio", "videocite", "audiocite"}

// org-media-note--video-types and --audio-types, plus webm/m4v/oga/weba
// which browsers record and play.
var mediaVideoExt = map[string]bool{"avi": true, "rmvb": true, "ogg": true, "ogv": true, "mp4": true, "mkv": true, "mov": true,
	"webm": true, "flv": true, "ts": true, "mpg": true, "m4v": true}
var mediaAudioExt = map[string]bool{"flac": true, "mp3": true, "wav": true, "m4a": true, "aac": true, "opus": true, "oga": true, "weba": true}

// MediaKindOf is "video" or "audio" for a file name by its extension, "" for
// neither.
func MediaKindOf(name string) string {
	if i := strings.IndexAny(name, "?#"); i >= 0 {
		name = name[:i]
	}
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
	switch {
	case mediaAudioExt[ext]:
		return "audio"
	case mediaVideoExt[ext]:
		return "video"
	}
	return ""
}

var mediaStampRe = regexp.MustCompile(`^(?:(\d+):)?(\d{1,2}):(\d{1,2})(?:[.,](\d+))?$`)
var mediaSecondsRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)s?$`)

// ParseMediaStamp reads 0:12:30, 12:30, 0:12:30.250, 754 or 754.5s as
// seconds.
func ParseMediaStamp(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if m := mediaStampRe.FindStringSubmatch(s); m != nil {
		h, _ := strconv.Atoi(m[1])
		mi, _ := strconv.Atoi(m[2])
		se, _ := strconv.Atoi(m[3])
		if m[1] != "" && (mi > 59 || se > 59) {
			return 0, false
		}
		if m[1] == "" && se > 59 {
			return 0, false
		}
		t := float64(h*3600 + mi*60 + se)
		if m[4] != "" {
			f, _ := strconv.ParseFloat("0."+m[4], 64)
			t += f
		}
		return t, true
	}
	if m := mediaSecondsRe.FindStringSubmatch(s); m != nil {
		f, _ := strconv.ParseFloat(m[1], 64)
		return f, true
	}
	return 0, false
}

// MediaStamp writes seconds the way org-media-note does: org-timer's H:MM:SS
// (0:12:30), rounded; with fraction, H:MM:SS.fff.
func MediaStamp(seconds float64, fraction bool) string {
	if seconds < 0 || math.IsNaN(seconds) {
		seconds = 0
	}
	if fraction {
		whole := math.Floor(seconds)
		ms := int(math.Round((seconds - whole) * 1000))
		if ms == 1000 {
			whole++
			ms = 0
		}
		t := int(whole)
		return fmt.Sprintf("%d:%02d:%02d.%03d", t/3600, (t/60)%60, t%60, ms)
	}
	t := int(math.Round(seconds))
	return fmt.Sprintf("%d:%02d:%02d", t/3600, (t/60)%60, t%60)
}

// MediaLink is one org-media-note link.
type MediaLink struct {
	Type   string  // video, audio, videocite, audiocite
	Target string  // a path, a url or (for the cite types) a citation key
	Start  float64 // seconds; -1 when the link names no time
	End    float64 // the end of an A-B loop; -1 when it is not one
	Label  string  // the description, "" when there is none
}

// IsCite is whether the target is a citation key.
func (l MediaLink) IsCite() bool { return strings.HasSuffix(l.Type, "cite") }

// Kind is video or audio.
func (l MediaLink) Kind() string { return strings.TrimSuffix(l.Type, "cite") }

// SplitMediaTarget splits `path#0:01:23-0:01:40` into its path and times. The
// split is at the last `#` followed by times, so a url with its own fragment
// keeps it. start/end are -1 when absent.
func SplitMediaTarget(target string) (string, float64, float64) {
	i := strings.LastIndex(target, "#")
	if i < 0 {
		return target, -1, -1
	}
	frag := target[i+1:]
	a, b := frag, ""
	if j := strings.Index(frag, "-"); j >= 0 {
		a, b = frag[:j], frag[j+1:]
	}
	start, ok := ParseMediaStamp(a)
	if !ok {
		return target, -1, -1
	}
	end := -1.0
	if b != "" {
		e, ok := ParseMediaStamp(b)
		if !ok {
			return target, -1, -1
		}
		end = e
	}
	return target[:i], start, end
}

var mediaLinkRe = regexp.MustCompile(`\[\[(videocite|audiocite|video|audio):([^\]]+)\](?:\[([^\]]*)\])?\]`)

// MediaLinkSpan is a link and where it is in its line (byte offsets).
type MediaLinkSpan struct {
	MediaLink
	From, To int
}

// FindMediaLinks is every org-media-note link in a line.
func FindMediaLinks(line string) []MediaLinkSpan {
	out := []MediaLinkSpan{}
	for _, m := range mediaLinkRe.FindAllStringSubmatchIndex(line, -1) {
		typ := line[m[2]:m[3]]
		target, start, end := SplitMediaTarget(line[m[4]:m[5]])
		label := ""
		if m[6] >= 0 {
			label = line[m[6]:m[7]]
		}
		out = append(out, MediaLinkSpan{MediaLink{Type: typ, Target: target, Start: start, End: end, Label: label}, m[0], m[1]})
	}
	return out
}

// MediaLinkText writes a link the way org-media-note-insert-link does:
// `[[video:file.mp4#0:12:30][0:12:30]]`, an A-B loop as
// `[[video:file.mp4#0:12:30-0:12:40][0:12:30-0:12:40]]`.
func MediaLinkText(typ, target string, start, end float64, fraction bool) string {
	if typ == "" {
		typ = "video"
	}
	if start < 0 {
		return "[[" + typ + ":" + target + "]]"
	}
	ts := MediaStamp(start, fraction)
	if end >= 0 && end > start {
		ts += "-" + MediaStamp(end, fraction)
	}
	return "[[" + typ + ":" + target + "#" + ts + "][" + ts + "]]"
}

var youtubeIdRe = regexp.MustCompile(`^[A-Za-z0-9_-]{6,20}$`)

// YoutubeId is the video id of a YouTube url (watch?v=, youtu.be/, embed/,
// shorts/, live/), or "".
func YoutubeId(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	host = strings.TrimPrefix(host, "m.")
	id := ""
	switch host {
	case "youtu.be":
		id = strings.Trim(u.Path, "/")
	case "youtube.com", "music.youtube.com", "youtube-nocookie.com":
		if u.Path == "/watch" {
			id = u.Query().Get("v")
		} else {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) == 2 && (parts[0] == "embed" || parts[0] == "shorts" || parts[0] == "live" || parts[0] == "v") {
				id = parts[1]
			}
		}
	}
	if youtubeIdRe.MatchString(id) {
		return id
	}
	return ""
}

var vimeoIdRe = regexp.MustCompile(`^(?:www\.|player\.)?vimeo\.com$`)

// VimeoId is the video id of a Vimeo url, or "".
func VimeoId(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !vimeoIdRe.MatchString(strings.ToLower(u.Host)) {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if _, err := strconv.Atoi(parts[i]); err == nil && parts[i] != "" {
			return parts[i]
		}
	}
	return ""
}

// IsOnlineMedia is whether a target is a url rather than a file.
func IsOnlineMedia(target string) bool {
	t := strings.ToLower(target)
	return strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://")
}
