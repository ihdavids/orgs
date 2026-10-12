# Changelog

Each release is cut by `./makerelease.sh`, which tags the version below and
pushes the tag; `.github/workflows/release.yml` then builds and publishes it.
Add the next version's section here before bumping `VERSION` in that script.

## v0.1.0 - unreleased

The first tagged release. Binaries for linux, macOS and windows (amd64 and
arm64), each a single `orgs` executable that is both the server and the
command line client.

### Server

- Watches org directories, parses them with go-org and serves a REST API
  (HTTP and/or HTTPS) with JWT logins, a per-user keystore and token expiry.
- Embeds the worg web app, the html/latex templates, the documentation
  themes and fonts, so a server needs no files beside the binary.
- Login tickets: `POST /login/ticket` (logged in) hands out a one-use,
  one-minute link, `GET /login/ticket/<t>`, that logs a browser in to worg
  without a password. `orgs worg [View]` opens one.
- The command line makes its own token for a server on this machine when its
  config holds that server's keys (`orgJWS`, `orgJWE`, `orgSalt`) and it has
  no good token of its own - so the binary VS Code installs, which reads the
  server's `orgs.yaml` beside it, needs no login.
- `httpsOnly: true` serves https and nothing else: no http listener on
  `port`. Needs `allowHttps` and a certificate.
- `orgs cert` makes a self-signed certificate for this machine (localhost,
  127.0.0.1, ::1, plus `-host` names) at `servercrt`/`serverkey`, good for
  820 days; it leaves an existing pair alone unless `-force`.
- `GET /version` reports the running version and whether a newer release is
  on github (cached 6h; `server.noUpdateCheck` turns it off).
- Exporters: html, markdown, latex/pdf, pandoc, mermaid, gantt, Confluence,
  reveal.js, impress.js, deck.js, webslides, tangle.
- Background plugins: Jira, Todoist, Trello, Google Calendar, ics calendars,
  GitHub/GitLab, git sync of the org directory, notifications, auto clock out.
- D&D character sheets, rulesets and session logs.

### Command line

- Tasks and planning: `agenda`, `capture`, `refile`, `search`, `find`,
  `grep`, `projects`, `plan`, `review`, `triage`, `month`, `daypage`.
- Time: `clockin`, `clockout`, `clocks`, `clocktable`, `focus`.
- Views in the terminal: `tui`, `kanban`, `gantt`, `cols`, `habits`,
  `insights`, `pres`, `drill`.
- Notes and links: `links`, `mentions`, `golink`, `rec`, `contact`,
  `tables`, `code`, `snip`, `export`.
- `orgs mcp` serves the org files to AI assistants over MCP.
- `-local` (or `-orgdir`) runs a server in-process for one command, for git
  hooks, CI and machines with no daemon.
- Every command takes `-json`, `-format`, `-dry-run` and `-no-color`.
- `orgs version` prints the client's and the server's version.
