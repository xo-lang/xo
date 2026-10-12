#!/bin/sh
# go-version-check.sh: run the Go backend's version sensitive tests with
# another Go toolchain (decision 0176, toolchain.md "Go versions").
#
#   GOTOOLCHAIN=go1.27rc1 scripts/go-version-check.sh [--download] [--keep] [-n] [step...]
#
# GOTOOLCHAIN names the Go to test (default: the go command on PATH,
# GOTOOLCHAIN=local). Nothing is downloaded unless --download is given:
# without it GOPROXY=off, so the go command uses a toolchain that is
# already in the module cache (golang.org/toolchain@v0.0.1-goX.Y.Z...)
# and fails with "toolchain not available" otherwise.
#
# Steps (default: all, in this order):
#   version  the go command's version and GOROOT
#   unit     go test -short over the repo (runs/ excluded)
#   rt-min   go vet -stdversion of std/rt with a go.mod at gotool.MinGo, for
#            darwin, linux, windows, and wasip1 and every rt build tag
#   gen-min  TestGeneratedGoVets with XO_GO_LINE=MinGo: the generated Go of
#            every golden program vets (language and std API) at MinGo
#   goldens  the Go backend goldens (TestGolden, TestGoldenNoMono,
#            TestGeneratedGoVets) and the Go timers link (runtime linkname)
#            at the toolchain's own language version (XO_GO_LINE)
#   usego    `use go`: TestGoBindingSignatures, TestReplayGoCalls,
#            TestReplayGoCallback, internal/gobind, cmd/xo TestBind, and
#            the checker's TestGoInterop, with bindings regenerated from
#            scratch (a fresh XO_GOBIND_DIR)
#
# Builds use a Go cache, an Xo Go cache, and a binding cache under a
# temporary directory that is removed at the end (--keep keeps it), so
# a new toolchain does not grow the user's caches. -n prints the
# commands without running them. The goldens, gen-min, and usego steps
# are heavy (minutes; run one at a time on a shared machine).
set -u

download=0 keep=0 dry=0 steps=""
for a in "$@"; do
	case "$a" in
	--download) download=1 ;;
	--keep) keep=1 ;;
	-n) dry=1 ;;
	-h | --help)
		sed -n '2,33p' "$0"
		exit 0
		;;
	-*)
		echo "unknown flag $a" >&2
		exit 2
		;;
	*) steps="$steps $a" ;;
	esac
done
[ -n "$steps" ] || steps="version unit rt-min gen-min goldens usego"

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root" || exit 1
: "${GOTOOLCHAIN:=local}"
export GOTOOLCHAIN
[ "$download" = 1 ] || export GOPROXY=off
work=$(mktemp -d "${TMPDIR:-/tmp}/xo-go-version-check.XXXXXX") || exit 1
if [ "$keep" = 0 ]; then
	trap 'rm -rf "$work"' EXIT
fi
export GOCACHE="$work/go-build" XO_GOCACHE="$work/xo-gocache" XO_GOBIND_DIR="$work/gobind"
export GOWORK=off GOMAXPROCS=${GOMAXPROCS:-4}

min=$(sed -n 's/^const MinGo = "\(.*\)"$/\1/p' internal/gotool/gotool.go)
fail=0

run() {
	echo "+ $*"
	[ "$dry" = 1 ] && return 0
	"$@" || {
		echo "FAIL: $*" >&2
		fail=1
	}
}

# lang is the toolchain's language version (1.27 for go1.27rc1).
lang() {
	go env GOVERSION | sed -n 's/^\(devel \)\{0,1\}go\([0-9]*\.[0-9]*\).*/\2/p'
}

rtmin() {
	m="$work/rtmin"
	mkdir -p "$m/rt"
	for f in std/rt/*.go std/rt/*.s; do
		case "$f" in *_test.go) ;; *) cp "$f" "$m/rt/" ;; esac
	done
	printf 'module xortmin\n\ngo %s\n' "$min" >"$m/go.mod"
	for t in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 wasip1/wasm; do
		for tags in "" xo_gotimers xo_pure xo_nohttp; do
			(cd "$m" && CGO_ENABLED=0 GOOS=${t%/*} GOARCH=${t#*/} go vet -stdversion -tags="$tags" ./rt) ||
				{
					echo "FAIL: rt at go $min, $t, tags '$tags'" >&2
					fail=1
				}
		done
	done
}

for s in $steps; do
	echo "== $s (GOTOOLCHAIN=$GOTOOLCHAIN)"
	case "$s" in
	version)
		run go version
		run go env GOVERSION GOROOT
		;;
	unit)
		pkgs=$(go list ./... | grep -v /runs/)
		run go test -short -count=1 -p 2 -parallel 2 $pkgs
		;;
	rt-min)
		if [ "$dry" = 1 ]; then echo "+ go vet -stdversion std/rt at go $min for each target and tag"; else rtmin; fi
		;;
	gen-min)
		run env XO_GO_LINE="$min" go test -count=1 -timeout 60m ./internal/gen -run 'TestGeneratedGoVets$'
		;;
	goldens)
		l=$(lang)
		run env XO_GO_LINE="$l" go test -count=1 -timeout 90m ./internal/gen -run 'TestGolden$|TestGoldenNoMono$|TestGeneratedGoVets$'
		run env XO_GO_LINE="$l" go test -count=1 ./internal/build -run 'GoTimers|TestGoTooOld|TestGoCommandsUseXoCache'
		;;
	usego)
		l=$(lang)
		run env XO_GO_LINE="$l" go test -count=1 -timeout 60m ./internal/gen -run 'TestGoBindingSignatures|TestReplayGoCalls|TestReplayGoCallback'
		run env XO_GO_LINE="$l" go test -count=1 ./internal/gobind ./internal/gotool
		run env XO_GO_LINE="$l" go test -count=1 ./internal/check -run 'TestGoInterop'
		run env XO_GO_LINE="$l" go test -count=1 ./cmd/xo -run 'TestBind$'
		;;
	*)
		echo "unknown step $s" >&2
		fail=1
		;;
	esac
done
[ "$keep" = 1 ] && echo "caches kept in $work"
if [ "$fail" = 0 ]; then echo "PASS"; else echo "FAIL"; fi
exit $fail
