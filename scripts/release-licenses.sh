#!/bin/bash
# release-licenses.sh STAGE: checks a staged release tree (decision 0302,
# point 4). Each THIRD_PARTY_NOTICES item marked [binary] (shipped by
# scripts/release.sh, not in the repository) names the license file it
# ships beside the binary (a lib/xo/... path containing LICENSE); when
# that file's directory is in STAGE (the binary was staged for this
# target), the file must be there too. An item that names no such path
# fails. Exit 0 when complete, 1 with one line per gap.
set -u
stage=${1:?usage: release-licenses.sh STAGE}
notices="$stage/THIRD_PARTY_NOTICES"
[ -f "$notices" ] || { echo "release-licenses: $notices is missing" >&2; exit 1; }
rc=0
# the items: a heading line `N. title [keys]`, then its body to the next
awk '
	/^[0-9]+\. / { item = $0; binary = ($0 ~ /\[binary\]/); next }
	binary {
		line = $0
		while (match(line, /lib\/xo\/[^ ,;)]*LICENSE[^ ,;)]*/)) {
			print item "\t" substr(line, RSTART, RLENGTH)
			line = substr(line, RSTART + RLENGTH)
		}
	}
' "$notices" > "$stage/.licenses.$$"
# every [binary] item names at least one license path
while IFS= read -r item; do
	if ! grep -qF "$item	" "$stage/.licenses.$$"; then
		echo "release-licenses: THIRD_PARTY_NOTICES \"$item\" is [binary] but names no lib/xo/.../LICENSE file it ships" >&2
		rc=1
	fi
done < <(grep -E '^[0-9]+\. .*\[binary\]' "$notices")
while IFS=$'\t' read -r item path; do
	path=${path%.}
	dir=$(dirname "$path")
	if [ -d "$stage/$dir" ] && [ ! -f "$stage/$path" ]; then
		echo "release-licenses: $stage/$dir is staged without $path (THIRD_PARTY_NOTICES \"$item\")" >&2
		rc=1
	fi
done < "$stage/.licenses.$$"
rm -f "$stage/.licenses.$$"
exit $rc
