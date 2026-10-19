#!/bin/bash
# wine-goldens.sh: build every golden for windows/amd64 with the LLVM
# backend on this Mac (the winxc cross toolchain) and run them under Wine
# with Box64 in Docker (scripts/docker-wine.sh box64), as
# scripts/linux-goldens.sh does for Linux. knowledgebase/infrastructure/
# toolchain.md "Windows programs under Wine in Docker".
#
#   scripts/wine-goldens.sh XO WORK [NAME...]
#
# XO is a darwin xo (its std beside it or $XO_STD); WORK a scratch
# directory (the programs in WORK/win/bin, logs in WORK/win/log, the
# summary in WORK/win/summary.txt). A golden's want_windows.txt replaces
# its want.txt. Each program runs in a fresh wineserver (killed after it:
# one crash or hang in a long lived wineserver failed the rest,
# gotchas/wine-box64-harness.md), Box64's own lines are dropped from its
# standard output, and a program stops after $XO_GOLDEN_TIMEOUT seconds
# (60); $XO_WINE_BACKEND picks the backend (llvm, the default, or amd64,
# decision 0254 6b). Known Wine gaps (symbolic links, the \\localhost share, sh,
# AF_UNIX, no PATH in the prefix) are in the toolchain notes.
set -eu
xo=$1
work=$2
shift 2
root=$(cd "$(dirname "$0")/.." && pwd)
X=${XO_WINXC:-$HOME/Library/Caches/xo/winxc}
mkdir -p "$work"
work=$(cd "$work" && pwd)
W=$work/win
rm -rf "$W/bin" "$W/gold" "$W/blog" "$W/log" "$W/w"
mkdir -p "$W/bin" "$W/gold" "$W/blog"
export XO_CLANG=$X/LLVM-23.1.3-macOS-ARM64/bin/clang XO_AR=$X/LLVM-23.1.3-macOS-ARM64/bin/llvm-ar XO_WINSYSROOT=$X/winsysroot
dirs=$(find "$root/internal/llvm/testdata/golden" "$root/internal/llvm/testdata/windows" -mindepth 1 -maxdepth 1 -type d | sort)
if [ $# -gt 0 ]; then
  sel=""
  for n in "$@"; do sel="$sel $(echo "$dirs" | grep "/$n\$" || true)"; done
  dirs=$sel
fi
for d in $dirs; do
  n=$(basename "$d")
  cp -R "$d" "$W/gold/$n"
  if [ -f "$d/want_windows.txt" ]; then cp "$d/want_windows.txt" "$W/gold/$n/want.txt"; fi
  (cd "$d" && "$xo" build --backend="${XO_WINE_BACKEND:-llvm}" --target=windows/amd64 -o "$W/bin/$n.exe" . > "$W/blog/$n.txt" 2>&1) || echo "build failed: $n"
done
cat > "$W/run.sh" <<'RUN'
#!/bin/sh
wineboot -u >/dev/null 2>&1
wineserver -w
mkdir -p /work/win/w /work/win/log
: > /work/win/summary.txt
for d in /work/win/gold/*; do
  n=$(basename $d)
  [ -f /work/win/bin/$n.exe ] || { echo "nobuild $n" >> /work/win/summary.txt; continue; }
  cat > /work/win/w/$n <<EOW
#!/bin/sh
wine /work/win/bin/$n.exe "\$@" > /tmp/o.\$\$
rc=\$?
grep -av 'BOX64' /tmp/o.\$\$
rm -f /tmp/o.\$\$
exit \$rc
EOW
  chmod +x /work/win/w/$n
  XO_GOLDEN_TIMEOUT=${XO_GOLDEN_TIMEOUT:-60} sh /xo/scripts/golden-run.sh /work/win/w/$n $d > /work/win/log/$n.txt 2>&1
  head -1 /work/win/log/$n.txt | grep -E "^(ok|FAIL)" >> /work/win/summary.txt || echo "FAIL $n (no result)" >> /work/win/summary.txt
  wineserver -k >/dev/null 2>&1
  wineserver -w >/dev/null 2>&1
done
echo "ok $(grep -c '^ok' /work/win/summary.txt) of $(wc -l < /work/win/summary.txt)"
RUN
"$root/scripts/docker-wine.sh" box64 "$work" "XO_GOLDEN_TIMEOUT=${XO_GOLDEN_TIMEOUT:-60} sh /work/win/run.sh" 2>&1 | grep -v '^#[0-9]' | tail -1
