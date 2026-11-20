#!/bin/bash
# bisect-pass-self.sh [FLAG...] DIR: `xo dev bisect-pass --self=TOP` for
# this tree (task #459): bisect the optional passes of xo's own build when
# an xo built natively compiles DIR wrong. Each step builds this tree's
# compiler/xo with a pass prefix and has that xo build and run DIR (a
# golden directory, relative to the tree's top). The xo that runs the
# bisect is $XO_CLI (else bin/xo); --backend defaults to this machine's
# (amd64 on x86_64, else arm64). Each build and run is saved under
# ~/xo-work/bisect-self/COMMIT-DIRNAME (BISECT_OUT overrides), so a run
# its lease's time limit stopped resumes where it was when started again;
# a finished run removes it. On the linux/amd64 host, from the Mac:
#   amd64-run.sh REV NAME script scripts/bisect-pass-self.sh --ref=want internal/llvm/testdata/golden/native_json
set -u
top=$(cd "$(dirname "$0")/.." && pwd)
cd "$top" || exit 2
xo=${XO_CLI:-$top/bin/xo}
be=arm64
[ "$(uname -m)" = x86_64 ] && be=amd64
case " $* " in *" --backend="*) be="" ;; esac
dir=${!#}
out=${BISECT_OUT:-$HOME/xo-work/bisect-self/$(git -C "$top" rev-parse --short=12 HEAD)-$(basename "$dir")}
mkdir -p "$out" || exit 2
echo "bisect-pass-self: saving to $out ($(ls "$out/tried" 2>/dev/null | wc -l | tr -d ' ') attempts saved before)"
exec "$xo" dev bisect-pass --self="$top" --out="$out" ${be:+--backend=$be} "$@"
