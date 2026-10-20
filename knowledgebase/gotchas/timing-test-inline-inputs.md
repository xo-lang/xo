# A timing test that makes its inputs between timings reports a leak

Status: Observed (2026-10-20, decision 0285)

Symptom: bench/ct's first version reported ECDSA P-256 signing as
leaking (|t| = 6.1 with 2,000 samples per class, the threshold 4.5) while
P-384 and Ed25519 passed. The P-256 code is constant time in the scalar.

Cause: the random class generated a fresh key right before each timed
signature and the fixed class reused one key. Key generation (allocation,
a scalar multiplication through other tables) changed the cache and
allocator state the next timing started in, so the classes differed in
what ran before them, not in the secret.

Fix: prepare every input first (the class of each sample and its key or
message), then time only the operation, in the prepared random order, as
dudect does. P-256 then measured |t| about 1 over two runs. Any timing
test should keep work other than the operation out of the timed loop.
