# a handle released before the runtime call that uses its address (Observed 2026-10-29)

Symptom: Linux TSan (--race, arm64) `heap-use-after-free` in chan.recv_w (pool.lock_k, chan.xo:694),
TestRaceSyncTable chan. Darwin silent (race heap is not libc malloc/free, memory stays mapped).
Cause: Chan[Int] ops pass `rt.obj_addr(h)` (conc.xo chan_word) to llrt_chan_*_w; rc.xo saw h's last
use at obj_addr, so the release came BEFORE the call; the sender's task end then freed the channel.
Fix: per site: conc.xo chan_hold calls the prelude's llrt_chan_hold[T] after each word channel op, using the handle. The general rule (rc.xo: an rt.obj_addr result borrows its handle) failed the inliner's verify, because std code moves a handle after taking its address on purpose. Select's send arm (select_send_w) has no hold yet.
Check: test/codegen/chanrc; Docker: scripts/docker-linux.sh arm64 (race_sync, XO_THREADS=1, XO_RACE_CASE=chan).
Lesson: darwin hides use-after-free of Xo heap blocks; a Linux TSan run is the detector.
