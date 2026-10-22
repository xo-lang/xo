#!/bin/bash
# release-publish.sh: the owner's last step of a binary release: a GitHub
# Release on xo-lang/xo for a tag publish.sh already pushed, with the
# archives and SHA256SUMS of scripts/release.sh attached.
#
#   scripts/release-publish.sh TAG DIST NOTES.md
#
# Run it in a terminal (it refuses without one, as publish.sh push does),
# from the published tree the archives were built in (../xo-publish).
# Before asking, it checks:
#
#   - TAG is on xo-lang/xo (git ls-remote) and names this tree's HEAD;
#   - DIST/SHA256SUMS lists every DIST/*.tar.gz and their sums match;
#   - every archive's version is TAG (xo-<TAG>-<os>-<arch>.tar.gz);
#   - the host's archive reports TAG's commit in `xo version --json`;
#   - no release exists for TAG yet.
#
# It then shows the files and the notes and asks for the word `release`.
# gh uses the owner's own login (gh auth status); agents never run this.
# XO_RELEASE_REPO overrides the repository (a rehearsal on a scratch
# repository); --draft makes a draft release.
set -euo pipefail
die() { echo "release-publish.sh: $*" >&2; exit 1; }

draft=()
if [ "${1:-}" = --draft ]; then
	draft=(--draft)
	shift
fi
[ $# -eq 3 ] || die "usage: scripts/release-publish.sh [--draft] TAG DIST NOTES.md"
tag=$1
dist=$2
notes=$3
repo=${XO_RELEASE_REPO:-xo-lang/xo}
[ -t 0 ] && [ -t 1 ] || die "runs only from the owner's terminal"
[ -d scripts/lead ] && die "this is the private checkout; run it in the published tree"
[ -f "$notes" ] || die "no notes file $notes"
command -v gh >/dev/null || die "gh is not installed"

# the tag on the public repository is this tree's HEAD
remote=$(git ls-remote "https://github.com/$repo.git" "refs/tags/$tag^{}" "refs/tags/$tag" | awk '{print $1}' | head -1)
[ -n "$remote" ] || die "$tag is not on $repo: the owner pushes it first (publish.sh push)"
head=$(git rev-parse HEAD)
peeled=$(git ls-remote "https://github.com/$repo.git" "refs/tags/$tag^{}" | awk '{print $1}')
[ "${peeled:-$remote}" = "$head" ] || die "$tag on $repo is ${peeled:-$remote}, this tree is at $head"

# the archives and their sums
v=${tag#v}
shopt -s nullglob
archives=("$dist"/xo-*.tar.gz)
[ ${#archives[@]} -gt 0 ] || die "no archives in $dist"
[ -f "$dist/SHA256SUMS" ] || die "no $dist/SHA256SUMS"
for a in "${archives[@]}"; do
	b=$(basename "$a")
	case $b in "xo-$v-"*) ;; "xo-$tag-"*) ;; *) die "$b is not version $v" ;; esac
	grep -q " $b\$" "$dist/SHA256SUMS" || die "$b is not in SHA256SUMS"
done
[ "$(wc -l <"$dist/SHA256SUMS" | tr -d ' ')" = ${#archives[@]} ] || die "SHA256SUMS lists other files than $dist/*.tar.gz"
(cd "$dist" && shasum -a 256 -c SHA256SUMS) || die "a checksum does not match"

# the host's archive names this commit
plat="$(uname -s | tr A-Z a-z)-$(uname -m | sed 's/aarch64/arm64/; s/x86_64/amd64/')"
host=$(ls "$dist"/xo-*-"$plat".tar.gz 2>/dev/null | head -1)
if [ -n "$host" ]; then
	t=$(mktemp -d)
	trap 'rm -rf "${t:?}"' EXIT
	tar -xzf "$host" -C "$t"
	commit=$("$t"/*/bin/xo version --json | sed -n 's/.*"commit":"\([0-9a-f]*\)".*/\1/p')
	[ "$commit" = "$head" ] || die "$(basename "$host") says commit '$commit', the tag is $head"
	echo "$(basename "$host"): commit $commit"
fi

gh release view "$tag" --repo "$repo" >/dev/null 2>&1 && die "$repo already has a release $tag"

echo
echo "Release $tag on $repo${draft:+ (draft)}, commit $head, files:"
for a in "${archives[@]}" "$dist/SHA256SUMS"; do
	printf '  %s  %s\n' "$(du -h "$a" | cut -f1)" "$(basename "$a")"
done
echo "Notes ($notes):"
sed 's/^/  | /' "$notes"
echo
read -r -p "Type 'release' to publish: " ok
[ "$ok" = release ] || die "not confirmed"
gh release create "$tag" --repo "$repo" --verify-tag --title "Xo $tag" --notes-file "$notes" \
	${draft[@]+"${draft[@]}"} "${archives[@]}" "$dist/SHA256SUMS"
echo "release-publish.sh: published; https://github.com/$repo/releases/tag/$tag"
