#!/bin/bash
# docker-llvm-smoke.sh: a Linux LLVM smoke test in Docker (linux/arm64):
# the darwin xo XO_CLI cross builds a linux/arm64 xo (LLVM links only for
# its host), which builds hello and the async_preempt_regs golden with
# --backend=llvm in scripts/docker-linux.sh's image twice, linked by
# ld.bfd (with the LLVM plugin, the image's default) and by ld.lld, and
# runs each against its want.txt's stdout.
#
#   scripts/docker-llvm-smoke.sh XO_CLI
#
# Exit 0 when all four build and print what they should, 1 when one does
# not, 2 on a usage or setup error; 0 with a note when Docker is not
# running (hourly.sh runs it on the Mac). It exists because the darwin
# gates use the C library's start and Apple's linker: decision 0318 stage
# 4 broke every Linux LLVM build twice (a link error, then a crash at
# start) and no darwin gate saw it (gotchas/llvm-linux-section-bounds-and-start.md).
# About three minutes, most of it the cross build.
set -uo pipefail

xo=${1:?usage: docker-llvm-smoke.sh XO_CLI}
[ -x "$xo" ] || { echo "docker-llvm-smoke: $xo is not an executable xo" >&2; exit 2; }
if ! docker info >/dev/null 2>&1; then
  echo "docker-llvm-smoke: skipped, Docker is not running"
  exit 0
fi
root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/xo-llvm-smoke.XXXXXX")
trap 'rm -rf "${work:?}"' EXIT
mkdir -p "$work/xo/bin" "$work/progs/hello"

(cd "$root/compiler/xo" && XO_STD="$root/std" XO_BIN_CACHE=off "$xo" build --backend=arm64 --targets=linux/arm64 -o "$work/xo/bin/xo" .) > "$work/cross.log" 2>&1 ||
  { tail -5 "$work/cross.log" >&2; echo "docker-llvm-smoke: the linux/arm64 xo did not build" >&2; exit 2; }
cp -R "$root/std" "$work/xo/std"
printf 'module example.com/hello\n' > "$work/progs/hello/xo.mod"
printf 'fn main(os: Os) {\n  os.stdio.println("hi")\n}\n' > "$work/progs/hello/main.xo"
printf 'exit 0\n--- stdout\nhi\n--- stderr\n' > "$work/progs/hello/want.txt"
cp -R "$root/internal/llvm/testdata/golden/async_preempt_regs" "$work/progs/"

cat > "$work/inside.sh" <<'IN'
#!/bin/bash
# as root: /usr/bin/ld points at each linker in turn
orig=$(readlink -f /usr/bin/ld)
fail=0
for ld in bfd lld; do
  case $ld in
    bfd) ln -sf "$orig" /usr/bin/ld ;;
    lld) ln -sf "$(command -v ld.lld)" /usr/bin/ld ;;
  esac
  for p in hello async_preempt_regs; do
    d=/work/progs/$p
    if ! (cd $d && XO_STD=/work/xo/std XO_BIN_CACHE=off XO_BACKEND_CACHE=off /work/xo/bin/xo build --backend=llvm -o /work/bin-$p . > /work/build.log 2>&1); then
      echo "FAIL $ld $p: build"; head -c 1500 /work/build.log; fail=1; continue
    fi
    envs=$( [ -f $d/env.txt ] && cat $d/env.txt || true )
    got=$(env $envs /work/bin-$p 2>/dev/null)
    want=$(sed -n '/^--- stdout$/,/^--- stderr$/p' $d/want.txt | sed '1d;$d')
    if [ "$got" = "$want" ]; then echo "ok $ld $p"; else echo "FAIL $ld $p: output"; echo "got:  $got"; echo "want: $want"; fail=1; fi
  done
done
ln -sf "$orig" /usr/bin/ld
exit $fail
IN
chmod +x "$work/inside.sh"
st=0
XO_DOCKER_ROOT=1 nice -n 10 "$root/scripts/docker-linux.sh" arm64 "$work" -- ./inside.sh || st=$?
case $st in
  0) echo "docker-llvm-smoke: ok" ;;
  1) echo "docker-llvm-smoke: FAILED" >&2 ;;
  *) echo "docker-llvm-smoke: the container failed ($st)" >&2; st=2 ;;
esac
exit $st
