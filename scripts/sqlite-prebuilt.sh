#!/bin/bash
# sqlite-prebuilt.sh REPO XO PLAT...: std/sqlite's prebuilt objects
# (decision 0534 stage 2) into REPO/lib/xo/sqlite/PLAT/ (git ignored),
# which release.sh's layout copies: sqlite.o (sqlite.obj for
# windows-amd64), its .sha256, and its .key (compiler/xo cffi.xo
# sqlite_key: xo links the object in place of the three sources only
# while the key matches).
#
# The object is one compile of sqlite3.c, xo_sqlite.c and the binding's
# `use c` adapter (std/native/.xo/cbind/c_xo_sqlite_*.c, which XO writes
# when it binds the header: a build of a std/sqlite program first).
#   darwin-arm64, darwin-amd64: Apple's clang (-arch), on the Mac;
#   linux-arm64, linux-amd64: $XO_SQLITE_CC (a cc for that target, run on
#     its offload host), else cc when the host is that target;
#   windows-amd64: $XO_CLANG with $XO_WINSYSROOT (xwin's sysroot), with
#     the addendum's flags: the object then needs no C runtime, only
#     KERNEL32 and the C functions cabi_wincrt.xo defines (Xo's PE linker).
# darwin and linux keep a checkout's flags (-g -O2, SQLite's defaults), so
# a prebuilt and a compiled program behave alike.
set -euo pipefail
repo=${1:?usage: sqlite-prebuilt.sh REPO XO PLAT...}
xo=${2:?usage: sqlite-prebuilt.sh REPO XO PLAT...}
shift 2
src="$repo/std/native/sqlite"
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
	case $plat in
	darwin-arm64) cmd=(cc -arch arm64 -g -O2) ;;
	darwin-amd64) cmd=(cc -arch x86_64 -g -O2) ;;
	linux-arm64|linux-amd64) cmd=(${XO_SQLITE_CC:-cc} -g -O2 -fPIC) ;;
	windows-amd64)
		ext=obj
		cmd=("${XO_CLANG:?windows-amd64 needs XO_CLANG}" --target=x86_64-pc-windows-msvc -Xmicrosoft-windows-sys-root "${XO_WINSYSROOT:?windows-amd64 needs XO_WINSYSROOT}" -O2 -DSQLITE_THREADSAFE=2 -DSQLITE_OMIT_LOAD_EXTENSION -DSQLITE_WIN32_MALLOC=1 -DSQLITE_MAX_WORKER_THREADS=0 -DSQLITE_OMIT_SEH)
		;;
	*) echo "sqlite-prebuilt.sh: no recipe for $plat" >&2; exit 1 ;;
	esac
	obj="$out/sqlite.$ext"
	echo "+ ${cmd[*]} -I $src -c unit.c -o $obj" >&2
	# the adapter includes "sqlite/xo_sqlite.h", relative to std/native
	# (compiler/cli/bind std_key)
	"${cmd[@]}" -I "$src" -I "$repo/std/native" -c "$work/unit.c" -o "$work/sqlite.$ext"
	mv "$work/sqlite.$ext" "$obj"
	(cd "$out" && shasum -a 256 "sqlite.$ext" > "sqlite.$ext.sha256")
	python3 -I - "$src" "$adapter" "$goos" "$arch" > "$obj.key" <<'EOF'
import hashlib, sys
d, adapter, goos, arch = sys.argv[1:5]
b = f'xo sqlite prebuilt v1 {goos}/{arch}\n'.encode()
for f in [f'{d}/sqlite3.c', f'{d}/xo_sqlite.c', f'{d}/xo_sqlite.h', adapter]:
    data = open(f, 'rb').read()
    if f == adapter:
        data = data.replace(f'{d}/'.encode(), b'')
    b += f'{len(data)}\n'.encode() + data
print(hashlib.sha256(b).hexdigest())
EOF
	if [ "$plat" = windows-amd64 ]; then
		# every symbol the object needs is an import of compiler/macho
		# peimports.xo (__imp_ too), a C function cabi_wincrt.xo defines,
		# or the runtime's (xo_x_*): a SQLite update that needs another
		# fails here, not at a user's link (decision 0534 item 5)
		nm="$(dirname "$XO_CLANG")/llvm-nm"
		"$nm" -u "$obj" | awk '{print $NF}' | python3 -I -c '
import re, sys
repo = sys.argv[1]
imports = set(re.findall(r"^\s*\"([A-Za-z0-9_]+)\": \"", open(repo + "/compiler/macho/peimports.xo").read(), re.M))
crt = set(re.findall(r"^fn ([A-Za-z_][A-Za-z0-9_]*)\(", open(repo + "/std/native/llrt/cabi_wincrt.xo").read(), re.M))
bad = []
for s in sys.stdin.read().split():
    n = s[len("__imp_"):] if s.startswith("__imp_") else s
    # _fltused: listed by clang, named by no relocation; __chkstk: the
    # emitter defines it (compiler/arm64 x86win.xo w64_chkstk)
    if n in imports or n in crt or n.startswith("xo_x_") or n in ("_fltused", "__chkstk"):
        continue
    bad.append(s)
if bad:
    sys.exit("sqlite-prebuilt.sh: windows-amd64 object needs symbols nothing defines: " + " ".join(bad))
' "$repo"
	fi
	echo "$plat: $obj ($(wc -c < "$obj") bytes), key $(cat "$obj.key")" >&2
done
