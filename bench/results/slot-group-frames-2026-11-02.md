# slot_group frames over 504 bytes (regression B), 2026-11-02

Change: compiler/arm64 regalloc.xo allocates a function again without
slot_group's runs when the slots its runs leave unused take the frame over
504 bytes (the single `stp x29, x30, [sp, #-N]!` prologue) or 4 KiB (the
page probe). Branch lang1-regb-bnarrow, measured at 3535a4ee8 on main
1ef022848.

## HTTP /alloc (bench/perf/jitter), linux/arm64 first offload host

Lang 4's alloc-points.sh: exclusive host, server on CPUs 0-5, loadgen on
6-11, `-path /alloc -d 10s -warmup 2s -awake`, 3 rounds with the arms
interleaved and rotated, ratios against v0.35.0's server (rps higher is
better, CPU per request lower is better). The HTTP/1.1 idle park is on, as
on main.

| arm | c32 rps | c32 cpu | c256 rps | c256 cpu |
|---|---|---|---|---|
| main 1ef022848 | 0.777 | 1.319 | 0.652 | 1.534 |
| B only (no slot past a gap) 815493087 | 0.991 | 1.013 | 0.832 | 1.202 |
| this change 3535a4ee8 | 1.002 | 0.994 | 0.835 | 1.197 |

An earlier job, same setup: main 0.776 / 0.660 rps; a guard keeping word
copies after word stores (store to load forwarding) 0.778 / 0.655, so
forwarding was not the cause; that guard with B only 0.991 / 0.833.
The rest at c256 is the park itself (park off: 1.017, Lang 4's l4pts5).

## Frames (Mac, the /alloc server's request path, `sub sp` or `stp` size)

| function | main | -slot_group | this change |
|---|---|---|---|
| http.respond | 0x240 (576) | 0x170 | 0x170 |

serve_h1 and its closures keep slot_group's frames (they cross no
threshold).

## Code size

xo's native __text (compiler/xo built by each tree's xo): 32,694,988 ->
32,695,348 (+360 bytes). B only: 33,812,924 (+3.4%), since its members
spill out of word order and it stopped grouping almost everywhere.

## Compile time

self --ab (ab-host.sh, linux/arm64 first offload host, 3 rounds, main 1ef022848 against this change): wall 1.003 (0.996 to 1.004), cpu 1.000 (0.999 to 1.001), RSS 0.995, all within noise; the host was loaded (median load 7) through the run, the cpu ratio is the reliable one.
