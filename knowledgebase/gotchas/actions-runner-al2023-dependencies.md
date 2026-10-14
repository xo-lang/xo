# The Actions runner's dependency installer does not know Amazon Linux 2023

Status: Observed (2026-10-14).

Symptom: the first boot of runner xo-ci-arm64 logged `Can't detect
current OS type based on /etc/os-release` from
`bin/installdependencies.sh`, then `config.sh` failed with
`Libicu's dependencies is missing for Dotnet Core 6.0` (plus `ldd:
./bin/libcoreclr.so: No such file or directory`, because config.sh was
run from another directory).

Cause: installdependencies.sh (runner 2.338.0) matches only some
distributions by ID and does not treat AL2023 (`ID_LIKE="fedora"`) as
one; .NET needs libicu, which AL2023 does not install by default.
config.sh checks its libraries with relative paths, so it must run from
the runner directory.

Fix: infra/ci-runners/boot/linux.sh installs `libicu` with dnf instead
of calling installdependencies.sh, and runs `(cd $R && ./config.sh ...)`.
