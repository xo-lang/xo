#!/bin/sh
# CPython 3.13 against Xo, and Go for the server rows: the twins of
# bench/perf's rows in Python's own idiom (asyncio tasks, threading,
# multiprocessing, the json module), run from a virtual environment of this
# directory (.venv, made from the Homebrew python3.13 by `python3.13 -m venv
# .venv` and `pip install --require-hashes -r requirements.txt`; the system
# python3 is not used and nothing outside .venv is changed).
#
#   compute   compute/fib.py against llvm/xo/fib
#   conc      conc/conc.py against conc/xo: spawn fanout pingpong (asyncio
#             tasks), mutex (threading.Lock under the GIL), pmap
#             (multiprocessing, one process per CPU)
#   json      json/jsontwin.py against jsontwin/xo, Go's jsontwin as a reference
#   http      http/server_asyncio.py (asyncio.Protocol, standard library) and
#             http/server_starlette.py (Starlette on uvicorn with httptools and
#             uvloop), one process each, against the Go and Xo CRUD servers,
#             through bench/perf/http/run.sh
#
# A Python program takes seconds where the others take milliseconds, so the
# slow rows (fib, pmap) run SLOW_ROUNDS rounds (default 3) instead of ROUNDS.
# See bench/perf/twins/lib.sh and bench/perf/rust/run.sh. Writes
# bench/results/python-vs-xo-DATE.md.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
xo=${XO:-$root/.tools/bin/xo}
rounds=${ROUNDS:-10}
slow=${SLOW_ROUNDS:-3}
maxload=${MAXLOAD:-5}
rows=${ROWS:-compute conc json http}
out=${OUT:-$(mktemp -d)}
mkdir -p "$out"
out=$(cd "$out" && pwd)
date=$(date +%F)
result=${RESULT:-$root/bench/results/python-vs-xo-$date.md}
py="$here/.venv/bin/python"
[ -x "$py" ] || { echo "python/run.sh: no $py (see this file's header)" >&2; exit 1; }
. "$root/bench/perf/xocheck.sh" "$xo" "$root"
. "$here/../twins/lib.sh"
header "Python" "$("$py" --version) (Homebrew python3.13, virtual environment of bench/perf/python)" "packages (requirements.txt, hashes): $(grep -E '^(uvicorn|starlette|httptools|uvloop)==' "$here/requirements.txt" | sed 's/ .*//' | tr '\n' ' ')" > "$out/header.md"
: > "$out/body.md"
for r in $rows; do
	case $r in
	compute)
		(cd "$here/../llvm/xo/fib" && "$xo" build -o "$out/fib-xo" . >/dev/null)
		wrapper "$out/fib-py" "$py" "$here/compute/fib.py"
		echo "## Compute (recursive fib 40, $slow rounds)" >> "$out/body.md"; echo >> "$out/body.md"
		agree fib "$out/fib-py" "$out/fib-xo"
		(rounds=$slow; ab fib "$out/fib-py" "$out/fib-xo" >> "$out/body.md") ;;
	conc)
		wrapper "$out/conc-py" "$py" "$here/conc/conc.py"
		echo >> "$out/body.md"; echo "## Concurrency (asyncio, threading, multiprocessing; no rwmutex row: the standard library has no read write lock)" >> "$out/body.md"; echo >> "$out/body.md"
		for w in spawn fanout pingpong mutex pmap; do
			(cd "$here/../conc/xo/$w" && "$xo" build -o "$out/conc-xo-$w" . >/dev/null)
			agree "$w" "$out/conc-py" "$out/conc-xo-$w" "$w"
			case $w in pmap) (rounds=$slow; ab "$w" "$out/conc-py" "$out/conc-xo-$w" "$w" >> "$out/body.md") ;; *) ab "$w" "$out/conc-py" "$out/conc-xo-$w" "$w" >> "$out/body.md" ;; esac
		done ;;
	json)
		wrapper "$out/json-py" "$py" "$here/json/jsontwin.py"
		(cd "$here/../jsontwin/go" && go build -o "$out/json-go" .)
		(cd "$here/../jsontwin/xo" && "$xo" build -o "$out/json-xo" . >/dev/null)
		echo >> "$out/body.md"; echo "## JSON (json.loads and json.dumps against std/json; 1000 users, 100 rounds)" >> "$out/body.md"; echo >> "$out/body.md"
		for w in decode encode; do
			agree "json $w" "$out/json-py" "$out/json-xo" "$w"
			ab "json $w, Python" "$out/json-py" "$out/json-xo" "$w" >> "$out/body.md"
			ab "json $w, Go" "$out/json-go" "$out/json-xo" "$w" >> "$out/body.md"
		done ;;
	http)
		wait_load
		echo >> "$out/body.md"; echo "## HTTP: the users CRUD API (asyncio.Protocol and Starlette on uvicorn, one process each; Go net/http; Xo std/http)" >> "$out/body.md"; echo >> "$out/body.md"
		echo '```' >> "$out/body.md"
		PERF_OUT="$out/crud" CONFIGS="go xo-arenas python-asyncio python-starlette" XO="$xo" sh "$here/../http/run.sh" >> "$out/body.md"
		echo '```' >> "$out/body.md" ;;
	*) echo "unknown row group $r" >&2; exit 2 ;;
	esac
done
mkdir -p "$(dirname "$result")"
cat "$out/header.md" "$out/body.md" > "$result"
echo "wrote $result"
