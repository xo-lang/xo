# Timing rows against Go, part 3 (2026-11-03)

Follows std-twin-rows2-2026-11-03.md. Host 1, `xo bench --ab --rounds=15`, Go twin vs Xo `--release`. The toml, rate, aead,
uuid and websocket twins build against vendored Go modules (bench/perf/<twin>/govendor, `go build -mod=vendor`), so the
host fetches nothing. base32 and cookie are re-measured after this change set (base32 decode copies only when a newline
is present; set-cookie attributes and Expires dates compare with equal_fold).

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| base32 | encode | 55.12 | 53.72 | 0.975 | 0.962 to 0.997 | 0.553 |
| base32 | decode | 172.4 | 81.2 | 0.471 | 0.466 to 0.476 | 0.763 |
| base32 | none | 10.66 | 4.669 | 0.438 | 0.43 to 0.451 | 0.807 |
| cookie | cookie | 26.68 | 20.19 | 0.756 | 0.733 to 0.771 | 0.633 |
| cookie | setcookie | 27.24 | 27.88 | 1.024 | 1.004 to 1.047 | 0.63 |
| toml | none | 0.769 | 0.445 | 0.578 | 0.548 to 0.615 | 1.0 |
| toml | decode | 49.67 | 58.32 | 1.174 | 1.15 to 1.191 | 0.624 |
| toml | encode | 34.85 | 46.1 | 1.323 | 1.311 to 1.342 | 0.626 |
| rate | none | 3.268 | 3.036 | 0.929 | 0.911 to 0.946 | 1.0 |
| rate | allow | 357.8 | 461.4 | 1.29 | 1.271 to 1.323 | 1.0 |
| aead | none | 3.602 | 2.009 | 0.558 | 0.545 to 0.562 | 1.0 |
| aead | gcm_small | 50.49 | 59.45 | 1.177 | 1.155 to 1.288 | 0.591 |
| aead | gcm_big | 35.83 | 13.86 | 0.387 | 0.371 to 0.408 | 0.458 |
| aead | chacha_small | 103.0 | 119.1 | 1.156 | 1.146 to 1.161 | 0.597 |
| aead | chacha_big | 140.5 | 66.7 | 0.475 | 0.469 to 0.503 | 0.594 |
| aead | gcm_key | 38.52 | 50.29 | 1.305 | 1.278 to 1.32 | 0.58 |
| aead | hkdf | 134.5 | 160.0 | 1.19 | 1.149 to 1.206 | 0.585 |
| uuid | none | 1.099 | 0.787 | 0.716 | 0.697 to 0.751 | 0.684 |
| uuid | parse | 12.11 | 11.04 | 0.912 | 0.888 to 0.923 | 0.453 |
| uuid | string | 8.078 | 5.794 | 0.717 | 0.707 to 0.74 | 0.454 |
| uuid | bytes | 1.391 | 1.39 | 0.999 | 0.974 to 1.023 | 0.684 |
| websocket | none | 1.381 | 1.054 | 0.763 | 0.7 to 0.821 | 1.0 |
| websocket | small | 84.75 | 95.07 | 1.122 | 1.119 to 1.126 | 0.86 |
| websocket | big | 117.1 | 46.68 | 0.399 | 0.388 to 0.423 | 0.845 |
| websocket | dial | 47.56 | 36.45 | 0.766 | 0.756 to 0.773 | 0.763 |


Before and after (Xo / Go wall): base32 decode 1.76 to 0.47; cookie setcookie 1.52 to 1.02.
Over 1.5 now: none of these. Unchanged from part 2: strconv itoa 1.52 and atoi 1.46 (num.append_int is already a word at
a time digit pass; the rest is the result Str's allocation), trim 2.2 (ADR 0465), time string layouts 5.4 and 4.7 (ADR 0464),
walkdir 1.92, dns mx 4.66.
