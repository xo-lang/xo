# Infrastructure

- [toolchain.md](toolchain.md): host toolchain, Go module, build and test commands, `xo parse`/`check`/`build`/`run`/`test`, package layout, std declaration files, code generation conventions, runtime layout, record and replay and crash bundles (`xo run --record`, `xo replay`), the LLVM backend prototype (`--backend=llvm`), the self hosted lexer (`selfhost/`, `xo lexdump`), std/time zone data (`xo_tzdata.go`, `XO_ZONEINFO`, `--tzdata=system`), modules and workspaces (module cache location, `XO_MODCACHE`, `XO_OFFLINE`, `XO_WORK`, `XO_GIT_BASE`, `xo get`, `xo mod tidy`, `xo vendor`, effect ceilings, `xo publish`, `xo init`).
- [namespaces.md](namespaces.md): status of the Xo name across GitHub, Homebrew, npm, crates, file extensions.
