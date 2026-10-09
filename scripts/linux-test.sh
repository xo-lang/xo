#!/usr/bin/env bash
# Run the Xo test suite inside a Linux container (Docker).
#
#   scripts/linux-test.sh [arm64|amd64] [go test arguments...]
#
# arm64 (default) runs natively on an arm64 host: the Go backend tests and the
# native goldens of internal/llvm and internal/arm64 (ELF, epoll, futex,
# signals, TCP, HTTP, ThreadSanitizer). amd64 runs under emulation (qemu or
# Rosetta in Docker Desktop) and defaults to `-short` for the whole module plus
# the full internal/llvm suite (the x86-64 LLVM backend and runtime).
#
# Extra arguments replace the default `go test` package list and flags, for
# example: scripts/linux-test.sh arm64 -run TestGolden ./internal/llvm
#
# The repository is mounted read only at /src. Builds, the Go cache, and
# temporary files live in the named volume xo-linux-cache-<arch> (remove it
# with `docker volume rm xo-linux-cache-<arch>`). Nothing is written to the
# host. The container is removed when the run ends.
set -euo pipefail

arch="${1:-arm64}"
case "$arch" in
arm64 | amd64) shift || true ;;
-*) arch=arm64 ;;
*)
	echo "usage: $0 [arm64|amd64] [go test arguments...]" >&2
	exit 2
	;;
esac

repo="$(cd "$(dirname "$0")/.." && pwd)"
go_version="$(sed -n 's/^go version go\([0-9.]*\) .*/\1/p' <<<"$(go version 2>/dev/null || echo 'go version go1.26.1 x')")"
image="xo-linux-test:${arch}"
name="xo-linux-${arch}-$$"

docker build --platform "linux/${arch}" --build-arg "GO_VERSION=${go_version}" \
	-t "$image" -f "$repo/bench/linux/Dockerfile" "$repo/bench/linux" >&2

if [ $# -gt 0 ]; then
	tests=("$@")
	cmd='go test "$@"'
elif [ "$arch" = arm64 ]; then
	cmd='go vet ./... && go test -count=1 -p 2 -timeout 60m ./...'
	tests=()
else
	cmd='go test -count=1 -p 2 -short -timeout 120m ./... && go test -count=1 -p 1 -timeout 120m ./internal/llvm'
	tests=()
fi

exec docker run --rm --name "$name" --platform "linux/${arch}" \
	--init --cap-add SYS_PTRACE --security-opt seccomp=unconfined \
	-v "$repo:/src:ro" \
	-v "xo-linux-cache-${arch}:/cache" \
	-e GOCACHE=/cache/go-build -e GOMODCACHE=/cache/mod \
	-e TMPDIR=/cache/tmp -e HOME=/cache/home \
	-w /src "$image" \
	bash -c 'mkdir -p /cache/tmp /cache/home && rm -rf /cache/tmp/* && echo "uname: $(uname -srm)" && go version && clang --version | head -1 && '"$cmd" \
	bash "${tests[@]+"${tests[@]}"}"
