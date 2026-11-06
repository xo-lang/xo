# Two xos sharing one back-end cache slowed the second build of each --ab round

Status: Observed (2026-11-06)

## Symptom

On the linux/arm64 offload host, `xo bench --ab` of small whole program
builds (0529 steps 1 and 2) gave B / A that moved by several percent
between runs of the same pair. The per-round values (`--json`,
`per_round`) showed the first build of each round about 20% faster than
the second, whichever side ran first. With 3 rounds the mix is two
A-first rounds and one B-first, so the effect leaks into B / A.

## Cause

The two xos shared one back-end cache (XO_BACKEND_CACHE_DIR, the same
HOME). Measured on the linux/arm64 host with json_decode_fields built
whole, 20 rounds each (2026-11-06):

| Setup | First slot A / B (ms) | Second slot A / B (ms) |
|---|---|---|
| A vs B, one shared cache | 193 / 177 | 240 / 226 |
| A vs B, a cache per side | 192 / 179 | 193 / 179 |
| A vs A, one shared cache | 192 / 194 | 194 / 193 |

The second build of a round came after the other xo's build, which left
the shared cache in its own state (entries keyed without the xo, or
evicted), so the second side rebuilt what its own previous build had
cached; the first build followed the same side's build (rounds alternate
A B, B A) and found its state. A against A, or a cache per side, has no
effect. The ratio over the slots happened to be close (0.932 shared,
0.930 per cache), but a few rounds with an uneven A-first/B-first mix
were not.

## Fix

`xo bench --ab` now warns when both sides of a build, run or test
share one back-end cache (same XO_BACKEND_CACHE_DIR, cache not off).

Give each side its own back-end cache: `--env-a=XO_BACKEND_CACHE_DIR=DIR/a
--env-b=XO_BACKEND_CACHE_DIR=DIR/b` (ab-host.sh's edit mode does; its self
mode builds with the cache off), and its own output with `{side}` (`-o
out-{side}`). The "same slot B/A" column of `xo bench --ab` (B's B-first
runs against A's A-first runs, and the second slots) shows the effect:
when the two disagree, the sides interfere.
