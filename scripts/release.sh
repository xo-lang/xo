#!/usr/bin/env bash
# Build the release archives of xo (decisions 0157 and 0178) into dist/:
#
#   scripts/release.sh [version]
#
# Since switch point B (decision 0178, 2026-10-12) bin/xo is xo1, the self
# hosted toolchain (compiler/xo); since decision 0215 (2026-10-13) the Go
# implementation of the compiler is gone, so the first xo1 is built by an
# existing xo (the seed), and it builds the other targets' xo1 (no
# runtime packs since decision 0234: there is no C runtime):
#
#   seed         $XO_SEED, else the checkout's bin/xo made current by
#                `go run ./scripts/xobuild` (built by an earlier xo, or
#                by the xo1 of the go-compiler-final tag:
#                internal/xotool)
#   darwin/arm64 the seed: xo build --backend=arm64 compiler/xo
#   linux/arm64  the darwin xo1: --backend=arm64 --target=linux/arm64
#                --ld=macho (decision 0182 stage 3: compiler/macho's ELF
#                writer and linker)
#   linux/amd64  the darwin xo1: --backend=go --targets=linux/amd64 (no
#                native backend for amd64: compiler/gen and the Go
#                toolchain)
#   windows/amd64 (only with XO_RELEASE_WINDOWS=1, decision 0197) the
#                darwin xo1: --backend=llvm --targets=windows/amd64, split
#                modules ($XO_RELEASE_WIN_SPLIT, default 16; LTO
#                $XO_RELEASE_WIN_LTO, default thin; jobs
#                $XO_RELEASE_WIN_JOBS, default 4), compiled by an upstream
#                clang and linked by its lld-link ($XO_WIN_CLANG; Apple's
#                clang has no lld-link) against a Windows sysroot
#                ($XO_WINSYSROOT: MSVC's C library and the Windows SDK in
#                clang's /winsysroot layout, made by `xwin --accept-license
#                splat --use-winsysroot-style --preserve-ms-arch-notation`,
#                which accepts Microsoft's license for the build tools);
#                bin/xo.exe and bin/xo-gobind.exe. Not published yet: the
#                archive is built for trying.
#
# Each archive holds
#
#   xo-<version>-<os>-<arch>/bin/xo                      xo1
#   xo-<version>-<os>-<arch>/bin/xo-gobind               the Go binder helper (`use go`)
#   xo-<version>-<os>-<arch>/lib/xo/std/                 std (with llrt and rt, and
#                                                        std/tzdata: the IANA zone
#                                                        database std/time embeds,
#                                                        decision 0094's addendum)
#   xo-<version>-<os>-<arch>/lib/xo/race/                ThreadSanitizer's runtime for
#                                                        native --race and its license
#                                                        (decision 0273; darwin/arm64
#                                                        and linux/arm64, built first)
#   xo-<version>-<os>-<arch>/README.md, CHANGELOG.md
#
# xo1 finds std and xo-gobind beside itself (bin/../lib/xo), so
# bin/xo does not work copied alone (gotchas/xo1-outside-release-layout.md).
# The script then writes dist/xo-<version>-<os>-<arch>.tar.gz
# (scripts/disttar: the same bytes for the same inputs) and
# dist/SHA256SUMS. It never tags, pushes, or uploads.
#
# Compilers: the host's Apple clang (run this script on a Mac; no C
# runtime is compiled since decision 0234, so no zig for linux/arm64)
# and Go (xo-gobind, the linux/amd64 xo1's go build, and building the
# seed when there is no xo yet).
#
# ThreadSanitizer's runtime (decision 0273 part 3) is built first, from
# the pinned LLVM sources, into the checkout's lib/xo/race (git ignored):
# darwin-arm64 on the host and linux-arm64 in Docker
# (scripts/race-runtime.sh WORK <checkout> linux-arm64), so the script
# refuses to start when Docker is not running. Work files go to
# $XO_RELEASE_RACE_WORK (default $TMPDIR/xo-release-race);
# XO_RELEASE_RACE=reuse keeps a runtime lib/xo/race already has.
#
# Environment: XO_SEED (the xo that builds the first
# xo1), XO_RELEASE_ALLOW_DIRTY=1 (build from a
# modified tree, for trying the script), XO_RELEASE_WINDOWS=1 with
# XO_WIN_CLANG and XO_WINSYSROOT (the windows/amd64 archive, above),
# XO_RELEASE_DRY_RUN=1 (run the checks, then print each step's command
# instead of running it: nothing is built, written, or deleted).
set -euo pipefail

dry="${XO_RELEASE_DRY_RUN:-}"
# run CMD...: runs it, or under XO_RELEASE_DRY_RUN=1 prints it
run() {
	if [ "$dry" = 1 ]; then
		printf '+'
		printf ' %q' "$@"
		printf '\n'
	else
		"$@"
	fi
}

repo="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo"
# the version xo1 prints (compiler/cli/core VERSION)
version="${1:-v$(sed -n 's/^pub const VERSION = "\(.*\)"$/\1/p' compiler/cli/core/cli.xo)}"

if [ "$(uname -s)/$(uname -m)" != "Darwin/arm64" ]; then
	echo "release.sh: run on darwin/arm64 (the darwin xo1 is built by the host)" >&2
	exit 1
fi
if [ -n "$(git status --porcelain)" ] && [ "${XO_RELEASE_ALLOW_DIRTY:-}" != 1 ]; then
	echo "release.sh: the tree has changes; commit them (or set XO_RELEASE_ALLOW_DIRTY=1)" >&2
	exit 1
fi
if [ "${XO_RELEASE_WINDOWS:-}" = 1 ]; then
	win_clang="${XO_WIN_CLANG:-}"
	if [ -z "$win_clang" ] || ! "$win_clang" --version >/dev/null 2>&1 || [ ! -x "$(dirname "$win_clang")/lld-link" ]; then
		echo "release.sh: XO_RELEASE_WINDOWS=1 needs XO_WIN_CLANG, an upstream clang with lld-link beside it (LLVM 23.1.3 was measured)" >&2
		exit 1
	fi
	if [ ! -d "${XO_WINSYSROOT:-}/Windows Kits/10/Lib" ]; then
		echo "release.sh: XO_RELEASE_WINDOWS=1 needs XO_WINSYSROOT, a Windows sysroot (xwin --accept-license splat --use-winsysroot-style --preserve-ms-arch-notation --output DIR)" >&2
		exit 1
	fi
fi
# the linux/arm64 race runtime builds in Docker (below)
race_reuse() { [ "${XO_RELEASE_RACE:-}" = reuse ] && [ -d "lib/xo/race/$1" ] && [ -f lib/xo/race/LICENSE.TXT ]; }
if ! race_reuse linux-arm64 && ! docker info >/dev/null 2>&1; then
	echo "release.sh: Docker is not running: the linux/arm64 race runtime (scripts/race-runtime.sh ... linux-arm64) builds in it; start Docker, or set XO_RELEASE_RACE=reuse with lib/xo/race/linux-arm64 built" >&2
	exit 1
fi

dist="$repo/dist"
run rm -rf "$dist"
run mkdir -p "$dist"
unset XO_STD XO_RTPACK XO_CLI
# the seed: an existing xo (decision 0215: no Go compiler to build one)
if [ -n "${XO_SEED:-}" ]; then
	host_xo="$XO_SEED"
elif [ "$dry" = 1 ]; then
	run go run ./scripts/xobuild
	host_xo='$(go run ./scripts/xobuild)'
else
	host_xo="$(go run ./scripts/xobuild)"
fi

# ThreadSanitizer's runtime for native --race (decision 0273 part 3) into
# the checkout's lib/xo/race, which layout copies: the host's dylib and
# linux/arm64's static archive (built in Docker)
race_work="${XO_RELEASE_RACE_WORK:-${TMPDIR:-/tmp}/xo-release-race}"
for plat in darwin-arm64 linux-arm64; do
	if race_reuse "$plat"; then
		echo "== race runtime $plat: kept lib/xo/race/$plat (XO_RELEASE_RACE=reuse)" >&2
		continue
	fi
	echo "== race runtime $plat: scripts/race-runtime.sh (LLVM compiler-rt, several minutes)" >&2
	if [ "$plat" = darwin-arm64 ]; then
		run bash scripts/race-runtime.sh "$race_work/$plat" "$repo" >&2
	else
		run bash scripts/race-runtime.sh "$race_work/$plat" "$repo" "$plat" >&2
	fi
done

stage_of() { echo "$dist/xo-$version-${1%/*}-${1#*/}"; }

# the layout around bin/xo: lib/xo (the tracked files of std),
# bin/xo-gobind, the docs
layout() {
	local target=$1 stage
	stage=$(stage_of "$target")
	if [ "$dry" = 1 ]; then
		echo "+ layout $stage: lib/xo/std (git ls-files std), bin/xo-gobind (go build ./cmd/xo-gobind for $target), README.md, CHANGELOG.md, lib/xo/race/${target%/*}-${target#*/} when built" >&2
		return
	fi
	mkdir -p "$stage/bin" "$stage/lib/xo"
	git ls-files -z std | rsync -a --from0 --files-from=- ./ "$stage/lib/xo/"
	# the zone database std/time embeds is std/tzdata/zoneinfo.zip, a
	# tracked file of std (decision 0094's addendum), copied above
	local exe=""
	[ "${target%/*}" = windows ] && exe=.exe
	GOOS="${target%/*}" GOARCH="${target#*/}" CGO_ENABLED=0 \
		go build -trimpath -o "$stage/bin/xo-gobind$exe" ./cmd/xo-gobind
	cp README.md CHANGELOG.md "$stage/"
	# ThreadSanitizer's runtime for native --race (decision 0273 part 3),
	# when scripts/race-runtime.sh built it for this target into the
	# checkout's lib/xo/race, with its license
	local plat="${target%/*}-${target#*/}"
	if [ -d "lib/xo/race/$plat" ]; then
		mkdir -p "$stage/lib/xo/race"
		cp -R "lib/xo/race/$plat" "$stage/lib/xo/race/"
		cp lib/xo/race/LICENSE.TXT "$stage/lib/xo/race/"
	fi
}

targets="darwin/arm64 linux/arm64 linux/amd64"
[ "${XO_RELEASE_WINDOWS:-}" = 1 ] && targets="$targets windows/amd64"
for target in $targets; do
	layout "$target"
done
darwin=$(stage_of darwin/arm64)
linux_arm64=$(stage_of linux/arm64)
linux_amd64=$(stage_of linux/amd64)

echo "== darwin/arm64: the seed builds xo1" >&2
run "$host_xo" build --backend=arm64 --std="$repo/std" -o "$darwin/bin/xo" compiler/xo >&2
xo1="$darwin/bin/xo"
run "$xo1" version >&2
# no runtime packs since decision 0234: a program has no C runtime

echo "== linux/arm64: xo1 cross builds xo1 (compiler/macho's ELF link, no cc)" >&2
run "$xo1" build --backend=arm64 \
	--target=linux/arm64 --ld=macho -o "$linux_arm64/bin/xo" compiler/xo >&2

echo "== linux/amd64: xo1 builds xo1 with the Go backend" >&2
run "$xo1" build --backend=go --targets=linux/amd64 -o "$linux_amd64/bin/xo" compiler/xo >&2

if [ "${XO_RELEASE_WINDOWS:-}" = 1 ]; then
	echo "== windows/amd64: xo1 cross builds xo1 with the LLVM backend" >&2
	# bench/results/windows-2026-10-12.md, "xo1 built by Xo for Windows"
	XO_CLANG="$win_clang" XO_AR="$(dirname "$win_clang")/llvm-ar" XO_WINSYSROOT="$XO_WINSYSROOT" \
		XO_LLVM_SPLIT="${XO_RELEASE_WIN_SPLIT:-16}" XO_LLVM_LTO="${XO_RELEASE_WIN_LTO:-thin}" \
		XO_LLVM_JOBS="${XO_RELEASE_WIN_JOBS:-4}" XO_LLVM_OPT=O2 \
		run "$xo1" build --backend=llvm --targets=windows/amd64 \
		-o "$(stage_of windows/amd64)/bin/xo.exe" compiler/xo >&2
fi

for target in $targets; do
	stage=$(stage_of "$target")
	run go run ./scripts/disttar -o "$stage.tar.gz" "$stage"
done
if [ "$dry" = 1 ]; then
	echo "+ SHA256SUMS of $dist/*.tar.gz" >&2
	exit 0
fi
(cd "$dist" && shasum -a 256 ./*.tar.gz | sed 's| \./| |' >SHA256SUMS)
cat "$dist/SHA256SUMS"
