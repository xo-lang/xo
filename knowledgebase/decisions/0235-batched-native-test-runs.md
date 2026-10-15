# 0235: Batched native test runs

Status: Proposed (2026-10-15, claimed; being filled in)

Native `xo test` (`--no-gc`) runs many tests and property cases per
process, the test world reset between them, with one process per test
as the fallback and behind XO_TEST_BATCH=0.
