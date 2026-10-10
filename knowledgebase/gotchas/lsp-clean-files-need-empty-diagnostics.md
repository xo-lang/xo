# LSP: a clean file must still get an empty publishDiagnostics

Status: Observed (2026-10-10, internal/lsp TestDiagnosticsAndQuickFix)

Symptom: the editor keeps showing old errors after they are fixed, and a
client that waits for the first diagnostics of a clean file never gets
any.

Cause: the first `xo lsp` published only files that had diagnostics,
grouped from `check.Result.Diagnostics`. A file with none produced no
notification. LSP has no "everything is fine" message besides an empty
list.

Fix: `Server.publish` sends a notification for every open document and
every file with diagnostics, empty when clean, and an empty one for each
file published last time that has none now. TestDiagnosticsAndQuickFix
covers both the clean open and the clearing after an incremental edit.
