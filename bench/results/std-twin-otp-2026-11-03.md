# std/otp against Go after the SHA-2 changes, 2026-11-03 (Xo Lang 2)

Linux/arm64 offload host (the first, exclusive). bench/perf/otp (HOTP
codes through HMAC) against its Go twin, `xo bench --ab` with Go as A and
Xo as B, 15 rounds (twin-otp2.txt), at main with c5fbea698 (Lang 1:
SHA-512 on arm64's SHA512 instructions) and this push (SHA-2's finish in
one padding append and four words a call).

| workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | Xo / Go CPU | before (std-twin-rows18) |
|---|---|---|---|---|---|---|
| sha1 | 136.2 | 127.4 | 0.936 | 0.481-1.152 (Go's spread 116%: no claim) | 0.913 | 1.071 |
| sha256 | 91.08 | 119.9 | 1.316 | 1.274-1.33 | 1.18 | 1.327 |
| sha512 | 193.1 | 181.1 | 0.937 | 0.93-0.942 | 0.873 | 1.376 |

The finish change alone, against its parent (ab-host, 3 rounds): sha256
0.931 (ab-sha256.txt), sha512 1.015, noise (ab-sha512.txt). sha256's twin
row barely moved (1.327 to 1.316) where its own --ab said 0.931; the two
were measured an hour apart against different parents. XO_PROF before
the change (prof-otp256.txt): sha256's remaining time is spread over the
HMAC state's drops and allocations (drop Str 15%, drop crypto.State 15%)
and the block (25%), which already runs on the SHA-256 instructions.
prof-otp512.txt is the profile that sent SHA-512 to its instructions
(72% in the software block).

## 2026-11-04 refresh (host 1, n=6 unless noted, main at 80294e16b)

| workload | Go ms | Xo ms | ratio | interval |
|---|---|---|---|---|
| sha1 | 92.65 | 102.4 | 1.105 | 1.086 to 1.17 (n=7) |
| sha256 | 87.0 | 110.6 | 1.272 | 1.243 to 1.314 |
| sha512 | 178.6 | 165.8 | 0.928 | 0.911 to 0.951 |

The HMAC cost is the hash's compression function (std/native/llrt sha1, sha256): sha512 is ahead of Go, sha256 is not, which is Go's SHA-NI path on this host against Xo's scalar rounds for 256.
