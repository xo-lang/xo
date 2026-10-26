# 0289: Agent tools: one name resolver, text output at a terminal, symbols of a directory

Status: Accepted (owner, 2026-10-20: the fix-up list from the two tool
polls of 2026-10-20, through the lead)
Its output rule (text at a terminal, JSON otherwise) is superseded by
decision 0386 (2026-10-26): text everywhere unless --json.

## Context

Six sessions used the agent tools of decisions 0261 to 0268 for a day
and reported back (the lead's poll, round 2). The tools beat grep where
they worked, but the same failures came from several of them:

- `xo def` and `xo refs` resolved names only against the checked
  program's own declarations, by qualified-name suffix. `xo def Str.find`
  failed while `xo query def Str.find` worked; module-qualified names as
  code writes them failed (`check.Checker.check_dirs`, `tls.client`,
  `pool.park_wait`, `tlsio.client_with`).
- `xo refs pool.park_wait` missed tasks.xo's call (grep found it), and
  `xo query callers` answered for tasks.park_wait instead of pool's.
- `xo vet std/native` reported 5,909 check errors: the runtime checked as
  program code. There was no `xo vet --std-module` and no vet of a file.
- `xo doc std/tls` said "No exported declarations": std declaration
  files write no `pub`.
- `xo symbols --dir=DIR` listed every imported module's public names
  (1,289 entries for compiler/cli/servers) and none of the directory's
  private declarations.
- Output and flags: agent commands printed JSON only, `xo query` wanted
  its flags after the kind, `query type` needed `--dir` for a file
  outside `.`, and `--dir` at a workspace root failed with "no .xo files".

## Decision

1. One name resolver. `xo def`, `refs`, `doc`, and `xo query def`,
   `callers`, `effects` resolve a name as code writes it: `name`,
   `Type.member`, `module.name`, `module.Type.member`, an import's local
   name (`j.f` for `use std/json as j`), `main.name` (the program's own
   modules), a module path (`std/tls.client`), and a prelude method
   (`Str.find`). The agent commands use `xo query def`'s resolver
   (compiler/cli/semantic) and fall back to their suffix match for bare
   member names (`check_dirs`).
2. A std or runtime module a name refers to is loaded with the program
   whether it imports it or not: the checker's `preload` list
   (`std/NAME`, `native/NAME`, `native/llrt/NAME`), filled from the name's
   first segment or module path when std has that file. For `refs` and
   `callers`, every module of that tree loads (all of std, or all of the
   runtime), so cross-module uses are found. A name that resolves to
   nothing lists std functions with its last segment
   (`nosuchmod.park_wait`: did you mean `pool.park_wait`; the poll's
   `crypto.verify_pss` pointed to `rsa.verify_pss` until std/crypto
   gained its own on 2026-10-20).
3. `xo refs` of a std declaration searches std files too (the editor's
   find references and rename still search program files only).
4. `xo doc` documents all of a std declaration file (no `pub` there) and
   loads a std module named by path whether the program imports it or
   not. `xo doc std/tls.client` (a module path whose last segment names a
   declaration) is a declaration lookup.
5. `xo vet` takes a `.xo` file (its directory checked, that file's
   findings), a std directory (std, std/native, std/native/llrt: its
   modules loaded together as std, native mode for the runtime, one
   check), or `--std-module=M` (one module with its private test file, as
   `xo test --std-module` checks it; like that flag, left out of the help
   text, which names no `--std` flag, and documented in
   knowledgebase/infrastructure/agent-tools.md). `DIR/...` still means
   DIR (decision 0210).
6. Output: agent commands print text at a terminal and JSON otherwise;
   `--json` and `--text` choose. Errors go to standard error in text form
   and stay `{"error": ...}` on standard output in JSON form. Scripts and
   agents that read standard output from a pipe see the JSON they saw
   before.
7. `xo symbols [DIR]` lists the declarations of that directory's files,
   private ones included; `--pub` keeps only the public ones (`--all` is
   accepted and is the default).
8. A declaration whose text is not at hand (an interface method, a
   prelude method without its source position) gets a signature built
   from its checked parameters, result, error, and declared effects:
   `xo query def Fs.read` answers `fn Fs.read(path: Str) -> Bytes ! FsErr
   uses fs.read` (it answered an empty signature, Lib 1).
9. Flags anywhere for `xo query`; `query type` checks the file's own
   directory when `--dir` is not given; agent commands with `--dir` at a
   directory with no .xo files (a workspace or repository root) check the
   modules under it, as `xo query` does. Module discovery under a root
   skips the checker's std directory (a checkout's std/), whose modules
   load as std, never as program directories.

## Consequences

Landed in a1fc1cb4 (2026-10-20).

Measured 2026-10-20 on the Mac: every failing name from the poll
resolves with `xo def` (Str.find, tls.client, tls.Verify,
pool.park_wait, tlsio.client_with, check.Checker.check_dirs,
check.new_checker); `xo refs pool.park_wait` finds pool.xo:1764 and
tasks.xo:184 in 1.1 s; `xo query callers pool.park_wait` lists
pool.scope_join and tasks.park_wait_n; `xo vet std/native/llrt` checks
134 runtime modules with no check errors in 0.55 s, `xo vet std/native`
31 modules in 0.55 s; `xo symbols compiler/cli/servers` lists 532
declarations of its 13 files. TestAgentNames and TestVetStd (cmd/xo)
run the poll's commands; TestAgentCLI's symbols case now expects private
names by default and checks `--pub`.

A lookup that names a std or runtime module checks it with the program,
and `refs` or `callers` of one checks all of its tree: about 1 s for the
runtime.

Design note, type rename (`xo edit rename Type New`, not built yet):
`xo edit` (compiler/cli/semantic edit.xo) finds a function's uses by
walking calls, which does not reach a type's uses (type expressions,
struct literals, patterns, static calls `T.f`, receivers). The
identifier resolution that does (compiler/cli/servers resolve.xo, what
the editor's rename uses) lives in servers, which imports semantic. The
plan: move the identifier walk and resolution (Site, idents_of,
Res.resolve) into semantic, have servers use it from there, and rename a
type by rewriting every identifier that resolves to it, with the edit's
check-before-and-after rule unchanged.

Not in this decision (the same fix-up list, later; type rename and
rename speed were done the same day, see the addendum): fault report locals
(`XO_TRACEBACK=locals` prints `<unavailable>` in the faulting frame on
arm64: its variables' location ranges end before the cold fault stub
the fault runs from, and inlined frames' variables are not described at
all, compiler/arm64 dwvars.xo `dv.inl >= 0`); `xo edit rename` of types,
a diff in `--dry-run`, and rename speed.

## Alternatives

- Load every std and runtime module for every lookup: simplest, but it
  makes each `xo def` pay for the runtime's check.
- Keep JSON as the only output and leave text to `jq`: the poll asked
  for text at a terminal four times; JSON stays the default in a pipe.
- Vet each std module in its own check (the first version): 146 s for
  the runtime directory against 0.55 s for one check.

## Addendum (2026-10-20): type rename and rename speed

Item 7's rest, built as the design note above planned (landed in 643adbd9):

- The identifier walk and resolution (`Walk`, `idents_of`, `IdentSite`,
  `LspTarget`, `Res.resolve`) moved from compiler/cli/servers resolve.xo
  to compiler/cli/semantic ident.xo. The server keeps its names through
  aliases (servers idents.xo: `type Site = semantic.IdentSite`, and so
  on), so the editor's code did not change.
- `xo edit rename TYPE NEW` rewrites every identifier of the user files
  that this resolution resolves to the type: its declaration, type
  expressions, struct literals, patterns and variants, receivers, and
  type arguments. The edit's rule is unchanged: the program must check
  clean before and after, so a missed or colliding name rejects the edit.
  cmd/xo TestEditRenameType renames a struct and an enum written in all
  of those places across two files.
- Members (2026-10-21, Xo Publish, from the agent poll's round 2):
  `xo edit rename Type.field NEW` and `Enum.Variant NEW` rewrite every
  identifier the same resolution resolves to the member (declaration,
  selectors, struct literal keys, constructor arguments, patterns); a
  struct literal's key resolves by the literal's own type name, so a
  literal converted to an interface (`[Rect{w: 2}]` as `List[Area]`)
  counts. A field of a struct that derives Json or SqlRow keeps its name
  on the wire (decision 0277): its declaration becomes `new as "old"`, or,
  with a `case:` rule, the edit asks for an explicit key first. A variant
  of an enum that derives Json is not renamed (its encoding would change).
  `xo edit rename Iface.m NEW` renames the interface method, the method of
  that name of every user type that has all of the interface's methods,
  and every call of them. cmd/xo TestEditRenameMembers: the program checks,
  prints the same, and keeps its test passing after each rename.
- Rename speed: the edit's second check (of the edited program) reuses
  the first check's std parses (`Checker.parse_cache`,
  `semantic.load_session_pc`). The poll's 3 to 13 s were mostly the load
  on the machine: the rename's own CPU time was 0.6 s.
- Found while doing it: on a case-insensitive file system (macOS) the
  module preload took `Chan.new` for the runtime module chan.xo. A
  capitalized first segment names a type now, never a module, and module
  files are matched by listing their directory, exact case.
