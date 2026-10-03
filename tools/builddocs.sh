#!/bin/sh
# Builds docs.org: the SDOC blocks in the Go sources plus the guides in docs/.
# docs/overview.org is the intro; every other docs/*.org names its section
# with #+DOC_SECTION.
set -e
cd "$(dirname "$0")/.."
go build -o docex ./cmd/docex
./docex -src . -out docs.org -start docs/overview.org
