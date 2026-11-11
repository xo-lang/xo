# An environment variable the linker reads must be in the binary cache's key

Status: Observed and fixed (2026-11-11)

## Symptom

TestDebugInfoSameCode failed in tiers 908 to 912 for about ten goldens
(value_semantics, maps, closures, http_server, ...). Every program
function "differed": in the build with debug information a `bl` went to
an address otool could not name, while the build without it called the
`_xo_` symbol. The subtest passed when run alone, until it began failing
alone on the Mac too. It looked like nondeterministic code generation.

## Cause

The unnamed targets were branch island veneers (`adrp x16; add; br x16`).
The build with debug information was TestGoldenRunIslands' link, made
with XO_BRANCH_REACH=262144. The linker reads XO_BRANCH_REACH (and
XO_DATA_SHIFT and XO_STD_HALF) from the environment. These change the
linked bytes but no object, and macho_bin_key, native_bin_key and std's
half keys held only the flags and inputs. So the islanded link was
stored in the test bin cache (~/Library/Caches/xo/testbin, a top-level
entry, not run/) under the default link's key, and served to the next
default link of the same objects. The run cache (run/) was safe: its key
holds every XO_ variable.

How it was found: XO_TEST_KEEP_DIR kept both binaries, otool showed the
veneers, and the kept binary's bytes matched one entry in the testbin
cache (shasum by size).

## Fix

compiler/xo cache.xo link_env_key: the binary keys and std's half keys
hold XO_BRANCH_REACH, XO_DATA_SHIFT and XO_STD_HALF. Two guards:

- internal/xotool TestLinkEnvKeyed: every XO_ variable the link code
  (compiler/xo ld.xo, compiler/macho) reads is in link_env_key or in
  linkEnvNoOutput with its reason.
- internal/arm64 TestBinCacheLinkEnv: on a fresh bin cache, a default
  build after an islanded build is not the islanded binary.

A cache that already holds a poisoned entry keeps serving it to an xo
without the fix: delete that entry by its path (found by the binary's
bytes), or run with XO_BIN_CACHE=off.

## Rule

A new linker knob read from the environment goes into link_env_key
unless it cannot change the linked bytes. A test whose builds set such a
knob shares the bin cache with every other test.
