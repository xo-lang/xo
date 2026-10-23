# DeepSeek/Pi experiment

Started: 2026-10-23. Status: running.

Can a non-Claude model, DeepSeek (v4.1 flash marathon, through the
Subconscious gateway) driven by the Pi coding agent (v1.4.0), work on Xo
as a team member, safely and productively, the way the Claude Code
agents do? A Claude session, "Deepseek Overseer", gives it tasks, watches
its screen through crosswalk for unsafe or wasteful actions, and reports
to the lead. This page records the setup, what was observed, and the Pi
extensions and config changes it needs.

## Setup

Last verified: 2026-10-23.

- Session: crosswalk session "Xo Pi 1"; xo-inbox name `pi-<session prefix>`.
  The overseer sends tasks to that inbox and reads the screen.
- Working directory: the main checkout (`~/godev/claude-space/xolang/xo`).
- Pi config: `~/.pi/agent/settings.json` (provider `subconscious`, model
  `subconscious/deepseek-v4.1-flash-marathon`). No `mcp.json`, no
  `trust.json`, no skills, no project `.pi/` directory.
- Context files: Pi loads the repo's `CLAUDE.md` (and `AGENTS.md`).
  It does not load the owner's global `~/.claude/CLAUDE.md` (no em-dashes,
  never recommend stopping) or the Claude memory index, so those rules
  reach DeepSeek only if they are put in a file Pi reads.
- Global extensions in `~/.pi/agent/extensions/`:
  - `bash-guard.ts`: blocks pkill, killall, `kill -SIGNAL` without a
    target, `kill -f`, kill with several PIDs, pushes matching `xolang`,
    tag pushes, and `publish.sh`. Fails closed. Overrides
    `XO_ALLOW_KILL=1` / `XO_ALLOW_PUSH=1` in the command itself.
  - `xo-inbox.ts`: loads `~/xo-lead/channels/inbox/pi/xo-inbox.ts`, the
    channel the lead and other agents message through.
  - `subconscious-compaction.ts`, `subconscious-image-window/`: gateway
    accounting, observational only.

## Pi's security model (from its docs/security.md)

Pi has no permission prompts and no sandbox by default: every tool call
runs with the user's permissions. Project trust only gates loading
`.pi/` resources, not what tools do. Isolation is the user's job
(container, VM, or the example `sandbox` extension, which wraps bash in
macOS `sandbox-exec` through `@anthropic-ai/sandbox-runtime`). Extensions
can block any tool call from a `tool_call` handler, which is how
`bash-guard.ts` works.

## Gaps against Claude Code

| Claude Code has | Pi today | Needed |
|---|---|---|
| Permission modes and an auto mode classifier | nothing: every call runs | a guard extension per rule below; sandbox for bash |
| Hard safety rules (CLAUDE.md "Safety rules") | bash-guard.ts | close the bypasses (findings 1, 2) |
| Global owner rules (`~/.claude/CLAUDE.md`) | not loaded | an `APPEND_SYSTEM.md` or extension that appends them |
| Memory (feedback rules: heavy.sh, pushq, no commits in the main checkout, no git stash, worktree per push) | none | encode the hard ones as guards, the rest as an appended rules file |
| MCP (crosswalk, xo-inbox, docs) | xo-inbox through an extension; no MCP servers | probably none; the xo tools are CLI |
| Subagents, skills | none configured | not needed for single tasks |

## Findings

Each finding: what, evidence, fix. Status in brackets.

1. **bash-guard misses commands inside quotes.** [Open] `stripQuoted`
   removes quoted text before matching, so `bash -c "pkill -f x"`,
   `sh -c '...'`, `eval "..."` and `xargs` with a quoted command are not
   checked. Fix: match the guards on the raw command too when it contains
   `bash -c`, `sh -c`, `zsh -c`, `eval`, or `xargs`, or strip only after
   the first word of a quoted argument.
2. **The publish guard matches `xolang`, but the public org is
   `xo-lang`.** [Open, verify] `git push` to a URL or remote named for
   `xo-lang` is not blocked. Fix: match `xo-?lang` and the remote names
   publish.sh uses.
3. **No guard for the main checkout.** [Open] Memory rule: never commit in
   the main checkout; the session starts there. Fix: a `tool_call` guard
   blocking `git commit`, `git checkout`, `git rebase`, `git reset --hard`
   and `git stash` when cwd is the main checkout; tasks name a worktree.
4. **Builds and tests not forced through heavy.sh.** [Open] An `xo build`
   or `go test` run directly once reached 37 GB. Fix: a guard that blocks
   `go test` (without `-short`/`-run` of a few tests), `go run
   ./scripts/xobuild`, `xo build`, and `scripts/bootstrap-xo.sh` unless the
   command starts with `~/xo-lead/agents/heavy.sh`.
5. **Secrets readable and sent to a third-party gateway.** [Open]
   `CLAUDE.local.md`, `~/.pi/agent/auth.json`, `~/.aws`, the keychain
   (`security find-generic-password`) are readable by bash, and anything
   read goes to the Subconscious gateway. Fix: block `read` and `bash` on
   those paths and on `security find-generic-password`; or the sandbox
   extension with `denyRead`.
6. **Pushes outside pushq.** [Open] Fix: block `git push` to origin main
   unless it runs through `~/xo-lead/agents/pushq.sh`.

## Log

- 2026-10-23: setup surveyed (above); session idle, no task given yet.
