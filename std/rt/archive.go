package rt

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"time"
)

// std/archive (std.md 18, decision 0164): gzip, tar, and zip in memory
// with Go's packages. Native builds report it as XO1201 (planned).

// M_display is `archive: msg`.
func (e Archive_ArchiveErr) M_display(*Task) string { return "archive: " + e.F_msg }

func archiveErr(msg string) Archive_ArchiveErr { return Archive_ArchiveErr{F_msg: msg} }

// Archive_file is `archive.file(path, data)`.
func Archive_file(_ *Task, p string, data Bytes) Archive_Entry {
	return Archive_Entry{F_path: p, F_data: data, F_mode: 0o644}
}

// Archive_dir is `archive.dir(path)`.
func Archive_dir(_ *Task, p string) Archive_Entry {
	return Archive_Entry{F_path: p, F_data: Bytes(""), F_mode: 0o755, F_is_dir: true}
}

// Archive_gzip is `archive.gzip(data)`. The header carries no name and no
// time, so the output depends on the data only.
func Archive_gzip(_ *Task, data Bytes) Bytes {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write([]byte(data))
	w.Close()
	return Bytes(b.String())
}

// Archive_gunzip is `archive.gunzip(data)`; concatenated members are read
// as one stream, as gzip(1) does.
func Archive_gunzip(_ *Task, data Bytes) Result[Bytes, Archive_ArchiveErr] {
	r, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return Err[Bytes](archiveErr("gunzip: " + err.Error()))
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return Err[Bytes](archiveErr("gunzip: " + err.Error()))
	}
	return Ok[Bytes, Archive_ArchiveErr](Bytes(out))
}

// archivePath checks an entry's path: relative, inside the archive's
// root, `/` separated; directories end without `/`.
func archivePath(p string) (string, bool) {
	p = strings.ReplaceAll(p, "\\", "/")
	if p == "" || strings.Contains(p, "\x00") || strings.HasPrefix(p, "/") || len(p) >= 2 && p[1] == ':' {
		return "", false
	}
	clean := path.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") || clean == "." {
		return "", false
	}
	return clean, true
}

func entryTime(e Archive_Entry) time.Time { return time.Unix(0, int64(e.F_modified)).UTC() }

// Archive_tar is `archive.tar(entries)`.
func Archive_tar(_ *Task, entries List[Archive_Entry]) Result[Bytes, Archive_ArchiveErr] {
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for _, e := range entries.s {
		p, ok := archivePath(e.F_path)
		if !ok {
			return Err[Bytes](archiveErr("bad entry path " + strconv.Quote(e.F_path)))
		}
		h := &tar.Header{Name: p, Mode: e.F_mode & 0o7777, ModTime: entryTime(e), Typeflag: tar.TypeReg, Size: int64(len(e.F_data)), Format: tar.FormatPAX}
		if e.F_is_dir {
			h.Typeflag, h.Name, h.Size = tar.TypeDir, p+"/", 0
		}
		if err := w.WriteHeader(h); err != nil {
			return Err[Bytes](archiveErr("tar: " + err.Error()))
		}
		if !e.F_is_dir {
			if _, err := w.Write([]byte(e.F_data)); err != nil {
				return Err[Bytes](archiveErr("tar: " + err.Error()))
			}
		}
	}
	if err := w.Close(); err != nil {
		return Err[Bytes](archiveErr("tar: " + err.Error()))
	}
	return Ok[Bytes, Archive_ArchiveErr](Bytes(b.String()))
}

// Archive_untar is `archive.untar(data)`.
func Archive_untar(_ *Task, data Bytes) Result[List[Archive_Entry], Archive_ArchiveErr] {
	r := tar.NewReader(strings.NewReader(string(data)))
	var out []Archive_Entry
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Err[List[Archive_Entry]](archiveErr("untar: " + err.Error()))
		}
		// a name from the archive: outside bytes (decision 0259)
		p, ok := archivePath(textOf(h.Name))
		if !ok {
			return Err[List[Archive_Entry]](archiveErr("untar: entry path " + strconv.Quote(h.Name) + archivePathWhy(h.Name)))
		}
		e := Archive_Entry{F_path: p, F_data: Bytes(""), F_mode: h.Mode & 0o7777, F_modified: Time(h.ModTime.UnixNano())}
		switch h.Typeflag {
		case tar.TypeDir:
			e.F_is_dir = true
		case tar.TypeXGlobalHeader:
			// describes the archive (`git archive` writes one holding
			// the commit), not an entry: skipped (std.md 18)
			continue
		case tar.TypeReg:
			b, err := io.ReadAll(r)
			if err != nil {
				return Err[List[Archive_Entry]](archiveErr("untar: " + err.Error()))
			}
			e.F_data = Bytes(b)
		default:
			return Err[List[Archive_Entry]](archiveErr("untar: entry " + strconv.Quote(h.Name) + " is not a file or a directory"))
		}
		out = append(out, e)
	}
	return Ok[List[Archive_Entry], Archive_ArchiveErr](ListOf(out...))
}

// Archive_zip is `archive.zip(entries)`.
func Archive_zip(_ *Task, entries List[Archive_Entry]) Result[Bytes, Archive_ArchiveErr] {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for i, e := range entries.s {
		p, ok := archivePath(e.F_path)
		if !ok {
			return Err[Bytes](archiveErr("bad entry path " + strconv.Quote(e.F_path)))
		}
		// the native backend writes no zip64 yet, so neither does this one
		if i >= 65535 {
			return Err[Bytes](archiveErr("zip: more than 65535 entries (zip64 is not supported)"))
		}
		if b.Len() > 4000000000 {
			return Err[Bytes](archiveErr("zip: archive larger than 4 GB (zip64 is not supported)"))
		}
		h := &zip.FileHeader{Name: p, Method: zip.Deflate, Modified: entryTime(e)}
		h.SetMode(fs.FileMode(e.F_mode & 0o777))
		if e.F_is_dir {
			h.Name, h.Method = p+"/", zip.Store
			h.SetMode(fs.ModeDir | fs.FileMode(e.F_mode&0o777))
		}
		fw, err := w.CreateHeader(h)
		if err != nil {
			return Err[Bytes](archiveErr("zip: " + err.Error()))
		}
		if !e.F_is_dir {
			if _, err := fw.Write([]byte(e.F_data)); err != nil {
				return Err[Bytes](archiveErr("zip: " + err.Error()))
			}
		}
	}
	if err := w.Flush(); err != nil {
		return Err[Bytes](archiveErr("zip: " + err.Error()))
	}
	if b.Len() > 4000000000 {
		return Err[Bytes](archiveErr("zip: archive larger than 4 GB (zip64 is not supported)"))
	}
	if err := w.Close(); err != nil {
		return Err[Bytes](archiveErr("zip: " + err.Error()))
	}
	return Ok[Bytes, Archive_ArchiveErr](Bytes(b.String()))
}

// Archive_unzip is `archive.unzip(data)`.
func Archive_unzip(_ *Task, data Bytes) Result[List[Archive_Entry], Archive_ArchiveErr] {
	r, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		return Err[List[Archive_Entry]](archiveErr("unzip: " + err.Error()))
	}
	var out []Archive_Entry
	for _, f := range r.File {
		// a name from the archive: outside bytes (decision 0259)
		p, ok := archivePath(textOf(f.Name))
		if !ok {
			return Err[List[Archive_Entry]](archiveErr("unzip: entry path " + strconv.Quote(f.Name) + archivePathWhy(f.Name)))
		}
		m := f.Mode()
		e := Archive_Entry{F_path: p, F_data: Bytes(""), F_mode: int64(m.Perm()), F_modified: Time(f.Modified.UnixNano())}
		switch {
		case m.IsDir():
			e.F_is_dir = true
		case m.IsRegular():
			rc, err := f.Open()
			if err != nil {
				return Err[List[Archive_Entry]](archiveErr("unzip: " + err.Error()))
			}
			b, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return Err[List[Archive_Entry]](archiveErr("unzip: " + err.Error()))
			}
			e.F_data = Bytes(b)
		default:
			return Err[List[Archive_Entry]](archiveErr("unzip: entry " + strconv.Quote(f.Name) + " is not a file or a directory"))
		}
		out = append(out, e)
	}
	return Ok[List[Archive_Entry], Archive_ArchiveErr](ListOf(out...))
}

// Archive_zip_reader is `archive.zip_reader(data)`: unzip's checks on every
// entry, none inflated. The Go backend keeps an entry's index in the
// ZipFile's offset, and reads an entry by opening the archive again.
func Archive_zip_reader(_ *Task, data Bytes) Result[Archive_ZipReader, Archive_ArchiveErr] {
	r, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		return Err[Archive_ZipReader](archiveErr("zip_reader: " + err.Error()))
	}
	var out []Archive_ZipFile
	for i, f := range r.File {
		p, ok := archivePath(textOf(f.Name))
		if !ok {
			return Err[Archive_ZipReader](archiveErr("zip_reader: entry path " + strconv.Quote(f.Name) + archivePathWhy(f.Name)))
		}
		m := f.Mode()
		if !m.IsDir() && !m.IsRegular() {
			return Err[Archive_ZipReader](archiveErr("zip_reader: entry " + strconv.Quote(f.Name) + " is not a file or a directory"))
		}
		out = append(out, Archive_ZipFile{F_path: p, F_size: int64(f.UncompressedSize64), F_compressed: int64(f.CompressedSize64), F_mode: int64(m.Perm()),
			F_is_dir: m.IsDir(), F_modified: Time(f.Modified.UnixNano()), F_offset: int64(i), F_method: int64(f.Method), F_flags: int64(f.Flags), F_crc: int64(f.CRC32)})
	}
	return Ok[Archive_ZipReader, Archive_ArchiveErr](Archive_ZipReader{F_data: data, F_list: ListOf(out...)})
}

func (r *Archive_ZipReader) M_files(*Task) List[Archive_ZipFile] { return r.F_list }

func (r *Archive_ZipReader) M_read(_ *Task, f Archive_ZipFile) Result[Bytes, Archive_ArchiveErr] {
	if f.F_is_dir {
		return Ok[Bytes, Archive_ArchiveErr](Bytes(""))
	}
	zr, err := zip.NewReader(strings.NewReader(string(r.F_data)), int64(len(r.F_data)))
	if err != nil || f.F_offset < 0 || int(f.F_offset) >= len(zr.File) {
		return Err[Bytes](archiveErr("zip_reader: not a file of this archive"))
	}
	rc, err := zr.File[f.F_offset].Open()
	if err != nil {
		return Err[Bytes](archiveErr("zip_reader: " + err.Error()))
	}
	b, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return Err[Bytes](archiveErr("zip_reader: " + err.Error()))
	}
	return Ok[Bytes, Archive_ArchiveErr](Bytes(b))
}

// Archive_ZipWriter is `archive.ZipWriter` (move only): Go's zip.Writer over
// the File, which a Write adapter feeds with the current task.
type Archive_ZipWriter struct {
	zw  *zip.Writer
	fw  *zipFileWriter
	cur io.Writer
	n   int
}

type zipFileWriter struct {
	f   File
	tk  *Task
	err string
}

func (z *zipFileWriter) Write(p []byte) (int, error) {
	if r := z.f.M_write(z.tk, Bytes(append([]byte(nil), p...))); !r.Ok {
		z.err = r.E.M_display(z.tk)
		return 0, io.ErrClosedPipe
	}
	return len(p), nil
}

func (Archive_ZipWriter) XoDebug(b *strings.Builder) { b.WriteString("<zip_writer>") }
func (x Archive_ZipWriter) M_debug(*Task) string     { return Debug(x) }

func Archive_zip_writer(_ *Task, f File) Archive_ZipWriter {
	fw := &zipFileWriter{f: f}
	return Archive_ZipWriter{zw: zip.NewWriter(fw), fw: fw}
}

func (x *Archive_ZipWriter) fail(tk *Task, err error) Result[Unit, Archive_ArchiveErr] {
	if x.fw.err != "" {
		return Err[Unit](archiveErr("zip_writer: " + x.fw.err))
	}
	return Err[Unit](archiveErr("zip_writer: " + err.Error()))
}

func (x *Archive_ZipWriter) start(tk *Task, name string, mode int64, isDir bool, mod time.Time) error {
	h := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: mod}
	h.SetMode(fs.FileMode(mode & 0o777))
	if isDir {
		h.Name = name + "/"
		h.Method = zip.Store
		h.SetMode(fs.ModeDir | fs.FileMode(mode&0o777))
	}
	// the native backend writes no zip64 yet, so neither does this one
	if x.n >= 65535 {
		return errors.New("more than 65535 entries (zip64 is not supported)")
	}
	x.fw.tk = tk
	w, err := x.zw.CreateHeader(h)
	x.cur = w
	if err == nil {
		x.n++
	}
	return err
}

func (x *Archive_ZipWriter) M_add(tk *Task, e Archive_Entry) Result[Unit, Archive_ArchiveErr] {
	p, ok := archivePath(textOf(e.F_path))
	if !ok {
		return Err[Unit](archiveErr("bad entry path " + strconv.Quote(e.F_path)))
	}
	if err := x.start(tk, p, e.F_mode, e.F_is_dir, entryTime(e)); err != nil {
		return x.fail(tk, err)
	}
	if !e.F_is_dir {
		if _, err := x.cur.Write([]byte(e.F_data)); err != nil {
			return x.fail(tk, err)
		}
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

func (x *Archive_ZipWriter) M_create(tk *Task, p string, mode int64) Result[Unit, Archive_ArchiveErr] {
	clean, ok := archivePath(textOf(p))
	if !ok {
		return Err[Unit](archiveErr("bad entry path " + strconv.Quote(p)))
	}
	if err := x.start(tk, clean, mode, false, time.Unix(0, 0).UTC()); err != nil {
		return x.fail(tk, err)
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

func (x *Archive_ZipWriter) M_write(tk *Task, data Bytes) Result[Unit, Archive_ArchiveErr] {
	if x.cur == nil {
		return Err[Unit](archiveErr("zip_writer: write without create"))
	}
	x.fw.tk = tk
	if _, err := x.cur.Write([]byte(data)); err != nil {
		return x.fail(tk, err)
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

func (x *Archive_ZipWriter) M_close(tk *Task) Result[Unit, Archive_ArchiveErr] {
	x.fw.tk = tk
	if err := x.zw.Close(); err != nil {
		return x.fail(tk, err)
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

// Archive_zip_open is `archive.zip_open(f, size)`: the Go backend reads the
// File whole through read_at and opens it as zip_reader does (its memory is
// the archive's; the native backends read the directory and each entry).
func Archive_zip_open(tk *Task, f File, size int64) Result[Archive_ZipReader, Archive_ArchiveErr] {
	var buf []byte
	for int64(len(buf)) < size {
		r := f.M_read_at(tk, int64(len(buf)), size-int64(len(buf)))
		if !r.Ok {
			return Err[Archive_ZipReader](archiveErr("zip_open: " + r.E.M_display(tk)))
		}
		if !r.V.Some || len(r.V.V) == 0 {
			return Err[Archive_ZipReader](archiveErr("zip_open: unexpected EOF"))
		}
		buf = append(buf, r.V.V...)
	}
	r := Archive_zip_reader(tk, Bytes(buf))
	if !r.Ok {
		return Err[Archive_ZipReader](archiveErr("zip_open:" + strings.TrimPrefix(r.E.F_msg, "zip_reader:")))
	}
	return r
}

// archivePathWhy is the end of an entry path error: a NUL byte is its own
// reason, every other refused name leaves the archive.
func archivePathWhy(name string) string {
	if strings.Contains(name, "\x00") {
		return " contains a NUL byte"
	}
	return " leaves the archive"
}
