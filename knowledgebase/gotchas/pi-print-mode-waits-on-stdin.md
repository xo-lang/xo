# Pi's print mode waits on an open stdin

Status: Observed (2026-10-23).

Symptom: `pi -p "PROMPT"` run from a Claude Code Bash call (or any
script) printed nothing and sat at 0% CPU until the 180 s timeout; no
model call was made and the guard log stayed empty.

Cause (inferred from the idle process and the fix, not from Pi's
source): in print mode Pi also reads stdin when it is not a terminal. A non-interactive shell's stdin is an open
pipe that never reaches end of file, so Pi waits for it forever.

Fix: close stdin: `pi -p "PROMPT" </dev/null`. The xo-guard end-to-end
check in knowledgebase/infrastructure/pi-deepseek-experiment.md does.
