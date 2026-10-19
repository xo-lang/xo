#!/bin/bash
# coff-compare.sh: Xo's COFF writer (compiler/macho coff.xo, decision 0254
# 6b step 3) against llvm-mc on the windows/amd64 assembly of every golden
# (or the NAMEs given): the objects must be byte identical.
#
#   scripts/coff-compare.sh XO MACHO LLVM_MC WORK [NAME...]
#
# XO a built xo, MACHO a built compiler/macho (`xo build -o MACHO
# compiler/macho`), LLVM_MC winxc's (~/Library/Caches/xo/winxc/LLVM-*/bin/
# llvm-mc), WORK a scratch directory; a differing golden's assembly and
# both objects stay in WORK. Prints the count the same and each that
# differs.
set -u
xo=$1; macho=$2; mc=$3; work=$4; shift 4
root=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$work"
names=("$@")
if [ ${#names[@]} -eq 0 ]; then
  for d in "$root"/internal/llvm/testdata/golden/*/; do names+=("$(basename "$d")"); done
fi
same=0
for n in "${names[@]}"; do
  d="$root/internal/llvm/testdata/golden/$n"
  (cd "$d" && "$xo" asm --backend=amd64 --target=windows/amd64 . > "$work/$n.s" 2>/dev/null)
  "$mc" -triple=x86_64-pc-windows-msvc -filetype=obj -o "$work/$n.ref.obj" "$work/$n.s" 2>"$work/$n.mcerr"
  if "$macho" asm --windows-amd64 -o "$work/$n.obj" "$work/$n.s" 2>"$work/$n.err" && cmp -s "$work/$n.ref.obj" "$work/$n.obj"; then
    same=$((same + 1))
    rm -f "$work/$n.s" "$work/$n.obj" "$work/$n.ref.obj" "$work/$n.err" "$work/$n.mcerr"
  else
    echo "differs: $n"
  fi
done
echo "same: $same of ${#names[@]}"
