# Timing rows against Go, part 14: re-measures after codegen work (2026-11-03)

Linux/arm64 offload host 1, 15 rounds, Go and Xo alternated, caches off, at main after Lang 1's `rt.sha1_block` (b39aeaae1) and the ubfx and load offset codegen (7bcf94107). These replace the otp rows of part 11 and the bcrypt rows of part 9; the other rows of those parts stand.

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | before |
|---|---|---|---|---|---|---|
| otp | sha1 | 92.48 | 126.0 | 1.363 | 1.348-1.377 | 4.778 |
| otp | sha256 | 85.41 | 132.8 | 1.554 | 1.53-1.572 | 1.574 |
| otp | sha512 | 179.0 | 296.6 | 1.657 | 1.643-1.692 | 1.573 |
| bcrypt | hash | 882.9 | 1047 | 1.186 | 1.185-1.189 | 1.44 |
| bcrypt | compare | 929.6 | 1111 | 1.195 | 1.164-1.211 | 1.55 |

Not Proven. otp SHA-1 fell from 4.8 to 1.36 of Go with the hardware compression; what is left is the HMAC around it (Go's crypto/hmac reuses the two key pad states, this port compresses the pads again for each code), and SHA-256 and SHA-512 (1.55, 1.66) have no hardware path yet. bcrypt fell from 1.44 and 1.55 to 1.19 and 1.20; the rest is the checked S-box loads and overflow checked adds in the F loop.

Next: otp keeps the HMAC pad states per call (two fewer compressions of four); SHA-256 and SHA-512 on the CPU's instructions (Lang 1, same shape as SHA-1); bcrypt's F loop with a word array that needs no bounds check.
