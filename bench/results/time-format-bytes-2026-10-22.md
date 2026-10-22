# time Layout.format with a byte buffer (2026-10-22)

Instructions retired (`/usr/bin/time -l`, native arm64, median of 2), program
timel (600,000 formats of an ISO and a long layout) and mail.compose
(200,000 messages with the Date header), before and after.

| program | before | after | change |
|---|---|---|---|
| timel format_layout (Layout.format) | 2,390 M | 1,637 M | -31.5% |
| timel format (Str layout each call) | 6,232 M | 5,626 M | -9.7% |
| mail.compose, hand written Date | 1,954 M | (unchanged code) | |
| mail.compose, Layout Date, old format | 2,211 M | | +13% against hand written |
| mail.compose, Layout Date, new format | | 1,911 M | -2% against hand written |

Go's time.Format on the same layouts: 955 M, so a Layout format goes from
2.5x to 1.7x Go. The text is unchanged: 2,625 combinations (five layouts
with every field and argument form, five zones including a -59 s one, 15
times from 1900 to 2100, 7 nanosecond values) were compared with the old
formatter's output.
