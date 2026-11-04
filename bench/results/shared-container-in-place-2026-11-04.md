# Decision 0488 prototype: a held-once shared container written in place (2026-11-04)

Xo Lang 2. Branch l2-0488m: ShareIn plus rc.hold (l2-0488, 3249e2cb7 and
7132d35f1) with l2-cjfix's map change (maph_shared takes -1 as held once;
a callv of a closure literal names its callee) and the httpsession
bench. Not on main: decision 0488 is Proposed.

## Mac (darwin/arm64, single runs, /usr/bin/time; not --ab claims)

| program | main (eda09dfa0 or d212bf31b) | l2-cjfix alone | 0488 + l2-cjfix |
|---|---|---|---|
| Mutex[List[Str]] 1,000, 20,000 writes | 0.02 to 0.03 s | | under 0.01 s (0488 alone) |
| Mutex[List[Str]] 100,000, 20,000 writes | 2.55 to 3.28 s | | 0.01 s (0488 alone) |
| httpsession store (1,000 sessions) | 0.47 to 0.56 s | 0.50 to 0.59 s | 0.52 s |
| httpsession store100k | not run | 7.08 s | 0.46 s |
| httpsession plain | 0.41 s | 0.41 s | 0.41 s |
| cookiejar both / get / set | 0.27 / 0.16 / 0.09 s | 0.18 / 0.10 / 0.06 s | 0.17 / 0.10 / 0.05 s |

## Host 1 (linux/arm64, 12 CPUs)

Self build, `ab-host.sh e622870f9 7132d35f1 -- self`, n=5 (0488 alone,
the per-store test's cost on xo's own code): wall 1.01 (0.99 to 1.02),
CPU 1.007 (0.993 to 1.019), max RSS 1.005 (0.991 to 1.03), all within
noise.

## Checks

- TestTSanSharedMaps (tsan_maps now also pushes and sets lists in place
  under the lock and reads their elements on other threads): 0488 alone
  10 of 10 in each mode; 0488 + l2-cjfix 5 of 5.
- Negative control: the same build with the ShareIn pass off (rl_copy
  still holding -1) fails 3 of 3 in every mode, TSan reporting races at
  tsan_maps main.xo:42 (the list writes). The test sees the bug the pass
  prevents.
- Goldens map, mutex, conc, chan, list, str, json, http_server on arm64
  and LLVM (0488 alone); map, mutex, conc, list, http_server, json on
  arm64 (combined).
- Combined, Mac: XO_FREE_CHECK=1 goldens (map, mutex, conc, chan, list,
  http server, h2c and client, json) 83 of 83 on arm64; 273 of 273 on
  LLVM with Go parity; wasm goldens (map, mutex, list, conc) pass.
- xo's native __text (Mac, `size -m bin/xo`): 33,147,308 at e622870f9,
  33,154,468 with 0488 alone: +7,160 bytes (0.02%).
- To do before acceptance: TestGoldenTasks under TSan x20 (host), the
  --ab rows on host 1 for httpsession and cookiejar.
