# Xo against Go, TLS and HTTP/2 rows, xo 0.55.0 on linux/arm64 (2026-11-22)

A rerun of the TLS and HTTP/2 rows of `net-vs-go-2026-10-23.md` (task
#688) with the published xo 0.55.0 on the linux/arm64 timing host (host
1). It replaces that file's TLS and h2 rows; its WebSocket, CRUD and
HTTP/1.1 rows are covered by `xo-vs-go-survey-2026-11-21.txt` and
`release-vs-go-v0.55.0-linux-arm64-http.md`.

## Method

- Machine: linux/arm64 timing host (host 1), 12 cores; xo 0.55.0
  linux/arm64 (the release binary); go1.26.11 linux/arm64.
- Load: 0.66 at the start, 7.25 at the end (another lane's tier lease
  ran on the host during the run).
- `REPS=5 bench/perf/tlsrate/native.sh XO OUT arm64`: Xo server and Go
  server, one client; reps interleaved (Go, Xo, Go, Xo ...) so a load
  burst hits both alike. Ratio = Xo throughput over Go throughput (above
  1 is Xo faster). Units: h2 rows requests/s (download MB/s), handshakes
  per second, bulk MB/s (median of the client's samples).

## Results (Xo / Go throughput, higher is better for Xo)

| row | reps | Xo median | Go median | ratio | per rep |
|---|---|---|---|---|---|
| HTTP/2, 1 stream | 5 | 17005 | 13933 | 1.221 | 1.25, 1.22, 1.21, 1.20, 1.22 |
| HTTP/2, 100 streams | 5 | 108126 | 71699 | 1.508 | 1.54, 1.52, 1.49, 1.48, 1.51 |
| HTTP/2, 256 MiB download | 5 | 1273 | 975 | 1.305 | 1.38, 1.29, 1.27, 1.32, 1.32 |
| TLS 1.3 handshake, x25519 | 5 | 14758 | 10419 | 1.417 | 1.42, 1.40, 1.41, 1.42, 1.40 |
| TLS 1.3 handshake, P-256 | 5 | 14293 | 10634 | 1.344 | 1.34, 1.34, 1.35, 1.33, 1.44 |
| TLS 1.3 handshake, resumed | 5 | 15498 | 11504 | 1.347 | 1.33, 1.33, 1.38, 1.32, 1.43 |
| TLS 1.3 bulk, AES-128-GCM | 5 | 3254 | 3276 | 0.993 | 1.01, 0.97, 1.00, 0.99, 1.01 |
| TLS 1.2 bulk, ChaCha20-Poly1305 | 5 | 986 | 1044 | 0.944 | 0.95, 0.95, 0.95, 0.94, 0.96 |

Xo is ahead on every HTTP/2 and handshake row, with every rep on the
same side. TLS 1.3 bulk AES-GCM is even (0.97 to 1.01 per rep). TLS 1.2
ChaCha20-Poly1305 bulk is 5.6% behind Go, every rep between 0.94 and
0.96: the one gap these rows show.

## Raw output

The client's JSON lines, as `native.sh` printed them:

```
xo 0.55.0 linux/arm64
go version go1.26.11 linux/arm64
go h2 1 stream {"errors":0,"mode":"h2","p50_us":75,"p99_us":113,"requests":77871,"rps":12978.456011686423,"server_cpu_us_per_req":0,"streams":1}
go h2 100 streams {"errors":0,"mode":"h2","p50_us":1394,"p99_us":2810,"requests":419989,"rps":69997.10889368944,"server_cpu_us_per_req":0,"streams":100}
go h2 download {"MBps_best":965.5413313286488,"MBps_median":924.4594851079881,"mode":"h2big"}
go hs x25519 {"errors":0,"handshakes":62515,"mode":"hs","per_sec":10418.881661631774,"resumed_pct":0,"server_cpu_us_per_conn":0}
go hs p256 {"errors":0,"handshakes":64212,"mode":"hs","per_sec":10701.944403398826,"resumed_pct":0,"server_cpu_us_per_conn":0}
go hs resumed {"errors":0,"handshakes":69022,"mode":"hs","per_sec":11503.662506175393,"resumed_pct":100,"server_cpu_us_per_conn":0}
go bulk tls13 {"MBps_best":3450.5017387033117,"MBps_median":3330.502212807925,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.3 TLS_AES_128_GCM_SHA256"}
go bulk tls12 chacha {"MBps_best":1048.5300206955244,"MBps_median":1045.991771471317,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.2 TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256"}
xo h2 1 stream {"errors":0,"mode":"h2","p50_us":60,"p99_us":90,"requests":97045,"rps":16174.04524271768,"server_cpu_us_per_req":0,"streams":1}
xo h2 100 streams {"errors":0,"mode":"h2","p50_us":879,"p99_us":1689,"requests":648760,"rps":108125.83465638995,"server_cpu_us_per_req":0,"streams":100}
xo h2 download {"MBps_best":1298.418411184341,"MBps_median":1271.7478822793685,"mode":"h2big"}
xo hs x25519 {"errors":0,"handshakes":88691,"mode":"hs","per_sec":14781.790212387654,"resumed_pct":0,"server_cpu_us_per_conn":0}
xo hs p256 {"errors":0,"handshakes":86150,"mode":"hs","per_sec":14358.294439106747,"resumed_pct":0,"server_cpu_us_per_conn":0}
xo hs resumed {"errors":0,"handshakes":92065,"mode":"hs","per_sec":15344.050939277142,"resumed_pct":100,"server_cpu_us_per_conn":0}
xo bulk tls13 {"MBps_best":3530.3820957365283,"MBps_median":3373.9619390338667,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.3 TLS_AES_128_GCM_SHA256"}
xo bulk tls12 chacha {"MBps_best":994.8269207746637,"MBps_median":990.6354772232072,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.2 TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256"}
go h2 1 stream {"errors":0,"mode":"h2","p50_us":69,"p99_us":114,"requests":82960,"rps":13826.581395834768,"server_cpu_us_per_req":0,"streams":1}
go h2 100 streams {"errors":0,"mode":"h2","p50_us":1370,"p99_us":2745,"requests":428108,"rps":71351.09744660517,"server_cpu_us_per_req":0,"streams":100}
go h2 download {"MBps_best":1014.5796551489543,"MBps_median":987.6615204745029,"mode":"h2big"}
go hs x25519 {"errors":0,"handshakes":63125,"mode":"hs","per_sec":10520.64227496274,"resumed_pct":0,"server_cpu_us_per_conn":0}
go hs p256 {"errors":0,"handshakes":63769,"mode":"hs","per_sec":10628.153229138268,"resumed_pct":0,"server_cpu_us_per_conn":0}
go hs resumed {"errors":0,"handshakes":70073,"mode":"hs","per_sec":11678.79772273261,"resumed_pct":100,"server_cpu_us_per_conn":0}
go bulk tls13 {"MBps_best":3405.5542962246504,"MBps_median":3205.5347046525017,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.3 TLS_AES_128_GCM_SHA256"}
go bulk tls12 chacha {"MBps_best":1043.9726385475303,"MBps_median":1033.067090625748,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.2 TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256"}
xo h2 1 stream {"errors":0,"mode":"h2","p50_us":57,"p99_us":90,"requests":101161,"rps":16860.139611762635,"server_cpu_us_per_req":0,"streams":1}
xo h2 100 streams {"errors":0,"mode":"h2","p50_us":876,"p99_us":1684,"requests":650720,"rps":108453.05160845627,"server_cpu_us_per_req":0,"streams":100}
xo h2 download {"MBps_best":1275.3343417025928,"MBps_median":1272.503971830673,"mode":"h2big"}
xo hs x25519 {"errors":0,"handshakes":88551,"mode":"hs","per_sec":14758.449567918085,"resumed_pct":0,"server_cpu_us_per_conn":0}
xo hs p256 {"errors":0,"handshakes":85757,"mode":"hs","per_sec":14292.806329457908,"resumed_pct":0,"server_cpu_us_per_conn":0}
xo hs resumed {"errors":0,"handshakes":92991,"mode":"hs","per_sec":15497.704229049146,"resumed_pct":100,"server_cpu_us_per_conn":0}
xo bulk tls13 {"MBps_best":3523.2819118802536,"MBps_median":3112.1607059754224,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.3 TLS_AES_128_GCM_SHA256"}
xo bulk tls12 chacha {"MBps_best":993.3591625165858,"MBps_median":985.7955106457276,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.2 TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256"}
go h2 1 stream {"errors":0,"mode":"h2","p50_us":68,"p99_us":108,"requests":84221,"rps":14036.764506398038,"server_cpu_us_per_req":0,"streams":1}
go h2 100 streams {"errors":0,"mode":"h2","p50_us":1350,"p99_us":2655,"requests":435738,"rps":72620.42043794222,"server_cpu_us_per_req":0,"streams":100}
go h2 download {"MBps_best":999.1480968154715,"MBps_median":989.9163636320993,"mode":"h2big"}
go hs x25519 {"errors":0,"handshakes":62215,"mode":"hs","per_sec":10369.111935037503,"resumed_pct":0,"server_cpu_us_per_conn":0}
go hs p256 {"errors":0,"handshakes":63805,"mode":"hs","per_sec":10634.164224353615,"resumed_pct":0,"server_cpu_us_per_conn":0}
go hs resumed {"errors":0,"handshakes":68922,"mode":"hs","per_sec":11486.3100977566,"resumed_pct":100,"server_cpu_us_per_conn":0}
go bulk tls13 {"MBps_best":3433.0841175800224,"MBps_median":3276.3770064547425,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.3 TLS_AES_128_GCM_SHA256"}
go bulk tls12 chacha {"MBps_best":1045.2129561228571,"MBps_median":1044.8495722774605,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.2 TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256"}
xo h2 1 stream {"errors":0,"mode":"h2","p50_us":57,"p99_us":87,"requests":102293,"rps":17048.784073713217,"server_cpu_us_per_req":0,"streams":1}
xo h2 100 streams {"errors":0,"mode":"h2","p50_us":878,"p99_us":1692,"requests":647300,"rps":107883.11751315674,"server_cpu_us_per_req":0,"streams":100}
xo h2 download {"MBps_best":1277.3121744985285,"MBps_median":1260.8503282103823,"mode":"h2big"}
xo hs x25519 {"errors":0,"handshakes":87672,"mode":"hs","per_sec":14611.964580597856,"resumed_pct":0,"server_cpu_us_per_conn":0}
xo hs p256 {"errors":0,"handshakes":86405,"mode":"hs","per_sec":14400.693519400069,"resumed_pct":0,"server_cpu_us_per_conn":0}
xo hs resumed {"errors":0,"handshakes":94959,"mode":"hs","per_sec":15825.95916630911,"resumed_pct":100,"server_cpu_us_per_conn":0}
xo bulk tls13 {"MBps_best":3411.381799853777,"MBps_median":3274.6620252877997,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.3 TLS_AES_128_GCM_SHA256"}
xo bulk tls12 chacha {"MBps_best":992.8106006746995,"MBps_median":992.4634655049242,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.2 TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256"}
go h2 1 stream {"errors":0,"mode":"h2","p50_us":67,"p99_us":107,"requests":85352,"rps":14225.251334242892,"server_cpu_us_per_req":0,"streams":1}
go h2 100 streams {"errors":0,"mode":"h2","p50_us":1356,"p99_us":2704,"requests":433939,"rps":72322.94263429803,"server_cpu_us_per_req":0,"streams":100}
go h2 download {"MBps_best":985.700983551176,"MBps_median":973.8486518962386,"mode":"h2big"}
go hs x25519 {"errors":0,"handshakes":62671,"mode":"hs","per_sec":10445.067071211299,"resumed_pct":0,"server_cpu_us_per_conn":0}
go hs p256 {"errors":0,"handshakes":64450,"mode":"hs","per_sec":10741.037191762389,"resumed_pct":0,"server_cpu_us_per_conn":0}
go hs resumed {"errors":0,"handshakes":69587,"mode":"hs","per_sec":11597.722911413493,"resumed_pct":100,"server_cpu_us_per_conn":0}
go bulk tls13 {"MBps_best":3400.087588097846,"MBps_median":3296.8291804521336,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.3 TLS_AES_128_GCM_SHA256"}
go bulk tls12 chacha {"MBps_best":1059.9698795715863,"MBps_median":1044.2883642647698,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.2 TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256"}
xo h2 1 stream {"errors":0,"mode":"h2","p50_us":57,"p99_us":87,"requests":102207,"rps":17034.23229568332,"server_cpu_us_per_req":0,"streams":1}
xo h2 100 streams {"errors":0,"mode":"h2","p50_us":886,"p99_us":1704,"requests":643648,"rps":107274.59659847598,"server_cpu_us_per_req":0,"streams":100}
xo h2 download {"MBps_best":1297.9025782978158,"MBps_median":1285.572457663918,"mode":"h2big"}
xo hs x25519 {"errors":0,"handshakes":89069,"mode":"hs","per_sec":14843.87785518161,"resumed_pct":0,"server_cpu_us_per_conn":0}
xo hs p256 {"errors":0,"handshakes":85493,"mode":"hs","per_sec":14248.70083228951,"resumed_pct":0,"server_cpu_us_per_conn":0}
xo hs resumed {"errors":0,"handshakes":92095,"mode":"hs","per_sec":15348.247258053127,"resumed_pct":100,"server_cpu_us_per_conn":0}
xo bulk tls13 {"MBps_best":3387.69204305556,"MBps_median":3254.4962147863657,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.3 TLS_AES_128_GCM_SHA256"}
xo bulk tls12 chacha {"MBps_best":990.539234165783,"MBps_median":984.8820771939726,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.2 TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256"}
go h2 1 stream {"errors":0,"mode":"h2","p50_us":68,"p99_us":110,"requests":83596,"rps":13932.663562004804,"server_cpu_us_per_req":0,"streams":1}
go h2 100 streams {"errors":0,"mode":"h2","p50_us":1359,"p99_us":2722,"requests":430196,"rps":71699.27304619458,"server_cpu_us_per_req":0,"streams":100}
go h2 download {"MBps_best":1013.7261922407698,"MBps_median":974.91236081741,"mode":"h2big"}
go hs x25519 {"errors":0,"handshakes":62326,"mode":"hs","per_sec":10387.574617174196,"resumed_pct":0,"server_cpu_us_per_conn":0}
go hs p256 {"errors":0,"handshakes":59295,"mode":"hs","per_sec":9882.219036983523,"resumed_pct":0,"server_cpu_us_per_conn":0}
go hs resumed {"errors":0,"handshakes":66386,"mode":"hs","per_sec":11064.28092736672,"resumed_pct":100,"server_cpu_us_per_conn":0}
go bulk tls13 {"MBps_best":3396.990048397811,"MBps_median":3203.6062563206083,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.3 TLS_AES_128_GCM_SHA256"}
go bulk tls12 chacha {"MBps_best":1046.7305935867257,"MBps_median":1027.30558290661,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.2 TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256"}
xo h2 1 stream {"errors":0,"mode":"h2","p50_us":57,"p99_us":87,"requests":102030,"rps":17004.89867064297,"server_cpu_us_per_req":0,"streams":1}
xo h2 100 streams {"errors":0,"mode":"h2","p50_us":876,"p99_us":1690,"requests":650584,"rps":108429.41065651676,"server_cpu_us_per_req":0,"streams":100}
xo h2 download {"MBps_best":1295.849581892201,"MBps_median":1285.5719404950762,"mode":"h2big"}
xo hs x25519 {"errors":0,"handshakes":87471,"mode":"hs","per_sec":14577.855427974038,"resumed_pct":0,"server_cpu_us_per_conn":0}
xo hs p256 {"errors":0,"handshakes":85341,"mode":"hs","per_sec":14219.38831930369,"resumed_pct":0,"server_cpu_us_per_conn":0}
xo hs resumed {"errors":0,"handshakes":95231,"mode":"hs","per_sec":15871.652785347072,"resumed_pct":100,"server_cpu_us_per_conn":0}
xo bulk tls13 {"MBps_best":3552.2667016963387,"MBps_median":3233.8370611533624,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.3 TLS_AES_128_GCM_SHA256"}
xo bulk tls12 chacha {"MBps_best":989.1044320819364,"MBps_median":984.308839381215,"mode":"bulk","server_cpu_us_per_MB_median":0,"suite":"TLS 1.2 TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256"}
```
