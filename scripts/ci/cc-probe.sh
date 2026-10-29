#!/bin/bash
# xo-ci: before xobuild
# cc-probe.sh: what the C toolchain on a runner looks like, for a runner
# whose seed smoke build said only "xo: /usr/bin/cc failed" (the adopted
# amd64 machine, run 40813097594, 2026-10-28): the compilers and their
# versions, the OS, a hello C compile and link with its full stderr,
# /opt/llvm, the boot log's tail, and xotool's seed check with cc's
# stderr shown. Marked "before xobuild" so suite.sh runs it ahead of
# the xo build and ends the job with its status.
#   gh workflow run ci.yml -f ref=BRANCH -f suite=script -f runner=xo-amd64 -f packages=scripts/ci/cc-probe.sh
set -uo pipefail
cd "$(dirname "$0")/../.."; OLDPWD=$PWD
out=${XO_CI_OUT:-$(mktemp -d)}
mkdir -p "$out"
echo "== machine: $(uname -m) $(uname -r); $(grep PRETTY_NAME /etc/os-release 2>/dev/null)"
for c in cc gcc clang clang-15 ld ld.lld as; do
  p=$(command -v "$c" 2>/dev/null || true)
  echo "== $c: ${p:-none}$( [ -n "$p" ] && [ -L "$p" ] && echo " -> $(readlink -f "$p")")"
  [ -n "$p" ] && "$p" --version 2>&1 | head -2
done
echo "== /opt/llvm/bin:"; ls /opt/llvm/bin 2>&1 | head -5; echo "== /usr/local/bin:"; ls -la /usr/local/bin 2>&1 | head -12
echo "== PATH: $PATH"
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
printf '#include <stdio.h>\nint main(void) { puts("hello from cc"); return 0; }\n' > "$tmp/t.c"
for c in cc gcc clang; do
  command -v "$c" >/dev/null 2>&1 || continue
  echo "== $c hello:"; if "$c" -o "$tmp/t-$c" "$tmp/t.c" 2>&1; then "$tmp/t-$c"; else echo "$c FAILED (exit $?)"; fi
  echo "== $c hello with -fuse-ld=lld:"; "$c" -fuse-ld=lld -o "$tmp/t2-$c" "$tmp/t.c" 2>&1 | head -5
done
echo "== boot logs:"; for f in /var/log/xo-ci-boot.log /var/log/xo-ci-boot /var/log/xo-setup.log /var/log/cloud-init-output.log; do [ -r "$f" ] && { echo "-- $f (tail)"; tail -15 "$f"; }; done 2>&1 | cut -c1-200
echo "== boot units:"; systemctl list-units --no-pager 'xo-*' 2>&1 | head -8; ls -la /var/lib/xo-ready 2>&1
echo "== seed check (xotool, cc stderr kept):"
XO_XOBUILD_VERBOSE=1 go run ./scripts/xobuild 2>&1 | tail -25 | cut -c1-300
seed=$(ls -d ~/.cache/xo/seeds/*/linux-*/tree/xo/bin/xo 2>/dev/null | tail -1)
if [ -n "$seed" ]; then
  echo "== seed xo $seed: a hello program, its stderr:"
  mkdir -p "$tmp/h" && printf 'fn main(os: Os) {\n  os.stdio.println("hi")\n}\n' > "$tmp/h/main.xo" && printf 'module example.com/h\n' > "$tmp/h/xo.mod"
  (cd "$tmp/h" && XO_STD="$(dirname "$seed")/../std" "$seed" build -o "$tmp/h/h" . 2>&1 | head -20; "$tmp/h/h" 2>&1 | head -2)
  # xotool's smoke build itself (SmokeXo): the native backend named, the
  # CHECKOUT's std (not the seed's), full stderr
  echo "== smoke replica: seed xo, --backend=amd64, the checkout's std:"
  (cd "$tmp/h" && "$seed" build --backend=amd64 --std="$OLDPWD/std" -o "$tmp/h/s1" . 2>&1 | head -30; echo "rc=$?")
  echo "== smoke replica with the seed's own std:"
  (cd "$tmp/h" && "$seed" build --backend=amd64 --std="$(dirname "$seed")/../std" -o "$tmp/h/s2" . 2>&1 | head -30; echo "rc=$?")
  echo "== smoke replica as xotool runs it: XO_NOLIBC=0 (the cc link with libc), the checkout's std:"
  (cd "$tmp/h" && XO_NOLIBC=0 XO_TEST_JOBS=2 "$seed" build --backend=amd64 --std="$OLDPWD/std" -o "$tmp/h/s3" . 2>&1 | head -40; echo "rc=$?")
  echo "== cc static hello:"; cc -static -o "$tmp/ts" "$tmp/t.c" 2>&1 | head -5; [ -x "$tmp/ts" ] && "$tmp/ts"
  echo "== cc -no-pie hello:"; cc -no-pie -o "$tmp/tn" "$tmp/t.c" 2>&1 | head -5
  echo "== glibc-static, libc.a:"; rpm -q glibc-static 2>&1; ls /usr/lib64/libc.a /usr/lib64/libm.a /usr/lib64/libpthread.a 2>&1
  echo "== the checkout std's C parts (std/native, llrt) compiled by cc alone:"
  for c in $(ls "$OLDPWD"/std/native/*.c "$OLDPWD"/std/llrt/*.c 2>/dev/null | head -40); do cc -c -O2 -o "$tmp/o.o" "$c" 2>&1 | head -3 | sed "s#^#$(basename "$c"): #"; done
fi
cp -R "$tmp" "$out/probe" 2>/dev/null || true
echo "cc-probe done"
