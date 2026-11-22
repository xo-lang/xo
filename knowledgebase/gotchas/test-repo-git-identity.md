# A test's scratch repo commit fails on a host with no git identity

Status: Observed 2026-11-22 (task #711, scripts/lead/tier-move_test.sh).

**Symptom:** TestTierMove passed on the Mac and failed 10 times on host 3:
`unable to auto-detect email address (got xo@xo-host3.(none))`, then
`ambiguous argument 'HEAD': unknown revision`, and every later check
failed.

**Cause:** the test made commits in a temporary repository with the
host's identity. The Mac has a ~/.gitconfig (and git guesses one from the
host name), host 3's container has neither.

**Fix:** a test that commits sets its own identity
(`export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t
GIT_COMMITTER_EMAIL=t@t`, or `git config user.*` in the repo). To prove
it on the Mac, run the test with an empty HOME, `GIT_CONFIG_NOSYSTEM=1`
and `user.useConfigOnly=true` (GIT_CONFIG_COUNT), as TestTierMove now
does (noGitIdentity, internal/repotest/tiercheck_test.go).
