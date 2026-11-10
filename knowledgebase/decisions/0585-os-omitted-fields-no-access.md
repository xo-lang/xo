# 0585: An omitted `Os` field is its no-access value

Status: Accepted (rule: owner, 2026-11-10, option B, relayed by the lead);
the per-field values below are Proposed (Xo Lang 1, 2026-11-10) until
implemented. Related: 0004 (effects and capabilities), 0156
(attenuation), 0307 (user capabilities and `Os` literals), 0584 (adds
`Os.secrets`, which relies on this).

## Context

`Os` (std/prelude.xo, core 6.3) is a struct of capabilities, and a
program narrows authority by building one with a literal:
`Os { fs: os.fs.sub("data"), net: os.net.only(["api.x"]), ... }`. The
checker requires every field without a default (`XO0403`), so each new
`Os` field (0584's `secrets` next) breaks every such literal in every
program and library: the one compatibility hazard of growing `Os`.

## Decision

1. **Rule.** A field left out of an `Os { ... }` literal (and so of the
   native runtime's `OsFields` record it lowers to, decision 0307) is
   that capability's no-access value. A field written in the literal is
   unchanged. Implemented as defaults on `Os`'s fields in
   std/prelude.xo, so the checker's existing default rule applies and
   `XO0403` still names a field that has no no-access value.
2. **Why least authority makes it safe.** An omitted field grants
   nothing: code given the literal can do no more than with a value
   attenuated to nothing (0156), so leaving a field out can only take
   authority away, never add it. A forgotten field fails closed (a
   `Denied` error at the first use), never open.
3. **No-access values**, each an existing attenuation of the program's
   own capability, so it reports the errors that attenuation already
   documents:

   | Field | No-access value | Behavior |
   |---|---|---|
   | `fs` | `fs.read_only().write_only()` | every read and write fails with `FsErr.Denied(path)` |
   | `net` | `net.only([])` | `dial` and `listen` fail with `NetErr.Denied(addr)`, `lookup_host` is refused as `only` documents; `interfaces` is checked in step 1 and denied if `only` does not already deny it |
   | `env` | `env.only([])` | `get` is `None`, `vars` empty, `home`/`config_dir`/`cache_dir` `Missing`; `args` and `program` unchanged (as `only` documents) |
   | `proc` | `proc.only([])` | `run`, `cmd` and `kill` fail with `Denied`; `exit`, `set_grace`, `pid`, `ppid`, `usage` unchanged (as `only` documents) |
   | `stdio` | a discarding Stdio | output dropped, input at end (`read_line` `None`, `read_all` ""), every `*_is_tty` false, `width` `None`, `read_secret` `None`, `raw_input` false |
   | `secrets` (0584) | `Secrets.only([])` | every store `None` |

   **No no-access value, still required** (`XO0403` as today):
   `clock` (a clock must answer `now`; a frozen one changes program
   behavior silently, and sleeping would spin), `rand` (a fixed seed
   would silently weaken keys made from it), `tasks` (a group has no
   empty form: spawning must run its task). A literal writes them, as
   `clock: os.clock`.
4. **New fields.** A field added to `Os` from now on must have a
   no-access value, and it is its default; a field that cannot have
   one needs its own ADR, since adding it breaks literals.
5. **Narrowing field by field** is unchanged: an explicit field is the
   value written, attenuated or not; omission is equivalent to writing
   the no-access value. `os` itself (main's parameter) is the runtime's
   full `Os`, never affected. `TestOs` is not an `Os` literal and keeps
   its fakes.
6. **Backends.** Each no-access value is the program's root capability
   attenuated as in the table (native: the llrt root records, Go: std/rt
   root values, wasm the same), reached by declaration-only prelude
   functions used as the defaults, so the default works in any function,
   not only where main's `os` is in scope.

## Consequences

- Adding `Os.secrets` (0584) and later fields breaks no program.
- A literal that forgets a field compiles and fails at run time with
  `Denied` instead of at compile time; `xo audit` and `xo vet` can list
  omitted fields of a literal (a later vet note, not part of this ADR).
- Behavior of literals that name every field is unchanged.

## Alternatives

- **Keep every field required** (option A): each new field is breaking.
- **Omitted = the enclosing function's capability**: grants authority
  silently; rejected by least authority.
- **No-access Clock/Rand** (frozen clock, fixed seed): silent behavior
  and security changes; rejected, they stay required.
