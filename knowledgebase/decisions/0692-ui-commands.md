# 0692: std/ui commands, server side effects a Msg asks for

Status: Proposed (xo-4c, 2026-11-17)

## Context

Decision 0664 stage 4b plans commands: an App's update stays a pure
function of (model, Msg), so a Msg that should send an email, charge a
card, call another service or start a timer had no place to say so. A
program did it beside the Host (after `apply`, or in an `allow` hook),
which runs on whichever instance took the sync, runs again when a
group-commit batch loses an append race and is replayed, and cannot
bring a result back to every replica. std/ui is on the std Proven push
(task #186) and its feature audit lists commands as not built.

## Decision

- `ui.commands(tasks, of, run)` makes a `Commands[M, E, cx]`: `of(model,
  msg)` plans a list of commands for a Msg from the model before it (pure,
  like update); `run(c)` performs one and may answer a Msg.
- `host(..., commands:)` and `host_with(..., commands:)` take it. The
  Host plans for each Msg it commits, after the append succeeds, by
  replaying update over the batch from the model before it; Msgs the
  Host refuses (`allow` says no) plan none. Commands run in tasks of `tasks` after the batch is published,
  on the instance that committed it.
- A command's Msg is applied with `try_apply` as a server Msg
  (`SERVER_CLIENT`), so it is logged, every replica sees it, and it may
  plan commands of its own.
- At most once: a crash between the append and the end of a command loses
  it. An instance that loses an append race replays the winner's Msgs but
  does not run their commands (the winner does), so with several Hosts on
  one store each command runs once.
- Clients and `predict` never plan or run commands.
- The Host's type carries the commands' effects: `Host[M, E, fx, bx, lx,
  cx]`; `try_apply` and the sync handler perform `cx`. A Host without
  commands infers `cx` empty, so existing programs compile unchanged.
- `Host.commands_running()` counts commands started and not finished.

## Consequences

- Side effects sit next to the Msg that asks for them and run exactly
  where the Msg was decided, once, with results flowing back through the
  log like any other Msg.
- At most once is the honest guarantee without a durable outbox; a
  command that must not be lost (a payment) needs its own idempotency
  key and a reconciling Msg until a later stage adds the outbox.
- Planning replays update once more per committed batch (the cost is
  the App's update over the batch, only when commands are given).
- Tests: std/ui "a command runs once for each Msg the host commits,
  planned from the model before it, and a refused Msg plans none" and
  "two Hosts on one store run each command once, after a lost race too"
  (both under `--race` and `--explore=100`).

## Alternatives

- Elm's `update -> (model, Cmd)`: changes every App's update type and
  makes the client compute commands it must not run; rejected for a
  separate planner with the same inputs.
- A `with_commands` method on a built Host: the effect parameter would
  have to change after construction; an option at construction keeps the
  type fixed.
- A pull runner (the program asks the Host for pending commands): every
  program writes the same loop and must know which instance owns a batch.
- A durable outbox in the store (commands written with the batch, marked
  done by a later Msg): at least once across crashes; a later stage, on
  top of this API (`of` and `run` stay the same).
