# A golden that writes files in its own directory got them committed

Status: Observed (2026-10-29, Xo Lang 2; the lead's ruling)

## Symptom

`git status` in a worktree showed `internal/llvm/testdata/golden/handle_drop_leak/h.txt`
and `w.txt` deleted after a golden run: they had been committed (dc0e7f5f6, a wasm change
whose `git add -A` took them) and the program removes them at its end.

## Cause

The golden ran with its own directory as the working directory and wrote its scratch files
there. A backend that fails midway (wasm under WASI's limits) never reaches the cleanup, so
the files stay in testdata, and the next `git add -A` commits them; every later passing run
then deletes the committed copies, so an agent's `git add -A` would commit the deletion.

## Fix

A golden's scratch files live in a fresh directory under `$TMPDIR` (as fs_handles and
fs_files do: `os.fs.sub(os.env.get("TMPDIR") ?? "/tmp")`, a random name, `remove_all` at the
end), never beside main.xo. handle_drop_leak does so since 2026-10-29. Before `git add -A`,
check `git status` for files under testdata/golden that the change did not mean to touch.
