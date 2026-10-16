# Some ad hoc binaries fail AMFI and wait at exec on Gatekeeper

Status: Observed (2026-10-16)

## Symptom

A freshly built program never starts: the process sleeps (ps state
`SN`) with its only thread at `_dyld_start + 0`, `sample` says "Binary
images description not available", and `xo test` reports every test as
"did not finish" (dev goldens: "run: timed out"). `codesign -v` says the
binary is valid. It depends on the binary's layout: bench/perf's
test_fakes golden with a 7,300 word constant ran and with a 7,600 word
one did not (`__TEXT` 0x230000 against 0x234000; xo lang 4 saw the same
for http_tls_keys at 0x384000 against 0x388000). It was first seen with
selfhost/macho's output, but a binary Apple's ld linked through cc
(internal/llvm's http_tls_keys golden, with LC_FUNCTION_STARTS) waited
the same way, so it is not the linker's.

## Cause

macOS, not the program. `log stream` on the exec shows the kernel
rejecting the ad hoc signature:

```
AMFI: '<path>' has no CMS blob?
AMFI: '<path>': Unrecoverable CT signature issue, bailing out.
AMFI: code signature validation failed.
```

syspolicyd then assesses the binary with `allowUI is YES`: an online
notarization lookup ("ticket not available"), then
`GatekeeperPolicyScanError -67018 "Code did not match any currently
allowed policy"`, and the exec waits on the outcome: a dialog for the
Mac's user. Later assessments log "waiting on another evaluation" and
queue behind any dialog still pending, so binaries that would otherwise
run wait too. Re-signing with `codesign -f -s -` or raising the minimum
OS with vtool does not change AMFI's verdict.

## Fix

On the Mac, a person: dismiss the pending Gatekeeper dialogs, and enable
the terminal app that runs the builds under System Settings > Privacy &
Security > Developer Tools, which exempts the programs it starts from
these assessments. Mitigation in the toolchain: the darwin/arm64 default
link is cc whenever the host has one (selfhost/xo main.xo default_ld;
fewer of its binaries were rejected than macho's in the repros), and
`--ld=macho` still selects macho. To see whether a binary that sleeps at
`_dyld_start` is waiting on this, run `log stream --predicate 'process
== "syspolicyd" OR senderImagePath CONTAINS[c] "AppleMobileFileIntegrity"'`
while it starts. Which layout property AMFI rejects is not known; bisect
macho's output (LC_FUNCTION_STARTS, LC_DATA_IN_CODE, LC_SOURCE_VERSION,
LC_BUILD_VERSION's tools, __cstring dedup, section order) only once the
Gatekeeper state is clean.
