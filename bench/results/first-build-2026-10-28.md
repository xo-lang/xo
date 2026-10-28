# First build of a brand-new user, 2026-10-28

Every cache empty for each round (a fresh HOME: xo's user cache; GOCACHE, GOPATH and the module cache for Go), then one edit and its build. n=5 rounds, the three tools interleaved, on the development Mac (arm64), wall time from outside the process (perl Time::HiRes, which adds about 5 ms to every figure). xo main's first build goes whole program and, where separate compilation is the default, starts std's variant in the background (decision 0388, std_variant_peek); its edit build then waits for that variant first (bg wait). Script: ~/xo-lead/notes/xo-lang-4/scripts/firstbuild.sh.

## fib

| tool | first build ms (median) | range | edit build ms (median) | range | background variant wait ms |
|---|---|---|---|---|---|
| xo 0.33.0 (release) | 954 | 938 to 1058 | 245 | 237 to 248 | - |
| xo main (eaa59def6) | 954 | 946 to 1016 | 241 | 240 to 260 | - |
| go build (go1.26.1) | 1482 | 1450 to 1501 | 108 | 107 to 119 | - |

## HTTP server

| tool | first build ms (median) | range | edit build ms (median) | range | background variant wait ms |
|---|---|---|---|---|---|
| xo 0.33.0 (release) | 2620 | 2609 to 2642 | 554 | 549 to 560 | - |
| xo main (eaa59def6) | 2623 | 2608 to 2672 | 529 | 526 to 535 | - |
| go build (go1.26.1) | 3048 | 3013 to 3149 | 257 | 255 to 278 | - |

## Notes

- This is the baseline before separate compilation is the default (decision 0388 step d,
  Lang 1): xo main's first build is the whole program and starts no background build, so
  its edit build is a plain warm one. Main's rows are measured again when step d lands.
- A first build: xo is faster than Go's on both (fib 954 against 1482 ms, the HTTP server
  2623 against 3048 ms; Go compiles its runtime and standard packages into an empty
  GOCACHE). An edit build: Go's is about twice as fast (108 against 241 ms, 257 against
  529 ms), which separate compilation addresses (decision 0388 milestone: fib edit 111.5 ms).
- Raw rows: ~/xo-lead/notes/xo-lang-4/fb-baseline/.
