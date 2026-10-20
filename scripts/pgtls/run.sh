#!/bin/bash
# scripts/pgtls/run.sh [XO]: std/tls against a real Postgres (decision
# 0281): a test CA and a server certificate for db.test and 127.0.0.1
# (openssl), postgres:16-alpine with ssl=on in Docker on 127.0.0.1:55432,
# then main.xo built by XO (default: bin/xo) on the arm64 and Go backends,
# which must print the same lines: libpq's verify-full, verify-ca and
# require, each followed by a StartupMessage over TLS. The container is
# removed by its ID at the end.
set -u
here=$(cd "$(dirname "$0")" && pwd)
xo=${1:-$here/../../bin/xo}
work=$(mktemp -d "${TMPDIR:-/tmp}/pgtls.XXXXXX")
mkdir -p "$work/certs"
cd "$work/certs" || exit 1
printf 'subjectAltName = DNS:db.test, IP:127.0.0.1\nextendedKeyUsage = serverAuth\n' >srv.ext
openssl ecparam -name prime256v1 -genkey -noout -out ca.key 2>/dev/null
openssl req -x509 -new -key ca.key -sha256 -days 3650 -subj "/CN=Xo PG Test CA" -out ca.pem 2>/dev/null
openssl ecparam -name prime256v1 -genkey -noout -out server.key 2>/dev/null
openssl req -new -key server.key -subj "/CN=db.test" -out server.csr 2>/dev/null
openssl x509 -req -in server.csr -CA ca.pem -CAkey ca.key -CAcreateserial -days 3650 -sha256 -extfile srv.ext -out server.crt 2>/dev/null
cid=$(docker run -d --rm -e POSTGRES_PASSWORD=pw -p 127.0.0.1:55432:5432 -v "$work/certs:/certs:ro" postgres:16-alpine sh -c 'cp /certs/server.crt /certs/server.key /tmp/ && chown postgres /tmp/server.crt /tmp/server.key && chmod 600 /tmp/server.key && exec docker-entrypoint.sh postgres -c ssl=on -c ssl_cert_file=/tmp/server.crt -c ssl_key_file=/tmp/server.key') || exit 1
for _ in $(seq 1 60); do
  [ "$(docker logs "$cid" 2>&1 | grep -c 'ready to accept connections')" -ge 2 ] && break
  sleep 1
done
status=0
for b in arm64 go; do
  (cd "$here" && "$xo" build --backend=$b -o "$work/pg-$b" .) >"$work/build-$b.txt" 2>&1 || { echo "$b: build failed"; cat "$work/build-$b.txt"; status=1; }
  PG_CA="$work/certs/ca.pem" "$work/pg-$b" >"$work/out-$b.txt" 2>&1
done
docker stop "$cid" >/dev/null
cat "$work/out-arm64.txt"
if ! diff "$work/out-arm64.txt" "$work/out-go.txt"; then
  echo "arm64 and Go differ"
  status=1
fi
rm -rf "${work:?}"
exit $status
