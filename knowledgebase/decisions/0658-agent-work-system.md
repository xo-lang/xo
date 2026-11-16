# 0658: an agent work system (xw): Postgres, an Xo CLI and a server-rendered Xo web view

Status: Accepted (owner, 2026-11-15: "the whole system is fine",
including the Postgres container; drafted by the lead). The owner chose
the server-rendered Xo web view over Grafana, Metabase and a terminal
UI. Built after Lang 2's artifact-keyed selective reruns land.

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
4. **Backend and CLI first; the web view last** (owner, 2026-11-15: the
   backend has the most to figure out, without a web front end's
   overhead). Stages 1 to 3 answer every question through `xw`
   commands; the web view (stage 4) is a thin layer over the same
   queries. **Web view: server-rendered HTML from std/http and std/template**,
   no JavaScript, no build step, a meta refresh every 30 s, one small
   inline stylesheet that reads on a phone. Pages: `/` (each agent: task,
   state, last event, idle minutes, running jobs, tokens today), `/tasks`,
   `/gates` (queue, step durations, failures by cause), `/std` (stdcov
   by axis over time), `/decisions` (pending owner, unacknowledged),
   `/search?q=`. Read-only in stage 1; writes stay in the CLI.
5. **Access** (owner, 2026-11-15): the server listens on 0.0.0.0 on the
   Mac Studio (the owner's phone reaches it over the VPN into the home
   network; no router port forward; never the OCI host), over plain
   HTTP (owner: fine for now; HTTPS with a self-signed certificate is the
   step up). The web view logs in with a username and password (stored
   as std/crypto's hash_password_argon2id, set by `xw user set`), giving
   a session cookie. Agents and tools use the CLI, which sends a bearer
   token from `~/xo-lead/xw.env` (one per agent, so events carry who
   sent them and a token can be revoked alone). The database holds no secrets: ingesters drop
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
8. **Schema changes through a new std/migrate** (stage 1; std has none,
   std-coverage.md lists migrations as planned): numbered `.sql` files
   embedded with std/embed, goose's file format, a version table, one
   transaction per migration, a Postgres advisory lock so two processes
   never migrate at once, forward only; `migrate.up(db, files)` and
   `migrate.status(db, files)`. It follows the std rules (golang-migrate
   and goose tests ported where they fit, std/changes, its own Proven
   marks).
9. **Code** in its own private repository, xo-lang/workstream (owner,
   2026-11-15: stays private for now), built by a dedicated agent,
   xw-1, which brings decisions to the lead. std/migrate is a std module
   and lands in this repository; the hooks in heavyd, pushq and gate.sh
   land here through pushq. Tests use `sql.memory` where the queries
   allow, plus one Postgres test in the container.

### Stages

| Stage | Contents | Done when |
|---|---|---|
| 1 | std/migrate; schema; the server's write API and per-agent tokens; `xw event` and spool; heavyd, pushq and git ingest; `xw status` | the lead's "what is everyone doing" is one `xw status`, matching the agents' screens |
| 2 | tasks, decisions, acks; board.sh wrapper; todo.md retired | a decision sent to an agent shows unacknowledged until it acks |
| 3 | transcript ingest (tool time, tokens); `xw gates`, `xw std`; alert loop replaces the three watchers | writing vs testing for any agent and window is one command |
| 4 | web view: login, `/`, `/tasks`, `/gates`, `/std`, `/decisions` over stage 1 to 3's queries | the owner reads status from a phone over the VPN |
| 5 | full-text search; pgvector trial | search answers the 20 fixed questions; vectors kept only if better |

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

## Addendum: the alert loop as built (xw-1, 2026-11-15)

Stage 3's alert loop landed as `xw alerts` (workstream 7c0b520). Every
minute the server turns `xw check`'s problems, the bottleneck review's
thresholds (bottlewatch3.sh: a run that waited over 30 minutes, 6 pushq
refusals in 3 hours, a worktree refused twice in a row, a BLOCKED memory
lease) and the main checks' results (hourly.log, ctier's runs.tsv) into
alert rows, one per problem while it lasts. `xw alerts --wait` blocks
until a new one and replaces idlewatch, bottlewatch and the alert half
of mainwatch (mainwatch still runs hourly.sh). Two departures from the
plan above:

- No LISTEN/NOTIFY: alerts change only on the server's one-minute pass,
  so the client asks every 20 seconds over loopback HTTP. That survives
  an xw restart, which a held LISTEN connection would not, and costs no
  model turn.
- hangcheck.sh is not folded in yet: on 2026-11-15 it reported the host
  VMs (Virtualization.framework 56G, VMware 32G) as RUNAWAY and every
  offload over 25 minutes, which as alerts would wake the lead each
  minute. It moved in later the same day (workstream 0dbece7) by class,
  with ~/xo-lead/xw-hang-ignore.txt's ignore lines and per-class minutes.

## Addendum: the web view as built (xw-1, 2026-11-15)

Stage 4's read-only view landed in workstream (private commit): `/`, `/tasks`,
`/alerts`, `/gates`, `/std` and `/decisions`, each a std/template html
mode page of `<pre>` sections holding the text the matching `xw` command
prints (the same functions, so a page and its command never disagree),
with the 30 second refresh and one inline stylesheet (phone width, dark
mode). Login is `xw user set NAME` (the password on stdin, 12 or more
characters, stored as `hash_password_argon2id`), which gives an HttpOnly,
SameSite=Strict session cookie for 7 days; the database keeps only the
token's SHA-256 digest, a failed login waits a second, and a new
password ends the user's sessions. Departures and what is left:

- `/alerts` is added (the alert loop is stage 3's main output); `/search`
  waits for stage 5.
- The server still listens on 127.0.0.1: moving to 0.0.0.0 (item 5) is
  `XW_ADDR=0.0.0.0:7480` in the server's environment after the owner
  sets a password, so the view is never reachable without one.
- Pages are preformatted text, not tables; tables and per-agent rows
  are the next step once the owner has used it from the phone.
