#!/bin/sh
# docker-wine.sh: run a command with Wine in a Linux container on this
# Mac's Docker, to run cross linked Windows programs (windows/amd64 under
# Wine in an emulated linux/amd64 container; windows/arm64 under an
# arm64 Wine in a native linux/arm64 container, where Wine supports ARM64
# PE). knowledgebase/infrastructure/toolchain.md "Windows programs under
# Wine in Docker".
#
#   scripts/docker-wine.sh ARCH WORK [--] COMMAND...
#
# As scripts/docker-linux.sh: ARCH is amd64 or arm64, WORK is mounted at
# /work (the working directory), the checkout read only at /xo. The image
# xo-wine-test:ARCH-BASE (Debian's wine, $XO_DOCKER_BASE, trixie by
# default) is built once; the container is started with --rm and nothing
# else is started or stopped. WINEPREFIX is /work/wine-ARCH (made on the
# first run), WINEDEBUG=-all unless set. Run a program with `wine
# PROGRAM.exe ARGS`.
set -eu
if [ $# -lt 3 ]; then
  echo "usage: scripts/docker-wine.sh amd64|arm64 WORKDIR [--] COMMAND..." >&2
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
  echo "docker-wine.sh: ARCH must be arm64 or amd64, not $arch" >&2
  exit 2
  ;;
esac
repo=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$work"
work=$(cd "$work" && pwd)
base=${XO_DOCKER_BASE:-debian:trixie-slim}
image=xo-wine-test:$arch-$(echo "$base" | tr ':/' '--')
if ! docker image inspect "$image" >/dev/null 2>&1; then
  docker build --platform "linux/$arch" --build-arg BASE="$base" -t "$image" - >&2 <<'DOCKERFILE'
ARG BASE=debian:trixie-slim
FROM $BASE
RUN apt-get update && apt-get install -y --no-install-recommends \
    wine file procps ca-certificates \
  && (apt-get install -y --no-install-recommends wine64 || true) \
  && rm -rf /var/lib/apt/lists/*
DOCKERFILE
fi
exec docker run --rm --platform "linux/$arch" -v "$repo:/xo:ro" -v "$work:/work" -w /work \
  -e HOME=/work/home -e WINEPREFIX="/work/wine-$arch" -e WINEDEBUG="${WINEDEBUG:--all}" \
  -e XDG_RUNTIME_DIR=/work/home/xdg "$image" sh -c "mkdir -p /work/home/xdg && chmod 700 /work/home/xdg && $*"
