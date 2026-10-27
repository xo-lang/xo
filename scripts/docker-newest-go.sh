#!/bin/bash
# docker-newest-go.sh: `use go` against the newest Go release, in Docker
# (owner, 2026-10-27, after a 0.33.0 tester's Go 1.27 broke every bind:
# decision 0393). The newest version is read from go.dev/VERSION when the
# script runs and printed first; the container is the official golang
# image of that version. The xo under test is laid out as a release is:
# a Linux xo of this checkout (cross built here with the Go backend), its
# std, xo-gobind built with this Mac's Go as scripts/release.sh builds the
# shipped helper, and the helper's source under lib/xo (decision 0393).
#
#   scripts/docker-newest-go.sh DARWIN_XO ARCH WORK
#
# DARWIN_XO builds the Linux xo; ARCH arm64 or amd64; WORK a scratch
# directory (WORK/rel the release layout, WORK/run the programs). In the
# container it runs the use-go goldens of internal/gen (go_*: stdout
# against want.txt, --backend=go) and binds strings, strconv and
# encoding/json in one program (xo run --backend=go, then its output).
# XO_ROOT tests another checkout (main before a fix); XO_NEWEST_GO=go1.N.M
# pins a version instead of the newest. Prints PASS or FAIL
# per case and exits 1 when any failed. Start Docker with docker-up.sh
# first and stop it after (toolchain.md "Linux in Docker on the Mac");
# run it through heavy.sh (HEAVY_CLASS=bulk: it cross builds xo).
set -euo pipefail
if [ $# -ne 3 ]; then
  echo "usage: scripts/docker-newest-go.sh DARWIN_XO ARCH WORK" >&2
  exit 2
fi
xo=$1
arch=$2
work=$3
# XO_ROOT: the checkout to test (default: the one holding this script)
root=${XO_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}
case "$arch" in arm64 | amd64) ;; *)
  echo "ARCH is arm64 or amd64" >&2
  exit 2
  ;;
esac
ver=${XO_NEWEST_GO:-$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -1)}
case "$ver" in go1.*) ;; *)
  echo "go.dev/VERSION gave '$ver'" >&2
  exit 1
  ;;
esac
echo "newest Go: $ver (golang:${ver#go})"
mkdir -p "$work"
work=$(cd "$work" && pwd)
rel=$work/rel
rm -rf "${rel:?}" "${work:?}/run"
mkdir -p "$rel/bin" "$rel/lib/xo" "$work/run"
# the release layout (scripts/release.sh): bin/xo, bin/xo-gobind,
# lib/xo/std, and the helper's source beside std (decision 0393)
(cd "$root" && XO_STD="$root/std" nice -n 10 "$xo" build --backend=go --targets="linux/$arch" --std=std -o "$rel/bin/xo" compiler/xo > "$work/xo-build.txt" 2>&1) || {
  tail -5 "$work/xo-build.txt"
  exit 1
}
(cd "$root" && GOOS=linux GOARCH=$arch CGO_ENABLED=0 go build -o "$rel/bin/xo-gobind" ./cmd/xo-gobind)
echo "xo-gobind built with $(go env GOVERSION)"
(cd "$root" && git ls-files std | tar -cf - -T -) | (cd "$rel/lib/xo" && tar -xf -)
if [ -d "$root/internal/gobind" ]; then
  (cd "$root" && git ls-files go.mod cmd/xo-gobind internal/gobind internal/gotool | grep -v '_test\.go$' | tar -cf - -T -) | (cd "$rel/lib/xo" && tar -xf -)
fi
for g in "$root"/internal/gen/testdata/golden/go_*/; do
  n=$(basename "$g")
  cp -R "$g" "$work/run/$n"
done
mkdir -p "$work/run/bind_std"
cat > "$work/run/bind_std/xo.mod" <<'EOF'
module example.com/bindstd
EOF
cat > "$work/run/bind_std/main.xo" <<'EOF'
use go "strings"
use go "strconv"
use go "encoding/json"

fn main(os: Os) uses (stdio, go) {
  os.stdio.println("${strings.to_upper("go")} ${strconv.itoa(27)} ${json.valid("{}".bytes())}")
}
EOF
printf 'exit 0\n--- stdout\nGO 27 true\n' > "$work/run/bind_std/want.txt"
cat > "$work/run/run.sh" <<'EOF'
#!/bin/sh
# in the container: each case, its exit status and stdout against the
# want.txt ones (stderr holds paths and crash bundles, not compared)
export PATH=/rel/bin:$PATH HOME=/tmp/home XO_STD=/rel/lib/xo/std
mkdir -p "$HOME"
go version
fail=0
for d in /run/*/; do
  n=$(basename "$d")
  [ -f "$d/want.txt" ] || continue
  cd "$d"
  want=$(sed -n '/^--- stdout$/,/^--- stderr$/p' want.txt | sed '1d;/^--- stderr$/d')
  want_code=$(sed -n 's/^exit //p' want.txt | head -1)
  got=$(xo run --backend=go . 2>/tmp/err-$n.txt)
  code=$?
  if [ "$code" = "${want_code:-0}" ] && [ "$got" = "$want" ]; then
    echo "PASS $n"
  else
    echo "FAIL $n (exit $code, want ${want_code:-0})"
    head -5 /tmp/err-$n.txt | sed 's/^/  /'
    fail=1
  fi
done
exit $fail
EOF
chmod +x "$work/run/run.sh"
nice -n 10 docker run --rm --platform "linux/$arch" -v "$rel:/rel:ro" -v "$work/run:/run" \
  "golang:${ver#go}" sh /run/run.sh
