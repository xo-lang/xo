# 0410: std's small batch S8: text width and precision, a tab writer, a priority queue, host facts

Status: Proposed (lead request, 2026-10-29: items 12, 13 and 9 of the std-vs-Go survey, knowledgebase/infrastructure/std-coverage.md; Xo Lang 3). Stage by stage; each stage has its own std/changes entry, spec/std.md section and a timing row against Go where it has a hot path.

## Context

The survey found CLI and scheduler programs writing their own padding and fixed precision float text (`strconv.FormatFloat`, `%-10s`, `%08.3f`), an aligned table writer (`text/tabwriter`), a priority queue with `Remove` and `Fix` (`container/heap`), and no way to read the host name or the process id. None is a language change; all are std.

## Decision

1. **Width and precision, as methods (no format strings).** Xo keeps `${x}` and `${x:?}` (core 1.6) and adds no format mini language to interpolation. In the prelude:
   - `fn (s: Str) pad_left(width: Int, fill: Str = " ") -> Str`, `pad_right`, `center`: pad to `width` runes (Go's `%*s`, `%-*s`), `fill` one rune; a longer string is returned as it is (Go's width is a minimum).
   - `fn (n: Int) to_str_width(width: Int, base: Int = 10, zero: Bool = false, plus: Bool = false) -> Str` (`%08d`, `%x`, `%+d`): sign before the zero padding, as Go.
   - `fn (f: Float) to_fixed(prec: Int) -> Str`, `to_exp(prec: Int) -> Str`, `to_general(prec: Int) -> Str` (strconv.FormatFloat 'f', 'e', 'g' at 64 bits; `prec` < 0 the shortest text that round trips, as `${f}`), and `to_str_width` with the same width and zero rules.
   Ported from Go's fmt/format.go (fmtInteger, fmtFloat padding) and strconv's ftoa with its tests; the native path reuses the shortest-text and big-decimal code Float already has.
2. **`std/tabwriter`** (Go's text/tabwriter, ported with its tests): `TabWriter.new(min_width, tab_width, padding, pad_char, flags)`, `write(text)`, `flush() -> Str`; cells end at a tab, columns are aligned over runs of consecutive lines, flags AlignRight, Debug, TabIndent, DiscardEmptyColumns, FilterHTML omitted (no HTML escapes), StripEscape kept. It builds a Str (no Writer interface in Xo), so it is pure.
3. **`PriorityQueue[T]`** (container/heap's semantics on a binary heap of a List): `PriorityQueue.new(less: fn(T, T) -> Bool)`, `push(x)`, `pop() -> T?`, `peek() -> T?`, `len()`, `remove(i: Int) -> T?`, `fix(i: Int)`, `update`: an index lookup is the caller's, as in Go (the element moves report their new index through an `on_move: fn(T, Int)` option so a caller can keep its index; Go's heap.Interface Swap). A value type: copy-on-write like List.
4. **Host facts.** `Env.hostname() -> Str ! EnvErr` (`uses env`: the host's identity is environment, not a process action; `Env.only(names)` does not hide it, as Go's os.Hostname is not an environment variable, and a test's FakeEnv sets it); `Proc.pid() -> Int` and `Proc.ppid() -> Int` (`Proc`: the process's own identity; `Proc.kill` already takes pids). Both are native syscalls (gethostname, getpid; WASI: the host name is an error `Unsupported`, the pid is 1), no Net capability.

## Consequences

- Seven new functions on existing types and two small std modules; no compiler change except the std index (`go run ./scripts/stdsync`) and the Go backend's `std/rt` twins.
- Hot paths with a Go twin row: pad and fixed float (strconv.FormatFloat 'f'), tabwriter on a 10,000 line table, PriorityQueue push/pop against container/heap over 1M ints.

## Alternatives

- A `${x:>8.3}` mini language in interpolation: a language change with an ADR of its own; the methods cover the survey's cases and can be sugar for it later.
- `fmt.Sprintf`: format strings fail at run time; rejected in the language's design (typed interpolation).
