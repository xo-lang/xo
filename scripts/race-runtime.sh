#!/bin/bash
# race-runtime.sh: builds ThreadSanitizer's runtime for native --race
# (decision 0273 part 3) from LLVM compiler-rt at the pinned release, for
# the release archive and the seeds; a maintainer step, never run on
# users' machines.
#
#   scripts/race-runtime.sh WORKDIR OUTDIR
#
# Downloads the pinned sources into WORKDIR (checked against the pinned
# SHA-256 sums), builds with clang, cmake, and ninja (cmake and ninja from
# PATH, else a Python virtual environment in WORKDIR), and writes
# OUTDIR/lib/xo/race/GOOS-GOARCH/ (the dylib on darwin) and
# OUTDIR/lib/xo/race/LICENSE.TXT, compiler-rt's license (Apache 2.0 with
# LLVM exceptions; THIRD_PARTY_NOTICES names it). The host's platform only:
# darwin/arm64 (the dylib) and linux/arm64 (the static archive
# libclang_rt.tsan-aarch64.a, which Xo's ELF linker links in; on the Mac
# run it in Docker: scripts/docker-linux.sh arm64 WORK -- bash
# /xo/scripts/race-runtime.sh /work/w /work/out, which installs clang,
# cmake, and ninja when missing).
set -euo pipefail

# 23.1.3: 21.1.8's runtime crashed at start on macOS 27 (a null atomic
# load in its initialization, a C program too;
# gotchas/tsan-runtime-macos-version.md); since LLVM 22 a release has only
# the whole llvm-project source
LLVM_VERSION=23.1.3
SUM_LLVM_PROJECT=c44186a7762ed28954be72e5ff6df9808e0779d4f1bf014ecc4e7e211d31ee34
URL=https://github.com/llvm/llvm-project/releases/download/llvmorg-$LLVM_VERSION

work=${1:?WORKDIR}
out=${2:?OUTDIR}
mkdir -p "$work" "$out"
work=$(cd "$work" && pwd)
out=$(cd "$out" && pwd)

case "$(uname -s)-$(uname -m)" in
  Darwin-arm64) goos=darwin goarch=arm64 lib=libclang_rt.tsan_osx_dynamic.dylib target=clang_rt.tsan_osx_dynamic libdir=darwin ;;
  Linux-aarch64) goos=linux goarch=arm64 lib=libclang_rt.tsan-aarch64.a target=clang_rt.tsan-aarch64 libdir=linux ;;
  *) echo "race-runtime.sh: darwin/arm64 and linux/arm64 only so far (decision 0273)" >&2; exit 2 ;;
esac
missing=""
for t in clang cmake ninja curl xz; do command -v $t >/dev/null || missing=1; done
if [ "$goos" = linux ] && [ -n "$missing" ]; then
  # the Docker image (scripts/docker-linux.sh, as root)
  (apt-get update -qq && apt-get install -y -qq clang cmake ninja-build python3 xz-utils curl) >/dev/null 2>&1
fi

tarball="$work/dl/llvm-project-$LLVM_VERSION.src.tar.xz"
mkdir -p "$work/dl"
[ -f "$tarball" ] || curl -fsSL -o "$tarball" "$URL/llvm-project-$LLVM_VERSION.src.tar.xz"
if command -v shasum >/dev/null; then sum="shasum -a 256"; else sum=sha256sum; fi
echo "$SUM_LLVM_PROJECT  $tarball" | $sum -c - >/dev/null || { echo "race-runtime.sh: $tarball: SHA-256 mismatch" >&2; exit 1; }

# compiler-rt, the shared cmake modules, and llvm's (compiler-rt's
# standalone build reads them)
top="llvm-project-$LLVM_VERSION.src"
src="$work/src/$top"
rm -rf "${work:?}/src"
mkdir -p "$work/src"
tar -C "$work/src" -xf "$tarball" "$top/compiler-rt" "$top/cmake" "$top/llvm/cmake"

if ! command -v cmake >/dev/null || ! command -v ninja >/dev/null; then
  [ -x "$work/venv/bin/cmake" ] || { python3 -m venv "$work/venv" && "$work/venv/bin/pip" install -q cmake ninja; }
  PATH="$work/venv/bin:$PATH"
fi

build="$work/build"
rm -rf "${build:?}"
platform=()
if [ "$goos" = linux ]; then
  # no outline atomics: the inline ARMv8.0 loops, so the archive calls no
  # libgcc __aarch64_* helper (Xo's ELF linker synthesizes only some); no
  # thread safe statics: no __cxa_guard_* from a C++ runtime Xo does not
  # link (the runtime initializes once, before threads, in __tsan_init)
  platform=(-DCMAKE_C_FLAGS=-mno-outline-atomics "-DCMAKE_CXX_FLAGS=-mno-outline-atomics -fno-threadsafe-statics")
fi
if [ "$goos" = darwin ]; then
  platform=(-DCOMPILER_RT_ENABLE_IOS=OFF -DCOMPILER_RT_ENABLE_WATCHOS=OFF -DCOMPILER_RT_ENABLE_TVOS=OFF
    -DCOMPILER_RT_ENABLE_XROS=OFF -DCMAKE_OSX_ARCHITECTURES=arm64 -DDARWIN_osx_ARCHS=arm64)
fi
cmake -G Ninja -S "$src/compiler-rt" -B "$build" \
  -DCMAKE_BUILD_TYPE=Release -DCMAKE_C_COMPILER=clang -DCMAKE_CXX_COMPILER=clang++ \
  -DCOMPILER_RT_BUILD_SANITIZERS=ON -DCOMPILER_RT_SANITIZERS_TO_BUILD=tsan \
  -DCOMPILER_RT_BUILD_BUILTINS=OFF -DCOMPILER_RT_BUILD_XRAY=OFF -DCOMPILER_RT_BUILD_LIBFUZZER=OFF \
  -DCOMPILER_RT_BUILD_PROFILE=OFF -DCOMPILER_RT_BUILD_MEMPROF=OFF -DCOMPILER_RT_BUILD_ORC=OFF \
  -DCOMPILER_RT_BUILD_CTX_PROFILE=OFF -DCOMPILER_RT_INCLUDE_TESTS=OFF \
  "${platform[@]}" \
  >"$work/configure.txt" 2>&1 || { tail -30 "$work/configure.txt"; exit 1; }
ninja -C "$build" "$target" >"$work/build.txt" 2>&1 || { tail -30 "$work/build.txt"; exit 1; }

dest="$out/lib/xo/race/$goos-$goarch"
mkdir -p "$dest"
cp "$build/lib/$libdir/$lib" "$dest/$lib"
cp "$src/compiler-rt/LICENSE.TXT" "$out/lib/xo/race/LICENSE.TXT"
echo "$dest/$lib"
