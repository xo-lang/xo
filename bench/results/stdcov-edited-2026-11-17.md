# TestStdAPICovered on the edited modules only (2026-11-17)

Task #259, owner decision #67. `XO_STDCOV_JOBS=8 XO_CLI=... go run
./scripts/stdcoverage -since=PUSH^` in a worktree at each push, on the Mac
(darwin/arm64), xo built from main; wall seconds of the whole check
(listing the inputs of all 977 programs, sweeping the ones that reach
the edited modules, resolving).

| push | modules checked | programs swept | seconds | open |
|---|---|---|---|---|
| 6cee17f78 toml strict decode | toml | 219 | 67.6 | toml.StrictErr.display (fixed by golden toml_strict_errs, 9707e7f96) |
| 0076bbe94 amqp null key | amqp | 222 | 60.6 | amqp.decode, amqp.encode (standing on main) |
| 998e3992d grpchealth service | grpchealth | 216 | 82.2 | none |

The full sweep (gate step stdcov) took 29 minutes a run (54 at most);
a run on host 3 on 2026-11-17 was stopped at its 30 minute lease before
it reported. The edited-module check takes one to one and a half
minutes. Listing the inputs of every program is most of it (0.34 s a
program, 8 at once); a cache of each program's inputs keyed on its files
and std's would bring it to the sweep alone. An edit to
std/native/prelude.xo reaches every program, so it still sweeps all of
them.

The toml push left StrictErr.display without a resolved call outside
std's own tests, which no gate step ran for: the edited-module check
found it in a minute.
