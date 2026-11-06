# std twin timego: format and parse rows (2026-11-06)

Host 2 (linux/arm64, 18 CPUs, OFF_EXCLUSIVE=1), `xo bench --ab`, n=5, Xo 0.41.0. Ratio is Xo wall over Go wall (time.Format and time.Parse, go1.26).

## First row: layout translated on every call (4e41ef13b5)

| workload | Go ms | Xo ms | ratio | interval | RSS ratio | verdict |
|---|---|---|---|---|---|---|
| format | 30.49 | 458.3 | 15.03 | 14.38-15.18 | 0.154 | worse |
| parse | 8.332 | 212.1 | 25.46 | 23.99-26.49 | 0.275 | worse |

The cost is translating the Go layout and parsing the core layout text on every call (time.md: a string layout costs 5.4x and 4.7x
against a pre-parsed Layout). `timego.layout(go_layout)` translates once into a time.Layout.

## After `timego.layout` (bench/perf/timego translates once)

Xo at 89509d0e2 (the layouts translated once with `timego.layout`, then `format` and `parse` on the pre-parsed core Layout).

| workload | Go ms | Xo ms | ratio | interval | RSS ratio | verdict |
|---|---|---|---|---|---|---|
| format | 29.84 | 37.2 | 1.247 | 1.195-1.257 | 0.182 | worse |
| parse | 8.287 | 27.59 | 3.329 | 3.079-3.558 | 0.272 | worse |

Format is within 25% of Go. Parse is 3.3x: the workload parses RFC 3339, which Go's time.Parse short-cuts with a hand written fast path
(parseRFC3339), while core's Layout parse is the general walk (the std-twin-rows2 rows put it at 1.9x on other layouts). Next step:
XO_PROF on parse, then a fast path in core time's Layout.parse for the RFC 3339 shape.
