#!/bin/bash
# remote-windows.sh: run a command on (or hold) a temporary Windows Server
# 2025 EC2 instance with clang, lld-link, the MSVC build tools and Windows
# SDK, Go, and Git, for the LLVM backend's Windows port (decisions 0196,
# 0197). Owner-approved 2026-10-12: Windows x86 on demand, about $1.40 an
# hour for 16 vCPUs, terminated after each run.
#
# Usage: scripts/remote-windows.sh [--dry-run] [--hold] [--batch] [--cmd CMD] [COMMIT]
#   COMMIT defaults to HEAD; it is sent as a git bundle and checked out at
#   ~/xo (C:\Users\Administrator\xo). Commands run in Git Bash (the
#   instance's OpenSSH default shell) with go, clang, and git on PATH.
#   --cmd CMD  runs CMD in the checkout; output goes to $XO_REMOTE_OUT
#              (default bench/results/remote/windows-<commit>.log, not
#              committed by this script).
#   --batch    runs scripts/windows-batch.sh (xo1 built by the Go seed;
#              the LLVM goldens built by xo1 for windows/amd64, run, and
#              for windows/arm64; the LLVM runtime tests; xo test LLVM
#              against Go), the Windows checks of a batch of merges; the
#              log's "batch:" lines are the summary (about 25 minutes
#              with setup, 2026-10-12)
#   --hold     after setup writes the address to $XO_REMOTE_HOLD (default
#              bench/results/remote/windows-hold.addr) and waits until the
#              lifetime ends or this script is stopped (which terminates
#              the instance), for work over
#              ssh -i ~/.ssh/xo-ci.pem Administrator@ADDR.
#   XO_REMOTE_TYPE (default c7i.4xlarge), XO_REMOTE_MAX_MIN (default 240).
#
# Safety (knowledgebase/gotchas/ec2-instance-outlived-its-timers.md): the
# instance terminates itself (instance-initiated shutdown behavior
# terminate) by `shutdown /s /t` and by a scheduled task, both set first
# thing in user data, after XO_REMOTE_MAX_MIN minutes; this script starts
# a local watchdog (its PID captured) that terminates the instance after
# the same limit, and its EXIT trap terminates it too. It only ever
# terminates the instance it launched, by the id run-instances returned.
# Security group xo-ci-ssh: SSH from this host's public IP only; no RDP.
set -euo pipefail

TYPE=${XO_REMOTE_TYPE:-c7i.4xlarge}
REGION=${XO_REMOTE_REGION:-us-east-1}
MAXMIN=${XO_REMOTE_MAX_MIN:-240}
GOVER=${XO_REMOTE_GO:-1.26.1}
DRY=""
CMD=""
HOLD=""
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY="--dry-run"; shift;;
    --cmd) CMD="$2"; shift 2;;
    --hold) HOLD=1; shift;;
    --batch) CMD="bash scripts/windows-batch.sh"; shift;;
    *) break;;
  esac
done
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
commit=$(git rev-parse --short "${1:-HEAD}")
out=${XO_REMOTE_OUT:-$root/bench/results/remote/windows-$commit.log}
mkdir -p "$(dirname "$out")"
aws() { command aws --region "$REGION" "$@"; }
log() { echo "remote-windows: $*" >&2; }

key=~/.ssh/xo-ci.pem
[ -f "$key" ] || { log "no $key: run scripts/remote-full.sh --dry-run once to create the key pair"; exit 1; }
pub=$(ssh-keygen -y -f "$key")
vpc=$(aws ec2 describe-vpcs --filters Name=isDefault,Values=true --query 'Vpcs[0].VpcId' --output text)
sg=$(aws ec2 describe-security-groups --filters Name=group-name,Values=xo-ci-ssh Name=vpc-id,Values="$vpc" --query 'SecurityGroups[0].GroupId' --output text)
myip=$(curl -fsS https://checkip.amazonaws.com | tr -d '[:space:]')
[ -z "$DRY" ] && aws ec2 authorize-security-group-ingress --group-id "$sg" --protocol tcp --port 22 --cidr "$myip/32" >/dev/null 2>&1 || true

ami=$(aws ssm get-parameter --name /aws/service/ami-windows-latest/Windows_Server-2025-English-Full-Base --query Parameter.Value --output text 2>/dev/null) ||
  ami=$(aws ec2 describe-images --owners amazon --filters 'Name=name,Values=Windows_Server-2025-English-Full-Base-*' Name=state,Values=available \
    --query 'sort_by(Images,&CreationDate)[-1].ImageId' --output text)
# user data (EC2Launch v2, as SYSTEM): lifetime first, then OpenSSH with
# the xo-ci key, then the toolchain; C:\xo-ready marks the end, and
# C:\xo-setup.log has the transcript
userdata=$(cat <<EOF
<powershell>
\$ErrorActionPreference = 'Continue'
\$ProgressPreference = 'SilentlyContinue'
shutdown /s /f /t $((MAXMIN * 60)) /c "xo lifetime"
\$at = (Get-Date).AddMinutes($MAXMIN + 2).ToString('HH:mm')
schtasks /create /tn xo-lifetime /sc once /st \$at /tr "shutdown /s /f /t 0" /ru SYSTEM /f
Start-Transcript -Path C:\xo-setup.log
Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0
Set-Service sshd -StartupType Automatic
New-NetFirewallRule -Name xo-sshd -DisplayName 'xo sshd' -Protocol TCP -LocalPort 22 -Action Allow -Direction Inbound
New-Item -ItemType Directory -Force C:\ProgramData\ssh | Out-Null
Set-Content -Path C:\ProgramData\ssh\administrators_authorized_keys -Value '$pub' -Encoding ascii
icacls C:\ProgramData\ssh\administrators_authorized_keys /inheritance:r /grant 'Administrators:F' /grant 'SYSTEM:F'
function Get-Asset(\$repo, \$pattern) {
  \$r = Invoke-RestMethod -UseBasicParsing "https://api.github.com/repos/\$repo/releases/latest"
  (\$r.assets | Where-Object { \$_.name -match \$pattern } | Select-Object -First 1).browser_download_url
}
\$d = 'C:\xo-dl'
New-Item -ItemType Directory -Force \$d | Out-Null
Invoke-WebRequest -UseBasicParsing (Get-Asset 'git-for-windows/git' '^Git-[0-9.]+-64-bit\.exe$') -OutFile \$d\git.exe
Start-Process -Wait \$d\git.exe -ArgumentList '/VERYSILENT','/NORESTART','/SUPPRESSMSGBOXES'
New-ItemProperty -Path HKLM:\SOFTWARE\OpenSSH -Name DefaultShell -Value 'C:\Program Files\Git\bin\bash.exe' -PropertyType String -Force
Start-Service sshd
Invoke-WebRequest -UseBasicParsing https://go.dev/dl/go$GOVER.windows-amd64.msi -OutFile \$d\go.msi
Start-Process -Wait msiexec -ArgumentList '/i',"\$d\go.msi",'/qn','/norestart'
Invoke-WebRequest -UseBasicParsing (Get-Asset 'llvm/llvm-project' '^LLVM-[0-9.]+-win64\.msi$') -OutFile \$d\llvm.msi
Start-Process -Wait msiexec -ArgumentList '/i',"\$d\llvm.msi",'/qn','/norestart'
Invoke-WebRequest -UseBasicParsing https://aka.ms/vs/17/release/vs_BuildTools.exe -OutFile \$d\vsbt.exe
Start-Process -Wait \$d\vsbt.exe -ArgumentList '--quiet','--wait','--norestart','--nocache','--add','Microsoft.VisualStudio.Workload.VCTools','--add','Microsoft.VisualStudio.Component.VC.Tools.x86.x64','--add','Microsoft.VisualStudio.Component.VC.Tools.ARM64','--includeRecommended'
# the aws CLI, for internal/xotool's prebuilt seeds (instance profile
# xo-ci-seed-read; decision 0216): installed unless the AMI has one (not
# confirmed for Windows Server 2025 base AMIs, 2026-10-13); the
# transcript records which
if (Get-Command aws -ErrorAction SilentlyContinue) { aws --version } else {
  Invoke-WebRequest -UseBasicParsing https://awscli.amazonaws.com/AWSCLIV2.msi -OutFile \$d\awscli.msi
  Start-Process -Wait msiexec -ArgumentList '/i',"\$d\awscli.msi",'/qn','/norestart'
  & 'C:\Program Files\Amazon\AWSCLIV2\aws.exe' --version
}
\$p = [Environment]::GetEnvironmentVariable('Path', 'Machine')
[Environment]::SetEnvironmentVariable('Path', "\$p;C:\Program Files\LLVM\bin;C:\Program Files\Go\bin;C:\Program Files\Git\cmd;C:\Program Files\Amazon\AWSCLIV2", 'Machine')
Restart-Service sshd
Set-Content -Path C:\xo-ready -Value ready
Stop-Transcript
</powershell>
EOF
)
log "launching $TYPE (on demand, Windows Server 2025 $ami) for $commit, self-terminating after ${MAXMIN} min"
id=$(aws ec2 run-instances $DRY --image-id "$ami" --instance-type "$TYPE" --key-name xo-ci --security-group-ids "$sg" \
  --iam-instance-profile Name=xo-ci-seed-read \
  --instance-initiated-shutdown-behavior terminate \
  --block-device-mappings 'DeviceName=/dev/sda1,Ebs={VolumeSize=120,VolumeType=gp3,DeleteOnTermination=true}' \
  --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=xo-ci-windows},{Key=purpose,Value=xo-remote-windows}]' \
  --user-data "$userdata" --query 'Instances[0].InstanceId' --output text 2>&1) || {
  if [ -n "$DRY" ] && echo "$id" | grep -q DryRunOperation; then log "dry run OK: the launch would succeed"; exit 0; fi
  log "launch failed: $id"; exit 1; }
log "instance id $id"
cleanup() { log "terminating $id"; aws ec2 terminate-instances --instance-ids "$id" >/dev/null || true; [ -n "${wd:-}" ] && kill "$wd" 2>/dev/null || true; }
trap cleanup EXIT
( sleep $((MAXMIN * 60)); aws ec2 terminate-instances --instance-ids "$id" >/dev/null 2>&1 ) &
wd=$!
log "local watchdog pid $wd"
aws ec2 wait instance-running --instance-ids "$id"
ip=$(aws ec2 describe-instances --instance-ids "$id" --query 'Reservations[0].Instances[0].PublicIpAddress' --output text)
ssh_() { ssh -i "$key" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=10 -o ServerAliveInterval=60 -o ServerAliveCountMax=5 "Administrator@$ip" "$@"; }
log "instance $id at $ip; waiting for setup (about 20 minutes: the build tools)"
for _ in $(seq 1 240); do ssh_ 'test -f /c/xo-ready' 2>/dev/null && break; sleep 15; done
ssh_ 'test -f /c/xo-ready'

bundle=$(mktemp -t xo-bundle).bundle
# bundle the commit through a temporary ref: git bundle refuses a bare
# sha, and the old fallback to HEAD silently sent the wrong commit when
# COMMIT was not the checkout's HEAD (an agent's branch, 2026-10-13)
tmpref="refs/heads/xo-remote-$(git rev-parse "$commit")"
git update-ref "$tmpref" "$commit"
git bundle create "$bundle" "$tmpref" >/dev/null 2>&1 || { git update-ref -d "$tmpref"; log "cannot bundle $commit"; exit 1; }
git update-ref -d "$tmpref"
scp -i "$key" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -q "$bundle" "Administrator@$ip:xo.bundle"
rm -f "$bundle"
# LF line endings: Git for Windows converts to CRLF by default, and every
# golden's want.txt would differ (gotchas/windows-git-crlf-goldens.md)
ssh_ "git config --global core.autocrlf false && git clone -q xo.bundle xo && cd xo && git checkout -q $commit" || true
if [ -n "$HOLD" ]; then
  hold=${XO_REMOTE_HOLD:-$root/bench/results/remote/windows-hold.addr}
  echo "Administrator@$ip" > "$hold"
  log "holding $id at Administrator@$ip (address in $hold, checkout at ~/xo); stop this script to terminate it"
  trap 'exit 143' TERM INT HUP
  sleep $((MAXMIN * 60)) &
  wait $!
  exit 0
fi
log "running on $commit: ${CMD:-go version}"
ssh_ "cd xo && { uname -a; go version; clang --version | head -1; } && ${CMD:-true}" > "$out" 2>&1 || true
log "log: $out"
