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
# IMAGE (default postgres:17-alpine postgres:14-alpine) runs once, and
# postgres:17-alpine once more with ssl=on (TLS=0 skips it): a test CA and
# a server certificate for 127.0.0.1 (openssl), every statement over
# sslmode=verify-full, and the outcome (ok or fail) of each sslmode,
# right and wrong names and roots, which must match pgx's. The
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
if [ "${TLS:-1}" = 1 ]; then
	certs="$WORK/certs"
	mkdir -p "$certs"
	(
		cd "$certs" || exit 1
		printf 'subjectAltName = IP:127.0.0.1\nextendedKeyUsage = serverAuth\n' >srv.ext
		openssl ecparam -name prime256v1 -genkey -noout -out ca.key 2>/dev/null
		openssl req -x509 -new -key ca.key -sha256 -days 3650 -subj "/CN=Xo PG Test CA" -out ca.pem 2>/dev/null
		openssl ecparam -name prime256v1 -genkey -noout -out other.key 2>/dev/null
		openssl req -x509 -new -key other.key -sha256 -days 3650 -subj "/CN=Other CA" -out other.pem 2>/dev/null
		openssl ecparam -name prime256v1 -genkey -noout -out server.key 2>/dev/null
		openssl req -new -key server.key -subj "/CN=127.0.0.1" -out server.csr 2>/dev/null
		openssl x509 -req -in server.csr -CA ca.pem -CAkey ca.key -CAcreateserial -days 3650 -sha256 -extfile srv.ext -out server.crt 2>/dev/null
	)
	cid=$(docker run -d --rm -e POSTGRES_PASSWORD=pencil -e POSTGRES_USER=app -e POSTGRES_DB=app -p 127.0.0.1::5432 -v "$certs:/certs:ro" postgres:17-alpine sh -c 'cp /certs/server.crt /certs/server.key /tmp/ && chown postgres /tmp/server.crt /tmp/server.key && chmod 600 /tmp/server.key && exec docker-entrypoint.sh postgres -c ssl=on -c ssl_cert_file=/tmp/server.crt -c ssl_key_file=/tmp/server.key')
	trap 'docker rm -f "$cid" >/dev/null 2>&1 || true' EXIT
	port=$(docker port "$cid" 5432 | head -1 | sed 's/.*://')
	base="postgres://app:pencil@127.0.0.1:$port/app"
	for _ in $(seq 1 60); do
		"$WORK/gopgx" probe "$base?sslmode=disable" 2>/dev/null | grep -q ok && break
		sleep 0.5
	done
	"$WORK/gopgx" "$base?sslmode=verify-full&sslrootcert=$certs/ca.pem" < "$HERE/pgdiff/cases.txt" > "$WORK/tls.want"
	PG_ROOTS="$certs/ca.pem" "$WORK/xodiff" "$base?sslmode=verify-full" < "$HERE/pgdiff/cases.txt" > "$WORK/tls.got" 2>&1 || true
	# each mode: pgx with sslrootcert, the Xo driver with PG_ROOTS
	: > "$WORK/modes.want"
	: > "$WORK/modes.got"
	for m in "disable ca" "prefer ca" "require ca" "verify-ca ca" "verify-full ca" "verify-full other" "verify-ca other" "require other" "verify-full none"; do
		set -- $m
		q="sslmode=$1"
		root="$certs/$2.pem"
		if [ "$2" = none ]; then
			echo "$m $("$WORK/gopgx" probe "$base?$q")" >> "$WORK/modes.want"
			echo "$m $("$WORK/xodiff" probe "$base?$q")" >> "$WORK/modes.got"
		else
			echo "$m $("$WORK/gopgx" probe "$base?$q&sslrootcert=$root")" >> "$WORK/modes.want"
			echo "$m $(PG_ROOTS="$root" "$WORK/xodiff" probe "$base?$q")" >> "$WORK/modes.got"
		fi
	done
	cat "$WORK/tls.want" "$WORK/modes.want" > "$WORK/tls-all.want"
	cat "$WORK/tls.got" "$WORK/modes.got" > "$WORK/tls-all.got"
	if diff -u "$WORK/tls-all.want" "$WORK/tls-all.got" > "$WORK/tls.diff"; then
		echo "pg-diff TLS (postgres:17-alpine, ssl=on): ok ($(grep -c '^== ' "$WORK/tls.want") statements over verify-full, $(wc -l < "$WORK/modes.want" | tr -d ' ') sslmode outcomes)"
	else
		echo "pg-diff TLS: DIFFERS ($WORK/tls.diff)"
		status=1
	fi
	docker rm -f "$cid" >/dev/null 2>&1 || true
	trap - EXIT
fi
exit $status
