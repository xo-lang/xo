# Brotli article sources (task #391), 2026-11-19

The raw outputs the brotli article cites, beyond
brotli-enc11-381-2026-11-19.md and brotli-float32-count-2026-11-19.md.
Each section names its commit, host and command.

## 1. update_nodes disassembly before and after #381 (darwin/arm64, the Mac)

Built from clean worktrees at 8ea9d8269 (the parent of fc8c0a611, #381)
and fc8c0a611: `go run ./scripts/xobuild`, then in bench/perf/brotli
`bin/xo build -o br .` (a debug build, for its line tables and variable
locations; it has the same number of stack accesses in update_nodes as
`xo build --release` at each commit: 517 at 8ea9d8269, 579 at fc8c0a611,
counted with `grep -c '\[sp'` over the function), then
`objdump -d -l --no-show-raw-insn --disassemble-symbols=_xo_f_brotli_2eupdate__nodes br`.
At both commits the source line is std/brotli.xo:5109,
`let backward = q.dc[4 * qi + dci[j]] + dco[j]`, which appears 3 times in
the function; below is the first copy of each.

Stack slot names come from the debug info (`lldb -b -o 'image lookup -v -a ADDR' br`
at the line's first instruction): before #381 qi is sp+0x90 (WSP+144),
dci sp+0x130, dco sp+0x128, j sp+0x60, and q's second field (dc) sp+0xe8;
after it j is in register x8 and qi, dci, dco and q.dc are at sp+0x80,
sp+0x120, sp+0x118 and sp+0xd8. Over the whole function the stack
access count rose (517 to 579); this line's fell (9 loads to 6).

### 8ea9d8269: 28 instructions, 9 loads from the stack

```
ldr	x16, [sp, #0x90]
lsl	x10, x16, #2
ldr	x16, [sp, #0x130]
ldr	x9, [x16, #0x8]
ldr	x16, [sp, #0x60]
cmp	x16, x9
b.hs	0x100039ea0 
ldr	x16, [sp, #0xb8]
ldr	x17, [sp, #0x60]
ldr	x9, [x16, x17, lsl #3]
adds	x10, x10, x9
b.vs	0x100039ec0 
ldr	x16, [sp, #0xe8]
ldr	x9, [x16, #0x8]
cmp	x10, x9
b.hs	0x100039ee8 
add	x9, x16, #0x18
ldr	x10, [x9, x10, lsl #3]
ldr	x16, [sp, #0x128]
ldr	x9, [x16, #0x8]
ldr	x16, [sp, #0x60]
cmp	x16, x9
b.hs	0x100039f08 
ldr	x16, [sp, #0xc0]
ldr	x9, [x16, x17, lsl #3]
adds	x16, x10, x9
b.vs	0x100039f28 
str	x16, [sp, #0x70]
```

### fc8c0a611: 25 instructions, 6 loads from the stack

```
ldr	x16, [sp, #0x80]
lsl	x10, x16, #2
ldr	x16, [sp, #0x120]
ldr	x9, [x16, #0x8]
cmp	x8, x9
b.hs	0x10003b1bc 
ldr	x16, [sp, #0xa8]
ldr	x9, [x16, x8, lsl #3]
adds	x10, x10, x9
b.vs	0x10003b1e4 
ldr	x16, [sp, #0xd8]
ldr	x9, [x16, #0x8]
cmp	x10, x9
b.hs	0x10003b214 
add	x9, x16, #0x18
ldr	x10, [x9, x10, lsl #3]
ldr	x16, [sp, #0x118]
ldr	x9, [x16, #0x8]
cmp	x8, x9
b.hs	0x10003b23c 
ldr	x16, [sp, #0xb0]
ldr	x9, [x16, x8, lsl #3]
adds	x16, x10, x9
b.vs	0x10003b264 
str	x16, [sp, #0x60]
```

The Float32 path, std/brotli.xo:5098 (`let min_cost0 = q.cost[q0] + ...`),
the same 5 instructions at both commits (8ea9d8269 shown):

```
ldr	s16, [x9, x10, lsl #2]
fcvt	d16, s16
fcvt	s17, d16
fcvt	s16, d8
fadd	s18, s17, s16
```

## 2. Debug info in release binaries (darwin/arm64)

`dwarfdump --debug-info br | grep -c DW_TAG_variable`: 0 for
`xo build --release` at 8ea9d8269, fc8c0a611 and 16efc3b56; 22,422 for the
debug build at 8ea9d8269 (2,890 subprograms).

## 3. Twin timings on the Mac (darwin/arm64), commit 3a80fdd33

`XO_CLI=bin/xo go run ./scripts/twincheck -time 9 bench/perf/brotli`
(xotool: commit 3a80fdd33b0a, on main):

```
time bench/perf/brotli/none xo 5.0 go 7.3 ratio 0.680
time bench/perf/brotli/text xo 72.1 go 110.1 ratio 0.655
time bench/perf/brotli/html xo 35.0 go 38.7 ratio 0.906
time bench/perf/brotli/mixed xo 35.3 go 44.0 ratio 0.801
time bench/perf/brotli/enc1 xo 53.1 go 67.7 ratio 0.784
time bench/perf/brotli/enc6 xo 48.8 go 63.9 ratio 0.763
time bench/perf/brotli/enc11 xo 364.9 go 276.3 ratio 1.321
twincheck: 7 cases, 0 mismatches or failures, 0 directories skipped
```

## 4. xo bench --ab on host 1 (linux/arm64), enc1

One run each, `ab-host.sh BASE NEW NAME -- bench/perf/brotli enc1 800`
(the bench's repeat argument cherry-picked onto both revisions, so the
A and B revisions printed are those bundles).

std-side single 8-byte load (ld64 through std/internal le_word, bounds
checked once), 12 rounds; the tool marks 38 ms runs "too short":

```
ab-host l2brab3: A e6c1d8b8e21 B 9df1a39350c on Linux aarch64, 12 CPUs
metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               38.77       30.35       1.788% 1.618%     0.783   0.776 to 0.798    0.784 0.785     too short (n=12)
cpu_ms                38.73       30.34       1.767% 1.671%     0.783   0.775 to 0.799    0.784 0.786     too coarse (n=12)
max_rss_kb            3728        3728        0.858% 0.858%     1.0     0.983 to 1.017    1.0 1.0         within noise (n=12)
load: median 1.165 (A) / 1.165 (B); loaded: 0 of 12 rounds (at or above 6.0; baseline 1.17 + 3.0, at least 6.0)
```

#417 load combining (6e433dda9 against its parent), 4 rounds:

```
ab-host l2ab417: A 90e7f35c05f B b82cf29f646 on Linux aarch64, 12 CPUs
metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               245.4       241.8       0.649% 0.593%     0.985   0.977 to 0.991    0.985 0.984     better (n=4)
max_rss_kb            3652        3720        3.505% 0%         1.019   1.001 to 1.037    1.001 1.037     worse (n=4)
load: median 0.95 (A) / 0.95 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.95 + 3.0, at least 6.0)
```

#418 inlining past a landing pad (af88679ee against its parent), 4 rounds:

```
ab-host l2ab418: A 1854aeee3aef B 74097e4c1e77 on Linux aarch64, 12 CPUs
metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               246.2       245.8       0.526% 0.297%     0.998   0.985 to 1.006    0.994 1.001     within noise (n=4)
max_rss_kb            3644        3652        0% 3.505%         1.002   0.985 to 1.02     0.985 1.02      within noise (n=4)
load: median 0.86 (A) / 0.86 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.86 + 3.0, at least 6.0)
```

## 5. Inlining of the bit writer and match length (16efc3b56)

Xo, `XO_INLINE_REPORT=1 xo build --release .` in bench/perf/brotli,
darwin/arm64:

```
inline brotli.dict_match_length: instrs 46 blocks 20 sites 2 loop
inline brotli.match_len: instrs 46 blocks 19 sites 4 loop
inline brotli.match_len_code_of: instrs 9 blocks 6 sites 1 small leaf
inline brotli.match_len_of: instrs 3 blocks 1 sites 8 small leaf
inline brotli.write_bits: instrs 70 blocks 16 sites 104 no (pads)
```

The same report at 5ffd0743b (the parent of af88679ee, #418, and so
before #418 and #417):

```
inline brotli.write_bits: instrs 70 blocks 16 sites 104 no (hard: pad before operands)
inline brotli.match_len: instrs 46 blocks 19 sites 4 loop
```

So match_len was inlined at both; write_bits at neither (#418 changed
the reason, not the outcome, for this program).

Go 1.26.11 with andybalholm/brotli v1.2.2 (bench/perf/brotli/twin.go.txt as
main.go), `go build -gcflags='github.com/andybalholm/brotli=-m'`: writeBits
and findMatchLengthWithLimit are both "can inline"; writeBits is inlined at
151 call sites (of 196 calls in the module's sources, `grep -hE '\bwriteBits\(' *.go`
less comments and the definition, v1.2.2) and findMatchLengthWithLimit at 21, among them
compress_fragment_two_pass.go:334 and :401 (createCommands, quality 1)
and :655 to :772 (storeCommands). Xo's write_bits is inlined nowhere
("no (pads)": its callers have landing pads).

## 6. enc1 CPU profile (darwin/arm64, 16efc3b56)

`xo build --release --profile-support`, `XO_PROF=cpu ./br enc1 200`,
`xo prof`:

```
xo cpu profile: 606 samples (6060ms) in 7346ms of wall time at 100 Hz

top 30 by flat
      flat  flat%   sum%        cum   cum%  line (the hottest)  function
    2300ms 37.95% 37.95%     3290ms 54.29%  -                   brotli.create_commands
    1690ms 27.89% 65.84%     1690ms 27.89%  -                   brotli.write_bits
     820ms 13.53% 79.37%      820ms 13.53%  -                   brotli.after_match_n
```

In `go tool pprof -top -addresses`, the samples inside create_commands
fall on only 12 distinct addresses, so this profile does not resolve
single instructions; the article claims only the function split.

## 7. Work counters and the two rewrites (darwin/arm64, 2026-11-18)

Experiments in a scratch tree off main of 2026-11-18 (before #381),
html input at quality 11. Xo: counters added to std/brotli.xo's
update_nodes, printed by a deliberate fault after one encode:

```
fault: cnt calls 200086 cache 578875 l 3367580 m 12101411 at std/brotli.xo:3035:22
```

Go: the same four counters in a vendored copy of andybalholm/brotli
v1.2.2's updateNodes, 20 encodes per run (rerun 2026-11-19):

```
calls 4001720 cache 11577500 l 67351600 m 242028220
```

Divided by 20: 200,086, 578,875, 3,367,580 and 12,101,411, the same as
Xo's.

Twin timings of enc11 on the Mac (`go run ./scripts/twincheck -time 5
bench/perf/brotli`) in that scratch tree while update_nodes was being
rewritten, in time order (2026-11-18):

```
21:21 time bench/perf/brotli/enc11 xo 335.2 go 208.7 ratio 1.606
21:23 time bench/perf/brotli/enc11 xo 357.6 go 228.0 ratio 1.568
21:25 time bench/perf/brotli/enc11 xo 333.6 go 208.3 ratio 1.601
```

The working note of that session (not committed) maps these to the
split into un_cache and un_matches (1.57, 148 stack accesses in the
smaller function) and to rb_match and zn_update written in line (1.60,
210); the logs do not record which edit each run had, so the article
says only that enc11 stayed between 1.57 and 1.61 of Go through both.

Profiles of 20 enc11 encodes on the Mac, the same scratch tree. Xo (`XO_PROF=cpu`, 2026-11-18):

```
xo cpu profile: 333 samples (3330ms) in 3427ms of wall time at 100 Hz
    1660ms 49.85% 49.85%     2510ms 75.38%  brotli.xo:5109      brotli.update_nodes
```

Go (pprof of the counting build above, rerun 2026-11-19; `go tool pprof -top -cum`):

```
Duration: 2.24s, Total samples = 1.77s (79.10%)
     0.65s 36.72% 37.85%      1.21s 68.36%  github.com/andybalholm/brotli.updateNodes
```
