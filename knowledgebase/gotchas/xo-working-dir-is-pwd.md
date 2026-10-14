# xo takes its working directory from $PWD

Status: Observed (2026-10-13)

Symptom: in VS Code, Go to Definition on a std name (`http.Request` in
examples/notes-service/main_test.xo) said "No definition found", while
definitions in user code worked. The server logged nothing: definition
answered `null`.

Cause: xo has no getcwd; the CLI and servers read the working directory
from `$PWD` (`os.env.get("PWD")` in selfhost/cli/servers/lsp.xo,
core/cli.xo, servers.xo, serve.xo, and others). A process started without
a shell inherits whatever PWD its parent had. VS Code launched from the
Dock passes `PWD=/` while starting the server in the workspace folder, so
`span_location` resolved std's relative file paths against `/` and found
no file. Reproduced with `PWD=/ bin/xo lsp`: the same request returns
null; with PWD equal to the real directory it returns
`<user cache>/xo/lsp-std/std/http.xo`.

Fix: editors/vscode/extension.js starts `xo lsp` with `cwd` and `PWD` set
to the same directory (the first workspace folder, else the home
directory). Any other program that spawns xo without a shell must do the
same. The root fix is a working directory call in std (the OS getcwd) for
the toolchain to use instead of `$PWD`.
