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
Mac's user. The dialog the owner saw was XProtect's "Malware Blocked:
'http_stream' was not opened because it contains malware", a false
positive on macho's layout (gotchas/xprotect-false-positive-empty-list.md,
fixed); with the terminal under Developer Tools (done 2026-10-16) the
stalls stopped. Later assessments log "waiting on another evaluation" and
queue behind any dialog still pending, so binaries that would otherwise
run wait too. Re-signing with `codesign -f -s -` or raising the minimum
OS with vtool does not change AMFI's verdict.

A stall can also stick to one path (observed 2026-10-16, after the
XProtect fix and with the terminal under Developer Tools): the dev
goldens' cc linked http_tls_keys at
`<user cache>/xo/dev/goldens/bin-arm64/internal_llvm_testdata_golden_http_tls_keys`
waited at `_dyld_start` on every exec (seven in a row, six of them at
once), while a copy at another path, a copy with the same name in
another directory, and a hard link to the same inode at another path
all started in under 0.4 s; some minutes later the path started again.
Every `xo dev goldens` run removed and linked each binary again at that
fixed path, in a directory that sessions running at once share, so one
session could replace a binary another was starting.

## Fix

On the Mac, a person: dismiss the pending Gatekeeper dialogs, and enable
the terminal app that runs the builds under System Settings > Privacy &
Security > Developer Tools, which exempts the programs it starts from
these assessments. `xo dev goldens` puts each run's binaries in a fresh
`run-<random>` directory under its output directory and removes it at
the end (selfhost/cli/dev goldens.xo), so no path is linked again while
another run starts it. The toolchain's mitigation, cc as the
darwin/arm64 default link (6faa44cb), was undone the same day once
both causes were fixed (decision 0182 addendum): macho is the default
again, `--ld=cc` selects cc. To see whether a binary that sleeps at
`_dyld_start` is waiting on this, run `log stream --predicate 'process
== "syspolicyd" OR senderImagePath CONTAINS[c] "AppleMobileFileIntegrity"'`
while it starts. Which layout property AMFI rejects is not known; bisect
macho's output (LC_FUNCTION_STARTS, LC_DATA_IN_CODE, LC_SOURCE_VERSION,
LC_BUILD_VERSION's tools, __cstring dedup, section order) only once the
Gatekeeper state is clean.
