#!/bin/bash
# intel-goldens.sh: build every golden for darwin/amd64 on this Mac and run
# them on an Intel Mac over ssh (decision 0254 stage 6a; this Mac has no
# Rosetta). knowledgebase/infrastructure/toolchain.md "darwin/amd64 with
# Xo's amd64 backend".
#
#   scripts/intel-goldens.sh XO HOST REMOTE_DIR WORK [BACKEND]
#
# XO is a darwin xo (its std beside it or $XO_STD); HOST an ssh
# destination (user@host, key auth); REMOTE_DIR a directory that must not
# exist yet on HOST (it is made, and is the only thing written there:
# nothing on HOST is deleted, so the owner's data stays; the directory is
# left for them to remove); WORK a local scratch directory; BACKEND amd64
# (default, Xo's own) or llvm. Each program runs through
# scripts/golden-run.sh with a 60 s limit ($XO_GOLDEN_TIMEOUT), by a perl
# alarm shim as `timeout` (macOS has none). The summary and logs land in
# WORK/result. Ask the machine's owner before each run.
set -eu
xo=$1
host=$2
rdir=$3
work=$4
be=${5:-amd64}
root=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$work"
work=$(cd "$work" && pwd)
P=$work/pkg
rm -rf "$P" "$work/result"
mkdir -p "$P/d64/bin" "$P/d64/gold" "$P/d64/tools" "$work/blog" "$work/result"
for d in $(find "$root/internal/llvm/testdata/golden" -mindepth 1 -maxdepth 1 -type d | sort); do
  n=$(basename "$d")
  cp -R "$d" "$P/d64/gold/$n"
  (cd "$d" && "$xo" build --backend="$be" --target=darwin/amd64 -o "$work/bin-$n" . > "$work/blog/$n.txt" 2>&1) || { echo "build failed: $n"; continue; }
  cp "$work/bin-$n" "$P/d64/bin/$n"
  rm -rf "$work/bin-$n" "$work/bin-$n.dSYM"
done
cp "$root/scripts/golden-run.sh" "$P/d64/tools/"
cat > "$P/d64/tools/timeout" <<'SHIM'
#!/bin/sh
# timeout SECONDS CMD...: exit 124 when it fires (golden-run.sh's limit)
s=$1; shift
perl -e 'my $s = shift; my $pid = fork(); if ($pid == 0) { exec @ARGV or exit 127 } $SIG{ALRM} = sub { kill "KILL", $pid; exit 124 }; alarm $s; waitpid($pid, 0); exit($? >> 8) if $? >> 8 || !($? & 127); exit 128 + ($? & 127)' "$s" "$@"
SHIM
chmod +x "$P/d64/tools/timeout"
cat > "$P/d64/run.sh" <<'RUN'
#!/bin/sh
# runs every golden here; writes summary.txt and log/ in this directory only
cd "$(dirname "$0")"
here=$(pwd)
PATH=$here/tools:$PATH
export PATH XO_GOLDEN_TIMEOUT=${XO_GOLDEN_TIMEOUT:-60}
mkdir -p log
: > summary.txt
for d in gold/*; do
  n=$(basename $d)
  if [ -x bin/$n ]; then
    sh tools/golden-run.sh $here/bin/$n $here/$d > log/$n.txt 2>&1
    head -1 log/$n.txt >> summary.txt
  else
    echo "nobuild $n" >> summary.txt
  fi
done
echo "ok $(grep -c '^ok' summary.txt) of $(wc -l < summary.txt)"
RUN
chmod +x "$P/d64/run.sh"
(cd "$P" && tar -czf "$work/d64.tgz" d64)
ssh -o BatchMode=yes "$host" "test ! -e $rdir && mkdir -p $rdir" || { echo "intel-goldens.sh: $host:$rdir exists (or cannot be made); choose a new directory" >&2; exit 2; }
scp -q -o BatchMode=yes "$work/d64.tgz" "$host:$rdir/"
ssh -o BatchMode=yes "$host" "cd $rdir && tar -xzf d64.tgz && caffeinate -i sh d64/run.sh"
scp -q -r -o BatchMode=yes "$host:$rdir/d64/summary.txt" "$host:$rdir/d64/log" "$work/result/"
grep -v '^ok' "$work/result/summary.txt" || true
