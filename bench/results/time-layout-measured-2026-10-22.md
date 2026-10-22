# time.Layout measured against Go (2026-10-22)

Instructions retired (`/usr/bin/time -l`, median of 5, setup run subtracted),
600,000 format calls and 600,000 parse calls, program `timel` (ISO and long
layouts), twins in Go. Mac arm64, commit of the time.Layout change.

| mode | Go time pkg | Xo native Str layout | Xo native Layout | Xo Go backend Str | Xo Go backend Layout |
|---|---|---|---|---|---|
| format | 955 M | 6,202 M | 2,375 M | 5,666 M | 5,522 M |
| parse | 471 M | 3,291 M | 1,586 M | 960 M | 888 M |

Native format goes from 6.5x to 2.5x Go (2.6x fewer instructions), parse from
7.0x to 3.4x (2.1x fewer). The estimate in time-layout-parse-cost-2026-10-21.md
(about 2,400 M for format) held: 2,375 M. The remaining cost is the item
walk and field formatting, not layout parsing. The Go backend gains little.
