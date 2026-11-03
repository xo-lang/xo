# Timing rows against Go, part 11 (2026-11-03)

Linux/arm64 offload host, `xo bench --ab` style runs (15 rounds, Go and Xo alternated, caches off) of bench/perf/otp: `otp.hotp` (decision 0473) against a hand written RFC 4226 HOTP over Go's crypto/hmac, 300,000 8 digit codes per algorithm. `none` is the setup alone.

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| otp | none | 3.639 | 2.382 | 0.655 | 0.356-0.959 | 0.638 |
| otp | sha1 | 104.4 | 499.0 | 4.778 | 4.679-4.827 | 0.223 |
| otp | sha256 | 97.43 | 153.4 | 1.574 | 1.536-1.655 | 0.226 |
| otp | sha512 | 205.7 | 323.6 | 1.573 | 1.561-1.589 | 0.22 |

Not Proven: SHA-1 is 4.8 of Go's wall and SHA-256 and SHA-512 are 1.57. Each code is one HMAC (four compressions, two for the key pads and two for the message), so the time is the hash: Go's SHA-1 and SHA-256 use the ARMv8 crypto instructions, and `std/native/llrt/sha1.xo` is scalar code with no hardware path (SHA-256 has `rt.sha256_block`; SHA-512 is scalar in both, Go's with an assembly round). The fix is a hardware SHA-1 intrinsic in the arm64 backend (`rt.sha1_block`, with its LLVM and wasm lowerings, as `rt.sha256_block`), a compiler change handed to Lang 1; it also speeds `crypto.hmac(Sha1)` everywhere (TOTP is SHA-1 by default). The per call overhead beyond the hash (the message bytes, the code text) is the rest of the 1.57.
