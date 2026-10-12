#!/bin/sh
# Cut a release: tag VERSION on the current commit and push the tag to origin.
# The push starts .github/workflows/release.yml, which builds every platform
# with goreleaser and publishes a github release.
#
# For the next release: add its section to CHANGELOG.md, bump VERSION, commit.
set -e

VERSION=v0.1.0

cd "$(dirname "$0")"

fail() { echo "makerelease: $*" >&2; exit 1; }

[ "$(git rev-parse --abbrev-ref HEAD)" = main ] || fail "not on main"
[ -z "$(git status --porcelain --untracked-files=no)" ] || fail "uncommitted changes - commit or stash them first"
git rev-parse -q --verify "refs/tags/$VERSION" >/dev/null && fail "$VERSION is already tagged here - bump VERSION in $0"
git ls-remote --exit-code --tags origin "refs/tags/$VERSION" >/dev/null 2>&1 && fail "$VERSION is already on origin - bump VERSION in $0"
grep -q "^## $VERSION" CHANGELOG.md || fail "CHANGELOG.md has no '## $VERSION' section"

# The release builds the tagged commit as github has it, so it must be pushed.
git fetch -q origin main
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || fail "main and origin/main differ - push (or pull) first"

echo "checking the build"
go build ./... || fail "go build failed"

git tag -a "$VERSION" -m "orgs $VERSION"
git push origin "$VERSION"

echo "pushed $VERSION - follow the build with: gh run watch"
