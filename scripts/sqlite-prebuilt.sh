#!/bin/bash
# sqlite-prebuilt.sh REPO XO PLAT...: std/sqlite's prebuilt objects
# (decision 0534 stage 2) into REPO/lib/xo/sqlite/PLAT/ (git ignored),
# which release.sh's layout copies: sqlite.o (sqlite.obj for
# windows), its .sha256, and its .key (compiler/xo cffi.xo
# sqlite_key: xo links the object in place of the three sources only
# while the key matches).
# Beside them its build record, .build (xw decision 238): the SQLite
# version (std/native/sqlite/VERSION), the compiler's --version line,
# and for windows the SDK and MSVC versions of XO_WINSYSROOT.
#
# The object is one compile of sqlite3.c, xo_sqlite.c and the binding's
# `use c` adapter (std/native/.xo/cbind/c_xo_sqlite_*.c, which XO writes
# when it binds the header: a build of a std/sqlite program first).
#   darwin-arm64, darwin-amd64: Apple's clang (-arch), on the Mac;
#   linux-arm64, linux-amd64: on the Mac (release.sh), Debian's clang in
#     the linux/arm64 Docker image (scripts/docker-linux.sh, native, no
#     emulation) with --target for each, against Debian's glibc headers
#     (libc6-dev-amd64-cross for amd64; task #622); elsewhere
#     $XO_SQLITE_CC (a cc for that target, run on its offload host), else
#     cc when the host is that target;
#   windows-amd64: $XO_CLANG with $XO_WINSYSROOT (xwin's sysroot), with
#     the addendum's flags: the object then needs no C runtime, only
#     KERNEL32 and the C functions cabi_wincrt.xo and cabi_winmath.xo
#     define (Xo's PE linker);
#   windows-arm64: the same for aarch64-pc-windows-msvc (decision 0569
#     stage 6, owner 2026-11-15), with -fno-stack-protector as xo's own
#     windows/arm64 `use c` compile (compiler/xo main.xo).
# darwin and linux keep a checkout's flags (-g -O2 and the features), so
# a prebuilt and a compiled program behave alike.
# Their debug information names the checkout as . (-ffile-prefix-map): the
# 0.44.0 build's objects held the release checkout's path under $HOME
# (gotchas/debug-map-names-cache-paths.md).
set -euo pipefail
repo=${1:?usage: sqlite-prebuilt.sh REPO XO PLAT...}
xo=${2:?usage: sqlite-prebuilt.sh REPO XO PLAT...}
shift 2
src="$repo/std/native/sqlite"
# the SQLite features every object is compiled with: compiler/xo cffi.xo
# sqlite_feature_defines, which xo's own compile of sqlite3.c and the
# object's key use (xw decision 251)
features=(-DSQLITE_ENABLE_FTS5 -DSQLITE_ENABLE_PERCENTILE -DSQLITE_ENABLE_MATH_FUNCTIONS)
work=$(mktemp -d)
trap 'rm -rf "${work:?}"' EXIT

# the adapter: bind the header with XO (a type check of a std/sqlite program)
mkdir -p "$work/p"
printf 'use std/sqlite\n\nfn main() {\n  let _ = sqlite.driver\n}\n' > "$work/p/main.xo"
(cd "$work/p" && XO_STD="$repo/std" "$xo" check . >/dev/null) || true
adapters=("$repo"/std/native/.xo/cbind/c_xo_sqlite_*.c)
if [ "${#adapters[@]}" != 1 ] || [ ! -f "${adapters[0]}" ]; then
	echo "sqlite-prebuilt.sh: want one std/native/.xo/cbind/c_xo_sqlite_*.c, have: ${adapters[*]}" >&2
	exit 1
fi
adapter=${adapters[0]}

printf '#include "%s/sqlite3.c"\n#include "%s/xo_sqlite.c"\n#include "%s"\n' "$src" "$src" "$adapter" > "$work/unit.c"

for plat in "$@"; do
	goos=${plat%-*}
	arch=${plat#*-}
	out="$repo/lib/xo/sqlite/$plat"
	mkdir -p "$out"
	ext=o
	docker=0
	case $plat in
	darwin-arm64) cmd=(cc -arch arm64 -g -O2 "-ffile-prefix-map=$repo=.") ;;
	darwin-amd64) cmd=(cc -arch x86_64 -g -O2 "-ffile-prefix-map=$repo=.") ;;
	linux-arm64|linux-amd64)
		if [ "$(uname -s)" = Darwin ] && [ -z "${XO_SQLITE_CC:-}" ]; then
			# docker-linux.sh mounts REPO at /xo and the work directory at /work
			docker=1
			tgt=$([ "$arch" = arm64 ] && echo aarch64 || echo x86_64)-linux-gnu
			cmd=(clang "--target=$tgt" -g -O2 -fPIC "-ffile-prefix-map=/xo=.")
			# amd64's glibc headers are the cross package's, not the image's
			# own /usr/include (arm64's)
			[ "$arch" = amd64 ] && cmd+=(-nostdlibinc -isystem /usr/x86_64-linux-gnu/include)
		else
			cmd=(${XO_SQLITE_CC:-cc} -g -O2 -fPIC "-ffile-prefix-map=$repo=.")
		fi
		;;
	# the windows defines are compiler/xo cffi.xo sqlite_win_defines', which
	# a windows/arm64 build from source uses
	windows-amd64)
		ext=obj
		cmd=("${XO_CLANG:?windows-amd64 needs XO_CLANG}" --target=x86_64-pc-windows-msvc -Xmicrosoft-windows-sys-root "${XO_WINSYSROOT:?windows-amd64 needs XO_WINSYSROOT}" -O2 -DSQLITE_THREADSAFE=2 -DSQLITE_OMIT_LOAD_EXTENSION -DSQLITE_WIN32_MALLOC=1 -DSQLITE_MAX_WORKER_THREADS=0 -DSQLITE_OMIT_SEH)
		;;
	windows-arm64)
		ext=obj
		cmd=("${XO_CLANG:?windows-arm64 needs XO_CLANG}" --target=aarch64-pc-windows-msvc -Xmicrosoft-windows-sys-root "${XO_WINSYSROOT:?windows-arm64 needs XO_WINSYSROOT}" -fno-stack-protector -O2 -DSQLITE_THREADSAFE=2 -DSQLITE_OMIT_LOAD_EXTENSION -DSQLITE_WIN32_MALLOC=1 -DSQLITE_MAX_WORKER_THREADS=0 -DSQLITE_OMIT_SEH)
		;;
	*) echo "sqlite-prebuilt.sh: no recipe for $plat" >&2; exit 1 ;;
	esac
	obj="$out/sqlite.$ext"
	echo "+ ${cmd[*]} -I $src -c unit.c -o $obj" >&2
	# the adapter includes "sqlite/xo_sqlite.h", relative to std/native
	# (compiler/cli/bind std_key)
	if [ "$docker" = 1 ]; then
		[ "$(cd "$(dirname "$0")/.." && pwd)" = "$(cd "$repo" && pwd)" ] || { echo "sqlite-prebuilt.sh: a linux object from the Mac needs REPO to be this script's checkout (docker-linux.sh mounts it)" >&2; exit 1; }
		printf '#include "/xo/std/native/sqlite/sqlite3.c"\n#include "/xo/std/native/sqlite/xo_sqlite.c"\n#include "/xo/%s"\n' "${adapter#"$repo"/}" > "$work/dunit.c"
		dev=libc6-dev; [ "$arch" = amd64 ] && dev=libc6-dev-amd64-cross
		rm -f "$work/sqlite.o" "$work/cc-version"
		XO_DOCKER_ROOT=1 nice -n 10 "$(dirname "$0")/docker-linux.sh" arm64 "$work" -- "(apt-get update -qq && apt-get install -y -qq --no-install-recommends $dev) >/dev/null 2>&1 && clang --version | head -1 > /work/cc-version && ${cmd[*]} ${features[*]} -I /xo/std/native/sqlite -I /xo/std/native -c /work/dunit.c -o /work/sqlite.o"
		[ -s "$work/sqlite.o" ] || { echo "sqlite-prebuilt.sh: the Docker compile wrote no $plat object" >&2; exit 1; }
	else
		"${cmd[@]}" "${features[@]}" -I "$src" -I "$repo/std/native" -c "$work/unit.c" -o "$work/sqlite.$ext"
	fi
	mv "$work/sqlite.$ext" "$obj"
	(cd "$out" && shasum -a 256 "sqlite.$ext" > "sqlite.$ext.sha256")
	python3 -I - "$src" "$adapter" "$goos" "$arch" "${features[*]}" > "$obj.key" <<'EOF'
import hashlib, sys
d, adapter, goos, arch, features = sys.argv[1:6]
b = f'xo sqlite prebuilt v2 {goos}/{arch} {features}\n'.encode()
for f in [f'{d}/sqlite3.c', f'{d}/xo_sqlite.c', f'{d}/xo_sqlite.h', adapter]:
    data = open(f, 'rb').read()
    if f == adapter:
        data = data.replace(f'{d}/'.encode(), b'')
    b += f'{len(data)}\n'.encode() + data
print(hashlib.sha256(b).hexdigest())
EOF
	if [ "$goos" = windows ]; then
		# every symbol the object needs is an import of compiler/macho
		# peimports.xo (__imp_ too), a C function cabi_wincrt.xo or
		# cabi_winmath.xo defines,
		# or the runtime's (xo_x_*): a SQLite update that needs another
		# fails here, not at a user's link (decision 0534 item 5)
		nm="$(dirname "$XO_CLANG")/llvm-nm"
		"$nm" -u "$obj" | awk '{print $NF}' | python3 -I -c '
import re, sys
repo = sys.argv[1]
imports = set(re.findall(r"^\s*\"([A-Za-z0-9_]+)\": \"", open(repo + "/compiler/macho/peimports.xo").read(), re.M))
# cabi_winmath.xo: the C math library, which SQLITE_ENABLE_MATH_FUNCTIONS
# calls (task #621: the 0.56.0 recut failed on acos through trunc)
crt = set()
for f in ("cabi_wincrt.xo", "cabi_winmath.xo"):
    crt |= set(re.findall(r"^fn ([A-Za-z_][A-Za-z0-9_]*)\(", open(repo + "/std/native/llrt/" + f).read(), re.M))
bad = []
for s in sys.stdin.read().split():
    n = s[len("__imp_"):] if s.startswith("__imp_") else s
    # _fltused: listed by clang, named by no relocation; __chkstk: the
    # amd64 emitter defines it (compiler/arm64 x86win.xo w64_chkstk), and
    # on arm64 every link has it and the libgcc outline atomics
    # (__aarch64_*), compiler/xo link.xo win_arm64_atomics
    arm = sys.argv[2] == "windows-arm64"
    if n in imports or n in crt or n.startswith("xo_x_") or n in ("_fltused", "__chkstk") or (arm and n.startswith("__aarch64_")):
        continue
    bad.append(s)
if bad:
    sys.exit("sqlite-prebuilt.sh: " + sys.argv[2] + " object needs symbols nothing defines: " + " ".join(bad))
' "$repo" "$plat"
	fi
	# the build record beside the object (owner, 2026-11-20, xw decision
	# 238): what was compiled and with what, for whoever asks which
	# SQLite and which toolchain a release's object came from. No paths
	# (a release host's directories are private) and no date (two builds
	# of one input give one record).
	{
		echo "object: sqlite.$ext ($plat)"
		echo "sqlite: $(sed -n '1s/^SQLite \([0-9.]*\).*/\1/p' "$src/VERSION")"
		echo "sqlite3.c sha256: $(shasum -a 256 "$src/sqlite3.c" | cut -d' ' -f1)"
		echo "key: $(cat "$obj.key")"
		if [ "$docker" = 1 ]; then
			echo "compiler: $(cat "$work/cc-version") (docker-linux.sh, --target=$tgt)"
		else
			echo "compiler: $("${cmd[0]}" --version 2>/dev/null | head -1)"
		fi
		if [ "$goos" = windows ]; then
			echo "windows sdk: $(ls "$XO_WINSYSROOT/Windows Kits/10/Include" 2>/dev/null | sort -V | tail -1)"
			echo "msvc: $(ls "$XO_WINSYSROOT/VC/Tools/MSVC" 2>/dev/null | sort -V | tail -1)"
		fi
	} > "$obj.build"
	if grep -q ': *$' "$obj.build"; then
		echo "sqlite-prebuilt.sh: $plat: a line of the build record is empty:" >&2
		cat "$obj.build" >&2
		exit 1
	fi
	echo "$plat: $obj ($(wc -c < "$obj") bytes), key $(cat "$obj.key")" >&2
done
