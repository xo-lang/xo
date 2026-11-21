# LLVM golden builds timed out at 480 s on a loaded host, with nothing slower than before

Status: Observed (2026-11-20 and 2026-11-21, task #583)

Symptom: the main tier's devllvm step (internal/devgoldens
TestDevGoldensLLVM, `xo dev goldens --backend=llvm --timeout=480 -j 8`)
failed with 14 and then 23 of 888 programs "build: timed out after 480.0
s" on host 4 (ctier 491faaf07, bcbb0667d). Eight goldens were in both
lists (amqp_tls_hostname, grpc_hooks, http_brotli_responses,
tls_close_write and others), which looked like slow programs and not
load; small ones such as fs_cwd_path were in one list too.

Cause: load, on programs whose clang step is long already. Measured at
1a933d78b:

- alone on the Mac the clang step (`xo build --phase-times`, phase cc)
  is nearly all of a build: amqp_tls_hostname 53 of 55 s, otlp_export
  40 of 43 s, pass_const_edges (one function of 1,600 lines of checked
  arithmetic) 108 s; fs_cwd_path builds in 5 s;
- with the 0.54.0 compiler (ab4fb8dfc) the same three took 100, 46 and
  40 s on the Mac: no compile-time regression;
- the whole step alone on host 3 (no other gate, the Mac busy) passed,
  890 of 890 in 2591 s, with 24 builds between 150 and 269 s, each 40
  to 50 s alone there; pass_const_edges takes 235 s alone on that host;
- hosts 3 and 4 are containers on the Mac, so a tier step beside two
  gates and the Mac's own jobs runs the same builds several times
  slower again, and the ones nearest the limit pass it: the same
  goldens each time.

Fix: `xo dev goldens --timeout-load=N` (compiler/cli/dev goldens.xo
scaled_ms, limit): a child's limit is the timeout times the 1 minute
load per CPU as it starts, never under the timeout and at most N times
it. TestDevGoldensLLVM passes `--timeout-load=4`. A hung build still
fails, after at most 4 times the timeout. Unit tests: goldens_test.xo
"scaled_ms" and "milli".

Avoid: before reading a list of timed out builds as slow programs,
build two of them alone with `--phase-times` and compare with the last
release's xo; and look at what else ran on the Mac in that hour.
