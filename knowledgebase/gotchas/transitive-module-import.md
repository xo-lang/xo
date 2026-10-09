# An import of a module that is only required transitively resolved

Status: Observed (2026-10-09)

Symptom: a main module requiring only `github.com/acme/billing` could
`use github.com/acme/money` and check clean, because billing requires
money. `cmd/xo/mod_test.go` expected XO1306 and got exit 0.

Cause: imports resolve against every module of the build list (the
roots the checker registers from `mod.Load`), so any module in the graph
was importable. Its version is then chosen by another module's xo.mod: an
upgrade of billing that drops money would break the main module.

Fix: `checkRequired` (`internal/check/deps.go`) reports XO1306 when a
module imports a package of a required module version its own xo.mod does
not require (workspace modules and the importer's own module are exempt).
`xo get <path>` or `xo mod tidy` adds the requirement.
