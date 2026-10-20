#!/bin/bash
# pg-diff.sh: the Postgres differential test of std/sql's driver (decision
# 0278): the statements of scripts/pgdiff/cases.txt through pgx's pgconn
# (scripts/pgdiff/gopgx, the reference, printing spec/std.md 10.9's
# mapping of text values) and through std/sql's driver
# (scripts/pgdiff/xo), against a PostgreSQL server in Docker, outputs
# compared line by line.
#
#   scripts/pg-diff.sh XO WORK [IMAGE...]
#
# XO is the xo to build the Xo side with; WORK a scratch directory; each
# IMAGE (default postgres:17-alpine postgres:14-alpine) runs once. The
# containers this script starts are removed by their ID when it ends; it
# stops nothing else. Needs Docker and Go (the reference fetches pgx
# v5.11.0 into the module cache once).
set -euo pipefail
XO=${1:?xo}
WORK=${2:?work dir}
shift 2
IMAGES=("$@")
[ ${#IMAGES[@]} -gt 0 ] || IMAGES=(postgres:17-alpine postgres:14-alpine)
HERE=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$WORK"
(cd "$HERE/pgdiff/gopgx" && go build -mod=mod -o "$WORK/gopgx" .)
"$XO" build -o "$WORK/xodiff" "$HERE/pgdiff/xo"
status=0
for img in "${IMAGES[@]}"; do
	cid=$(docker run -d --rm -e POSTGRES_PASSWORD=pencil -e POSTGRES_USER=app -e POSTGRES_DB=app -p 127.0.0.1::5432 "$img")
	trap 'docker rm -f "$cid" >/dev/null 2>&1 || true' EXIT
	port=$(docker port "$cid" 5432 | head -1 | sed 's/.*://')
	dsn="postgres://app:pencil@127.0.0.1:$port/app?sslmode=disable"
	for _ in $(seq 1 60); do
		"$WORK/gopgx" "$dsn" < /dev/null >/dev/null 2>&1 && break
		sleep 0.5
	done
	name=$(echo "$img" | tr ':/' '__')
	"$WORK/gopgx" "$dsn" < "$HERE/pgdiff/cases.txt" > "$WORK/$name.want"
	"$WORK/xodiff" "$dsn" < "$HERE/pgdiff/cases.txt" > "$WORK/$name.got" 2>&1 || true
	if diff -u "$WORK/$name.want" "$WORK/$name.got" > "$WORK/$name.diff"; then
		echo "pg-diff $img: ok ($(grep -c '^== ' "$WORK/$name.want") statements)"
	else
		echo "pg-diff $img: DIFFERS ($WORK/$name.diff)"
		status=1
	fi
	docker rm -f "$cid" >/dev/null 2>&1 || true
	trap - EXIT
done
exit $status
