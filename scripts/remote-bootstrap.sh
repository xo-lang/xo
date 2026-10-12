#!/bin/bash
# remote-bootstrap.sh: test the bootstrap chain without Go (decision 0178,
# stage 7 step 3; toolchain.md "Bootstrapping without Go") on a clean
# temporary EC2 Graviton instance (Amazon Linux 2023, linux/arm64), with
# remote-full.sh's setup (key pair and security group xo-ci), and
# terminate it. Owner approved 2026-10-12 (one instance, with a watchdog).
#
# Usage: scripts/remote-bootstrap.sh [--dry-run] [COMMIT]
#   The log lands in $XO_REMOTE_OUT (default
#   bench/results/remote/bootstrap-<commit>.log, not committed here).
#
# On the instance: phase 1 makes the seed, the stand-in for the previous
# release's binary: Go builds cmd/xo, which builds selfhost/xo with the
# arm64 backend (the Go driver, XO_SELF_DRIVER=0). Phase 2 deletes the Go
# toolchain from the machine (/usr/local/go, the Go caches, the Go built
# xo) and runs scripts/bootstrap-xo.sh with the seed: s1, s2, s3 and the
# fixed point, with clang the only compiler installed.
#
# Safety as remote-full.sh: the instance powers itself off (and so
# terminates) after XO_REMOTE_MAX_MIN minutes (default 120), a local
# watchdog terminates it after the same time, and the EXIT trap
# terminates it; only the instance this script launched (by its id).
set -euo pipefail

TYPE=${XO_REMOTE_TYPE:-c8g.4xlarge}
REGION=${XO_REMOTE_REGION:-us-east-1}
MAXMIN=${XO_REMOTE_MAX_MIN:-120}
GOVER=${XO_REMOTE_GO:-1.26.1}
DRY=""
[ "${1:-}" = "--dry-run" ] && { DRY="--dry-run"; shift; }
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
commit=$(git rev-parse --short "${1:-HEAD}")
out=${XO_REMOTE_OUT:-$root/bench/results/remote/bootstrap-$commit.log}
mkdir -p "$(dirname "$out")"
aws() { command aws --region "$REGION" "$@"; }
log() { echo "remote-bootstrap: $*" >&2; }

key=~/.ssh/xo-ci.pem
[ -f "$key" ] || { log "no $key: run scripts/remote-full.sh --dry-run once to set up xo-ci"; exit 1; }
vpc=$(aws ec2 describe-vpcs --filters Name=isDefault,Values=true --query 'Vpcs[0].VpcId' --output text)
sg=$(aws ec2 describe-security-groups --filters Name=group-name,Values=xo-ci-ssh Name=vpc-id,Values="$vpc" --query 'SecurityGroups[0].GroupId' --output text)
myip=$(curl -fsS https://checkip.amazonaws.com | tr -d '[:space:]')
[ -z "$DRY" ] && { aws ec2 authorize-security-group-ingress --group-id "$sg" --protocol tcp --port 22 --cidr "$myip/32" >/dev/null 2>&1 || true; }

ami=$(aws ssm get-parameter --name /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64 --query Parameter.Value --output text)
userdata=$(cat <<EOF
#!/bin/bash
systemd-run --on-active=${MAXMIN}m /usr/sbin/poweroff
shutdown -h +$MAXMIN
dnf install -y git clang llvm tar gzip >/var/log/xo-setup.log 2>&1
curl -fsSL https://go.dev/dl/go$GOVER.linux-arm64.tar.gz | tar -C /usr/local -xz
touch /var/run/xo-ready
EOF
)
log "launching $TYPE (spot) for $commit, self-terminating after ${MAXMIN} min"
id=$(aws ec2 run-instances $DRY --image-id "$ami" --instance-type "$TYPE" --key-name xo-ci --security-group-ids "$sg" \
  --instance-market-options 'MarketType=spot,SpotOptions={SpotInstanceType=one-time}' \
  --instance-initiated-shutdown-behavior terminate \
  --block-device-mappings 'DeviceName=/dev/xvda,Ebs={VolumeSize=60,VolumeType=gp3,DeleteOnTermination=true}' \
  --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=xo-ci},{Key=purpose,Value=xo-remote-bootstrap}]' \
  --user-data "$userdata" --query 'Instances[0].InstanceId' --output text 2>&1) || {
  if [ -n "$DRY" ] && echo "$id" | grep -q DryRunOperation; then log "dry run OK: the launch would succeed"; exit 0; fi
  log "launch failed: $id"; exit 1; }
cleanup() { log "terminating $id"; aws ec2 terminate-instances --instance-ids "$id" >/dev/null || true; [ -n "${wd:-}" ] && kill "$wd" 2>/dev/null || true; }
trap cleanup EXIT
( sleep $((MAXMIN * 60)); aws ec2 terminate-instances --instance-ids "$id" >/dev/null 2>&1 ) &
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
log "phase 1: the seed (Go builds cmd/xo, which builds selfhost/xo)"
ssh_ "set -eo pipefail; export PATH=/usr/local/go/bin:\$PATH; git clone -q xo.bundle xo && cd xo && git checkout -q $commit && \
  { uname -a; nproc; free -g; go version; clang --version | head -1; } && \
  go build -o \$HOME/xo-go ./cmd/xo && mkdir -p \$HOME/seed && \
  t0=\$(date +%s) && XO_SELF_DRIVER=0 \$HOME/xo-go build --backend=arm64 -o \$HOME/seed/xo selfhost/xo && \
  echo \"seed built in \$(( \$(date +%s) - t0 )) s\"" > "$out" 2>&1 || { log "phase 1 failed: $out"; tail -20 "$out" >&2; exit 1; }
log "phase 2: delete Go, then scripts/bootstrap-xo.sh"
ssh_ "set -o pipefail; sudo rm -rf /usr/local/go \$HOME/go \$HOME/.cache/go-build \$HOME/xo-go; \
  echo \"== phase 2: go on PATH: \$(command -v go || echo none); /usr/local/go: \$(ls -d /usr/local/go 2>/dev/null || echo absent)\"; \
  cd xo && scripts/bootstrap-xo.sh \$HOME/seed/xo .tools/bootstrap; rc=\$?; cat .tools/bootstrap/log.txt; exit \$rc" >> "$out" 2>&1 && rc=0 || rc=$?
log "log: $out (exit $rc)"
grep -E '^bootstrap:' "$out" | tail -15 >&2 || true
exit $rc
