#!/bin/bash
# coff-compare-arm64.sh: Xo's ARM64 COFF writer (compiler/macho
# coffa64.xo and coffseh64.xo, decision 0569 stage 3) against clang on
# the windows/arm64 assembly of every arm64 run golden and the LLVM
# goldens (or the NAMEs given, each a directory under internal/arm64/
# testdata/run or internal/llvm/testdata/golden): the objects must be
# byte identical.
#
#   scripts/coff-compare-arm64.sh XO MACHO CLANG WORK [NAME...]
#
# XO a built xo, MACHO a built compiler/macho (`xo build -o MACHO
# compiler/macho`), CLANG the clang whose objects are the reference (the
# Mac's Apple clang 21 on 2026-11-08), WORK a scratch directory; a
# differing golden's assembly and both objects stay in WORK. Prints the
# count the same and each that differs.
set -u
xo=$1; macho=$2; cc=$3; work=$4; shift 4
root=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$work"
dirs=()
if [ $# -eq 0 ]; then
  for d in "$root"/internal/arm64/testdata/run/*/ "$root"/internal/llvm/testdata/golden/*/; do dirs+=("${d%/}"); done
else
  for n in "$@"; do
    if [ -d "$root/internal/arm64/testdata/run/$n" ]; then
      dirs+=("$root/internal/arm64/testdata/run/$n")
    else
      dirs+=("$root/internal/llvm/testdata/golden/$n")
    fi
  done
fi
same=0
for d in "${dirs[@]}"; do
  n=$(basename "$d")
  if ! (cd "$d" && "$xo" asm --backend=arm64 --targets=windows/arm64 . > "$work/$n.s" 2>"$work/$n.asmerr"); then
    echo "no assembly: $n: $(head -1 "$work/$n.asmerr")"
    continue
  fi
  "$cc" --target=aarch64-pc-windows-msvc -c -o "$work/$n.ref.obj" "$work/$n.s" 2>"$work/$n.ccerr"
  if "$macho" asm --windows-arm64 -o "$work/$n.obj" "$work/$n.s" 2>"$work/$n.err" && cmp -s "$work/$n.ref.obj" "$work/$n.obj"; then
    same=$((same + 1))
    rm -f "$work/$n.s" "$work/$n.obj" "$work/$n.ref.obj" "$work/$n.err" "$work/$n.ccerr" "$work/$n.asmerr"
  else
    echo "differs: $n"
  fi
done
echo "same: $same of ${#dirs[@]}"
