package rt

import "path"

// std/path (std.md 9): `/` separated paths, independent of the host OS.

// Path_join is `path.join(a, b)`: joins and cleans.
func Path_join(_ *Task, a, b string) string { return path.Join(a, b) }

// Path_dir is `path.dir(p)`: everything but the last element ("." if none).
func Path_dir(_ *Task, p string) string { return path.Dir(p) }

// Path_base is `path.base(p)`: the last element.
func Path_base(_ *Task, p string) string { return path.Base(p) }

// Path_ext is `path.ext(p)`: the extension including the dot, or "".
func Path_ext(_ *Task, p string) string { return path.Ext(p) }

// Path_clean is `path.clean(p)`: removes `.`, `..`, and repeated `/`.
func Path_clean(_ *Task, p string) string { return path.Clean(p) }
