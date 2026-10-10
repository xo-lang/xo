#!/usr/bin/env bash
# Vendor Mbed TLS into third_party/mbedtls/src (decision 0105).
#
#   scripts/vendor-mbedtls.sh [version sha256]
#
# Downloads the release tarball from GitHub, checks its SHA-256 against the
# pinned value (or the one given), copies the .c and .h files the native
# runtime compiles (include/, library/, and TF-PSA-Crypto's include/,
# core/, dispatch/, drivers/builtin, drivers/everest, drivers/p256-m,
# extras/, platform/, utilities/) and LICENSE, and regenerates
# third_party/mbedtls/mbedtls.tgz. The release tarball already holds the
# generated sources (error.c, the PSA driver wrappers), so no Perl or
# Python runs. After an update: set Version in mbedtls.go, the pinned
# values below and in knowledgebase/infrastructure/toolchain.md, then run
# `go test ./third_party/mbedtls ./internal/build` and the TLS tests
# (`go test ./internal/llvm -run 'TestTLSDiff|http_tls'`).
set -euo pipefail

version="${1:-4.1.1}"
sum="${2:-3359a349e23db3d5536fcee032ae7b2ecbfc08972fab643089b5cbf2a375c98c}"
repo="$(cd "$(dirname "$0")/.." && pwd)"
dest="$repo/third_party/mbedtls"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

url="https://github.com/Mbed-TLS/mbedtls/releases/download/mbedtls-$version/mbedtls-$version.tar.bz2"
curl -fsSL -o "$tmp/mbedtls.tar.bz2" "$url"
got="$(shasum -a 256 "$tmp/mbedtls.tar.bz2" | cut -d' ' -f1)"
if [ "$got" != "$sum" ]; then
	echo "vendor-mbedtls: SHA-256 mismatch: got $got, want $sum" >&2
	exit 1
fi
mkdir "$tmp/x"
tar -xjf "$tmp/mbedtls.tar.bz2" -C "$tmp/x"
src="$tmp/x/mbedtls-$version"

rm -rf "$dest/src"
mkdir -p "$dest/src"
for d in include library tf-psa-crypto/include tf-psa-crypto/core tf-psa-crypto/dispatch \
	tf-psa-crypto/drivers/builtin tf-psa-crypto/drivers/everest tf-psa-crypto/drivers/p256-m \
	tf-psa-crypto/extras tf-psa-crypto/platform tf-psa-crypto/utilities; do
	[ -d "$src/$d" ] || continue
	(cd "$src" && find "$d" -type f \( -name '*.c' -o -name '*.h' \)) | while read -r f; do
		mkdir -p "$dest/src/$(dirname "$f")"
		cp "$src/$f" "$dest/src/$f"
	done
done
cp "$src/LICENSE" "$dest/LICENSE"
(cd "$dest" && go generate .)
echo "vendored mbedtls-$version: $(find "$dest/src" -name '*.c' | wc -l | tr -d ' ') .c files, mbedtls.tgz $(wc -c <"$dest/mbedtls.tgz" | tr -d ' ') bytes"
