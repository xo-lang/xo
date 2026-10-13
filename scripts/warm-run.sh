#!/bin/bash
# warm-run.sh: run a command for one commit on a warm, reusable EC2
# Graviton or x86 instance instead of launching a fresh one (owner,
# 2026-10-13). The instance (tag Name=xo-warm, role=<arch>) is created on
# first use, STOPPED (not terminated) when idle, and started again by the
# next job, so the toolchain, Go, build caches and checkout survive and a
# job costs its test time plus about a minute of start, not 5 to 6
# minutes of setup.
#
# Usage: scripts/warm-run.sh ARCH COMMIT CMD   (ARCH arm64 or amd64)
#   CMD runs in ~/xo checked out at COMMIT, with go on PATH; its output
#   goes to $XO_REMOTE_OUT (default bench/results/remote/warm-<commit>.log).
#   Exit status is CMD's.
# Also: scripts/warm-run.sh --stop-idle MINUTES  stops every running warm
# instance whose last job ended more than MINUTES ago and that has no job
# running; the lead runs it every few minutes.
#
# Safety: one job at a time per instance (a local lock); every boot
# schedules a poweroff after 8 hours (shutdown behavior stop), so a
# forgotten instance stops by itself; only instances tagged xo-warm are
# ever started or stopped. On demand only.
set -euo pipefail
REGION=${XO_REMOTE_REGION:-us-east-1}
GOVER=${XO_REMOTE_GO:-1.26.1}
STATE=${XO_WARM_STATE:-$HOME/.cache/xo-warm}
key=~/.ssh/xo-ci.pem
aws() { command aws --region "$REGION" "$@"; }
log() { echo "warm-run: $*" >&2; }
mkdir -p "$STATE"

if [ "${1:-}" = --stop-idle ]; then
  idle=$((${2:-30} * 60)); now=$(date +%s)
  aws ec2 describe-instances --filters Name=tag:Name,Values=xo-warm Name=instance-state-name,Values=running \
    --query 'Reservations[].Instances[].[InstanceId,Tags[?Key==`role`]|[0].Value]' --output text |
  while read -r id role; do
    [ -n "$id" ] || continue
    [ -d "$STATE/$role.lock" ] && continue
    last=$(cat "$STATE/$role.last" 2>/dev/null || echo 0)
    if [ $((now - last)) -ge "$idle" ]; then
      log "stopping idle $role instance $id"; aws ec2 stop-instances --instance-ids "$id" >/dev/null
    fi
  done
  exit 0
fi

arch=$1 commit=$2 cmd=$3
case "$arch" in
  arm64) TYPE=${XO_WARM_TYPE:-r8g.4xlarge}; AMIARCH=arm64;;
  amd64) TYPE=${XO_WARM_TYPE:-m7i.4xlarge}; AMIARCH=x86_64;;
  *) echo "warm-run: ARCH must be arm64 or amd64" >&2; exit 2;;
esac
root=$(cd "$(dirname "$0")/.." && pwd); cd "$root"
full=$(git rev-parse "$commit^{commit}")
out=${XO_REMOTE_OUT:-$root/bench/results/remote/warm-${full:0:10}.log}
mkdir -p "$(dirname "$out")"
[ -s "$key" ] || { log "$key is missing or empty"; exit 1; }

# one job at a time per instance
until mkdir "$STATE/$arch.lock" 2>/dev/null; do sleep 15; done
trap 'date +%s > "$STATE/$arch.last"; rmdir "$STATE/$arch.lock" 2>/dev/null || true' EXIT

vpc=$(aws ec2 describe-vpcs --filters Name=isDefault,Values=true --query 'Vpcs[0].VpcId' --output text)
sg=$(aws ec2 describe-security-groups --filters Name=group-name,Values=xo-ci-ssh Name=vpc-id,Values="$vpc" --query 'SecurityGroups[0].GroupId' --output text)
myip=$(curl -fsS https://checkip.amazonaws.com | tr -d '[:space:]')
aws ec2 authorize-security-group-ingress --group-id "$sg" --protocol tcp --port 22 --cidr "$myip/32" >/dev/null 2>&1 || true

read -r id st < <(aws ec2 describe-instances --filters Name=tag:Name,Values=xo-warm "Name=tag:role,Values=$arch" \
  Name=instance-state-name,Values=pending,running,stopping,stopped \
  --query 'Reservations[].Instances[0].[InstanceId,State.Name]' --output text | head -1) || true
if [ -z "${id:-}" ] || [ "$id" = None ]; then
  ami=$(aws ssm get-parameter --name /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-$AMIARCH --query Parameter.Value --output text 2>/dev/null) ||
    ami=$(aws ec2 describe-images --owners amazon --filters "Name=name,Values=al2023-ami-2023.*-kernel-6.18-$AMIARCH" Name=state,Values=available \
      --query 'sort_by(Images,&CreationDate)[-1].ImageId' --output text)
  userdata=$(cat <<EOF
#!/bin/bash
# every boot: power off (stop) after 8 hours whatever happens
cat > /etc/systemd/system/xo-warm-cap.service <<'UNIT'
[Unit]
Description=xo warm instance lifetime cap
[Service]
Type=oneshot
ExecStart=/usr/bin/systemd-run --on-active=480m /usr/sbin/poweroff
[Install]
WantedBy=multi-user.target
UNIT
systemctl enable --now xo-warm-cap.service
dnf install -y git clang llvm gcc tar gzip make nodejs >/var/log/xo-setup.log 2>&1
curl -fsSL https://go.dev/dl/go$GOVER.linux-$arch.tar.gz | tar -C /usr/local -xz
touch /var/lib/xo-ready
EOF
)
  log "creating warm $arch instance ($TYPE, on demand, stops when idle)"
  id=$(aws ec2 run-instances --image-id "$ami" --instance-type "$TYPE" --key-name xo-ci --security-group-ids "$sg" \
    --instance-initiated-shutdown-behavior stop \
    --block-device-mappings 'DeviceName=/dev/xvda,Ebs={VolumeSize=120,VolumeType=gp3,DeleteOnTermination=true}' \
    --tag-specifications "ResourceType=instance,Tags=[{Key=Name,Value=xo-warm},{Key=role,Value=$arch},{Key=purpose,Value=xo-warm-run}]" \
    --user-data "$userdata" --query 'Instances[0].InstanceId' --output text)
  st=pending
fi
case "$st" in
  stopping) aws ec2 wait instance-stopped --instance-ids "$id"; aws ec2 start-instances --instance-ids "$id" >/dev/null;;
  stopped) log "starting warm $arch instance $id"; aws ec2 start-instances --instance-ids "$id" >/dev/null;;
esac
aws ec2 wait instance-running --instance-ids "$id"
ip=$(aws ec2 describe-instances --instance-ids "$id" --query 'Reservations[0].Instances[0].PublicIpAddress' --output text)
ssh_() { ssh -i "$key" -o StrictHostKeyChecking=accept-new -o ConnectTimeout=10 -o ServerAliveInterval=60 -o ServerAliveCountMax=5 "ec2-user@$ip" "$@"; }
for _ in $(seq 1 90); do ssh_ test -f /var/lib/xo-ready 2>/dev/null && break; sleep 10; done
ssh_ test -f /var/lib/xo-ready
log "warm $arch instance $id at $ip; running on ${full:0:10}"

# ship the commit through a temporary branch ref (gotchas/ec2-bundle-sent-head.md)
bundle=$(mktemp -t xo-warm).bundle
tmpref="refs/heads/xo-remote-$full"
git update-ref "$tmpref" "$full"
git bundle create "$bundle" "$tmpref" >/dev/null 2>&1 || { git update-ref -d "$tmpref"; log "cannot bundle $full"; exit 1; }
git update-ref -d "$tmpref"
scp -i "$key" -o StrictHostKeyChecking=accept-new -q "$bundle" "ec2-user@$ip:xo.bundle"; rm -f "$bundle"
qcmd=$(printf '%q' "$cmd")
set +e
ssh_ "set -o pipefail; export PATH=/usr/local/go/bin:\$PATH; \
  { [ -d xo/.git ] || git clone -q xo.bundle xo; } && cd xo && git fetch -q ../xo.bundle '+refs/heads/*:refs/remotes/b/*' && \
  git checkout -q -f $full && git clean -qfdx -e .tools && \
  { uname -a; nproc; go version; clang --version | head -1; echo \"node \$(node --version)\"; } && bash -c $qcmd 2>&1" > "$out"
rc=$?
set -e
log "exit $rc; log $out"
exit $rc
