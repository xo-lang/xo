# errors.Is(ENOTEMPTY, fs.ErrExist) is true in Go

Status: Observed (2026-10-07, std/rt/caps.go)

Symptom: removing a non empty directory reported `FsErr.Exists` instead of
`FsErr.NotEmpty`.

Cause: Go's `syscall.Errno.Is` maps ENOTEMPTY (and EEXIST) to
`fs.ErrExist`.

Fix: test the specific errnos (ENOTEMPTY, ENOTDIR, EISDIR) before the
`fs.Err*` sentinels when mapping OS errors (`mapFsErr`). Golden test:
`fs_rooting`.
