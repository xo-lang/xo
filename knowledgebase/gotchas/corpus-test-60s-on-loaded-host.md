# A corpus test near xo test's 60 s limit fails only on a loaded host

Status: Observed (2026-11-11, Xo Lang 3; diagnosed by Xo Lang 2)

- Symptom: `TestStdCorpora/go/regexp` failed on host 3 (the Docker container on the Mac) at about 81 s, twice, and
  passed on the Mac in 21 s. offload-gate kept only the last 60 lines, so the cause was not in the log.
- Cause: `xo test` stops a test after 60 s of real time (its `--timeout` default, compiler/xo/test.xo). The whole regexp
  corpus was one test. Under host 3's load it ran past the limit: "the test did not finish within 1m0s of real time".
- Fix: the generator splits the corpus into parts (scripts/stdcorpus/regexp.go `PARTS`), one test each. A corpus test
  that takes more than about 15 s on the Mac gets the same split. Do not raise `--timeout` for all corpora: that hides
  hangs. A failed step's whole log is now kept as ~/xo-lead/agents/offgate-NAME-STEP.log.gz (bec4d3b06).
