#!/bin/bash
# winarm-goldens.sh: every internal/llvm golden cross built for
# windows/arm64 on the native arm64 backend (decision 0569), run on a
# Windows on ARM host through ~/xo-lead/agents/offload.sh, and compared
# with its want.txt (want_windows.txt when there is one) as the golden
# tests compare: exit code and stdout exactly, stderr without the lines
# internal/arm64's stableStderr drops, log times as <time>.
#
#   scripts/winarm-goldens.sh XO WORK [NAME...]
#
# XO a built xo (the checkout's bin/xo), WORK a scratch directory on the
# Mac. OFFLOAD_ENV names the host (default offload-winarm.env); the
# remote job runs under Git Bash in ~/xo-ci/winarm-goldens. Prints
# "winarm goldens: pass P, stderr only S, fail F" and each failure; exit
# status 1 when any golden fails. Goldens with args.txt, env.txt or
# stdin.txt get them, and every file of the golden's directory sits
# beside its program (the program runs there). Git Bash's path conversion
# is off, so env values such as /etc/x reach the program as written.
# A `use c` golden needs XO_WINSYSROOT to build (decision 0569) and is
# listed as a build failure without it.
#
# First run (2026-11-09, winarm VM, Windows 11 ARM in VMware Fusion):
# 639 of 642 after the runner matched the harness; sys_c (rt.sym_present
# on windows/arm64) and net_lookup_dial_timeout (the VM's network lets
# the black hole dial succeed) remain. Last verified: 2026-11-09.
set -u
xo=${1:?XO}; work=${2:?WORK}; shift 2
root=$(cd "$(dirname "$0")/.." && pwd)
lead=${LEAD:-$HOME/xo-lead}
export OFFLOAD_ENV=${OFFLOAD_ENV:-offload-winarm.env}
names=("$@")
if [ ${#names[@]} -eq 0 ]; then
  for d in "$root"/internal/llvm/testdata/golden/*/; do names+=("$(basename "$d")"); done
fi
out=$work/g
mkdir -p "$out"
: > "$work/build-fail.txt"
for n in "${names[@]}"; do
  d=$root/internal/llvm/testdata/golden/$n
  mkdir -p "$out/$n"
  (cd "$d" && find . -maxdepth 1 -type f ! -name 'want*.txt' -exec cp {} "$out/$n/" \;)
  w=$d/want.txt
  [ -f "$d/want_windows.txt" ] && w=$d/want_windows.txt
  cp "$w" "$out/$n/want.txt"
  if ! (cd "$d" && "$xo" build --backend=arm64 --targets=windows/arm64 --std="$root/std" -o "$out/$n/prog.exe" . > "$work/$n.build.log" 2>&1); then
    echo "BUILD $n $(head -c 160 "$work/$n.build.log" | tr '\n' ' ')" >> "$work/build-fail.txt"
  fi
done
(cd "$out" && tar -czf "$work/goldens.tgz" --exclude='*.pdb' .)
"$lead/agents/offload.sh" - <<< 'mkdir -p ~/xo-ci/winarm-goldens && echo ok' > /dev/null
"$lead/agents/offload.sh" --put "$work/goldens.tgz" xo-ci/winarm-goldens/goldens.tgz || exit 2
cat > "$work/remote.sh" <<'EOF'
set -u
cd ~/xo-ci/winarm-goldens || exit 3
rm -rf ./g && mkdir g && cd g || exit 3
tar -xzf ../goldens.tgz
pass=0; fail=0; std=0
for d in */; do
  n=${d%/}
  [ -f "$n/prog.exe" ] || continue
  (
    cd "$n" || exit 3
    args=(); [ -f args.txt ] && read -r -d '' -a args < args.txt
    envs=(); [ -f env.txt ] && read -r -d '' -a envs < env.txt
    in=/dev/null; [ -f stdin.txt ] && in=stdin.txt
    MSYS_NO_PATHCONV=1 MSYS2_ENV_CONV_EXCL='*' MSYS2_ARG_CONV_EXCL='*' env ${envs[@]+"${envs[@]}"} timeout 60 ./prog.exe ${args[@]+"${args[@]}"} > out.txt 2> err.txt < "$in"; echo $? > rc.txt
  )
  tr -d '\r' < "$n/err.txt" | sed -E 's/[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?Z/<time>/g' |
    grep -avE '^$|^[[:blank:]]|^crash bundle:|^traceback:$|^xo leakcheck:' > "$n/err2.txt"
  { echo "exit $(cat "$n/rc.txt")"; echo "--- stdout"; cat "$n/out.txt"; echo "--- stderr"; cat "$n/err2.txt"; } > "$n/got.txt"
  if cmp -s "$n/got.txt" "$n/want.txt"; then
    pass=$((pass + 1))
  elif diff <(sed '/^--- stderr/,$d' "$n/got.txt") <(sed '/^--- stderr/,$d' "$n/want.txt") > /dev/null; then
    std=$((std + 1)); echo "STDERR $n"
  else
    fail=$((fail + 1)); echo "FAIL $n"; diff -a "$n/want.txt" "$n/got.txt" | head -8
  fi
done
echo "winarm goldens: pass $pass, stderr only $std, fail $fail"
EOF
"$lead/agents/offload.sh" "$work/remote.sh" | tee "$work/run.log"
cat "$work/build-fail.txt"
! grep -qE '^(FAIL|STDERR|BUILD) ' "$work/run.log" "$work/build-fail.txt"
