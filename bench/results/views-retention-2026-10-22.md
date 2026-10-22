# Memory a stored view retains, and the compaction rule (2026-10-22)

Decision 0330 item 3. Design note for the owner; `clone()` is implemented,
automatic compaction is not.

## Measured

`mb/retain`: 2,000 request bodies of 1 MiB each; from each, a 16 byte
token (`b[1048000..b.len()]`) is pushed on a `List[Str]` and the body goes
out of scope. Native arm64, maximum resident set size:

| stored value | max RSS |
| --- | --- |
| the view | 2,100,871,168 bytes (2.1 GB) |
| `t.clone()` | 4,915,200 bytes (4.9 MB) |

Each stored token pins its whole 1 MiB body: a 427-fold difference. Before
views the slice copied and the program held 4.9 MB.

## The rule decision 0330 proposed

A view stored beyond the function (a struct field, a Map or List element, a
return from a parse) is compacted (copied) when it is under 1/4 of its
buffer and the buffer is over 4 KB, at the point it is stored; a view used
in place is never copied.

## What it takes, and the options

The compiler knows the store sites (field store, `List.push`, `Map.put`,
`return` of a Str that is not a literal); the runtime knows the sizes (a
view's length against its object's capacity). Options, cheapest first:

1. **No automatic rule, `clone()` only** (today). Matches Go
   (`strings.Clone` exists for this). Risk: the bug above, found only by a
   memory graph.
2. **Compact in the runtime at `List.push`, `Map.put` and struct field
   stores of a Str**: one check per store (`off != 0 && len * 4 < cap &&
   cap > 4096`), a copy only when it fires. Costs a compare on every Str
   store, and `Str`-typed generic code is where the stores are hardest to
   see.
3. **Compact at return from a parse function** only (std parsers: url,
   http headers, mime, toml): the std functions that keep tokens
   (http request fields, mime params) call `clone()` themselves. No
   compiler work; the library choosing, as Go's net/http does.

Recommendation: option 3 now (std stores long lived tokens through `clone()`
where a request body is the source), option 2 if user code shows the problem.
Decision for the owner.
