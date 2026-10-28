#!/bin/bash
# release-verify.sh: check a release archive of scripts/release.sh on the
# host it was built for (decision 0178, switch point B):
#
#   scripts/release-verify.sh ARCHIVE [OUTDIR]
#
# Run from a checkout of the commit the archive was built from (the cmd/xo
# tests come from it). OUTDIR defaults to .tools/release-verify. Steps:
#
#   1. unpack ARCHIVE into OUTDIR (a clean layout: bin/xo, lib/xo/...),
#      and check that xo-gobind's source under lib/xo builds with this
#      machine's Go (decision 0393).
#   2. without Go: every directory holding `go` is dropped from PATH,
#      GOROOT unset, XO_GO pointing at nothing, empty caches. `xo
#      version`, `xo check`, and a hello program
#      with `xo run` on each native backend: arm64 with the default
#      linker (compiler/macho on darwin with the pack, decision 0182; cc
#      on linux), arm64 with --ld=cc, arm64 with --ld=macho (no cc), llvm
#      (clang); and the time zone golden on arm64, its zone data from the
#      archive's lib/xo/std/tzdata/zoneinfo.zip (decision 0094's addendum). On a host with no native backend
#      (linux/amd64) these are expected to fail and are reported as
#      "unsupported", not as failures; `xo build` with the default (Go)
#      backend must say Go is missing.
#      Also without Go, the checks of bugs 0.25.2 shipped (lead,
#      2026-10-22): `xo build --targets=all` of hello gives the six
#      binaries a native backend builds, NAME-OS-ARCH (windows/arm64 needs
#      the Go backend); `xo build --race` and a run with no clang on PATH
#      report the planted race with a main.xo line (where lib/xo/race
#      ships for the host); the time_civil golden (daylight saving gaps
#      and overlaps, Earlier never after Later) matches; on darwin, atos
#      names main.xo for a function of a built binary (the embedded DWARF,
#      decision 0346; CoreSymbolication on the Intel laptop's macOS).
#      And decision 0388's first use: a first build goes whole program and
#      starts std's variant in the background, and a later edit links
#      through it (sepcomp-first-use; UNSUPPORTED until the default is on).
#   3. with Go: a hello program on the Go backend, a `use go` program
#      (bin/xo-gobind), then cmd/xo's whole suite (no -short) with XO_CLI
#      = the archive's bin/xo, no XO_STD (the switch point B gate; since
#      decision 0215 cmd/xo holds only these tests: there is no Go xo to
#      compare with); and on arm64 with 5 GB free, internal/arm64
#      TestStrViewPast2GiB (a Str view past 2 GiB).
#
# Also, without Go: `xo version --json` names the commit this checkout is
# at (release.sh stamps it; a release is built from the published tree),
# and on darwin bin/xo and bin/xo-gobind carry a Developer ID signature
# that codesign --verify --strict accepts and spctl calls notarized
# (XO_VERIFY_UNSIGNED=1 reports these as UNSUPPORTED, for trial archives).
# The native checks use the host's backend (arm64, or amd64 on an Intel
# Mac and linux/amd64).
#
# A windows archive is checked on any host for what does not need
# Windows: its layout and the Authenticode signatures of bin/xo.exe and
# bin/xo-gobind.exe (scripts/verify-windows-sig.sh: osslsigncode against
# Microsoft's roots, the timestamp, XO_WIN_SIGNER when set;
# XO_VERIFY_UNSIGNED=1 reports them as UNSUPPORTED). Running it is
# scripts/ci/release-smoke.sh's job on the Windows runner.
#
# Prints "PASS name", "FAIL name", or "UNSUPPORTED name" per check and a
# summary line; exit status 0 when nothing failed.
set -u
[ $# -ge 1 ] || { echo "usage: $0 ARCHIVE [OUTDIR]" >&2; exit 2; }
archive=$1
case "$archive" in /*) ;; *) archive=$(pwd)/$archive ;; esac
cd "$(dirname "$0")/.."
root=$(pwd)
out=${2:-.tools/release-verify}
case "$out" in /*) ;; *) out=$root/$out ;; esac
rm -rf "$out"
mkdir -p "$out/unpack" "$out/shim" "$out/hello" "$out/usego/app" "$out/tz" "$out/civil" "$out/race" "$out/all" "$out/noclang"
cp internal/gen/testdata/golden/time_zones/main.xo "$out/tz/main.xo"
cp internal/gen/testdata/golden/time_civil/main.xo "$out/civil/main.xo"
tar -xzf "$archive" -C "$out/unpack" || { echo "FAIL unpack"; exit 1; }
rel=$(echo "$out"/unpack/*)
xo=$rel/bin/xo
host="$(uname -s | tr A-Z a-z)/$(uname -m | sed 's/aarch64/arm64/; s/x86_64/amd64/')"
fails=0
pass() { echo "PASS $1"; }
fail() { echo "FAIL $1"; fails=$((fails + 1)); }
native=1
nb=${host#*/}
# check NAME CMD...: runs CMD, its output in OUTDIR/NAME.txt
check() {
  local name=$1; shift
  if "$@" > "$out/$name.txt" 2>&1; then pass "$name"; else
    if [ "$native" = 0 ] && [[ "$name" == native-* ]]; then echo "UNSUPPORTED $name ($(tail -1 "$out/$name.txt"))"
    else fail "$name"; tail -5 "$out/$name.txt" | sed 's/^/  /'; fi
  fi
}
echo "host $host; archive $(basename "$archive"); $(uname -sr)"
case "$(basename "$archive")" in
  *-windows-*)
    for f in xo.exe xo-gobind.exe; do
      if [ -f "$rel/bin/$f" ]; then pass "layout-$f"; else fail "layout-$f"; continue; fi
      if [ "${XO_VERIFY_UNSIGNED:-}" = 1 ]; then
        echo "UNSUPPORTED signed-$f (XO_VERIFY_UNSIGNED=1)"
      elif bash "$root/scripts/verify-windows-sig.sh" "$rel/bin/$f" > "$out/sign-$f.txt" 2>&1; then
        pass "signed-$f ($(sed -n 's/.*: ok //p' "$out/sign-$f.txt"))"
      else fail "signed-$f"; sed 's/^/  /' "$out/sign-$f.txt"; fi
    done
    for f in lib/xo/std/prelude.xo lib/xo/go.mod lib/xo/cmd/xo-gobind/main.go LICENSE THIRD_PARTY_NOTICES; do
      if [ -f "$rel/$f" ]; then pass "layout-$f"; else fail "layout-$f"; fi
    done
    echo "release-verify: $fails failed (a windows archive: run it with scripts/ci/release-smoke.sh on Windows)"
    [ "$fails" = 0 ]
    exit
    ;;
esac
# xo-gobind's source (decision 0393): there, and it builds with this Go
for f in lib/xo/go.mod lib/xo/cmd/xo-gobind/main.go lib/xo/internal/gobind/gobind.go lib/xo/internal/gotool/gotool.go; do
  if [ -f "$rel/$f" ]; then pass "layout-$f"; else fail "layout-$f"; fi
done
if (cd "$rel/lib/xo" && go build -o "$out/xo-gobind-local" ./cmd/xo-gobind) > "$out/gobind-source.txt" 2>&1; then
  pass "gobind-source-builds ($(go env GOVERSION))"
else fail "gobind-source-builds"; tail -5 "$out/gobind-source.txt" | sed 's/^/  /'; fi
printf 'fn main(os: Os) uses (stdio) {\n  os.stdio.println("hello from the released xo")\n}\n' > "$out/hello/main.xo"
printf 'module ex\n' > "$out/usego/xo.mod"
printf 'use go "strings"\n\nfn main(os: Os) uses (stdio, go) {\n  os.stdio.println(strings.to_upper("hello from use go"))\n}\n' > "$out/usego/app/main.xo"

# 2. without Go
gopath_env=$PATH
newpath=""
IFS=: read -ra dirs <<< "$PATH"
for d in "${dirs[@]}"; do
  [ -n "$d" ] || continue
  if [ -e "$d/go" ] || [ -e "$d/gofmt" ]; then
    for f in "$d"/*; do
      b=$(basename "$f")
      case "$b" in go|gofmt|go[0-9]*) continue ;; esac
      [ -x "$f" ] && [ ! -e "$out/shim/$b" ] && ln -s "$f" "$out/shim/$b"
    done
    d=$out/shim
    case ":$newpath:" in *":$d:"*) continue ;; esac
  fi
  newpath=${newpath:+$newpath:}$d
done
(
  mkdir -p "$out/home"; export PATH=$newpath XO_GO=/nonexistent/go HOME="$out/home"
  export XO_BUILD_DIR="$out/cache-nogo/build" XO_RTOBJ_CACHE="$out/cache-nogo/rtobj" XO_BIN_CACHE=off
  unset GOROOT GOPATH GOFLAGS GOTOOLCHAIN XO_STD XO_RTPACK XO_CLI XDG_CACHE_HOME
  if command -v go >/dev/null 2>&1; then echo "FAIL no-go-on-path ($(command -v go))"; exit 1; fi
  echo "PASS no-go-on-path"
  check version "$xo" version
  cat "$out/version.txt"
  # the stamped commit is this checkout's (release.sh, decision 0264)
  commit=$("$xo" version --json 2>/dev/null | sed -n 's/.*"commit":"\([0-9a-f]*\)".*/\1/p')
  head=$(git -C "$root" rev-parse HEAD)
  if [ -n "$commit" ] && [ "$commit" = "$head" ]; then pass "stamp-commit ($commit)"
  else fail "stamp-commit: xo version --json says '${commit}', this checkout is $head"; fi
  if [ "${host%/*}" = darwin ]; then
    for f in "$xo" "$rel/bin/xo-gobind"; do
      b=$(basename "$f")
      if [ "${XO_VERIFY_UNSIGNED:-}" = 1 ]; then
        echo "UNSUPPORTED signed-$b (XO_VERIFY_UNSIGNED=1)"
        continue
      fi
      if codesign --verify --strict "$f" 2>"$out/sign-$b.txt" && codesign -dvv "$f" 2>&1 | grep -q '^Authority=Developer ID Application'; then
        pass "signed-$b"
      else fail "signed-$b"; codesign -dvv "$f" 2>&1 | grep -E '^(Authority|Signature)' | sed 's/^/  /'; fi
      if spctl --assess --type open --context context:primary-signature -vv "$f" 2>&1 | grep -q 'source=Notarized Developer ID'; then
        pass "notarized-$b"
      else fail "notarized-$b"; spctl --assess --type open --context context:primary-signature -vv "$f" 2>&1 | sed 's/^/  /'; fi
    done
  fi
  check check "$xo" check "$out/hello"
  check native-$nb-run "$xo" run --backend=$nb "$out/hello"
  check native-$nb-ld-cc-run "$xo" run --backend=$nb --ld=cc "$out/hello"
  check native-$nb-ld-macho-run "$xo" run --backend=$nb --ld=macho "$out/hello"
  check native-llvm-run "$xo" run --backend=llvm "$out/hello"
  check native-$nb-tz-run "$xo" build --backend=$nb -o "$out/tz/tz" "$out/tz"
  if [ "$native" = 1 ]; then
    # want.txt is "exit 0", "--- stdout", the output, "--- stderr"
    sed -n '/^--- stdout$/,/^--- stderr$/p' internal/gen/testdata/golden/time_zones/want.txt | sed '1d;$d' > "$out/tz/want.txt"
    "$out/tz/tz" > "$out/tz/got.txt" 2>&1
    if cmp -s "$out/tz/got.txt" "$out/tz/want.txt"; then
      pass native-$nb-tz-output
    else
      fail native-$nb-tz-output; diff "$out/tz/got.txt" "$out/tz/want.txt" | head -5 | sed 's/^/  /'
    fi
  fi
  # the time_civil golden: daylight saving gaps and overlaps
  check native-$nb-civil-run "$xo" build --backend=$nb -o "$out/civil/civil" "$out/civil"
  if [ "$native" = 1 ]; then
    sed -n '/^--- stdout$/,/^--- stderr$/p' internal/gen/testdata/golden/time_civil/want.txt | sed '1d;$d' > "$out/civil/want.txt"
    "$out/civil/civil" > "$out/civil/got.txt" 2>&1
    if cmp -s "$out/civil/got.txt" "$out/civil/want.txt"; then pass native-$nb-civil-output
    else fail native-$nb-civil-output; diff "$out/civil/got.txt" "$out/civil/want.txt" | head -5 | sed 's/^/  /'; fi
  fi
  # decision 0388's first use (lead, 2026-10-28): the release ships no std
  # stores, so a user's first build goes whole program and starts std's
  # variant in the background (stdcache.xo std_variant_peek); a later
  # build links through it. UNSUPPORTED while this xo's default is not
  # separate compilation (no "std variant" line).
  sc="$out/sepcomp"; mkdir -p "$sc/p"; cp "$out/hello/main.xo" "$sc/p/main.xo"
  XO_SEPCOMP_BG=1 XO_BACKEND_CACHE_DIR="$sc/cache" "$xo" build --phase-times -o "$sc/b1" "$sc/p" > "$sc/1.txt" 2>&1
  if ! grep -q "std variant" "$sc/1.txt"; then
    echo "UNSUPPORTED sepcomp-first-use (this xo's default is the whole program)"
  elif ! grep -q "std variant: miss, building in the background" "$sc/1.txt" || [ ! -x "$sc/b1" ]; then
    fail sepcomp-first-use; grep "std variant\|xo:" "$sc/1.txt" | head -3 | sed 's/^/  /'
  else
    hit=0
    for i in $(seq 1 60); do
      sleep 2
      printf 'fn main(os: Os) uses (stdio) {\n  os.stdio.println("hello from the released xo, edit %s")\n}\n' "$i" > "$sc/p/main.xo"
      XO_SEPCOMP_BG=1 XO_BACKEND_CACHE_DIR="$sc/cache" "$xo" build --phase-times -o "$sc/b2" "$sc/p" > "$sc/2.txt" 2>&1
      if grep -q "^cache std .* reused" "$sc/2.txt"; then hit=$i; break; fi
    done
    if [ "$hit" != 0 ] && "$sc/b2" | grep -q "edit $hit"; then pass "sepcomp-first-use (the variant ready after about $((hit * 2)) s, then an edit linked through it)"
    else fail sepcomp-first-use; tail -3 "$sc/2.txt" | sed 's/^/  /'; fi
  fi
  # --targets=all: the six binaries a native backend builds
  check native-targets-all "$xo" build --targets=all -o "$out/all/" "$out/hello"
  missing=""
  for f in hello-linux-amd64 hello-linux-arm64 hello-darwin-amd64 hello-darwin-arm64 hello-windows-amd64.exe hello-wasip1-wasm.wasm; do
    [ -s "$out/all/$f" ] || missing="$missing $f"
  done
  if [ -z "$missing" ]; then pass native-targets-all-binaries; else fail "native-targets-all-binaries (missing:$missing)"; fi
  # --race with no clang on PATH: the native ThreadSanitizer where it ships
  if [ -d "$rel/lib/xo/race/${host%/*}-${host#*/}" ]; then
    noclang=""
    IFS=: read -ra cdirs <<< "$PATH"
    for d in "${cdirs[@]}"; do
      [ -n "$d" ] || continue
      if ls "$d"/clang* >/dev/null 2>&1; then
        for f in "$d"/*; do
          b=$(basename "$f")
          case "$b" in clang*|cc|c++|gcc*|g++*|ld.lld|lld*) continue ;; esac
          [ -x "$f" ] && [ ! -e "$out/noclang/$b" ] && ln -s "$f" "$out/noclang/$b"
        done
        d=$out/noclang
        case ":$noclang:" in *":$d:"*) continue ;; esac
      fi
      noclang=${noclang:+$noclang:}$d
    done
    if PATH=$noclang command -v clang >/dev/null 2>&1; then
      fail "race-no-clang (clang still on PATH: $(PATH=$noclang command -v clang))"
    # built in place: it plants the race with std/internal, which only
    # the checkout's own modules may use
    elif (cd "$root" && PATH=$noclang "$xo" build --race -o "$out/race/race" internal/llvm/testdata/tsan_race) > "$out/race-build.txt" 2>&1; then
      XO_THREADS=1 "$out/race/race" > "$out/race-run.txt" 2>&1
      # windows/amd64's runtime is Go's flavor of TSan: "WARNING: DATA
      # RACE" (exit 66, "Found N data race(s)"), not LLVM's text (Windows
      # field report 0.26.0, W7)
      if grep -q -e "WARNING: ThreadSanitizer: data race" -e "WARNING: DATA RACE" "$out/race-run.txt" && grep -q "main.xo:[0-9]" "$out/race-run.txt"; then
        pass race-no-clang
      else fail race-no-clang; head -12 "$out/race-run.txt" | sed 's/^/  /'; fi
    else fail race-no-clang-build; tail -5 "$out/race-build.txt" | sed 's/^/  /'; fi
  else
    echo "UNSUPPORTED race-no-clang (no lib/xo/race for $host)"
  fi
  # atos on darwin: a function's address named with its main.xo line
  if [ "${host%/*}" = darwin ] && [ "$native" = 1 ] && [ -x "$out/tz/tz" ]; then
    addr=$(nm "$out/tz/tz" 2>/dev/null | awk '$3 == "_xo_f_main" || $3 == "xo_f_main" { print "0x" $1; exit }')
    if [ -z "$addr" ]; then fail "atos (no xo_f_main symbol)"
    elif atos -o "$out/tz/tz" -arch "$(uname -m)" "$addr" 2>&1 | tee "$out/atos.txt" | grep -q "main.xo:[0-9]"; then
      pass "atos ($(head -1 "$out/atos.txt"))"
    else fail atos; sed 's/^/  /' "$out/atos.txt"; fi
  fi
  for f in native-$nb-run native-$nb-ld-cc-run native-$nb-ld-macho-run native-llvm-run; do
    grep -q "hello from the released xo" "$out/$f.txt" 2>/dev/null && echo "  $f: $(head -1 "$out/$f.txt")"
  done
  # the Go backend with no Go must say so, not crash (native has been the
  # default backend since 2026-10-20)
  if "$xo" build --backend=go -o "$out/hello-go" "$out/hello" > "$out/go-missing.txt" 2>&1; then
    fail go-missing-reported
  elif grep -qi "go" "$out/go-missing.txt" && ! grep -qi "fault\|panic" "$out/go-missing.txt"; then
    pass go-missing-reported; sed 's/^/  /' "$out/go-missing.txt" | head -3
  else
    fail go-missing-reported; sed 's/^/  /' "$out/go-missing.txt" | head -5
  fi
  exit $fails
)
fails=$((fails + $?))

# 3. with Go
export PATH=$gopath_env
unset XO_STD XO_RTPACK XO_CLI XO_GO
if command -v go >/dev/null 2>&1; then
  check go-backend-run "$xo" run --backend=go "$out/hello"
  check use-go-run "$xo" run --backend=go "$out/usego/app"
  grep -h "hello" "$out/go-backend-run.txt" "$out/use-go-run.txt" 2>/dev/null | sed 's/^/  /'
  export XO_NO_SERVE=1 GOMAXPROCS=${GOMAXPROCS:-4}
  ( cd "$root" && XO_CLI="$xo" go test -count=1 -p 1 -parallel 2 -timeout 120m ./cmd/xo -v ) > "$out/cmdxo.log" 2>&1
  grep -E '^(--- |ok|FAIL|panic)' "$out/cmdxo.log" > "$out/cmdxo.txt"
  p=$(grep -c '^--- PASS' "$out/cmdxo.txt"); f=$(grep -c '^--- FAIL' "$out/cmdxo.txt"); s=$(grep -c '^--- SKIP' "$out/cmdxo.txt")
  echo "cmd/xo against the archive's xo: pass $p fail $f skip $s"
  grep -E '^--- (FAIL|SKIP)' "$out/cmdxo.txt" | sed 's/^/  /'
  # the first failure messages (file:line: text), for a remote run whose
  # full log stays on the instance
  grep -E '^[[:space:]]+[a-z_0-9]+_test\.go:[0-9]+:' "$out/cmdxo.log" | head -80 | cut -c1-400 | sed 's/^[[:space:]]*/    /'
  if [ "$f" = 0 ] && grep -q '^ok' "$out/cmdxo.txt"; then pass cmdxo-gate; else fail cmdxo-gate; fi
  # a Str view past 2 GiB (the 0.25.2 truncation): about 2.2 GB at its
  # peak, so only with 5 GB free; arm64 hosts (the test's own skip)
  freegb=0
  if [ "${host%/*}" = darwin ]; then
    freegb=$(vm_stat | awk '/page size of/ { ps = $8 } /Pages (free|inactive|speculative)/ { gsub("\\.", "", $NF); n += $NF } END { printf "%d", n * ps / 1073741824 }')
  elif [ -r /proc/meminfo ]; then
    freegb=$(awk '/^MemAvailable:/ { printf "%d", $2 / 1048576 }' /proc/meminfo)
  fi
  if [ "$nb" != arm64 ]; then echo "UNSUPPORTED str-view-past-2gib (not arm64)"
  elif [ "$freegb" -lt 5 ]; then echo "UNSUPPORTED str-view-past-2gib (${freegb} GB free, 5 needed)"
  else
    ( cd "$root" && XO_CLI="$xo" go test -count=1 -timeout 30m -run '^TestStrViewPast2GiB$' ./internal/arm64 ) > "$out/bigstr.txt" 2>&1
    if grep -q '^ok' "$out/bigstr.txt"; then pass str-view-past-2gib; else fail str-view-past-2gib; tail -5 "$out/bigstr.txt" | sed 's/^/  /'; fi
  fi
else
  echo "no go on PATH: the Go backend checks and cmd/xo's tests are not run"
  fail go-present
fi
echo "release-verify: $fails failed"
[ "$fails" = 0 ]
