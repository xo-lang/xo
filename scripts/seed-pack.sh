#!/bin/bash
# seed-pack.sh XO OUTDIR: pack a built xo as a prebuilt seed (decision
# 0216 item 1; knowledgebase/infrastructure/toolchain.md "Prebuilt
# seeds"). No upload: the lead uploads OUTDIR with `aws s3 cp`.
#
# XO is a built xo in its layout: a checkout's bin/xo (std at
# bin/../std, the tracked files of std/ and third_party/ are packed) or
# a release's bin/xo (lib/xo/std and lib/xo/third_party beside it).
# Writes, for the platform <goos>-<goarch>:
#
#   OUTDIR/<goos>-<goarch>.tar.gz   bin/xo, lib/xo/std, lib/xo/third_party,
#                                   lib/xo/SEED (version, commit, date, platform)
#   OUTDIR/<goos>-<goarch>.sha256   "<hex>  <goos>-<goarch>.tar.gz"
#   OUTDIR/<goos>-<goarch>.info     version, commit, date (for the manifest)
#   OUTDIR/manifest.txt             rebuilt from every .info and .sha256 in OUTDIR
#
# The version and platform come from `XO version`; for an xo of another
# host (a cross built linux xo) set XO_SEED_VERSION and XO_SEED_PLATFORM
# (e.g. 0.10.0 and linux-arm64). The commit is $XO_SEED_COMMIT, else the
# HEAD of the checkout holding XO's std, else of the working directory.
# A checkout with changes under selfhost/, std/ or third_party/ is
# refused unless XO_SEED_ALLOW_DIRTY=1.
#
#   seed-pack.sh --manifest OUTDIR   only rebuild OUTDIR/manifest.txt
set -euo pipefail

die() { echo "seed-pack: $*" >&2; exit 1; }
sha256() { if command -v sha256sum >/dev/null; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi; }

manifest() {
  local out=$1 v="" c="" d="" f pv pc pd
  local tmp="$out/manifest.txt.tmp"
  : > "$tmp.plats"
  for f in "$out"/*.info; do
    [ -f "$f" ] || continue
    pv=$(sed -n 's/^version //p' "$f"); pc=$(sed -n 's/^commit //p' "$f"); pd=$(sed -n 's/^date //p' "$f")
    [ -z "$v" ] && { v=$pv; c=$pc; d=$pd; }
    [ "$pv" = "$v" ] || die "$f: version $pv, but another platform in $out is $v"
    [ "$pc" = "$c" ] || die "$f: commit $pc, but another platform in $out is $c"
    local plat; plat=$(basename "$f" .info)
    [ -f "$out/$plat.sha256" ] || die "$out/$plat.sha256 missing"
    echo "sha256 $plat $(cut -d' ' -f1 "$out/$plat.sha256")" >> "$tmp.plats"
  done
  [ -n "$v" ] || die "no .info files in $out"
  { echo "version $v"; echo "commit $c"; echo "date $d"; sort "$tmp.plats"; } > "$tmp"
  rm -f "$tmp.plats"
  mv "$tmp" "$out/manifest.txt"
}

if [ "${1:-}" = "--manifest" ]; then
  [ $# -eq 2 ] || die "usage: seed-pack.sh --manifest OUTDIR"
  manifest "$2"; cat "$2/manifest.txt"; exit 0
fi
[ $# -eq 2 ] || die "usage: seed-pack.sh XO OUTDIR (or --manifest OUTDIR)"
xo=$1; out=$2
[ -f "$xo" ] || die "$xo: no such file"
xo=$(cd "$(dirname "$xo")" && pwd)/$(basename "$xo")
bindir=$(dirname "$xo"); prefix=$(dirname "$bindir")

# the layout: a release's lib/xo, else a checkout
if [ -d "$prefix/lib/xo/std" ]; then
  layout=release; stdroot=$prefix/lib/xo
elif [ -d "$prefix/std" ]; then
  layout=checkout; stdroot=$prefix
else
  die "$xo: no lib/xo/std or std beside its bin directory (not a release or checkout layout)"
fi

# version and platform
if line=$("$xo" version 2>/dev/null); then
  # "xo 0.9.0 darwin/arm64"
  set -- $line
  hv=${2:-}; hp=$(echo "${3:-}" | tr / -)
  [ -z "${XO_SEED_VERSION:-}" ] || [ "$XO_SEED_VERSION" = "$hv" ] || die "XO_SEED_VERSION=$XO_SEED_VERSION but $xo is $hv"
  [ -z "${XO_SEED_PLATFORM:-}" ] || [ "$XO_SEED_PLATFORM" = "$hp" ] || die "XO_SEED_PLATFORM=$XO_SEED_PLATFORM but $xo is $hp"
  version=$hv; plat=$hp
else
  [ -n "${XO_SEED_VERSION:-}" ] && [ -n "${XO_SEED_PLATFORM:-}" ] || die "$xo does not run here: set XO_SEED_VERSION and XO_SEED_PLATFORM"
  version=$XO_SEED_VERSION; plat=$XO_SEED_PLATFORM
fi
echo "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([.+-][0-9A-Za-z.-]+)?$' || die "version $version"
echo "$plat" | grep -Eq '^[a-z0-9]+-[a-z0-9]+$' || die "platform $plat"

# commit
gitroot=""
if [ "$layout" = checkout ] && git -C "$prefix" rev-parse --git-dir >/dev/null 2>&1; then gitroot=$prefix; fi
if [ -n "${XO_SEED_COMMIT:-}" ]; then
  commit=$XO_SEED_COMMIT
elif [ -n "$gitroot" ]; then
  commit=$(git -C "$gitroot" rev-parse HEAD)
elif git rev-parse --git-dir >/dev/null 2>&1; then
  commit=$(git rev-parse HEAD)
else
  commit=unknown
fi
if [ -n "$gitroot" ] && [ -z "${XO_SEED_ALLOW_DIRTY:-}" ] && [ -n "$(git -C "$gitroot" status --porcelain -- selfhost std third_party)" ]; then
  die "$gitroot has changes under selfhost/, std/ or third_party/ (commit them, or XO_SEED_ALLOW_DIRTY=1)"
fi
date=$(date -u +%Y-%m-%d)

mkdir -p "$out"
out=$(cd "$out" && pwd)
stage=$(mktemp -d "${TMPDIR:-/tmp}/seed-pack.XXXXXX")
trap 'rm -rf "$stage"' EXIT
mkdir -p "$stage/bin" "$stage/lib/xo"
cp "$xo" "$stage/bin/$(basename "$xo")"
chmod 755 "$stage/bin/$(basename "$xo")"
if [ -n "$gitroot" ]; then
  # the tracked files only (a checkout's std holds no build output, but be sure)
  (cd "$gitroot" && git ls-files -z -- std third_party | tar -c -f - --null -T -) | tar -xf - -C "$stage/lib/xo"
else
  (cd "$stage/lib/xo" && cp -R "$stdroot/std" . && { [ ! -d "$stdroot/third_party" ] || cp -R "$stdroot/third_party" .; })
fi
[ -f "$stage/lib/xo/std/prelude.xo" ] || die "packed std has no prelude.xo"
printf 'version %s\ncommit %s\ndate %s\nplatform %s\n' "$version" "$commit" "$date" "$plat" > "$stage/lib/xo/SEED"

tgz="$out/$plat.tar.gz"
taropts=()
if tar --version 2>/dev/null | grep -q bsdtar; then taropts=(--no-xattrs --no-mac-metadata); fi
COPYFILE_DISABLE=1 tar ${taropts[@]+"${taropts[@]}"} -C "$stage" -czf "$tgz.tmp" bin lib
mv "$tgz.tmp" "$tgz"
echo "$(sha256 "$tgz")  $plat.tar.gz" > "$out/$plat.sha256"
printf 'version %s\ncommit %s\ndate %s\n' "$version" "$commit" "$date" > "$out/$plat.info"
manifest "$out"
echo "seed-pack: $tgz ($(wc -c < "$tgz" | tr -d ' ') bytes, $layout layout, xo $version, commit $commit)"
cat "$out/$plat.sha256"
