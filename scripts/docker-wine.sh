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
# default; box64's is pinned and carries a booted prefix, below) is
# built once; the container is started with --rm and nothing else is
# started or stopped. WINEPREFIX is /work/wine-ARCH (box64: copied from
# the image's when WORK has none; otherwise made on the first run),
# WINEDEBUG=-all unless set. Run a program with `wine
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
  # pinned (owner, 2026-10-22): the base by digest and Box64's and Wine's
  # packages by version, the ones that ran the goldens that day (image
  # 76f2ef49be04), so a rebuild or a pull never changes the runner
  # (gotchas/wine-box64-harness.md); the tag names the pins. A pinned
  # package Debian has since replaced fails the build: take it from
  # snapshot.debian.org, or move the pins after a goldens run.
  pin_base=debian:trixie-slim@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad
  pin_box64=0.3.4+dfsg-1
  pin_wine=10.0~repack-6
  base=${XO_DOCKER_BASE:-$pin_base}
  image=xo-wine-test:box64-trixie-62bd423-box64-$pin_box64-wine-$pin_wine-prefix1
  image=$(echo "$image" | tr '+~' '__')
  [ -n "${XO_DOCKER_BASE:-}" ] && image=xo-wine-test:box64-$(echo "$base" | tr ':/@' '---')
  platform=arm64
  if ! docker image inspect "$image" >/dev/null 2>&1; then
    docker build --platform linux/arm64 --build-arg BASE="$base" --build-arg BOX64="$pin_box64" --build-arg WINE="$pin_wine" -t "$image" - >&2 <<'DOCKERFILE'
ARG BASE=debian:trixie-slim
FROM $BASE
ARG BOX64
ARG WINE
RUN dpkg --add-architecture amd64 && apt-get update && apt-get install -y --no-install-recommends \
    box64=$BOX64 wine64:amd64=$WINE libwine:amd64=$WINE file procps ca-certificates \
  && rm -rf /var/lib/apt/lists/* \
  && printf '#!/bin/sh\nexec box64 /usr/lib/wine/wine64 "$@"\n' > /usr/local/bin/wine \
  && printf '#!/bin/sh\nexec box64 /usr/lib/wine/wineserver "$@"\n' > /usr/local/bin/wineserver \
  && chmod +x /usr/local/bin/wine /usr/local/bin/wineserver
ENV BOX64_LOG=0
# a prefix booted once here, checked with `cmd /c echo` and made again
# when Box64 stopped its wineboot part way (an illegal instruction, more
# often under load: every program in it exits 53 or c0000135); each run
# copies it into a new WORK instead of booting its own
RUN export HOME=/root WINEPREFIX=/opt/xo/wine-box64 WINEDEBUG=-all && mkdir -p /opt/xo \
  && for i in 1 2 3 4 5; do \
       wineboot -u >/dev/null 2>&1; wineserver -w; \
       if wine cmd /c echo xo-ok 2>/dev/null | grep -q xo-ok; then break; fi; \
       wineserver -k; wineserver -w; rm -rf "$WINEPREFIX"; \
     done \
  && wine cmd /c echo xo-ok 2>/dev/null | grep -q xo-ok \
  && (wineserver -k || true) && wineserver -w
DOCKERFILE
  fi
  prep="[ -d /work/wine-box64 ] || cp -a /opt/xo/wine-box64 /work/wine-box64; "
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
  -e XDG_RUNTIME_DIR=/work/home/xdg "$image" sh -c "mkdir -p /work/home/xdg && chmod 700 /work/home/xdg && ${prep:-}$*"
