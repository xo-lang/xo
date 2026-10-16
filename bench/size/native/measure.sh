#!/bin/bash
# Binary sizes of the same four programs in Xo, Go, and Rust on the host
# (darwin/arm64 or linux/arm64): hello, an HTTPS GET, a JSON HTTP server,
# and the notes service (examples/notes-service; go/notes and rust/notes
# port it). Results: bench/results/binary-size-native-2026-10-16.md.
#
#   bench/size/native/measure.sh OUTDIR
#
# XO is the xo binary (default the checkout's bin/xo), GO and CARGO the
# toolchains (default go and cargo on PATH). Prints one line per build:
#   <program> <build> <bytes> <stripped bytes> <debug info bytes>
# where stripped is `strip` of a copy (strip -S on Linux keeps symbols
# too, so plain strip) and debug info is the PROG.dSYM bundle on darwin
# (0 elsewhere: the ELF keeps it, which strip removes).
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
out=${1:?usage: measure.sh OUTDIR}
mkdir -p "$out"
out=$(cd "$out" && pwd)
xo=${XO:-$root/bin/xo}
go=${GO:-go}
cargo=${CARGO:-cargo}
size() { if [ "$(uname)" = Darwin ]; then stat -f %z "$1"; else stat -c %s "$1"; fi; }
dsym() {
	if [ -d "$1.dSYM" ]; then
		du -sk "$1.dSYM" | awk '{print $1 * 1024}'
	else
		echo 0
	fi
}
row() { # program build binary
	cp "$3" "$3.strip"
	strip "$3.strip" 2>/dev/null
	echo "$1 $2 $(size "$3") $(size "$3.strip") $(dsym "$3")"
}
progs="hello https_get json_server notes"
xodir() { if [ "$1" = notes ]; then echo "$root/examples/notes-service"; else echo "$here/xo/$1"; fi; }
for p in $progs; do
	for be in arm64 llvm; do
		(cd "$(xodir "$p")" && XO_BIN_CACHE=off "$xo" build --backend=$be -o "$out/xo-$be-$p" . >/dev/null)
		row "$p" "xo-$be" "$out/xo-$be-$p"
		(cd "$(xodir "$p")" && XO_BIN_CACHE=off "$xo" build --backend=$be -s -o "$out/xo-$be-s-$p" . >/dev/null)
		row "$p" "xo-$be-s" "$out/xo-$be-s-$p"
	done
	(cd "$here/go" && "$go" build -o "$out/go-$p" "./$p")
	row "$p" go "$out/go-$p"
	(cd "$here/go" && "$go" build -ldflags="-s -w" -o "$out/go-sw-$p" "./$p")
	row "$p" go-sw "$out/go-sw-$p"
done
(cd "$here/rust" && CARGO_TARGET_DIR="$out/rust-target" "$cargo" build --release --locked -q)
for p in $progs; do
	cp "$out/rust-target/release/$p" "$out/rust-$p"
	row "$p" rust "$out/rust-$p"
done
(cd "$here/rust" && CARGO_TARGET_DIR="$out/rust-target-strip" CARGO_PROFILE_RELEASE_STRIP=true "$cargo" build --release --locked -q)
for p in $progs; do
	cp "$out/rust-target-strip/release/$p" "$out/rust-strip-$p"
	row "$p" rust-strip "$out/rust-strip-$p"
done
