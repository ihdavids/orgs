# Notes on the html exporter

Loaded when working in this directory. References like **Traps: line edits** point at the Traps list in the root `CLAUDE.md`.

## Audio in an exported page

A heading with a property pointing at an audio file gets a player in the html export (`WriteAudio` in `plugs/html/html.go`). `:AUDIO:` is what orgs writes from a voice note, but any property works (`:INTERVIEW: [[file:takes/mira.wav]]`) with no exporter knowledge of the name.

1. **`preload="none"`**: nothing is fetched until played, so forty voice notes cost one page, not forty recordings.
2. **Paths** (see **Traps: media paths**): `MediaSrc` joins the link (relative to its org file, so moving the org directory keeps the audio) with the first org directory, and falls back to resolving against the root because many files are written that way. Outside the org directory there is nothing to serve, so it returns a `file://` link rather than a url that would 404.
3. The url is a path (`/images/...`), not `http://localhost:port/...`. The `filelinks;`/`httpslinks;`/`httplinks;` opts still produce absolute forms for an exported file on disk and for a vscode webview. `WriteRegularLink` keeps its own older rule for images, deliberately not shared, because changing it would change every page already exported.
