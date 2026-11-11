# heavy.sh runs scripts/gate.sh relative to the caller's directory

Observed 2026-11-11 (Xo Azure 1): two xo gates, link-key-env and FakeNet,
were started as `heavy.sh NAME scripts/gate.sh ~/xo-wt/WT arm64 llvm cmd
corpora-all` from an agent whose shell sat in an SDK repository (aws). The
relative `scripts/gate.sh` was that repository's /bin/sh gate, which took
the xo arguments as package directories and sat idle with no children for
two hours (an earlier one for 1 h 46 min). The NAME-cmd.log showed only the
start line, which reads like a slow gate, not a wrong one.

Fixes:

- `heavy.sh` (scripts/lead/heavyd/heavy.sh) prints and logs the job's
  directory and git toplevel ("=== ... cwd DIR (git toplevel TOP)") before
  it queues, and refuses a relative `scripts/gate.sh` whose toplevel has no
  compiler/xo, exit 64.
- xo's scripts/gate.sh refuses a WORKTREE without compiler/xo, exit 2.

Check: an xo gate whose log shows nothing past its start line after a few
minutes: `lsof -a -p PID -d cwd` on its gate.sh process, and look at the
log's cwd line.
