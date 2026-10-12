#!/bin/bash
# remote-xo1-x86.sh: run scripts/xo1-host-check.sh (xo1 built by the LLVM
# backend, the LLVM goldens with it, and the bootstrap fixed point
# without Go) for one commit on a temporary EC2 x86-64 instance (Amazon
# Linux 2023, linux/amd64), with remote-full.sh's setup (key pair and
# security group xo-ci), fetch the logs, and terminate it. Decision 0178
# stage 7 and decision 0196: x86 hosts need an xo1 built by LLVM.
#
# Usage: scripts/remote-xo1-x86.sh [--dry-run] [COMMIT]
#   The summary lands in $XO_REMOTE_OUT (default
#   bench/results/remote/xo1-x86-<commit>.log, not committed here), the
#   step logs beside it in xo1-x86-<commit>/.
#
# Safety as remote-full.sh: the instance powers itself off (and so
# terminates) after XO_REMOTE_MAX_MIN minutes (default 240), a local
# watchdog terminates it after the same time, and the EXIT trap
# terminates it; only the instance this script launched (by its id).
set -euo pipefail

TYPE=${XO_REMOTE_TYPE:-m7i.4xlarge}  # 16 vCPUs, 64 GiB: xo1 compiling selfhost/xo peaks near 17 GB
REGION=${XO_REMOTE_REGION:-us-east-1}
MAXMIN=${XO_REMOTE_MAX_MIN:-240}
GOVER=${XO_REMOTE_GO:-1.26.1}
DRY=""
[ "${1:-}" = "--dry-run" ] && { DRY="--dry-run"; shift; }
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
commit=$(git rev-parse --short "${1:-HEAD}")
out=${XO_REMOTE_OUT:-$root/bench/results/remote/xo1-x86-$commit.log}
mkdir -p "$(dirname "$out")"
aws() { command aws --region "$REGION" "$@"; }
log() { echo "remote-xo1-x86: $*" >&2; }

key=~/.ssh/xo-ci.pem
[ -f "$key" ] || { log "no $key: run scripts/remote-full.sh --dry-run once to set up xo-ci"; exit 1; }
vpc=$(aws ec2 describe-vpcs --filters Name=isDefault,Values=true --query 'Vpcs[0].VpcId' --output text)
sg=$(aws ec2 describe-security-groups --filters Name=group-name,Values=xo-ci-ssh Name=vpc-id,Values="$vpc" --query 'SecurityGroups[0].GroupId' --output text)
myip=$(curl -fsS https://checkip.amazonaws.com | tr -d '[:space:]')
[ -z "$DRY" ] && { aws ec2 authorize-security-group-ingress --group-id "$sg" --protocol tcp --port 22 --cidr "$myip/32" >/dev/null 2>&1 || true; }

# the SSM alias, else (the agent sandbox cannot resolve ssm.*) the newest
# AL2023 kernel 6.18 x86_64 image by name
ami=$(aws ssm get-parameter --name /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64 --query Parameter.Value --output text 2>/dev/null) ||
  ami=$(aws ec2 describe-images --owners amazon --filters 'Name=name,Values=al2023-ami-2023.*-kernel-6.18-x86_64' Name=state,Values=available \
    --query 'sort_by(Images,&CreationDate)[-1].ImageId' --output text)
userdata=$(cat <<EOF
#!/bin/bash
systemd-run --on-active=${MAXMIN}m /usr/sbin/poweroff
shutdown -h +$MAXMIN
dnf install -y git clang llvm lld gcc tar gzip time file python3 >/var/log/xo-setup.log 2>&1
curl -fsSL https://go.dev/dl/go$GOVER.linux-amd64.tar.gz | tar -C /usr/local -xz
touch /var/run/xo-ready
EOF
)
log "launching $TYPE (spot) for $commit, self-terminating after ${MAXMIN} min"
id=$(aws ec2 run-instances $DRY --image-id "$ami" --instance-type "$TYPE" --key-name xo-ci --security-group-ids "$sg" \
  --instance-market-options 'MarketType=spot,SpotOptions={SpotInstanceType=one-time}' \
  --instance-initiated-shutdown-behavior terminate \
  --block-device-mappings 'DeviceName=/dev/xvda,Ebs={VolumeSize=80,VolumeType=gp3,DeleteOnTermination=true}' \
  --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=xo-ci},{Key=purpose,Value=xo-remote-xo1-x86}]' \
  --user-data "$userdata" --query 'Instances[0].InstanceId' --output text 2>&1) || {
  if [ -n "$DRY" ] && echo "$id" | grep -q DryRunOperation; then log "dry run OK: the launch would succeed"; exit 0; fi
  log "launch failed: $id"; exit 1; }
log "instance id $id"
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
log "running scripts/xo1-host-check.sh on $commit"
ssh_ "set -o pipefail; export PATH=/usr/local/go/bin:\$PATH; git clone -q xo.bundle xo && cd xo && git checkout -q $commit && \
  scripts/xo1-host-check.sh \$HOME/out; rc=\$?; cat \$HOME/out/summary.txt; exit \$rc" > "$out" 2>&1 && rc=0 || rc=$?
steps=${out%.log}
mkdir -p "$steps"
scp -i "$key" -q "ec2-user@$ip:out/*.err" "ec2-user@$ip:out/*.log" "ec2-user@$ip:out/bootstrap/log.txt" "ec2-user@$ip:out/bootstrap/times.txt" "$steps/" 2>/dev/null || true
log "log: $out (exit $rc), step logs in $steps"
grep -E '^xo1-host:' "$out" | tail -25 >&2 || true
exit $rc
