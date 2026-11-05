# Race and use-after-free stress, 2026-11-05

Every concurrent golden (names matching the conc, http, map, net, task and sync
pattern of race-stress.sh) built with `--race` (decision 0273 mode 2: the C
library's heap, ThreadSanitizer with halt_on_error), run in rounds of copies at
once, each run's stdout and exit compared with its want.txt. Scripts:
~/xo-lead/notes/xo-lang-4/scripts/race-stress.sh (linux, with CPU burners) and
race-stress-mac.sh (darwin, no burners). Also `xo vet --cost --cost-std`
(TestVetCostStd): clean.

| Host | Revision | Shape | Runs | Failed |
|---|---|---|---|---|
| linux/arm64 offload host 1 (12 CPUs) | 772a57e34 | 2 rounds x 12 copies, 6 burners | 3568 | 162 |
| darwin/arm64 Mac | 772a57e34 | 2 rounds x 6 copies | 1784 | 63 |
| linux/arm64 offload host 1 | e06836883 | 2 rounds x 12 copies, 6 burners | 3584 | 19 |
| darwin/arm64 Mac | e06836883 | 2 rounds x 6 copies | 1796 | 23 |

## Found and fixed

- cbe600955 (security, std/changes/http.md): hserve.xo released three
  rt.heap_large blocks 8 bytes off (response trailers, the HTTP/2 settings
  block); TSan's allocator CHECK in http_h2_config, http_trailers,
  http_server_limits (24 of 24 each). rt.heap_release now ends the program on
  a misaligned prefix.
- 88b8626b0: the LLVM backend's --race instrumented llrt code inlined into
  program functions (http_h2_config, websocket_tls_deadline).
- e06836883: the unwinder's scratch memset and memcpy went through TSan's
  interceptors (lazy_const_cycle_tasks, 23 of 24; 0 of 48 after).

## Not bugs

- http_raw_path (22 of 24 first run): copies collided on its fixed port
  18737; the harness now runs `const PORT = N` goldens one copy at a time.
- http_proxy_body_next_race (20 of 24 first run): one POST body per run was
  empty (cut before its first byte), which the check at 772a57e34 did not count;
  never another request's bytes. af8800b4d counts it; 0 of 24 in the rerun.
- map_log_model (24 of 24 first run, 19 of 24 rerun): the harness's 60 s
  timeout; it takes 33.7 s alone under --race on host 1 (5.4 s without) and
  its output was complete when stopped.
- http_tls_keys: timeouts under TSan in the first run.

## Baseline

At e06836883 the only linux failures are map_log_model's timeouts. Darwin: map_log_model (timeouts, 12 of 12), http_raw_path (its fixed port, 10 of 12: race-stress-mac.sh lacks the one copy rule) and one http_h2_config report of the runtime's TLS ops table (hconn.tls_ops_set, 1 of 12), the llrt-inlined-instrumentation class of 88b8626b0 on the `xo build --race` path, which that fix does not reach yet; a false report of --race builds only, followed up next.
