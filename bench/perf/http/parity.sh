#!/bin/sh
# parity.sh A B: runs 26 requests of the users CRUD API against two servers
# (each a command that listens on 127.0.0.1:$PORT) and diffs the responses,
# status line, headers (names compared without case, Date, Content-Length
# and Connection left out) and body. For the twins of bench/perf/{rust,node,
# python}/http: they must answer as the Go server does before they are timed.
# Usage: PORT_A=19101 PORT_B=19102 sh parity.sh "/path/srv-go" "node dist/server.js"
set -u
pa=${PORT_A:-19101}
pb=${PORT_B:-19102}
out=$(mktemp -d)
req() { # port method path body
	if [ -n "$4" ]; then curl -s -i -X "$2" -H 'Content-Type: application/json' -d "$4" "http://127.0.0.1:$1$3"
	else curl -s -i -X "$2" "http://127.0.0.1:$1$3"; fi |
		grep -v -i -E '^(date|content-length|connection|keep-alive|transfer-encoding):' | tr -d '\r' | awk -F': ' 'NR>1 && NF>1 && /^[A-Za-z-]+: /{ $1=tolower($1); print $1": "$2; next } {print}'
}
script() {
	p=$1
	req "$p" POST /users '{"name":"Ann","email":"a@x"}'
	req "$p" POST /users '{"name":"Bob","email":"b@x","nickname":"bobby"}'
	req "$p" POST /users '{"name":"Ann","email":"a@x"}'
	req "$p" POST /users '{"name":"","email":"c@x"}'
	req "$p" POST /users '{"email":"c@x"}'
	req "$p" POST /users '{"name":"C","email":"cx"}'
	req "$p" POST /users '{"name":"C","email":"c@x","zzz":1}'
	req "$p" POST /users '[1]'
	req "$p" POST /users '{bad'
	req "$p" POST /users '{"name":5,"email":"c@x"}'
	req "$p" POST /users '{"name":"C","email":"c@x","nickname":5}'
	req "$p" GET /users ''
	req "$p" GET /users/1 ''
	req "$p" GET /users/0 ''
	req "$p" GET /users/abc ''
	req "$p" GET /users/9 ''
	req "$p" PATCH /users/1 '{"nickname":"n1"}'
	req "$p" PATCH /users/1 '{"nickname":null}'
	req "$p" PATCH /users/1 '{"email":"b@x"}'
	req "$p" PATCH /users/1 '{"email":"a2@x","name":"Ann2"}'
	req "$p" PATCH /users/9 '{"name":"x"}'
	req "$p" PATCH /users/1 '{"name":"éé"}'
	req "$p" GET /users ''
	req "$p" DELETE /users/2 ''
	req "$p" DELETE /users/2 ''
	req "$p" GET /users ''
}
PORT=$pa sh -c "exec $1" >/dev/null 2>&1 &
a=$!
PORT=$pb sh -c "exec $2" >/dev/null 2>&1 &
b=$!
sleep 1.5
script "$pa" > "$out/a.txt"
script "$pb" > "$out/b.txt"
kill "$a" "$b" 2>/dev/null
if diff "$out/a.txt" "$out/b.txt"; then echo "PARITY-SAME ($(wc -l < "$out/a.txt") lines)"; rc=0; else rc=1; fi
rm -rf "$out"
exit $rc
