# std/cookiejar against Go, 2026-11-03 (Xo Lang 2)

Linux/arm64 offload host (the first, exclusive). bench/perf/cookiejar at
1d445d2a7 against Go's net/http/cookiejar (Go 1.26.1): `xo bench --ab`
with Go as A and Xo as B, 15 rounds (twin-cj.txt; script twin.sh).

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| cookiejar | none | 0.867 | 0.528 | 0.609 | 0.577-0.632 | 0.152 |
| cookiejar | set | 135.1 | 128.4 | 0.951 | 0.843-1.348 (Go's spread 48%: no claim) | 0.09 |
| cookiejar | get | 120.3 | 170.2 | 1.415 | 1.397-1.421 | 0.087 |
| cookiejar | both | 208.9 | 308.4 | 1.476 | 1.459-1.49 | 0.089 |

Before (std-twin-rows12-2026-11-03.md): set 1.666, get 1.54, both 1.681.

## What changed, each against its parent (ab-host, 3 rounds, `both`)

| commit | change | wall B/A | file |
|---|---|---|---|
| (dropped) | take the domain's map out of the jar with remove, put it back | 1.032 worse | ab-cj.txt |
| (dropped) | no allocation per cookie in a lookup (has_dot_suffix, equal_fold, update of last_access) | 0.988 noise | ab-cj2.txt (first run) |
| (dropped) | the walk's binding kept from escaping | 1.008 noise | ab-cj2.txt |
| a host name is not parsed as an IP address | 4 failed netip parses (and their errors) per set | 0.908 | ab-cj3.txt |
| a domain's map changed through Map.update | no get and write back | 0.965 | ab-cj4.txt |
| compiler: Map.update's closure keeps no value | see below | 0.99 | ab-cj5.txt |

Controls: a plain map walk of 35 entries is 0.159 of Go's (bench/perf/mapiter,
twin-mapiter.txt), so iteration is not the cost; lists-only maps
(XO_MAP_LOG=0) gave the same ratio as the hybrid (twin-cj-nolog.txt).

## The map log form

XO_PROF (prof-cj2.txt) showed the jar's maps in decision 0291's log form.
A repro found why: Map.update's closure kept the value it was given for a
fault, so each write in it copied the nested map's lists and the second
copy turned it into the log form (gotchas/var-closure-kept-value-copies-map.md).
The compiler fix keeps no value for update (Mutex.with still does, core
5.4): an update loop on a local map of maps went from 43 to 18 profile
samples. The xo self build is unchanged (0.998, ab-updself.txt).
cookiejar gains little from it because its state sits in a Mutex: `with`
keeps the protected value for a fault, so the jar's map is copied and
runs in the log form; that is the largest cost left (the log form's put
and lookup, about 20% of `both`).
