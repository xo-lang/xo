#!/usr/bin/env bash
# Installs the module services of decision 0568 on a linux/arm64 host, as
# root, from an xo release archive: the xomod user, /srv/xomod, the
# xo-sumdb unit and cloudflared's config. It starts nothing, creates no
# key and no cloud resource: README.md lists the owner's steps after it.
#
#   sudo ./install.sh xo-<version>-linux-arm64.tar.gz
set -euo pipefail

archive=${1:?usage: install.sh <xo release archive for linux/arm64>}
here=$(cd "$(dirname "$0")" && pwd)
root=/srv/xomod

[ "$(id -u)" = 0 ] || { echo "install.sh: run as root" >&2; exit 2; }
[ "$(uname -m)" = aarch64 ] || { echo "install.sh: want a linux/arm64 host, not $(uname -m)" >&2; exit 2; }
command -v git >/dev/null || { echo "install.sh: git is required (the log fetches module versions with it; dnf install git)" >&2; exit 2; }
# SELinux (Oracle Linux 9 enforces it): label the release as programs, so
# systemd may run xo (as unconfined_service_t), and the rest as data
selinux=0
if command -v selinuxenabled >/dev/null && selinuxenabled; then
  command -v semanage >/dev/null || { echo "install.sh: SELinux is on and semanage is missing (dnf install policycoreutils-python-utils)" >&2; exit 2; }
  selinux=1
fi

if ! id xomod >/dev/null 2>&1; then
  useradd --system --home-dir "$root" --shell /usr/sbin/nologin xomod
fi
install -d -o xomod -g xomod -m 0700 "$root" "$root/log" "$root/modcache"

# the release: bin/xo beside its std (xo finds std from the binary)
stage=$(mktemp -d "$root/.xo-install.XXXXXX")
trap 'rm -rf "${stage:?}"' EXIT
tar -xzf "$archive" -C "$stage"
top=$(find "$stage" -mindepth 1 -maxdepth 1 -type d | head -1)
[ -x "$top/bin/xo" ] || { echo "install.sh: $archive has no bin/xo" >&2; exit 1; }
rm -rf "${root:?}/xo.new"
mv "$top" "$root/xo.new"
chown -R root:root "$root/xo.new"
chmod -R go-w "$root/xo.new"
if [ -d "$root/xo" ]; then mv "$root/xo" "$root/xo.old"; fi
mv "$root/xo.new" "$root/xo"
rm -rf "${root:?}/xo.old"
if [ "$selinux" = 1 ]; then
  label() { semanage fcontext -a -t "$1" "$2" 2>/dev/null || semanage fcontext -m -t "$1" "$2"; }
  label var_lib_t "$root(/.*)?"
  label usr_t "$root/xo(/.*)?"
  label bin_t "$root/xo/bin(/.*)?"
  restorecon -R "$root"
  ls -Zd "$root" "$root/log" "$root/xo/bin/xo"
fi
"$root/xo/bin/xo" version

# settings the owner fills in (never in the repository)
if [ ! -e "$root/xomod.env" ]; then
  install -o xomod -g xomod -m 0600 /dev/null "$root/xomod.env"
  cat > "$root/xomod.env" <<'ENV'
XO_FILL_ACCESS_KEY_ID=
XO_FILL_SECRET_ACCESS_KEY=
XO_R2_ENDPOINT=https://ACCOUNT_ID.r2.cloudflarestorage.com
XO_PROXY_BUCKET=
XO_LOG_BUCKET=
ENV
fi

install -o root -g root -m 0644 "$here/xo-sumdb.service" /etc/systemd/system/xo-sumdb.service
install -d -m 0755 /etc/cloudflared
if [ ! -e /etc/cloudflared/config.yml ]; then
  install -o root -g root -m 0644 "$here/cloudflared-config.yml" /etc/cloudflared/config.yml
fi
systemctl daemon-reload
echo "installed; next: README.md, \"After install.sh\""
