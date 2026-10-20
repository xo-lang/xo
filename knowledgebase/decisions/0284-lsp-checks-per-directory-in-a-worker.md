# 0284: `xo lsp` checks each open directory on its own, in a worker task

Status: Accepted (owner, 2026-10-20: "implement 1 and 2")

## Context

In VS Code, saving an Xo file took one to two seconds before the format
on save edits applied and the dirty dot cleared. The server
(compiler/cli/servers/lsp.xo) handled one message at a time, and every
edit marked its one check of every open document stale. Right after an
edit VS Code asks for code actions and semantic tokens; both rechecked
(Session.fresh), the check covering the directory of every open tab,
and the formatting request of the save queued behind it, although
formatting reads only the text (3 ms).

Measured on the Mac (2026-10-20, a script sending VS Code's messages
after each edit, scratch drive2.py): with one tab
(compiler/cli/servers/lsp.xo) the formatting reply came 0.3 s after the
edit; with seven tabs in different directories (compiler/check, ssa,
arm64, std, examples) 1.3 s, every edit, and 4.7 s for the first check.
The check's time is about the sum of each directory's own: `xo check`
took 0.59 s for compiler/arm64, 0.60 s for std, 0.35 s for
compiler/cli/servers, 0.34 s for compiler/ssa, 0.18 s for
compiler/check; a sample of the server put all of the recheck in
check_dirs.

## Decision

1. Checks run in a worker task. The server loop answers requests that
   read no check (formatting, documentSymbol, initialize, shutdown) at
   once. A request that reads one (hover, definition, codeAction,
   semanticTokens, completion, signatureHelp, prepareRename) waits for
   a check of its directory that includes every change made before it
   arrived; the loop starts that check ahead of the debounced ones.
2. Each open directory (that of an open document, std copies aside) is
   checked on its own, and its last check (a View: resolution, file
   keys, diagnostics, and the directories of the user files it read) is
   kept. A change in directory d makes stale d and every open directory
   whose last check read d; after the 150 ms debounce the stale ones
   are checked, one at a time. didOpen, didClose, and
   didChangeWatchedFiles start checks at once (a watched file makes
   every directory stale); didSave starts none (the check has the saved
   text already).
3. references and rename read a check over every open directory (key
   ALL), as before, so they find uses in every open directory; so does
   a request in a std copy or outside an open directory.
4. Completion and signatureHelp check a patched text (the placeholder
   after `x.`, a closing `)`); the worker runs that check too, over the
   request's directory only, and the request is answered from it.
5. A file's diagnostics come from its own directory's check when that
   directory is open, else from the first open directory's check that
   reports it.
6. The loop is a task of its own, and main's body waits for it on a
   channel, not `await` (gotchas/root-inline-task-waits-for-helped-work.md):
   a task the root thread runs inline helps with other work while it is
   parked and cannot move, so its messages waited for whole checks.

## Consequences

Same machine, seven tabs, after the change: formatting replies 3 to 13
ms after the request at startup (40 requests sent every 100 ms while
the seven first checks ran, none over 30 ms) and about 8 ms after
each edit; codeAction and semanticTokens wait for their directory's
check (0.3 to 1.0 s, the longer ones while other directories' checks
are queued). With one tab, formatting after an edit replied about 5 ms
after the request (it waited for the whole check before).

In the owner's VS Code (2026-10-20, through a timing proxy between the
editor and the server, seven saves after newline edits, several tabs
open): formatting replied in 0 to 1 ms, codeAction in at most 3 ms,
semanticTokens in at most 11 ms; the owner: "lightning fast".

The same scripted session (scratch func.py: hover, definition,
references across directories, an unsaved rename in compiler/check
reported in compiler/cli/servers and cleared on revert, completion
after `s.`, signatureHelp, a syntax error, didClose clearing
diagnostics, semantic tokens) passes on the new server and on main's;
TestLSPCompletion, TestLSPStalePWD, and TestAgentCLI pass.

Two directories that share imports each check them, so the total work
after a change in a widely imported directory can exceed the old single
check; it runs in the background. The headless session of `xo agent`
(agent.xo) keeps the single synchronous check (recheck).

## Alternatives

- Answer a waiting formatting request before starting a recheck: only
  helps when the request is already queued; VS Code's codeAction and
  semanticTokens arrive first.
- The checker's module cache (modcache.xo, XO_CHK_CACHE) to skip
  unchanged modules in one check: opt in because a lazily checked body
  built wrong (os_tasks, 2026-10-19), and requests need the bodies'
  resolution.
- Keep one check over every open directory, only in a worker: answers
  formatting at once, but every edit still costs the whole check before
  hover or diagnostics.
