# http1.query_parts: a plain query is read in place (2026-10-23)

`xo bench --ab --rounds=10`, load 5.2 to 5.4, 0 loaded rounds. Rows:
`qrow` (300k `http.new_request` calls over four URLs with plain queries:
`?q=xolang&page=2&sort=asc&lang=en`, an `id/fields/expand` query, eight
single letter pairs, and one with `%20`, `+` and a repeated key) and `up`
(400k calls over four URLs, two with queries, one with `%20`, one with `+`).
Go twin: `http.NewRequest` plus `URL.Query()`.

| row | comparison | wall | instructions | max RSS |
|---|---|---|---|---|
| qrow | old code (A), new (B) | 0.821 (0.80 to 0.848) | 0.756 | same |
| up | old, new | 0.98 (0.94 to 0.999) | 0.937 | same |
| qrow | Go (A), new (B) | 0.971 (0.742 to 1.042) within noise | 0.827 | 0.172 |
| up | Go, new | 0.704 (0.534 to 0.839) | 0.722 | 0.173 |

Change: `query_parts` finds `&` with `scan.find_byte` (the count of pairs too),
checks for `;` once, and a query with no `%` or `+` that is valid UTF-8
makes every key and value a `raw.str_cut` view of the query.
