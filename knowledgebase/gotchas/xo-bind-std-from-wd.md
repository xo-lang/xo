# `xo bind` looked for xo-gobind next to "std" in the working directory

Status: Observed (2026-10-14), fixed (2026-10-14).

Symptom: cmd/xo TestBind failed in the Linux arm64 milestone run at main
f8ebc9ec (and on the Mac with the main checkout's bin/xo) with `xo bind: xo-gobind not
found: set XO_GOBIND, put xo-gobind on PATH or beside xo, or build it
with go build github.com/xo-lang/xo/cmd/xo-gobind`, although the
checkout has cmd/xo-gobind and Go was installed.

Cause: since decision 0215 nothing builds xo-gobind for the tests, so
`xo bind` falls back to building it from the repository that holds the
std directory (selfhost/cli/bind gobind_helper). `xo bind` ran before
xo resolved its std and passed `$XO_STD`, else the relative "std": in
the test's temporary project that is `<tmp>/std`, whose parent has no
cmd/xo-gobind. `xo check` was not affected (it uses check.std_dir, which
finds std beside the binary), so only the explicit `xo bind` broke.

Fix: selfhost/xo runs `xo bind` through bind.run_std with
check.std_dir (the std beside bin/xo, or a release's lib/xo/std), so the
helper is found beside a release's binary or built from the checkout's
cmd/xo-gobind. remote-full.sh needs no XO_GOBIND.
