# 0658: an agent work system (xw): Postgres, an Xo CLI and a server-rendered Xo web view

Status: Proposed (owner asked for it, 2026-11-15; drafted by the lead).
The owner chose the server-rendered Xo web view over Grafana, Metabase
and a terminal UI. Build timing is the owner's: not before Lang 2's
artifact-keyed selective reruns land.

## Context

The lead and four workers coordinate through about ten separate stores,
none queryable and none kept over time:

- `~/xo-lead/agents/board.sh` (status lines and claims, written by hand),
- `~/xo-lead/notes/lead/todo.md` (the lead's event log),
- `~/xo-lead/notes/AGENT/` (reports and resume notes),
- heavyd's lanes and leases, pushq's queue, offload-gate logs,
- `~/xo-lead/notes/lead/agent-transcripts.txt` (session to transcript map),
- the ctier failures file, `go run ./scripts/stdcov` output,
- `isdone.sh` (a grep over git log), the lead's auto memory.

What this cost on 2026-11-15 alone:

- The board was stale: Lang 1's line dated 11-14, aws-43's 11-13. "What
  is everyone doing" took four screen reads of the agents' terminals.
- A decision crossed a report: aws-43 kept crypto C1 held after the
  owner said push it, because nothing records whether an agent has
  seen a decision.
- "How much did Lang 3 write vs test" needed a one-off script over a
  transcript; the gate failure audit (47 failed gates) was hand work.
- The lead's memory index silently lost about 35 entries past its size
  limit.
- The idle, bottleneck and main watchers are three shell loops re-deriving
  state from logs.

Status written by hand goes stale; the tools already see almost every
event (heavyd starts and ends every lease, pushq every push, gate.sh
every step, git every commit, the transcripts every tool call and its
tokens).

## Decision

Build `xw`, a single Xo program (CLI and server in one binary), backed
by Postgres.

1. **Events are recorded by the tools, not typed by agents.** heavyd,
   pushq, offload-gate, gate.sh and ctier call `xw event KIND key=value...`
   (fire and forget: a down server never fails or slows the caller; the
   event goes to a local spool file and is sent later). A git ingester
   reads origin/main; a transcript ingester reads each session's jsonl
   for tool calls, durations and token usage (counts only, never
   message text). Agents write only what no tool can infer: claims,
   blocks, decisions and acknowledgements.
2. **Tasks and decisions are records with states.** A task: queued,
   assigned, acknowledged, working, gating, pushed (hash), done, blocked
   (reason). A decision: who decided, when, which tasks it changes, and
   per agent whether it was acknowledged. `xw ack` closes the crossed
   message gap; the status page shows unacknowledged decisions.
3. **Postgres** (17, in a Docker container on the Mac Studio, data under
   `~/xo-lead/xw/`), opened by std/sql's native Postgres driver.
   LISTEN/NOTIFY (std/sql, decision 0648 item 14) feeds the alert loop.
   Full-text search (tsvector) over notes, gotchas, decisions and
   reports first; pgvector only in stage 4, kept if it beats full text
   on a fixed set of 20 lead questions.
4. **Web view: server-rendered HTML from std/http and std/template**,
   no JavaScript, no build step, a meta refresh every 30 s, one small
   inline stylesheet that reads on a phone. Pages: `/` (each agent: task,
   state, last event, idle minutes, running jobs, tokens today), `/tasks`,
   `/gates` (queue, step durations, failures by cause), `/std` (stdcov
   by axis over time), `/decisions` (pending owner, unacknowledged),
   `/search?q=`. Read-only in stage 1; writes stay in the CLI.
5. **Access:** the server listens on the Mac Studio's VPN address only
   (never 0.0.0.0, never the OCI host), with a token cookie from
   `~/xo-lead/xw.env`. The database holds no secrets: ingesters drop
   message text, and the private values of CLAUDE.local.md never enter
   an event (the CLI refuses values matching the patterns of
   `~/xo-lead/xw-redact.txt`).
6. **CLI:** `xw status [AGENT]`, `xw task add|claim|ack|start|block|done`,
   `xw decide TEXT --affects TASK...`, `xw ack DECISION`, `xw event`,
   `xw search TEXT`, `xw sql QUERY` (a read-only role). Output is short
   text by default and `--json` for scripts, in the style of decision
   0261's agent commands.
7. **It replaces, not joins, the old stores.** Stage 2 turns board.sh
   into a wrapper over `xw task`, retires todo.md for `xw decide` and
   events, and isdone.sh becomes a query. Stage 3 replaces the idle,
   bottleneck and main watchers with one alert loop on the same data.
8. **Code** in `scripts/lead/xw/` (an Xo module), installed by
   `scripts/lead/install.sh` like the other lead tools. Tests use
   `sql.memory` where the queries allow, plus one Postgres test in the
   container.

### Stages

| Stage | Contents | Done when |
|---|---|---|
| 1 | schema; `xw event` and spool; heavyd, pushq and git ingest; `/` and `xw status` | the lead's "what is everyone doing" is one `xw status`, matching the agents' screens |
| 2 | tasks, decisions, acks; board.sh wrapper; todo.md retired | a decision sent to an agent shows unacknowledged until it acks |
| 3 | transcript ingest (tool time, tokens); `/gates`, `/std`; alert loop replaces the three watchers | writing vs testing for any agent and window is one page |
| 4 | full-text search; pgvector trial | search answers the 20 fixed questions; vectors kept only if better |

## Consequences

- One worker for about a week, taken from the std Proven push; the
  owner sets when.
- Xo's std gets daily real use (http, template, sql with Postgres,
  json, cli, time), which is evidence for std Proven and the start of
  an "Xo for agent infrastructure" product if the owner later wants
  one.
- A Postgres container runs on the Mac Studio full time, small next to
  the gates; Docker Desktop is already up for host 3.
- If xw is down, nothing else stops: the tools spool events and work
  as today.
- Agent instructions (CLAUDE.md, agent-tools.md) change at stage 2 to
  `xw task` and `xw ack`.

## Alternatives

- Grafana or Metabase on Postgres: no UI code and strong charts, but
  owner chose the Xo view (dogfooding, phone over VPN, one binary).
- SQLite (std/sqlite) instead of Postgres: no container, but many
  concurrent writer processes, LISTEN/NOTIFY and pgvector favor
  Postgres.
- A terminal UI: Xo has no TUI library yet; not readable from a phone.
- Keep the files and add more scripts: today's state; it is what went
  stale.
