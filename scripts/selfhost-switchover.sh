#!/bin/bash
# The switch-over experiment of decision 0158: the self hosted compiler
# driver (selfhost/xo: lexer through arm64 emitter in Xo) built by the Go
# compiler with the arm64 backend (A) compiles itself (B), B compiles it
# again (C); the assembly of A, B, and C's builds is compared with the Go
# emitter's (xo asmdump), and B runs the arm64 goldens
# (scripts/selfhost-goldens.py). Writes into DIR (default .tools/sw) and
# its log to DIR/log.txt. XO: the Go `xo` binary (default .tools/xo, built
# from ./cmd/xo first).
set -u
cd "$(dirname "$0")/.."
DIR=${1:-.tools/sw}
XO=${XO:-.tools/xo}
mkdir -p "$DIR"
export GOMAXPROCS=${GOMAXPROCS:-4}
L=$DIR/log.txt
: > "$L"
t() { /usr/bin/time -p "$@" >> "$L" 2>&1; echo "exit $? : $*" >> "$L"; }
if [ "$XO" = .tools/xo ]; then
	mkdir -p .tools && go build -o .tools/xo ./cmd/xo
fi
echo "== A: the Go compiler, arm64 backend" >> "$L"
XO_SELF_DRIVER=0 t "$XO" build --backend=arm64 -o "$DIR/A" selfhost/xo
echo "== G: the Go emitter's assembly of selfhost/xo (line root selfhost)" >> "$L"
(cd selfhost && /usr/bin/time -p "../$XO" asmdump xo) > "$DIR/G.s" 2>> "$L"
echo "== B: A compiles selfhost/xo" >> "$L"
t "$DIR/A" build --backend=arm64 -o "$DIR/B" --asm="$DIR/B.s" selfhost/xo
echo "== C: B compiles selfhost/xo" >> "$L"
t "$DIR/B" build --backend=arm64 -o "$DIR/C" --asm="$DIR/C.s" selfhost/xo
echo "== compare" >> "$L"
cmp "$DIR/G.s" "$DIR/B.s" >> "$L" 2>&1 && echo "G.s == B.s" >> "$L"
cmp "$DIR/B.s" "$DIR/C.s" >> "$L" 2>&1 && echo "B.s == C.s" >> "$L"
cmp "$DIR/B" "$DIR/C" >> "$L" 2>&1 && echo "B == C (binaries)" >> "$L"
shasum -a 256 "$DIR/A" "$DIR/B" "$DIR/C" "$DIR/G.s" "$DIR/B.s" "$DIR/C.s" >> "$L"
wc -l "$DIR/B.s" >> "$L"
echo "== goldens with B" >> "$L"
XO_GO_XO="$XO" XO_GOLD_DIR="$DIR/gold" SHOW=6 python3 scripts/selfhost-goldens.py "$DIR/B" >> "$L" 2>&1
echo "switch done" >> "$L"
