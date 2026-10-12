#!/bin/bash
# remote-full.sh: run the full test suite (XO_SELFHOST_TIER=full) of one
# commit on a temporary EC2 Graviton instance, fetch the log, and
# terminate the instance. Owner-approved 2026-10-12 to move the long
# combined run off the development Mac (RAM, heavy-test lock).
#
# Usage: scripts/remote-full.sh [--dry-run] [--on-demand] [COMMIT]
#   COMMIT defaults to HEAD. The log lands in
#   $XO_REMOTE_OUT (default bench/results/remote/<commit>.log, not
#   committed by this script).
#
# Safety: the instance shuts itself down (and so terminates, by
# instance-initiated-shutdown-behavior) after XO_REMOTE_MAX_MIN minutes
# (default 300) even if this script dies; this script also terminates it
# on exit. It only ever terminates the instance it launched (by the id
# run-instances returned). Spot by default (about $0.70 to $0.80 an hour
# for c8g.16xlarge in us-east-1 on 2026-10-12; $2.55 on demand).
#
# Setup made on first use, all named xo-ci: an EC2 key pair (private key
# at ~/.ssh/xo-ci.pem) and a security group allowing SSH from this host's
# current public IP only.
set -euo pipefail

TYPE=${XO_REMOTE_TYPE:-c8g.8xlarge}  # the account quota is 32 vCPUs (2026-10-12)
REGION=${XO_REMOTE_REGION:-us-east-1}
MAXMIN=${XO_REMOTE_MAX_MIN:-300}
GOVER=${XO_REMOTE_GO:-1.26.1}
DRY=""
MARKET='--instance-market-options MarketType=spot,SpotOptions={SpotInstanceType=one-time}'
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY="--dry-run"; shift;;
    --on-demand) MARKET=""; shift;;
    *) break;;
  esac
done
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

ami=$(aws ssm get-parameter --name /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64 --query Parameter.Value --output text)
userdata=$(cat <<EOF
#!/bin/bash
shutdown -h +$MAXMIN
dnf install -y git clang llvm gcc tar gzip make >/var/log/xo-setup.log 2>&1
curl -fsSL https://go.dev/dl/go$GOVER.linux-arm64.tar.gz | tar -C /usr/local -xz
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
cleanup() { log "terminating $id"; aws ec2 terminate-instances --instance-ids "$id" >/dev/null || true; }
trap cleanup EXIT
aws ec2 wait instance-running --instance-ids "$id"
ip=$(aws ec2 describe-instances --instance-ids "$id" --query 'Reservations[0].Instances[0].PublicIpAddress' --output text)
ssh_() { ssh -i "$key" -o StrictHostKeyChecking=accept-new -o ConnectTimeout=10 -o ServerAliveInterval=60 "ec2-user@$ip" "$@"; }
log "instance $id at $ip; waiting for setup"
for _ in $(seq 1 90); do ssh_ test -f /var/run/xo-ready 2>/dev/null && break; sleep 10; done
ssh_ test -f /var/run/xo-ready

bundle=$(mktemp -t xo-bundle).bundle
git bundle create "$bundle" "$commit" >/dev/null 2>&1 || git bundle create "$bundle" HEAD >/dev/null
scp -i "$key" -o StrictHostKeyChecking=accept-new -q "$bundle" "ec2-user@$ip:xo.bundle"
rm -f "$bundle"
log "running the full suite (XO_SELFHOST_TIER=full) on $commit"
ssh_ "set -o pipefail; export PATH=/usr/local/go/bin:\$PATH; git clone -q xo.bundle xo && cd xo && git checkout -q $commit && \
  { uname -a; nproc; free -g; go version; clang --version | head -1; } && \
  XO_SELFHOST_TIER=full go test -count=1 -timeout 240m \$(go list ./... 2>/dev/null | grep -v /runs/) 2>&1" > "$out" || true
log "log: $out"
grep -E '^(FAIL|--- FAIL|ok|panic)' "$out" | grep -v '^ok' | head -30 >&2 || true
grep -c '^ok' "$out" | sed 's/^/remote-full: packages ok: /' >&2
