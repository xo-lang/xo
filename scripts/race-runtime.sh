#!/bin/bash
# race-runtime.sh: builds ThreadSanitizer's runtime for native --race
# (decision 0273 part 3) from LLVM compiler-rt at the pinned release, for
# the release archive and the seeds; a maintainer step, never run on
# users' machines.
#
#   scripts/race-runtime.sh WORKDIR OUTDIR [linux-arm64|linux-amd64|darwin-amd64|windows-amd64]
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
# cmake, and ninja when missing). On the Mac, linux-arm64 as the third
# argument does that itself: the script copied into WORKDIR, run there in
# Docker as root, the archive and the license copied to OUTDIR/lib/xo/race
# (seed-cut.sh's linux-arm64 seed; before release.sh, OUTDIR the checkout).
# linux-amd64 does the same in Docker linux/amd64 (libclang_rt.tsan-x86_64.a
# for the amd64 backend's --race), and darwin-amd64 on an arm64 Mac builds
# the x86_64 dylib (lib/xo/race/darwin-amd64). windows-amd64 builds
# ThreadSanitizer's Go flavor (compiler-rt lib/tsan/go, as Go's
# race_windows_amd64.syso; decision 0273's windows addendum) with MinGW-w64
# g++ in Docker linux/arm64: one COFF object, race_windows_amd64.syso, with
# scripts/race-windows.cpp's additions, which Xo's PE linker links.
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

if [ "${3:-}" = windows-amd64 ]; then
  here=$(cd "$(dirname "$0")" && pwd)
  if [ "$(uname -s)" = Darwin ]; then
    cp "$0" "$work/race-runtime.sh"
    cp "$here/race-windows.cpp" "$work/race-windows.cpp"
    XO_DOCKER_ROOT=1 nice -n 10 "$here/docker-linux.sh" arm64 "$work" -- bash /work/race-runtime.sh /work/w /work/out windows-amd64
    o="$work/out/lib/xo/race/windows-amd64/race_windows_amd64.syso"
    [ -s "$o" ] || { echo "race-runtime.sh: Docker build wrote no $o" >&2; exit 1; }
    mkdir -p "$out/lib/xo/race"
    rm -rf "${out:?}/lib/xo/race/windows-amd64"
    cp -R "$work/out/lib/xo/race/windows-amd64" "$out/lib/xo/race/"
    cp "$work/out/lib/xo/race/LICENSE.TXT" "$out/lib/xo/race/"
    echo "$out/lib/xo/race/windows-amd64/race_windows_amd64.syso"
    exit 0
  fi
  command -v x86_64-w64-mingw32-g++ >/dev/null || (apt-get update -qq && apt-get install -y -qq g++-mingw-w64-x86-64 xz-utils curl) >/dev/null 2>&1
  tarball="$work/dl/llvm-project-$LLVM_VERSION.src.tar.xz"
  mkdir -p "$work/dl"
  [ -f "$tarball" ] || curl -fsSL -o "$tarball" "$URL/llvm-project-$LLVM_VERSION.src.tar.xz"
  echo "$SUM_LLVM_PROJECT  $tarball" | sha256sum -c - >/dev/null || { echo "race-runtime.sh: $tarball: SHA-256 mismatch" >&2; exit 1; }
  top="llvm-project-$LLVM_VERSION.src"
  rm -rf "${work:?}/src"
  mkdir -p "$work/src"
  tar -C "$work/src" -xf "$tarball" "$top/compiler-rt"
  g="$work/src/$top/compiler-rt/lib/tsan/go"
  cp "$here/race-windows.cpp" "$g/xo_race_windows.cpp"
  # Go's build.bat flags (Windows 7 APIs, SSE3 only), no C runtime
  # imports (_CRTIMP empty: race-windows.cpp defines _errno and exit), no
  # test program (it needs a POSIX C library)
  (cd "$g" && GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-g++ SKIP_TEST=1 DEPENDS_ON_LIBC=1 \
    ADD_SRCS=xo_race_windows.cpp \
    EXTRA_CFLAGS="-ffile-prefix-map=$work=. -fdebug-prefix-map=$work=. -DWINVER=0x0600 -D_WIN32_WINNT=0x0600 -DGetProcessMemoryInfo=K32GetProcessMemoryInfo -DSANITIZER_WINDOWS=1 -D_CRTIMP= -msse3 -Wno-format -Wno-maybe-uninitialized" \
    ./buildgo.sh >"$work/build.txt" 2>&1) || true
  [ -s "$g/race_windows_amd64.syso" ] || { tail -30 "$work/build.txt"; exit 1; }
  dest="$out/lib/xo/race/windows-amd64"
  mkdir -p "$dest"
  cp "$g/race_windows_amd64.syso" "$dest/"
  cp "$work/src/$top/compiler-rt/LICENSE.TXT" "$out/lib/xo/race/LICENSE.TXT"
  echo "$dest/race_windows_amd64.syso"
  exit 0
fi

if [ "${3:-}" = linux-arm64 ] || [ "${3:-}" = linux-amd64 ]; then
  if [ "$(uname -s)" = Darwin ]; then
    here=$(cd "$(dirname "$0")" && pwd)
    da=${3#linux-}
    an=$([ "$da" = arm64 ] && echo aarch64 || echo x86_64)
    cp "$0" "$work/race-runtime.sh"
    XO_DOCKER_ROOT=1 nice -n 10 "$here/docker-linux.sh" "$da" "$work" -- bash /work/race-runtime.sh /work/w /work/out
    a="$work/out/lib/xo/race/$3/libclang_rt.tsan-$an.a"
    [ -s "$a" ] || { echo "race-runtime.sh: Docker build wrote no $a" >&2; exit 1; }
    mkdir -p "$out/lib/xo/race"
    rm -rf "${out:?}/lib/xo/race/$3"
    cp -R "$work/out/lib/xo/race/$3" "$out/lib/xo/race/"
    cp "$work/out/lib/xo/race/LICENSE.TXT" "$out/lib/xo/race/"
    echo "$out/lib/xo/race/$3/libclang_rt.tsan-$an.a"
    exit 0
  fi
fi

case "$(uname -s)-$(uname -m)" in
  Darwin-arm64) goos=darwin goarch=arm64 lib=libclang_rt.tsan_osx_dynamic.dylib target=clang_rt.tsan_osx_dynamic libdir=darwin ;;
  Linux-aarch64) goos=linux goarch=arm64 lib=libclang_rt.tsan-aarch64.a target=clang_rt.tsan-aarch64 libdir=linux ;;
  Linux-x86_64) goos=linux goarch=amd64 lib=libclang_rt.tsan-x86_64.a target=clang_rt.tsan-x86_64 libdir=linux ;;
  *) echo "race-runtime.sh: darwin and linux, arm64 and x86_64 hosts only (decision 0273)" >&2; exit 2 ;;
esac
# darwin-amd64 on an arm64 Mac: the x86_64 dylib, cross built
darch=arm64
if [ "$goos" = darwin ] && [ "${3:-}" = darwin-amd64 ]; then
  goarch=amd64 darch=x86_64
fi
if [ -n "${3:-}" ] && [ "$3" != "$goos-$goarch" ]; then
  echo "race-runtime.sh: cannot build $3 on this host ($goos-$goarch)" >&2; exit 2
fi
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
# the build's own paths (WORKDIR, under the cutter's home directory on the
# Mac) become "." in __FILE__ strings and debug info, so no packed runtime
# names them (seed-pack.sh's XO_SEED_FORBID check; 0.29 to 0.33's darwin
# dylib held 88 of them)
pmap="-ffile-prefix-map=$work=. -fdebug-prefix-map=$work=."
cflags=$pmap
cxxflags=$pmap
platform=()
if [ "$goos" = linux ] && [ "$goarch" = amd64 ]; then
  # no thread safe statics (as arm64's below); initial exec TLS, which
  # Xo's ELF linker relaxes to local exec (sanitizer_coverage's
  # __sancov_lowest_stack was general dynamic: an R_X86_64_TLSGD and a
  # __tls_get_addr call)
  cflags="$cflags -ftls-model=initial-exec"
  cxxflags="$cxxflags -fno-threadsafe-statics -ftls-model=initial-exec"
elif [ "$goos" = linux ]; then
  # no outline atomics: the inline ARMv8.0 loops, so the archive calls no
  # libgcc __aarch64_* helper (Xo's ELF linker synthesizes only some); no
  # thread safe statics: no __cxa_guard_* from a C++ runtime Xo does not
  # link (the runtime initializes once, before threads, in __tsan_init)
  cflags="$cflags -mno-outline-atomics"
  cxxflags="$cxxflags -mno-outline-atomics -fno-threadsafe-statics"
fi
if [ "$goos" = darwin ]; then
  platform=(-DCOMPILER_RT_ENABLE_IOS=OFF -DCOMPILER_RT_ENABLE_WATCHOS=OFF -DCOMPILER_RT_ENABLE_TVOS=OFF
    -DCOMPILER_RT_ENABLE_XROS=OFF -DCMAKE_OSX_ARCHITECTURES=$darch -DDARWIN_osx_ARCHS=$darch
    "-DCMAKE_SHARED_LINKER_FLAGS=-Wl,-oso_prefix,$work/")
  # ld64 records each object's path in the dylib's debug map (N_OSO);
  # -ffile-prefix-map does not reach those, -oso_prefix does (the 0.34.0
  # seed cut's darwin pack held 100+ WORKDIR/race/build paths, 2026-10-29)
fi
cmake -G Ninja -S "$src/compiler-rt" -B "$build" \
  -DCMAKE_BUILD_TYPE=Release -DCMAKE_C_COMPILER=clang -DCMAKE_CXX_COMPILER=clang++ \
  -DCOMPILER_RT_BUILD_SANITIZERS=ON -DCOMPILER_RT_SANITIZERS_TO_BUILD=tsan \
  -DCOMPILER_RT_BUILD_BUILTINS=OFF -DCOMPILER_RT_BUILD_XRAY=OFF -DCOMPILER_RT_BUILD_LIBFUZZER=OFF \
  -DCOMPILER_RT_BUILD_PROFILE=OFF -DCOMPILER_RT_BUILD_MEMPROF=OFF -DCOMPILER_RT_BUILD_ORC=OFF \
  -DCOMPILER_RT_BUILD_CTX_PROFILE=OFF -DCOMPILER_RT_INCLUDE_TESTS=OFF \
  "-DCMAKE_C_FLAGS=$cflags" "-DCMAKE_CXX_FLAGS=$cxxflags" "-DCMAKE_ASM_FLAGS=$pmap" \
  ${platform[@]+"${platform[@]}"} \
  >"$work/configure.txt" 2>&1 || { tail -30 "$work/configure.txt"; exit 1; }
ninja -C "$build" "$target" >"$work/build.txt" 2>&1 || { tail -30 "$work/build.txt"; exit 1; }

dest="$out/lib/xo/race/$goos-$goarch"
mkdir -p "$dest"
cp "$build/lib/$libdir/$lib" "$dest/$lib"
cp "$src/compiler-rt/LICENSE.TXT" "$out/lib/xo/race/LICENSE.TXT"
echo "$dest/$lib"
