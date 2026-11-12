# An untarred xo on the sumdb host fails 203/EXEC

Status: Observed 2026-11-12 (lead, updating sum.xolang.org to xo 0.49.1)

Symptom: after swapping in a new xo build under /srv/xomod/xo,
`systemctl restart xo-sumdb` failed with status 203/EXEC, "Permission
denied", though the file was mode 755 and ran from a shell. The service
was down about 20 s.

Cause: the OCI host runs SELinux Enforcing. Files keep the label of the
place they were created, and the tarball was unpacked in a mktemp
directory, so the new tree was labeled `var_lib_t`. systemd may only
exec a service binary labeled for it (`bin_t`, as the old build was).
`mv` keeps labels, so moving the tree into place did not change them.
A plain shell user can still run the file, which hides the cause.

Fix: before the restart, give the new tree the old build's labels and
check them:

    sudo chcon -R --reference=/srv/xomod/xo-old /srv/xomod/xo
    ls -Z /srv/xomod/xo/bin/xo        # bin_t, as xo-old's

`restorecon -R /srv/xomod/xo` does the same only if the policy has a
file-context rule for the path (`semanage fcontext -l | grep xomod`);
without one it would set the default label, not `bin_t`. A 203/EXEC on
an SELinux host: look at `ls -Z` and `ausearch -m avc -ts recent` first.
The update steps (lead notes, sumdb-copystall-update.md) now hold the
chcon.
