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
#   darwin/amd64 the darwin xo1: --backend=amd64 --target=darwin/amd64
#                (decision 0254 stage 6a; runs on an Intel Mac only)
#   linux/arm64  the darwin xo1: --backend=arm64 --target=linux/arm64
#                --ld=macho (decision 0182 stage 3: compiler/macho's ELF
#                writer and linker)
#   linux/amd64  the darwin xo1: --backend=go --targets=linux/amd64 (no
#                native backend for amd64: compiler/gen and the Go
#                toolchain)
#   windows/amd64 the darwin xo1: --backend=amd64 --targets=windows/amd64,
#                linked by Xo's PE linker with no C runtime (decision 0254
#                6b step 4; owner 2026-10-22: no Microsoft SDK); with
#                XO_RELEASE_WINDOWS=llvm instead (decision 0197) the
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
#   xo-<version>-<os>-<arch>/README.md, CHANGELOG.md, LICENSE,
#                                                        THIRD_PARTY_NOTICES (the binaries
#                                                        carry Go's and musl's code, whose
#                                                        licenses ask for their notices)
#
# xo1 finds std and xo-gobind beside itself (bin/../lib/xo), so
# bin/xo does not work copied alone (gotchas/xo1-outside-release-layout.md).
# Every xo1 is stamped (decision 0264, `xo version -v` and `--json`)
# with the commit it was built from, so a release is built from the
# published tree (../xo-publish after publish.sh prepare, decision 0249):
# its commits are the public ones, and its std and docs are the scrubbed
# ones. A private checkout (it has scripts/lead) is refused unless
# XO_RELEASE_PRIVATE=1 (trials; its archives name a private commit).
#
# The darwin executables (bin/xo, bin/xo-gobind, the race dylib) are
# signed with the Developer ID whose identity (a SHA-1 hash or the full
# name) is in $XO_SIGN_ID, never in this file, and notarized in one
# submission: with the keychain profile $XO_NOTARY_PROFILE (made once by
# the owner: xcrun notarytool store-credentials NAME), else the App Store
# Connect key in APPLE_API_KEY, APPLE_API_KEY_ID, APPLE_API_ISSUER
# (scripts/sign-macos.sh, code-signing.md). A bare Mach-O and a tar.gz
# cannot carry a stapled ticket: Gatekeeper looks the notarization up
# online on first run. XO_RELEASE_UNSIGNED=1 skips signing (trials), and
# XO_RELEASE_NO_NOTARIZE=1 signs without notarizing.
#
# The windows executables (bin/xo.exe, bin/xo-gobind.exe) are signed
# with Azure Artifact Signing (scripts/sign-windows.sh, code-signing.md
# "Windows"; each signature is billed): the six AZURE_* variables come
# from the environment, or from the mode 600 file $XO_AZURE_ENV, read in
# the subshell that signs and nowhere else; JSIGN_JAR names jsign's jar
# and JAVA a Java runtime. The credentials are checked before the build
# (sign-windows.sh --check-auth, a token only). XO_RELEASE_UNSIGNED=1
# skips this too; XO_RELEASE_WIN_UNSIGNED=1 skips only Windows signing.
#
# The script then writes dist/xo-<version>-<os>-<arch>.tar.gz
# (scripts/disttar: the same bytes for the same inputs) and
# dist/SHA256SUMS. It never tags, pushes, or uploads
# (scripts/release-publish.sh, the owner's, uploads).
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
# modified tree, for trying the script), XO_RELEASE_WINDOWS=0 (no
# windows/amd64 archive) or llvm with XO_WIN_CLANG and XO_WINSYSROOT
# (above),
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
if [ -d scripts/lead ] && [ "${XO_RELEASE_PRIVATE:-}" != 1 ]; then
	echo "release.sh: this is the private checkout; build from the published tree (publish.sh prepare, then scripts/release.sh in ../xo-publish) so xo version names a public commit, or set XO_RELEASE_PRIVATE=1 for a trial" >&2
	exit 1
fi
sign=1
if [ "${XO_RELEASE_UNSIGNED:-}" = 1 ]; then
	sign=0
elif [ -z "${XO_SIGN_ID:-}" ]; then
	echo "release.sh: set XO_SIGN_ID to the Developer ID Application identity (its SHA-1 from security find-identity -v -p codesigning), or XO_RELEASE_UNSIGNED=1 for a trial" >&2
	exit 1
elif [ "${XO_RELEASE_NO_NOTARIZE:-}" != 1 ] && [ -z "${XO_NOTARY_PROFILE:-}" ] && [ -z "${APPLE_API_KEY:-}" ]; then
	echo "release.sh: notarizing needs XO_NOTARY_PROFILE (xcrun notarytool store-credentials NAME, the owner) or APPLE_API_KEY, APPLE_API_KEY_ID, APPLE_API_ISSUER; XO_RELEASE_NO_NOTARIZE=1 signs only" >&2
	exit 1
fi
win=${XO_RELEASE_WINDOWS:-native}
[ "$win" = 1 ] && win=native
case $win in native|llvm|0) ;; *) echo "release.sh: XO_RELEASE_WINDOWS is native (the default), llvm, or 0" >&2; exit 1 ;; esac
# win_sign ARGS...: scripts/sign-windows.sh with the Azure credentials of
# the environment or $XO_AZURE_ENV, which only this subshell reads
win_sign() {
	(
		if [ -n "${XO_AZURE_ENV:-}" ]; then
			set -a
			# shellcheck disable=SC1090
			. "$XO_AZURE_ENV" || exit 2
			set +a
		fi
		bash scripts/sign-windows.sh "$@"
	)
}
winsign=0
if [ "$win" != 0 ] && [ "$sign" = 1 ] && [ "${XO_RELEASE_WIN_UNSIGNED:-}" != 1 ]; then
	winsign=1
	if ! win_sign --check-auth >&2; then
		echo "release.sh: Windows signing needs working Azure Artifact Signing credentials: the six AZURE_* variables (or XO_AZURE_ENV, a mode 600 file of them), JSIGN_JAR and JAVA (code-signing.md \"Windows\"); XO_RELEASE_WIN_UNSIGNED=1 builds unsigned Windows executables" >&2
		exit 1
	fi
fi
if [ "$win" = llvm ]; then
	win_clang="${XO_WIN_CLANG:-}"
	if [ -z "$win_clang" ] || ! "$win_clang" --version >/dev/null 2>&1 || [ ! -x "$(dirname "$win_clang")/lld-link" ]; then
		echo "release.sh: XO_RELEASE_WINDOWS=llvm needs XO_WIN_CLANG, an upstream clang with lld-link beside it (LLVM 23.1.3 was measured)" >&2
		exit 1
	fi
	if [ ! -d "${XO_WINSYSROOT:-}/Windows Kits/10/Lib" ]; then
		echo "release.sh: XO_RELEASE_WINDOWS=llvm needs XO_WINSYSROOT, a Windows sysroot (xwin --accept-license splat --use-winsysroot-style --preserve-ms-arch-notation --output DIR)" >&2
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
# a release's inlined module must verify; no silent slower build
# (decision 0342)
export XO_INLINE_STRICT=1
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
race_plats="darwin-arm64 darwin-amd64 linux-arm64"
[ "$win" != 0 ] && race_plats="$race_plats windows-amd64"
for plat in $race_plats; do
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
		echo "+ layout $stage: lib/xo/std (git ls-files std), bin/xo-gobind (go build ./cmd/xo-gobind for $target), README.md, CHANGELOG.md, LICENSE, THIRD_PARTY_NOTICES, lib/xo/race/${target%/*}-${target#*/} when built" >&2
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
	cp README.md CHANGELOG.md LICENSE THIRD_PARTY_NOTICES "$stage/"
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

targets="darwin/arm64 darwin/amd64 linux/arm64 linux/amd64"
[ "$win" != 0 ] && targets="$targets windows/amd64"
for target in $targets; do
	layout "$target"
done
darwin=$(stage_of darwin/arm64)
darwin_amd64=$(stage_of darwin/amd64)
linux_arm64=$(stage_of linux/arm64)
linux_amd64=$(stage_of linux/amd64)

# the provenance stamps (decision 0264): this tree's commit, source key,
# and code generation key, the same for every target
stamp=()
if [ "$dry" = 1 ]; then
	stamp=(--stamp 'XO_COMMIT=...' --stamp 'XO_SOURCE_KEY=...')
else
	while IFS= read -r a; do stamp+=("$a"); done < <(go run ./scripts/xobuild --stamp-args="$host_xo")
	[ ${#stamp[@]} -gt 0 ] || { echo "release.sh: no provenance stamp (go run ./scripts/xobuild --stamp-args)" >&2; exit 1; }
fi

echo "== darwin/arm64: the seed builds xo1" >&2
run "$host_xo" build --backend=arm64 --std="$repo/std" "${stamp[@]}" -o "$darwin/bin/xo" compiler/xo >&2
xo1="$darwin/bin/xo"
run "$xo1" version >&2
# no runtime packs since decision 0234: a program has no C runtime

echo "== linux/arm64: xo1 cross builds xo1 (compiler/macho's ELF link, no cc)" >&2
run "$xo1" build --backend=arm64 "${stamp[@]}" \
	--target=linux/arm64 --ld=macho -o "$linux_arm64/bin/xo" compiler/xo >&2

echo "== darwin/amd64: xo1 cross builds xo1 with the amd64 backend" >&2
run "$xo1" build --backend=amd64 "${stamp[@]}" \
	--target=darwin/amd64 -o "$darwin_amd64/bin/xo" compiler/xo >&2

echo "== linux/amd64: xo1 builds xo1 with the Go backend" >&2
run "$xo1" build --backend=go "${stamp[@]}" --targets=linux/amd64 -o "$linux_amd64/bin/xo" compiler/xo >&2

if [ "$win" = native ]; then
	echo "== windows/amd64: xo1 cross builds xo1 with the amd64 backend and Xo's PE linker" >&2
	run "$xo1" build --backend=amd64 "${stamp[@]}" --targets=windows/amd64 \
		-o "$(stage_of windows/amd64)/bin/xo.exe" compiler/xo >&2
elif [ "$win" = llvm ]; then
	echo "== windows/amd64: xo1 cross builds xo1 with the LLVM backend" >&2
	# bench/results/windows-2026-10-12.md, "xo1 built by Xo for Windows"
	XO_CLANG="$win_clang" XO_AR="$(dirname "$win_clang")/llvm-ar" XO_WINSYSROOT="$XO_WINSYSROOT" \
		XO_LLVM_SPLIT="${XO_RELEASE_WIN_SPLIT:-16}" XO_LLVM_LTO="${XO_RELEASE_WIN_LTO:-thin}" \
		XO_LLVM_JOBS="${XO_RELEASE_WIN_JOBS:-4}" XO_LLVM_OPT=O2 \
		run "$xo1" build --backend=llvm "${stamp[@]}" --targets=windows/amd64 \
		-o "$(stage_of windows/amd64)/bin/xo.exe" compiler/xo >&2
fi

# sign the darwin executables with the Developer ID and notarize them in
# one submission, before they are archived
if [ "$sign" = 1 ]; then
	signed=()
	for st in "$darwin" "$darwin_amd64"; do
		signed+=("$st/bin/xo" "$st/bin/xo-gobind")
		for d in "$st"/lib/xo/race/darwin-*/*.dylib; do
			[ -e "$d" ] && signed+=("$d")
		done
	done
	notarize=--notarize
	[ "${XO_RELEASE_NO_NOTARIZE:-}" = 1 ] && notarize=
	echo "== darwin: sign ${#signed[@]} files${notarize:+ and notarize them}" >&2
	XO_SIGN_IDENTITY="$XO_SIGN_ID" run bash scripts/sign-macos.sh $notarize "${signed[@]}" >&2
fi
# sign the windows executables (Authenticode, RFC 3161 timestamp), before
# they are archived; sign-windows.sh checks each signature it made
if [ "$winsign" = 1 ]; then
	wst=$(stage_of windows/amd64)
	echo "== windows: sign bin/xo.exe and bin/xo-gobind.exe (Azure Artifact Signing)" >&2
	run win_sign "$wst/bin/xo.exe" "$wst/bin/xo-gobind.exe" >&2
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
