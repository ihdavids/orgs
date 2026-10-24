#!/bin/sh
# Builds docs.org: the SDOC blocks in the Go sources plus the guides in docs/.
# docs/overview.org is the intro; every other docs/*.org names its section
# with #+DOC_SECTION.
#
# Then exports it to docs/docs.html, the page that gets published, with its
# pictures linked relative to docs/ (images/x.png) rather than through a
# server's /images. The export runs a server in this process (-orgdir) with a
# config of its own, so it needs no running server and starts none of the
# plugins a real orgs.yaml would.
set -e
cd "$(dirname "$0")/.."
go build -o docex ./cmd/docex
./docex -src . -out docs.org -start docs/overview.org
# The part `orgs help` and the man pages carry in the binary.
./docex -src . -bundle cmd/oc/commands/help/docs.json

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
go build -o "$tmp/orgs" ./cmd/orgs
cat > "$tmp/orgs.yaml" <<EOF
server:
  noUpdateCheck: true
  exporters:
    - name: "html"
EOF
"$tmp/orgs" -config "$tmp/orgs.yaml" -orgdir . export -f html docs.org -out docs/docs.html -rel 2>"$tmp/export.log" || {
	cat "$tmp/export.log" >&2
	exit 1
}
echo "wrote docs.org and docs/docs.html"
