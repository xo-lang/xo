#!/bin/bash
# warm.sh [--log=URL] [--git-base=BASE] [--dry-run] REPO...: looks up
# every tagged version of each repository in the checksum log (decision
# 0568), so the log records it, the index lists it and the proxy bucket
# is filled, without waiting for a client to ask. Run it after a release,
# from anywhere: it needs only git and curl, and no credentials.
#
#   scripts/modservices/warm.sh github.com/xo-lang/openai github.com/xo-lang/gcp
#
# REPO is a repository path (github.com/OWNER/NAME). Its tags are read
# with `git ls-remote` from BASE/REPO (default https://REPO). A tag
# vX.Y.Z[-pre] is a version of the module at the root, DIR/vX.Y.Z one of
# the module in DIR; a major version of 2 or more adds /vN to the path
# (spec/core.md 2.6). Each version is asked of URL/lookup/PATH@VERSION
# (default https://sum.xolang.org): a version the log holds answers at
# once, a new one is fetched from git by the log, which then writes it
# and the module's @v/list to the proxy bucket (--fill-to).
#
# A 429 (the log's --rate, per client and minute) waits for the next
# minute; a 5xx, Cloudflare's 524 (its 100 s limit while the log fetches
# a large repository) or no answer is tried again up to 3 times, since
# the log keeps what a cut fetch cloned. It exits 1 when a version still
# failed. WARM_WAIT (seconds, default 61) is the wait after a 429.
set -u
log=https://sum.xolang.org
base=
dry=0
repos=()
for a in "$@"; do
  case $a in
    --log=*) log=${a#--log=} ;;
    --git-base=*) base=${a#--git-base=} ;;
    --dry-run) dry=1 ;;
    -*) echo "warm.sh: unknown flag $a" >&2; exit 2 ;;
    *) repos+=("$a") ;;
  esac
done
if [ ${#repos[@]} -eq 0 ]; then
  echo "usage: warm.sh [--log=URL] [--git-base=BASE] [--dry-run] REPO..." >&2
  exit 2
fi
log=${log%/}
wait_s=${WARM_WAIT:-61}
fails=0

# ask URL: prints the status code; the body goes to $body
body=$(mktemp)
trap 'rm -f "$body"' EXIT
ask() {
  curl -s -m 110 -o "$body" -w '%{http_code}' "$1"
}

# versions_of REPO: tags on standard input to "<module path> <version>"
versions_of() {
  local tag dir ver path
  while read -r tag; do
    dir=
    ver=$tag
    case $tag in
      */*) dir=${tag%/*}; ver=${tag##*/} ;;
    esac
    [[ $ver =~ ^v([0-9]+)\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || continue
    path=$1
    [ -n "$dir" ] && path=$path/$dir
    [ "${BASH_REMATCH[1]}" -ge 2 ] && path=$path/v${BASH_REMATCH[1]}
    echo "$path $ver"
  done
}

for repo in "${repos[@]}"; do
  repo=${repo%/}
  url=https://$repo
  [ -n "$base" ] && url=$base/$repo
  tags=$(git ls-remote --tags --refs "$url" 2>&1) || {
    echo "$repo: git ls-remote: $tags" >&2
    fails=$((fails + 1))
    continue
  }
  pairs=$(printf '%s\n' "$tags" | sed -n 's|^[0-9a-f]*[[:space:]]*refs/tags/||p' | versions_of "$repo" | sort -t' ' -k1,1 -k2,2V)
  if [ -z "$pairs" ]; then
    echo "$repo: no version tags"
    continue
  fi
  while read -r path ver; do
    if [ $dry = 1 ]; then
      echo "$path@$ver"
      continue
    fi
    tries=0
    while :; do
      code=$(ask "$log/lookup/$path@$ver")
      case $code in
        200) echo "$path@$ver: ok"; break ;;
        429) echo "$path@$ver: 429, waiting ${wait_s}s"; sleep "$wait_s"; continue ;;
      esac
      tries=$((tries + 1))
      if [ "$code" = 404 ] || [ "$code" = 410 ] || [ $tries -gt 3 ]; then
        echo "$path@$ver: $code $(head -c 300 "$body" | tr '\n' ' ')" >&2
        fails=$((fails + 1))
        break
      fi
      echo "$path@$ver: $code, trying again ($tries of 3)"
      sleep $((tries * 5))
    done
  done <<< "$pairs"
done

[ $fails = 0 ]
