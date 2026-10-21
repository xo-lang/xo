#!/bin/bash
# The Autobahn TestSuite's fuzzingclient against std/websocket's server
# (decision 0313 stage 3): bench/websocket/autobahn/server built with
# XO (default the checkout's bin/xo) for the host and run on port 9001,
# crossbario/autobahn-testsuite in Docker (linux/amd64, emulated on an
# arm64 Mac) connecting to it through host.docker.internal, cases 1 to 7
# and 9 (10 and 12 to 13 are compression and the like, not offered).
# The client side: the fuzzingserver in Docker on port 9001 and
# bench/websocket/autobahn/client dialing it for each case.
#   bench/websocket/autobahn/run.sh OUT [server|client|both]
# BACKEND (default the native one) picks xo build's backend. OUT gets the binaries, their logs, the reports (reports/ and
# reports-client/ index.html), and summary.txt and summary-client.txt:
# each case's behavior and the counts by behavior.
set -euo pipefail
out=${1:?OUT}
mode=${2:-both}
root=$(cd "$(dirname "$0")/../../.." && pwd)
here=$root/bench/websocket/autobahn
xo=${XO:-$root/bin/xo}
mkdir -p "$out/reports"
summarize() { # reports-dir summary-file
python3 - "$out" "$1" "$2" <<'PY'
import json, sys, collections
out = sys.argv[1]
idx = json.load(open(out + "/" + sys.argv[2] + "/index.json"))
cases = idx["xo"]
counts = collections.Counter()
lines = []
def key(k):
    return [int(x) for x in k.split(".")]
for k in sorted(cases, key=key):
    b = cases[k]["behavior"]
    bc = cases[k].get("behaviorClose", "")
    counts[b] += 1
    lines.append(f"{k} {b} {bc}")
with open(out + "/" + sys.argv[3], "w") as f:
    f.write("\n".join(lines) + "\n")
    f.write("counts: " + ", ".join(f"{b} {n}" for b, n in sorted(counts.items())) + "\n")
print("counts: " + ", ".join(f"{b} {n}" for b, n in sorted(counts.items())))
PY
}
if [ "$mode" != client ]; then
(cd "$here/server" && XO_STD="$root/std" "$xo" build ${BACKEND:+--backend=$BACKEND} -o "$out/server" .)
cp "$here/fuzzingclient.json" "$out/fuzzingclient.json"
PORT=9001 "$out/server" > "$out/server.log" 2>&1 &
pid=$!
trap 'kill "$pid" 2>/dev/null || true' EXIT
sleep 1
docker run --rm --platform linux/amd64 -v "$out:/work" crossbario/autobahn-testsuite \
	wstest -m fuzzingclient -s /work/fuzzingclient.json > "$out/wstest.log" 2>&1 || true
kill "$pid" 2>/dev/null || true
wait "$pid" 2>/dev/null || true
summarize reports summary.txt
fi
if [ "$mode" != server ]; then
mkdir -p "$out/reports-client"
(cd "$here/client" && XO_STD="$root/std" "$xo" build ${BACKEND:+--backend=$BACKEND} -o "$out/client" .)
cp "$here/fuzzingserver.json" "$out/fuzzingserver.json"
cid=$(docker run -d --rm --platform linux/amd64 -p 9001:9001 -v "$out:/work" crossbario/autobahn-testsuite \
	wstest -m fuzzingserver -s /work/fuzzingserver.json)
trap 'docker stop "$cid" > /dev/null 2>&1 || true' EXIT
for _ in $(seq 1 60); do
	if nc -z 127.0.0.1 9001 2>/dev/null; then break; fi
	sleep 1
done
sleep 2
AB_URL=ws://127.0.0.1:9001 "$out/client" > "$out/client.log" 2>&1 || true
docker stop "$cid" > /dev/null 2>&1 || true
summarize reports-client summary-client.txt
fi
