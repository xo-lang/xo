package rt

import "strings"

// ---------------------------------------------------------------------------
// std/internal (decision 0213): primitives for std and the toolchain's own
// modules, not std's public surface.

// Internal_SharedLock is `internal.SharedLock`: a held shared advisory
// lock (a handle).
type Internal_SharedLock struct{ l FileLock }

func (Internal_SharedLock) XoDebug(b *strings.Builder) { b.WriteString("<shared lock>") }
func (l Internal_SharedLock) M_debug(*Task) string     { return Debug(l) }

// M_unlock releases the lock; a second unlock does nothing.
func (l Internal_SharedLock) M_unlock(tk *Task) { l.l.M_unlock(tk) }

// Internal_lock_shared is `internal.lock_shared(f, path)`: a shared
// flock(2) (Unix) or LockFileEx without LOCKFILE_EXCLUSIVE_LOCK (Windows)
// lock of path, waiting while it is held exclusively. Only the root Fs
// takes it (as on the native backends); any other Fs is Denied.
func Internal_lock_shared(tk *Task, f Fs, p string) Result[Internal_SharedLock, FsErr] {
	rf, ok := f.(*realFs)
	if !ok || rf.sub || rf.readOnly || rf.writeOnly {
		return Err[Internal_SharedLock](fsErr(6, p))
	}
	r := rf.lockFileMode(tk, p, true, true)
	if !r.Ok {
		return Err[Internal_SharedLock](r.E)
	}
	if !r.V.Some {
		return Err[Internal_SharedLock](FsCanceled)
	}
	return Ok[Internal_SharedLock, FsErr](Internal_SharedLock{r.V.V})
}
