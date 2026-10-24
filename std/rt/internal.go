package rt

import (
	"os"
	"strings"
)

// ---------------------------------------------------------------------------
// std/internal (decision 0214): primitives for std and the toolchain's own
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

// Internal_dir_names is `internal.dir_names(f, dir)`: the names in dir
// without a stat of any entry (os.ReadDir's names; Fs.read_dir stats every
// one). Only the root Fs takes it, as on the native backends.
func Internal_dir_names(tk *Task, f Fs, dir string) Result[List[string], FsErr] {
	rf, ok := f.(*realFs)
	if !ok || rf.sub || rf.readOnly || rf.writeOnly {
		return Err[List[string]](fsErr(6, dir))
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return Err[List[string]](mapFsErr(err, dir))
	}
	names := make([]string, 0, len(ents))
	for _, de := range ents {
		names = append(names, de.Name())
	}
	return Ok[List[string], FsErr](ListOf(names...))
}

// Internal_rt_probe is `internal.rt_probe`: a probe of the native runtime
// (decision 0241), which the Go backend does not have.
func Internal_rt_probe(_ *Task, op string, _, _, _, _ int64) int64 {
	panic("internal.rt_probe(" + op + "): the native backends only")
}

// Internal_obj_word is `internal.obj_word`: the native runtime's object
// address, which the Go backend does not have.
func Internal_obj_word[T any](_ *Task, _ T) int64 {
	panic("internal.obj_word: the native backends only")
}

// Internal_rt_queued and Internal_rt_allocs are `internal.rt_queued` and
// `internal.rt_allocs`: the native runtime's real-time section counters,
// which the Go backend does not have.
func Internal_rt_queued(_ *Task) int64 { panic("internal.rt_queued: the native backends only") }
func Internal_rt_allocs(_ *Task) int64 { panic("internal.rt_allocs: the native backends only") }
