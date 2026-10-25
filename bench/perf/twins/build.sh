#!/usr/bin/env bash
# twins/build.sh: what it costs to build, start and ship each language's twin
# of the users CRUD server and of a hello program, measured the way
# scripts/release-vs-go.sh measures Go and Xo:
#
#   - build: cold (empty caches), second (again, unchanged), warm (the third
#     unchanged build) and edit (one new function appended to the main file),
#     wall time and peak RSS of the build (/usr/bin/time -l), one build each:
#       xo      xo build (XO_BACKEND_CACHE_DIR empty for cold)
#       go      go build -trimpath (GOCACHE empty for cold)
#       rust    cargo build --release --locked --offline (CARGO_TARGET_DIR
#               empty for cold; the crates are downloaded first, not timed)
#       node    tsc -p . (the whole bench/perf/node project; tsc keeps no
#               cache, so every build is a full one) and esbuild bundling
#               dist/http/server.js into one file
#       python  no build step: the first import of the server module with no
#               __pycache__ (CPython compiles it to bytecode then), again, and
#               after an edit; the installed packages were compiled at install
#   - size of what ships: the binary (xo, go, rust), each as built by the
#     build rows (xo build and go build keep DWARF, cargo --release has
#     none) and again with the shipping flags (xo build --release, go build
#     -ldflags="-s -w", cargo --release), so the sizes compare like with
#     like; for Node the esbuild
#     bundle, Node's own binary, and a single executable application (the
#     bundle injected into a copy of the official Node binary with postject,
#     signed ad hoc); for Python the virtual environment's site-packages and
#     the interpreter's installation
#   - startup: `xo bench --ab` of the hello program, A = the other language,
#     B = Xo, wall and max RSS (Node runs dist/hello/hello.js, Python
#     hello.py)
#
# Writes bench/results/twins-build-DATE.md. Environment: XO (default
# .tools/bin/xo; XO_BENCH_ANY=1 for a released archive's xo), OUT (a
# directory for copies, caches and logs), RESULT, ROUNDS (startup rounds,
# default 20), MAXLOAD (default 5), NODE_SEA (an official Node binary of the
# same version as `node`; default: downloaded from nodejs.org into OUT and
# checked against its SHASUMS256.txt). Needs bench/perf/node's npm ci and
# bench/perf/python's .venv (see python/run.sh).
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
perf=$root/bench/perf
xo=${XO:-$root/.tools/bin/xo}
xo=$(cd "$(dirname "$xo")" && pwd)/$(basename "$xo")
rounds=${ROUNDS:-20}
maxload=${MAXLOAD:-5}
out=${OUT:-$(mktemp -d)}
mkdir -p "$out"
out=$(cd "$out" && pwd)
date=$(date +%F)
result=${RESULT:-$root/bench/results/twins-build-$date.md}
cargo=${CARGO:-$(command -v cargo || echo "$HOME/.cargo/bin/cargo")}
py="$perf/python/.venv/bin/python"
[ -x "$py" ] || { echo "twins/build.sh: no $py (see python/run.sh)" >&2; exit 1; }
[ -d "$perf/node/node_modules" ] || { echo "twins/build.sh: run npm ci in bench/perf/node" >&2; exit 1; }
. "$perf/xocheck.sh" "$xo" "$root"
. "$here/lib.sh"
work=$out/work
mkdir -p "$work"

# timed LOG CMD...: runs CMD under /usr/bin/time -l; prints "ms peak_mb"
timed() {
	local log=$1
	shift
	/usr/bin/time -l "$@" >"$log" 2>&1 || { echo "twins/build.sh: failed: $* (log $log)" >&2; tail -20 "$log" >&2; exit 1; }
	local ms rss
	ms=$(awk '$2 == "real" {printf "%d", $1 * 1000}' "$log" | tail -1)
	rss=$(awk '/maximum resident set size/ {print $1}' "$log" | tail -1)
	echo "${ms:-0} $((${rss:-0} / 1048576))"
}
# mb BYTES: megabytes with two decimals, or kilobytes under 0.1 MB
mb() { python3 -c "b=$1; print(f'{b/1048576:.2f}' if b >= 104858 else f'{b/1024:.1f} KB')"; }
dusize() { du -sk "$1" | awk '{print $1 * 1024}'; }

# row NAME SIDE COLD SECOND WARM EDIT SIZE: a table row from four "ms mb" pairs
row() {
	local name=$1 side=$2 size=$7
	set -- $3 $4 $5 $6
	echo "| $name | $side | $1 | $3 | $5 | $7 | $2 / $6 / $8 | $size |"
}

# measure NAME SIDE DIR MAINFILE EDIT BIN CMD...: cold, second, warm and edit
# builds of a copy of DIR (CMD runs in the copy; the caller has pointed its
# caches into $work/cache-SIDE-NAME); BIN is the output, relative to the copy
measure() {
	local name=$1 side=$2 src=$3 mainf=$4 edit=$5 bin=$6
	shift 6
	local d=$work/src-$side-$name
	rm -rf "${d:?}"
	cp -R "$src" "$d"
	rm -rf "${d:?}/target" "${d:?}/__pycache__"
	local cold second warm ed
	wait_load
	cold=$(cd "$d" && timed "$out/build-$side-$name-cold.log" "$@")
	second=$(cd "$d" && timed "$out/build-$side-$name-second.log" "$@")
	warm=$(cd "$d" && timed "$out/build-$side-$name-warm.log" "$@")
	cp "$d/$mainf" "$d/$mainf.orig"
	printf '%b' "$edit" >>"$d/$mainf"
	ed=$(cd "$d" && timed "$out/build-$side-$name-edit.log" "$@")
	mv "$d/$mainf.orig" "$d/$mainf"
	(cd "$d" && "$@" >/dev/null 2>&1)
	local size=n/a
	[ "$bin" = - ] || size=$(mb "$(stat -f %z "$d/$bin")")
	row "$name" "$side" "$cold" "$second" "$warm" "$ed" "$size"
}

xo_edit='\nfn twins_build_edit() -> Int {\n  7\n}\n'
go_edit='\nfunc twinsBuildEdit() int {\n\treturn 7\n}\n'
rust_edit='\n#[allow(dead_code)]\nfn twins_build_edit() -> i64 {\n    7\n}\n'
ts_edit='\nexport function twinsBuildEdit(): number {\n  return 7;\n}\n'
py_edit='\n\ndef twins_build_edit():\n    return 7\n'

xobuild() { local c=$work/cache-xo-$1; rm -rf "${c:?}"; mkdir -p "$c"; echo env XO_BACKEND_CACHE_DIR="$c/backend" "$xo" build -o prog .; }
gobuild() { local c=$work/cache-go-$1; rm -rf "${c:?}"; mkdir -p "$c"; echo env GOCACHE="$c/go" GOFLAGS=-trimpath go build -o prog .; }
rustbuild() { local c=$work/cache-rust-$1; rm -rf "${c:?}"; echo env CARGO_TARGET_DIR="$c" "$cargo" build --release --locked --offline -q; }

body=$out/body.md
: >"$body"
{
	echo "## Build and size"
	echo
	echo "Wall milliseconds of one build; peak MB is the build's largest resident set (cold / warm / edit); size MB is the output as built (a binary, with DWARF for Xo and Go, none for Rust; the shipping-flag sizes are in What ships; Node's tsc output directory or esbuild bundle; Python has none)."
	echo
	echo "| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB |"
	echo "|---|---|---|---|---|---|---|---|"
} >>"$body"

# Xo, Go and Rust: the CRUD server and hello
for p in "crud $perf/http/xo main.xo" "hello $here/hello/xo main.xo"; do
	set -- $p
	measure "$1" xo "$2" "$3" "$xo_edit" prog $(xobuild "$1") >>"$body"
done
for p in "crud $perf/http/go main.go" "hello $here/hello/go main.go"; do
	set -- $p
	measure "$1" go "$2" "$3" "$go_edit" prog $(gobuild "$1") >>"$body"
done
(cd "$perf/rust/http" && "$cargo" fetch --locked -q)
for p in "crud $perf/rust/http crudperf" "hello $here/hello/rust hello"; do
	set -- $p
	measure "$1" rust "$2" src/main.rs "$rust_edit" "../cache-rust-$1/release/$3" $(rustbuild "$1") >>"$body"
done

# Node: tsc over the project, then esbuild's bundle of the server
nd=$work/src-node
rm -rf "${nd:?}"
cp -R "$perf/node" "$nd"
rm -rf "${nd:?}/dist"
tsc_row=$(measure project node "$nd" src/http/server.ts "$ts_edit" dist/http/server.js npx --no-install tsc -p .)
tsc_size=$(mb "$(dusize "$work/src-node-project/dist")")
echo "$tsc_row" | sed "s/| [^|]* |\$/| $tsc_size (dist) |/" >>"$body"
(cd "$nd" && npx --no-install tsc -p .)
measure bundle node "$nd" dist/http/server.js '\nexport function twinsBuildEdit() { return 7; }\n' server.bundle.js \
	npx --no-install esbuild dist/http/server.js --bundle --platform=node --minify --outfile=server.bundle.js --log-level=warning >>"$body"
bundle=$work/src-node-bundle/server.bundle.js

# Python: the first import compiles the module to bytecode
pd=$perf/python/http
for m in server_asyncio server_starlette; do
	measure "$m" python "$pd" "$m.py" "$py_edit" - env PYTHONPATH=. "$py" -c "import $m" >>"$body"
done

# Node single executable application from the official binary
nodever=$(node --version)
if [ -z "${NODE_SEA:-}" ]; then
	dl=$out/node-dist
	mkdir -p "$dl"
	tgz=node-$nodever-darwin-arm64.tar.gz
	curl -fsSL -o "$dl/$tgz" "https://nodejs.org/dist/$nodever/$tgz"
	curl -fsSL -o "$dl/SHASUMS256.txt" "https://nodejs.org/dist/$nodever/SHASUMS256.txt"
	(cd "$dl" && grep " $tgz\$" SHASUMS256.txt | shasum -a 256 -c - >/dev/null)
	tar -xzf "$dl/$tgz" -C "$dl"
	NODE_SEA=$dl/node-$nodever-darwin-arm64/bin/node
fi
sea=$work/sea
rm -rf "${sea:?}"
mkdir -p "$sea"
cp "$bundle" "$sea/server.js"
printf '{ "main": "server.js", "output": "sea-prep.blob", "disableExperimentalSEAWarning": true }\n' >"$sea/sea-config.json"
sea_build() {
	"$NODE_SEA" --experimental-sea-config sea-config.json >/dev/null
	cp "$NODE_SEA" crud-sea
	chmod u+w crud-sea
	codesign --remove-signature crud-sea
	"$perf/node/node_modules/.bin/postject" crud-sea NODE_SEA_BLOB sea-prep.blob --sentinel-fuse NODE_SEA_FUSE_fce680ab2cc467b6e072b8b5df1996b2 --macho-segment-name NODE_SEA >/dev/null
	codesign --sign - crud-sea
}
wait_load
sea_t=$(cd "$sea" && timed "$out/build-node-sea.log" bash -c "$(declare -f sea_build); NODE_SEA='$NODE_SEA'; perf='$perf'; sea_build")
set -- $sea_t
{
	echo
	echo "Node single executable application (the bundle injected into a copy of the official node $nodever binary with postject, signed ad hoc): $1 ms, peak $2 MB, $(mb "$(stat -f %z "$sea/crud-sea")") MB."
} >>"$body"

# the shipping flags: Xo --release and Go -s -w drop the debug info the
# plain builds keep (Rust's release profile has none); built here into the
# copies the build rows used, with their warm caches (the time is not measured)
for n in crud hello; do
	(cd "$work/src-xo-$n" && env XO_BACKEND_CACHE_DIR="$work/cache-xo-$n/backend" "$xo" build --release -o prog-release . >/dev/null)
	(cd "$work/src-go-$n" && env GOCACHE="$work/cache-go-$n/go" go build -trimpath -ldflags="-s -w" -o prog-stripped . >/dev/null)
done

# sizes of what ships
pyprefix=$("$py" -c 'import sys; print(sys.base_prefix)')
site=$("$py" -c 'import sysconfig; print(sysconfig.get_paths()["purelib"])')
{
	echo
	echo "## What ships"
	echo
	echo "| side | what | MB |"
	echo "|---|---|---|"
	echo "| xo | the CRUD server binary, \`xo build\` (with DWARF) | $(mb "$(stat -f %z "$work/src-xo-crud/prog")") |"
	echo "| xo | the CRUD server binary, \`xo build --release\` | $(mb "$(stat -f %z "$work/src-xo-crud/prog-release")") |"
	echo "| go | the CRUD server binary, \`go build -trimpath\` (with DWARF) | $(mb "$(stat -f %z "$work/src-go-crud/prog")") |"
	echo "| go | the CRUD server binary, \`go build -trimpath -ldflags=\"-s -w\"\` | $(mb "$(stat -f %z "$work/src-go-crud/prog-stripped")") |"
	echo "| rust | the CRUD server binary, \`cargo build --release\` (no debug info) | $(mb "$(stat -f %z "$work/cache-rust-crud/release/crudperf")") |"
	echo "| xo | the hello binary, \`xo build\` (with DWARF) | $(mb "$(stat -f %z "$work/src-xo-hello/prog")") |"
	echo "| xo | the hello binary, \`xo build --release\` | $(mb "$(stat -f %z "$work/src-xo-hello/prog-release")") |"
	echo "| go | the hello binary, \`go build -trimpath\` (with DWARF) | $(mb "$(stat -f %z "$work/src-go-hello/prog")") |"
	echo "| go | the hello binary, \`go build -trimpath -ldflags=\"-s -w\"\` | $(mb "$(stat -f %z "$work/src-go-hello/prog-stripped")") |"
	echo "| rust | the hello binary, \`cargo build --release\` (no debug info) | $(mb "$(stat -f %z "$work/cache-rust-hello/release/hello")") |"
	echo "| node | the esbuild bundle of the server (needs a node runtime) | $(mb "$(stat -f %z "$bundle")") |"
	echo "| node | the official node $nodever binary (the runtime) | $(mb "$(stat -f %z "$NODE_SEA")") |"
	echo "| node | the single executable application | $(mb "$(stat -f %z "$sea/crud-sea")") |"
	echo "| python | the server sources (http/*.py) | $(mb "$(cat "$pd"/*.py | wc -c)") |"
	echo "| python | the virtual environment's site-packages (uvicorn, Starlette, uvloop, httptools and their dependencies) | $(mb "$(dusize "$site")") |"
	echo "| python | the interpreter's installation ($pyprefix) | $(mb "$(dusize "$pyprefix")") |"
} >>"$body"

# startup
(cd "$here/hello/xo" && "$xo" build -o "$work/hello-xo" . >/dev/null)
cp "$work/src-go-hello/prog" "$work/hello-go"
cp "$work/cache-rust-hello/release/hello" "$work/hello-rust"
wrapper "$work/hello-node" node "$nd/dist/hello/hello.js"
wrapper "$work/hello-python" "$py" "$perf/python/hello.py"
{
	echo
	echo "## Startup (hello, \`xo bench --ab\`, $rounds rounds, A = the other language, B = Xo)"
	echo
} >>"$body"
for s in go rust node python; do
	agree "hello $s" "$work/hello-$s" "$work/hello-xo"
	{
		echo "| hello, $s | load $(load) |"
		wait_load
		"$xo" bench --ab --rounds="$rounds" "$work/hello-$s" "$work/hello-xo" 2>&1 | grep -E '^(wall_ms|cpu_ms|max_rss_kb)' | sed 's/^/    /'
	} >>"$body"
done

{
	echo "# Build, size and startup of the language twins: $date"
	echo
	echo "Written by bench/perf/twins/build.sh. The CRUD server is bench/perf/http's users API in each language (Xo std/http, Go net/http, Rust axum on tokio, Node node:http, Python asyncio.Protocol and Starlette on uvicorn); hello prints one line."
	echo
	echo "## Machine and versions"
	echo
	echo "- $(uname -srm), $(sysctl -n machdep.cpu.brand_string), $(sysctl -n hw.ncpu) CPUs, $(sysctl -n hw.memsize | awk '{printf "%d GB", $1/1073741824}'), load average at start $(load)"
	echo "- $XO_STAMP"
	echo "- $(go version)"
	echo "- $("$cargo" --version), $(rustc --version 2>/dev/null || "$HOME/.cargo/bin/rustc" --version)"
	echo "- node $nodever (Homebrew, for tsc, esbuild and the startup row), typescript $(cd "$perf/node" && npx --no-install tsc --version | sed 's/Version //'), esbuild $(cd "$perf/node" && npx --no-install esbuild --version), pinned in package-lock.json"
	echo "- $("$py" --version) (the virtual environment of bench/perf/python)"
	echo
	cat "$body"
} >"$result"
echo "wrote $result"
