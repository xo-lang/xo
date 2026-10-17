# task_for_pid from a non-Apple debugger asks for the password

Status: Observed (2026-10-17, decision 0253 stage 2). The owner set the
rule to allow on the development Mac (below); other machines still ask.

Symptom: a process that calls `task_for_pid` on its own child (spawned
with POSIX_SPAWN_START_SUSPENDED, same user) fails with KERN_FAILURE (5)
when it is ad hoc signed without entitlements, and blocks when it is ad
hoc signed with `com.apple.security.cs.debugger`: macOS shows the
"Developer Tools Access" password dialog on the logged in user's screen,
and the call waits for it. lldb on the same Mac attaches without a
prompt.

Cause: the right `system.privilege.taskport` (`security
authorizationdb read system.privilege.taskport`) is class user, group
_developer, shared, timeout 36000: a member of _developer (with
`DevToolsSecurity -enable`) must authenticate once per 10 hours.
Apple's debugserver is a platform binary and is not asked.

On the development Mac (owner, 2026-10-17): `system.privilege.taskport`
is set to `allow` (`security authorizationdb read
system.privilege.taskport` shows rule allow), so a debugger with the
entitlement gets the task port with no prompt there. This Mac only, a
system security setting the owner chose; to restore the old rule (class
user, group _developer, shared, timeout 36000), write the backup back:
`sudo security authorizationdb write system.privilege.taskport <
~/xo-lead/scratch-2026-10-16/taskport-rule-backup.plist` (a copy is in the
lead's scratchpad too). Agents never change the rule.

Fix elsewhere: none chosen yet (decision 0253 stage 2):
authorize once per 10 hours (the first `xo debug` asks), change the rule
on a machine (`sudo security authorizationdb write
system.privilege.taskport allow`: a system security change), or drive
Apple's debugserver. Do not run a probe that calls task_for_pid with the
entitlement in an agent session: it puts the dialog on the owner's
screen and blocks until it times out or is killed. Linux ptrace has no
such prompt (a container may need `--cap-add SYS_PTRACE`).
