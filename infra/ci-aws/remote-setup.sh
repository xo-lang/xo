#!/usr/bin/env bash
# Runs on a fresh Ubuntu 24.04 instance: installs Go (version $1, from
# go.dev) plus clang, lld, build-essential, and clang's sanitizer runtimes.
set -euo pipefail
go_version="$1"
case "$(uname -m)" in
aarch64) goarch=arm64 ;;
x86_64) goarch=amd64 ;;
*) echo "unknown arch $(uname -m)" >&2; exit 1 ;;
esac
export DEBIAN_FRONTEND=noninteractive
# cloud-init may still hold the apt lock right after boot
cloud-init status --wait >/dev/null 2>&1 || true
sudo -E apt-get update -q
sudo -E apt-get install -y -q --no-install-recommends clang lld llvm build-essential rsync python3
curl -fsSL "https://go.dev/dl/go${go_version}.linux-${goarch}.tar.gz" -o /tmp/go.tgz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf /tmp/go.tgz
# Ubuntu's clang does not depend on the compiler-rt sanitizer runtimes
# (TSan, ASan, UBSan archives): install the ones matching clang's major
major="$(clang -dumpversion | cut -d. -f1)"
sudo -E apt-get install -y -q --no-install-recommends "libclang-rt-${major}-dev"
/usr/local/go/bin/go version
clang --version | head -1
