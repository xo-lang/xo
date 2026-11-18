# A directory's mtime is not a sign of use

Status: Observed (2026-11-17, task #303).

## Symptom

The first dry run of scripts/lead/diskcap.sh's scratch step offered 21 GB
of the 147 GB under /private/tmp/claude-501. A single session's
scratchpad held 107 GB and was kept as "changed in the last 24 hours",
although that session's transcript was last written on 2026-11-15 and xw
sessions did not list it.

## Cause

No file in it was new. Only its directories were: a cleanup that deletes
entries (diskcheck.sh's temporary build trees, `rm` of a finished run's
output) updates the parent directory's mtime. The idle check was
`find PATH -mmin -1440`, which counts directories too, so deleting files
looked like using the tree.

## Fix

Idle checks for cleanup count files only: `find PATH -type f -mmin -N`
(diskcap.sh's `changed_within`). diskcap_test.sh covers it: a session
directory whose only recent change is a removed entry is deleted.
