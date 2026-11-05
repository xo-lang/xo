package rt

// `internal.dbg_*`: `xo debug`'s process control (decision 0253 stage 2)
// is in the native runtime (std/native/llrt/dbgmac.xo); a Go backend
// build of the toolchain makes no debuggee, so the others never see a
// handle.

func Internal_dbg_launch(_ *Task, _ string, _ List[string], _ string) Result[int64, string] {
	return Err[int64]("xo debug's process control needs a native build of xo (decision 0253)")
}
func Internal_dbg_attach(_ *Task, _ int64) Result[int64, string] {
	return Err[int64]("xo debug's process control needs a native build of xo (decision 0253)")
}
func Internal_dbg_threads(_ *Task, _ int64) List[int64]      { return ListOf[int64]() }
func Internal_dbg_pid(_ *Task, _ int64) int64                { return 0 }
func Internal_dbg_read(_ *Task, _, _, _ int64) Option[Bytes] { return None[Bytes]() }
func Internal_dbg_write(_ *Task, _, _ int64, _ Bytes) bool   { return false }
func Internal_dbg_regs(_ *Task, _, _ int64) List[int64]      { return ListOf[int64]() }
func Internal_dbg_set_pc(_ *Task, _, _, _ int64) bool        { return false }
func Internal_dbg_set_step(_ *Task, _, _ int64, _ bool) bool { return false }
func Internal_dbg_resume(_ *Task, _ int64, _ bool) bool      { return false }
func Internal_dbg_wait(_ *Task, _, _ int64) List[int64]      { return ListOf[int64](0) }
func Internal_dbg_kill(_ *Task, _ int64)                     {}
func Internal_dbg_close(_ *Task, _ int64)                    {}
