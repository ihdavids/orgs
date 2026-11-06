package htmlexp

// org-media-note links (`[[video:x.mp4#0:12:30][0:12:30]]`) drawn as a player
// once per recording, with every timestamp link seeking it - only while the
// server has features.mediaNotes on. Elsewhere, and while it is off, they stay
// links. The player goes where the first link to that recording is, as a
// span (a <video> is phrasing content, so a paragraph or list item around it
// stays whole). A small script, written once with the first player, does the
// seeking; worg's file view runs it too, but catches the clicks first to open
// its Media Notes panel at that moment.

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// Set by the server (medianote.go): whether the feature is on, and what a
// link plays - its kind, and an absolute local path or (online) a url.
var MediaNotesOn func() bool
var MediaNoteResolve func(docPath, typ, target string) (kind, where string, online bool)

func isMediaLinkType(p string) bool {
	switch p {
	case "video", "audio", "videocite", "audiocite":
		return true
	}
	return false
}

func mediaSecs(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

const mediaNoteScript = `<script>(function(){if(window.__orgsMediaNotes)return;window.__orgsMediaNotes=1;
function post(f,m,a){try{f.contentWindow.postMessage(JSON.stringify(m),'*')}catch(e){}}
document.addEventListener('click',function(ev){var a=ev.target&&ev.target.closest&&ev.target.closest('a.org-media-link');if(!a||ev.defaultPrevented)return;
var p=document.querySelector('[data-media-player="'+a.getAttribute('data-media')+'"]');if(!p)return;ev.preventDefault();
var s=parseFloat(a.getAttribute('data-start')||'0'),e=parseFloat(a.getAttribute('data-end')||'-1');
var v=p.querySelector('video,audio');if(v){v.__orgsLoop=e>s?[s,e]:null;if(!v.__orgsWired){v.__orgsWired=1;v.addEventListener('timeupdate',function(){var l=v.__orgsLoop;if(l&&v.currentTime>=l[1])v.currentTime=l[0];});}
v.currentTime=s;var pr=v.play();if(pr&&pr.catch)pr.catch(function(){});p.scrollIntoView({block:'nearest'});return;}
var f=p.querySelector('iframe');if(!f)return;if(f.getAttribute('data-kind')==='vimeo'){post(f,{method:'setCurrentTime',value:s});post(f,{method:'play'});}
else{post(f,{event:'command',func:'seekTo',args:[s,true]});post(f,{event:'command',func:'playVideo',args:[]});}p.scrollIntoView({block:'nearest'});});})();</script>`

// writeMediaNoteLink writes an org-media-note link, answering false when it
// is not one or the feature is off (the caller writes it as any link).
func (w *OrgHtmlWriter) writeMediaNoteLink(l org.RegularLink) bool {
	if MediaNotesOn == nil || MediaNoteResolve == nil || !MediaNotesOn() || !isMediaLinkType(l.Protocol) {
		return false
	}
	raw := strings.TrimPrefix(l.URL, l.Protocol+":")
	target, start, end := common.SplitMediaTarget(raw)
	docPath := ""
	if w.SrcDoc != nil {
		docPath = w.SrcDoc.Path
	}
	kind, where, online := MediaNoteResolve(docPath, l.Protocol, target)
	if where == "" {
		return false
	}
	sum := sha1.Sum([]byte(where))
	id := "m" + hex.EncodeToString(sum[:6])
	yt, vimeo := "", ""
	src := where
	if online {
		yt, vimeo = common.YoutubeId(where), common.VimeoId(where)
	} else {
		src = w.MediaSrc(w.SrcDoc, where)
		if src == "" || strings.HasPrefix(src, "file:") {
			src = "/media/file?file=" + url.QueryEscape(docPath) + "&path=" + url.QueryEscape(where)
		}
	}
	desc := ""
	if l.Description != nil {
		desc = w.WriteNodesAsString(l.Description...)
	} else if start >= 0 {
		desc = html.EscapeString(common.MediaStamp(start, false))
	} else {
		desc = html.EscapeString(target)
	}

	if w.mediaPlayers == nil {
		w.mediaPlayers = map[string]bool{}
	}
	if !w.mediaPlayers[id] {
		player := ""
		switch {
		case yt != "":
			player = fmt.Sprintf(`<iframe data-kind="youtube" src="https://www.youtube-nocookie.com/embed/%s?enablejsapi=1" width="560" height="315" style="max-width:100%%;border:0" allow="autoplay; encrypted-media; picture-in-picture" allowfullscreen></iframe>`, url.PathEscape(yt))
		case vimeo != "":
			player = fmt.Sprintf(`<iframe data-kind="vimeo" src="https://player.vimeo.com/video/%s" width="560" height="315" style="max-width:100%%;border:0" allow="autoplay; fullscreen; picture-in-picture" allowfullscreen></iframe>`, url.PathEscape(vimeo))
		case kind == "audio":
			player = fmt.Sprintf(`<audio controls preload="metadata" src="%s" style="width:100%%"></audio>`, html.EscapeString(src))
		default:
			player = fmt.Sprintf(`<video controls preload="metadata" src="%s" style="max-width:100%%;max-height:60vh"></video>`, html.EscapeString(src))
		}
		w.WriteString(fmt.Sprintf(`<span class="org-media-player" data-media-player="%s" style="display:block;margin:.4em 0">%s</span>`, id, player))
		if len(w.mediaPlayers) == 0 {
			w.WriteString(mediaNoteScript)
		}
		w.mediaPlayers[id] = true
	}

	href := src
	switch {
	case yt != "" && start >= 0:
		href = fmt.Sprintf("https://www.youtube.com/watch?v=%s&t=%ds", url.QueryEscape(yt), int(start))
	case vimeo != "" && start >= 0:
		href = fmt.Sprintf("https://vimeo.com/%s#t=%ds", vimeo, int(start))
	case !online && start >= 0:
		href = src + "#t=" + mediaSecs(start)
		if end >= 0 {
			href += "," + mediaSecs(end)
		}
	}
	attrs := fmt.Sprintf(` data-media="%s" data-media-type="%s" data-media-target="%s"`, id, html.EscapeString(l.Protocol), html.EscapeString(target))
	if start >= 0 {
		attrs += fmt.Sprintf(` data-start="%s"`, mediaSecs(start))
	}
	if end >= 0 {
		attrs += fmt.Sprintf(` data-end="%s"`, mediaSecs(end))
	}
	w.WriteString(fmt.Sprintf(`<a class="org-media-link" href="%s"%s>%s</a>`, html.EscapeString(href), attrs, desc))
	return true
}
