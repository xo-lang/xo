#!/bin/sh
# rust.sh under valgrind on linux/arm64 in Docker (CALLGRIND=1): exact
# instruction counts of the Go, Rust, and Xo sides, the method of the
# Linux rows in bench/results. On this Mac's Docker, natively:
#
#   - a Linux xo built here with the Go backend (`$XO build --backend=go
#     --targets=linux/arm64`, the checkout's selfhost/xo and std),
#   - the Go side cross compiled here (GOOS=linux, CGO_ENABLED=0),
#   - the Rust side built in the official image rust:$RUST_IMAGE
#     (default 1.99.1-slim-trixie, Debian trixie like the run's image),
#   - the Xo sides built and every side run in scripts/docker-linux.sh's
#     image (trixie, clang) with valgrind added (as root, in the
#     container only).
#
# Docker runs at nice 10 and outside the heavy lock (docker-linux.sh's
# rule); everything else is rust.sh's (WORKLOADS, SIDES). OUT (default a
# new temporary directory) keeps the binaries and the Linux xo.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
xo=${XO:-$root/.tools/bin/xo}
out=${OUT:-$(mktemp -d)}
mkdir -p "$out"
out=$(cd "$out" && pwd)
image=rust:${RUST_IMAGE:-1.99.1-slim-trixie}
works=${WORKLOADS:-fib sieve nbody sort interp collatz strings}
sides=${SIDES:-go rust xo-llvm xo-arm64}
if [ ! -x "$out/lx/bin/xo" ]; then
	"$xo" build --backend=go --targets=linux/arm64 -o "$out/lx/bin/xo" "$root/selfhost/xo" >/dev/null
	rm -rf "$out/lx/std"
	cp -R "$root/std" "$out/lx/std"
fi
(cd "$here/go" && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$out/llperf-go" .)
rm -rf "$out/rust-src"
mkdir -p "$out/rust-src"
cp -R "$here/rust/Cargo.toml" "$here/rust/src" "$out/rust-src/"
nice -n 10 docker run --rm --platform linux/arm64 --user "$(id -u):$(id -g)" \
	-e CARGO_HOME=/out/cargo-home -v "$out:/out" -w /out/rust-src "$image" \
	sh -c 'CARGO_TARGET_DIR=/out/rust-target cargo build --release -q && cp /out/rust-target/release/llperf /out/llperf-rust'
# the Xo sides, built by the Linux xo, then every side under callgrind
XO_DOCKER_ROOT=1 nice -n 10 "$root/scripts/docker-linux.sh" arm64 "$out" -- "
	set -e
	(apt-get update -qq && apt-get install -y -qq valgrind) >/dev/null 2>&1
	export HOME=/work/home
	mkdir -p \$HOME
	for w in $works; do
		case ' $sides ' in *' xo-llvm '*) /work/lx/bin/xo build --backend=llvm -o /work/ll-\$w /xo/bench/perf/llvm/xo/\$w >/dev/null ;; esac
		case ' $sides ' in *' xo-arm64 '*) /work/lx/bin/xo build --backend=arm64 -o /work/a64-\$w /xo/bench/perf/llvm/xo/\$w >/dev/null ;; esac
	done
	PREBUILT=1 CALLGRIND=1 PERF_OUT=/work SIDES='$sides' WORKLOADS='$works' sh /xo/bench/perf/llvm/rust.sh
"
