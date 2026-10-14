#!/bin/bash
# seed-pack.sh XO OUTDIR: pack a built xo as a prebuilt seed (decision
# 0216 item 1; knowledgebase/infrastructure/toolchain.md "Prebuilt
# seeds"). No upload: the lead uploads OUTDIR with `aws s3 cp`.
#
# XO is a built xo in its layout: a checkout's bin/xo (std at
# bin/../std; the tracked files of std/ are packed) or a release's
# bin/xo (lib/xo/std beside it). For the platform <goos>-<goarch> it
# writes, in the shape of the bucket's seeds/<seed>/ directory:
#
#   OUTDIR/<goos>-<goarch>.tar.gz          xo/bin/xo and xo/std (xo finds std
#                                          at bin/../std), xo/SEED
#   OUTDIR/<goos>-<goarch>.tar.gz.sha256   "<hex>  <goos>-<goarch>.tar.gz"
#   OUTDIR/<goos>-<goarch>.info            version, commit, date, note (for the manifest)
#   OUTDIR/manifest.txt                    rebuilt from every .info in OUTDIR
#
# and prints the seed name, <version>-<commit, 8 hex> (the directory
# under seeds/ and the line of seeds/latest.txt), e.g. 0.9.0-9b6cd014.
#
# The version and platform come from `XO version`; for an xo of another
# host (a cross built linux xo) set XO_SEED_VERSION and XO_SEED_PLATFORM
# (e.g. 0.10.0 and linux-arm64). The commit (full hash) is
# $XO_SEED_COMMIT, else the HEAD of the checkout holding XO's std, else
# of the working directory. XO_SEED_NOTE is the manifest note (default:
# the layout packed). A checkout with changes under selfhost/ or std/ is
# refused unless XO_SEED_ALLOW_DIRTY=1.
#
#   seed-pack.sh --manifest OUTDIR   only rebuild OUTDIR/manifest.txt
#
# Seeds are built at clang -O2: an xo whose bin/xo.opt names another
# level (`go run ./scripts/xobuild` builds xo at -O0 on an LLVM host,
# decision 0220) is refused unless XO_SEED_ALLOW_OPT=1; rebuild it with
# XO_LLVM_OPT=O2 go run ./scripts/xobuild.
set -euo pipefail

die() { echo "seed-pack: $*" >&2; exit 1; }
sha256() { if command -v sha256sum >/dev/null; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi; }
field() { sed -n "s/^$1 //p" "$2"; }

manifest() {
  local out=$1 v="" c="" d="" f plat
  local tmp="$out/manifest.txt.tmp"
  : > "$tmp.plats"
  for f in "$out"/*.info; do
    [ -f "$f" ] || continue
    plat=$(basename "$f" .info)
    if [ -z "$v" ]; then v=$(field version "$f"); c=$(field commit "$f"); d=$(field date "$f"); fi
    [ "$(field version "$f")" = "$v" ] || die "$f: version $(field version "$f"), another platform in $out is $v"
    [ "$(field commit "$f")" = "$c" ] || die "$f: commit $(field commit "$f"), another platform in $out is $c"
    [ -f "$out/$plat.tar.gz.sha256" ] || die "$out/$plat.tar.gz.sha256 missing"
    echo "$plat $(cut -d' ' -f1 "$out/$plat.tar.gz.sha256") ($(field note "$f"))" >> "$tmp.plats"
  done
  [ -n "$v" ] || die "no .info files in $out"
  {
    echo "version $v"
    echo "commit ${c:0:10}"
    echo "commit_full $c"
    echo "date $d"
    echo "layout xo/bin/xo plus xo/std (xo finds std at bin/../std)"
    sort "$tmp.plats"
  } > "$tmp"
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
if [ -f "$bindir/xo.opt" ] && [ "$(cat "$bindir/xo.opt")" != O2 ] && [ "${XO_SEED_ALLOW_OPT:-0}" != 1 ]; then
  die "$xo was built at clang -$(cat "$bindir/xo.opt") (bin/xo.opt); seeds are -O2: XO_LLVM_OPT=O2 go run ./scripts/xobuild"
fi

# the layout: a release's lib/xo, else a checkout
if [ -d "$prefix/lib/xo/std" ]; then
  layout=release; std=$prefix/lib/xo/std
elif [ -d "$prefix/std" ]; then
  layout=checkout; std=$prefix/std
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
echo "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || die "version $version"
echo "$plat" | grep -Eq '^[a-z0-9]+-[a-z0-9]+$' || die "platform $plat"

# commit
gitroot=""
if [ "$layout" = checkout ] && git -C "$prefix" rev-parse --git-dir >/dev/null 2>&1; then gitroot=$prefix; fi
if [ -n "${XO_SEED_COMMIT:-}" ]; then
  commit=$XO_SEED_COMMIT
elif [ -n "$gitroot" ]; then
  commit=$(git -C "$gitroot" rev-parse HEAD)
else
  commit=$(git rev-parse HEAD 2>/dev/null) || die "no commit: set XO_SEED_COMMIT"
fi
echo "$commit" | grep -Eq '^[0-9a-f]{40}$' || die "commit $commit: want a full hash"
if [ -n "$gitroot" ] && [ -z "${XO_SEED_ALLOW_DIRTY:-}" ] && [ -n "$(git -C "$gitroot" status --porcelain -- selfhost std)" ]; then
  die "$gitroot has changes under selfhost/ or std/ (commit them, or XO_SEED_ALLOW_DIRTY=1)"
fi
date=$(date -u +%Y-%m-%d)
seed=$version-${commit:0:8}

mkdir -p "$out"
out=$(cd "$out" && pwd)
stage=$(mktemp -d "${TMPDIR:-/tmp}/seed-pack.XXXXXX")
trap 'rm -rf "$stage"' EXIT
mkdir -p "$stage/xo/bin"
cp "$xo" "$stage/xo/bin/$(basename "$xo")"
chmod 755 "$stage/xo/bin/$(basename "$xo")"
if [ -n "$gitroot" ]; then
  # the tracked files only
  (cd "$gitroot" && git ls-files -z -- std | tar -c -f - --null -T -) | tar -xf - -C "$stage/xo"
else
  cp -R "$std" "$stage/xo/std"
fi
[ -f "$stage/xo/std/prelude.xo" ] || die "packed std has no prelude.xo"
printf 'seed %s\nversion %s\ncommit %s\ndate %s\nplatform %s\n' "$seed" "$version" "$commit" "$date" "$plat" > "$stage/xo/SEED"

tgz="$out/$plat.tar.gz"
taropts=()
if tar --version 2>/dev/null | grep -q bsdtar; then taropts=(--no-xattrs --no-mac-metadata); fi
# gzip's level: XO_SEED_GZIP_LEVEL (1 to 9, default 6, gzip's and tar
# -z's default); internal/xotool's TestSeedPackHello packs at 1, which
# took about 1 s instead of 4 for the 87 MB xo
level=${XO_SEED_GZIP_LEVEL:-6}
echo "$level" | grep -Eq '^[1-9]$' || die "XO_SEED_GZIP_LEVEL=$level: want 1 to 9"
COPYFILE_DISABLE=1 tar ${taropts[@]+"${taropts[@]}"} -C "$stage" -cf - xo | gzip -n "-$level" > "$tgz.tmp"
mv "$tgz.tmp" "$tgz"
echo "$(sha256 "$tgz")  $plat.tar.gz" > "$tgz.sha256"
printf 'seed %s\nversion %s\ncommit %s\ndate %s\nnote %s\n' "$seed" "$version" "$commit" "$date" \
  "${XO_SEED_NOTE:-seed-pack.sh from a $layout layout}" > "$out/$plat.info"
manifest "$out"
echo "seed-pack: $tgz ($(wc -c < "$tgz" | tr -d ' ') bytes, $layout layout, xo $version, commit $commit)" >&2
cat "$tgz.sha256" >&2
echo "$seed"
