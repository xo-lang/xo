#!/bin/sh
# docker-wine.sh: run a command with Wine in a Linux container on this
# Mac's Docker, to run cross linked windows/amd64 programs.
# knowledgebase/infrastructure/toolchain.md "Windows programs under Wine
# in Docker".
#
#   scripts/docker-wine.sh MODE WORK [--] COMMAND...
#
# MODE box64 (use this one): Debian's amd64 Wine run by Box64 in a native
# linux/arm64 container; it runs structured exceptions (a stack
# overflow), which Docker Desktop's Rosetta stops (the amd64 mode:
# Debian's amd64 Wine in an emulated linux/amd64 container; ordinary
# programs run there). MODE arm64: Debian's arm64 Wine, which runs no
# ARM64 PE yet (gotchas/wine-rosetta-no-exceptions.md). As
# scripts/docker-linux.sh: WORK is mounted at
# /work (the working directory), the checkout read only at /xo. The image
# xo-wine-test:ARCH-BASE (Debian's wine, $XO_DOCKER_BASE, trixie by
# default) is built once; the container is started with --rm and nothing
# else is started or stopped. WINEPREFIX is /work/wine-ARCH (made on the
# first run), WINEDEBUG=-all unless set. Run a program with `wine
# PROGRAM.exe ARGS`.
set -eu
if [ $# -lt 3 ]; then
  echo "usage: scripts/docker-wine.sh box64|amd64|arm64 WORKDIR [--] COMMAND..." >&2
  exit 2
fi
arch=$1
work=$2
shift 2
if [ "${1:-}" = "--" ]; then
  shift
fi
case $arch in
box64 | arm64 | amd64) ;;
*)
  echo "docker-wine.sh: MODE must be box64, amd64, or arm64, not $arch" >&2
  exit 2
  ;;
esac
repo=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$work"
work=$(cd "$work" && pwd)
base=${XO_DOCKER_BASE:-debian:trixie-slim}
image=xo-wine-test:$arch-$(echo "$base" | tr ':/' '--')
platform=$arch
if [ "$arch" = box64 ]; then
  platform=arm64
  if ! docker image inspect "$image" >/dev/null 2>&1; then
    docker build --platform linux/arm64 --build-arg BASE="$base" -t "$image" - >&2 <<'DOCKERFILE'
ARG BASE=debian:trixie-slim
FROM $BASE
RUN dpkg --add-architecture amd64 && apt-get update && apt-get install -y --no-install-recommends \
    box64 wine64:amd64 libwine:amd64 file procps ca-certificates \
  && rm -rf /var/lib/apt/lists/* \
  && printf '#!/bin/sh\nexec box64 /usr/lib/wine/wine64 "$@"\n' > /usr/local/bin/wine \
  && printf '#!/bin/sh\nexec box64 /usr/lib/wine/wineserver "$@"\n' > /usr/local/bin/wineserver \
  && chmod +x /usr/local/bin/wine /usr/local/bin/wineserver
ENV BOX64_LOG=0
DOCKERFILE
  fi
elif ! docker image inspect "$image" >/dev/null 2>&1; then
  docker build --platform "linux/$arch" --build-arg BASE="$base" -t "$image" - >&2 <<'DOCKERFILE'
ARG BASE=debian:trixie-slim
FROM $BASE
RUN apt-get update && apt-get install -y --no-install-recommends \
    wine file procps ca-certificates \
  && (apt-get install -y --no-install-recommends wine64 || true) \
  && rm -rf /var/lib/apt/lists/*
DOCKERFILE
fi
exec docker run --rm --platform "linux/$platform" -v "$repo:/xo:ro" -v "$work:/work" -w /work \
  -e HOME=/work/home -e WINEPREFIX="/work/wine-$arch" -e WINEDEBUG="${WINEDEBUG:--all}" \
  -e XDG_RUNTIME_DIR=/work/home/xdg "$image" sh -c "mkdir -p /work/home/xdg && chmod 700 /work/home/xdg && $*"
