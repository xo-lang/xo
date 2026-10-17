# A self hosted binary outside a release layout finds no std or xo-gobind

Status: Observed (2026-10-12, decision 0178 switch point B gate runs 1
and 2; bench/results/retire-go-2026-10-12.md "Switch point B gate").

## Symptom

With no `$XO_STD`, cmd/xo's suite against xo1 (`XO_CLI`) failed TestBind
("xo-gobind not found", exit 2), TestAudit and TestVouches (XO1101
"cannot bind Go package `os`: xo-gobind not found"), and
TestModulesXoSame's audit step, where the stage 4 binary xomod faulted
with `fault: prelude type missing: Option at check/types.xo:126:3` and
printed nothing. Earlier gates set `XO_STD=<repo>/std` and passed.

## Cause

The Xo commands find their files from the running binary (Env.program,
compiler/check locate.xo and compiler/cli/bind gobind_helper): std as
`<bin>/../lib/xo/std`, then `<bin>/std`, `<bin>/../std`, else `std`
relative to the working directory; xo-gobind as `$XO_GOBIND`, beside the
binary, in `<prefix>/bin` of a release layout, beside std's parent, on
`$PATH`, else built from the Go repository holding std. A binary in a
temporary directory (xomod) has no std near it, and a release layout
whose bin/ lacks xo-gobind, with std linked into lib/xo (no go.mod
above it), has no way to get the binder. The Go `xo` embeds std and
binds in process, so it never needed either.

## Fix

Lay a self hosted binary out as a release before testing it: bin/xo,
bin/xo-gobind (`go build ./cmd/xo-gobind`), lib/xo/std and
lib/xo/third_party. The gate script does this, and TestModulesXoSame now
builds xomod into such a layout. Switch point B's release archives must
ship xo-gobind in bin/ (scripts/release.sh does not build it yet; the
flip proposal in decision 0178 lists it).
