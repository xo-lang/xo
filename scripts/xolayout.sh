#!/bin/bash
# xolayout.sh WORK: remakes compiler/xo.layout (decision 0453) from callgrind
# runs of a warm and an edit build of coll and conc-spawn in the linux/arm64
# Docker image. WORK is a new scratch directory outside the checkout; run with
# Docker up (docker-up.sh) and bin/xo built from this checkout.
set -eu
root=$(cd "$(dirname "$0")/.." && pwd)
O=${1:?WORK}
mkdir -p "$O/xo/bin"
(cd "$root/compiler/xo" && XO_STD="$root/std" XO_BIN_CACHE=off "$root/bin/xo" build --layout=off --backend=arm64 --targets=linux/arm64 -o "$O/xo/bin/xo" .)
cp -R "$root/std" "$O/xo/std"
cp -R "$root/bench/perf/xo" "$O/coll"
cp -R "$root/bench/perf/conc/xo/spawn" "$O/spawn"
cat > "$O/inside.sh" <<'IN'
#!/bin/sh
set -eu
apt-get update > /dev/null 2>&1
apt-get install -y --no-install-recommends valgrind > /dev/null 2>&1
X=/work/xo/bin/xo
export XO_BACKEND_CACHE_DIR=/work/cache XO_BIN_CACHE=off XO_STD=/work/xo/std
for p in coll spawn; do
  cd /work/$p
  cp main.xo /work/$p.orig
  $X build -o /work/out . > /dev/null 2>&1
  for c in 0 1; do XO_BACKEND_CACHE=on $X dev stdcache --conc=$c > /dev/null 2>&1; done
  printf '\nfn release_vs_go_records() -> Int {\n  1\n}\n' >> main.xo
  $X build -o /work/out . > /dev/null 2>&1
  cp /work/$p.orig main.xo
  $X build -o /work/out . > /dev/null 2>&1
  XO_THREADS=2 valgrind --tool=callgrind --callgrind-out-file=/work/cg.$p.warm $X build -o /work/out . > /dev/null 2>&1
  printf '\nfn release_vs_go_edit() -> Int {\n  7\n}\n' >> main.xo
  XO_THREADS=2 valgrind --tool=callgrind --callgrind-out-file=/work/cg.$p.edit $X build -o /work/out . > /dev/null 2>&1
  cp /work/$p.orig main.xo
done
IN
chmod +x "$O/inside.sh"
XO_DOCKER_ROOT=1 "$root/scripts/docker-linux.sh" arm64 "$O" -- ./inside.sh
cd "$root" && go run ./scripts/xolayout compiler/xo.layout "$O"/cg.*
