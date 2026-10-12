# 0175: Capability attenuation and resource budgets on the native backends

Status: Accepted, implemented (2026-10-12). `Net.only` and
`task.with_budget` (steps, running time, and allocation) build natively
(arm64 and LLVM, `--no-gc`) with the Go backend's output; `Env.only` and
`Proc.only` were already native (decision 0172) and are now covered by
the same goldens. Follow-up of decisions 0156, 0172, and 0173.

## Context

Decision 0156 gave `Net`, `Env`, and `Proc` an `only` attenuation and
left it planned natively (XO1201), because the native backends lowered
those capabilities to zero sized values. Decision 0172 made `Proc` and
`Env` carry their lists (a struct of one `Str`) and enforced `only` on
every `Proc` and `Env` path (`run`, `cmd` with `output`, `run`, `spawn`,
`pipeline`, `get`, `vars`, `home`). `Net` stayed a zero sized capability,
so `Net.only` was still XO1201. Decision 0173 added `task.with_budget` on
the Go backend only (native XO1201), with a plan: polls at loop headers
and call entries in the SSA builder, and an exact per task allocation
count in std/llrt's allocator.

Constraints: the same output as the Go backend; internal/ssa and
selfhost/ssa stay mirrored function by function (TestSelfhostSSA); C
runtime edits go into the existing files (decision 0157's packs).

## Decision

### 1. `Net` carries its attenuation lists

- Natively a `Net` (and a test's `FakeNet`) lowers to `xo.Net`, the same
  one field struct as `xo.Proc` and `xo.Env` (build_io.go `listsType`):
  the lists of `only` in one `Str` (`\u{1}` then entries ended by
  `\u{0}`), `""` for none. `os.net` is `""`. The intrinsics
  `net_lists` and `net_new` (std/native/rt.xo) read and make it.
- `Net.only` is the native prelude's `net_only` (capImpls): the entries
  trimmed and lowered (std/rt's `normHosts`) appended as one more list,
  so attenuating again intersects.
- The check is in C (std/llrt/xo_rt.c, `net_admits`): std/rt
  attenuate.go's `hostAdmits` (exact host, `*.domain` for subdomains
  only, `host:port`, brackets trimmed), Go's `net.SplitHostPort` ported
  for the addresses (an address it rejects is denied), and `:port`
  read as `0.0.0.0`. `rt.net_dial` and `rt.net_listen` take the lists
  first and report `NetErr.Denied(addr)` (status 6) before cancellation,
  the test world, or a name lookup, as onlyNet does.
- `http.client(net)` keeps the lists (`Client.lists`); `send` checks
  each request's host and port (80 or 443 by scheme) through
  `rt.net_admits` before the test network or the C client and fails with
  `ClientErr.Connect("denied by Net.only: host:port")` (IPv6 hosts in
  brackets); `rthttp.cl_do` takes the lists and checks each redirect
  after the 10 redirect limit, as std/rt's `CheckRedirect`: `Protocol("Get
  \"<Location>\": denied by Net.only: redirect to host:port")`.
- `sql.open` with an attenuated `Net` fails with std/rt's `SqlErr.Conn`
  text.

### 2. `task.with_budget` natively

- Polls. When the program calls `task.with_budget` in one of its own
  modules (`usesBudgets`, the same scan as internal/gen's), the SSA
  builder emits `rt.tick` and an unwinding check at the start of every
  loop iteration (`loopBound`, after a `bound` count) and at the entry
  of every function, method, closure, and test body of the program's own
  modules (not std or native code). Unlike the Go backend, leaf functions
  poll too: the builder has no call graph pass before it lowers a body,
  and the cost is paid only by programs that use budgets. A step is
  therefore one loop iteration or one call of a program function, which
  counts a few more steps than the Go backend for the same code; no
  output depends on the exact count.
- `with_budget` is lowered like `with_timeout`: f runs as the body of a
  scope; `rt.budget_open(scope, cpu, alloc, steps, loc)` (a negative
  ceiling faults with std/rt's text at the call) gives the scope a
  budget and makes it the context's; after the scope closes,
  `rt.budget_end` restores the enclosing budget and returns the resource
  whose ceiling was passed (1 cpu, 2 alloc, 3 steps), stopping the
  unwinding, or 0. A ceiling passed while the code around is canceled
  keeps unwinding, as std/rt's `CheckCancel`.
- Runtime (xo_rt.c "Resource budgets"): a context counts polls down from
  a quantum of 1024 (cut to land exactly on a `steps` ceiling, as
  std/rt); at zero it charges its steps, its running time, and its
  allocation to every budget of its chain (`refs` counted; a child
  holds its parent), checks the ceilings in std/rt's order (cpu, steps,
  alloc), and a passed one cancels the budget's scope body; the poll
  then sets the unwinding flag, unless the context runs shielded code,
  where past twice the ceiling it faults with `XO-F008`. Under a budget
  a poll is a cancellation point. A task started under a budget works
  under it (`xo_task_spawn` hands the spawning context's budget to the
  task). Without a budget a poll costs a call and a decrement.
- Running time is monotonic time outside waits: `park_wait_n` and a
  task run inline on a thread end the stretch (`bud_pause`) and start a
  new one when the context runs again (`bud_resume`).
- Allocation is exact per context: each heap counts the bytes its
  thread allocates (`xo_heap.abytes`, one add in `alloc_in`, plus a
  large block grown in place), and a context charges what the count grew
  by while it ran. The Go backend counts process wide (decision 0173),
  an upper bound; both fail the golden's allocation loop.

### Tests

- internal/llvm golden `net_only` (also arm64's TestGoldenRun):
  dial and listen allowed and denied (other host, name, no port, too
  many colons, IPv6, `:0`, subdomain rules, `host:port`, intersection,
  an empty list, trimmed and case folded entries), HTTP requests (other
  host, IPv6, default ports, case), redirects within and away (with a
  port), `sql.open`, `Env.only` and `Proc.only` on every path; identical
  on Go, LLVM, and arm64.
- internal/llvm golden `task_budget`: internal/gen's `budget` golden
  plus a budget without ceilings, a deadline around a budgeted loop
  (`TimedOut`), recursion through a function value, and a negative
  ceiling (the fault); identical on the three backends, no allocation
  live at exit without the fault line.
- internal/gen golden `test_net_only` (cmd/xo TestNoGCTests): FakeNet,
  FakeEnv, and FakeProc attenuated in tests; cmd/xo TestAttenuationFakes
  now passes natively too.
- cmd/xo TestSupplyChainAttacks runs its three run time rows again with
  `--no-gc` on each native backend.

## Consequences

- Every `Net` value natively is a struct of one string (two words), like
  `Proc` and `Env`.
- In a program that calls `with_budget`, every loop iteration and every
  call of a program function costs a runtime call and an unwinding check
  natively; programs without a budget are unchanged.
- Native and Go differences that remain: `XO-F008` natively names the
  `with_budget` call as its location (the Go backend names the poll);
  `Debug` of an attenuated capability prints `<net>` (Go:
  `<net only [...]>`); hosts are lowered ASCII only in dial and listen
  addresses and redirects (Go's `strings.ToLower` is Unicode).

## Alternatives

- **Net.dial and listen in Xo** (a prelude `net_dial` checking the lists,
  then the unattenuated dial): the prelude would have to call the
  primitive dial without going through `Net.dial` again, which needs a
  second intrinsic per operation; the C check also serves the client's
  redirects, which happen inside `cl_do`.
- **A budget on the task instead of the scope**: f runs inline as the
  scope body (as with `with_timeout`), so the context's budget pointer,
  switched at open and end, is the task's budget for that stretch.
- **Leaf analysis for entry polls** (Go's `mayRecurse`): needs a walk of
  each body before lowering it, in both builders; left for when the poll
  cost of budgeted programs matters.
- **A process wide allocation count** (Go's): simpler, but the native
  heaps are per thread already, so the exact count costs one add.
