# 0504: xo debug --dap, the Debug Adapter Protocol over decision 0253's session

Status: Accepted (owner, 2026-11-04) as the plan: built only after
decision 0253 stage 2's live process control. Proposed by Xo Lang 2 the
same day at the lead's request.

## Context

spec/core.md 11.5 said `xo debug --dap` speaks the Debug Adapter Protocol
for editors; nothing in compiler/ does (the owner's v0.38.1 "not tested
yet" list; the spec was corrected in 8b6299554 to mark it [later]).
Decision 0253 (accepted 2026-10-17) designs `xo debug`: its own DWARF
reader, unwinder and process control, in Xo, with a batch mode, a
prompt and a `--json` line protocol (stage 5), and stages 1 to 6 for
macOS arm64, then Linux and Windows. DAP is not one of its stages. Built
today: stage 1, stage 2's parts that call no OS, the unwinder, the batch
session and the `--json` protocol v1, on a recorded thread (`xo dev
debug --snap`). Not built: the live process control of stage 2 (so no
`xo debug` command), step, next and finish (stage 3), tasks (stage 4).

Editors (VS Code, Zed, Neovim's nvim-dap, JetBrains through its DAP
support) attach debuggers through DAP: a JSON-RPC-like protocol over
standard input and output with `initialize`, `launch`/`attach`,
`setBreakpoints`, `threads`, `stackTrace`, `scopes`, `variables`,
`continue`, `next`, `stepIn`, `stepOut`, `evaluate`, `disconnect`
requests and `stopped`, `exited`, `output` events.

## Decision

Add DAP as decision 0253's stage 7: `xo debug --dap [PROG]` serves DAP
on standard input and output, mapping each request onto the session's
commands (compiler/debug session.xo) so DAP and `--json` share one
implementation:

- `setBreakpoints` -> `break FILE:LINE` per line (verified when it
  resolves to addresses); `continue`, `next`, `stepIn`, `stepOut` ->
  continue, next, step, finish; `stackTrace` -> backtrace (inlined calls
  as frames, as `--json`); `scopes`/`variables` -> locals and print with
  a field path as the variables reference; `threads` -> threads, and
  with stage 4 each task as a DAP thread (tasks are what an Xo
  programmer debugs); `evaluate` -> print's field path.
- It comes after stage 2's live process control on macOS arm64 (DAP over
  a recorded thread is of little use in an editor), and gains Linux and
  Windows with stage 6.
- Tests: a scripted DAP client (Go, as the other protocol harnesses)
  that launches a golden program, sets a breakpoint, steps, reads
  locals, and checks each response, beside session.xo's goldens.
- An editor extension (VS Code's launch configuration) is a separate
  item.

## Consequences

- One debugger behind three front ends (batch, `--json`, DAP).
- spec 11.5 documents `--dap` when it is built; until then it stays
  [later], as 8b6299554 left it.
- Every session command DAP needs must exist first: DAP cannot ship
  before stages 2 and 3.

## Alternatives

- Ship a DAP adapter that drives lldb-dap or Delve: against decision
  0253 (no lldb or gdb to install, Xo's own views) and Delve sees only
  the Go backend.
- Drop DAP from the spec and leave editors to the `--json` protocol:
  every editor would need its own adapter.

## Implementation

- 2026-11-05 (Xo Lang 2, with decision 0253 stages 2 and 3):
  compiler/cli/servers dap.xo, `xo debug --dap`. Requests become session
  commands through exec_json (the --json protocol), parsed with the
  servers' JSON reader; framing is LSP's (Reader, send). The
  `initialized` event follows the launch response, so breakpoints are set
  on a live session; configurationDone runs the program. A variables
  reference is (frame, field path), numbered per stop. The program's
  standard output and error go to a temporary file (std/internal
  dbg_launch's `out`: posix_spawn file actions, stdin /dev/null), whose
  new bytes are an output event at each stop, since the adapter's
  standard output is the protocol. The editor's stack starts at the frame
  the stop selected (a next or finish out of an inlined call selects the
  frame it was about; DAP has no selected frame). A relative source path
  is resolved against launch's cwd, std's (`./std/...`) against xo's std
  directory. The --json frames gained `path` (spec 11.5). Test: cmd/xo
  TestDebugDAP, a Go DAP client (breakpoint by full path, stack and
  source paths, locals and a struct's fields, evaluate, next, stepOut,
  stepIn into std with a source file that exists, output, exit code). Not
  yet: attach, pause (one thread of control: requests wait while the
  program runs), conditional breakpoints, an editor extension.
