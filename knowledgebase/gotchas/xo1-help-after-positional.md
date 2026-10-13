# Help after a positional argument, and xo1's own usage line

Status: Observed (2026-10-13, the shipped xo, xo1 at 96225edd)

Symptom: `xo` alone and `xo build -h` printed xo1's internal usage line
(`usage: xo1 build|run|test|asm|keys ... --go-tree --phase-times`);
`xo help` listed only the commands first ported ("self hosted: check,
parse, ..."); `xo edit rename --help` printed `flag provided but not
defined: --help`. Each command parsed its flags with its own copy of
Go's flag rules, so help and usage errors differed per command, and
Go's rule that flags stop at the first positional argument (see
go-flag-stops-at-first-argument.md) meant `-h` after a subcommand or
operation was an unknown flag.

Cause: the commands were ported one package at a time (decision 0178),
each with the Go side's `Usage of NAME:` text, and xo1's own usage line
was never meant for users.

Fix: selfhost/cli/cmdhelp holds every user command's usage; xo1's
front end (selfhost/xo opts.xo `front`) answers `-h`, `--help`, and
`xo help [command]` anywhere before `--` (run: before its directory,
after which arguments are the program's) on stdout with exit 0, and
every flag parser prints the cmdhelp usage after its error (exit 2).
cmd/xo TestCLIHelp pins it against the shipped xo. The differential
tests that compare a usage error with the Go `cmd/xo` drop the usage
first (`dropUsage`): the help is xo1's, the Go side is frozen.

Also: a test that calls `t.Chdir` must resolve its testdata paths
before it (TestCLIHelp's `-update-help` first wrote its goldens into
the temporary directory); `clihelpDir` is absolute for that reason.
