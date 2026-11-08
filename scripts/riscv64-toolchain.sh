#!/bin/bash
# riscv64-toolchain.sh [DIR]: the linux/riscv64 test toolchain of decision
# 0571 under DIR (default /work/riscv64) on a Debian trixie host, with no
# root, no system packages and no binfmt registration: qemu-user for the
# host's architecture (run as DIR/qemu/usr/bin/qemu-riscv64, named on the
# command line) and a riscv64 glibc sysroot (DIR/sysroot, for
# XO_LINUXSYSROOT). Every file comes from deb.debian.org and is verified:
# Release by its detached signature (sqv, else gpgv) against the host's
# debian-archive-keyring, each Packages index by its SHA256 in Release,
# each .deb by its SHA256 in that index. Rerun it to rebuild DIR from scratch.
# Last verified: 2026-11-08 (offload host 2, Debian 13 trixie, arm64)
set -euo pipefail
exec </dev/null
D=${1:-/work/riscv64}
M=http://deb.debian.org/debian
SUITE=trixie
KEYRING=/usr/share/keyrings/debian-archive-keyring.gpg
HOSTARCH=$(dpkg --print-architecture)
[ -r "$KEYRING" ] || { echo "riscv64-toolchain: no $KEYRING (debian-archive-keyring)"; exit 2; }
rm -rf "${D:?}.new"
mkdir -p "$D.new/dl" "$D.new/sysroot" "$D.new/qemu"
cd "$D.new/dl"
# Release and its detached signature, checked with sqv (Sequoia, what
# trixie's apt uses; trixie has no gpgv by default) or gpgv
curl -fsSL -o Release "$M/dists/$SUITE/Release"
curl -fsSL -o Release.gpg "$M/dists/$SUITE/Release.gpg"
if command -v sqv >/dev/null; then
	sqv --keyring "$KEYRING" Release.gpg Release >/dev/null || { echo "riscv64-toolchain: Release signature check failed (sqv)"; exit 1; }
elif command -v gpgv >/dev/null; then
	gpgv --keyring "$KEYRING" Release.gpg Release || { echo "riscv64-toolchain: Release signature check failed (gpgv)"; exit 1; }
else
	echo "riscv64-toolchain: neither sqv nor gpgv is installed"; exit 2
fi
echo "riscv64-toolchain: Release signed ($(awk '/^Version:/{print $2}' Release), $(awk '/^Date:/{sub(/^Date: /,""); print}' Release))"
# index ARCH: main/binary-ARCH/Packages.xz, checked against Release
index() {
	local path="main/binary-$1/Packages.xz" want got
	want=$(awk -v p="$path" '/^SHA256:/{s=1; next} /^[A-Za-z]/{s=0} s && $3==p {print $1}' Release)
	[ -n "$want" ] || { echo "riscv64-toolchain: $path not in Release"; exit 1; }
	curl -fsSL -o "Packages-$1.xz" "$M/dists/$SUITE/$path"
	got=$(sha256sum "Packages-$1.xz" | cut -d' ' -f1)
	[ "$got" = "$want" ] || { echo "riscv64-toolchain: $path sha256 $got, Release says $want"; exit 1; }
	xz -dc "Packages-$1.xz" > "Packages-$1"
}
# fetch ARCH PKG DEST: the package's .deb, checked against the index, unpacked into DEST
fetch() {
	local rec file want got
	# the stanza whose Package field is exactly $2 (a literal match: names
	# such as libstdc++6 hold regular expression characters)
	rec=$(awk -v p="$2" 'BEGIN{RS=""} index("\n" $0 "\n", "\nPackage: " p "\n") {print; exit}' "Packages-$1")
	file=$(printf '%s\n' "$rec" | awk '/^Filename: /{print $2}')
	want=$(printf '%s\n' "$rec" | awk '/^SHA256: /{print $2}')
	[ -n "$file" ] && [ -n "$want" ] || { echo "riscv64-toolchain: $2 ($1) not in the index"; exit 1; }
	curl -fsSL -o "$2_$1.deb" "$M/$file"
	got=$(sha256sum "$2_$1.deb" | cut -d' ' -f1)
	[ "$got" = "$want" ] || { echo "riscv64-toolchain: $2 sha256 $got, the index says $want"; exit 1; }
	dpkg-deb -x "$2_$1.deb" "$3"
	echo "  $2 ($1) $(printf '%s\n' "$rec" | awk '/^Version: /{print $2}')"
}
index riscv64
index "$HOSTARCH"
echo "riscv64-toolchain: sysroot"
for p in libc6 libc6-dev linux-libc-dev libgcc-s1 libgcc-14-dev libstdc++6; do
	fetch riscv64 "$p" "$D.new/sysroot"
done
# trixie's /usr is merged: base-files makes /lib a link to usr/lib, which
# libc.so's GROUP and the dynamic loader's path name; make it here
[ -e "$D.new/sysroot/lib" ] || ln -s usr/lib "$D.new/sysroot/lib"
# the static qemu needs no host libraries; trixie names it qemu-user-static
# or (after the rename) qemu-user, so take the first that has the binary
Q=
for qp in qemu-user-static qemu-user; do
	grep -q "^Package: $qp\$" "Packages-$HOSTARCH" || continue
	echo "riscv64-toolchain: $qp ($HOSTARCH)"
	fetch "$HOSTARCH" "$qp" "$D.new/qemu"
	Q=$(find "$D.new/qemu" -type f \( -name qemu-riscv64-static -o -name qemu-riscv64 \) | head -1)
	[ -n "$Q" ] && break
done
[ -n "$Q" ] || { echo "riscv64-toolchain: no qemu-riscv64 in qemu-user-static or qemu-user"; exit 1; }
rm -rf "$D.new/dl"
rm -rf "${D:?}"
mv "$D.new" "$D"
echo "riscv64-toolchain: ready: qemu ${Q#$D.new/} -> $D/${Q#$D.new/}, sysroot $D/sysroot"
"$D/${Q#$D.new/}" --version | head -1
