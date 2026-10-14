# rt.bytes_buf is not zeroed: a structure the call reads must be cleared

Status: Observed (2026-10-14, B16 part 2 on windows/amd64)

Symptom: every Fs.lock and try_lock on Windows through windows.xo
failed with `Io(... "input/output error")` (fs_files, fs_ops_edges, the
sysio lock workload), while the same code's other calls worked.

Cause: windows.flock passed LockFileEx and UnlockFileEx an OVERLAPPED
from `rt.bytes_buf(32)`, which is not zeroed (rt.xo says so): a stale
hEvent or offset made the call fail, and xo_w_errno's map turns the
unexpected Win32 error into EIO. Buffers a call only writes (the
results of ReadFile, FindFirstFileExW, DeviceIoControl) are safe.

Fix: store zeros into every word of an input structure built in a
bytes_buf (windows.flock clears the four words of its OVERLAPPED).
