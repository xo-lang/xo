# bench/perf/netgo

Go side of the WebSocket echo and HTTP client rows of
`bench/results/net-vs-go-2026-10-22.md`, run in `scripts/docker-linux.sh arm64`
(`XO_DOCKER_ROOT=1 nice -n 10 scripts/docker-linux.sh arm64 WORK -- ./inside.sh`).

- `ws/`: a gorilla/websocket echo server (`gosrv`) and load client (`cli`: `-c` connections, `-size`, `-d`);
  the Xo server is `bench/websocket/autobahn/server`. Cross build with
  `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build` into WORK as `gosrv.bin`, `cli.bin`, and the Xo
  server as `xosrv`; `inside-c1.sh` alternates the servers (1 connection, 64 B), `inside-strace.sh`
  runs four configurations with `strace -f -c` on the server.
- `httpclient/`: a Go client (`goc`) with the same N, ADDR, W environment as
  `bench/perf/httpclient/main.xo`; `inside.sh` runs both against `bench/perf/httpclient/serve`
  (built as `serve.bin`; the clients as `xoc.bin`, `goc.bin`) with `time` and `strace -c -f`.
