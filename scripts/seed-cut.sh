#!/bin/bash
# seed-cut.sh VERSION NOTES WORKDIR: cut seed VERSION (decision 0216;
# knowledgebase/infrastructure/toolchain.md "Cutting a seed"). Run on the
# development Mac (darwin/arm64, with Go, clang, git, and push access),
# only with the owner's approval of this version. It uploads nothing:
# scripts/seed-upload.sh WORKDIR does that after it succeeds.
#
#   VERSION  the new minor version, X.Y.0, greater than the current one
#   NOTES    a file with the v<VERSION> tag's message: "Xo vX.Y.0", then
#            what the version brings over the previous seed (owner,
#            2026-10-14; toolchain.md says what goes in it)
#   WORKDIR  an empty or missing directory outside the repository
#
# Steps, each stopping the script on failure:
#   1. a worktree of origin/main at WORKDIR/tree; VERSION in
#      compiler/cli/core/cli.xo, compiler/cli/servers/servers.xo and
#      std/VERSION, one commit
#   2. go test -short of internal/xotool and cmd/xo
#   3. the gate: xo built from the bump (go run ./scripts/xobuild) builds
#      and runs hello, and the self rebuild fixed point holds
#      (scripts/bootstrap-xo.sh, s2 and s3 identical)
#   4. push the commit to main (refused if main moved: rerun), then the
#      annotated tag vVERSION with NOTES, pushed (a tag, not a release)
#   5. darwin-arm64: xo at clang -O2 and ThreadSanitizer's runtime
#      (scripts/race-runtime.sh, into the tree's lib/xo/race), packed by
#      scripts/seed-pack.sh
#   6. linux-arm64, linux-amd64, windows-amd64, darwin-amd64, windows-arm64
#      (from 0.46: decision 0569, xo builds itself there): cross built
#      by that xo with the Go backend, packed with the checkout's std;
#      linux-arm64, linux-amd64 and darwin-amd64 also with
#      ThreadSanitizer's runtime (scripts/race-runtime.sh ... PLATFORM:
#      the Linux archives built in Docker, which must run)
#   7. WORKDIR/out holds the six tarballs, their .sha256, manifest.txt,
#      and SEED (the seed's name, VERSION-<commit, 8 hex>)
#
# XO_SEED_TAG=vX.Y.Z (a patch seed, X.Y.Z with Z above 0, VERSION the same
# X.Y.Z): cuts the seed of a tag that already exists (a release branch's
# vX.Y.Z). Step 1 makes the tree a worktree of that tag and checks that
# the three version files say X.Y.Z; there is no bump commit, no step 4
# push or tag, and no push lock. NOTES is unused. Steps 2, 3, 5 and 6 run
# as for X.Y.0. This script itself must still match origin/main's.
#
# XO_SEED_DRY_RUN=1 does every step but the pushes (no commit reaches
# main, no tag is made): a rehearsal; seed-upload.sh refuses its output.
# XO_SEED_REF (dry runs only, default origin/main) is the commit the cut
# builds and checks this script against: a branch changing the seed
# scripts rehearses itself (run from that branch's worktree).
# XO_HEAVY, when set, prefixes the heavy steps (the lead's slot lock,
# e.g. XO_HEAVY="$XO_LEAD_SCRATCH/agents/heavy.sh seed").
# XO_PUSH_LOCK, when set, is the push lock directory that pushq.sh takes
# (e.g. "$XO_LEAD_SCRATCH/locks/push"): seed-cut takes it (mkdir, a "who"
# file of NAME@PID, a lock whose PID is gone is stale) before it reads
# origin/main and holds it through the gate until its push, so main
# cannot move under the gate (2026-10-21: the 0.24.0 cut passed its gate
# and its push was refused, main having moved in the 20 minutes). Other
# pushes wait meanwhile. A dry run takes no lock.
# XO_RACE_REMOTE_LINUX_AMD64, when set, builds linux-amd64's race runtime
# on a linux/amd64 machine instead of Docker's amd64 emulation (passed to
# scripts/race-runtime.sh; the lead's "$XO_LEAD_SCRATCH/agents/race-remote.sh"
# over the amd64 offload host). Unset, Docker must run linux/amd64
# containers, checked before anything is built (2026-11-10: Docker
# Desktop answered "exec format error" and the 0.46.0 cut failed at step 6,
# gotchas/docker-amd64-emulation-exec-format.md).
# Without the lock (2026-11-06), a push refused because main moved puts
# the version commit on the new main in a worktree of its own, checks that
# it changes only the version files, and pushes that; the tag stays on the
# gated commit the seed is built from (off main's first parent line then),
# and its message names the commit main got.
set -euo pipefail

die() { echo "seed-cut: $*" >&2; exit 1; }
step() { echo "== $(date +%H:%M:%S) $*"; }
# heavy LOG CMD...: CMD with its output in LOG, under $XO_HEAVY when set
# (heavy.sh sends a command's output to its own log, so the redirect is
# made inside it)
heavy() {
  local l=$1; shift
  if [ -n "${XO_HEAVY:-}" ]; then $XO_HEAVY sh -c 'l=$1; shift; exec "$@" > "$l" 2>&1' sh "$l" "$@"; else "$@" > "$l" 2>&1; fi
}

[ $# -eq 3 ] || die "usage: seed-cut.sh VERSION NOTES WORKDIR (XO_SEED_TAG=vX.Y.Z for a patch seed)"
version=$1
patch_tag=${XO_SEED_TAG:-}
notes=$(cd "$(dirname "$2")" && pwd)/$(basename "$2")
work=$3
dry=${XO_SEED_DRY_RUN:-0}
ref=origin/main
if [ -n "${XO_SEED_REF:-}" ]; then
  [ "$dry" = 1 ] || die "XO_SEED_REF is for dry runs (XO_SEED_DRY_RUN=1)"
  ref=$XO_SEED_REF
fi

if [ -n "$patch_tag" ]; then
  echo "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || die "version $version: want X.Y.Z"
  [ "$patch_tag" = "v$version" ] || die "XO_SEED_TAG $patch_tag: want v$version"
else
  echo "$version" | grep -Eq '^[0-9]+\.[0-9]+\.0$' || die "version $version: want X.Y.0"
  [ -s "$notes" ] || die "$notes: write the tag's message first"
  head -1 "$notes" | grep -qx "Xo v$version" || die "$notes: its first line must be \"Xo v$version\""
fi
# heavy.sh takes NAME before the command: without it the first heavy step
# ran `-c` as the command and failed after the 9 minute -short step
# (2026-11-06, the 0.43.0 cut)
case "${XO_HEAVY:-}" in
  *heavy.sh) die "XO_HEAVY=$XO_HEAVY: give heavy.sh a job name (XO_HEAVY=\"\$HOME/xo-lead/agents/heavy.sh seed\")" ;;
esac
[ "$(uname -s)-$(uname -m)" = Darwin-arm64 ] || die "run on the development Mac (darwin/arm64): the darwin-arm64 seed is built natively"
command -v go >/dev/null && command -v clang >/dev/null || die "needs go and clang"
# before anything is pushed: step 6 builds linux-arm64's race runtime in it
# a daemon just started can answer once and then not for a few seconds
# (the 0.44.0 cut's first try): one minute of retries, then the fix
d=0
until docker info >/dev/null 2>&1; do
  [ $d -lt 12 ] || die "Docker is not answering after a minute (step 6 builds the linux-arm64 race runtime in it): start Docker Desktop (open -a Docker), wait for docker info to succeed, then rerun"
  sleep 5; d=$((d + 1))
done
if [ -z "${XO_RACE_REMOTE_LINUX_AMD64:-}" ] && [ "$(docker run --rm --platform linux/amd64 alpine:3.20 uname -m 2>&1 | tail -1)" != x86_64 ]; then
  die "Docker cannot run linux/amd64 containers (step 6 builds linux-amd64's race runtime in one): set XO_RACE_REMOTE_LINUX_AMD64 (the lead's agents/race-remote.sh), or turn on Docker Desktop's x86_64/amd64 emulation (Rosetta)"
fi

repo=$(git rev-parse --show-toplevel)
# a stale copy of this script (e.g. the main checkout, kept at an old commit)
# runs old paths and steps: run it from a checkout of origin/main
# (2026-10-18: a pre-rename copy read selfhost/ paths and failed)
git -C "$repo" fetch -q origin
git -C "$repo" diff --quiet "$ref" -- scripts/seed-cut.sh scripts/seed-pack.sh scripts/seed-upload.sh scripts/race-runtime.sh scripts/race-windows.cpp ||
  die "this checkout's seed scripts differ from $ref's: run seed-cut.sh from a worktree of $ref"
case "$work" in "$repo"/*) die "WORKDIR must be outside the repository" ;; esac
mkdir -p "$work"
work=$(cd "$work" && pwd)
# no packed file may name WORKDIR, which is under the cutter's home
# directory (seed-pack.sh; the TSan runtimes did through 0.33.0)
export XO_SEED_FORBID=$work
# whole program builds, as release.sh: xobuild's darwin-arm64 build is a
# host build, which links std's separately compiled object (decision 0388)
# when the cache holds one, so the seed would depend on the cache (the
# 0.47.0 seed was one; gotchas/seed-built-separately-compiled.md)
export XO_SEPCOMP=0
# cold builds, as scripts/release.sh (lead, 2026-11-12): no cache feeds a
# seed. The back-end, front-end, module check and binary caches are off for
# every xo build here (the cross builds of step 5 among them), and xobuild's
# xo is built from the seed in an empty checkout cache.
export XO_BACKEND_CACHE=off XO_FE_CACHE=off XO_CHK_CACHE=off XO_BIN_CACHE=off
if [ -z "${XO_CHECKOUT_CACHE:-}" ]; then
  XO_CHECKOUT_CACHE=$(mktemp -d "${TMPDIR:-/tmp}/xo-seed-checkout.XXXXXX") || die "mktemp"
  export XO_CHECKOUT_CACHE
fi
[ -z "$(ls -A "$work")" ] || die "$work is not empty"
# a worktree a deleted WORKDIR left registered fails step 1's worktree
# add after the checks (the 0.44.0 cut's third try,
# gotchas/seed-cut-environment-failures.md): name it here, with its fix
stale=$(git -C "$repo" worktree list --porcelain | sed -n "s|^worktree \($work/.*\)|\1|p")
[ -z "$stale" ] || die "$(printf 'a worktree under the empty %s is still registered: %s; run: git -C %s worktree remove --force PATH (each)' "$work" "$(echo $stale)" "$repo")"
log=$work/logs
mkdir -p "$log"

locked=""
if [ -n "${XO_PUSH_LOCK:-}" ] && [ "$dry" != 1 ] && [ -z "$patch_tag" ]; then
  waited=0
  until mkdir "$XO_PUSH_LOCK" 2>/dev/null; do
    holder=$(cat "$XO_PUSH_LOCK/who" 2>/dev/null || true); hpid=${holder##*@}
    if [ -n "$hpid" ] && ! kill -0 "$hpid" 2>/dev/null; then
      echo "seed-cut: stale push lock from $holder, taking it"; rm -rf "${XO_PUSH_LOCK:?}"; continue
    fi
    [ $((waited % 60)) = 0 ] && echo "seed-cut: waiting for the push lock ($holder)"
    sleep 5; waited=$((waited + 5))
  done
  echo "seed-$version@$$" > "$XO_PUSH_LOCK/who"
  locked=$XO_PUSH_LOCK
  trap 'rm -rf "${locked:?}"' EXIT
fi

git -C "$repo" fetch -q origin --tags
if [ -n "$patch_tag" ]; then
  git -C "$repo" rev-parse -q --verify "refs/tags/$patch_tag^{commit}" >/dev/null || die "tag $patch_tag does not exist"
elif git -C "$repo" rev-parse -q --verify "refs/tags/v$version" >/dev/null; then
  die "tag v$version exists"
fi
cur=$(git -C "$repo" show "$ref":compiler/cli/core/cli.xo | sed -n 's/^pub const VERSION = "\(.*\)"$/\1/p')
[ -n "$cur" ] || die "no VERSION in origin/main's compiler/cli/core/cli.xo"
newest=$(printf '%s\n%s\n' "$cur" "$version" | sort -t. -k1,1n -k2,2n -k3,3n | tail -1)
prebumped=0
if [ -n "$patch_tag" ]; then
  # the tag carries its own version: check all three files there
  t=refs/tags/$patch_tag
  for f in compiler/cli/core/cli.xo compiler/cli/servers/servers.xo std/VERSION; do
    git -C "$repo" show "$t:$f" | grep -qx "\(pub const \(XO_\)\{0,1\}VERSION = \)\{0,1\}\"\{0,1\}$version\"\{0,1\}" ||
      die "$patch_tag's $f does not say $version"
  done
  prebumped=1
  ref=$t
elif [ "$version" = "$cur" ]; then
  # main already carries the bump (2026-10-18: a manual "Version X" commit):
  # cut from origin/main as is when its head is that commit
  git -C "$repo" log -1 --format=%s "$ref" | grep -q "^Version $version " ||
    die "main is already at $version but its head is not the \"Version $version\" commit; tag or bump by hand"
  prebumped=1
else
  [ "$newest" = "$version" ] || die "version $version is not above main's $cur"
fi

step "1. worktree of $ref, version $cur to $version"
tree=$work/tree
git -C "$repo" worktree add -q --detach "$tree" "$ref"
cd "$tree"
if [ "$prebumped" = 0 ]; then
  sed -i '' "s/^pub const VERSION = \"$cur\"$/pub const VERSION = \"$version\"/" compiler/cli/core/cli.xo
  sed -i '' "s/^pub const XO_VERSION = \"$cur\"$/pub const XO_VERSION = \"$version\"/" compiler/cli/servers/servers.xo
  # std's stamp (an xo older than its std warns: compiler/xo provenance.xo)
  echo "$version" > std/VERSION
  # the spec drafts move on to the next minor (decision 0089; internal/repotest
  # TestSpecDraftFollowsVersion): both Status lines, and an empty "Changes in"
  # section on top of core.md's
  minor=${version#*.}; minor=${minor%%.*}
  draft=${version%%.*}.$((minor + 1)); tagged=${version%.*}
  for f in spec/core.md spec/std.md; do
    grep -q "^Status: draft $draft " "$f" && continue
    sed -i '' -E "s/^Status: draft [0-9]+\.[0-9]+ \([0-9-]+; [^)]*\)/Status: draft $draft ($(date +%Y-%m-%d); PLACEHOLDER)/" "$f"
    grep -q "^Status: draft $draft " "$f" || die "$f: no Status: draft line to move to $draft"
  done
  sed -i '' "s/^Status: draft $draft (\([0-9-]*\); PLACEHOLDER)$/Status: draft $draft (\1; $tagged tagged as toolchain v$version, decision 0089)/" spec/core.md
  sed -i '' "s/^Status: draft $draft (\([0-9-]*\); PLACEHOLDER)$/Status: draft $draft (\1; the same draft number as \`spec\/core.md\`, decision 0089)/" spec/std.md
  if ! grep -q "^## Changes in $draft\$" spec/core.md; then
    awk -v d="$draft" '!done && /^## Changes in /{print "## Changes in " d; print ""; print "None yet."; print ""; done=1} {print}' spec/core.md > spec/core.md.new && mv spec/core.md.new spec/core.md
  fi
  [ "$(git diff --name-only | sort | tr '\n' ' ')" = "compiler/cli/core/cli.xo compiler/cli/servers/servers.xo spec/core.md spec/std.md std/VERSION " ] ||
    die "the bump changed $(git diff --name-only | tr '\n' ' ')(want cli.xo, servers.xo, the two spec drafts and std/VERSION)"
  git commit -qam "Version $version (seed $version, decision 0216; tag v$version)"
fi
commit=$(git rev-parse HEAD)

step "2. go test -short ./internal/xotool ./cmd/xo"
# the tests run with the caches at their defaults: the cold exports above are
# for the seed's builds, and tests such as cmd/xo TestLSPModuleCache check the
# caches themselves (the 0.50.0 cut failed it, 2026-11-12)
env -u XO_BACKEND_CACHE -u XO_FE_CACHE -u XO_CHK_CACHE -u XO_BIN_CACHE XO_SELF_DRIVER=0 GOMAXPROCS=4 go test -short -p 2 ./internal/xotool ./cmd/xo > "$log/short.log" 2>&1 &&
  env -u XO_BACKEND_CACHE -u XO_FE_CACHE -u XO_CHK_CACHE -u XO_BIN_CACHE go test -count=1 ./internal/repotest -run '^TestSpecDraftFollowsVersion$' >> "$log/short.log" 2>&1 ||
  { grep -E '^(FAIL|--- FAIL)' "$log/short.log" | head; die "-short failed ($log/short.log)"; }

step "3. gate: xobuild, hello, fixed point"
heavy "$log/xobuild.log" go run ./scripts/xobuild || { tail "$log/xobuild.log"; die "xobuild failed"; }
hello=$work/hello
mkdir -p "$hello"
printf 'module example.com/hello\nedition preview\n' > "$hello/xo.mod"
printf 'fn main(os: Os) {\n  os.stdio.println("hi")\n}\n' > "$hello/main.xo"
( cd "$hello" && "$tree/bin/xo" run --backend=arm64 . ) > "$log/hello.log" 2>&1 || { tail "$log/hello.log"; die "hello failed"; }
grep -qx hi "$log/hello.log" || die "hello printed no hi ($log/hello.log)"
GOMAXPROCS=6 heavy "$log/boot.log" sh scripts/bootstrap-xo.sh "$tree/bin/xo" "$work/boot" ||
  { tail -20 "$log/boot.log"; die "fixed point failed"; }
grep -q 'fixed point: s2 and s3 binaries identical' "$log/boot.log" || die "fixed point not reached ($log/boot.log)"

if [ -n "$patch_tag" ]; then
  step "4. patch seed: $patch_tag exists at $commit, nothing to push"
elif [ "$dry" = 1 ]; then
  step "4. dry run: not pushing $commit, no tag"
else
  step "4. push $commit to main, tag v$version"
  pushed=$commit
  cp "$notes" "$work/tag-notes.txt"
  if ! git push -q origin HEAD:main 2>"$log/push.log"; then
    # main moved since step 1: put the version commit on the new main in a
    # worktree of its own (the tree stays at $commit, which steps 2 and 3
    # gated and steps 5 and 6 build), check that it changes only the
    # version files, and push that; the tag stays on $commit, whose tree
    # is the one gated and built (a tag needs not be on main's first parent
    # line), so the seed's source is the tag's, and its message names both
    [ "$prebumped" = 0 ] || die "push refused and main's version commit was made by hand: rerun with a new WORKDIR"
    git fetch -q origin
    onto=$work/push
    rm -rf "${onto:?}"; git worktree prune
    git worktree add -q --detach "$onto" origin/main
    ( cd "$onto" && git cherry-pick "$commit" ) > "$log/push.log" 2>&1 ||
      { tail "$log/push.log"; die "the version commit does not apply to the new main"; }
    pushed=$(git -C "$onto" rev-parse HEAD)
    [ "$(git -C "$onto" diff --name-only origin/main "$pushed" | sort | tr '\n' ' ')" = "compiler/cli/core/cli.xo compiler/cli/servers/servers.xo spec/core.md spec/std.md std/VERSION " ] ||
      die "the rebased version commit changes more than the version files"
    git -C "$onto" push -q origin HEAD:main || die "push refused twice (main moved again?): rerun with a new WORKDIR"
    git worktree remove --force "$onto"
    printf '\nThis tag is %s, the gated commit the seed was built from; main moved during the cut, so its version commit landed on main as %s (the same version files on the newer main).\n' "$commit" "$pushed" >> "$work/tag-notes.txt"
  fi
  git tag -a "v$version" -F "$work/tag-notes.txt" "$commit"
  git push -q origin "v$version"
  if [ -n "$locked" ]; then
    rm -rf "${locked:?}"; trap - EXIT; locked=""
  fi
fi

step "5. darwin-arm64 at -O2"
out=$work/out
mkdir -p "$out"
XO_LLVM_OPT=O2 heavy "$log/xobuild-o2.log" go run ./scripts/xobuild || { tail "$log/xobuild-o2.log"; die "xobuild -O2 failed"; }
# ThreadSanitizer's runtime for native --race (decision 0273 part 3) into
# the tree's lib/xo/race, which seed-pack.sh packs
heavy "$log/race-runtime.log" bash "$repo/scripts/race-runtime.sh" "$work/race" "$tree" || { tail "$log/race-runtime.log"; die "race-runtime.sh failed"; }
[ "$(bin/xo version | awk '{print $2}')" = "$version" ] || die "bin/xo version: $(bin/xo version), want xo $version"
XO_SEED_COMMIT=$commit XO_SEED_NOTE="native darwin/arm64 build at -O2, fixed point ok (seed-cut.sh)" \
  bash scripts/seed-pack.sh bin/xo "$out" > "$log/pack-darwin-arm64.log"

step "6. cross builds"
# each platform's native backend, as scripts/release.sh builds the
# releases (decision 0362; the Linux ones with compiler/macho's ELF link
# and no C library); XO_SEED_CROSS_BACKEND=go builds them all with the Go
# backend as before
for p in linux/arm64 linux/amd64 windows/amd64 darwin/amd64 windows/arm64; do
  plat=${p/\//-}; exe=xo; [ "${p%/*}" = windows ] && exe=xo.exe
  d=$work/cross/$plat
  mkdir -p "$d/bin"
  be=${XO_SEED_CROSS_BACKEND:-${p#*/}}
  ld=(); [ "$p" = linux/arm64 ] && [ "$be" = arm64 ] && ld=(--ld=macho)
  bin/xo build --backend="$be" ${ld[@]+"${ld[@]}"} --targets="$p" --std="$tree/std" -o "$d/bin/$exe" compiler/xo > "$log/cross-$plat.log" 2>&1 ||
    { tail "$log/cross-$plat.log"; die "cross build $plat failed"; }
  (git ls-files -z -- std | tar -c -f - --null -T -) | tar -xf - -C "$d"
  # ThreadSanitizer's runtime for native --race (decision 0273's linux,
  # amd64 and Windows addenda) into $d/lib/xo/race, which seed-pack.sh
  # packs: the Linux static archives and Windows' Go flavor object built
  # in Docker, darwin-amd64's dylib cross built here
  # (windows/arm64 has no ThreadSanitizer runtime: --race is not
  # offered there, decision 0569)
  # the cutter's race-runtime.sh (checked against origin/main above), not
  # the tree's: a patch seed's tree is an older tag, whose copy lacked
  # XO_RACE_REMOTE_LINUX_AMD64 (the 0.46.0 cut's second try, 2026-11-10)
  if [ "$plat" != windows-arm64 ]; then
    bash "$repo/scripts/race-runtime.sh" "$work/race-$plat" "$d" "$plat" > "$log/race-runtime-$plat.log" 2>&1 ||
      { tail "$log/race-runtime-$plat.log"; die "race-runtime.sh $plat failed"; }
  fi
  XO_SEED_VERSION=$version XO_SEED_PLATFORM=$plat XO_SEED_COMMIT=$commit \
    XO_SEED_NOTE="cross built by the darwin-arm64 $version xo with --backend=$be --targets=$p (seed-cut.sh)" \
    bash scripts/seed-pack.sh "$d/bin/$exe" "$out" > "$log/pack-$plat.log"
  echo "   $plat ok"
done

seed=$version-${commit:0:8}
n=$(ls "$out"/*.tar.gz | wc -l | tr -d ' ')
[ "$n" = 6 ] || die "$n tarballs in $out, want 6"
echo "$seed" > "$out/SEED"
[ "$dry" = 1 ] && echo dry-run > "$out/DRY_RUN"
cat "$out/manifest.txt"
echo "SEED PACKED: $seed ($out)"
