#!/bin/bash
# run40k.sh XOSERVER: the 40,000 connection test of decision 0449 on
# linux (the offload host): an idle h1 server holds 40,000 connections
# and every one must answer a second request, with under 20,000
# mappings in the server. ulimit -n is raised for both processes.
set -euo pipefail
srv=${1:?usage: run40k.sh XOSERVER CLIENT}
cli=${2:?usage: run40k.sh XOSERVER CLIENT}
ulimit -n 200000
PORT=18080 "$srv" &
pid=$!
trap 'kill $pid 2>/dev/null || true' EXIT
sleep 2
"$cli" -addr 127.0.0.1:18080 -n 40000 -pid $pid -settle 10s -maxmaps 20000 -again -srcs 3
