#!/bin/bash
# mysql-diff.sh: the MySQL differential test of std/sql's driver (decision
# 0648 item 12): the statements of scripts/mysqldiff/cases.txt through
# go-sql-driver/mysql (scripts/mysqldiff/gomysql, the reference, its own
# Go module: std never depends on it) and through std/sql's driver
# (scripts/mysqldiff/xo), against MySQL in Docker, outputs compared line by
# line; each SELECT also prints its column types.
#
#   scripts/mysql-diff.sh XO WORK [IMAGE...]
#
# XO is the xo to build the Xo side with; WORK a scratch directory; each
# IMAGE (default mysql:8.4 mysql:8.0) runs once, with a test CA and a
# server certificate for 127.0.0.1 (openssl): every statement over
# tls=true verified against that CA with interpolateParams=true (the
# text protocol), then with arguments prepared (the binary protocol),
# then prepared with parseTime=true and clientFoundRows=true, then
# compress=true (the compressed protocol) prepared, and interpolated with
# parseTime=true and timeTruncate=1s, then
# parseTime=true with loc=America/New_York prepared and interpolated (the
# Xo side passes the zone as sql.open's loc), multi.txt with
# multiStatements=true, then multi.txt over tls=false with RSA full
# authentication, the key fetched (allowPublicKeyRetrieval=true) and
# pinned (the container's public_key.pem), for caching_sha2_password and
# a sha256_password user
# (caching_sha2_password's full
# authentication on the first connection, its fast path after), then the
# outcome (ok or fail) of each tls mode with right, wrong and no roots,
# which must match go-sql-driver's, and the Xo driver's message for a
# certificate that does not verify, which must name tls=custom and
# tls=skip-verify (decision 0648 item 4). The containers this script
# starts are removed by their ID when it ends; it stops nothing else.
# Needs Docker, Go and openssl.
set -euo pipefail
XO=${1:?xo}
WORK=${2:?work dir}
shift 2
IMAGES=("$@")
[ ${#IMAGES[@]} -gt 0 ] || IMAGES=(mysql:8.4 mysql:8.0)
HERE=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$WORK"
(cd "$HERE/mysqldiff/gomysql" && go build -mod=mod -o "$WORK/gomysql" .)
"$XO" build -o "$WORK/xodiff" "$HERE/mysqldiff/xo"
certs="$WORK/certs"
mkdir -p "$certs"
(
	cd "$certs" || exit 1
	printf 'subjectAltName = IP:127.0.0.1\nextendedKeyUsage = serverAuth\n' >srv.ext
	openssl ecparam -name prime256v1 -genkey -noout -out ca.key 2>/dev/null
	openssl req -x509 -new -key ca.key -sha256 -days 3650 -subj "/CN=Xo MySQL Test CA" -out ca.pem 2>/dev/null
	openssl ecparam -name prime256v1 -genkey -noout -out other.key 2>/dev/null
	openssl req -x509 -new -key other.key -sha256 -days 3650 -subj "/CN=Other CA" -out other.pem 2>/dev/null
	openssl ecparam -name prime256v1 -genkey -noout -out server.key 2>/dev/null
	openssl req -new -key server.key -subj "/CN=127.0.0.1" -out server.csr 2>/dev/null
	openssl x509 -req -in server.csr -CA ca.pem -CAkey ca.key -CAcreateserial -days 3650 -sha256 -extfile srv.ext -out server.crt 2>/dev/null
	# a test key, read by the container's mysql user
	chmod 644 server.key
)
status=0
for img in "${IMAGES[@]}"; do
	cid=$(docker run -d --rm -e MYSQL_ROOT_PASSWORD=rootpencil -e MYSQL_USER=app -e MYSQL_PASSWORD=pencil -e MYSQL_DATABASE=app -p 127.0.0.1::3306 -v "$certs:/certs:ro" "$img" --ssl-ca=/certs/ca.pem --ssl-cert=/certs/server.crt --ssl-key=/certs/server.key)
	trap 'docker rm -f "$cid" >/dev/null 2>&1 || true' EXIT
	port=$(docker port "$cid" 3306 | head -1 | sed 's/.*://')
	base="app:pencil@tcp(127.0.0.1:$port)/app"
	up=0
	for _ in $(seq 1 240); do
		if [ "$("$WORK/gomysql" probe "$base?tls=skip-verify" 2>/dev/null)" = ok ]; then
			up=1
			break
		fi
		sleep 0.5
	done
	name=$(echo "$img" | tr ':/' '__')
	if [ $up = 0 ]; then
		echo "mysql-diff $img: the server did not come up"
		docker logs "$cid" 2>&1 | tail -20
		status=1
		docker rm -f "$cid" >/dev/null 2>&1 || true
		trap - EXIT
		continue
	fi
	# the reference first would fill the server's caching_sha2 cache, so
	# the Xo driver goes first: its first connection takes the full path
	docker exec "$cid" mysql -uroot -prootpencil -e 'FLUSH PRIVILEGES' >/dev/null 2>&1 || true
	: > "$WORK/$name.got"
	: > "$WORK/$name.want"
	# each pass: a case file and the DSN's parameters
	# RSA: the server's key (MY_PUBKEY: go-sql-driver's
	# RegisterServerPubKey("srv"), the Xo driver's server_pub_key:), and a
	# sha256_password user where the server still has the plugin
	docker exec "$cid" cat /var/lib/mysql/public_key.pem > "$WORK/$name.pub" 2>/dev/null || true
	rsa_passes=("multi.txt tls=false&multiStatements=true&allowPublicKeyRetrieval=true" "multi.txt tls=false&multiStatements=true&serverPubKey=srv")
	if docker exec "$cid" mysql -uroot -prootpencil -e "CREATE USER 'sha'@'%' IDENTIFIED WITH sha256_password BY 'pencil'; GRANT ALL ON app.* TO 'sha'@'%'" >/dev/null 2>&1; then
		rsa_passes+=("multi.txt tls=false&multiStatements=true&allowPublicKeyRetrieval=true sha" "multi.txt tls=false&multiStatements=true&serverPubKey=srv sha" "multi.txt tls=true&multiStatements=true sha")
	else
		echo "mysql-diff $img: no sha256_password (the server has no such plugin): its passes skipped"
	fi
	# each pass: a case file, the DSN's parameters, and the user (app);
	# the auth cache is flushed before each side, so every first
	# connection takes caching_sha2's full path (clear over TLS, RSA over
	# plain TCP)
	for pass in "cases.txt tls=true&interpolateParams=true" "cases.txt tls=true" "cases.txt tls=true&parseTime=true&clientFoundRows=true" "cases.txt tls=true&parseTime=true&loc=America%2FNew_York" "cases.txt tls=true&parseTime=true&interpolateParams=true&loc=America%2FNew_York" "multi.txt tls=true&multiStatements=true" "cases.txt tls=true&compress=true" "cases.txt tls=true&compress=true&interpolateParams=true&parseTime=true&timeTruncate=1s" "${rsa_passes[@]}"; do
		set -- $pass
		user=${3:-app}
		dsn="$user:pencil@tcp(127.0.0.1:$port)/app?$2"
		echo "=== $1 $2 $user" | tee -a "$WORK/$name.got" >> "$WORK/$name.want"
		docker exec "$cid" mysql -uroot -prootpencil -e 'FLUSH PRIVILEGES' >/dev/null 2>&1 || true
		MY_ROOTS="$certs/ca.pem" MY_PUBKEY="$WORK/$name.pub" "$WORK/xodiff" "$dsn" < "$HERE/mysqldiff/$1" >> "$WORK/$name.got" 2>&1 || true
		docker exec "$cid" mysql -uroot -prootpencil -e 'FLUSH PRIVILEGES' >/dev/null 2>&1 || true
		MY_ROOTS="$certs/ca.pem" MY_PUBKEY="$WORK/$name.pub" "$WORK/gomysql" "$dsn" < "$HERE/mysqldiff/$1" >> "$WORK/$name.want" 2>&1 || true
	done
	# each tls mode: go-sql-driver with tls=xo for the roots, the Xo
	# driver with roots: (the cache is filled now, so tls=false takes the
	# fast path, which needs no RSA)
	: > "$WORK/$name.modes.want"
	: > "$WORK/$name.modes.got"
	for m in "true ca" "custom ca" "true other" "custom other" "true none" "custom none" "skip-verify none" "skip-verify other" "preferred none" "false none"; do
		set -- $m
		if [ "$2" = none ]; then
			echo "$m $("$WORK/gomysql" probe "$base?tls=$1")" >> "$WORK/$name.modes.want"
			echo "$m $("$WORK/xodiff" probe "$base?tls=$1" 2>/dev/null)" >> "$WORK/$name.modes.got"
		else
			echo "$m $(MY_ROOTS="$certs/$2.pem" "$WORK/gomysql" probe "$base?tls=$1")" >> "$WORK/$name.modes.want"
			echo "$m $(MY_ROOTS="$certs/$2.pem" "$WORK/xodiff" probe "$base?tls=$1" 2>/dev/null)" >> "$WORK/$name.modes.got"
		fi
	done
	why=$(MY_WHY=1 "$WORK/xodiff" probe "$base" 2>&1 >/dev/null || true)
	hint=ok
	case "$why" in
	*tls=custom*skip-verify*) ;;
	*) hint="MISSING ($why)" ;;
	esac
	# plain TCP with no key and an empty auth cache: the Xo driver refuses
	# to fetch the server's key unless allowed (go-sql-driver fetches it)
	docker exec "$cid" mysql -uroot -prootpencil -e 'FLUSH PRIVILEGES' >/dev/null 2>&1 || true
	why=$(MY_WHY=1 "$WORK/xodiff" probe "$base?tls=false" 2>&1 >/dev/null || true)
	case "$why" in
	*allowPublicKeyRetrieval=true*) ;;
	*) hint="MISSING the key retrieval refusal ($why)" ;;
	esac
	cat "$WORK/$name.want" "$WORK/$name.modes.want" > "$WORK/$name.all.want"
	cat "$WORK/$name.got" "$WORK/$name.modes.got" > "$WORK/$name.all.got"
	if diff -u "$WORK/$name.all.want" "$WORK/$name.all.got" > "$WORK/$name.diff" && [ "$hint" = ok ]; then
		echo "mysql-diff $img: ok ($(grep -c '^== ' "$WORK/$name.want") statements in $(grep -c "^=== " "$WORK/$name.want") passes (tls=true interpolated, prepared, with parseTime and clientFoundRows, in loc=America/New_York, multiStatements, compressed (with timeTruncate); RSA full authentication over tls=false), $(wc -l < "$WORK/$name.modes.want" | tr -d ' ') tls outcomes, the self signed hint, the key retrieval refusal)"
	else
		echo "mysql-diff $img: DIFFERS ($WORK/$name.diff; hint $hint)"
		status=1
	fi
	docker rm -f "$cid" >/dev/null 2>&1 || true
	trap - EXIT
done
exit $status
