#!/bin/sh
# Build worg and pull the result into this repo, where it is embedded into the
# orgs binary by worg/embed.go and served by the server itself.
#
# The build's filenames carry content hashes, so the old ones have to go rather
# than be written over - otherwise every build leaves its predecessor behind in
# the binary, and the embed grows a little every time.
set -e

WORG=${WORG:-../worg}
HERE=$(cd "$(dirname "$0")/.." && pwd)

if [ ! -d "$WORG" ]; then
    echo "worg is not at $WORG - set WORG to where it is" >&2
    exit 1
fi

echo "building worg in $WORG"
(cd "$WORG" && npm run build)

echo "pulling the build into $HERE/worg"
find "$HERE/worg" -mindepth 1 -maxdepth 1 ! -name embed.go -exec rm -rf {} +
cp -R "$WORG/build/." "$HERE/worg/"

echo "done - rebuild orgs to pick it up:  go build -o orgs ./cmd/orgs"
