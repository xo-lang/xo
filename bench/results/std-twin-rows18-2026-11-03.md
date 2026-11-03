# Timing rows against Go, part 18 (2026-11-03)

Linux/arm64 offload host (the first, exclusive). bench/perf/otp at
c5643cda2 against Go's crypto/hmac HOTP twin (Go 1.26.1): `xo bench --ab`
with Go as A and Xo as B, 15 rounds (twin-otp-sha1.txt for sha1, a rerun:
the first run's sha1 spreads were 43% and 26%; twin-otp.txt for the rest;
script twin.sh).

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | Xo / Go CPU | peak memory |
|---|---|---|---|---|---|---|---|
| otp | sha1 | 91.48 | 97.96 | 1.071 | 1.035-1.088 | 0.969 | 0.081 |
| otp | sha256 | 87.77 | 116.5 | 1.327 | 1.31-1.34 | | 0.113 |
| otp | sha512 | 177.7 | 244.5 | 1.376 | 1.359-1.39 | | 0.112 |

sha1 was 1.36 of Go's wall (std-twin-rows14). XO_PROF (prof-otp.txt) put
28% of hotp's time in HMAC's padded key, built a checked byte at a time,
more than SHA-1's compressions (24%); it is now the pad constant with the
key XORed in a word at a time (075763993: ab-host against its parent, sha1
wall 0.798, ab-otp.txt). SHA-1's final block took a push per zero word and
per output word; it now takes one append and one call (c5643cda2: 0.953,
ab-otp2.txt). Both sides compute four compressions per code (the twin
builds hmac.New per code). The target, below 1.0 of Go's wall, is not met:
1.071 wall, 0.969 CPU (Go's collector runs on another thread). What is
left is spread thin (the compressions 35%, then allocation and drops of
a few percent each); a third fix, the state list taken out of the hasher
in write, measured within noise (1.014) and was dropped.
