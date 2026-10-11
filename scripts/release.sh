#!/usr/bin/env bash
# Build the release archives of xo (decision 0157) into dist/:
#
#   scripts/release.sh [version]
#
# For each target (darwin/arm64, linux/arm64, linux/amd64) it builds xo
# (pure Go, -trimpath) and, for the arm64 backend's targets, the prebuilt C
# runtime (xo rtpack: every variant of std/llrt and the TLS library) into
#
#   dist/xo-<version>-<os>-<arch>/bin/xo
#   dist/xo-<version>-<os>-<arch>/lib/xo/rtpack/<os>-<arch>/
#
# then writes dist/xo-<version>-<os>-<arch>.tar.gz (scripts/disttar: the
# same bytes for the same inputs) and dist/SHA256SUMS. It never tags,
# pushes, or uploads.
#
# Compilers (both pinned, the pack manifests record them):
#   darwin/arm64  the host's Apple clang (run this script on a Mac)
#   linux/arm64   zig cc 0.15.1 with glibc 2.31 headers: $ZIG, else zig on PATH
#                 (https://ziglang.org/download/, check the tarball's SHA-256
#                 against https://ziglang.org/download/index.json)
#
# Environment: ZIG (zig binary), XO_RELEASE_ALLOW_DIRTY=1 (build from a
# modified tree, for trying the script).
set -euo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo"
version="${1:-v$(sed -n 's/^const version = "\(.*\)"$/\1/p' cmd/xo/version.go)}"
zig_version=0.15.1
linux_glibc=2.31

if [ "$(uname -s)/$(uname -m)" != "Darwin/arm64" ]; then
	echo "release.sh: run on darwin/arm64 (the darwin pack is built by the host's Apple clang)" >&2
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
host_xo="$dist/.host/xo"
go build -trimpath -o "$host_xo" ./cmd/xo

for target in darwin/arm64 linux/arm64 linux/amd64; do
	os="${target%/*}"
	arch="${target#*/}"
	name="xo-$version-$os-$arch"
	stage="$dist/$name"
	mkdir -p "$stage/bin"
	echo "== $target" >&2
	GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build -trimpath -o "$stage/bin/xo" ./cmd/xo
	case "$target" in
	darwin/arm64)
		"$host_xo" rtpack --target="$target" -o "$stage/lib/xo/rtpack" >&2
		;;
	linux/arm64)
		zcache="$dist/.zig-cache"
		ZIG_GLOBAL_CACHE_DIR="$zcache" ZIG_LOCAL_CACHE_DIR="$zcache" \
			"$host_xo" rtpack --target="$target" \
			--cc="$zig cc -target aarch64-linux-gnu.$linux_glibc" --ar="$zig ar" \
			-o "$stage/lib/xo/rtpack" >&2
		;;
	esac
	cp README.md CHANGELOG.md "$stage/"
	go run ./scripts/disttar -o "$dist/$name.tar.gz" "$stage"
done
rm -rf "$dist/.host" "$dist/.zig-cache"
(cd "$dist" && shasum -a 256 ./*.tar.gz | sed 's| \./| |' >SHA256SUMS)
cat "$dist/SHA256SUMS"
