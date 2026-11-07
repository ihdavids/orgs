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

# The character sheet's casting effects, lifted out for the map screens
# (public/dicefx.js), so worg never ships an older copy than the sheet.
python3 "$HERE/tools/dicefx/extract.py" "$WORG/public/dicefx.js"

echo "building worg in $WORG"
# No source maps: they hold worg's whole source, would be embedded in the
# binary (30 MB of them) and published with every release.
(cd "$WORG" && GENERATE_SOURCEMAP=false npm run build)

# A failed `npm run build` leaves the previous build's public files behind and
# no `static/`, which looked enough like a build to be copied over the embedded
# copy - and the binary then served whatever it had last, while this script
# said "done". Checked explicitly rather than trusted to exit codes.
if [ ! -f "$WORG/build/index.html" ] || [ ! -d "$WORG/build/static" ]; then
    echo "worg did not build: $WORG/build has no index.html or static/ - nothing copied" >&2
    exit 1
fi

echo "pulling the build into $HERE/worg"
find "$HERE/worg" -mindepth 1 -maxdepth 1 ! -name embed.go -exec rm -rf {} +
cp -R "$WORG/build/." "$HERE/worg/"

if [ ! -f "$HERE/worg/index.html" ] || [ ! -d "$HERE/worg/static" ]; then
    echo "the copy into $HERE/worg is incomplete" >&2
    exit 1
fi

# Stored gzipped: the server hands a .gz to any browser that takes gzip as it
# is and unzips it for one that does not (internal/app/orgs/worgserve.go), so
# only the compressed copy goes into the binary - about a third of the size.
# index.html and manifest.json stay as they are: embed.go names them, and they
# are tiny. A file gzip saves less than a tenth of (fonts, pictures) is left.
echo "compressing"
find "$HERE/worg" -type f -size +1k \( -name '*.js' -o -name '*.css' -o -name '*.json' \
    -o -name '*.wasm' -o -name '*.svg' -o -name '*.txt' -o -name '*.aff' -o -name '*.dic' \
    -o -name '*.ttf' -o -name '*.otf' -o -name '*.html' -o -name '*.map' \) \
    ! -path "$HERE/worg/index.html" ! -path "$HERE/worg/manifest.json" |
while read -r f; do
    gzip -9 -n -c "$f" > "$f.gz"
    if [ "$(wc -c < "$f.gz")" -lt "$(( $(wc -c < "$f") * 9 / 10 ))" ]; then
        rm "$f"
    else
        rm "$f.gz"
    fi
done

echo "done - rebuild orgs to pick it up:  go build -o orgs ./cmd/orgs"
