#!/bin/bash
# lld-probe.sh: proves the runner clang fix before the boot script changes
# (knowledgebase/gotchas/runner-clang-no-lld.md): installs the pinned upstream
# LLVM release (the same version and SHA-256 as infra/ci-runners/boot/linux.sh)
# into a temporary directory, makes its clang link with lld through a
# clang.cfg, and builds and runs one LTO golden with it on the runner
#   gh workflow run ci.yml -f ref=BRANCH -f suite=script -f packages=scripts/ci/lld-probe.sh
# Exit 1 when the install, the checksum, the lld link or the golden fails.
set -uo pipefail
cd "$(dirname "$0")/../.."
out=${XO_CI_OUT:-$(mktemp -d)}
mkdir -p "$out"
LLVMVER=23.1.3
case $(uname -m) in
  aarch64) lt=ARM64; sha=56d005e339c979a696ed6c244e6472e16f3b1371e48987a07e6145bda7ee0d03;;
  x86_64) lt=X64; sha=ca9fe385a5d9e2d88132dc0e23a00e9f87f38fa75f67207b5e457d208e71d013;;
  *) echo "unsupported machine $(uname -m)"; exit 1;;
esac
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "== before: clang $(clang --version 2>&1 | head -1); ld.lld $(command -v ld.lld || echo none)"
curl -fsSL "https://github.com/llvm/llvm-project/releases/download/llvmorg-$LLVMVER/LLVM-$LLVMVER-Linux-$lt.tar.xz" -o "$tmp/llvm.tar.xz" || { echo "download failed"; exit 1; }
echo "$sha  $tmp/llvm.tar.xz" | sha256sum -c - || { echo "checksum mismatch"; exit 1; }
mkdir "$tmp/llvm"
tar -C "$tmp/llvm" --strip-components=1 -xJf "$tmp/llvm.tar.xz" || { echo "unpack failed"; exit 1; }
rm -f "$tmp/llvm.tar.xz"
test -x "$tmp/llvm/bin/ld.lld" || { echo "no ld.lld in the release"; exit 1; }
printf -- '-fuse-ld=lld\n' > "$tmp/llvm/bin/clang.cfg"
export XO_CLANG=$tmp/llvm/bin/clang
echo "== after: $("$XO_CLANG" --version | head -1)"
rc=0
# one LTO golden: a single module build at -O2 links with -flto
XO_LLVM_LTO=full "$XO_CLI" dev goldens --backend=llvm --force --no-cache --only=arith internal/llvm/testdata/golden > "$out/lld-probe.txt" 2>&1 || rc=1
tail -3 "$out/lld-probe.txt"
echo "lld-probe exit $rc"
exit $rc
