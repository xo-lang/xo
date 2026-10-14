# Proc.run on Windows read a NULL command line

Status: Observed (2026-10-14, netfix agent, windows/amd64 on EC2). Fixed
in 1a53b12c and verified on Windows on 2026-10-14 (queue job
winfix-netonly on the warm instance: net_only passes, golden set 154 of
154).

Symptom: the net_only golden crashed on windows/amd64 with an access
violation (exit 3221225477, or 139 under Git Bash) after its redirect
lines. It was the only golden of 154 that failed. The crash happened with
the default path and with XO_LLRT_C=sched, and at main 4f792c55. The
missing sql, env, and proc lines were still buffered: stderr markers
(eprintln flushes stdout) showed it got past the sql step. Every run
crashed, also with XO_THREADS=1.

Cause: on Windows, pr_w_spawn builds the child's command line from
`c->cmdline`, which std/native/llrt/procspec.xo builds for Proc.cmd and
pipelines. Windows' xo_proc_run (Proc.run) fills its own pr_cmd with the
name and argv only, so `cmdline` was NULL and `nbuf_str` called strlen
on NULL. A debug build (`-O0 -g`, a PDB, and a vectored handler that
printed the unwound frames, symbolized by llvm-symbolizer) showed it:
strlen <- nbuf_str (xo_std.c:63) <- pr_w_spawn (xo_proc.c:876) <-
pr_start <- xo_proc_run, reading address 0. Any Proc.run of a command
that exists crashed on Windows. net_only was the only golden that
reaches one (`sh -c "echo allowed"`); proc_cmd uses Proc.cmd.

Fix: xo_proc_run on Windows sets `pc.cmdline` from its argv with
pr_w_cmdline, which quotes each argument as procspec.xo's quote does
(Go's syscall.EscapeArg, and also for line ends and other spaces).

Avoid: a C path that builds a pr_cmd by hand must fill every field that
pr_start reads on each platform. Check the Windows-only fields
(cmdline) as well as the POSIX ones.
