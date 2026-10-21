package rt

import (
	"sort"
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

// M_fs is Dir.fs (decision 0304): a read-only Fs over the files, as
// std/native/embed.xo's DirFs.
func (d *Embed_Dir) M_fs(tk *Task) Fs {
	return embedDirFs{files: d.F_files}
}

type embedDirFs struct {
	files  Map[string, Bytes]
	prefix string // "" or ending in "/"
	noRead bool   // write_only
}

// embedClean is path as a key of the root's files (dfs_clean); false when
// it climbs above the root.
func embedClean(p string) (string, bool) {
	var out []string
	for _, s := range strings.Split(p, "/") {
		switch s {
		case "", ".":
		case "..":
			if len(out) == 0 {
				return "", false
			}
			out = out[:len(out)-1]
		default:
			out = append(out, s)
		}
	}
	return strings.Join(out, "/"), true
}

func (d embedDirFs) key(p string) (string, bool) {
	c, ok := embedClean(p)
	if !ok {
		return "", false
	}
	return strings.TrimSuffix(d.prefix+c, "/"), true
}

func (d embedDirFs) isDir(k string) bool {
	if k == "" {
		return true
	}
	for f := range Map_All(d.files) {
		if strings.HasPrefix(f, k+"/") {
			return true
		}
	}
	return false
}

func (d embedDirFs) info(p, k string) Option[FileInfo] {
	name := strings.TrimSuffix(p, "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if b := Map_get(nil, d.files, k); b.Some {
		return Some(FileInfo{F_name: name, F_path: p, F_size: int64(len(b.V))})
	}
	if d.isDir(k) {
		return Some(FileInfo{F_name: name, F_path: p, F_is_dir: true})
	}
	return None[FileInfo]()
}

func (d embedDirFs) readKey(p string) (string, *FsErr) {
	if d.noRead {
		e := fsErr(6, p)
		return "", &e
	}
	k, ok := d.key(p)
	if !ok {
		e := fsErr(1, p)
		return "", &e
	}
	return k, nil
}

func embedDenied[T any](p string) Result[T, FsErr] { return Err[T](fsErr(6, p)) }

func (d embedDirFs) M_sub(tk *Task, dir string) Fs {
	k, ok := d.key(dir)
	switch {
	case !ok:
		return embedDirFs{noRead: d.noRead}
	case k == "":
		return d
	}
	d.prefix = k + "/"
	return d
}
func (d embedDirFs) M_cwd(tk *Task) Fs { return d }
func (d embedDirFs) M_cwd_path(tk *Task) Result[string, FsErr] {
	return Ok[string, FsErr]("/" + strings.TrimSuffix(d.prefix, "/"))
}
func (d embedDirFs) M_read_only(tk *Task) Fs        { return d }
func (d embedDirFs) M_write_only(tk *Task) Fs       { d.noRead = true; return d }
func (d embedDirFs) M_case_sensitive(tk *Task) bool { return true }

func (d embedDirFs) M_stat(tk *Task, p string) Result[Option[FileInfo], FsErr] {
	k, e := d.readKey(p)
	if e != nil {
		return Err[Option[FileInfo]](*e)
	}
	return Ok[Option[FileInfo], FsErr](d.info(p, k))
}

func (d embedDirFs) M_exists(tk *Task, p string) Result[bool, FsErr] {
	k, e := d.readKey(p)
	if e != nil {
		return Err[bool](*e)
	}
	return Ok[bool, FsErr](d.info(p, k).Some)
}

func (d embedDirFs) M_read(tk *Task, p string) Result[Bytes, FsErr] {
	k, e := d.readKey(p)
	if e != nil {
		return Err[Bytes](*e)
	}
	if b := Map_get(nil, d.files, k); b.Some {
		return Ok[Bytes, FsErr](b.V)
	}
	if d.isDir(k) {
		return Err[Bytes](fsErr(5, p))
	}
	return Err[Bytes](fsErr(1, p))
}

func (d embedDirFs) M_read_text(tk *Task, p string) Result[string, FsErr] {
	r := d.M_read(tk, p)
	if !r.Ok {
		return Err[string](r.E)
	}
	if !utf8.ValidString(string(r.V)) {
		return Err[string](fsIo(p, "not valid UTF-8"))
	}
	return Ok[string, FsErr](string(r.V))
}

// children are the names directly under the directory key k, sorted.
func (d embedDirFs) children(k string) []string {
	pre := ""
	if k != "" {
		pre = k + "/"
	}
	seen := map[string]bool{}
	var names []string
	for f := range Map_All(d.files) {
		if !strings.HasPrefix(f, pre) {
			continue
		}
		n, _, _ := strings.Cut(f[len(pre):], "/")
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

func embedJoin(dir, name string) string {
	switch {
	case dir == "" || dir == ".":
		return name
	case strings.HasSuffix(dir, "/"):
		return dir + name
	}
	return dir + "/" + name
}

func (d embedDirFs) M_read_dir(tk *Task, dir string) Result[List[FileInfo], FsErr] {
	k, e := d.readKey(dir)
	if e != nil {
		return Err[List[FileInfo]](*e)
	}
	if Map_contains(nil, d.files, k) {
		return Err[List[FileInfo]](fsErr(4, dir))
	}
	if !d.isDir(k) {
		return Err[List[FileInfo]](fsErr(1, dir))
	}
	var out []FileInfo
	for _, n := range d.children(k) {
		ck := n
		if k != "" {
			ck = k + "/" + n
		}
		if fi := d.info(embedJoin(dir, n), ck); fi.Some {
			out = append(out, fi.V)
		}
	}
	return Ok[List[FileInfo], FsErr](ListOf(out...))
}

func (d embedDirFs) M_walk(tk *Task, dir string) Result[List[FileInfo], FsErr] {
	r := d.M_read_dir(tk, dir)
	if !r.Ok {
		return r
	}
	var out []FileInfo
	for fi := range List_All(r.V) {
		out = append(out, fi)
		if fi.F_is_dir {
			w := d.M_walk(tk, fi.F_path)
			if !w.Ok {
				return w
			}
			for x := range List_All(w.V) {
				out = append(out, x)
			}
		}
	}
	return Ok[List[FileInfo], FsErr](ListOf(out...))
}

func (d embedDirFs) M_write(tk *Task, p string, b Bytes) Result[Unit, FsErr] {
	return embedDenied[Unit](p)
}
func (d embedDirFs) M_create_temp(tk *Task, dir, prefix string) Result[string, FsErr] {
	return embedDenied[string](dir)
}
func (d embedDirFs) M_rename(tk *Task, from, to string) Result[Unit, FsErr] {
	return embedDenied[Unit](from)
}
func (d embedDirFs) M_remove(tk *Task, p string) Result[Unit, FsErr] { return embedDenied[Unit](p) }
func (d embedDirFs) M_remove_all(tk *Task, p string) Result[Unit, FsErr] {
	return embedDenied[Unit](p)
}
func (d embedDirFs) M_create_dir_all(tk *Task, dir string) Result[Unit, FsErr] {
	return embedDenied[Unit](dir)
}
func (d embedDirFs) M_set_modified(tk *Task, p string, t Time) Result[Unit, FsErr] {
	return embedDenied[Unit](p)
}
func (d embedDirFs) M_open(tk *Task, p string) Result[File, FsErr]   { return embedDenied[File](p) }
func (d embedDirFs) M_create(tk *Task, p string) Result[File, FsErr] { return embedDenied[File](p) }
func (d embedDirFs) M_open_append(tk *Task, p string) Result[File, FsErr] {
	return embedDenied[File](p)
}
func (d embedDirFs) M_create_new(tk *Task, p string) Result[File, FsErr] {
	return embedDenied[File](p)
}
func (d embedDirFs) M_append(tk *Task, p string, b Bytes) Result[Unit, FsErr] {
	return embedDenied[Unit](p)
}
func (d embedDirFs) M_copy(tk *Task, from, to string) Result[Unit, FsErr] {
	return embedDenied[Unit](to)
}

func (d embedDirFs) M_mode(tk *Task, p string) Result[int64, FsErr] {
	r := d.M_stat(tk, p)
	switch {
	case !r.Ok:
		return Err[int64](r.E)
	case !r.V.Some:
		return Err[int64](fsErr(1, p))
	case r.V.V.F_is_dir:
		return Ok[int64, FsErr](0o555)
	}
	return Ok[int64, FsErr](0o444)
}

func (d embedDirFs) M_set_mode(tk *Task, p string, m int64) Result[Unit, FsErr] {
	return embedDenied[Unit](p)
}
func (d embedDirFs) M_symlink(tk *Task, target, link string) Result[Unit, FsErr] {
	return embedDenied[Unit](link)
}

func (d embedDirFs) M_read_link(tk *Task, p string) Result[Option[string], FsErr] {
	if _, e := d.readKey(p); e != nil {
		return Err[Option[string]](*e)
	}
	return Ok[Option[string], FsErr](None[string]())
}

func (d embedDirFs) M_glob(tk *Task, pattern string) Result[List[string], FsErr] {
	if d.noRead {
		return embedDenied[List[string]](pattern)
	}
	return fsGlob(tk, d, pattern)
}

func (d embedDirFs) M_lock(tk *Task, p string) Result[FileLock, FsErr] {
	return embedDenied[FileLock](p)
}
func (d embedDirFs) M_try_lock(tk *Task, p string) Result[Option[FileLock], FsErr] {
	return embedDenied[Option[FileLock]](p)
}
