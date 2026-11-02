#!/bin/bash
# thp-always.sh DIR: on the WSL host, set THP enabled=always (owner, 2026-11-02),
# run job2.sh DIR, and set it back to madvise on any exit; confirm it reads madvise.
set -u
S=${1:?DIR}
WSL=$(command -v wsl.exe || echo /mnt/c/Windows/System32/wsl.exe)
F=/sys/kernel/mm/transparent_hugepage/enabled
restore() {
  "$WSL" -u root -e sh -c "echo madvise > $F" < /dev/null
  echo "restored: $(cat $F)"
  grep -q '\[madvise\]' $F && echo THP-RESTORED-OK || echo THP-RESTORE-FAILED
}
trap restore EXIT
echo "before: $(cat $F)"
"$WSL" -u root -e sh -c "echo always > $F" < /dev/null
echo "set: $(cat $F)"
grep -q '\[always\]' $F || { echo "could not set always"; exit 3; }
bash "$S/job2.sh" "$S"
