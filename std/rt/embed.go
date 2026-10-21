package rt

import (
	"strings"
	"unicode/utf8"
)

// std/embed (std.md 31, decision 0304): a Dir is the files a constant
// embeds at build time, a Map of slash paths to bytes; its methods are
// lookups, as std/native/embed.xo's.

func (d *Embed_Dir) M_read(tk *Task, path string) Option[Bytes] {
	return Map_get(tk, d.F_files, path)
}

func (d *Embed_Dir) M_read_text(tk *Task, path string) Option[string] {
	b := Map_get(tk, d.F_files, path)
	if !b.Some || !utf8.ValidString(string(b.V)) {
		return None[string]()
	}
	return Some(string(b.V))
}

func (d *Embed_Dir) M_contains(tk *Task, path string) bool {
	return Map_contains(tk, d.F_files, path)
}

func (d *Embed_Dir) M_paths(tk *Task) List[string] {
	return List_sort(tk, Map_keys(tk, d.F_files))
}

func (d *Embed_Dir) M_sub(tk *Task, dir string) Embed_Dir {
	prefix := strings.TrimSuffix(dir, "/") + "/"
	var files Map[string, Bytes]
	for p, b := range Map_All(d.F_files) {
		if strings.HasPrefix(p, prefix) {
			Map_put(tk, &files, strings.TrimPrefix(p, prefix), b)
		}
	}
	return Embed_Dir{F_files: files}
}

func (d *Embed_Dir) M_len(tk *Task) int64 {
	return Map_len(tk, d.F_files)
}
