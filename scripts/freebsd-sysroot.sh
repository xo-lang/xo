#!/bin/bash
# freebsd-sysroot.sh ARCH DIR: FreeBSD 14.5-RELEASE's base.txz for ARCH
# (amd64 or arm64), checked against its MANIFEST's sha256, unpacked into
# DIR (usr/include, lib, usr/lib only): the headers a `use c` binding
# compiles against for --targets=freebsd/ARCH (XO_FREEBSDSYSROOT=DIR,
# decision 0573). About 190 MB to fetch, 460 MB unpacked.
set -u
arch=${1:?usage: freebsd-sysroot.sh amd64|arm64 DIR}; dir=${2:?usage: freebsd-sysroot.sh amd64|arm64 DIR}
case $arch in
  amd64) path=amd64/amd64 ;;
  arm64) path=arm64/aarch64 ;;
  *) echo "freebsd-sysroot: ARCH is amd64 or arm64"; exit 2 ;;
esac
u=https://download.freebsd.org/releases/$path/14.5-RELEASE
mkdir -p "$dir" || exit 2
t=$(mktemp -d "${TMPDIR:-/tmp}/xofbsys.XXXXXX") || exit 2
trap 'rm -rf "${t:?}"' EXIT
want=$(curl -sSfL "$u/MANIFEST" | awk '$1 == "base.txz" { print $2 }')
[ -n "$want" ] || { echo "freebsd-sysroot: no base.txz in $u/MANIFEST"; exit 3; }
curl -sSfL -o "$t/base.txz" "$u/base.txz" || { echo "freebsd-sysroot: fetch failed"; exit 3; }
if command -v sha256sum > /dev/null; then got=$(sha256sum "$t/base.txz" | cut -d' ' -f1); else got=$(shasum -a 256 "$t/base.txz" | cut -d' ' -f1); fi
[ "$got" = "$want" ] || { echo "freebsd-sysroot: sha256 $got, MANIFEST says $want"; exit 4; }
tar -xf "$t/base.txz" -C "$dir" ./usr/include ./lib ./usr/lib || exit 5
echo "freebsd-sysroot: $dir ($arch, base.txz sha256 $got)"
