#!/bin/bash
# remote-full.sh: run the full test suite (XO_SELFHOST_TIER=full) of one
# commit on a temporary EC2 Graviton instance, fetch the log, and
# terminate the instance. Owner-approved 2026-10-12 to move the long
# combined run off the development Mac (RAM, heavy-test lock).
#
# Usage: scripts/remote-full.sh [--dry-run] [--on-demand] [--cmd CMD] [--hold] [COMMIT]
#   COMMIT defaults to HEAD. The log lands in
#   $XO_REMOTE_OUT (default bench/results/remote/<commit>.log, not
#   committed by this script). The log ends with the peak memory in use
#   (free -m, sampled every 10 s) and per-package durations, slowest
#   first.
#   XO_REMOTE_PKGS: the packages of the full suite (default every
#   package but runs/), e.g. ./internal/selfhost; XO_REMOTE_TESTFLAGS:
#   more go test flags (e.g. -v for per-test times). XO_SELFHOST_JOBS is
#   the instance's core count (internal/selfhost/jobs_test.go), unless
#   set here; XO_SELFHOST_HEAVY_JOBS passes through when set.
#   --cmd CMD  runs CMD (a shell command, in the checkout, with go on
#              PATH) instead of the full suite, e.g.
#              --cmd 'go test -count=1 -run TestGoldenThreads ./internal/llvm'
#   --arch A   arm64 (default: Graviton, $XO_REMOTE_TYPE or r8g.8xlarge)
#              or amd64 ($XO_REMOTE_TYPE or c7i.8xlarge); the AMI and the
#              Go download follow.
#   --put F    copies the local file F to the instance's home directory
#              before CMD runs (repeatable), e.g. a release archive and
#              a script for --cmd 'bash ~/verify.sh'. CMD runs in ~/xo
#              with /usr/local/go/bin first on PATH; a step that must not
#              see Go sets its own PATH.
#   --hold     after setup, clones COMMIT to ~/xo, writes the instance
#              address to $XO_REMOTE_HOLD (default
#              bench/results/remote/hold.addr) and waits until the
#              watchdog or until this script is stopped (which terminates
#              the instance), for debugging over
#              ssh -i ~/.ssh/xo-ci.pem ADDR.
#
# Safety: the instance shuts itself down (and so terminates, by
# instance-initiated-shutdown-behavior) after XO_REMOTE_MAX_MIN minutes
# (default 300) even if this script dies; this script also terminates it
# on exit. It only ever terminates the instance it launched (by the id
# run-instances returned). Spot by default (about $0.70 to $0.80 an hour
# for c8g.16xlarge in us-east-1 on 2026-10-12; $2.55 on demand; the
# default r8g.8xlarge was $0.47 to $0.52 spot, about c8g.8xlarge's).
#
# Setup made on first use, all named xo-ci: an EC2 key pair (private key
# at ~/.ssh/xo-ci.pem) and a security group allowing SSH from this host's
# current public IP only.
set -euo pipefail

REGION=${XO_REMOTE_REGION:-us-east-1}
MAXMIN=${XO_REMOTE_MAX_MIN:-300}
GOVER=${XO_REMOTE_GO:-1.26.1}
DRY=""
CMD=""
HOLD=""
ARCH=arm64
PUTS=()
MARKET='--instance-market-options MarketType=spot,SpotOptions={SpotInstanceType=one-time}'
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY="--dry-run"; shift;;
    --on-demand) MARKET=""; shift;;
    --cmd) CMD="$2"; shift 2;;
    --hold) HOLD=1; shift;;
    --arch) ARCH="$2"; shift 2;;
    --put) [ -f "$2" ] || { echo "remote-full: --put $2: no such file" >&2; exit 2; }; PUTS+=("$2"); shift 2;;
    *) break;;
  esac
done
case "$ARCH" in
  arm64) TYPE=${XO_REMOTE_TYPE:-r8g.8xlarge}; AMIARCH=arm64;;  # 32 vCPUs, 256 GiB: the full tier needs about 13 GB per heavy run (c8g.8xlarge, 64 GiB, ran out, 2026-10-12); spot quota raised to 128 vCPUs on 2026-10-12
  amd64) TYPE=${XO_REMOTE_TYPE:-c7i.8xlarge}; AMIARCH=x86_64;;
  *) echo "remote-full: --arch $ARCH: want arm64 or amd64" >&2; exit 2;;
esac
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
commit=$(git rev-parse --short "${1:-HEAD}")
out=${XO_REMOTE_OUT:-$root/bench/results/remote/$commit.log}
mkdir -p "$(dirname "$out")"
aws() { command aws --region "$REGION" "$@"; }
log() { echo "remote-full: $*" >&2; }

# one-time setup
key=~/.ssh/xo-ci.pem
if ! aws ec2 describe-key-pairs --key-names xo-ci >/dev/null 2>&1; then
  [ -n "$DRY" ] && { log "would create key pair xo-ci"; } || {
    aws ec2 create-key-pair --key-name xo-ci --query KeyMaterial --output text > "$key"
    chmod 600 "$key"; log "created key pair xo-ci ($key)"; }
fi
vpc=$(aws ec2 describe-vpcs --filters Name=isDefault,Values=true --query 'Vpcs[0].VpcId' --output text)
sg=$(aws ec2 describe-security-groups --filters Name=group-name,Values=xo-ci-ssh Name=vpc-id,Values="$vpc" --query 'SecurityGroups[0].GroupId' --output text 2>/dev/null || true)
myip=$(curl -fsS https://checkip.amazonaws.com | tr -d '[:space:]')
if [ -z "$sg" ] || [ "$sg" = "None" ]; then
  if [ -n "$DRY" ]; then log "would create security group xo-ci-ssh (SSH from $myip/32)"; sg=""
  else
    sg=$(aws ec2 create-security-group --group-name xo-ci-ssh --description "xo remote-full.sh SSH from the owner host" --vpc-id "$vpc" --query GroupId --output text)
    log "created security group $sg"
  fi
fi
if [ -n "$sg" ] && [ -z "$DRY" ]; then
  aws ec2 authorize-security-group-ingress --group-id "$sg" --protocol tcp --port 22 --cidr "$myip/32" >/dev/null 2>&1 || true
fi

# the SSM alias, else (the agent sandbox cannot resolve ssm.*, 2026-10-12)
# the newest AL2023 kernel 6.18 image of the architecture by name
ami=$(aws ssm get-parameter --name /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-$AMIARCH --query Parameter.Value --output text 2>/dev/null) ||
  ami=$(aws ec2 describe-images --owners amazon --filters "Name=name,Values=al2023-ami-2023.*-kernel-6.18-$AMIARCH" Name=state,Values=available \
    --query 'sort_by(Images,&CreationDate)[-1].ImageId' --output text)
userdata=$(cat <<EOF
#!/bin/bash
# hard lifetime: a systemd timer (survives cloud-init; shutdown -h +N did not
# fire on AL2023 on 2026-10-12) and the plain shutdown as a second net
systemd-run --on-active=${MAXMIN}m /usr/sbin/poweroff
shutdown -h +$MAXMIN
dnf install -y git clang llvm gcc tar gzip make >/var/log/xo-setup.log 2>&1
curl -fsSL https://go.dev/dl/go$GOVER.linux-$ARCH.tar.gz | tar -C /usr/local -xz
touch /var/run/xo-ready
EOF
)
sgarg=""; [ -n "$sg" ] && sgarg="--security-group-ids $sg"
log "launching $TYPE ($([ -n "$MARKET" ] && echo spot || echo on-demand)) for $commit, self-terminating after ${MAXMIN} min"
# shellcheck disable=SC2086
id=$(aws ec2 run-instances $DRY --image-id "$ami" --instance-type "$TYPE" --key-name xo-ci $sgarg \
  $MARKET --instance-initiated-shutdown-behavior terminate \
  --block-device-mappings 'DeviceName=/dev/xvda,Ebs={VolumeSize=120,VolumeType=gp3,DeleteOnTermination=true}' \
  --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=xo-ci},{Key=purpose,Value=xo-remote-full}]' \
  --user-data "$userdata" --query 'Instances[0].InstanceId' --output text 2>&1) || {
  if [ -n "$DRY" ] && echo "$id" | grep -q DryRunOperation; then log "dry run OK: the launch would succeed"; exit 0; fi
  log "launch failed: $id"; exit 1; }
cleanup() { log "terminating $id"; aws ec2 terminate-instances --instance-ids "$id" >/dev/null || true; [ -n "${wd:-}" ] && kill "$wd" 2>/dev/null || true; }
trap cleanup EXIT
# local watchdog: terminates the instance after MAXMIN minutes even if this
# script hangs waiting on ssh (2026-10-12: neither the instance's shutdown nor
# the EXIT trap fired, and the instance ran 5.5 hours). Its PID is captured.
# Its output goes nowhere: the sleep outlives this script, and holding
# this script's stdout would keep a pipeline reading it open (2026-10-12).
( sleep $((MAXMIN * 60)); aws ec2 terminate-instances --instance-ids "$id" >/dev/null 2>&1 ) >/dev/null 2>&1 </dev/null &
wd=$!
aws ec2 wait instance-running --instance-ids "$id"
ip=$(aws ec2 describe-instances --instance-ids "$id" --query 'Reservations[0].Instances[0].PublicIpAddress' --output text)
ssh_() { ssh -i "$key" -o StrictHostKeyChecking=accept-new -o ConnectTimeout=10 -o ServerAliveInterval=60 -o ServerAliveCountMax=5 "ec2-user@$ip" "$@"; }
log "instance $id at $ip; waiting for setup"
for _ in $(seq 1 90); do ssh_ test -f /var/run/xo-ready 2>/dev/null && break; sleep 10; done
ssh_ test -f /var/run/xo-ready

bundle=$(mktemp -t xo-bundle).bundle
git bundle create "$bundle" "$commit" >/dev/null 2>&1 || git bundle create "$bundle" HEAD >/dev/null
scp -i "$key" -o StrictHostKeyChecking=accept-new -q "$bundle" "ec2-user@$ip:xo.bundle"
rm -f "$bundle"
for f in "${PUTS[@]+"${PUTS[@]}"}"; do
  log "copying $f"
  scp -i "$key" -o StrictHostKeyChecking=accept-new -q "$f" "ec2-user@$ip:"
done
if [ -n "$HOLD" ]; then
  hold=${XO_REMOTE_HOLD:-$root/bench/results/remote/hold.addr}
  ssh_ "git clone -q xo.bundle xo && cd xo && git checkout -q $commit" || true
  echo "ec2-user@$ip" > "$hold"
  log "holding $id at ec2-user@$ip (address in $hold, checkout at ~/xo); stop this script to terminate it"
  # wait on a background sleep: bash defers a signal until a foreground
  # child exits, so stopping the script would not reach the EXIT trap
  trap 'exit 143' TERM INT HUP
  sleep $((MAXMIN * 60)) &
  wait $!
  exit 0
fi
if [ -n "$CMD" ]; then
  log "running on $commit: $CMD"
  run="$CMD"
else
  log "running the full suite (XO_SELFHOST_TIER=full) on $commit"
  pkgs=${XO_REMOTE_PKGS:-'$(go list ./... 2>/dev/null | grep -v /runs/)'}
  jobs=${XO_SELFHOST_JOBS:-'$(nproc)'}
  heavy=""; [ -n "${XO_SELFHOST_HEAVY_JOBS:-}" ] && heavy="XO_SELFHOST_HEAVY_JOBS=$XO_SELFHOST_HEAVY_JOBS"
  run="echo XO_SELFHOST_JOBS=$jobs $heavy; XO_SELFHOST_TIER=full XO_SELFHOST_JOBS=$jobs $heavy go test -count=1 -timeout 240m ${XO_REMOTE_TESTFLAGS:-} $pkgs"
fi
qrun=$(printf '%q' "$run")
ssh_ "set -o pipefail; export PATH=/usr/local/go/bin:\$PATH; git clone -q xo.bundle xo && cd xo && git checkout -q $commit && \
  { uname -a; nproc; free -g; go version; clang --version | head -1; } && \
  { while sleep 10; do free -m | awk '/^Mem:/ {print \$3}'; done > ../mem.txt 2>/dev/null </dev/null & sampler=\$!; } && \
  { bash -c $qrun 2>&1; kill \$sampler; echo \"remote-full: peak memory in use \$(sort -n ../mem.txt | tail -1) MB\"; }" > "$out" || true
# per-package durations, slowest first (go test prints one on each ok and
# FAIL package line)
{ echo; echo "per-package durations (slowest first):"
  grep -E '^(ok|FAIL)[[:space:]]+[^[:space:]]+[[:space:]]+[0-9.]+s' "$out" |
    awk '{d=$3; sub(/s$/, "", d); printf "%10.1fs  %-4s %s\n", d, $1, $2}' | sort -rn; } >> "$out" || true
log "log: $out"
grep -E '^(FAIL|--- FAIL|ok|panic)' "$out" | grep -v '^ok' | head -30 >&2 || true
grep -c '^ok' "$out" | sed 's/^/remote-full: packages ok: /' >&2
