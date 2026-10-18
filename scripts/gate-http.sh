#!/bin/sh
# gate-http.sh OUTDIR [STEP...]: the gate for a change to std/native, the
# runtime, HTTP, TLS, or the compiler, run from the checkout to gate
# (Xo Lang 3's gates, 2026-10-18). Each step writes OUTDIR/STEP.log and a
# line STEP=STATUS to OUTDIR/gate.log; a failed step does not stop the
# others, but a failed build stops the gate: every later step would run
# the bin/xo of an earlier build (gotchas/gate-stale-xo-after-failed-build.md). Run it in a heavy slot (scripts/lead/heavy.sh NAME
# scripts/gate-http.sh OUTDIR).
#
# Steps (default: all, in this order):
#   build    go run ./scripts/xobuild (bin/xo of this checkout; skipped
#            when XO_CLI is set)
#   short    go test -short over the repository (runs/ excluded)
#   llvm     internal/llvm TestGolden
#   arm64    internal/arm64 TestGoldenRun
#   tls      the TLS, crypto, and HTTP/1 diff tests and the fuzz corpora
#   h2diff   internal/llvm TestH2Diff, alone (it is load sensitive:
#            gotchas/h2diff-go-reference-streams-under-load.md)
#   codegen  xo dev codegen test/codegen/consttime (decision 0222)
#   fp       scripts/bootstrap-xo.sh: s2 and s3 identical; the stage
#            outputs (about 3 GB) are deleted after
#
# The seed for fp is $XO_SEED, else the newest seed unpacked under
# <user cache>/xo/seeds. A step needing more time takes
# GATE_TIMEOUT (default 90m).
set -u
out=${1:?usage: gate-http.sh OUTDIR [STEP...]}
shift
steps=${*:-build short llvm arm64 tls h2diff codegen fp}
mkdir -p "$out" || exit 1
out=$(cd "$out" && pwd)
cd "$(dirname "$0")/.." || exit 1
log=$out/gate.log
to=${GATE_TIMEOUT:-90m}
echo "$(git log --oneline -1 | cut -c1-12) $(date '+%Y-%m-%d %H:%M')" >> "$log"
: "${XO_CLI:=}"
if [ -z "$XO_CLI" ]; then
	XO_CLI=$PWD/bin/xo
	bootstrap_cli=1
else
	bootstrap_cli=0
fi
export XO_CLI
run() {
	name=$1
	shift
	"$@" > "$out/$name.log" 2>&1
	echo "$name=$?" >> "$log"
}
for s in $steps; do
	case $s in
	build)
		if [ "$bootstrap_cli" = 1 ]; then
			run build go run ./scripts/xobuild
			if ! tail -1 "$log" | grep -q '^build=0$'; then
				echo "gate=stopped (the build failed; bin/xo is an earlier build's)" >> "$log"
				exit 1
			fi
		fi ;;
	short)
		run short go test -short -count=1 $(go list ./... | grep -v /runs/) ;;
	llvm)
		run llvm go test -count=1 -timeout "$to" -run 'TestGolden$' ./internal/llvm ;;
	arm64)
		run arm64 go test -count=1 -timeout "$to" -run 'TestGoldenRun' ./internal/arm64 ;;
	tls)
		run tls go test -count=1 -timeout "$to" -run 'TestTLSServerReplayDiff$|TestTLSInteropMatrix$|TestNistecDiff$|TestX25519Diff$|TestConstantTimeLLVM$|TestCryptoXoDiff$|TestMLKEMAccumulated$|TestHTTPClientRedirectNotUTF8$|FuzzHTTPDiff$|TestHTTP1ClientDiff$|TestHTTPParseDiff$' ./internal/llvm ;;
	h2diff)
		run h2diff go test -count=1 -timeout "$to" -run 'TestH2Diff$' ./internal/llvm ;;
	codegen)
		run codegen "$XO_CLI" dev codegen test/codegen/consttime ;;
	fp)
		seed=${XO_SEED:-$(command ls -d "$HOME"/Library/Caches/xo/seeds/*/*/tree/xo/bin/xo "${XDG_CACHE_HOME:-$HOME/.cache}"/xo/seeds/*/*/tree/xo/bin/xo 2>/dev/null | sort -V | tail -1)}
		if [ -z "$seed" ]; then
			echo "fp=no-seed" >> "$log"
			continue
		fi
		run fp sh scripts/bootstrap-xo.sh "$seed" "$out/fp"
		grep "fixed point" "$out/fp.log" >> "$log"
		rm -rf "${out:?}/fp" ;;
	*)
		echo "$s=unknown-step" >> "$log" ;;
	esac
done
echo "done $(date '+%H:%M')" >> "$log"
