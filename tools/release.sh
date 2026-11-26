#!/bin/sh
# Makes a release: sets the version, builds, tests, and commits the installer and then the
# signed manifest. Nothing is pushed: `git push origin main` afterwards publishes it.
#
# The manifest names the installer by the commit that holds it, not by the branch. An
# address by branch is kept for some minutes by GitHub's servers, the manifest and the
# installer each on their own, so a program could be told of one release and handed the
# installer of another ("installer is larger than announced"). An address by commit always
# gives the same file.
#
# Usage: sh tools/release.sh VERSION "what is new"
set -e
cd "$(dirname "$0")/.."
V="$1"
NOTES="$2"
if [ -z "$V" ] || [ -z "$NOTES" ]; then
	echo 'usage: sh tools/release.sh VERSION "what is new"' >&2
	exit 2
fi
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
	echo "there are changes not committed: commit them first" >&2
	exit 1
fi

sed -i "s/appVersion = \"[0-9.]*\"/appVersion = \"$V\"/" cmd/mlite/install.go
grep -q "appVersion = \"$V\"" cmd/mlite/install.go
sh tools/build.sh
sh tools/go.sh test -count=1 ./cmd/mlite/ ./internal/...

mkdir -p release
cp build/Linnet-Setup.exe release/Linnet-Setup.exe
git add cmd/mlite/install.go release/Linnet-Setup.exe
git commit -q -m "Linnet $V: the installer

Co-Authored-By: Claude Fable 5.2 <noreply@anthropic.com>"
SHA="$(git rev-parse HEAD)"

sh tools/go.sh run ./cmd/release sign -version "$V" -notes "$NOTES" \
	-url "https://raw.githubusercontent.com/wielblad/linnet/$SHA/release/Linnet-Setup.exe"
if [ -n "$(git status --porcelain release/Linnet-Setup.exe)" ]; then
	echo "the signed installer is not the committed one" >&2
	exit 1
fi
git add latest.json
git commit -q -m "Linnet $V

Co-Authored-By: Claude Fable 5.2 <noreply@anthropic.com>"
echo "wydanie $V gotowe; instalator w commicie $SHA. Opublikuj: git push origin main"
