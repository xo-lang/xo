#!/bin/sh
# docker-linux.sh: run a command in a Linux container (Debian with clang,
# lld, and make: trixie, clang 19, by default; $XO_DOCKER_BASE picks
# another image, such as debian:bookworm-slim for clang 14) on this Mac's
# Docker, natively for linux/arm64 and
# emulated for linux/amd64 (knowledgebase/infrastructure/toolchain.md
# "Linux in Docker on the Mac").
#
#   scripts/docker-linux.sh ARCH WORK [--] COMMAND...
#
# ARCH is arm64 or amd64. WORK is a host directory mounted at /work (the
# command's working directory); the checkout is mounted read only at
# /xo. The image xo-linux-test:ARCH-BASE is built once from the Dockerfile
# below (docker build caches it). The container is started with --rm, so
# it is removed when the command ends; nothing else is started or
# stopped. The command runs under sh -c; its exit status is the script's.
set -eu
if [ $# -lt 3 ]; then
  echo "usage: scripts/docker-linux.sh arm64|amd64 WORKDIR [--] COMMAND..." >&2
  exit 2
fi
arch=$1
work=$2
shift 2
if [ "${1:-}" = "--" ]; then
  shift
fi
case $arch in
arm64 | amd64) ;;
*)
  echo "docker-linux.sh: ARCH must be arm64 or amd64, not $arch" >&2
  exit 2
  ;;
esac
repo=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$work"
work=$(cd "$work" && pwd)
base=${XO_DOCKER_BASE:-debian:trixie-slim}
image=xo-linux-test:$arch-$(echo "$base" | tr ':/' '--')
if ! docker image inspect "$image" >/dev/null 2>&1; then
  docker build --platform "linux/$arch" --build-arg BASE="$base" -t "$image" - >&2 <<'DOCKERFILE'
ARG BASE=debian:trixie-slim
FROM $BASE
RUN apt-get update && apt-get install -y --no-install-recommends \
    clang lld llvm make file binutils ca-certificates procps \
  && rm -rf /var/lib/apt/lists/*
DOCKERFILE
fi
exec docker run --rm --platform "linux/$arch" -v "$repo:/xo:ro" -v "$work:/work" -w /work \
  -e HOME=/work/home "$image" sh -c "mkdir -p /work/home && $*"
