#!/bin/sh
# lib_test.sh: bench/perf/twins/lib.sh's load() on a linux host, where
# there is no vm.loadavg sysctl. The old load() piped a failing sysctl into
# awk, so the pipeline succeeded with no output, the /proc/loadavg fallback
# never ran, and wait_load's test read "Illegal number" (host 1, task #687,
# 2026-11-22). Run: sh bench/perf/twins/lib_test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
t=$(mktemp -d)
trap 'rm -rf "$t"' EXIT
# a sysctl that fails as linux's does for vm.loadavg, and a fake loadavg
printf '#!/bin/sh\necho "sysctl: cannot stat /proc/sys/vm/loadavg" >&2\nexit 255\n' > "$t/sysctl"; chmod +x "$t/sysctl"
echo "2.50 1.00 0.50 1/100 42" > "$t/loadavg"
fail=0
got=$(PATH="$t:$PATH" LOADAVG_FILE="$t/loadavg" sh -c '. "$1"; load' sh "$here/lib.sh" 2>/dev/null)
[ "$got" = "2.50" ] && echo "ok load: $got from the loadavg file" || { echo "FAIL load: got '$got', want 2.50"; fail=1; }
got=$(PATH="$t:$PATH" LOADAVG_FILE="$t/loadavg" sh -c 'maxload=5; . "$1"; wait_load && echo done' sh "$here/lib.sh" 2>&1)
[ "$got" = "done" ] && echo "ok wait_load: under maxload, no error" || { echo "FAIL wait_load: '$got'"; fail=1; }
# ab with an xo bench that fails prints its error, not an empty table
printf '#!/bin/sh\necho "bench: program b exited 2: boom" >&2\nexit 1\n' > "$t/xo"; chmod +x "$t/xo"
got=$(LOADAVG_FILE="$t/loadavg" sh -c 'maxload=5; rounds=1; xo=$2; . "$1"; ab demo /bin/true /bin/true demo 1' sh "$here/lib.sh" "$t/xo" 2>&1)
case $got in *"no table"*boom*) echo "ok ab: a failed xo bench --ab shows its error" ;; *) echo "FAIL ab: '$got'"; fail=1 ;; esac
# rwmutex and pmap pass --max-load at the CPU count plus maxload
printf '#!/bin/sh\necho "args: $*" >&2\nexit 1\n' > "$t/xo"; chmod +x "$t/xo"
got=$(NCPU=12 LOADAVG_FILE="$t/loadavg" sh -c 'maxload=3; rounds=1; xo=$2; . "$1"; ab demo /bin/true /bin/true pmap 1' sh "$here/lib.sh" "$t/xo" 2>&1)
case $got in *"max load 15"*"--max-load=15"*) echo "ok ab: pmap runs with --max-load=15 on 12 CPUs" ;; *) echo "FAIL ab max-load: '$got'"; fail=1 ;; esac
got=$(NCPU=12 LOADAVG_FILE="$t/loadavg" sh -c 'maxload=3; rounds=1; xo=$2; . "$1"; ab demo /bin/true /bin/true fib 1' sh "$here/lib.sh" "$t/xo" 2>&1)
case $got in *max-load*) echo "FAIL ab: fib got a --max-load: '$got'"; fail=1 ;; *) echo "ok ab: fib keeps the default load limit" ;; esac
# header on linux/arm64, whose /proc/cpuinfo has no "model name": the CPU
# field read empty (", , 12 CPUs" in the 2026-11-22 results files); it now
# names the host (HOSTLABEL) or the cpuinfo CPU part
printf 'processor\t: 0\nCPU implementer\t: 0x61\nCPU part\t: 0x000\n' > "$t/cpuinfo"
printf '#!/bin/sh\necho "go version go1.0 linux/arm64"\n' > "$t/go"; chmod +x "$t/go"
got=$(PATH="$t:$PATH" CPUINFO_FILE="$t/cpuinfo" LOADAVG_FILE="$t/loadavg" NCPU=12 HOSTLABEL="host 1, the timing host" sh -c 'date=d; XO_STAMP=s; . "$1"; header Rust' sh "$here/lib.sh" 2>&1 | grep CPUs)
case $got in *", ,"*|"") echo "FAIL header: empty CPU field: '$got'"; fail=1 ;; *"host 1, the timing host"*) echo "ok header: names the host when cpuinfo has no model name" ;; *) echo "FAIL header: no host label: '$got'"; fail=1 ;; esac
got=$(PATH="$t:$PATH" CPUINFO_FILE="$t/cpuinfo" LOADAVG_FILE="$t/loadavg" NCPU=12 sh -c 'date=d; XO_STAMP=s; . "$1"; header Rust' sh "$here/lib.sh" 2>&1 | grep CPUs)
case $got in *", ,"*|"") echo "FAIL header: empty CPU field without HOSTLABEL: '$got'"; fail=1 ;; *"implementer 0x61 part 0x000"*) echo "ok header: cpuinfo implementer and part without a host label" ;; *) echo "FAIL header: '$got'"; fail=1 ;; esac
# xobuild points every xo cache, the run cache too, under its own directory
got=$(sh -c 'work=$2; xo=xo; . "$1"; xobuild crud' sh "$here/lib.sh" "$t" 2>&1)
case $got in *"XO_BACKEND_CACHE_DIR=$t/cache-xo-crud/backend"*"XO_BIN_CACHE=$t/cache-xo-crud/bin"*) echo "ok xobuild: the run cache is the row's own" ;; *) echo "FAIL xobuild: '$got'"; fail=1 ;; esac
exit $fail
