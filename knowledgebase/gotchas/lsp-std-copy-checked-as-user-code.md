# LSP: the std copy a definition opens was checked as user code

Status: Observed (2026-10-10, VS Code, go to definition into std/json.xo)

Symptom: after go to definition into std, the opened
`<user cache dir>/xo/lsp-std/std/json.xo` shows XO0172 "expected `{` to
start the body of `encode`" on every function.

Cause: `xo lsp` serves std definitions from copies of the embedded std
sources. Opening the copy made it an open document, so the server added
its directory to the checked modules. std files are bodiless signature
listings, which only parse in signature mode.

Fix: documents under `Server.StdDir` are left out of the check and its
overlay; requests on them are answered from the std module's file
(`stdKey` maps the copy to `std/<name>.xo`), and formatting skips them.
The copies are written read only. Regression test: `TestStdCopy`.
