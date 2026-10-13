#!/usr/bin/env bash
# Build the release archives of xo (decisions 0157 and 0178) into dist/:
#
#   scripts/release.sh [version]
#
# Since switch point B (decision 0178, 2026-10-12) bin/xo is xo1, the self
# hosted toolchain (selfhost/xo), not the Go cmd/xo. The Go cmd/xo is only
# the bootstrap seed: it is built for the host and builds the first xo1,
# which then builds the runtime packs and the other targets' xo1:
#
#   seed         go build ./cmd/xo (dist/.host/xo, deleted at the end)
#   darwin/arm64 the seed: xo build --backend=arm64 selfhost/xo (the
#                build the switch point B gate tested)
#   linux/arm64  the darwin xo1: --backend=arm64 --target=linux/arm64
#                --ld=macho (decision 0182 stage 3: selfhost/macho's ELF
#                writer and linker, with the linux pack built just before)
#   linux/amd64  the darwin xo1: --backend=go --targets=linux/amd64 (no
#                native backend for amd64: selfhost/gen and the Go
#                toolchain)
#
# Each archive holds
#
#   xo-<version>-<os>-<arch>/bin/xo                      xo1
#   xo-<version>-<os>-<arch>/bin/xo-gobind               the Go binder helper (`use go`)
#   xo-<version>-<os>-<arch>/lib/xo/std/                 std (with llrt and rt)
#   xo-<version>-<os>-<arch>/lib/xo/third_party/         the TLS library's sources
#   xo-<version>-<os>-<arch>/lib/xo/rtpack/<os>-<arch>/  the prebuilt C runtime (arm64 targets)
#   xo-<version>-<os>-<arch>/lib/xo/zoneinfo.zip         Go's zone database, for std/time
#                                                        (decision 0157 next step 1: no Go needed)
#   xo-<version>-<os>-<arch>/README.md, CHANGELOG.md
#
# xo1 finds std, the pack, and xo-gobind beside itself (bin/../lib/xo), so
# bin/xo does not work copied alone (gotchas/xo1-outside-release-layout.md).
# The script then writes dist/xo-<version>-<os>-<arch>.tar.gz
# (scripts/disttar: the same bytes for the same inputs) and
# dist/SHA256SUMS. It never tags, pushes, or uploads.
#
# Compilers (both pinned, the pack manifests record them):
#   darwin/arm64  the host's Apple clang (run this script on a Mac)
#   linux/arm64   zig cc 0.15.1 with glibc 2.31 headers: $ZIG, else zig on PATH
#                 (https://ziglang.org/download/, check the tarball's SHA-256
#                 against https://ziglang.org/download/index.json)
# and Go (the seed, xo-gobind, and the linux/amd64 xo1's go build).
#
# Environment: ZIG (zig binary), XO_RELEASE_ALLOW_DIRTY=1 (build from a
# modified tree, for trying the script).
set -euo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo"
# the version xo1 prints (selfhost/cli/core VERSION; TestSelfhostVersion
# keeps cmd/xo's equal to it)
version="${1:-v$(sed -n 's/^pub const VERSION = "\(.*\)"$/\1/p' selfhost/cli/core/cli.xo)}"
zig_version=0.15.1
linux_glibc=2.31

if [ "$(uname -s)/$(uname -m)" != "Darwin/arm64" ]; then
	echo "release.sh: run on darwin/arm64 (the darwin xo1 and pack are built by the host)" >&2
	exit 1
fi
if [ -n "$(git status --porcelain)" ] && [ "${XO_RELEASE_ALLOW_DIRTY:-}" != 1 ]; then
	echo "release.sh: the tree has changes; commit them (or set XO_RELEASE_ALLOW_DIRTY=1)" >&2
	exit 1
fi
zig="${ZIG:-zig}"
if ! command -v "$zig" >/dev/null; then
	echo "release.sh: zig $zig_version not found (set ZIG): it cross compiles the linux/arm64 runtime" >&2
	exit 1
fi
if [ "$("$zig" version)" != "$zig_version" ]; then
	echo "release.sh: zig $("$zig" version), want $zig_version (the linux pack's compiler is pinned)" >&2
	exit 1
fi

dist="$repo/dist"
rm -rf "$dist"
mkdir -p "$dist"
unset XO_STD XO_RTPACK XO_CLI
# the seed: the Go cmd/xo with its own driver (not switch point A's exec of
# an xo1 it would first rebuild from its embedded sources)
host_xo="$dist/.host/xo"
go build -trimpath -o "$host_xo" ./cmd/xo
export XO_SELF_DRIVER=0

stage_of() { echo "$dist/xo-$version-${1%/*}-${1#*/}"; }

# the layout around bin/xo: lib/xo (the tracked files of std and
# third_party), bin/xo-gobind, the docs
layout() {
	local target=$1 stage
	stage=$(stage_of "$target")
	mkdir -p "$stage/bin" "$stage/lib/xo"
	git ls-files -z std third_party | rsync -a --from0 --files-from=- ./ "$stage/lib/xo/"
	# the zone database std/time embeds (selfhost/xo tz.xo looks here
	# before `go env GOROOT`)
	cp "$(go env GOROOT)/lib/time/zoneinfo.zip" "$stage/lib/xo/zoneinfo.zip"
	GOOS="${target%/*}" GOARCH="${target#*/}" CGO_ENABLED=0 \
		go build -trimpath -o "$stage/bin/xo-gobind" ./cmd/xo-gobind
	cp README.md CHANGELOG.md "$stage/"
}

targets="darwin/arm64 linux/arm64 linux/amd64"
for target in $targets; do
	layout "$target"
done
darwin=$(stage_of darwin/arm64)
linux_arm64=$(stage_of linux/arm64)
linux_amd64=$(stage_of linux/amd64)

echo "== darwin/arm64: the seed builds xo1" >&2
"$host_xo" build --backend=arm64 -o "$darwin/bin/xo" selfhost/xo >&2
xo1="$darwin/bin/xo"
"$xo1" version >&2
# the packs, by xo1's `xo rtpack` (the Go command's files, byte for byte:
# TestRTPackXoSame)
"$xo1" rtpack --target=darwin/arm64 -o "$darwin/lib/xo/rtpack" >&2
zcache="$dist/.zig-cache"
ZIG_GLOBAL_CACHE_DIR="$zcache" ZIG_LOCAL_CACHE_DIR="$zcache" \
	"$xo1" rtpack --target=linux/arm64 \
	--cc="$zig cc -target aarch64-linux-gnu.$linux_glibc" --ar="$zig ar" \
	-o "$linux_arm64/lib/xo/rtpack" >&2

echo "== linux/arm64: xo1 cross builds xo1 (selfhost/macho's ELF link, no cc)" >&2
XO_RTPACK="$linux_arm64/lib/xo/rtpack" "$xo1" build --backend=arm64 \
	--target=linux/arm64 --ld=macho -o "$linux_arm64/bin/xo" selfhost/xo >&2

echo "== linux/amd64: xo1 builds xo1 with the Go backend" >&2
"$xo1" build --backend=go --targets=linux/amd64 -o "$linux_amd64/bin/xo" selfhost/xo >&2

for target in $targets; do
	stage=$(stage_of "$target")
	go run ./scripts/disttar -o "$stage.tar.gz" "$stage"
done
rm -rf "$dist/.host" "$dist/.zig-cache"
(cd "$dist" && shasum -a 256 ./*.tar.gz | sed 's| \./| |' >SHA256SUMS)
cat "$dist/SHA256SUMS"
