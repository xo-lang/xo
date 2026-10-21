#!/bin/bash
# xo-ci runner boot (decision 0225). Terraform renders this template to
# s3://<seed bucket>/ci/boot/linux.sh; every runner machine fetches and
# runs it as root on every boot (user data, cloud-init "always"). It is
# safe to run again: it installs only what is missing.
#
#   1. the 8 hour lifetime cap per boot (as scripts/warm-run.sh)
#   2. the idle stop: power off (stop) after ${idle_minutes} minutes without a job
#   3. toolchain (git, clang, llvm, Go) on a new machine
#   4. the GitHub Actions runner, registered as the ci-runner tag's name
#      with the ci-labels tag's labels; registered again when GitHub has
#      forgotten it (offline over 14 days) through the xo-ci-register
#      Lambda, so the GitHub App key never lives here
set -uo pipefail
exec >>/var/log/xo-ci-boot.log 2>&1
echo "== boot $(date -u +%FT%TZ)"
# the idle stop waits while this script runs (a new machine installs for minutes)
touch /run/xo-ci-booting
trap 'rm -f /run/xo-ci-booting' EXIT
export AWS_REGION=${region}
U=ec2-user
R=/home/$U/actions-runner
GOVER=1.26.1

imds() {
  t=$(curl -fsS -X PUT http://169.254.169.254/latest/api/token -H 'X-aws-ec2-metadata-token-ttl-seconds: 300')
  curl -fsS -H "X-aws-ec2-metadata-token: $t" "http://169.254.169.254/latest/meta-data/$1"
}
NAME=$(imds tags/instance/ci-runner) || { echo "no ci-runner tag"; exit 1; }
LABELS=$(imds tags/instance/ci-labels) || { echo "no ci-labels tag"; exit 1; }

# 1. lifetime cap
if [ ! -f /etc/systemd/system/xo-warm-cap.service ]; then
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
fi

# 2. idle stop, checked every minute (installed before anything that can fail)
cat > /usr/local/sbin/xo-ci-idle <<'IDLE'
#!/bin/bash
# Stop (power off) this runner machine after ${idle_minutes} minutes without a job.
# The stamp lives in /run, so every boot starts a fresh idle period.
S=/run/xo-ci-busy
svc=$(cat /home/ec2-user/actions-runner/.service 2>/dev/null)
busy() { [ -f /run/xo-ci-booting ] || pgrep -f Runner.Worker >/dev/null; }
if busy; then touch $S; exit 0; fi
[ -f $S ] || { touch $S; exit 0; }
[ $(( $(date +%s) - $(stat -c %Y $S) )) -ge $(( ${idle_minutes} * 60 )) ] || exit 0
# stop taking jobs first, then look once more: a job assigned in between keeps the machine up
[ -z "$svc" ] || systemctl stop "$svc"
if busy; then [ -z "$svc" ] || systemctl start "$svc"; touch $S; exit 0; fi
logger "xo-ci: idle ${idle_minutes} minutes, powering off"
poweroff
IDLE
chmod 755 /usr/local/sbin/xo-ci-idle
cat > /etc/systemd/system/xo-ci-idle.service <<'UNIT'
[Unit]
Description=xo-ci idle stop check
[Service]
Type=oneshot
ExecStart=/usr/local/sbin/xo-ci-idle
UNIT
cat > /etc/systemd/system/xo-ci-idle.timer <<'UNIT'
[Unit]
Description=xo-ci idle stop check every minute
[Timer]
OnBootSec=1min
OnUnitActiveSec=1min
[Install]
WantedBy=timers.target
UNIT
systemctl daemon-reload
systemctl enable --now xo-ci-idle.timer
# 3. toolchain
command -v git >/dev/null && command -v jq >/dev/null && rpm -q libicu >/dev/null ||
  dnf install -y git gcc tar gzip make nodejs jq libicu
# The LLVM backend needs clang 16 or later (XO1505) that links LTO
# bitcode: Amazon Linux 2023's clang 15 is too old, and its clang22
# package has no lld, so -flto failed at the link (owner approved the
# upstream release 2026-10-21). Install the pinned upstream LLVM (it ships
# ld.lld) to /opt/llvm, make its clang link with lld through clang.cfg, and
# put it first on PATH through /usr/local/bin. Same version and checksums
# as scripts/ci/lld-probe.sh.
LLVMVER=23.1.3
case $(uname -m) in
  aarch64) lt=ARM64; llvmsha=56d005e339c979a696ed6c244e6472e16f3b1371e48987a07e6145bda7ee0d03;;
  x86_64) lt=X64; llvmsha=ca9fe385a5d9e2d88132dc0e23a00e9f87f38fa75f67207b5e457d208e71d013;;
esac
if [ "$(cat /opt/llvm/.version 2>/dev/null)" != "$LLVMVER" ]; then
  t=/var/tmp/xo-llvm.tar.xz
  if curl -fsSL "https://github.com/llvm/llvm-project/releases/download/llvmorg-$LLVMVER/LLVM-$LLVMVER-Linux-$lt.tar.xz" -o $t &&
     echo "$llvmsha  $t" | sha256sum -c - &&
     rm -rf /opt/llvm.new && mkdir -p /opt/llvm.new &&
     tar -C /opt/llvm.new --strip-components=1 -xJf $t &&
     [ -x /opt/llvm.new/bin/ld.lld ]; then
    printf -- '-fuse-ld=lld\n' > /opt/llvm.new/bin/clang.cfg
    echo "$LLVMVER" > /opt/llvm.new/.version
    rm -rf /opt/llvm && mv /opt/llvm.new /opt/llvm
  else
    echo "LLVM $LLVMVER install failed"
    rm -rf /opt/llvm.new
  fi
  rm -f $t
fi
[ -x /opt/llvm/bin/clang ] && ln -sf /opt/llvm/bin/clang /usr/local/bin/clang
case $(uname -m) in aarch64) goarch=arm64 ra=arm64;; x86_64) goarch=amd64 ra=x64;; esac
[ -x /usr/local/go/bin/go ] || curl -fsSL "https://go.dev/dl/go$GOVER.linux-$goarch.tar.gz" | tar -C /usr/local -xz

# 4. runner
if [ ! -x $R/config.sh ]; then
  v=$(curl -fsS https://api.github.com/repos/actions/runner/releases/latest | jq -r .tag_name | sed 's/^v//')
  mkdir -p $R
  curl -fsSL "https://github.com/actions/runner/releases/download/v$v/actions-runner-linux-$ra-$v.tar.gz" | tar -C $R -xz
  # bin/installdependencies.sh does not recognize Amazon Linux 2023
  # (gotchas/actions-runner-al2023-dependencies.md): .NET needs libicu
  chown -R $U:$U $R
fi
reg=$(mktemp)
aws lambda invoke --function-name ${register_fn} --cli-binary-format raw-in-base64-out \
  --payload "{\"name\":\"$NAME\"}" "$reg" >/dev/null || { echo "register Lambda failed"; exit 1; }
if [ "$(jq -r .exists "$reg")" != true ] || [ ! -f $R/.runner ]; then
  echo "registering $NAME ($LABELS)"
  [ -f $R/.service ] && systemctl stop "$(cat $R/.service)"
  rm -f $R/.runner $R/.credentials $R/.credentials_rsaparams
  (cd $R && sudo -u $U ./config.sh --unattended --replace --url ${repo_url} \
    --token "$(jq -r .token "$reg")" --name "$NAME" --labels "$LABELS" --work _work) || { rm -f "$reg"; exit 1; }
fi
rm -f "$reg"
# job environment: Go on PATH; HOME (and so ~/.cache/xo, the warm caches) is ec2-user's
printf 'PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin\nLANG=C.UTF-8\nAWS_REGION=%s\n' "$AWS_REGION" > $R/.env
chown $U:$U $R/.env
[ -f $R/.service ] || (cd $R && ./svc.sh install $U)
(cd $R && ./svc.sh start)

echo "== ready $NAME ($LABELS)"
